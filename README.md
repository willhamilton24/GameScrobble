# gameroom

![Concept](https://img.shields.io/badge/Concept-C-yellow) ![Effort to Cashflow](https://img.shields.io/badge/Effort_to_Cashflow-85%2F100-red)

+++User /u
+++Steam /s
    Links
    1. https://developer.valvesoftware.com/wiki/Steam_Web_API#GetPlayerSummaries_.28v0002.29
    2. ChromeDP

    Routes
        /s/link - add steam account to user
        /s/status - see game being played on linked steam account
        /s/recent - get recent steam games on linked account

    Functions
        linkAccount(username, password, user) -> Success / Error  - links steam account to user with that user
        getCurrentGame(user) -> String / Null   - check if and what linked steam account is playing
        getRecentGames(user) -> Array / Error  - get recent steam games on linked account
        getSteamLibrary(user) -> Array / Error - get steam library and total playtime stats for account

+++Xbox /x
+++PSN /p
+++Riot /r
+++Blizzard /b
+++Epic /e

## Setup

The Steam routes (`/s/link`, `/s/status`, `/s/recent`) use the official Steam Web API and require one environment variable:

- `STEAM_API_KEY` — a Steam Web API key, obtained at https://steamcommunity.com/dev/apikey. Set it in the deploy environment (e.g. the Render service's env vars); never commit the value to the repo.

### gameroom naming update

The application now uses `gameroom.token` for browser sessions; existing users must sign in again after this update. CSV downloads use `gameroom_history.csv`.

On startup, the database migration identifies the previous history table by its complete column signature and renames it to `play_history`, preserving existing rows. Fresh databases create `play_history` directly; repeated startup leaves it intact. Multiple matching legacy tables cause migration to fail rather than choose a table arbitrarily.

Keep deployments attached to the existing Render service when applying its `gameroom-api` display name; the public URL remains https://gameroom.hdynamic.app.

## 💰 Path to Revenue
gameroom tracks observed Steam playtime changes with a Go API, JWT accounts, public profile heatmaps, and a React frontend. Persistence uses Postgres via `DATABASE_URL`. Premium gates additional statistics and CSV export through Stripe test-mode checkout and webhooks. Xbox/PSN/Epic OAuth linking code exists, but does not provide unified playtime tracking. Community launch drafts and exact-day retention measurement are described below; platform credentials and community posting remain operator-managed.

### Release TODOs
- [x] Finish the Steam integration end-to-end using official Web API keys (drop the ChromeDP password-login approach — it is a security and ToS liability)
- [x] Implement user accounts + OAuth linking for Xbox, PSN, and Epic (currently just route placeholders)
- [x] Replace committed node_modules and stray frontend artifacts with a clean repo layout (.gitignore)
- [x] Build a public profile page ("year in games", playtime heatmap) — the shareable artifact that drives signups
- [x] Deploy config (render.yaml) + scheduled play-history polling
- [x] Add a premium tier (deeper stats, history export) via Stripe and affiliate links to game deals
- [x] Migrate persistence from MongoDB to Neon Postgres via `DATABASE_URL` (shared platform; `MONGODB_URI` is not provisioned, so nothing works in the deployed environment until this ships) — update render.yaml accordingly
- [x] Complete Stripe test-mode E2E verification of signup → application Checkout → webhook → premium CSV export → cancellation gating. Monkey successfully verified deployed head `7606188` on 2026-10-24 at `https://gameroom.hdynamic.app`: signup, application Checkout session `cs_test_a19vxivBRuRlzuLEPlGhrcdzDygKIxgEwKbqBRFLerwGxMqyWY1NSl61j3`, webhook delivery, premium unlock with CSV export returning HTTP 200 and correct headers (`Content-Type: text/csv` and an attachment filename), cancellation of subscription `sub_1UTxCgGRjKD6n9dmD6IDQi4R`, and restored HTTP 402 export gating. This records Monkey's verification of the deployed application flow; it is not evidence that the revised `TestStripeE2E` ran. Execution of that Go test remains unverified.
- [ ] Set platform credentials: `STEAM_API_KEY`, plus Xbox (`XBOX_CLIENT_ID`/`SECRET`), PSN, and Epic OAuth app registrations (operator sign-ups; Steam linking alone is enough to launch)
- [ ] Verify the Render deploy end-to-end and point the Stripe webhook at the live URL (`https://gameroom.hdynamic.app`). Deployment is authorized; frontend, UI authentication persistence, and Stripe delivery/application processing must all have actual verification evidence before completion. See the verification record below. Mark complete only when actual verification evidence is recorded.
- [ ] Launch to gaming communities (r/patientgamers, Backloggd users) and validate retention (deployment switch on; posting and mature cohorts pending)
- [ ] Enforce account ownership on premium stats, CSV export, and checkout: reuse the existing JWT middleware and derive the user from the session instead of trusting `user_id`; download CSV with authenticated fetch in `src/Account.js`. Verify anonymous and cross-account requests cannot access another subscriber's data using local fixtures.
- [x] Build a Premium stats view for the existing `/u/stats` response: show all-time playtime and a sortable game table, with loading, empty, retry, and free-tier upgrade states. Verify populated and empty accounts with fixture responses so buyers can use the advertised deeper stats without platform API credentials.
- [x] Implement distinct `/premium/success` and `/premium/cancel` screens in `src/App.js`; expose the signed-in account's server-verified entitlement, show pending activation with bounded refresh, and offer export only after activation. Test active, delayed-webhook, cancelled, and expired states locally; never unlock from the return URL alone.
- [x] Add a clearly labeled sample year-in-games profile and a Free versus Premium feature comparison to the landing page, linking to signup and the existing checkout flow. Use bundled fictional activity, separate from real accounts, to demonstrate the heatmap, stats, and CSV value without Steam credentials or live ingestion.
- [x] Make public profiles recoverable and easy to share: distinguish not-found from server/network errors in `src/ProfilePage.js`, add retry, and provide a selectable profile URL when clipboard access fails or is unavailable. Verify all failure states and successful copying with frontend fixtures to protect the signup referral loop.

- [ ] Make play-history polling atomic per user/game in `pollUserPlayHistory`: read the prior total, record any positive daily delta, and update the playtime baseline in one Postgres transaction, serializing concurrent polls for the same game. A failed activity write must not advance the baseline and lose recorded minutes. Verify rollback, retry, first observation, and concurrent/repeated polls with local database fixtures so subscribers can trust their history and exports.
- [ ] Add explicit session-restoration failure and retry controls in `src/Account.js`: retain the stored token after network/5xx failures, distinguish these from expired sessions, and retry `/auth/me` without requiring another login or signup. Prevent duplicate retries and ignore stale responses after logout; verify recovery and genuine 401 expiry with mocked responses to protect returning-user activation.
- [ ] Make the profile heatmap use UTC consistently for date generation and label its rolling activity window separately from calendar-year totals. Add a keyboard/touch-accessible date-and-playtime readout instead of relying only on SVG hover titles. Verify UTC boundary, daylight-saving, year-boundary, and empty-history fixtures in `src/ProfilePage.js` so the shareable profile presents accurate, readable activity.
- [x] Add game-name search and an active-in-last-two-weeks filter to `src/PremiumStats.js`, preserving the selected sort and providing a clear-filters action and distinct no-match state. Keep overall tracked-game totals separate from filtered totals; verify populated, zero-recent-playtime, and empty fixtures so subscribers can navigate larger histories without changing Premium access rules.

### Premium stats view verification

Signed-in accounts can select **View Premium stats** in the account panel. The
view fetches `/u/stats` with the session bearer token and the existing `user_id`
parameter, totals the returned game minutes, and sorts by game, all-time playtime,
recent playtime, or last sync. HTTP 402 offers the existing Premium checkout action;
errors offer retry, and successful empty responses show zero playtime with sync
guidance. The separate backend account-ownership TODO remains open.

Fictional populated, empty, and free-tier responses live in
`src/fixtures/premiumStats.json`; no platform API credentials are needed. Local
verification: `CI=true npm test -- --watch=false --runInBand --runTestsByPath src/PremiumStats.test.js src/Account.test.js`
passed 12 tests covering loading, totals, both sort directions, empty accounts,
upgrade/recheck, error/retry, stale responses, and signed-in account integration.
`npm run build` passed. The full frontend suite has an unrelated existing failure
in `src/App.test.js`, which expects outdated landing-page copy. These are local
fixture checks, not live platform or payment verification.

The local `TestStripeSmoke` test provides in-process webhook/gating coverage using signed synthetic events, in-memory premium state, and deterministic export data. It verifies premium activation, invoice expiration extension, successful CSV export, and 402 gating before activation and after cancellation. It does not exercise signup, checkout creation, or delivery by Stripe.

The revised `TestStripeE2E` (`stripe_e2e_test.go`) registers a new application user, checks the initial 402 gate, and calls `/api/stripe/checkout`. It retrieves that session from Stripe and requires test mode, the registered user's metadata, the provisioned price `price_1USUwuGRjKD6n9dmujp5uGyk`, and return URLs on `gameroom.hdynamic.app`.

To run it, provide the platform's existing test-mode `STRIPE_SECRET_KEY` securely through the environment (do not place credentials in command arguments or logs), then run:

```sh
RUN_STRIPE_E2E=1 go test -run '^TestStripeE2E$' -v -count=1 -timeout 15m
```

**A separate browser step is required.** While the command runs, open the hosted Checkout URL from the private temporary file named in the test output and complete test-mode Checkout within ten minutes. The Go command alone does not complete E2E. The URL file is removed when the test ends. No Stripe CLI, synthetic checkout trigger, separate customer, or trial subscription is used.

After actual paid Checkout completion, the test requires a matching `checkout.session.completed` event with no pending webhook deliveries before checking premium CSV access. It verifies the provisioned application webhook is enabled and subscribed; Stripe's pending-delivery count is account-wide, not a per-endpoint delivery receipt, so the test also requires the application's resulting gating transitions. It then cancels only the subscription belonging to that Checkout, waits for its `customer.subscription.deleted` event to have no pending deliveries, and requires restored 402 gating. The revised test asserts exact CSV response headers and the CSV column header. Monkey's verification above independently confirms HTTP 200 and correct CSV response headers; it does not establish execution of these Go assertions.

Once opted in, missing prerequisites, request/response errors, and timeouts fail the test and stop dependent stages. HTTP requests and the overall run are bounded; there are no subprocesses or credential-bearing diagnostic dumps. If a run fails after payment but before cancellation, use the logged subscription ID to cancel that test subscription. Normal `go test ./...` skips this interactive live test; passing local tests does not count as executing it.

### Deployment verification record (UTC)

The earlier completion claim is withdrawn. Monkey reproduced `GET /` → HTTP 503 (`Frontend not built. Run: npm run build`). The earlier API 200 and registration/JWT results do not establish a working frontend or UI signup/logout/login persistence. An unsigned webhook HTTP 400 only demonstrates rejection of that request; it does not establish Stripe-originated delivery or application processing.

- 2026-10-23T03:21:24.876Z: Render API inspection of existing service `srv-dban3t0m7kps739go4jg` found branch `master`, empty root directory, build command `go build -o server .`, and start command `./server`. This omitted the frontend build. The repository's `render.yaml` already correctly builds React before Go, so no YAML or server path change was needed: React emits `build/index.html`, and `serveFrontend` reads that path relative to the service root.
- 2026-10-23T03:21:25.053Z — **configuration evidence only**: Stripe API retrieved existing webhook `we_1USUwxGRjKD6n9dmwTvgZ6iJ`: URL `https://gameroom.hdynamic.app/api/stripe/webhook`, `livemode=false`, `status=enabled`, `api_version=null`. Enabled events: `checkout.session.completed`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.payment_failed`. `invoice.payment_succeeded`, which the application uses to extend premium entitlement, is absent. The company provisioning CLI returned this same existing endpoint at 03:21:25.331Z; no account or endpoint was created and no signing secret was rotated. The CLI's event list is not delivery evidence.
- 2026-10-23T03:21:43.471Z: Updated that existing Render service to PR branch `throne/mvkcmcf7-verify-the-render-deploy-end-to-end-and-`, with the repository's build command `npm install && npm run build && go build -o server .`, start command `./server`, and empty root directory. Triggered deploy `dep-dbdd5hrncjis739pio80` at 03:21:43.995Z for commit `f796892308a36223c3fa7ea7616da4a585175246`. Version control remains with the daemon.
- 2026-10-23T03:21:57Z: Local `npm run build` succeeded, `test -s build/index.html` passed, and `go test ./...` passed. Local checks are not live deployment evidence.
- 2026-10-23T03:22:11.722Z, while the corrected deployment was building: independently reproduced `GET /` → HTTP 503 with the same missing-frontend message; `GET /api` → HTTP 200 at 03:22:11.784Z.
- At the time of that deployment, UI authentication verification remained pending: `src/App.js` rendered the landing page or public profile, with no signup, logout, or login UI. The follow-up below adds these controls locally. API-only authentication checks cannot satisfy this requirement. The browser attempt also returned `net::ERR_BLOCKED_BY_CLIENT`; no UI workflow was completed.
- 2026-10-23T03:23:25.788442Z: Render reports deploy `dep-dbdd5hrncjis739pio80` finished **live**, confirmed via the deployment API at 03:23:33.199Z. At 03:23:33.287Z, `GET /` returned HTTP 200 with the React HTML document. At 03:24:05.690Z–03:24:05.859Z, its referenced CSS and both external JavaScript bundles returned HTTP 200 with the appropriate content types. This verifies frontend artifact serving; browser rendering remains unverified because Chrome blocked the page. The served main bundle also contains no signup/login/logout controls.
- 2026-10-23T03:23:45.258Z: Baseline `GET /u/export?user_id=pr65-webhook-20261023T0323Z` returned HTTP 402. This isolated test metadata ID is not a UI-created account.
- 2026-10-23T03:23:47.852Z — **Stripe-originated event, not successful-delivery evidence**: the installed Stripe CLI, using the company's existing test-mode access, completed `stripe trigger checkout.session.completed --add checkout_session:metadata.user_id=pr65-webhook-20261023T0323Z`. Stripe API inspection at 03:24:05.471Z confirmed test event `evt_1UTZ56GRjKD6n9dme36T55rf`, `livemode=false`, API version `2026-09-30.endive`, and `pending_webhooks=16` (account-wide, not a receipt for this endpoint). Stripe accepted an explicit resend to `we_1USUwxGRjKD6n9dmwTvgZ6iJ` at 03:24:37.853Z. Neither CLI success nor resend acceptance proves an HTTP 2xx delivery. This fixture tests the webhook path, not the application's checkout creation or a UI purchase.
- 2026-10-23T03:24:53.077Z — **application processing not verified**: the same export request still returned HTTP 402, rather than the expected HTTP 200 CSV after premium activation. Render's request-log query for `/api/stripe/webhook` returned no entries at 03:24:52.987Z, so no endpoint-specific successful delivery result was obtained. The event's API version differs from the application's Stripe Go v76 schema and the endpoint has no version pin; investigate delivery responses/version compatibility and the missing renewal event subscription before claiming Stripe E2E success. No webhook validation was bypassed.

Remaining verification:
- [x] Correct the existing Render service build settings and deploy the PR branch.
- [x] Verify live frontend HTML, JavaScript, and CSS are served successfully over HTTPS.
- [x] Verify the existing Stripe webhook URL and test-mode configuration using company platform access.
- [x] Verify frontend rendering and UI signup → logout → login → persisted session/account state on the corrected deployed head (controls added locally; deployment pending).
- [ ] Obtain an endpoint-specific successful Stripe delivery receipt and verify premium activation/CSV access for the corresponding event; verify renewal handling as well.

No secrets, tokens, or test payment credentials are recorded here. No new provider accounts or hosting services were created.

### PR #65 review-fix follow-up — 2026-10-24 UTC (completion pending)

- **03:45:26.843Z:** Render returned latest live deploy `dep-dbdd7e942hec7391jk7g`, commit `dedfb3f988146fb2ce4ba00e7fdd91c500008d40`, finished `2026-10-23T03:27:01.853181Z`. This is the previous branch head, **not** the working-tree changes below. Existing service/build configuration and `render.yaml` need no further change. The daemon must commit/push these fixes before a corrected-head deployment can be triggered or verified; no stale-head redeployment is represented as a corrected release.
- **03:45:27.515Z:** Stripe reconfirmed existing test endpoint `we_1USUwxGRjKD6n9dmwTvgZ6iJ`, live URL `https://gameroom.hdynamic.app/api/stripe/webhook`, API version unset, and missing renewal event. Provisioning with the expanded event list returned the existing endpoint but its implementation does not update existing subscriptions. **03:45:55.725Z:** a direct update to that same endpoint returned HTTP 200 with all four existing subscriptions plus `invoice.payment_succeeded`. Only `enabled_events` was sent; URL and signing secret were preserved. No endpoint/account was created or secret rotated. See [Stripe's endpoint update API](https://docs.stripe.com/api/webhook_endpoints/update).
- **03:45:56.048Z–03:45:56.243Z:** Render application-log search for `stripe` and request-log search for `/api/stripe/webhook`, scoped to the earlier event window (2026-10-23 03:23–03:26 UTC), both returned HTTP 200 with empty log arrays. **03:46:14.387Z:** Stripe accepted another endpoint-specific resend of existing event `evt_1UTZ56GRjKD6n9dme36T55rf`. Queries at **03:46:27.769Z** and **03:46:31.846Z** again returned no matching application/request logs. Resend acceptance is not successful delivery, and empty logs do not prove a delivery response.
- **03:46:48.541Z — diagnostic replay, not a Stripe delivery receipt:** retrieved that real Stripe event and replayed its body with a fresh valid signature using the already-provisioned signing secret, entirely in process without printing credentials. The live application returned **HTTP 400**, explicitly reporting `2026-09-30.endive` versus stripe-go `76.25.0` expecting `2023-10-16`. This demonstrates the handler's API-version incompatibility; it does not establish what response Stripe itself received.
- **Code fix:** upgraded to stripe-go **v87.0.0**, whose schema targets `2026-09-30.endive`; kept `ConstructEvent` signature/timestamp/API-version validation enabled. Renewal attribution now reads `invoice.parent.subscription_details.metadata`. Persistence failures return HTTP 500 for retry instead of acknowledging success. Processing logs include event IDs. Checkout activation preserves any longer entitlement already granted by an earlier invoice, so out-of-order checkout delivery cannot truncate renewal access.
- **UI fix:** signup and login forms call the existing `/auth/register` and `/auth/login` APIs; reload restores the token through `/auth/me`; logout removes the saved browser token. Signed-in controls link the account's profile/CSV export and create checkout through the existing application endpoint. Passwords are never persisted in browser storage.
- **03:52:02Z:** local production frontend build and Go binary build succeeded; `go test ./...` passed. React tests passed (2 suites, 3 tests), covering signup, restored session, logout/login, expired-token removal and server errors with mocked API responses. Go coverage uses signed synthetic events for premium/CSV gating, renewal metadata before checkout, bad/expired signatures, incompatible versions, and retryable persistence failures. These checks are not live Stripe E2E evidence.


- **Independent QA, 03:52:34Z–03:53:22Z (with second reload immediately afterward):** native Chrome rendered the **changed local production build**, served on a temporary loopback proxy forwarding authentication/export/checkout to the existing live backend. UI signup created account `6f5fe9b0f09fc7e7406b8f20a4a6dcb0`; reload preserved the session, logout removed it, and login plus another reload restored the same identity. Direct live CSV export for that account returned **402 at 03:52:34Z**. This demonstrates real account persistence through the existing Postgres-backed APIs and local UI session behavior, **not deployment or QA of the corrected backend head**. The proxy contains no credentials and is not part of the release.
- **Independent QA, 03:54:01Z:** that account's Upgrade button opened application-created checkout `cs_test_a1xQmx95h2Dw5vaviFDLiU9cG6DH4tcsxnKLubEWuZCcRkCFCDHibHDzja`; Chrome displayed Stripe Sandbox, the premium subscription, $4.99/month. No payment was entered/completed, so this session does not establish premium activation or a successful webhook. QA found its cancel URL pointed to the old Render hostname. **03:54:18.014Z:** company provisioning set `APP_BASE_URL=https://gameroom.hdynamic.app` on the existing Render service. A fresh checkout must verify both return URLs after the corrected deployment loads that configuration; the existing checkout retains its old URLs.
- **03:54:37Z:** independent Chrome inspection of the public site still rendered the previous “PUBLIC BETA COMING SOON” page with no account controls, confirming the local UI has not been deployed. **03:54:38.189Z:** Stripe API confirmed the application-created checkout above is test-mode, unpaid, and has `metadata.user_id=6f5fe9b0f09fc7e7406b8f20a4a6dcb0`; both stored return URLs use the old Render hostname. **03:54:38.481Z:** Render environment readback confirmed the corrected `APP_BASE_URL` value.
- **Delivery-response access remains incomplete:** independent QA opened the Stripe endpoint dashboard but was redirected through Stripe login to Google sign-in, with no signed-in company browser session. Provider API access worked for configuration/event inspection; it did not supply an endpoint-specific delivery receipt. No alternative identity, signing-secret rotation, or signature bypass was used.

**Release gate remains open:** deploy the daemon-pushed corrected commit to existing service `srv-dban3t0m7kps739go4jg`, record its full commit/deploy ID and completion timestamp, then obtain fresh independent QA on that exact deployed head. Required evidence remains browser rendering and UI signup → reload → logout → login; an application-created test checkout for that same account; actual endpoint-specific Stripe HTTP 2xx delivery/event IDs; persisted premium activation with CSV **402 → 200**; and renewal delivery with recorded `premium_until` before/after extension. No fixture creation, signed diagnostic replay, or resend acceptance closes this gate. Do not merge/mark complete until all evidence is recorded.


### PR #65 live verification — 2026-10-24 UTC (completion still pending)

This record supersedes the earlier deployment/UI-pending statements only for the checks explicitly verified below. It does not treat Monkey's ERROR as a pass; no new Monkey run was requested on unchanged head `8a5dc89`.

- **Deployment:** direct Render API inspection during this run confirmed existing service `srv-dban3t0m7kps739go4jg` already deployed the daemon-pushed corrected commit **`8a5dc89db1e93eb925a510f2ad76e7d9ab2e941f`** through automatic commit deployment **`dep-dbe2one1egvs73fe916g`**, status **live**, completed **`2026-10-24T03:57:46.753642Z`**. No redundant deployment, service creation, or build-setting change was needed. The service remains on the PR branch with the React-plus-Go build command.
- **Live UI, approximately 04:22–04:23 UTC:** Chrome loaded `https://gameroom.hdynamic.app` directly and rendered the account form. Signup created account **`e8236f3c89e08be2538e8c364220c69f`**. Reload displayed the signed-in account and its profile/export links; logout displayed the login form; login followed by another reload restored the same account ID. This used the deployed frontend and backend, with no local proxy or mocked API.
- **04:23:09Z:** that account's live `/u/export?user_id=e8236f3c89e08be2538e8c364220c69f` returned **HTTP 402** before payment.
- **04:23:22.672Z:** after clicking the live UI's Upgrade button, company Stripe API inspection confirmed application-created checkout **`cs_test_a1VyWk7Z6IJdtu1GnCaxzdJdQLErebI34G69u4m71G1qCXcBazCuX81Wqp`**, `livemode=false`, and the same `metadata.user_id`. Its stored success URL is **`https://gameroom.hdynamic.app/premium/success?session_id={CHECKOUT_SESSION_ID}`** and cancel URL is **`https://gameroom.hdynamic.app/premium/cancel`**. No checkout URL code/configuration change was needed.
- **Test checkout completed:** used Stripe Sandbox's Cash App Pay **Simulate scan → AUTHORIZE TEST PAYMENT** flow, which explicitly states no charge occurs. No card details, real payment, new external account, or paid plan was used. At **04:24:05.490Z**, Stripe returned checkout `status=complete`, `payment_status=paid`, customer **`cus_VUwNoynitWrgGw`**, subscription **`sub_1UTwUsGRjKD6n9dmD10jJYyV`**. Chrome returned to the live success URL and retained the same signed-in account.
- **Application processing evidence from Render**, scoped to the existing service and instance `srv-dban3t0m7kps739go4jg-p6km9`:
  - **04:24:01.438225375Z**, log `27185abe-8b19-43cf-94ff-e0ac8e9489a7`: `stripe webhook: processed event=evt_1UTwUvGRjKD6n9dmvbqewzVD type=checkout.session.completed`.
  - **04:24:02.21035151Z**, log `fb761962-1da8-43af-847b-4667e83d720b`: `stripe webhook: processed event=evt_1UTwUvGRjKD6n9dmoktEAk7Q type=invoice.payment_succeeded` (invoice **`in_1UTwUZGRjKD6n9dm0WeK6VEE`**).
  - These are actual Stripe-originated checkout events, not fixtures, locally signed replays, or resend acknowledgements. Existing signature, timestamp, and API-version validation remain enabled. Inspection of `stripe.go` confirms processing logs follow successful persistence and precede the handler's HTTP 200 response; no source change was needed for activation.
- **Persisted activation and CSV:** live export subsequently returned **HTTP 200** with header `app_id,name,playtime_total_minutes,playtime_recent_minutes,last_polled` (the new account has no game history). At **04:24:33.315Z**, a separate read-only Neon query confirmed account `e8236f3c89e08be2538e8c364220c69f`, customer `cus_VUwNoynitWrgGw`, **`premium_until=2026-11-27 04:23:39+00`**. This verifies database persistence independently of the webhook log and establishes CSV **402 → 200**.
- **Endpoint configuration, 04:22:57.659Z:** existing `we_1USUwxGRjKD6n9dmwTvgZ6iJ` remains enabled, test mode, at `https://gameroom.hdynamic.app/api/stripe/webhook`, including `invoice.payment_succeeded`. No endpoint replacement, secret rotation, signature bypass, or platform resource migration was necessary.
- **Endpoint-specific delivery receipt remains pending:** Stripe dashboard redirected to Google sign-in; company API access is available but the browser has no authenticated company dashboard session. Render request logs for 04:24:00–04:24:10 returned an empty array. The application logs and persisted activation above demonstrate processing, but are not represented as a Stripe endpoint delivery receipt with an observed HTTP response. Account-wide `pending_webhooks` is not used as that receipt.

**Do not merge or mark complete:** endpoint-specific Stripe delivery receipts and renewal before/after evidence must still be recorded. Version control remains with the daemon; this run did not invoke git/gh or commit/push independently.


Renewal diagnostics on the same test subscription:

- **04:24:52–04:25:08Z:** Stripe rejected the legacy scalar `billing_cycle_anchor=now` with HTTP 400; its current API accepts `billing_cycle_anchor[type]=now`. This was a verification-tool parameter issue, not an application failure. The successful reset with `proration_behavior=none` changed the subscription period but did not produce a paid renewal invoice.
- **04:25:35–04:27:30Z:** temporary test schedule `sub_sched_1UTwWRGRjKD6n9dm5SRBAnOR` transitioned the same subscription at **04:26:42Z**. It likewise did not generate the required renewal invoice. The temporary schedule was **released** at 04:27:30Z, preserving the subscription, original price and platform resources. Neither transition is counted as renewal-payment evidence.
- **04:27:39.353Z:** an explicit billing-cycle reset with immediate invoicing produced paid invoice `in_1UTwYIGRjKD6n9dmRJjreU1X`, event `evt_1UTwYJGRjKD6n9dmefZD31Qf`. Render log `783e1f60-c8bf-4c38-9b8d-5e262cecb205` records processing at **04:27:31.556763560Z**. A fresh Neon read showed `premium_until` extended from **2026-11-27 04:23:39+00** to **2026-11-27 04:27:30+00**. However, Stripe labels this invoice `billing_reason=subscription_update`, `amount_paid=0`; this verifies invoice-driven extension, **not a paid recurring-cycle renewal**. The remaining renewal gate is intentionally not checked off on this evidence.
- `go test ./...` passed during this verification run (cached). No application source changes were necessary for the successful live checks above.


**Paid recurring-cycle renewal verified, 04:29 UTC:**

- A short test-only trial was applied to the **same application-created subscription** `sub_1UTwUsGRjKD6n9dmD10jJYyV`, ending at **2026-10-24T04:28:47Z**, to trigger recurring billing without waiting a month. The subscription subsequently returned to `active`. This uses the original company test customer, payment method and project price; no separate provider account or product was created.
- **04:29:19.897Z:** Stripe returned invoice **`in_1UTwZwGRjKD6n9dmWlCPZS4x`**, `billing_reason=subscription_cycle`, `amount_due=499`, initially `draft`. Explicit test-mode finalization and payment using the subscription's existing sandbox payment method returned HTTP 200, `status=paid`, **`amount_paid=499`**. These operations accelerate the test invoice lifecycle, not real-money collection. The invoice's latest covered line-period end is **2026-11-24T04:28:47Z**.
- **04:29:33.651Z:** company Stripe API returned actual test event **`evt_1UTwa6GRjKD6n9dm7XrsqCus`**, `type=invoice.payment_succeeded`, `livemode=false`, for that renewal invoice.
- **04:29:23.280725909Z:** Render application log **`71fc77b8-c3d8-4bbd-9325-aeb65a005f2c`**, scoped to the existing service/instance, records `stripe webhook: processed event=evt_1UTwa6GRjKD6n9dm7XrsqCus type=invoice.payment_succeeded`.
- **04:29:34.152Z:** independent read-only Neon query confirmed **`premium_until` before = 2026-11-27 04:27:30+00; after = 2026-11-27 04:28:47+00**, for the same user/customer. The resulting expiration equals the renewal invoice's latest line-period end plus the application's 72-hour grace period. The earlier initial-checkout expiration was 2026-11-27 04:23:39+00. **04:29:34.621Z:** live CSV export remained **HTTP 200**, `Content-Type: text/csv`.

Current review-fix checklist (supersedes earlier remaining-check lists):

- [x] Corrected pushed SHA, existing Render deploy ID, live status and completion timestamp recorded; already auto-deployed, so no redundant deploy was needed.
- [x] Direct live UI signup → reload → logout → login → reload verified for the same account.
- [x] Fresh application checkout for that account; both stored return URLs use the assigned live domain.
- [x] Completed Stripe sandbox checkout, corresponding application event logs, independently persisted premium activation and CSV 402 → 200.
- [x] Paid recurring-cycle test invoice, actual invoice event/application log and persisted `premium_until` before/after; CSV remains 200.
- [ ] Capture **Stripe endpoint-specific successful HTTP delivery receipts** for checkout and renewal at `we_1USUwxGRjKD6n9dmwTvgZ6iJ`. The browser requires company Google sign-in. The operator was asked to sign in in the existing tab; no API keys were requested. Configuration, account-wide counters, source-code response expectations and application processing logs are not substituted for those receipts.

**Completion remains pending; do not merge.** README is the only repository change from this run. The existing implementation passed the live functional checks, so no speculative application/configuration fix was added. Signature validation and existing platform resources were preserved. The daemon owns committing this evidence and all version-control operations. No Monkey rerun was requested.


### Frontend UI verification re-confirmed — 2026-10-24 UTC

- **Deployed head confirmed live:** `https://gameroom.hdynamic.app/` returned HTTP 200 with the React HTML document, `<div id="root">`, CSS chunk `/static/css/main.5f202340.chunk.css`, and JS bundles `/static/js/1.1e2cf13d.chunk.js` and `/static/js/main.faf29026.chunk.js`.
- **Account component confirmed deployed:** the live main JS bundle contains `gamescrobble.token`, `auth/me`, `Signed in as`, `Log out`, and `Create your account` — the key strings from `src/Account.js`. The controls are present in the deployed artifact, not only locally.
- **Test suites passed:** React suite 2 suites / 3 tests (including signup → restore → logout → login full flow and expired-session/error handling); Go suite passed (cached). No source changes were required.
- **Prior live UI evidence stands:** account `e8236f3c89e08be2538e8c364220c69f` was created, reload preserved, logout and login re-verified at 04:22–04:23 UTC today against the same deployed head (`dep-dbe2one1egvs73fe916g`, commit `8a5dc89`). No duplicate UI workflow was performed.
- **Remaining verification checklist item updated:** "Verify frontend rendering and UI signup → logout → login → persisted session/account state" marked [x]. The Stripe endpoint delivery receipt item remains open.

## Community launch retention (review corrections)

Community drafts are in `community/`. Check current community rules and obtain any
required moderator permission before posting. The posting text uses campaign links
on `https://gameroom.hdynamic.app`. Implemented tracking polls Steam's recently played
games about every 15 minutes and records increases in reported playtime; it is not
exact session recording or historical daily reconstruction. Xbox/PSN/Epic linking
code does not establish unified playtime tracking. Free public profiles expose top
games and a daily heatmap; Premium gates CSV export and additional game statistics.
The drafts omit a price claim and explicitly describe Stripe test mode, not a paid
launch. Claims were checked against `pollUserScrobbles`, `startScrobblePoller`,
`getPublicProfile`, `getDeepStats`, and `exportHistory`.

`GET /api/admin/retention` requires `Authorization: Bearer <ADMIN_KEY>`; never put
credentials in URLs. This endpoint accepts no query parameters and returns
`Cache-Control: no-store` on success and failure. Missing/wrong credentials return
401. Database query, scan, and iteration failures return 500 with no partial
metrics; server logs identify the failed operation without database error contents.

Retention is exact-day, per unique authenticated user: D1/D7/D30 means a return on
signup UTC date plus 1/7/30 calendar days. A target date must be strictly earlier
than today's UTC date to be eligible, so incomplete days are excluded. Successful
logins and authenticated API requests (including `/auth/me` session restoration)
write one `user_return_activity` row per user and UTC date. Anonymous profile views
and background Steam polling are excluded. These writes are best-effort and log
failures without failing authentication; metrics can undercount during write outages.

The migration records the first tracking start timestamp without resetting it on
subsequent runs. Pre-tracking signups are conservatively reported as historical
unavailable once mature, never as failures to return. No login timestamps or Steam
playtime are backfilled as return visits. Each overall and source D1/D7/D30 row
reports `eligible_users`, `returning_users`, `immature_users`,
`historical_unavailable_users`, and `rate` (a fraction, not percent). Rate is null
when there are no eligible users; zero means eligible users exist but none returned.
Overall rows have `scope: overall` and an empty source; source rows have `scope: source`.
Rows aggregate all eligible signup dates since tracking started, not rolling windows.

Attribution is first application entry per browser tab, saved in sessionStorage
before rendering and retained across navigation/reloads to registration. Missing or
empty `utm_source` means `direct`; unsupported campaign values or unavailable storage
mean `unknown`. Supported values are `reddit_patientgamers`, `backloggd`, `direct`,
and `unknown`. Registration rejects all other values with 400, including oversized,
non-ASCII, control-character, and malformed strings; nothing is byte-truncated.
Omitted API source means direct; legacy blank/unsupported database sources are
reported as unknown because their original attribution was not captured.

Run local QA with a disposable PostgreSQL database:

```sh
RETENTION_TEST_DATABASE_URL='<local disposable PostgreSQL URL>' go test ./... -count=1
CI=true npm test -- --watch=false --runInBand
npm run build
```

The PostgreSQL test creates/removes an isolated schema, runs migrations twice on
empty and preexisting users schemas, checks preservation of the tracking timestamp
and existing users, and tests fixed UTC maturity boundaries for all three horizons,
source breakdowns, historical unavailability, rates, and repeated activity.
The live Stripe E2E test remains opt-in and requires its documented browser checkout
step; local tests are not evidence of a new deployed Stripe checkout.
