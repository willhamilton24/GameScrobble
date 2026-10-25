# Review-fix verification — 2026-10-24

These are new executions for this review, not coverage inferred from an earlier failed QA run.

- `stripe_e2e_test.go`: changed the stale SDK import to the existing stripe-go/v87 dependency and set the raw HTTP Stripe-Version header to `stripe.APIVersion` (2026-09-30.endive). Its referenced response types compile unchanged with v87; no additional dependency or test disabling was needed. Live hosted Checkout was not executed: the existing RUN_STRIPE_E2E opt-in remains in place.
- `main.go`: all three banners now say `gameroom API v1.0.0`. Actual server requests to `/api`, `/game/portal`, and `/search/portal` returned HTTP 200 with that exact body on fresh, migrated, and repeated successful startups.
- Polling's remaining context identifiers are now `historyCtx` and `historyCancel`; behavior is unchanged.
- `go test ./...`: passed. A further uncached verbose run including the temporary database harness also passed; the existing live Stripe E2E test was skipped by its existing opt-in guard. Local Stripe smoke/signature/persistence tests passed.
- `CI=true npm test -- --watch=false --runInBand`: passed, 2 suites / 3 tests.
- `npm run build`: compiled successfully. Existing util._extend deprecation and outdated caniuse-lite warnings remain.

## Disposable PostgreSQL checks

Used a new local PostgreSQL 15 cluster with trust authentication on a private temporary Unix socket, no TCP listener, and three disposable databases. No shared/production database or platform credentials were used. A freshly compiled application binary exercised actual startup/migration and banner routes. A temporary Go harness exercised production polling and route handlers with real PostgreSQL and the production premium store; only Steam's HTTP response was stubbed.

| Check | Actual result |
| --- | --- |
| Fresh startup, then restart | Both succeeded; empty play_history created; all three banner routes passed. |
| Populated legacy table | Created the previous seven-column signature and composite primary key, with three rows spanning two users and two game IDs. Startup renamed the table; every column value (including timestamps, identifiers, title, total and recent minutes) matched before/after. Old table name disappeared. |
| Repeated migrated startup | Succeeded; all three rows and every value remained identical. Duplicate inserts failed with a unique-key violation after each startup. |
| Ambiguous matching tables | Two populated tables with matching signatures caused startup to fail with `query returned more than one row`, on both attempts. Both tables and their values remained unchanged; play_history was not created. Existing fail-safe behavior required no code change. |
| Polling migrated data | Linked-user poll changed total/recent minutes from 120/30 to 150/45, refreshed the timestamp, retained identifiers/title, and inserted a new game. Repeating the same poll kept four history rows and exactly 30 activity minutes, without duplicate activity. |
| Deep stats | HTTP 402 without premium; after inserting a disposable premium entitlement, HTTP 200 with three games ordered by totals 150, 90, 12. |
| Public profile | HTTP 200; one active game, 30 total minutes, matching top-game and daily heatmap values. |
| Premium CSV | HTTP 402 without premium; HTTP 200 with premium, correct CSV content type and gameroom_history.csv disposition. Parsed header plus three rows; comma-containing title and totals/recent minutes were correct. Expiring entitlement restored HTTP 402. |
| Migration after feature checks | Succeeded again and retained all four history rows. |

The first temporary feature-harness attempt failed because its manually constructed fasthttp context lacked server initialization. That attempt is **not** counted as feature coverage. The harness was corrected to use RequestCtx.Init, all three databases were recreated, and the startup checks plus full uncached Go suite/harness then passed. The temporary harness was removed from the repository after execution; no machine-specific test paths were added to the test suite.

Local evidence and harnesses: `/tmp/gameroom-review.X8AsuI/` (`check_startup.py`, `startup-results.log`, `review_postgres_qa_test.go`, `go-tests.log` for the failed attempt, `go-tests-rerun.log` for the successful rerun). Application children were stopped via their own subprocess handles; the disposable database server was stopped by its recorded launch PID. No git/gh commands, commits, merges, or deployments were performed.
