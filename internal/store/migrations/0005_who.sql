-- 0.9.0: who a visitor is, for sites that switch on "Know who visits"
-- (options.identify). Empty on every other site and on older visits.
ALTER TABLE visits ADD COLUMN ip        TEXT NOT NULL DEFAULT '';  -- the full IP address
ALTER TABLE visits ADD COLUMN network   TEXT NOT NULL DEFAULT '';  -- who owns the address: an internet provider, a company, a data centre
ALTER TABLE visits ADD COLUMN asn       INTEGER NOT NULL DEFAULT 0;
ALTER TABLE visits ADD COLUMN postal    TEXT NOT NULL DEFAULT '';
ALTER TABLE visits ADD COLUMN lat       REAL;
ALTER TABLE visits ADD COLUMN lon       REAL;
ALTER TABLE visits ADD COLUMN client_id TEXT NOT NULL DEFAULT '';  -- a random ID the browser keeps (localStorage): the same person on another day
ALTER TABLE visits ADD COLUMN agent     TEXT NOT NULL DEFAULT '';  -- the whole User-Agent
ALTER TABLE visits ADD COLUMN ref_url   TEXT NOT NULL DEFAULT '';  -- the whole referrer, with its query
ALTER TABLE visits ADD COLUMN details   TEXT NOT NULL DEFAULT '';  -- JSON: screen, time zone, languages, hardware, connection…
CREATE INDEX visits_client ON visits(site_id, client_id) WHERE client_id <> '';
CREATE INDEX visits_ip ON visits(site_id, ip) WHERE ip <> '';
