package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
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
