-- 0.2.0: how fast a page was, money on events, notes on the chart.

-- How fast the page was for this visitor (the first page of a load only);
-- NULL = not measured. Milliseconds, except cls (how much the layout moved).
ALTER TABLE hits ADD COLUMN ttfb INTEGER;
ALTER TABLE hits ADD COLUMN fcp  INTEGER;
ALTER TABLE hits ADD COLUMN lcp  INTEGER;
ALTER TABLE hits ADD COLUMN inp  INTEGER;
ALTER TABLE hits ADD COLUMN cls  REAL;

-- Money an event brought (pocket('Order placed', { revenue: { amount, currency } })).
ALTER TABLE hits ADD COLUMN revenue  REAL NOT NULL DEFAULT 0;
ALTER TABLE hits ADD COLUMN currency TEXT NOT NULL DEFAULT '';

-- What happened on a day ("newsletter went out", "new design"): shown on the chart.
CREATE TABLE notes (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  day        TEXT NOT NULL, -- YYYY-MM-DD in the site's time zone
  text       TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX notes_site ON notes(site_id, day);
