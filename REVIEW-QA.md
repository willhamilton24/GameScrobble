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

## PR #77 landing-page review fixes — 2026-10-25

- Sample reporting period is January 1–December 31, 2026 UTC. A deterministic fictional record set drives all totals, rankings, heatmap cells, and the five-row CSV excerpt. CSV uses the endpoint's column names and UTC timestamp format and describes per-game snapshots.
- Removed Priority sync, Sortable game library, and All-time playtime totals from Premium marketing. Premium intent reuses Account authentication and focuses its existing checkout control; no automatic payment request occurs on signup.
- Regression checks: `TZ=UTC CI=true npm test -- --watch=false --runInBand` and `TZ=America/New_York CI=true npm test -- --watch=false --runInBand`: 5 suites, 14 tests passed in each. Coverage includes four clocks in 2027/2028, a shared fixture fingerprint across timezones, 365 rendered dates, fixture aggregates, anonymous signup and restored-session CTA paths, and the existing checkout request.
- `npm run build`: passed; existing dependency deprecation/outdated Browserslist notices remain.
- Built-page Chromium inspection at 1440px and 375px: page scroll width equals viewport width. At 375px, heatmap and CSV containers are 325px wide with independently scrollable content (737px and 530px); both scrollLeft values reached 100. Desktop content fits. Full-page screenshots inspected for both widths.
- No git/gh, merge, deployment, live accounts, or payments used.
