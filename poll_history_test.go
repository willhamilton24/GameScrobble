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
	if tx.execCalls != 2 {
		t.Fatalf("expected 2 execs (advisory lock + baseline), got %d", tx.execCalls)
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
	if tx.execCalls != 3 {
		t.Fatalf("expected 3 execs (advisory lock + activity + baseline), got %d", tx.execCalls)
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
	if tx.execCalls != 2 {
		t.Fatalf("expected 2 execs (advisory lock + baseline), got %d", tx.execCalls)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatal("expected commit")
	}
}

// TestPollGameActivity_Rollback: activity write fails → baseline exec never called, tx rolled back.
func TestPollGameActivity_Rollback(t *testing.T) {
	tx := &fakePollTx{
		histRow:     pollRow{val: 50},
		execResults: []error{nil, errPollTest}, // lock ok, activity fails
	}
	usePollStarter(t, &fakePollStarter{tx})

	err := recordGameActivity("u1", "s1", 1, "Game", 150, 60, time.Now())
	if !errors.Is(err, errPollTest) {
		t.Fatalf("expected errPollTest, got %v", err)
	}
	if tx.execCalls != 2 {
		t.Fatalf("expected 2 execs (lock + activity, baseline not reached), got %d", tx.execCalls)
	}
	if tx.committed || !tx.rolledBack {
		t.Fatal("expected rollback, not commit")
	}
}

// TestPollGameActivity_BaselineRollback: baseline write fails → tx rolled back.
func TestPollGameActivity_BaselineRollback(t *testing.T) {
	tx := &fakePollTx{
		histRow:     pollRow{val: 50},
		execResults: []error{nil, nil, errPollTest}, // lock ok, activity ok, baseline fails
	}
	usePollStarter(t, &fakePollStarter{tx})

	err := recordGameActivity("u1", "s1", 1, "Game", 150, 60, time.Now())
	if !errors.Is(err, errPollTest) {
		t.Fatalf("expected errPollTest, got %v", err)
	}
	if tx.execCalls != 3 {
		t.Fatalf("expected 3 execs, got %d", tx.execCalls)
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
		wantExec int // 2 = lock + baseline, 3 = lock + activity + baseline
	}{
		{"first poll (stored=100, new=100, no delta)", 100, 100, 2},
		{"second poll same value",                    100, 100, 2},
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
// It verifies rollback and retry (activity failure does not advance the
// baseline or lose minutes), first observation, repeated and stale polls (no
// duplicate activity, no baseline regression), and concurrent serialisation
// (the advisory lock prevents double-counting, even with no prior row).
//
// Set POLL_TEST_DATABASE_URL to a disposable PostgreSQL database to enable.
func TestPollHistoryPostgres(t *testing.T) {
	pgURL := os.Getenv("POLL_TEST_DATABASE_URL")
	if pgURL == "" {
		t.Skip("set POLL_TEST_DATABASE_URL to a disposable PostgreSQL database")
	}

	ctx := context.Background()

	schema := fmt.Sprintf("poll_qa_%d", time.Now().UnixNano())

	cfg, err := pgxpool.ParseConfig(pgURL)
	if err != nil {
		t.Fatal(err)
	}
	// Set search_path on every connection the pool opens — a per-connection
	// `SET search_path` would only affect one pooled connection, and the
	// concurrent goroutines below get fresh connections.
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + schema)
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") //nolint:errcheck

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

	// --- rollback: a trigger makes the activity INSERT inside recordGameActivity
	// fail, so its transaction must roll back: no baseline advance, no minutes.
	exec(`CREATE FUNCTION poll_fail() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'injected'; END $$ LANGUAGE plpgsql`)
	exec(`CREATE TRIGGER poll_fail_trg BEFORE INSERT OR UPDATE ON play_activity FOR EACH ROW EXECUTE FUNCTION poll_fail()`)

	if err := recordGameActivity("u1", "s1", 1, "TestGame", 200, 60, now); err == nil {
		t.Fatal("expected error from injected activity-write failure")
	}
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 160 {
		t.Fatalf("rollback must not advance baseline: got %d want 160", baseline)
	}
	if err := pool.QueryRow(ctx, `SELECT minutes FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 60 {
		t.Fatalf("failed poll must not record minutes: got %d want 60", minutes)
	}

	// --- retry after the failure: the failed poll lost nothing ---
	exec(`DROP TRIGGER poll_fail_trg ON play_activity`)
	if err := recordGameActivity("u1", "s1", 1, "TestGame", 200, 60, now); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 200 {
		t.Fatalf("retry must advance baseline: got %d want 200", baseline)
	}
	if err := pool.QueryRow(ctx, `SELECT minutes FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 100 {
		t.Fatalf("retry must record the 40 minutes the failed poll saw: got %d want 100", minutes)
	}

	// --- stale poll: a smaller total must not lower the baseline or be
	// double-counted when the real total comes back.
	exec(`DELETE FROM play_activity WHERE user_id='u1' AND app_id=1`)
	if err := recordGameActivity("u1", "s1", 1, "TestGame", 150, 60, now); err != nil {
		t.Fatalf("stale poll: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM play_activity WHERE user_id='u1' AND app_id=1`).Scan(&actCount); err != nil {
		t.Fatal(err)
	}
	if actCount != 0 {
		t.Fatalf("stale poll must not record activity, got %d rows", actCount)
	}
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=1`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 200 {
		t.Fatalf("stale poll must not lower baseline: got %d want 200", baseline)
	}
	if err := recordGameActivity("u1", "s1", 1, "TestGame", 220, 60, now); err != nil {
		t.Fatalf("poll after stale: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(minutes),0) FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 20 {
		t.Fatalf("poll after stale: got %d minutes, want 20 (220-200, not 220-150)", minutes)
	}

	// --- concurrent serialisation: two goroutines both try to add delta ---
	// Both see baseline=220, new=280. Only one should win the lock; the second
	// sees the already-updated baseline and records 0 extra minutes.
	exec(`DELETE FROM play_activity WHERE user_id='u1' AND app_id=1`)

	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			done <- recordGameActivity("u1", "s1", 1, "TestGame", 280, 60, now)
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent poll: %v", err)
		}
	}
	// Exactly 60 minutes total (280-220), not 120 (double-counted).
	if err := pool.QueryRow(ctx, `SELECT COALESCE(SUM(minutes),0) FROM play_activity WHERE user_id='u1' AND app_id=1 AND date=$1`, today).Scan(&minutes); err != nil {
		t.Fatal(err)
	}
	if minutes != 60 {
		t.Fatalf("concurrent polls double-counted: got %d want 60", minutes)
	}

	// --- concurrent first observation: no play_history row exists, so FOR
	// UPDATE alone has nothing to lock; the advisory lock must serialise.
	exec(`DELETE FROM play_activity WHERE user_id='u1' AND app_id=2`)
	exec(`DELETE FROM play_history WHERE user_id='u1' AND app_id=2`)
	for i := 0; i < 2; i++ {
		go func() {
			done <- recordGameActivity("u1", "s1", 2, "FreshGame", 100, 10, now)
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent first observation: %v", err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM play_activity WHERE user_id='u1' AND app_id=2`).Scan(&actCount); err != nil {
		t.Fatal(err)
	}
	if actCount != 0 {
		t.Fatalf("concurrent first observation must not record activity, got %d rows", actCount)
	}
	if err := pool.QueryRow(ctx, `SELECT playtime_total FROM play_history WHERE user_id='u1' AND app_id=2`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	if baseline != 100 {
		t.Fatalf("concurrent first observation baseline: got %d want 100", baseline)
	}
}
