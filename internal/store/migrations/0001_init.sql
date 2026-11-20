CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Analytics: everything below is per person ----------
--
-- A site belongs to one person. Visits and hits belong to a site (deleted
-- with it) and are only ever read through a site the person owns. Times on
-- visits and hits are Unix seconds (UTC): there can be millions of them.

-- A website being counted. domain is box-wide unique: the tracking script
-- sends it (data-domain) and hits are matched to the site by it.
CREATE TABLE sites (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  domain     TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL DEFAULT '',
  tz         TEXT NOT NULL DEFAULT '',
  also       TEXT NOT NULL DEFAULT '',   -- other host names that count, space separated
  options    TEXT NOT NULL DEFAULT '{}', -- JSON, store.SiteOptions
  proxy_key  TEXT NOT NULL DEFAULT '',   -- a proxy in front (a Cloudflare Worker) proves itself with it
  position   INTEGER NOT NULL DEFAULT 0,
  first_hit  INTEGER NOT NULL DEFAULT 0,
  last_hit   INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX sites_user ON sites(user_id, position, domain);

-- Something a visit can do that counts: reach a page (match = a path,
-- * for any part) or send an event (match = its name).
CREATE TABLE goals (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  kind       TEXT NOT NULL DEFAULT 'page', -- page | event
  match      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX goals_user ON goals(user_id, site_id);

-- Steps a visit should take in order (2–8), each a page or an event.
CREATE TABLE funnels (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  steps      TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX funnels_user ON funnels(user_id, site_id);

-- One visit: a visitor's hits with less than 30 minutes between them.
-- visitor is a hash of the site, IP address and browser with a salt that
-- changes every day and is then thrown away: no cookies, no IP stored, and
-- nobody can be followed from one day to the next.
CREATE TABLE visits (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  site_id         INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  visitor         TEXT NOT NULL,
  started         INTEGER NOT NULL,
  last            INTEGER NOT NULL,
  hostname        TEXT NOT NULL DEFAULT '',
  entry           TEXT NOT NULL DEFAULT '',
  exit            TEXT NOT NULL DEFAULT '',
  pageviews       INTEGER NOT NULL DEFAULT 0,
  events          INTEGER NOT NULL DEFAULT 0,
  engaged         INTEGER NOT NULL DEFAULT 0, -- milliseconds the page was in front
  referrer        TEXT NOT NULL DEFAULT '',   -- host + path, no query
  source          TEXT NOT NULL DEFAULT '',   -- Google, Hacker News, a newsletter (utm_source)…
  channel         TEXT NOT NULL DEFAULT '',   -- Direct, Search, Social, AI, Email, Paid, Referral
  utm_source      TEXT NOT NULL DEFAULT '',
  utm_medium      TEXT NOT NULL DEFAULT '',
  utm_campaign    TEXT NOT NULL DEFAULT '',
  utm_content     TEXT NOT NULL DEFAULT '',
  utm_term        TEXT NOT NULL DEFAULT '',
  country         TEXT NOT NULL DEFAULT '',   -- ISO 3166-1 alpha-2
  region          TEXT NOT NULL DEFAULT '',
  city            TEXT NOT NULL DEFAULT '',
  device          TEXT NOT NULL DEFAULT '',   -- Phone | Tablet | Laptop | Desktop
  browser         TEXT NOT NULL DEFAULT '',
  browser_version TEXT NOT NULL DEFAULT '',
  os              TEXT NOT NULL DEFAULT '',
  os_version      TEXT NOT NULL DEFAULT '',
  language        TEXT NOT NULL DEFAULT ''
);
CREATE INDEX visits_site_time ON visits(site_id, started);
CREATE INDEX visits_visitor ON visits(site_id, visitor, last);

-- What happened in a visit: a page seen (name 'pageview') or an event
-- (outbound link, download, a click, the site's own). props: JSON object.
CREATE TABLE hits (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  site_id  INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  visit_id INTEGER NOT NULL REFERENCES visits(id) ON DELETE CASCADE,
  ts       INTEGER NOT NULL,
  name     TEXT NOT NULL,
  path     TEXT NOT NULL DEFAULT '',
  props    TEXT NOT NULL DEFAULT '',
  engaged  INTEGER NOT NULL DEFAULT 0, -- milliseconds on this page
  scroll   INTEGER NOT NULL DEFAULT 0  -- how far down, 0–100
);
CREATE INDEX hits_site_time ON hits(site_id, ts);
CREATE INDEX hits_visit ON hits(visit_id, ts);
CREATE INDEX hits_site_name ON hits(site_id, name, ts);
