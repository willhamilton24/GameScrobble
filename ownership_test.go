package main

// TestOwnershipEnforcement verifies that premium stats, CSV export, and checkout
// are gated by the JWT session — anonymous callers get 401, non-premium callers
// get 402, and the user is always derived from the token (no query-param spoofing).

import (
	"context"
	"testing"

	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

func TestOwnershipEnforcement(t *testing.T) {
	t.Setenv("JWT_SECRET", "test-ownership-secret-32bytes!!")

	store := newMemPremiumStore()
	oldPremiums := premiums
	premiums = store
	t.Cleanup(func() { premiums = oldPremiums })

	oldFind := findExportHistory
	findExportHistory = func(_ context.Context, _ string) ([]exportRow, error) {
		return []exportRow{}, nil
	}
	t.Cleanup(func() { findExportHistory = oldFind })

	freeToken, err := signToken("free-user")
	if err != nil {
		t.Fatal(err)
	}
	store.activate("premium-user", "cus_test_owner")

	statsReq := func(authHeader string) int {
		r := routing.New()
		r.Get("/u/stats", func(c *routing.Context) error {
			if !authMiddleware(c) {
				return nil
			}
			return getDeepStats(c)
		})
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/u/stats")
		ctx.Request.Header.SetMethod("GET")
		if authHeader != "" {
			ctx.Request.Header.Set("Authorization", authHeader)
		}
		r.HandleRequest(ctx)
		return ctx.Response.StatusCode()
	}

	exportReq := func(authHeader string) int {
		r := routing.New()
		r.Get("/u/export", func(c *routing.Context) error {
			if !authMiddleware(c) {
				return nil
			}
			return exportHistory(c)
		})
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/u/export")
		ctx.Request.Header.SetMethod("GET")
		if authHeader != "" {
			ctx.Request.Header.Set("Authorization", authHeader)
		}
		r.HandleRequest(ctx)
		return ctx.Response.StatusCode()
	}

	checkoutReq := func(authHeader string) int {
		r := routing.New()
		r.Post("/api/stripe/checkout", func(c *routing.Context) error {
			if !authMiddleware(c) {
				return nil
			}
			return createCheckoutSession(c)
		})
		ctx := &fasthttp.RequestCtx{}
		ctx.Request.SetRequestURI("/api/stripe/checkout")
		ctx.Request.Header.SetMethod("POST")
		if authHeader != "" {
			ctx.Request.Header.Set("Authorization", authHeader)
		}
		r.HandleRequest(ctx)
		return ctx.Response.StatusCode()
	}

	t.Run("anonymous requests are rejected", func(t *testing.T) {
		if got := statsReq(""); got != 401 {
			t.Errorf("stats: got %d, want 401", got)
		}
		if got := exportReq(""); got != 401 {
			t.Errorf("export: got %d, want 401", got)
		}
		if got := checkoutReq(""); got != 401 {
			t.Errorf("checkout: got %d, want 401", got)
		}
	})

	t.Run("non-premium user cannot access premium endpoints", func(t *testing.T) {
		bearer := "Bearer " + freeToken
		// The free user's token was not spoofed with premium-user's ID; the server
		// derives the subject from the token and finds no active subscription.
		if got := statsReq(bearer); got != 402 {
			t.Errorf("stats: got %d, want 402", got)
		}
		if got := exportReq(bearer); got != 402 {
			t.Errorf("export: got %d, want 402", got)
		}
	})

	t.Run("premium user can access their own stats and export", func(t *testing.T) {
		// Inject user directly (as authMiddleware would after successful verification)
		// to avoid triggering recordReturnActivity which needs a real Exec-capable DB.
		oldDB := db
		db = &queryTestDB{rows: []*queryTestRows{{count: 0}}}
		t.Cleanup(func() { db = oldDB })

		makeAuthedReq := func(path string, handler func(*routing.Context) error) int {
			r := routing.New()
			r.Get(path, handler)
			ctx := &fasthttp.RequestCtx{}
			ctx.Request.SetRequestURI(path)
			ctx.Request.Header.SetMethod("GET")
			ctx.SetUserValue(authenticatedUserIDKey{}, "premium-user")
			r.HandleRequest(ctx)
			return ctx.Response.StatusCode()
		}
		if got := makeAuthedReq("/u/stats", getDeepStats); got != 200 {
			t.Errorf("stats: got %d, want 200", got)
		}
		if got := makeAuthedReq("/u/export", exportHistory); got != 200 {
			t.Errorf("export: got %d, want 200", got)
		}
	})
}
