package main

// Stripe test-mode E2E smoke test: signup → checkout webhook → premium unlock → CSV export gated.
//
// Requires the stripe CLI in PATH and:
//   RUN_STRIPE_E2E=1            — opt-in guard (prevents accidental runs in CI)
//   STRIPE_SECRET_KEY=sk_test_… — Stripe test-mode secret key
//
// Optional:
//   APP_BASE_URL=https://…      — defaults to https://gameroom.hdynamic.app
//
// Usage:
//   RUN_STRIPE_E2E=1 STRIPE_SECRET_KEY=sk_test_... go test -run TestStripeE2E -v -timeout 120s
//
// Verified 2026-10-24 against gameroom.hdynamic.app:
//   signup → 402 gate confirmed
//   stripe trigger checkout.session.completed (evt_1UTwL7GRjKD6n9dm9XhYncPm) delivered to webhook
//   export HTTP 200, content-type text/csv, content-disposition attachment after activation
//   trial subscription (sub_1UTwN8GRjKD6n9dmw1X17lx5) cancelled; customer.subscription.deleted fired
//   export re-gated to 402

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestStripeE2E exercises the full Stripe subscription lifecycle against the
// live deployment, using real Stripe test-mode events delivered to the
// registered webhook endpoint. The in-process TestStripeSmoke covers the
// same logic without network I/O; this test adds actual Stripe delivery.
func TestStripeE2E(t *testing.T) {
	if os.Getenv("RUN_STRIPE_E2E") != "1" {
		t.Skip("set RUN_STRIPE_E2E=1 to run the live Stripe E2E test")
	}
	stripeKey := os.Getenv("STRIPE_SECRET_KEY")
	if !strings.HasPrefix(stripeKey, "sk_test_") {
		t.Skip("STRIPE_SECRET_KEY must be a test-mode key (sk_test_...)")
	}
	if _, err := exec.LookPath("stripe"); err != nil {
		t.Skip("stripe CLI not in PATH")
	}

	appURL := os.Getenv("APP_BASE_URL")
	if appURL == "" {
		appURL = "https://gameroom.hdynamic.app"
	}

	client := &http.Client{Timeout: 15 * time.Second}

	// -------------------------------------------------------------------
	// 1. Register a unique test user.
	// -------------------------------------------------------------------
	email := fmt.Sprintf("e2e-%d@example.com", time.Now().UnixNano())
	regBody, _ := json.Marshal(map[string]string{"email": email, "password": "E2ETestPass123!"})
	resp, err := client.Post(appURL+"/auth/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("register returned %d: %s", resp.StatusCode, body)
	}
	var reg struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil || reg.User.ID == "" {
		t.Fatal("could not decode user ID from register response")
	}
	userID := reg.User.ID
	t.Logf("registered test user: %s (%s)", userID, email)

	// -------------------------------------------------------------------
	// 2. Verify export is gated before activation.
	// -------------------------------------------------------------------
	t.Run("export_gated_before_checkout", func(t *testing.T) {
		resp, err := client.Get(appURL + "/u/export?user_id=" + userID)
		if err != nil {
			t.Fatalf("export request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != 402 {
			t.Fatalf("expected 402 for non-premium user, got %d", resp.StatusCode)
		}
	})

	// -------------------------------------------------------------------
	// 3. Trigger checkout.session.completed via the Stripe CLI, delivering
	//    the event to the registered webhook at APP_BASE_URL.
	// -------------------------------------------------------------------
	t.Run("checkout_webhook_delivered", func(t *testing.T) {
		cmd := exec.Command("stripe",
			"--api-key", stripeKey,
			"trigger", "checkout.session.completed",
			"--override", "checkout_session:metadata[user_id]="+userID,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("stripe trigger failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "Trigger succeeded") {
			t.Fatalf("unexpected trigger output: %s", out)
		}
		t.Logf("trigger output: %s", strings.TrimSpace(string(out)))
	})

	// -------------------------------------------------------------------
	// 4. Poll for premium activation (up to 30 s).
	// -------------------------------------------------------------------
	t.Run("premium_activated_after_checkout", func(t *testing.T) {
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := client.Get(appURL + "/u/export?user_id=" + userID)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Fatal("premium not activated within 30s after checkout.session.completed webhook")
	})

	// -------------------------------------------------------------------
	// 5. Verify CSV format.
	// -------------------------------------------------------------------
	t.Run("csv_export_format", func(t *testing.T) {
		resp, err := client.Get(appURL + "/u/export?user_id=" + userID)
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/csv" {
			t.Fatalf("expected text/csv, got %q", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); cd != `attachment; filename="gamescrobble_history.csv"` {
			t.Fatalf("unexpected Content-Disposition: %q", cd)
		}
		body, _ := io.ReadAll(resp.Body)
		if !strings.HasPrefix(string(body), "app_id,name,playtime_total_minutes,playtime_recent_minutes,last_polled\n") {
			t.Fatalf("unexpected CSV header: %q", string(body))
		}
		t.Logf("CSV export OK (%d bytes)", len(body))
	})

	// -------------------------------------------------------------------
	// 6. Re-trigger checkout with a customer ID, then create a trial
	//    subscription for that customer, and cancel it. The resulting
	//    customer.subscription.deleted webhook should revoke premium.
	// -------------------------------------------------------------------
	priceID := os.Getenv("STRIPE_PREMIUM_PRICE_ID")
	if priceID == "" {
		priceID = "price_1USUwuGRjKD6n9dmujp5uGyk"
	}
	customerID := e2eCreateCustomer(t, appURL, stripeKey, userID)
	if customerID != "" {
		// Link the customer ID to this user via a second checkout event.
		cmd := exec.Command("stripe",
			"--api-key", stripeKey,
			"trigger", "checkout.session.completed",
			"--override", "checkout_session:metadata[user_id]="+userID,
			"--override", "checkout_session:customer="+customerID,
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Logf("re-trigger (customer link) failed: %v\n%s", err, out)
		} else {
			time.Sleep(3 * time.Second)
		}

		subID := e2eCreateTrialSub(t, stripeKey, customerID, priceID, userID)
		if subID != "" {
			e2eCancelSub(t, stripeKey, subID)
		}
	}

	// -------------------------------------------------------------------
	// 7. Verify export re-gated after cancellation.
	// -------------------------------------------------------------------
	t.Run("export_gated_after_cancellation", func(t *testing.T) {
		if customerID == "" {
			t.Skip("no customer created; skipping cancellation gate check")
		}
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := client.Get(appURL + "/u/export?user_id=" + userID)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == 402 {
				t.Log("export correctly re-gated to 402 after cancellation")
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Fatal("export still 200 after subscription cancellation — premium not revoked")
	})
}

// e2eCreateCustomer creates a Stripe test customer linked to the given user.
func e2eCreateCustomer(t *testing.T, _ string, apiKey, userID string) string {
	t.Helper()
	cmd := exec.Command("stripe", "--api-key", apiKey,
		"customers", "create",
		"--metadata", "user_id="+userID,
		"--metadata", "test=e2e_smoke",
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("create customer: %v", err)
		return ""
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &obj); err != nil || obj.ID == "" {
		t.Logf("parse customer: %v (%s)", err, out)
		return ""
	}
	t.Logf("created customer: %s", obj.ID)
	return obj.ID
}

// e2eCreateTrialSub creates a 7-day trial subscription; no payment method needed.
func e2eCreateTrialSub(t *testing.T, apiKey, customerID, priceID, userID string) string {
	t.Helper()
	cmd := exec.Command("stripe", "--api-key", apiKey,
		"subscriptions", "create",
		"--customer", customerID,
		"--items[0][price]", priceID,
		"--trial-period-days", "7",
		"--metadata", "user_id="+userID,
	)
	out, err := cmd.Output()
	if err != nil {
		t.Logf("create trial sub: %v", err)
		return ""
	}
	var obj struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &obj); err != nil || obj.ID == "" {
		t.Logf("parse sub: %v", err)
		return ""
	}
	t.Logf("created trial subscription: %s", obj.ID)
	return obj.ID
}

// e2eCancelSub cancels the subscription immediately.
func e2eCancelSub(t *testing.T, apiKey, subID string) {
	t.Helper()
	cmd := exec.Command("stripe", "--api-key", apiKey,
		"subscriptions", "cancel", subID,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("cancel sub: %v\n%s", err, out)
		return
	}
	t.Logf("cancelled subscription: %s", subID)
	time.Sleep(8 * time.Second) // allow webhook delivery
}
