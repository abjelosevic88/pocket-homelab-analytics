# Pocket Analytics API

Base: `<BASE_URL>/api/v1`. Auth: session cookie `pan_session`, a personal API token
(`Authorization: Bearer pan_…` or `X-API-Key`), or a Pocket Core app-link token
(`pcx_…`, audience `analytics`). Errors are `{"error": "…"}`. Everything is per person:
another person's ids answer 404. Times in answers are Unix seconds.

## Collecting (public, no auth)

| | |
|---|---|
| `GET /js/script.js` | The tracking script (also `/js/pa.js`, `/js/plausible.js`) |
| `POST /api/event` | One hit, JSON (any content type; CORS open). Answers `202` whenever the body is well formed |

Hit fields (Plausible's names): `n` name (`pageview`, `engagement` or an event), `u` page URL, `d` the site's domain
(comma-separated for several sites), `r` referrer, `w` window width, `p` properties (object, or a JSON string),
`h` 1 = pages by `#`; for `engagement` also `e` milliseconds in view since the last report and `sd` scroll depth 0–100.

Script attributes: `data-domain`, `data-api` (where to send), `data-hash`, `data-clicks` (every link and button),
`data-no-outbound`, `data-no-downloads`, `data-file-types="pdf,zip"`. Events: `window.pocket('Name', { props: {…} })`
(also `window.plausible`), or `data-pocket-event="Name"` + `data-pocket-prop-<key>="value"` on an element, or
Plausible's `class="plausible-event-name=Name"`. `?pocket_ignore=1` on any page stops counting that browser.

## Pocket Core app links

| Scope | Covers |
|---|---|
| `analytics:read` | `GET /summary`, `/sites`, `/sites/{id}`, `/sites/{id}/stats`, `/breakdown`, `/conversions`, `/live`, `/events`, `/sites/{id}/funnels/{fid}/counts`, `/day/{date}`, `/search` |

Visits one by one (`/sites/{id}/visits…`) are never open to a link. `GET /auth/me` answers for any link; everything else answers 403 to a link token.

Events on the bus (`events:read`): `analytics.spike` (`site`, `site_id`, `visitors_hour`, `usual_hour`, `text`), `analytics.quiet` (`site`, `site_id`, `last_hit`, `text`), `analytics.weekly` (`visitors`, `sites`, `text`); each with `user_email`.

## Ranges and filters

Every stats call takes `range` = `today`, `yesterday`, `7d`, `30d` (default), `month`, `lastmonth`, `12mo`, `year`, `all` or `custom` with `from`/`to` (`YYYY-MM-DD`, in the site's time zone), an optional `interval` (`hour`, `day`, `week`, `month`) and any number of filters `f=<dim>:<value>` (`f=!<dim>:<value>` = is not). Filter dims: `source`, `channel`, `referrer`, `utm_source`, `utm_medium`, `utm_campaign`, `utm_content`, `utm_term`, `country` (ISO code), `region`, `city`, `device`, `browser`, `os`, `language`, `hostname`, `entry`, `exit`, `page` (visits that saw it; `*` = anything), `event`, `goal` (goal id).

## Sites

| | |
|---|---|
| `GET /sites` | Your sites with `today`, `live`, `visitors_30d`, `visitors_prev_30d`, `spark` (30 days), the `snippet` to paste |
| `POST /sites` | `{domain, name?, tz?}` — 409 when the domain is counted on this box already |
| `GET /sites/{id}` | One site with `goals` and `funnels` |
| `PUT /sites/{id}` | `{name, domain, tz, also[], options{outbound, downloads, clicks, hash, localhost, exclude[], keep_days, spike, quiet, weekly}}` (any of them) |
| `DELETE /sites/{id}` · `POST /sites/{id}/reset` | Delete the site · forget its visits |
| `POST /sites/{id}/proxy-key` | A new proxy key (for a Worker in front) |
| `GET /sites/{id}/check` | `{installed, last_hit}` |
| `POST /sites/{id}/goals` · `PUT/DELETE /goals/{gid}` | `{name, kind: page|event, match}` |
| `POST /sites/{id}/funnels` · `PUT/DELETE /funnels/{fid}` | `{name, steps: [{kind, match, label?}]}` (2–8 steps) |

## Numbers

| | |
|---|---|
| `GET /sites/{id}/stats` | `{totals, previous, series[], previous_series[], range, live}`; totals: `visitors`, `visits`, `pageviews`, `views_per_visit`, `bounce_rate`, `duration` (s), `events` |
| `GET /sites/{id}/breakdown?dim=…&limit=10&offset=0` | A top list; `dim` = a filter dim or `page`, `event`, `outbound`, `download`, `click`, `prop` (with `event` and `key`). Rows: `value`, `visitors`, `visits`, `pageviews`, `events`, `bounce_rate`, `duration`, pages also `time_on_page` and `scroll` |
| `GET /sites/{id}/conversions` | Each goal: `visitors`, `conversions`, `rate` |
| `GET /sites/{id}/funnels/{fid}/counts` | Each step: `visitors`, `dropped`, `rate`, `next` |
| `GET /sites/{id}/live` | `now` (5 min), `minutes` (30), `pages`, `sources`, `countries`, `recent` |
| `GET /sites/{id}/visits?limit=50&offset=` · `/visits/{vid}` | Visits one by one; one visit with its hits |
| `GET /sites/{id}/events[?event=]` | Event names seen lately (and one event's property keys) |
| `GET /sites/{id}/export.csv[?dim=]` | Every visit, or a top list, as CSV |
| `POST /sites/{id}/explain` | Pocket AI explains the range (`{text}`; `fresh=1` asks again) |
| `POST /sites/{id}/make` | `{target: note|task, title, text, due?}` into Pocket Notes / Pocket Tasks |

## For the rest of the box

| | |
|---|---|
| `GET /summary` | Each site's `today`, `yesterday`, `week`, `week_before`, `live`, `top_sources`, `top_pages`; `total` |
| `GET /day/{date}` | Visitors per site on a day (Pocket Day) |
| `GET /search?q=` | Sites and pages (last 30 days) |
| `GET /export.json` · `POST /import` | Sites with options, goals and funnels |
