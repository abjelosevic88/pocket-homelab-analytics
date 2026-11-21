-- 0.5.0: how people read a page and what the robots are.
-- hits.sections: the parts of a page (data-pocket-section) a visitor had on
-- screen, as a JSON list of names in page order (pageviews only).
ALTER TABLE hits ADD COLUMN sections TEXT NOT NULL DEFAULT '';
-- goals.robot: a "robot" goal counts pages read by robots (install.sh fetched
-- by curl = an install run); this narrows it to one robot by name ("" = any).
ALTER TABLE goals ADD COLUMN robot TEXT NOT NULL DEFAULT '';
-- The robots without a name: their User-Agent as they sent it (a robot's
-- name is not a person's), so they can be told apart. A few hundred per
-- site at most; the least recent go first.
CREATE TABLE robot_agents (
  site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
  agent   TEXT NOT NULL,
  hits    INTEGER NOT NULL DEFAULT 0,
  first   INTEGER NOT NULL,
  last    INTEGER NOT NULL,
  path    TEXT NOT NULL DEFAULT '',  -- the last page it read
  PRIMARY KEY (site_id, agent)
) WITHOUT ROWID;
