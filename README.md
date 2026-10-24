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
- [x] Complete Stripe test-mode E2E verification of signup → application Checkout → webhook → premium CSV export → cancellation gating. Monkey successfully verified deployed head `7606188` on 2026-10-24 at `https://gameroom.hdynamic.app`: signup, application Checkout session `cs_test_a19vxivBRuRlzuLEPlGhrcdzDygKIxgEwKbqBRFLerwGxMqyWY1NSl61j3`, webhook delivery, premium unlock with CSV export returning HTTP 200 and correct headers (`Content-Type: text/csv` and `Content-Disposition: attachment; filename="gamescrobble_history.csv"`), cancellation of subscription `sub_1UTxCgGRjKD6n9dmD6IDQi4R`, and restored HTTP 402 export gating. This records Monkey's verification of the deployed application flow; it is not evidence that the revised `TestStripeE2E` ran. Execution of that Go test remains unverified.
- [ ] Set platform credentials: `STEAM_API_KEY`, plus Xbox (`XBOX_CLIENT_ID`/`SECRET`), PSN, and Epic OAuth app registrations (operator sign-ups; Steam linking alone is enough to launch)
- [x] Verify the Render deploy end-to-end and point the Stripe webhook at the live URL (`https://gameroom.hdynamic.app`). Monkey's successful verification of head `7606188`, recorded above, confirms the deployed Stripe flow and webhook delivery.
- [ ] Launch to gaming communities (r/patientgamers, Backloggd users) and validate retention (waits on the deployment switch)

The local `TestStripeSmoke` test provides in-process webhook/gating coverage using signed synthetic events, in-memory premium state, and deterministic export data. It verifies premium activation, invoice expiration extension, successful CSV export, and 402 gating before activation and after cancellation. It does not exercise hosted Checkout or Stripe delivery.

The revised `TestStripeE2E` (`stripe_e2e_test.go`) registers a new application user, checks the initial 402 gate, and calls `/api/stripe/checkout`. It retrieves that session from Stripe and requires test mode, the registered user's metadata, the provisioned price `price_1USUwuGRjKD6n9dmujp5uGyk`, and return URLs on `gameroom.hdynamic.app`.

To run it, provide the platform's existing test-mode `STRIPE_SECRET_KEY` securely through the environment (do not place credentials in command arguments or logs), then run:

```sh
RUN_STRIPE_E2E=1 go test -run '^TestStripeE2E$' -v -count=1 -timeout 15m
```

**A separate browser step is required.** While the command runs, open the hosted Checkout URL from the private temporary file named in the test output and complete test-mode Checkout within ten minutes. The Go command alone does not complete E2E. The URL file is removed when the test ends. No Stripe CLI, synthetic checkout trigger, separate customer, or trial subscription is used.

After actual paid Checkout completion, the test requires a matching `checkout.session.completed` event with no pending webhook deliveries before checking premium CSV access. It verifies the provisioned application webhook is enabled and subscribed; Stripe's pending-delivery count is account-wide, not a per-endpoint delivery receipt, so the test also requires the application's resulting gating transitions. It then cancels only the subscription belonging to that Checkout, waits for its `customer.subscription.deleted` event to have no pending deliveries, and requires restored 402 gating. The revised test asserts exact CSV response headers and the CSV column header. Monkey's verification above independently confirms HTTP 200 and correct CSV response headers; it does not establish execution of these Go assertions.

Once opted in, missing prerequisites, request/response errors, and timeouts fail the test and stop dependent stages. HTTP requests and the overall run are bounded; there are no subprocesses or credential-bearing diagnostic dumps. If a run fails after payment but before cancellation, use the logged subscription ID to cancel that test subscription. Normal `go test ./...` skips this interactive live test; passing local tests does not count as executing it.
