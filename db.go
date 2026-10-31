package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// txStarter opens a Postgres transaction. Kept separate from database so tests
// can swap only the transactional path without affecting the query interface.
type txStarter interface {
	Begin(context.Context) (pgx.Tx, error)
}

var db database
var dbPool txStarter

const dbTimeout = 5 * time.Second

func databaseURL() string {
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	return u
}

func initDB(ctx context.Context) {
	pool, err := pgxpool.New(ctx, databaseURL())
	if err != nil {
		log.Fatal("failed to connect to database: ", err)
	}
	db = pool
	dbPool = pool
}

func migrateDB(ctx context.Context) {
	if err := runMigrations(ctx); err != nil {
		log.Fatal("failed to run database migrations: ", err)
	}
}

func runMigrations(ctx context.Context) error {
	_, err := db.Exec(ctx, `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT        PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at  TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS signup_source  TEXT NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS retention_tracking (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO retention_tracking (singleton) VALUES (TRUE) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS user_return_activity (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_date DATE NOT NULL,
    PRIMARY KEY (user_id, activity_date)
);
CREATE TABLE IF NOT EXISTS steam_links (
    user_id            TEXT    PRIMARY KEY,
    steam_id           TEXT    NOT NULL,
    ownership_verified BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE TABLE IF NOT EXISTS xbox_links (
    user_id   TEXT        PRIMARY KEY,
    xuid      TEXT        NOT NULL,
    gamertag  TEXT        NOT NULL,
    linked_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS psn_links (
    user_id    TEXT        PRIMARY KEY,
    account_id TEXT        NOT NULL DEFAULT '',
    online_id  TEXT        NOT NULL,
    linked_at  TIMESTAMPTZ NOT NULL
);
CREATE TABLE IF NOT EXISTS epic_links (
    user_id      TEXT        PRIMARY KEY,
    epic_id      TEXT        NOT NULL,
    display_name TEXT        NOT NULL DEFAULT '',
    linked_at    TIMESTAMPTZ NOT NULL
);
-- Identify the previous play-history table by its complete column signature so
-- existing installations retain their data when adopting the new table name.
DO $$
DECLARE
    previous_table TEXT;
BEGIN
    IF to_regclass('public.play_history') IS NULL THEN
        SELECT table_name INTO STRICT previous_table
        FROM information_schema.columns
        WHERE table_schema = 'public'
        GROUP BY table_name
        HAVING array_agg(column_name::text ORDER BY column_name) =
            ARRAY['app_id', 'last_polled', 'name', 'playtime_recent', 'playtime_total', 'steam_id', 'user_id'];
        EXECUTE format('ALTER TABLE public.%I RENAME TO play_history', previous_table);
    END IF;
EXCEPTION
    WHEN no_data_found THEN NULL; -- Fresh installations create the table below.
END $$;
CREATE TABLE IF NOT EXISTS play_history (
    user_id         TEXT        NOT NULL,
    steam_id        TEXT        NOT NULL DEFAULT '',
    app_id          INTEGER     NOT NULL,
    name            TEXT        NOT NULL DEFAULT '',
    playtime_total  INTEGER     NOT NULL DEFAULT 0,
    playtime_recent INTEGER     NOT NULL DEFAULT 0,
    last_polled     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, app_id)
);
CREATE TABLE IF NOT EXISTS play_activity (
    user_id TEXT    NOT NULL,
    app_id  INTEGER NOT NULL,
    name    TEXT    NOT NULL DEFAULT '',
    date    TEXT    NOT NULL,
    minutes INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, app_id, date)
);
CREATE TABLE IF NOT EXISTS premium_users (
    user_id            TEXT        PRIMARY KEY,
    premium_until      TIMESTAMPTZ NOT NULL,
    stripe_customer_id TEXT        NOT NULL DEFAULT ''
);
ALTER TABLE premium_users ADD COLUMN IF NOT EXISTS payment_problem BOOLEAN NOT NULL DEFAULT FALSE;
CREATE TABLE IF NOT EXISTS games (
    title              TEXT  PRIMARY KEY,
    img                TEXT  NOT NULL DEFAULT '',
    dev                TEXT  NOT NULL DEFAULT '',
    pub                TEXT  NOT NULL DEFAULT '',
    rdate              TEXT  NOT NULL DEFAULT '',
    metacritic         TEXT  NOT NULL DEFAULT '',
    pc_offers          JSONB NOT NULL DEFAULT '[]',
    xbox_offers        JSONB NOT NULL DEFAULT '[]',
    playstation_offers JSONB NOT NULL DEFAULT '[]',
    switch_offers      JSONB NOT NULL DEFAULT '[]',
    platforms          JSONB NOT NULL DEFAULT '[]'
);
`)
	return err
}
