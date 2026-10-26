package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

// Exercise the real entitlement expiry check and account-scoped history queries.
type premiumFixtureDB struct {
	database
	until      map[string]time.Time
	historyIDs []string
}

func (d *premiumFixtureDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (d *premiumFixtureDB) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	return premiumFixtureRow{d.until[args[0].(string)]}
}

type premiumFixtureRow struct{ until time.Time }

func (r premiumFixtureRow) Scan(dest ...any) error { *dest[0].(*time.Time) = r.until; return nil }
func (d *premiumFixtureDB) Query(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
	d.historyIDs = append(d.historyIDs, args[0].(string))
	return &queryTestRows{}, nil
}

func TestPremiumRouteAuthenticationAndIsolation(t *testing.T) {
	t.Setenv("JWT_SECRET", "local-regression-fixture")
	oldDB, oldPremiums := db, premiums
	fixture := &premiumFixtureDB{until: map[string]time.Time{
		"active": time.Now().Add(time.Hour), "other": time.Now().Add(time.Hour),
		"expired": time.Now().Add(-time.Hour),
	}}
	db, premiums = fixture, pgPremiumStore{}
	t.Cleanup(func() { db, premiums = oldDB, oldPremiums })
	router := routing.New()
	registerPremiumRoutes(router)
	request := func(path, auth string) *fasthttp.RequestCtx {
		c := &fasthttp.RequestCtx{}
		c.Init(&fasthttp.Request{}, nil, nil)
		c.Request.SetRequestURI(path)
		if auth != "" {
			c.Request.Header.Set("Authorization", auth)
		}
		router.HandleRequest(c)
		return c
	}
	for _, path := range []string{"/api/premium/status", "/u/export", "/u/stats"} {
		for _, auth := range []string{"", "Bearer invalid"} {
			c := request(path+"?user_id=active", auth)
			if c.Response.StatusCode() != 401 {
				t.Fatalf("%s anonymous/invalid: %s", path, &c.Response)
			}
		}
	}
	for _, account := range []string{"active", "expired"} {
		token, err := signToken(account)
		if err != nil {
			t.Fatal(err)
		}
		auth := "Bearer " + token
		c := request("/api/premium/status?user_id=other&premium=true&session_id=fake", auth)
		var status struct {
			Premium bool   `json:"premium"`
			UserID  string `json:"user_id"`
		}
		if err := json.Unmarshal(c.Response.Body(), &status); err != nil {
			t.Fatal(err)
		}
		if c.Response.StatusCode() != 200 || status.Premium != (account == "active") || status.UserID != account {
			t.Fatalf("status for %s: %s", account, &c.Response)
		}
		for _, path := range []string{"/u/export", "/u/stats"} {
			c = request(path+"?user_id=other", auth)
			if c.Response.StatusCode() != 403 {
				t.Fatalf("cross-account %s: %s", path, &c.Response)
			}
			c = request(path, auth)
			want := 200
			if account == "expired" {
				want = 402
			}
			if c.Response.StatusCode() != want {
				t.Fatalf("%s %s: %s", account, path, &c.Response)
			}
		}
	}
	if len(fixture.historyIDs) != 2 {
		t.Fatalf("unauthorized history reads: %v", fixture.historyIDs)
	}
	for _, id := range fixture.historyIDs {
		if id != "active" {
			t.Fatalf("wrong history account: %s", id)
		}
	}
	// A previously active session loses access on the next request after expiry.
	fixture.until["active"] = time.Now().Add(-time.Second)
	token, _ := signToken("active")
	for _, path := range []string{"/u/export", "/u/stats"} {
		c := request(path, "Bearer "+token)
		if c.Response.StatusCode() != 402 {
			t.Fatalf("stale entitlement on %s: %s", path, &c.Response)
		}
	}
}
