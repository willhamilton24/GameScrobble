package main

// TestStripeE2E requires a separate browser step: complete the application's
// hosted test Checkout while this test polls. The Go command alone is not E2E.
// Supply STRIPE_SECRET_KEY through the environment, then run:
// RUN_STRIPE_E2E=1 go test -run '^TestStripeE2E$' -v -count=1 -timeout 15m
// The checkout URL is written to a temporary private file, never diagnostics.
//
// Monkey's reported deployed application-flow verification (2026-10-24):
// session cs_test_a1zwCaEuIPcO1ZATHRfg0Nkn4869FAMmp7fm7BxtNMcwICPYHqRFO4p5bM,
// checkout event evt_1UTwetGRjKD6n9dm6QH90A4p,
// subscription sub_1UTwesGRjKD6n9dmuswB4Tgq,
// cancellation event evt_1UTwfiGRjKD6n9dmfSdPdoHy.
// This historical evidence is not an execution of this revised test; see README.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	stripe "github.com/stripe/stripe-go/v87"
)

const e2eAppURL = "https://gameroom.hdynamic.app"
const e2ePriceID = "price_1USUwuGRjKD6n9dmujp5uGyk"
const e2eWebhookID = "we_1USUwxGRjKD6n9dmwTvgZ6iJ"

func TestStripeE2E(t *testing.T) {
	if os.Getenv("RUN_STRIPE_E2E") != "1" {
		t.Skip("set RUN_STRIPE_E2E=1 to run the live Stripe E2E test")
	}
	key := os.Getenv("STRIPE_SECRET_KEY")
	if !strings.HasPrefix(key, "sk_test_") {
		t.Fatal("STRIPE_SECRET_KEY must be a test-mode secret key")
	}
	if base := os.Getenv("APP_BASE_URL"); base != "" && strings.TrimRight(base, "/") != e2eAppURL {
		t.Fatal("APP_BASE_URL must be https://gameroom.hdynamic.app")
	}
	started := time.Now().Add(-time.Minute).Unix()
	ctx, cancel := context.WithTimeout(context.Background(), 13*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// No subprocesses: credentials stay in HTTP authorization headers, and raw
	// response bodies/transport errors (which may contain credentials) are not logged.
	stripeRequest := func(method, path string, dst any) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, "https://api.stripe.com/v1/"+path, nil)
		if err != nil {
			t.Fatal("could not construct Stripe request")
		}
		req.SetBasicAuth(key, "")
		req.Header.Set("Stripe-Version", stripe.APIVersion)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("Stripe request failed or timed out (details suppressed)")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Stripe request returned HTTP %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			t.Fatal("invalid Stripe JSON response")
		}
	}
	// Ensure our destination is subscribed before relying on pending_webhooks.
	var endpoint stripe.WebhookEndpoint
	stripeRequest(http.MethodGet, "webhook_endpoints/"+e2eWebhookID, &endpoint)
	if endpoint.Livemode || endpoint.Status != "enabled" || endpoint.URL != e2eAppURL+"/api/stripe/webhook" {
		t.Fatal("expected enabled test-mode application webhook endpoint")
	}
	for _, required := range []string{"checkout.session.completed", "customer.subscription.deleted"} {
		found := false
		for _, enabled := range endpoint.EnabledEvents {
			if enabled == "*" || enabled == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("webhook is not subscribed to %s", required)
		}
	}
	appPost := func(path string, data any, dst any) {
		t.Helper()
		body, err := json.Marshal(data)
		if err != nil {
			t.Fatal("could not encode application request")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e2eAppURL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal("could not construct application request")
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("application request failed or timed out (details suppressed)")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("application request returned HTTP %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			t.Fatal("invalid application JSON response")
		}
	}
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatal("could not generate signup password")
	}
	var reg struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	appPost("/auth/register", map[string]string{"email": fmt.Sprintf("e2e-%d@example.com", time.Now().UnixNano()), "password": hex.EncodeToString(random[:])}, &reg)
	if reg.User.ID == "" {
		t.Fatal("registration returned no user ID")
	}
	userID := reg.User.ID
	t.Logf("registered user %s", userID)
	export := func() (int, http.Header, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, e2eAppURL+"/u/export?user_id="+url.QueryEscape(userID), nil)
		if err != nil {
			t.Fatal("could not construct export request")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal("export request failed or timed out (details suppressed)")
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal("could not read export response")
		}
		return resp.StatusCode, resp.Header, body
	}
	if status, _, _ := export(); status != 402 {
		t.Fatalf("before checkout: expected 402, got %d", status)
	}
	var checkout struct {
		URL string `json:"url"`
	}
	appPost("/api/stripe/checkout", map[string]string{"user_id": userID}, &checkout)
	hosted, err := url.Parse(checkout.URL)
	if err != nil || hosted.Scheme != "https" || hosted.Host != "checkout.stripe.com" || hosted.User != nil {
		t.Fatal("application returned an invalid hosted Checkout URL")
	}
	sessionID := strings.TrimPrefix(hosted.Path, "/c/pay/")
	if !strings.HasPrefix(sessionID, "cs_test_") || strings.Contains(sessionID, "/") {
		t.Fatal("expected test Checkout session URL")
	}
	var sess stripe.CheckoutSession
	retrieveSession := func() {
		stripeRequest(http.MethodGet, "checkout/sessions/"+url.PathEscape(sessionID)+"?expand[]=line_items", &sess)
		if sess.ID != sessionID || sess.Livemode || sess.Mode != stripe.CheckoutSessionModeSubscription || sess.Metadata["user_id"] != userID {
			t.Fatal("Checkout session mode or application user association mismatch")
		}
		for _, target := range []string{sess.SuccessURL, sess.CancelURL} {
			u, err := url.Parse(target)
			if err != nil || u.Scheme != "https" || u.Host != "gameroom.hdynamic.app" || u.User != nil {
				t.Fatal("Checkout return URL must use gameroom.hdynamic.app")
			}
		}
		if sess.LineItems == nil || sess.LineItems.HasMore || len(sess.LineItems.Data) != 1 || sess.LineItems.Data[0].Price == nil || sess.LineItems.Data[0].Price.ID != e2ePriceID || sess.LineItems.Data[0].Quantity != 1 {
			t.Fatal("Checkout does not contain the provisioned premium price at quantity 1")
		}
	}
	retrieveSession()
	file, err := os.CreateTemp("", "gameroom-checkout-*.txt")
	if err != nil {
		t.Fatal("could not create private Checkout URL file")
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(checkout.URL + "\n"); err != nil {
		file.Close()
		t.Fatal("could not write Checkout URL")
	}
	if err := file.Close(); err != nil {
		t.Fatal("could not close Checkout URL file")
	}
	t.Logf("session %s verified; complete hosted test Checkout in a browser using the URL in %s within 10 minutes", sessionID, file.Name())
	poll := func(timeout time.Duration, label string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for {
			if check() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", label)
			}
			select {
			case <-ctx.Done():
				t.Fatalf("overall timeout waiting for %s", label)
			case <-time.After(2 * time.Second):
			}
		}
	}
	poll(10*time.Minute, "browser Checkout completion", func() bool {
		retrieveSession()
		if sess.Status == stripe.CheckoutSessionStatusExpired {
			t.Fatal("Checkout session expired")
		}
		return sess.Status == stripe.CheckoutSessionStatusComplete && sess.PaymentStatus == stripe.CheckoutSessionPaymentStatusPaid
	})
	if sess.Subscription == nil || sess.Subscription.ID == "" || sess.Customer == nil || sess.Customer.ID == "" {
		t.Fatal("completed Checkout missing subscription or customer")
	}
	subID := sess.Subscription.ID
	t.Logf("completed Checkout subscription %s; if this run fails, cancel this test subscription manually", subID)
	var sub stripe.Subscription
	stripeRequest(http.MethodGet, "subscriptions/"+url.PathEscape(subID), &sub)
	if sub.Livemode || sub.Customer == nil || sub.Customer.ID != sess.Customer.ID || sub.Metadata["user_id"] != userID || sub.Status != stripe.SubscriptionStatusActive {
		t.Fatal("completed Checkout subscription association/status mismatch")
	}
	// pending_webhooks is account-wide, not an endpoint delivery log. Require
	// the configured destination above, the matching real event with no pending
	// deliveries, and the application's resulting gating transition below.
	waitEvent := func(kind, objectID string) {
		t.Helper()
		poll(60*time.Second, kind+" delivery", func() bool {
			after := ""
			for {
				var events stripe.EventList
				path := "events?type=" + url.QueryEscape(kind) + fmt.Sprintf("&limit=100&created[gte]=%d", started)
				if after != "" {
					path += "&starting_after=" + url.QueryEscape(after)
				}
				stripeRequest(http.MethodGet, path, &events)
				for _, event := range events.Data {
					var object struct {
						ID string `json:"id"`
					}
					if event.Data == nil || json.Unmarshal(event.Data.Raw, &object) != nil {
						t.Fatal("invalid Stripe event data")
					}
					if object.ID == objectID {
						if event.Livemode {
							t.Fatal("unexpected live-mode event")
						}
						if event.PendingWebhooks != 0 {
							return false
						}
						t.Logf("%s: %s has no pending webhook deliveries", kind, event.ID)
						return true
					}
				}
				if !events.HasMore {
					return false
				}
				if len(events.Data) == 0 {
					t.Fatal("invalid Stripe event pagination")
				}
				after = events.Data[len(events.Data)-1].ID
			}
		})
	}
	waitEvent("checkout.session.completed", sessionID)
	poll(30*time.Second, "premium CSV export", func() bool {
		status, headers, body := export()
		if status == 402 {
			return false
		}
		if status != 200 {
			t.Fatalf("premium export returned %d", status)
		}
		if headers.Get("Content-Type") != "text/csv" || headers.Get("Content-Disposition") != `attachment; filename="gameroom_history.csv"` || !bytes.HasPrefix(body, []byte("app_id,name,playtime_total_minutes,playtime_recent_minutes,last_polled\n")) {
			t.Fatal("invalid CSV export headers or contents")
		}
		return true
	})
	stripeRequest(http.MethodDelete, "subscriptions/"+url.PathEscape(subID), &sub)
	if sub.ID != subID || sub.Status != stripe.SubscriptionStatusCanceled {
		t.Fatal("Checkout subscription cancellation not confirmed")
	}
	waitEvent("customer.subscription.deleted", subID)
	poll(30*time.Second, "restored 402 gating", func() bool {
		status, _, _ := export()
		if status == 200 {
			return false
		}
		if status != 402 {
			t.Fatalf("after cancellation: expected 402, got %d", status)
		}
		return true
	})
	t.Log("application Checkout, delivery, premium CSV export, cancellation, and restored 402 gating verified")
}
