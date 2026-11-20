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
| `POST /api/robot` | A robot's page request, from the site's own server: `{d: domain, u: page URL, ua: the robot's User-Agent}` with the site's proxy key in `X-Pocket-Proxy-Key`. Counted only when the User-Agent is a robot's; answers `202` |

Hit fields (Plausible's names): `n` name (`pageview`, `engagement` or an event), `u` page URL, `d` the site's domain
(comma-separated for several sites), `r` referrer, `w` window width, `p` properties (object, or a JSON string),
`h` 1 = pages by `#`; for `engagement` also `e` milliseconds in view since the last report, `sd` scroll depth 0–100 and
`v` how fast the page was (`t` server answered, `f` something showed, `l` main content showed, `i` slowest press — all
milliseconds — and `c` layout shift); for an event also `$` its money, `{amount, currency}`.

What the script sends by itself: `Outbound Link: Click` and `File Download` (`url`), `Contact Link: Click` (`kind`
email or phone, `to`), `Form: Submission` (`form`), `404` (`from`: the page with the link), `Click` (`text`, when on).
A page view whose address holds search words (`?q=…`, the site's search names) also counts a `Search` event (`term`).

Script attributes: `data-domain`, `data-api` (where to send), `data-hash`, `data-clicks` (every link and button),
`data-no-outbound`, `data-no-downloads`, `data-no-forms`, `data-no-404`, `data-no-speed`, `data-file-types="pdf,zip"`.
Events: `window.pocket('Name', { props: {…}, revenue: { amount: 24.5, currency: 'EUR' } })`
(also `window.plausible`), or `data-pocket-event="Name"` + `data-pocket-prop-<key>="value"` on an element, or
Plausible's `class="plausible-event-name=Name"`. `?pocket_ignore=1` on any page stops counting that browser.
On the page: `data-pocket-404` on any element marks a not-found page that answers 200; on a form,
`data-pocket-form="Name"` names it and `data-pocket-ignore` leaves it out.

## Pocket Core app links

| Scope | Covers |
|---|---|
| `analytics:read` | `GET /summary`, `/sites`, `/sites/{id}`, `/sites/{id}/stats`, `/breakdown`, `/conversions`, `/live`, `/events`, `/speed`, `/when`, `/paths`, `/notes`, `/keywords`, `/robots`, `/sites/{id}/funnels/{fid}/counts`, `/day/{date}`, `/search` |

Visits one by one (`/sites/{id}/visits…`) are never open to a link. `GET /auth/me` answers for any link; everything else answers 403 to a link token.

Events on the bus (`events:read`): `analytics.spike` (`site`, `site_id`, `visitors_hour`, `usual_hour`, `text`), `analytics.quiet` (`site`, `site_id`, `last_hit`, `text`), `analytics.weekly` (`visitors`, `sites`, `text`); each with `user_email`.

## Ranges and filters

Every stats call takes `range` = `today`, `yesterday`, `7d`, `30d` (default), `month`, `lastmonth`, `12mo`, `year`, `all` or `custom` with `from`/`to` (`YYYY-MM-DD`, in the site's time zone), an optional `interval` (`hour`, `day`, `week`, `month`) and any number of filters `f=<dim>:<value>` (`f=!<dim>:<value>` = is not). Filter dims: `source`, `channel`, `referrer`, `utm_source`, `utm_medium`, `utm_campaign`, `utm_content`, `utm_term`, `country` (ISO code), `region`, `city`, `device`, `browser`, `os`, `language`, `hostname`, `entry`, `exit`, `page` (visits that saw it; `*` = anything), `event`, `goal` (goal id).

## Google Search Console

`PUT /settings {"google_key": "<the service account JSON file>"}` saves a person's key (`""` removes it); settings answer only `google_configured` and `google_email`, never the key. `POST /google/test {key?}` tries the typed key (else the saved one): `{ok, email, properties[], sites[{site, domain, property}]}`. A site's property is found by its domain: `sc-domain:example.com`, else `https://example.com/` (and `www.`, `http`).

## Sites

| | |
|---|---|
| `GET /sites` | Your sites with `today`, `live`, `visitors_30d`, `visitors_prev_30d`, `spark` (30 days), the `snippet` to paste |
| `POST /sites` | `{domain, name?, tz?}` — 409 when the domain is counted on this box already |
| `GET /sites/{id}` | One site with `goals` and `funnels` |
| `PUT /sites/{id}` | `{name, domain, tz, also[], options{outbound, downloads, clicks, forms, not_found, search, search_params[], speed, hash, localhost, exclude[], keep_days, spike, quiet, weekly}}` (any of them; only the options sent change) |
| `DELETE /sites/{id}` · `POST /sites/{id}/reset` | Delete the site · forget its visits |
| `POST /sites/{id}/proxy-key` | A new proxy key (for a Worker in front) |
| `GET /sites/{id}/check` | `{installed, last_hit}` |
| `POST /sites/{id}/goals` · `PUT/DELETE /goals/{gid}` | `{name, kind: page|event, match}` |
| `POST /sites/{id}/funnels` · `PUT/DELETE /funnels/{fid}` | `{name, steps: [{kind, match, label?}]}` (2–8 steps) |
| `GET/POST /sites/{id}/notes` · `PUT/DELETE /notes/{nid}` | What happened on a day, for the chart: `{day: YYYY-MM-DD, text}`; `GET` takes `from`/`to` |

## Numbers

| | |
|---|---|
| `GET /sites/{id}/stats` | `{totals, previous, series[], previous_series[], notes[], robots (any robot ever seen), range, live}`; totals: `visitors`, `visits`, `pageviews`, `views_per_visit`, `bounce_rate`, `duration` (s), `events` |
| `GET /sites/{id}/breakdown?dim=…&limit=10&offset=0` | A top list; `dim` = a filter dim or `page`, `event`, `outbound`, `download`, `click`, `contact`, `form`, `search`, `notfound` (missing pages), `prop` (with `event` and `key`; `path` = only on that page). Rows: `value`, `visitors`, `visits`, `pageviews`, `events`, `bounce_rate`, `duration`, pages also `time_on_page` and `scroll` |
| `GET /sites/{id}/conversions` | Each goal: `visitors`, `conversions`, `rate`, `revenue[]` (`{currency, amount}`, when its events carried money) |
| `GET /sites/{id}/speed?dim=page&limit=10` | How fast pages were: `all` and `rows[]` by `page`, `device`, `browser`, `os` or `country`, each with `loads` and `lcp`, `inp`, `cls`, `ttfb`, `fcp` as `{p75, n}` (three of four loads stayed under `p75`; ms, `cls` a number) |
| `GET /sites/{id}/robots?by=name&kind=ai,search&name=…&limit=10` | The robots that read the site (never visitors): `by` = `name` (who), `path` (what they read) or `kind`; kinds `ai`, `search`, `preview` (a shared link), `seo`, `monitor`, `other`. Rows: `value`, `kind`, `hits` (pages read), `pages`, `last`; plus `total` and `seen`. Only the range applies, not the filters |
| `GET /sites/{id}/keywords?by=query&limit=10` | The words people searched on Google (Search Console), or with `by=page` the pages Google sent them to: `state` (`ok`, `off` = no key, `no_property` = the key was not added to this site, `error`), `rows[]` of `{words, clicks, impressions, ctr, position}`, `totals`, `from`/`to`, `partial` (only `page` and `device` filters reach Google). Kept 30 minutes |
| `GET /sites/{id}/when` | Visits by weekday and hour in the site's zone: `cells[7][24]` (0 = Sunday), `max`, `visits`, `week_start` |
| `GET /sites/{id}/paths?page=/menu` | Where people were before a page and went next: `views`, `before[]`, `after[]` of `{value, times}` (`""` = came in here / left) |
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
