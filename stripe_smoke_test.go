package main

// E2E smoke test for the Stripe premium flow, fully in-process:
//   non-premium gate → checkout.session.completed webhook → premium active
//   → export unblocked → invoice.payment_succeeded extends premium
//   → subscription cancelled → premium revoked → export gated again
//
// No live Stripe API, no real database. The in-memory premiumStore is swapped
// in for the duration of the test; mongoClient is initialised to a stub that
// fails quickly so exportHistory doesn't panic after the gate passes.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------------------------------------------------------------------------
// In-memory premiumStore
// ---------------------------------------------------------------------------

type memPremiumStore struct {
	mu        sync.Mutex
	records   map[string]time.Time // userID → premiumUntil
	customers map[string]string    // stripeCustomerID → userID
}

func newMemPremiumStore() *memPremiumStore {
	return &memPremiumStore{
		records:   make(map[string]time.Time),
		customers: make(map[string]string),
	}
}

func (s *memPremiumStore) activate(userID, customerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[userID] = time.Now().Add(7 * 24 * time.Hour)
	if customerID != "" {
		s.customers[customerID] = userID
	}
	return nil
}

func (s *memPremiumStore) extend(customerID, userID string, periodEnd time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if customerID != "" {
		if uid, ok := s.customers[customerID]; ok {
			userID = uid
		}
	}
	if userID == "" {
		return nil
	}
	until := periodEnd.Add(72 * time.Hour)
	if t, ok := s.records[userID]; !ok || until.After(t) {
		s.records[userID] = until
	}
	if customerID != "" {
		s.customers[customerID] = userID
	}
	return nil
}

func (s *memPremiumStore) deactivateByCustomer(customerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if uid, ok := s.customers[customerID]; ok {
		s.records[uid] = time.Now().Add(-time.Second)
	}
	return nil
}

func (s *memPremiumStore) checkPremium(_ context.Context, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.records[userID]
	return ok && t.After(time.Now()), nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// signStripePayload builds a valid Stripe-Signature header value.
// Stripe computes HMAC-SHA256 over "<unix>.<payload>" using the full secret
// string (including the "whsec_" prefix) as the key.
func signStripePayload(payload []byte, secret string, ts time.Time) string {
	body := fmt.Sprintf("%d.%s", ts.Unix(), string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return fmt.Sprintf("t=%d,v1=%x", ts.Unix(), mac.Sum(nil))
}

func doWebhookReq(payload []byte, sig string) *fasthttp.RequestCtx {
	r := routing.New()
	r.Post("/api/stripe/webhook", handleStripeWebhook)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/api/stripe/webhook")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.Set("Stripe-Signature", sig)
	ctx.Request.SetBody(payload)
	r.HandleRequest(ctx)
	return ctx
}

func doExportReq(userID string) *fasthttp.RequestCtx {
	r := routing.New()
	r.Get("/u/export", exportHistory)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/u/export?user_id=" + userID)
	ctx.Request.Header.SetMethod("GET")
	r.HandleRequest(ctx)
	return ctx
}

// ---------------------------------------------------------------------------
// Event payload builders
// ---------------------------------------------------------------------------

const smokeWebhookSecret = "whsec_smoke_test_secret"
const smokeUserID = "user_smoke_001"
const smokeCustomerID = "cus_smoke_001"

func checkoutCompletedEvent(userID, customerID string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"id":          "evt_smoke_co",
		"object":      "event",
		"type":        "checkout.session.completed",
		"api_version": "2023-10-16",
		"created":     time.Now().Unix(),
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":       "cs_smoke_001",
				"object":   "checkout.session",
				"customer": customerID,
				"metadata": map[string]string{"user_id": userID},
			},
		},
	})
	return b
}

func invoicePaymentSucceededEvent(customerID, userID string, periodEnd time.Time) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"id":          "evt_smoke_inv",
		"object":      "event",
		"type":        "invoice.payment_succeeded",
		"api_version": "2023-10-16",
		"created":     time.Now().Unix(),
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":         "in_smoke_001",
				"object":     "invoice",
				"customer":   customerID,
				"period_end": periodEnd.Unix(),
				"subscription_details": map[string]interface{}{
					"metadata": map[string]string{"user_id": userID},
				},
				"lines": map[string]interface{}{
					"object": "list",
					"data":   []interface{}{},
				},
			},
		},
	})
	return b
}

func subscriptionDeletedEvent(customerID string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"id":          "evt_smoke_del",
		"object":      "event",
		"type":        "customer.subscription.deleted",
		"api_version": "2023-10-16",
		"created":     time.Now().Unix(),
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":       "sub_smoke_001",
				"object":   "subscription",
				"customer": customerID,
			},
		},
	})
	return b
}

// ---------------------------------------------------------------------------
// Smoke test
// ---------------------------------------------------------------------------

// TestStripeSmoke exercises the full premium flow without external services.
func TestStripeSmoke(t *testing.T) {
	store := newMemPremiumStore()
	oldPremiums := premiums
	premiums = store
	t.Cleanup(func() { premiums = oldPremiums })

	t.Setenv("STRIPE_WEBHOOK_SECRET", smokeWebhookSecret)

	// Provide a non-nil mongoClient so exportHistory doesn't panic when the
	// scrobble query runs after the premium gate. The stub client points at a
	// non-existent server and fails in ~25 ms rather than hanging.
	oldMC := mongoClient
	if mongoClient == nil {
		opts := options.Client().
			ApplyURI("mongodb://localhost:27017/").
			SetServerSelectionTimeout(25 * time.Millisecond)
		c, err := mongo.Connect(context.Background(), opts)
		if err == nil {
			mongoClient = c
			t.Cleanup(func() {
				c.Disconnect(context.Background()) //nolint
				mongoClient = oldMC
			})
		}
	}

	// 1. Export is gated — non-premium user receives 402.
	t.Run("export_gated_before_checkout", func(t *testing.T) {
		ctx := doExportReq(smokeUserID)
		if ctx.Response.StatusCode() != 402 {
			t.Fatalf("expected 402 for non-premium user, got %d: %s",
				ctx.Response.StatusCode(), ctx.Response.Body())
		}
	})

	// 2. Simulate checkout.session.completed → premium activated.
	t.Run("checkout_session_completed_webhook", func(t *testing.T) {
		payload := checkoutCompletedEvent(smokeUserID, smokeCustomerID)
		sig := signStripePayload(payload, smokeWebhookSecret, time.Now())
		ctx := doWebhookReq(payload, sig)
		if ctx.Response.StatusCode() != 200 {
			t.Fatalf("webhook returned %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
		}
	})

	// 3. Premium flag is set in the store.
	t.Run("premium_active_after_checkout", func(t *testing.T) {
		ok, err := store.checkPremium(context.Background(), smokeUserID)
		if err != nil || !ok {
			t.Fatalf("expected premium active: ok=%v err=%v", ok, err)
		}
	})

	// 4. invoice.payment_succeeded extends the premium period.
	t.Run("invoice_payment_extends_premium", func(t *testing.T) {
		futureEnd := time.Now().Add(30 * 24 * time.Hour)
		payload := invoicePaymentSucceededEvent(smokeCustomerID, smokeUserID, futureEnd)
		sig := signStripePayload(payload, smokeWebhookSecret, time.Now())
		ctx := doWebhookReq(payload, sig)
		if ctx.Response.StatusCode() != 200 {
			t.Fatalf("invoice webhook returned %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
		}
		ok, err := store.checkPremium(context.Background(), smokeUserID)
		if err != nil || !ok {
			t.Fatalf("expected premium active after invoice: ok=%v err=%v", ok, err)
		}
	})

	// 5. Export is no longer gated — premium user passes the 402 check.
	//    The handler will return 500 because the stub mongoClient has no
	//    scrobble data, but it must not return 402.
	t.Run("export_unblocked_after_premium", func(t *testing.T) {
		ctx := doExportReq(smokeUserID)
		if ctx.Response.StatusCode() == 402 {
			t.Fatal("premium user must not receive 402 on export")
		}
	})

	// 6. Subscription cancelled → premium revoked.
	t.Run("subscription_deleted_revokes_premium", func(t *testing.T) {
		payload := subscriptionDeletedEvent(smokeCustomerID)
		sig := signStripePayload(payload, smokeWebhookSecret, time.Now())
		ctx := doWebhookReq(payload, sig)
		if ctx.Response.StatusCode() != 200 {
			t.Fatalf("cancel webhook returned %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
		}
		ok, err := store.checkPremium(context.Background(), smokeUserID)
		if err != nil || ok {
			t.Fatalf("expected premium inactive after cancel: ok=%v err=%v", ok, err)
		}
	})

	// 7. Export is gated again after cancellation.
	t.Run("export_gated_after_cancellation", func(t *testing.T) {
		ctx := doExportReq(smokeUserID)
		if ctx.Response.StatusCode() != 402 {
			t.Fatalf("expected 402 after cancellation, got %d", ctx.Response.StatusCode())
		}
	})
}
