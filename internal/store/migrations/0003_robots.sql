-- 0.4.0: the robots that read a site (search engines, AI assistants, link
-- previews, tools). They are never visitors and never in the other numbers;
-- only how often each read which page, by hour. ts = the hour's start, Unix
-- seconds. No address, nothing else is kept.
CREATE TABLE robots (
  site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  ts      INTEGER NOT NULL,
  name    TEXT NOT NULL,              -- Google, ChatGPT (answering someone), Slack…
  kind    TEXT NOT NULL DEFAULT '',   -- ai | search | preview | seo | monitor | other
  path    TEXT NOT NULL DEFAULT '',
  hits    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (site_id, ts, name, path)
) WITHOUT ROWID;
CREATE INDEX robots_site_name ON robots(site_id, name, ts);
