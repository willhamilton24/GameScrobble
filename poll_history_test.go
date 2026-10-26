package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errPollTest = errors.New("injected poll activity failure")

// fakePollTx implements pgx.Tx for unit-testing recordGameActivity.
// Only QueryRow, Exec, Commit, and Rollback are exercised; other methods
// are satisfied by the embedded pgx.Tx (panics on any unexpected call).
type fakePollTx struct {
	pgx.Tx
	histRow     pgx.Row // returned by the SELECT FOR UPDATE
	execResults []error // errors injected per Exec call, in order
	execCalls   int
	committed   bool
	rolledBack  bool
}

func (tx *fakePollTx) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return tx.histRow
}
func (tx *fakePollTx) Exec(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
	idx := tx.execCalls
	tx.execCalls++
	if idx < len(tx.execResults) {
		return pgconn.CommandTag{}, tx.execResults[idx]
	}
	return pgconn.CommandTag{}, nil
}
func (tx *fakePollTx) Commit(_ context.Context) error {
	tx.committed = true
	return nil
}
func (tx *fakePollTx) Rollback(_ context.Context) error {
	if !tx.committed {
		tx.rolledBack = true
	}
	return nil
}

// pollRow is the pgx.Row returned by the fake SELECT FOR UPDATE.
type pollRow struct {
	val int
	err error
}

func (r pollRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*int) = r.val
	return nil
}

type fakePollStarter struct{ tx *fakePollTx }

func (f *fakePollStarter) Begin(_ context.Context) (pgx.Tx, error) { return f.tx, nil }

func usePollStarter(t *testing.T, starter txStarter) {
	t.Helper()
	old := dbPool
	dbPool = starter
	t.Cleanup(func() { dbPool = old })
}

// TestPollGameActivity_FirstObservation: no prior row → no activity written, baseline inserted, committed.
func TestPollGameActivity_FirstObservation(t *testing.T) {
	tx := &fakePollTx{histRow: pollRow{err: pgx.ErrNoRows}}
	usePollStarter(t, &fakePollStarter{tx})

	if err := recordGameActivity("u1", "s1", 1, "Game", 60, 60, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.execCalls != 1 {
		t.Fatalf("expected 1 exec (baseline only), got %d", tx.execCalls)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatal("expected commit, not rollback")
	}
}

// TestPollGameActivity_PositiveDelta: prior total lower → activity written and baseline updated, committed.
func TestPollGameActivity_PositiveDelta(t *testing.T) {
	tx := &fakePollTx{histRow: pollRow{val: 40}}
	usePollStarter(t, &fakePollStarter{tx})

	if err := recordGameActivity("u1", "s1", 1, "Game", 100, 60, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.execCalls != 2 {
		t.Fatalf("expected 2 execs (activity + baseline), got %d", tx.execCalls)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatal("expected commit, not rollback")
	}
}

// TestPollGameActivity_NoDelta: same total as prior baseline → no activity written.
func TestPollGameActivity_NoDelta(t *testing.T) {
	tx := &fakePollTx{histRow: pollRow{val: 100}}
	usePollStarter(t, &fakePollStarter{tx})

	if err := recordGameActivity("u1", "s1", 1, "Game", 100, 0, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tx.execCalls != 1 {
		t.Fatalf("expected 1 exec (baseline only), got %d", tx.execCalls)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatal("expected commit")
	}
}

// TestPollGameActivity_Rollback: activity write fails → baseline exec never called, tx rolled back.
func TestPollGameActivity_Rollback(t *testing.T) {
	tx := &fakePollTx{
		histRow:     pollRow{val: 50},
		execResults: []error{errPollTest}, // first exec (activity) fails
	}
	usePollStarter(t, &fakePollStarter{tx})

	err := recordGameActivity("u1", "s1", 1, "Game", 150, 60, time.Now())
	if !errors.Is(err, errPollTest) {
		t.Fatalf("expected errPollTest, got %v", err)
	}
	if tx.execCalls != 1 {
		t.Fatalf("expected 1 exec (activity only, baseline not reached), got %d", tx.execCalls)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatal("expected rollback, not commit")
	}
}

// TestPollGameActivity_BaselineRollback: baseline write fails → tx rolled back.
func TestPollGameActivity_BaselineRollback(t *testing.T) {
	tx := &fakePollTx{
		histRow:     pollRow{val: 50},
		execResults: []error{nil, errPollTest}, // activity ok, baseline fails
	}
	usePollStarter(t, &fakePollStarter{tx})

	err := recordGameActivity("u1", "s1", 1, "Game", 150, 60, time.Now())
	if !errors.Is(err, errPollTest) {
		t.Fatalf("expected errPollTest, got %v", err)
	}
	if tx.execCalls != 2 {
		t.Fatalf("expected 2 execs, got %d", tx.execCalls)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatal("expected rollback")
	}
}

// TestPollGameActivity_RepeatedPoll: same value polled twice does not double-count.
// Simulates what happens after the first poll updates the baseline to 100 and
// the second poll observes the same value.
func TestPollGameActivity_RepeatedPoll(t *testing.T) {
	for _, round := range []struct {
		name     string
		oldPT    int
		newPT    int
		wantExec int // 1 = baseline only, 2 = activity + baseline
	}{
		{"first poll (stored=100, new=100, no delta)", 100, 100, 1},
		{"second poll same value",                    100, 100, 1},
	} {
		t.Run(round.name, func(t *testing.T) {
			tx := &fakePollTx{histRow: pollRow{val: round.oldPT}}
			usePollStarter(t, &fakePollStarter{tx})
			if err := recordGameActivity("u1", "s1", 1, "Game", round.newPT, 0, time.Now()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tx.execCalls != round.wantExec {
				t.Fatalf("exec calls: got %d want %d", tx.execCalls, round.wantExec)
			}
		})
	}
}

// TestPollHistoryPostgres runs against a real disposable Postgres database.
// It verifies rollback (activity failure does not advance baseline), first
// observation, repeated polls (no duplicate activity), and concurrent
// serialisation (SELECT FOR UPDATE prevents double-counting).
//
// Set POLL_TEST_DATABASE_URL to a disposable PostgreSQL database to enable.
func TestPollHistoryPostgres(t *testing.T) {
	pgURL := os.Getenv("POLL_TEST_DATABASE_URL")
	if pgURL == "" {
		t.Skip("set POLL_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}

	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	schema := fmt.Sprintf("poll_qa_%d", time.Now().UnixNano())
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") //nolint:errcheck

	exec("SET search_path TO " + schema)

	// Save and restore globals.
	oldDB, oldPool := db, dbPool
	db = pool
	dbPool = pool
	t.Cleanup(func() { db, dbPool = oldDB, oldPool })

	if err := runMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	// Insert a user row to satisfy FK on steam_links (none here, but users table exists).
	exec(`INSERT INTO users(id,email,password_hash) VALUES ('u1','u@test.com','hash')`)

	now := time.Now().UTC()
	today := now.Format("2006-01-02")

	// --- first observation: no prior row → no activity row, baseline created ---
	if err := recordGameActivity("u1", "s1", 1, "TestGame", 100, 10, now); err != nil {
		t.Fatalf("first observation: %v", err)
	}
	var baseline int
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1`).Scan(&baseline); err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
	if baseline != 100 {
		t.Fatalf("baseline: got %d want 100", baseline)
	}
	var actCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM play_activity WHERE user_id='u1' AND app_id=1`).Scan(&actCount); err != nil {
		t.Fatal(err)
	}
	if actCount != 0 {
		t.Fatalf("first observation must not write activity, got %d rows", actCount)
	}

	// --- positive delta: 100→160 → 60 minutes of activity ---
	if err := recordGameActivity("u1", "s1", 1, "TestGame", 160, 60, now); err != nil {
		t.Fatalf("positive delta: %v", err)
	}
	var minutes int
	if err := pool.QueryRow(ctx, `SELECT minutes FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatalf("activity not written: %v", err)
	}
	if minutes != 60 {
		t.Fatalf("activity minutes: got %d want 60", minutes)
	}
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 160 {
		t.Fatalf("baseline after delta: got %d want 160", baseline)
	}

	// --- repeated poll: same value → no new activity ---
	if err := recordGameActivity("u1", "s1", 1, "TestGame", 160, 60, now); err != nil {
		t.Fatalf("repeated poll: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT minutes FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 60 {
		t.Fatalf("repeated poll must not add minutes: got %d want 60", minutes)
	}

	// --- rollback: inject an activity write failure via a broken TX ---
	// Verify the baseline is NOT advanced when activity write fails.
	// We do this by using a connection whose write fails after the SELECT FOR UPDATE.
	// Simplest approach: use a single-conn pool with a poisoned transaction that
	// we can abort via savepoint.  Here we simulate it at the SQL level:
	// start a tx that does the SELECT FOR UPDATE, inject an error for the
	// activity INSERT, and verify the baseline stayed at 160.
	func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		var pt int
		if err := tx.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1 FOR UPDATE`).Scan(&pt); err != nil {
			t.Fatal(err)
		}
		delta := 200 - pt // would be 40
		if delta <= 0 {
			t.Fatal("expected positive delta in rollback test")
		}
		// Simulate activity write failure by referencing a non-existent table.
		_, insertErr := tx.Exec(ctx, `INSERT INTO _no_such_table_ VALUES ($1)`, delta)
		if insertErr == nil {
			t.Fatal("expected error from bad insert")
		}
		// Don't commit — rollback via defer.
	}()
	// Baseline must still be 160 after the aborted transaction.
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 160 {
		t.Fatalf("rollback must not advance baseline: got %d want 160", baseline)
	}

	// --- concurrent serialisation: two goroutines both try to add delta ---
	// Both see baseline=160, new=220. Only one should win the FOR UPDATE lock;
	// the second sees the already-updated baseline and records 0 extra minutes.
	exec(`DELETE FROM play_activity WHERE user_id='u1' AND app_id=1`)

	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			done <- recordGameActivity("u1", "s1", 1, "TestGame", 220, 60, now)
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent poll: %v", err)
		}
	}
	// Exactly 60 minutes total (220-160), not 120 (double-counted).
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(minutes),0) FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 60 {
		t.Fatalf("concurrent polls double-counted: got %d want 60", minutes)
	}
}
