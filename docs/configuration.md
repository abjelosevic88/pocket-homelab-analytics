# Configuration

Environment variables; every one has a default. Personal settings (time zone, report, notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings. Everything about a website (what is counted, goals, funnels, alerts, how long visits are kept) is set in the app.

| Variable | Default | What it does |
|---|---|---|
| `BASE_URL` | – | Where the app is (links in messages, the OIDC callback, the script's address when there is no `PUBLIC_URL`) |
| `PUBLIC_URL` | – | A second hostname that serves **only** `/js/script.js`, `/api/event` and the icon. Websites use it; the app itself can stay private |
| `GEO_DB` | `city` | Places from IP addresses: `city`, `country` (smaller) or `off`. DB-IP Lite (CC BY 4.0) is downloaded into `DATA_DIR/geo` and refreshed monthly; lookups happen on the box |
| `GEO_DB_PATH` | – | Use your own MaxMind-format file instead (e.g. `GeoLite2-City.mmdb`); nothing is downloaded |
| `HOST` / `PORT` | `0.0.0.0` / `8080` | Where it listens |
| `DATA_DIR` | `./data` | The database, the place database and the Pocket Core token |
| `DB_PATH` | `$DATA_DIR/pocket-analytics.db` | The SQLite file |
| `AUTH_MODE` | `local` | `local` (passwords), `proxy` (a header from your reverse proxy), `none` (one user, for trying it out) |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | – | The first account, made on an empty database |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL` / `NOTIFY_FORMAT` | – / `apprise` | Messages: an Apprise URL, an ntfy topic or a JSON webhook |
| `NOTIFY_HOUR` | `8` | Default hour of the Monday report |
| `OPENAI_API_KEY` / `OPENAI_MODEL` / `OPENAI_BASE_URL` | – | AI without Pocket Core |
| `CORE_URL` / `CORE_REGISTRATION_KEY` / `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY` | `true` | Take the client address from `X-Forwarded-For` / `X-Real-IP` |
| `METRICS_ENABLED` | `true` | Prometheus `/metrics` (hits received by result, stored, queued) |
| `DEMO_DATA` | `false` | Seed two websites with four months of visits into the owner account |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | |

## Privacy

No cookies and no local storage (except an opt-out flag a person sets themselves with `?pocket_ignore=1`). A visitor is a hash of the site, the IP address and the browser with a salt that changes every UTC day; salts older than yesterday are deleted, so hashes cannot be linked across days. The IP address is used for the hash and the place lookup and is never stored. Robots (by User-Agent and headless browsers) are not counted.
