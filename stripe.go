package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	routing "github.com/qiangxue/fasthttp-routing"
	stripe "github.com/stripe/stripe-go/v87"
	"github.com/stripe/stripe-go/v87/checkout/session"
	stripewh "github.com/stripe/stripe-go/v87/webhook"
)

// PremiumUser tracks active premium subscriptions linked to a user.
type PremiumUser struct {
	UserID           string
	PremiumUntil     time.Time
	StripeCustomerID string
}

// premiumStore abstracts premium-subscription persistence so tests can inject
// an in-memory stub without a live database connection.
type premiumStore interface {
	activate(userID, stripeCustomerID string) error
	extend(stripeCustomerID, userID string, periodEnd time.Time) error
	deactivateByCustomer(stripeCustomerID string) error
	checkPremium(ctx context.Context, userID string) (bool, error)
}

// premiums is the active store; defaults to pgPremiumStore and is
// overridden in tests.
var premiums premiumStore = pgPremiumStore{}

// pgPremiumStore delegates to the Postgres-backed helpers below.
type pgPremiumStore struct{}

func (pgPremiumStore) activate(userID, stripeCustomerID string) error {
	return activatePremium(userID, stripeCustomerID)
}
func (pgPremiumStore) extend(stripeCustomerID, userID string, periodEnd time.Time) error {
	return extendPremium(stripeCustomerID, userID, periodEnd)
}
func (pgPremiumStore) deactivateByCustomer(stripeCustomerID string) error {
	return deactivatePremiumByCustomer(stripeCustomerID)
}
func (pgPremiumStore) checkPremium(ctx context.Context, userID string) (bool, error) {
	return isPremium(ctx, userID)
}

func stripeKey() string           { return os.Getenv("STRIPE_SECRET_KEY") }
func stripeWebhookSecret() string { return os.Getenv("STRIPE_WEBHOOK_SECRET") }
func premiumPriceID() string      { return os.Getenv("STRIPE_PREMIUM_PRICE_ID") }

func appBaseURL() string {
	if u := os.Getenv("APP_BASE_URL"); u != "" {
		return u
	}
	return "https://gameroom.hdynamic.app"
}

// createCheckoutSession creates a Stripe Checkout session for a premium subscription.
// Derives the user from the authenticated session; body is ignored.
func createCheckoutSession(c *routing.Context) error {
	userID, _ := c.UserValue(authenticatedUserIDKey{}).(string)
	if userID == "" {
		c.SetStatusCode(401)
		c.SetContentType("application/json")
		c.WriteString(`{"error":"authentication required"}`)
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
		Metadata:   map[string]string{"user_id": userID},
		// Copy the user onto the subscription so renewal invoices
		// (invoice.payment_succeeded) can be attributed back to the user.
		SubscriptionData: &stripe.CheckoutSessionSubscriptionDataParams{
			Metadata: map[string]string{"user_id": userID},
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
		log.Printf("stripe webhook: rejected: %v", err)
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
				return stripeProcessingError(c, event.ID, err)
			}
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return stripeProcessingError(c, event.ID, err)
		}
		customerID := ""
		if sub.Customer != nil {
			customerID = sub.Customer.ID
		}
		if customerID != "" {
			if err := premiums.deactivateByCustomer(customerID); err != nil {
				return stripeProcessingError(c, event.ID, err)
			}
		}

	case "invoice.payment_succeeded":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return stripeProcessingError(c, event.ID, err)
		}
		customerID := ""
		if inv.Customer != nil {
			customerID = inv.Customer.ID
		}
		userID := ""
		if inv.Parent != nil && inv.Parent.SubscriptionDetails != nil {
			userID = inv.Parent.SubscriptionDetails.Metadata["user_id"]
		}
		if customerID == "" && userID == "" {
			log.Printf("stripe webhook: invoice %s has no customer or user_id metadata", inv.ID)
			break
		}
		if err := premiums.extend(customerID, userID, invoicePeriodEnd(&inv)); err != nil {
			return stripeProcessingError(c, event.ID, err)
		}

	case "invoice.payment_failed":
		log.Printf("stripe webhook: payment failed for event %s", event.ID)
	}

	log.Printf("stripe webhook: processed event=%s type=%s", event.ID, event.Type)
	c.SetStatusCode(200)
	c.WriteString("ok")
	return nil
}

// Acknowledgement follows persistence so Stripe can retry failed writes.
func stripeProcessingError(c *routing.Context, eventID string, err error) error {
	log.Printf("stripe webhook: processing failed event=%s: %v", eventID, err)
	c.SetStatusCode(500)
	c.WriteString("webhook processing failed")
	return nil
}

// activatePremium links the user to their Stripe customer and grants a short
// bridge entitlement. The invoice.payment_succeeded webhook for the first
// invoice follows immediately and sets premium_until to the real billing
// period end (see extendPremium).
func activatePremium(userID, stripeCustomerID string) error {
	until := time.Now().Add(7 * 24 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	_, err := db.Exec(ctx, `
		INSERT INTO premium_users (user_id, premium_until, stripe_customer_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			premium_until      = GREATEST(premium_users.premium_until, $2),
			stripe_customer_id = $3
	`, userID, until, stripeCustomerID)
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
// window covering renewal-payment delays. GREATEST guarantees a replayed or
// out-of-order webhook never shortens an existing entitlement.
func extendPremium(stripeCustomerID, userID string, periodEnd time.Time) error {
	until := periodEnd.Add(72 * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	if stripeCustomerID != "" {
		tag, err := db.Exec(ctx, `
			UPDATE premium_users
			SET premium_until = GREATEST(premium_until, $1)
			WHERE stripe_customer_id = $2
		`, until, stripeCustomerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			return nil
		}
	}

	// No document for this customer yet — the first invoice can arrive before
	// checkout.session.completed. Fall back to the user_id carried on the
	// subscription metadata and upsert, recording the customer ID.
	if userID == "" {
		return fmt.Errorf("no premium_users match for customer %q and no user_id metadata", stripeCustomerID)
	}
	_, err := db.Exec(ctx, `
		INSERT INTO premium_users (user_id, premium_until, stripe_customer_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			premium_until      = GREATEST(premium_users.premium_until, $2),
			stripe_customer_id = CASE WHEN $3 <> '' THEN $3 ELSE premium_users.stripe_customer_id END
	`, userID, until, stripeCustomerID)
	return err
}

// deactivatePremiumByCustomer expires the premium entitlement when a subscription is cancelled.
func deactivatePremiumByCustomer(stripeCustomerID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()

	_, err := db.Exec(ctx,
		`UPDATE premium_users SET premium_until = NOW() WHERE stripe_customer_id = $1`,
		stripeCustomerID,
	)
	return err
}

// isPremium returns true when the user has an active premium subscription.
func isPremium(ctx context.Context, userID string) (bool, error) {
	c, cancel := context.WithTimeout(ctx, dbTimeout)
	defer cancel()

	var until time.Time
	err := db.QueryRow(c, `SELECT premium_until FROM premium_users WHERE user_id = $1`, userID).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return until.After(time.Now()), nil
}
