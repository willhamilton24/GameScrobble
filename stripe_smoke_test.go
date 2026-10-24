package main

// In-process webhook and premium gating coverage:
//   non-premium gate → checkout.session.completed webhook → premium active
//   → export unblocked → invoice.payment_succeeded extends premium
//   → subscription cancelled → premium revoked → export gated again
//
// No signup, checkout creation, or Stripe delivery is exercised. Premium state
// and export data are injected in memory; no external services are contacted.

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
		"api_version": "2026-09-30.endive",
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
		"api_version": "2026-09-30.endive",
		"created":     time.Now().Unix(),
		"data": map[string]interface{}{
			"object": map[string]interface{}{
				"id":         "in_smoke_001",
				"object":     "invoice",
				"customer":   customerID,
				"period_end": periodEnd.Unix(),
				"parent": map[string]interface{}{
					"type": "subscription_details",
					"subscription_details": map[string]interface{}{
						"metadata": map[string]string{"user_id": userID},
					},
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
		"api_version": "2026-09-30.endive",
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

// TestStripeSmoke exercises webhook handling and export gating in-process.
func TestStripeSmoke(t *testing.T) {
	store := newMemPremiumStore()
	oldPremiums := premiums
	premiums = store
	t.Cleanup(func() { premiums = oldPremiums })

	t.Setenv("STRIPE_WEBHOOK_SECRET", smokeWebhookSecret)

	oldFindExportHistory := findExportHistory
	findExportHistory = func(_ context.Context, userID string) ([]exportRow, error) {
		if userID != smokeUserID {
			t.Fatalf("unexpected export user: %q", userID)
		}
		// LastPolled is zero so the CSV last_polled column is empty.
		return []exportRow{
			{AppID: 440, Name: "Team Fortress 2", PlaytimeTotal: 120, PlaytimeRecent: 30},
			{AppID: 570, Name: "Game, with comma", PlaytimeTotal: 90, PlaytimeRecent: 15},
		}, nil
	}
	t.Cleanup(func() { findExportHistory = oldFindExportHistory })

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
		store.mu.Lock()
		previousUntil := store.records[smokeUserID]
		store.mu.Unlock()
		futureEnd := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
		payload := invoicePaymentSucceededEvent(smokeCustomerID, smokeUserID, futureEnd)
		sig := signStripePayload(payload, smokeWebhookSecret, time.Now())
		ctx := doWebhookReq(payload, sig)
		if ctx.Response.StatusCode() != 200 {
			t.Fatalf("invoice webhook returned %d: %s", ctx.Response.StatusCode(), ctx.Response.Body())
		}
		store.mu.Lock()
		gotUntil := store.records[smokeUserID]
		store.mu.Unlock()
		wantUntil := futureEnd.Add(72 * time.Hour)
		if !gotUntil.Equal(wantUntil) || !gotUntil.After(previousUntil) {
			t.Fatalf("expected expiration to advance from %s to %s, got %s",
				previousUntil, wantUntil, gotUntil)
		}
	})

	// 5. Premium users receive a successful CSV export.
	t.Run("export_unblocked_after_premium", func(t *testing.T) {
		ctx := doExportReq(smokeUserID)
		if ctx.Response.StatusCode() != 200 {
			t.Fatalf("expected CSV export status 200, got %d: %s",
				ctx.Response.StatusCode(), ctx.Response.Body())
		}
		if got := string(ctx.Response.Header.ContentType()); got != "text/csv" {
			t.Fatalf("unexpected Content-Type: %q", got)
		}
		if got := string(ctx.Response.Header.Peek("Content-Disposition")); got != `attachment; filename="gameroom_history.csv"` {
			t.Fatalf("unexpected Content-Disposition: %q", got)
		}
		wantCSV := "app_id,name,playtime_total_minutes,playtime_recent_minutes,last_polled\n" +
			"440,Team Fortress 2,120,30,\n" +
			"570,\"Game, with comma\",90,15,\n"
		if got := string(ctx.Response.Body()); got != wantCSV {
			t.Fatalf("unexpected CSV contents: got %q, want %q", got, wantCSV)
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

func TestStripeRejectsInvalidSignaturesAndVersions(t *testing.T) {
	t.Setenv("STRIPE_WEBHOOK_SECRET", smokeWebhookSecret)
	payload := checkoutCompletedEvent(smokeUserID, smokeCustomerID)
	for name, sig := range map[string]string{
		"missing":      "",
		"wrong secret": signStripePayload(payload, "wrong-test-secret", time.Now()),
		"expired":      signStripePayload(payload, smokeWebhookSecret, time.Now().Add(-10*time.Minute)),
	} {
		t.Run(name, func(t *testing.T) {
			if got := doWebhookReq(payload, sig).Response.StatusCode(); got != 400 {
				t.Fatalf("got %d, want 400", got)
			}
		})
	}
	var event map[string]interface{}
	json.Unmarshal(payload, &event)
	event["api_version"] = "2023-10-16"
	payload, _ = json.Marshal(event)
	if got := doWebhookReq(payload, signStripePayload(payload, smokeWebhookSecret, time.Now())).Response.StatusCode(); got != 400 {
		t.Fatalf("incompatible version: got %d, want 400", got)
	}
}

func TestStripeInvoiceBeforeCheckoutUsesParentMetadata(t *testing.T) {
	store := newMemPremiumStore()
	previous := premiums
	premiums = store
	t.Cleanup(func() { premiums = previous })
	t.Setenv("STRIPE_WEBHOOK_SECRET", smokeWebhookSecret)
	end := time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second)
	payload := invoicePaymentSucceededEvent(smokeCustomerID, smokeUserID, end)
	ctx := doWebhookReq(payload, signStripePayload(payload, smokeWebhookSecret, time.Now()))
	if ctx.Response.StatusCode() != 200 {
		t.Fatalf("invoice failed: %s", ctx.Response.Body())
	}
	if got := store.records[smokeUserID]; !got.Equal(end.Add(72 * time.Hour)) {
		t.Fatalf("parent metadata not applied: got %v", got)
	}
}

type failingPremiumStore struct{ *memPremiumStore }

func (failingPremiumStore) activate(string, string) error {
	return fmt.Errorf("test database unavailable")
}
func (failingPremiumStore) extend(string, string, time.Time) error {
	return fmt.Errorf("test database unavailable")
}
func (failingPremiumStore) deactivateByCustomer(string) error {
	return fmt.Errorf("test database unavailable")
}

func TestStripeRetriesPersistenceFailures(t *testing.T) {
	previous := premiums
	premiums = failingPremiumStore{newMemPremiumStore()}
	t.Cleanup(func() { premiums = previous })
	t.Setenv("STRIPE_WEBHOOK_SECRET", smokeWebhookSecret)
	for _, payload := range [][]byte{
		checkoutCompletedEvent(smokeUserID, smokeCustomerID),
		invoicePaymentSucceededEvent(smokeCustomerID, smokeUserID, time.Now().Add(30*24*time.Hour)),
		subscriptionDeletedEvent(smokeCustomerID),
	} {
		ctx := doWebhookReq(payload, signStripePayload(payload, smokeWebhookSecret, time.Now()))
		if ctx.Response.StatusCode() != 500 {
			t.Fatalf("got %d, want retryable 500", ctx.Response.StatusCode())
		}
	}
}
