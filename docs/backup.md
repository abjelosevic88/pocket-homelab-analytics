# Backup

Everything lives in one SQLite file, `DATA_DIR/pocket-analytics.db` (WAL mode). Take a consistent copy with `pocket-analytics backup` (writes `DATA_DIR/backups/pocket-analytics-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin). Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration; `DATA_DIR/geo/` is the downloaded place database (no need to back it up — it comes back by itself).

Restore: stop the app, replace `pocket-analytics.db` (remove the `-wal`/`-shm` files), start.

Every person can export their websites with goals and funnels as JSON (`/api/v1/export.json`, back in with Settings → Backup & data), and a site's visits or any top list as CSV (`/api/v1/sites/<id>/export.csv?range=all[&dim=page]`).
