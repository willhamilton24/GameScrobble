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
- [x] Build a public profile page ("year in games", playtime heatmap) — the shareable artifact that drives signups
- [x] Deploy config (render.yaml) + scheduled scrobble polling
- [x] Add a premium tier (deeper stats, history export) via Stripe and affiliate links to game deals
- [ ] Migrate persistence from MongoDB to Neon Postgres via `DATABASE_URL` (shared platform; `MONGODB_URI` is not provisioned, so nothing works in the deployed environment until this ships) — update render.yaml accordingly
- [ ] Replace committed node_modules and stray frontend artifacts with a clean repo layout (.gitignore)
- [ ] Run a Stripe test-mode E2E smoke test: signup → checkout → webhook fires → premium unlock → CSV export gated correctly (shared Stripe product/price/webhook already provisioned)
- [ ] Set platform credentials: `STEAM_API_KEY`, plus Xbox (`XBOX_CLIENT_ID`/`SECRET`), PSN, and Epic OAuth app registrations (operator sign-ups; Steam linking alone is enough to launch)
- [ ] Verify the Render deploy end-to-end and point the Stripe webhook at the live URL (waits on the deployment switch)
- [ ] Launch to gaming communities (r/patientgamers, Backloggd users) and validate retention (waits on the deployment switch)
