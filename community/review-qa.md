# PR 73 review-fix QA

Verified 2026-10-25T02:53:06.529649+00:00 against the corrected working tree below. No git/gh commands, commits, pushes, merges, or production deployment were performed. This is new local QA of the fixes, not a rerun against unchanged c84dfb1.

- PASS: `go test ./... -count=1 -v` with `RETENTION_TEST_DATABASE_URL` set to a disposable local PostgreSQL 15.17 database. Whole package passed (1.126s).
- PASS: `TestRetentionAuthorization`, including missing/wrong/header/query-string credentials and no-store responses.
- PASS: `TestRetentionDatabaseFailures` for tracking-start query/Scan, cohort query, Scan and rows.Err; no partial metrics or sensitive error logs.
- PASS: `TestActivityFailureIsBestEffort` and `TestSignupSourceValidation` (handler rejects non-ASCII, oversized, control-character and unsupported values).
- PASS: `TestRetentionPostgres`: migrations twice on empty and legacy schemas; existing user and tracking-start preservation; deterministic UTC D1/D7/D30 maturity boundaries, overall/source counts, pre-tracking unavailability, repeated activity deduplication, rates and empty database output.
- PASS: `CI=true npm test -- --watch=false --runInBand`: 3 suites, 7 tests, including initial campaign entry, navigation to registration, actual registration payload, direct/unknown behavior, and oversized/malformed input.
- PASS: `npm run build`: optimized production build compiled successfully. Existing dependency deprecation and Browserslist-age notices remain.
- Stripe v87 E2E source compiled. Its existing environment-gated live browser checkout test was skipped; no tests were disabled and no new live Stripe delivery evidence is claimed.
- Community claims checked against implementation; no messages were posted. Actual drafts contain attributed public URLs and instructions to check rules/permission before posting.

## SHA-256 of reviewed files

```text
aeeb0b0a25e9cde086ec641af8066bb1aa7f1abc24e31aa04f9f2f93d30dc839  auth.go
a69bc772757f49d894ca068da04e85bfa5959f9754cfd0bdaf7c22da2fd3b0fc  db.go
7b0498eac1d08727bf863f512cfc348c9b4d3be7882974e3ace877c438ac7137  retention.go
af550f3ccfc9efedef37e3831b60314eb107c0d4ca850aa7b89bc924f6a19e5f  retention_test.go
0d402cf2749594df8a98b722914aa62dde58c39b4034c919d8534e7d4e211670  stripe_e2e_test.go
4627bd940bcc9c7386346ee0d36aada3ad4ac71a0d3cfed9350e1dc1bf08dbb1  src/index.js
1c1029a4aca9e07f6ae2068b8308b54ddbbe0a40470e73e93b96d6bd65193aff  src/Account.js
23af39a1f76e763bc652d459883065eb6bd209c0b27fcb58d3cdb19ec42cd16e  src/Account.test.js
1953a7f36f47c8a89b090589a4d8e39032b3abd510596f1b520319dbefbfb9b3  src/App.js
efa665ffe05d830a239121eac7e3456f34c50dbd9d445c504bd15c334c1bcfd4  src/attribution.js
d5aef32491435fb5404241a4d837785c52b420bf8cebd7b10609b23153b829ac  src/attribution.test.js
fbc9dad4ba27d6fe2ecbde1560841d1f9b39f083a2386c53fd717108faf148f9  community/backloggd.txt
6f980dacced83a515ca8bbbe146df29c4f68400760947bc47504a6b6e657ff1c  community/reddit-patientgamers.txt
9a72c3778300b2fa750ce5f86d79babca788ced4321251afcae2f94c18115f6b  README.md
```
