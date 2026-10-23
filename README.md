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

## 💰 Path to Revenue
"Last.fm for games" — unified play-history tracking across Steam/Xbox/PSN/Epic — monetizes via a Stripe premium tier (deeper stats, CSV history export) and affiliate game-deal links. The product loop is built: Go API with Steam Web API scrobble polling, JWT accounts, Xbox/PSN/Epic OAuth linking, public profile pages ("year in games" heatmap), Stripe checkout + webhook, and a React frontend. The remaining gap is plumbing: persistence still targets MongoDB (not on the shared platform — the Render service has no `MONGODB_URI`, so the deployed API currently has no database), and platform API credentials need operator sign-ups.

### Release TODOs
- [x] Finish the Steam integration end-to-end using official Web API keys (drop the ChromeDP password-login approach — it is a security and ToS liability)
- [x] Implement user accounts + OAuth linking for Xbox, PSN, and Epic (currently just route placeholders)
- [x] Replace committed node_modules and stray frontend artifacts with a clean repo layout (.gitignore)
- [x] Build a public profile page ("year in games", playtime heatmap) — the shareable artifact that drives signups
- [x] Deploy config (render.yaml) + scheduled scrobble polling
- [x] Add a premium tier (deeper stats, history export) via Stripe and affiliate links to game deals
- [x] Migrate persistence from MongoDB to Neon Postgres via `DATABASE_URL` (shared platform; `MONGODB_URI` is not provisioned, so nothing works in the deployed environment until this ships) — update render.yaml accordingly
- [ ] Run a Stripe test-mode E2E smoke test: signup → checkout → webhook fires → premium unlock → CSV export gated correctly (shared Stripe product/price/webhook already provisioned). Deployment is authorized; actual Stripe E2E verification remains pending. Mark complete only when actual verification evidence is recorded.
- [ ] Set platform credentials: `STEAM_API_KEY`, plus Xbox (`XBOX_CLIENT_ID`/`SECRET`), PSN, and Epic OAuth app registrations (operator sign-ups; Steam linking alone is enough to launch)
- [ ] Verify the Render deploy end-to-end and point the Stripe webhook at the live URL (`https://gameroom.hdynamic.app`). Deployment is authorized; frontend, UI authentication persistence, and Stripe delivery/application processing must all have actual verification evidence before completion. See the verification record below.
- [ ] Launch to gaming communities (r/patientgamers, Backloggd users) and validate retention (waits on the deployment switch)

The local `TestStripeSmoke` test provides in-process webhook/gating coverage using signed synthetic events, in-memory premium state, and deterministic export data. It verifies premium activation, invoice expiration extension, successful CSV export, and 402 gating before activation and after cancellation. It does not exercise signup, checkout creation, or delivery by Stripe.

### Deployment verification record (UTC)

The earlier completion claim is withdrawn. Monkey reproduced `GET /` → HTTP 503 (`Frontend not built. Run: npm run build`). The earlier API 200 and registration/JWT results do not establish a working frontend or UI signup/logout/login persistence. An unsigned webhook HTTP 400 only demonstrates rejection of that request; it does not establish Stripe-originated delivery or application processing.

- 2026-10-23T03:21:24.876Z: Render API inspection of existing service `srv-dban3t0m7kps739go4jg` found branch `master`, empty root directory, build command `go build -o server .`, and start command `./server`. This omitted the frontend build. The repository's `render.yaml` already correctly builds React before Go, so no YAML or server path change was needed: React emits `build/index.html`, and `serveFrontend` reads that path relative to the service root.
- 2026-10-23T03:21:25.053Z — **configuration evidence only**: Stripe API retrieved existing webhook `we_1USUwxGRjKD6n9dmwTvgZ6iJ`: URL `https://gameroom.hdynamic.app/api/stripe/webhook`, `livemode=false`, `status=enabled`, `api_version=null`. Enabled events: `checkout.session.completed`, `customer.subscription.updated`, `customer.subscription.deleted`, `invoice.payment_failed`. `invoice.payment_succeeded`, which the application uses to extend premium entitlement, is absent. The company provisioning CLI returned this same existing endpoint at 03:21:25.331Z; no account or endpoint was created and no signing secret was rotated. The CLI's event list is not delivery evidence.
- 2026-10-23T03:21:43.471Z: Updated that existing Render service to PR branch `throne/mvkcmcf7-verify-the-render-deploy-end-to-end-and-`, with the repository's build command `npm install && npm run build && go build -o server .`, start command `./server`, and empty root directory. Triggered deploy `dep-dbdd5hrncjis739pio80` at 03:21:43.995Z for commit `f796892308a36223c3fa7ea7616da4a585175246`. Version control remains with the daemon.
- 2026-10-23T03:21:57Z: Local `npm run build` succeeded, `test -s build/index.html` passed, and `go test ./...` passed. Local checks are not live deployment evidence.
- 2026-10-23T03:22:11.722Z, while the corrected deployment was building: independently reproduced `GET /` → HTTP 503 with the same missing-frontend message; `GET /api` → HTTP 200 at 03:22:11.784Z.
- UI authentication verification remains pending: `src/App.js` renders the landing page or public profile, with no signup, logout, or login UI. API-only authentication checks cannot satisfy this requirement. The browser attempt also returned `net::ERR_BLOCKED_BY_CLIENT`; no UI workflow was completed.
- 2026-10-23T03:23:25.788442Z: Render reports deploy `dep-dbdd5hrncjis739pio80` finished **live**, confirmed via the deployment API at 03:23:33.199Z. At 03:23:33.287Z, `GET /` returned HTTP 200 with the React HTML document. At 03:24:05.690Z–03:24:05.859Z, its referenced CSS and both external JavaScript bundles returned HTTP 200 with the appropriate content types. This verifies frontend artifact serving; browser rendering remains unverified because Chrome blocked the page. The served main bundle also contains no signup/login/logout controls.
- 2026-10-23T03:23:45.258Z: Baseline `GET /u/export?user_id=pr65-webhook-20261023T0323Z` returned HTTP 402. This isolated test metadata ID is not a UI-created account.
- 2026-10-23T03:23:47.852Z — **Stripe-originated event, not successful-delivery evidence**: the installed Stripe CLI, using the company's existing test-mode access, completed `stripe trigger checkout.session.completed --add checkout_session:metadata.user_id=pr65-webhook-20261023T0323Z`. Stripe API inspection at 03:24:05.471Z confirmed test event `evt_1UTZ56GRjKD6n9dme36T55rf`, `livemode=false`, API version `2026-09-30.endive`, and `pending_webhooks=16` (account-wide, not a receipt for this endpoint). Stripe accepted an explicit resend to `we_1USUwxGRjKD6n9dmwTvgZ6iJ` at 03:24:37.853Z. Neither CLI success nor resend acceptance proves an HTTP 2xx delivery. This fixture tests the webhook path, not the application's checkout creation or a UI purchase.
- 2026-10-23T03:24:53.077Z — **application processing not verified**: the same export request still returned HTTP 402, rather than the expected HTTP 200 CSV after premium activation. Render's request-log query for `/api/stripe/webhook` returned no entries at 03:24:52.987Z, so no endpoint-specific successful delivery result was obtained. The event's API version differs from the application's Stripe Go v76 schema and the endpoint has no version pin; investigate delivery responses/version compatibility and the missing renewal event subscription before claiming Stripe E2E success. No webhook validation was bypassed.

Remaining verification:
- [x] Correct the existing Render service build settings and deploy the PR branch.
- [x] Verify live frontend HTML, JavaScript, and CSS are served successfully over HTTPS.
- [x] Verify the existing Stripe webhook URL and test-mode configuration using company platform access.
- [ ] Verify frontend rendering and UI signup → logout → login → persisted session/account state (authentication UI is absent).
- [ ] Obtain an endpoint-specific successful Stripe delivery receipt and verify premium activation/CSV access for the corresponding event; verify renewal handling as well.

No secrets, tokens, or test payment credentials are recorded here. No new provider accounts or hosting services were created.
