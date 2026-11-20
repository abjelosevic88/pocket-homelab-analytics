package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- for the rest of the box: Pocket Day, Search, dashboards, Pocket AI ----

// handleSummary is each site's today, yesterday, last 7 days (against the
// 7 before) and the people on it now.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	sites, err := s.store.ListSites(ctx, uid)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	now := time.Now()
	out := []map[string]any{}
	total := map[string]int{}
	for _, site := range sites {
		today := dayStart(now.In(s.zoneOf(ctx, site)))
		t := s.periodTotals(ctx, site, today, today.AddDate(0, 0, 1))
		y := s.periodTotals(ctx, site, today.AddDate(0, 0, -1), today)
		w7 := s.periodTotals(ctx, site, today.AddDate(0, 0, -6), today.AddDate(0, 0, 1))
		p7 := s.periodTotals(ctx, site, today.AddDate(0, 0, -13), today.AddDate(0, 0, -6))
		q := store.Query{SiteID: site.ID, From: today.AddDate(0, 0, -6).Unix(), To: today.AddDate(0, 0, 1).Unix()}
		src, _ := s.store.Breakdown(ctx, q, "source", "", "", 3, 0)
		pages, _ := s.store.Breakdown(ctx, q, "page", "", "", 3, 0)
		live := s.store.LiveNow(ctx, site.ID, now.Unix()-300)
		out = append(out, map[string]any{"id": site.ID, "domain": site.Domain, "name": site.Label(), "url": fmt.Sprintf("%s/sites/%d", s.base(), site.ID),
			"today": t.Visitors, "yesterday": y.Visitors, "week": w7.Visitors, "week_before": p7.Visitors, "week_pageviews": w7.Pageviews,
			"live": live, "top_sources": src, "top_pages": pages, "last_hit": site.LastHit})
		total["today"] += t.Visitors
		total["yesterday"] += y.Visitors
		total["week"] += w7.Visitors
		total["live"] += live
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": out, "total": total, "app_url": s.base()})
}

// handleDay is one day's visitors per site (Pocket Day).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	day := chiParam(r, "date")
	sites, err := s.store.ListSites(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	items := []map[string]any{}
	total := 0
	for _, site := range sites {
		loc := s.zoneOf(ctx, site)
		d, err := time.ParseInLocation("2006-01-02", day, loc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the day is YYYY-MM-DD")
			return
		}
		t := s.periodTotals(ctx, site, d, d.AddDate(0, 0, 1))
		if t.Visitors == 0 {
			continue
		}
		total += t.Visitors
		items = append(items, map[string]any{"site_id": site.ID, "domain": site.Domain, "name": site.Label(), "visitors": t.Visitors, "pageviews": t.Pageviews,
			"title": fmt.Sprintf("%s: %d %s", site.Label(), t.Visitors, plural(t.Visitors, "visitor", "visitors")),
			"url":   fmt.Sprintf("%s/sites/%d?range=custom&from=%s&to=%s", s.base(), site.ID, day, day)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": day, "visitors": total, "items": items})
}

// handleSearch finds sites (and their pages) by words.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	sites, err := s.store.ListSites(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	results := []map[string]any{}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": results})
		return
	}
	now := time.Now()
	for _, site := range sites {
		today := dayStart(now.In(s.zoneOf(ctx, site)))
		if strings.Contains(strings.ToLower(site.Domain+" "+site.Name), q) {
			t := s.periodTotals(ctx, site, today.AddDate(0, 0, -29), today.AddDate(0, 0, 1))
			results = append(results, map[string]any{"kind": "site", "id": site.ID, "title": site.Label(), "subtitle": fmt.Sprintf("%d visitors in the last 30 days", t.Visitors),
				"url": fmt.Sprintf("%s/sites/%d", s.base(), site.ID)})
		}
		rows, _ := s.store.Breakdown(ctx, store.Query{SiteID: site.ID, From: today.AddDate(0, 0, -29).Unix(), To: today.AddDate(0, 0, 1).Unix(),
			Filters: []store.Filter{{Dim: "page", Value: "*" + strings.ReplaceAll(q, "*", "") + "*"}}}, "page", "", "", 5, 0)
		for _, row := range rows {
			results = append(results, map[string]any{"kind": "page", "site_id": site.ID, "title": site.Domain + row.Value,
				"subtitle": fmt.Sprintf("%d visitors in the last 30 days", row.Visitors),
				"url":      fmt.Sprintf("%s/sites/%d?f=page:%s", s.base(), site.ID, row.Value)})
		}
		if len(results) >= 20 {
			break
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
