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

// premiumStore abstracts premium-subscription persistence so tests can inject
// an in-memory stub without a live MongoDB connection.
type premiumStore interface {
	activate(userID, stripeCustomerID string) error
	extend(stripeCustomerID, userID string, periodEnd time.Time) error
	deactivateByCustomer(stripeCustomerID string) error
	checkPremium(ctx context.Context, userID string) (bool, error)
}

// premiums is the active store; defaults to mongoPremiumStore and is
// overridden in tests.
var premiums premiumStore = mongoPremiumStore{}

// mongoPremiumStore delegates to the MongoDB-backed helpers below.
type mongoPremiumStore struct{}

func (mongoPremiumStore) activate(userID, stripeCustomerID string) error {
	return activatePremium(userID, stripeCustomerID)
}
func (mongoPremiumStore) extend(stripeCustomerID, userID string, periodEnd time.Time) error {
	return extendPremium(stripeCustomerID, userID, periodEnd)
}
func (mongoPremiumStore) deactivateByCustomer(stripeCustomerID string) error {
	return deactivatePremiumByCustomer(stripeCustomerID)
}
func (mongoPremiumStore) checkPremium(ctx context.Context, userID string) (bool, error) {
	return isPremium(ctx, userID)
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
		// Copy the user onto the subscription so renewal invoices
		// (invoice.payment_succeeded) can be attributed back to the user.
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{"user_id": req.UserID},
		},
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
			if err := premiums.activate(userID, customerID); err != nil {
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
			if err := premiums.deactivateByCustomer(customerID); err != nil {
				log.Printf("stripe webhook: deactivatePremium error: %v", err)
			}
		}

	case "invoice.payment_succeeded":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			break
		}
		customerID := ""
		if inv.Customer != nil {
			customerID = inv.Customer.ID
		}
		userID := ""
		if inv.SubscriptionDetails != nil {
			userID = inv.SubscriptionDetails.Metadata["user_id"]
		}
		if customerID == "" && userID == "" {
			log.Printf("stripe webhook: invoice %s has no customer or user_id metadata", inv.ID)
			break
		}
		if err := premiums.extend(customerID, userID, invoicePeriodEnd(&inv)); err != nil {
			log.Printf("stripe webhook: extendPremium error for customer %s: %v", customerID, err)
		}

	case "invoice.payment_failed":
		log.Printf("stripe webhook: payment failed for event %s", event.ID)
	}

	c.SetStatusCode(200)
	c.WriteString("ok")
	return nil
}

// activatePremium links the user to their Stripe customer and grants a short
// bridge entitlement. The invoice.payment_succeeded webhook for the first
// invoice follows immediately and sets premium_until to the real billing
// period end (see extendPremium).
func activatePremium(userID, stripeCustomerID string) error {
	until := time.Now().Add(7 * 24 * time.Hour)
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

// invoicePeriodEnd returns the latest billing-period end covered by the
// invoice's line items, falling back to the invoice's own period when no
// lines are present.
func invoicePeriodEnd(inv *stripe.Invoice) time.Time {
	var end int64
	if inv.Lines != nil {
		for _, line := range inv.Lines.Data {
			if line.Period != nil && line.Period.End > end {
				end = line.Period.End
			}
		}
	}
	if end == 0 {
		end = inv.PeriodEnd
	}
	return time.Unix(end, 0)
}

// extendPremium pushes premium_until out to the paid period's end plus a grace
// window covering renewal-payment delays. $max guarantees a replayed or
// out-of-order webhook never shortens an existing entitlement.
func extendPremium(stripeCustomerID, userID string, periodEnd time.Time) error {
	until := periodEnd.Add(72 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), mongoUpdateTimeout)
	defer cancel()

	col := mongoClient.Database("Gamium").Collection("premium_users")

	if stripeCustomerID != "" {
		res, err := col.UpdateOne(ctx,
			bson.D{{Key: "stripe_customer_id", Value: stripeCustomerID}},
			bson.D{{Key: "$max", Value: bson.D{{Key: "premium_until", Value: until}}}},
		)
		if err != nil {
			return err
		}
		if res.MatchedCount > 0 {
			return nil
		}
	}

	// No document for this customer yet — the first invoice can arrive before
	// checkout.session.completed. Fall back to the user_id carried on the
	// subscription metadata and upsert, recording the customer ID.
	if userID == "" {
		return fmt.Errorf("no premium_users match for customer %q and no user_id metadata", stripeCustomerID)
	}
	set := bson.D{{Key: "user_id", Value: userID}}
	if stripeCustomerID != "" {
		set = append(set, bson.E{Key: "stripe_customer_id", Value: stripeCustomerID})
	}
	_, err := col.UpdateOne(ctx,
		bson.D{{Key: "user_id", Value: userID}},
		bson.D{
			{Key: "$max", Value: bson.D{{Key: "premium_until", Value: until}}},
			{Key: "$set", Value: set},
		},
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
