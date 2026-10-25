package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

var errQueryTest = errors.New("injected database failure")

// Embed unused methods so accidental extra database work fails the test.
type queryTestDB struct {
	database
	rows      []*queryTestRows
	totalsErr error
	queries   int
}

func (d *queryTestDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	r := d.rows[d.queries]
	d.queries++
	return r, nil
}
func (d *queryTestDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return queryTestRow{d.totalsErr}
}

type queryTestRow struct{ err error }

func (r queryTestRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	for _, d := range dest {
		*d.(*int) = 1
	}
	return nil
}

type queryTestRows struct {
	pgx.Rows
	count, index          int
	scanErr, iterationErr error
	closed                bool
}

func (r *queryTestRows) Next() bool { r.index++; return r.index <= r.count }
func (r *queryTestRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	for _, d := range dest {
		switch v := d.(type) {
		case *int:
			*v = 1
		case *string:
			*v = "partial-result"
		case *time.Time:
			*v = time.Now()
		}
	}
	return nil
}
func (r *queryTestRows) Err() error { return r.iterationErr }
func (r *queryTestRows) Close()     { r.closed = true }

func useQueryTestDB(t *testing.T, d *queryTestDB) {
	t.Helper()
	oldDB, oldPremiums := db, premiums
	db = d
	store := newMemPremiumStore()
	store.activate("test-user", "")
	premiums = store
	t.Cleanup(func() { db, premiums = oldDB, oldPremiums })
}
func failureRows(kind string) *queryTestRows {
	r := &queryTestRows{count: 1}
	if kind == "scan" {
		r.scanErr = errQueryTest
	} else {
		r.iterationErr = errQueryTest
	}
	return r
}
func TestQueryFailuresDoNotReturnPartialResponses(t *testing.T) {
	for _, endpoint := range []string{"stats", "export", "top", "heat"} {
		for _, kind := range []string{"scan", "iteration"} {
			t.Run(endpoint+"/"+kind, func(t *testing.T) {
				failed := failureRows(kind)
				d := &queryTestDB{rows: []*queryTestRows{failed}}
				handler := getDeepStats
				if endpoint == "export" {
					handler = exportHistory
				}
				if endpoint == "top" || endpoint == "heat" {
					handler = getPublicProfile
				}
				if endpoint == "heat" {
					d.rows = []*queryTestRows{{count: 1}, failed}
				}
				useQueryTestDB(t, d)
				router := routing.New()
				router.Get("/<user_id>", handler)
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/test-user?user_id=test-user")
				router.HandleRequest(ctx)
				if ctx.Response.StatusCode() != 500 {
					t.Fatalf("status=%d body=%s", ctx.Response.StatusCode(), ctx.Response.Body())
				}
				if strings.Contains(string(ctx.Response.Body()), "partial-result") || len(ctx.Response.Header.Peek("Content-Disposition")) > 0 {
					t.Fatalf("partial success response: %s", &ctx.Response)
				}
				if !failed.closed {
					t.Fatal("rows not closed")
				}
				if d.queries != len(d.rows) {
					t.Fatalf("unexpected queries: %d", d.queries)
				}
			})
		}
	}
}
func TestFindExportHistoryReturnsQueryFailures(t *testing.T) {
	for _, kind := range []string{"scan", "iteration"} {
		t.Run(kind, func(t *testing.T) {
			r := failureRows(kind)
			useQueryTestDB(t, &queryTestDB{rows: []*queryTestRows{r}})
			result, err := findExportHistory(context.Background(), "test-user")
			if !errors.Is(err, errQueryTest) || result != nil {
				t.Fatalf("result=%v err=%v", result, err)
			}
			if !r.closed {
				t.Fatal("rows not closed")
			}
		})
	}
}
func TestPublicProfileTotalsFailure(t *testing.T) {
	d := &queryTestDB{rows: []*queryTestRows{{count: 1}}, totalsErr: errQueryTest}
	useQueryTestDB(t, d)
	router := routing.New()
	router.Get("/<user_id>", getPublicProfile)
	ctx := &fasthttp.RequestCtx{}
	ctx.Request.SetRequestURI("/test-user")
	router.HandleRequest(ctx)
	if ctx.Response.StatusCode() != 500 || strings.Contains(string(ctx.Response.Body()), "total_minutes") {
		t.Fatalf("response: %s", &ctx.Response)
	}
	if d.queries != 1 || !d.rows[0].closed {
		t.Fatal("totals failure did not stop and close queries")
	}
}
func TestPollPlayHistoryAbortsOnLinkFailures(t *testing.T) {
	t.Setenv("STEAM_API_KEY", "test-placeholder")
	oldHTTP := steamHTTPGet
	t.Cleanup(func() { steamHTTPGet = oldHTTP })
	steamHTTPGet = func([]byte, string, time.Duration) (int, []byte, error) {
		t.Fatal("polling continued after link loading failed")
		return 0, nil, nil
	}
	for _, kind := range []string{"scan", "iteration"} {
		t.Run(kind, func(t *testing.T) {
			r := failureRows(kind)
			useQueryTestDB(t, &queryTestDB{rows: []*queryTestRows{r}})
			var logs bytes.Buffer
			oldOutput := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(oldOutput) })
			pollPlayHistory()
			if !strings.Contains(logs.String(), errQueryTest.Error()) {
				t.Fatalf("missing error log: %s", logs.String())
			}
			if !r.closed {
				t.Fatal("rows not closed")
			}
		})
	}
}
