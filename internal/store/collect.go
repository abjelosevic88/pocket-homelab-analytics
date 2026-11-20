package store

import (
	"context"
	"database/sql"
	"strconv"
)

// VisitGap is how long a visit waits for its next hit: 30 minutes.
const VisitGap = 30 * 60

// Incoming is one hit from the tracking script, already checked and
// enriched (site, visitor hash, place, browser).
type Incoming struct {
	SiteID      int64
	Visitor     string // today's hash
	PrevVisitor string // yesterday's (a visit over midnight UTC goes on)
	TS          int64
	Name        string // pageview | engagement | an event
	Path        string
	Props       string // JSON object or ""
	Engaged     int64  // engagement: milliseconds since the last report
	Scroll      int    // engagement: how far down, 0–100
	Visit       Visit  // what a new visit starts with
}

type openVisit struct {
	id   int64
	last int64
}

// VisitCache remembers the visits still open, so most hits need no lookup.
// Only the collector's writer uses it (one goroutine).
type VisitCache map[string]openVisit

// Sweep forgets visits that have ended.
func (c VisitCache) Sweep(now int64) {
	for k, v := range c {
		if now-v.last > VisitGap {
			delete(c, k)
		}
	}
}

func cacheKey(site int64, visitor string) string { return strconv.FormatInt(site, 10) + ":" + visitor }

// findVisit is the visitor's open visit, 0 when there is none.
func findVisit(ctx context.Context, tx *sql.Tx, cache VisitCache, in *Incoming) (int64, error) {
	for _, v := range []string{in.Visitor, in.PrevVisitor} {
		if v == "" {
			continue
		}
		if ov, ok := cache[cacheKey(in.SiteID, v)]; ok && in.TS-ov.last <= VisitGap {
			return ov.id, nil
		}
	}
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM visits WHERE site_id = ? AND visitor IN (?, ?) AND last >= ? ORDER BY last DESC LIMIT 1`,
		in.SiteID, in.Visitor, in.PrevVisitor, in.TS-VisitGap).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}

// SaveHits stores a batch of hits in one transaction: each joins its
// visitor's open visit or starts a new one.
func (s *Store) SaveHits(ctx context.Context, batch []Incoming, cache VisitCache) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	saved := 0
	seen := map[int64][2]int64{} // site → first, last
	for i := range batch {
		in := &batch[i]
		id, err := findVisit(ctx, tx, cache, in)
		if err != nil {
			return 0, err
		}
		if in.Name == "engagement" {
			if id == 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE hits SET engaged = engaged + ?, scroll = MAX(scroll, ?) WHERE id = (SELECT id FROM hits WHERE visit_id = ? AND name = 'pageview' AND path = ? ORDER BY ts DESC LIMIT 1)`,
				in.Engaged, in.Scroll, id, in.Path); err != nil {
				return 0, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE visits SET engaged = engaged + ?, last = MAX(last, ?) WHERE id = ?`, in.Engaged, in.TS, id); err != nil {
				return 0, err
			}
			cache[cacheKey(in.SiteID, in.Visitor)] = openVisit{id, in.TS}
			continue
		}
		if id == 0 {
			v := in.Visit
			res, err := tx.ExecContext(ctx, `INSERT INTO visits (site_id, visitor, started, last, hostname, entry, exit, referrer, source, channel,
				utm_source, utm_medium, utm_campaign, utm_content, utm_term, country, region, city, device, browser, browser_version, os, os_version, language)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				in.SiteID, in.Visitor, in.TS, in.TS, v.Hostname, in.Path, in.Path, v.Referrer, v.Source, v.Channel,
				v.UTMSource, v.UTMMedium, v.UTMCampaign, v.UTMContent, v.UTMTerm, v.Country, v.Region, v.City, v.Device, v.Browser, v.BrowserVersion, v.OS, v.OSVersion, v.Language)
			if err != nil {
				return 0, err
			}
			id, _ = res.LastInsertId()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hits (site_id, visit_id, ts, name, path, props) VALUES (?, ?, ?, ?, ?, ?)`, in.SiteID, id, in.TS, in.Name, in.Path, in.Props); err != nil {
			return 0, err
		}
		q := `UPDATE visits SET events = events + 1, last = MAX(last, ?) WHERE id = ?`
		args := []any{in.TS, id}
		if in.Name == "pageview" {
			q = `UPDATE visits SET pageviews = pageviews + 1, exit = ?, last = MAX(last, ?) WHERE id = ?`
			args = []any{in.Path, in.TS, id}
		}
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return 0, err
		}
		cache[cacheKey(in.SiteID, in.Visitor)] = openVisit{id, in.TS}
		fl, ok := seen[in.SiteID]
		if !ok || in.TS < fl[0] {
			fl[0] = in.TS
		}
		if in.TS > fl[1] {
			fl[1] = in.TS
		}
		seen[in.SiteID] = fl
		saved++
	}
	for site, fl := range seen {
		if _, err := tx.ExecContext(ctx, `UPDATE sites SET last_hit = MAX(last_hit, ?), first_hit = CASE WHEN first_hit = 0 OR first_hit > ? THEN ? ELSE first_hit END WHERE id = ?`, fl[1], fl[0], fl[0], site); err != nil {
			return 0, err
		}
	}
	return saved, tx.Commit()
}
