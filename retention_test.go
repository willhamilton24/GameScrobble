package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	routing "github.com/qiangxue/fasthttp-routing"
	"github.com/valyala/fasthttp"
)

type retentionDB struct {
	database
	queryErr, startErr, writeErr error
	rows                         *queryTestRows
	writes                       int
}

func (d *retentionDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return d.rows, d.queryErr
}
func (d *retentionDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return trackingRow{d.startErr}
}
func (d *retentionDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	d.writes++
	return pgconn.CommandTag{}, d.writeErr
}

type trackingRow struct{ err error }

func (r trackingRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*time.Time) = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return nil
}
func retentionRequest(uri, header string) *fasthttp.RequestCtx {
	router := routing.New()
	router.Get("/retention", getRetentionStats)
	c := &fasthttp.RequestCtx{}
	c.Request.SetRequestURI(uri)
	c.Request.Header.Set("Authorization", header)
	router.HandleRequest(c)
	return c
}
func TestRetentionAuthorization(t *testing.T) {
	t.Setenv("ADMIN_KEY", "test-admin")
	old := db
	db = &retentionDB{rows: &queryTestRows{}}
	t.Cleanup(func() { db = old })
	for _, tc := range []struct {
		uri, header string
		status      int
	}{
		{"/retention", "", 401}, {"/retention?key=test-admin", "", 401},
		{"/retention?key=test-admin", "Bearer test-admin", 401},
		{"/retention?admin_key=test-admin", "Bearer test-admin", 401},
		{"/retention", "test-admin", 401}, {"/retention", "Bearer wrong", 401}, {"/retention", "Bearer test-admin", 200},
	} {
		c := retentionRequest(tc.uri, tc.header)
		if c.Response.StatusCode() != tc.status {
			t.Fatalf("status %d want %d", c.Response.StatusCode(), tc.status)
		}
		if string(c.Response.Header.Peek("Cache-Control")) != "no-store" {
			t.Fatal("missing no-store")
		}
	}
	t.Setenv("ADMIN_KEY", "")
	if retentionRequest("/retention", "Bearer ").Response.StatusCode() != 401 {
		t.Fatal("empty key authorized")
	}
}
func TestRetentionDatabaseFailures(t *testing.T) {
	t.Setenv("ADMIN_KEY", "test-admin")
	sensitive := errors.New("database password and user data must not appear")
	for _, kind := range []string{"start", "query", "scan", "iteration"} {
		t.Run(kind, func(t *testing.T) {
			d := &retentionDB{rows: &queryTestRows{count: 1}}
			switch kind {
			case "start":
				d.startErr = sensitive
			case "query":
				d.queryErr = sensitive
			case "scan":
				d.rows.scanErr = sensitive
			case "iteration":
				d.rows.iterationErr = sensitive
			}
			old := db
			db = d
			t.Cleanup(func() { db = old })
			var logs bytes.Buffer
			output := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(output) })
			c := retentionRequest("/retention", "Bearer test-admin")
			if c.Response.StatusCode() != 500 || strings.Contains(string(c.Response.Body()), "cohorts") {
				t.Fatalf("partial response: %s", &c.Response)
			}
			if !strings.Contains(logs.String(), "failed") || strings.Contains(logs.String(), sensitive.Error()) {
				t.Fatalf("unsafe/missing log: %s", logs.String())
			}
			if (kind == "scan" || kind == "iteration") && !d.rows.closed {
				t.Fatal("rows not closed")
			}
		})
	}
}
func TestActivityFailureIsBestEffort(t *testing.T) {
	old := db
	d := &retentionDB{writeErr: errQueryTest}
	db = d
	t.Cleanup(func() { db = old })
	t.Setenv("JWT_SECRET", "test-only")
	token, _ := signToken("user")
	router := routing.New()
	router.Get("/me", func(c *routing.Context) error {
		if authMiddleware(c) {
			c.WriteString("ok")
		}
		return nil
	})
	c := &fasthttp.RequestCtx{}
	c.Request.SetRequestURI("/me")
	c.Request.Header.Set("Authorization", "Bearer "+token)
	var logs bytes.Buffer
	output := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(output) })
	router.HandleRequest(c)
	if c.Response.StatusCode() != 200 || string(c.Response.Body()) != "ok" || d.writes != 1 || !strings.Contains(logs.String(), "activity write failed") {
		t.Fatal("activity failure interrupted auth or was not logged")
	}
}
func TestSignupSourceValidation(t *testing.T) {
	for _, source := range []string{"", "direct", "unknown", "backloggd", "reddit_patientgamers"} {
		if _, ok := signupSource(source); !ok {
			t.Fatalf("rejected %q", source)
		}
	}
	for _, source := range []string{"é", strings.Repeat("a", 10000), "backloggd\n", "<script>", "BACKLOGGD", "a\x00b"} {
		router := routing.New()
		router.Post("/register", registerUser)
		c := &fasthttp.RequestCtx{}
		c.Request.SetRequestURI("/register")
		c.Request.Header.SetMethod("POST")
		// JSON encoding also checks malformed source rejection through the handler.
		body, _ := json.Marshal(map[string]string{"email": "test@example.com", "password": "test-password", "source": source})
		c.Request.SetBody(body)
		router.HandleRequest(c)
		if c.Response.StatusCode() != 400 {
			t.Fatalf("invalid source accepted: status %d", c.Response.StatusCode())
		}
	}
}

// Uses an isolated schema in a disposable local PostgreSQL database only.
func TestRetentionPostgres(t *testing.T) {
	url := os.Getenv("RETENTION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set RETENTION_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}
	ctx := context.Background()
	old := db
	defer func() { db = old }()
	// One connection keeps search_path confined to the isolated test schema.
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	db = pool
	schema := "retention_qa_" + fmt.Sprintf("%d", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	exec("SET search_path TO " + schema)
	for _, existing := range []bool{false, true} {
		if existing {
			exec("DROP SCHEMA " + schema + " CASCADE")
			exec("CREATE SCHEMA " + schema)
			exec(`CREATE TABLE users(id TEXT PRIMARY KEY,email TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()); INSERT INTO users VALUES ('legacy','legacy@example.com','hash','2025-12-01')`)
		}
		if err := runMigrations(ctx); err != nil {
			t.Fatal(err)
		}
		var start1, start2 time.Time
		if err := pool.QueryRow(ctx, "SELECT started_at FROM retention_tracking").Scan(&start1); err != nil {
			t.Fatal(err)
		}
		if err := runMigrations(ctx); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(ctx, "SELECT started_at FROM retention_tracking").Scan(&start2); err != nil {
			t.Fatal(err)
		}
		if !start1.Equal(start2) {
			t.Fatal("migration reset tracking start")
		}
		if existing {
			var count int
			if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE id='legacy'").Scan(&count); err != nil || count != 1 {
				t.Fatal("migration lost existing user")
			}
		}
	}
	exec("DELETE FROM users")
	exec("UPDATE retention_tracking SET started_at='2026-01-01T00:00:00Z'")
	// On Feb 1, Jan 1 D30 is complete; Jan 2 D30 is today and excluded.
	for _, u := range []struct{ id, created, source string }{
		{"old", "2025-12-31T23:59:59Z", ""},
		{"a", "2026-01-01T00:00:00Z", "backloggd"},
		{"b", "2026-01-02T00:00:00Z", "direct"},
		{"c", "2026-01-24T23:59:59Z", "reddit_patientgamers"},
		{"d", "2026-01-25T00:00:00Z", "reddit_patientgamers"},
		{"e", "2026-01-30T23:59:59Z", "direct"},
		{"f", "2026-01-31T00:00:00Z", "direct"},
	} {
		exec("INSERT INTO users(id,email,password_hash,created_at,signup_source)VALUES($1,$1,'hash',$2,$3)", u.id, u.created, u.source)
	}
	for i := 0; i < 3; i++ {
		exec(`INSERT INTO user_return_activity VALUES ('a','2026-01-02'),('a','2026-01-08'),('a','2026-01-31'),('c','2026-01-31'),('f','2026-02-01') ON CONFLICT DO NOTHING`)
	}
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows, err := pool.Query(ctx, retentionSQL, "2026-02-01", started)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]cohortMetric{}
	for rows.Next() {
		var m cohortMetric
		if err := rows.Scan(&m.Scope, &m.Source, &m.Day, &m.Eligible, &m.Returning, &m.Immature, &m.Unavailable); err != nil {
			t.Fatal(err)
		}
		got[fmt.Sprintf("%s/%s/%d", m.Scope, m.Source, m.Day)] = m
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		key                                       string
		eligible, returned, immature, unavailable int
	}{
		{"overall//1", 5, 1, 1, 1}, {"overall//7", 3, 2, 3, 1}, {"overall//30", 1, 1, 5, 1},
		{"source/backloggd/30", 1, 1, 0, 0}, {"source/direct/1", 2, 0, 1, 0},
		{"source/reddit_patientgamers/7", 1, 1, 1, 0}, {"source/unknown/30", 0, 0, 0, 1},
	} {
		m, ok := got[want.key]
		if !ok || m.Eligible != want.eligible || m.Returning != want.returned || m.Immature != want.immature || m.Unavailable != want.unavailable {
			t.Errorf("%s: %+v want %+v", want.key, m, want)
		}
	}
	rows.Close()
	recordReturnActivity("a")
	recordReturnActivity("a")
	var activities int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM user_return_activity WHERE user_id='a' AND activity_date=$1", time.Now().UTC().Format("2006-01-02")).Scan(&activities); err != nil || activities != 1 {
		t.Fatalf("repeated authenticated activity: count=%d err=%v", activities, err)
	}
	// Real endpoint rate serialization: zero vs unavailable, plus empty database.
	t.Setenv("ADMIN_KEY", "test-admin")
	c := retentionRequest("/retention", "Bearer test-admin")
	if c.Response.StatusCode() != 200 {
		t.Fatal("endpoint failed")
	}
	var response struct {
		Cohorts []cohortMetric `json:"cohorts"`
	}
	if err := json.Unmarshal(c.Response.Body(), &response); err != nil {
		t.Fatal(err)
	}
	for _, m := range response.Cohorts {
		if m.Eligible == 0 && m.Rate != nil {
			t.Fatal("unavailable rate must be null")
		}
		if m.Eligible > 0 && (m.Rate == nil || *m.Rate != float64(m.Returning)/float64(m.Eligible)) {
			t.Fatal("wrong rate")
		}
	}
	exec("DELETE FROM users")
	c = retentionRequest("/retention", "Bearer test-admin")
	if err := json.Unmarshal(c.Response.Body(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Cohorts) != 3 {
		t.Fatal("empty database missing horizons")
	}
}
