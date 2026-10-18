package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func linkRequest(t *testing.T, userID, body string) *fasthttp.RequestCtx {
	t.Helper()
	router := routing.New()
	router.Post("/s/link", linkAccount)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/s/link")
	ctx.Request.Header.SetMethod("POST")
	ctx.Request.Header.Set("Authorization", "Bearer caller-controlled")
	ctx.Request.Header.Set("X-User-ID", "victim")
	ctx.Request.SetBodyString(body)
	if userID != "" {
		// Simulate trusted server middleware, never a request header or body field.
		ctx.SetUserValue(authenticatedUserIDKey{}, userID)
	}
	router.HandleRequest(ctx)
	return ctx
}

func TestLinkAccountAuthorization(t *testing.T) {
	old := steamHTTPGet
	t.Cleanup(func() { steamHTTPGet = old })
	steamHTTPGet = func([]byte, string, time.Duration) (int, []byte, error) {
		t.Fatal("unauthorized request reached Steam")
		return 0, nil, nil
	}
	for _, tc := range []struct{ name, userID, body string }{
		{"unauthenticated", "", `{"user_id":"victim","steam_id":"76561198000000000"}`},
		{"no caller ID", "", `{"steam_id":"76561198000000000"}`},
		{"different user", "authenticated-user", `{"user_id":"victim","steam_id":"76561198000000000"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := linkRequest(t, tc.userID, tc.body)
			if ctx.Response.StatusCode() != 403 {
				t.Fatalf("status = %d", ctx.Response.StatusCode())
			}
		})
	}
}

func TestLinkAccountUpstreamFailures(t *testing.T) {
	t.Setenv("STEAM_API_KEY", "test-placeholder")
	old := steamHTTPGet
	t.Cleanup(func() { steamHTTPGet = old })
	// A nil Mongo client makes any accidental database access fail this test.
	for _, tc := range []struct {
		name, body string
		err        error
	}{
		{"malformed JSON", `{"response":`, nil},
		{"timeout", "", fasthttp.ErrTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steamHTTPGet = func(_ []byte, _ string, timeout time.Duration) (int, []byte, error) {
				if timeout <= 0 || timeout > 5*time.Second {
					t.Fatalf("unbounded timeout: %v", timeout)
				}
				return 200, []byte(tc.body), tc.err
			}
			ctx := linkRequest(t, "authenticated-user", `{"steam_id":"76561198000000000"}`)
			if ctx.Response.StatusCode() != 502 {
				t.Fatalf("status = %d", ctx.Response.StatusCode())
			}
		})
	}
}

func TestSteamHTTPTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	_, _, err := steamHTTPGet(nil, server.URL, 10*time.Millisecond)
	if !errors.Is(err, fasthttp.ErrTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

type deadlineUpdater struct {
	t    *testing.T
	wait bool
}

func (u deadlineUpdater) UpdateOne(ctx context.Context, _ interface{}, _ interface{}, _ ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > mongoUpdateTimeout {
		u.t.Fatal("Mongo update has no bounded deadline")
	}
	if u.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &mongo.UpdateResult{}, nil
}

func TestMongoUpdateTimeout(t *testing.T) {
	if err := saveSteamLink(context.Background(), deadlineUpdater{t: t}, SteamLink{}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := saveSteamLink(ctx, deadlineUpdater{t: t, wait: true}, SteamLink{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
