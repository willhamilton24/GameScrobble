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
"Last.fm for games" — unified play-history tracking across Steam/Xbox/PSN/Epic — can monetize via premium stats profiles and affiliate game-deal links, but only Steam is even stubbed and the whole product (API, auth, frontend) remains to be built.

### Release TODOs
- [x] Finish the Steam integration end-to-end using official Web API keys (drop the ChromeDP password-login approach — it is a security and ToS liability)
- [ ] Implement user accounts + OAuth linking for Xbox, PSN, and Epic (currently just route placeholders)
- [ ] Replace committed node_modules and stray frontend artifacts with a clean repo layout (.gitignore)
- [ ] Build a public profile page ("year in games", playtime heatmap) — the shareable artifact that drives signups
- [x] Deploy API (Fly.io/Render) + Mongo Atlas with scheduled scrobble polling
- [x] Add a premium tier (deeper stats, history export) via Stripe and affiliate links to game deals
- [ ] Launch to gaming communities (r/patientgamers, Backloggd users) and validate retention
