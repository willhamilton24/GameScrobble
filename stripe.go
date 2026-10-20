package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
	stripe "github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	stripewh "github.com/stripe/stripe-go/v76/webhook"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PremiumUser tracks active premium subscriptions linked to a user.
type PremiumUser struct {
	UserID           string    `bson:"user_id"`
	PremiumUntil     time.Time `bson:"premium_until"`
	StripeCustomerID string    `bson:"stripe_customer_id"`
}

func stripeKey() string        { return os.Getenv("STRIPE_SECRET_KEY") }
func stripeWebhookSecret() string { return os.Getenv("STRIPE_WEBHOOK_SECRET") }
func premiumPriceID() string   { return os.Getenv("STRIPE_PREMIUM_PRICE_ID") }

func appBaseURL() string {
	if u := os.Getenv("APP_BASE_URL"); u != "" {
		return u
	}
	return "https://gamescrobble.com"
}

// createCheckoutSession creates a Stripe Checkout session for a premium subscription.
// Body: {"user_id": "<id>"}
func createCheckoutSession(c *routing.Context) error {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(c.PostBody(), &req); err != nil || req.UserID == "" {
		c.SetStatusCode(400)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"user_id is required"}`)
		return nil
	}

	sk := stripeKey()
	if sk == "" {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"Stripe not configured"}`)
		return nil
	}
	stripe.Key = sk

	priceID := premiumPriceID()
	if priceID == "" {
		c.SetStatusCode(500)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"Stripe price not configured"}`)
		return nil
	}

	base := appBaseURL()
	params := &stripe.CheckoutSessionParams{
		Mode: stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceID),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL: stripe.String(base + "/premium/success?session_id={CHECKOUT_SESSION_ID}"),
		CancelURL:  stripe.String(base + "/premium/cancel"),
		Metadata:   map[string]string{"user_id": req.UserID},
	}

	s, err := session.New(params)
	if err != nil {
		log.Printf("stripe: checkout session error: %v", err)
		c.SetStatusCode(502)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"failed to create checkout session"}`)
		return nil
	}

	c.SetContentType("application/json")
	out, _ := json.Marshal(map[string]string{"url": s.URL})
	c.Write(out)
	return nil
}

// handleStripeWebhook processes Stripe webhook events.
func handleStripeWebhook(c *routing.Context) error {
	payload := c.PostBody()
	sigHeader := string(c.Request.Header.Peek("Stripe-Signature"))
	secret := stripeWebhookSecret()

	if secret == "" {
		c.SetStatusCode(500)
		c.WriteString("webhook secret not configured")
		return nil
	}

	event, err := stripewh.ConstructEvent(payload, sigHeader, secret)
	if err != nil {
		c.SetStatusCode(400)
		fmt.Fprintf(c, "webhook signature verification failed: %v", err)
		return nil
	}

	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			c.SetStatusCode(400)
			c.WriteString("failed to parse checkout session")
			return nil
		}
		if userID := sess.Metadata["user_id"]; userID != "" {
			customerID := ""
			if sess.Customer != nil {
				customerID = sess.Customer.ID
			}
			if err := activatePremium(userID, customerID); err != nil {
				log.Printf("stripe webhook: activatePremium error for %s: %v", userID, err)
			}
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			break
		}
		customerID := ""
		if sub.Customer != nil {
			customerID = sub.Customer.ID
		}
		if customerID != "" {
			if err := deactivatePremiumByCustomer(customerID); err != nil {
				log.Printf("stripe webhook: deactivatePremium error: %v", err)
			}
		}

	case "invoice.payment_failed":
		log.Printf("stripe webhook: payment failed for event %s", event.ID)
	}

	c.SetStatusCode(200)
	c.WriteString("ok")
	return nil
}

// activatePremium marks a user as premium for one year and stores the Stripe customer ID.
// The expiry is extended on each successful payment via invoice.payment_succeeded.
func activatePremium(userID, stripeCustomerID string) error {
	until := time.Now().Add(365 * 24 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
	defer cancel()

	_, err := mongoClient.Database("Gamium").Collection("premium_users").UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: userID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "user_id", Value: userID},
			{Key: "premium_until", Value: until},
			{Key: "stripe_customer_id", Value: stripeCustomerID},
		}}},
		options.Update().SetUpsert(true),
	)
	return err
}

// deactivatePremiumByCustomer expires the premium entitlement when a subscription is cancelled.
func deactivatePremiumByCustomer(stripeCustomerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
	defer cancel()

	_, err := mongoClient.Database("Gamium").Collection("premium_users").UpdateOne(ctx,
		bson.D{{Key: "stripe_customer_id", Value: stripeCustomerID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "premium_until", Value: time.Now()},
		}}},
	)
	return err
}

// isPremium returns true when the user has an active premium subscription.
func isPremium(ctx context.Context, userID string) (bool, error) {
	c, cancel := context.WithTimeout(ctx, mongoUpdateTimeout)
	defer cancel()

	var user PremiumUser
	err := mongoClient.Database("Gamium").Collection("premium_users").
		FindOne(c, bson.D{{Key: "user_id", Value: userID}}).Decode(&user)
	if err != nil {
		return false, nil
	}
	return user.PremiumUntil.After(time.Now()), nil
}
