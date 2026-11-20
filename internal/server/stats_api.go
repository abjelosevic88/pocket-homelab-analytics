package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// span is the time range a page looks at, in the site's time zone.
type span struct {
	Key      string    `json:"key"` // today | yesterday | 7d | 30d | month | lastmonth | 12mo | year | all | custom
	From     time.Time `json:"-"`
	To       time.Time `json:"-"` // exclusive
	PFrom    time.Time `json:"-"` // the period before, as long
	PTo      time.Time `json:"-"`
	Interval string    `json:"interval"` // hour | day | week | month
	FromDay  string    `json:"from"`
	ToDay    string    `json:"to"` // inclusive, for showing
	Label    string    `json:"label"`
}

var rangeKeys = map[string]bool{"today": true, "yesterday": true, "7d": true, "30d": true, "month": true, "lastmonth": true, "12mo": true, "year": true, "all": true, "custom": true}

// parseSpan reads ?range, ?from, ?to and ?interval.
func parseSpan(r *http.Request, loc *time.Location, firstHit int64, weekStart int) span {
	q := r.URL.Query()
	now := time.Now().In(loc)
	today := dayStart(now)
	sp := span{Key: q.Get("range")}
	if !rangeKeys[sp.Key] {
		sp.Key = "30d"
	}
	switch sp.Key {
	case "today":
		sp.From, sp.To, sp.Label = today, today.AddDate(0, 0, 1), "Today"
	case "yesterday":
		sp.From, sp.To, sp.Label = today.AddDate(0, 0, -1), today, "Yesterday"
	case "7d":
		sp.From, sp.To, sp.Label = today.AddDate(0, 0, -6), today.AddDate(0, 0, 1), "Last 7 days"
	case "30d":
		sp.From, sp.To, sp.Label = today.AddDate(0, 0, -29), today.AddDate(0, 0, 1), "Last 30 days"
	case "month":
		sp.From = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		sp.To, sp.Label = sp.From.AddDate(0, 1, 0), "This month"
	case "lastmonth":
		sp.To = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
		sp.From, sp.Label = sp.To.AddDate(0, -1, 0), "Last month"
	case "12mo":
		sp.From = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -11, 0)
		sp.To, sp.Label = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0), "Last 12 months"
	case "year":
		sp.From = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
		sp.To, sp.Label = sp.From.AddDate(1, 0, 0), "This year"
	case "all":
		first := today.AddDate(0, 0, -29)
		if firstHit > 0 {
			first = dayStart(time.Unix(firstHit, 0).In(loc))
		}
		sp.From, sp.To, sp.Label = first, today.AddDate(0, 0, 1), "All time"
	case "custom":
		a, errA := time.ParseInLocation("2006-01-02", q.Get("from"), loc)
		b, errB := time.ParseInLocation("2006-01-02", q.Get("to"), loc)
		if errA != nil || errB != nil {
			a, b = today.AddDate(0, 0, -29), today
		}
		if b.Before(a) {
			a, b = b, a
		}
		if b.Sub(a) > 3660*24*time.Hour {
			a = b.AddDate(-10, 0, 0)
		}
		sp.From, sp.To = a, b.AddDate(0, 0, 1)
	}
	days := int(sp.To.Sub(sp.From).Hours()/24 + 0.5)
	switch sp.Key {
	case "month", "lastmonth":
		sp.PTo, sp.PFrom = sp.From, sp.From.AddDate(0, -1, 0)
	case "12mo", "year":
		sp.PTo, sp.PFrom = sp.From, sp.From.AddDate(-1, 0, 0)
	default:
		sp.PTo, sp.PFrom = sp.From, sp.From.AddDate(0, 0, -days)
	}
	sp.Interval = q.Get("interval")
	auto := "month"
	switch {
	case days <= 2:
		auto = "hour"
	case days <= 100:
		auto = "day"
	}
	okInterval := map[string]bool{"hour": days <= 7, "day": days <= 750, "week": days >= 14, "month": days >= 60}
	if !okInterval[sp.Interval] {
		sp.Interval = auto
	}
	sp.FromDay, sp.ToDay = sp.From.Format("2006-01-02"), sp.To.AddDate(0, 0, -1).Format("2006-01-02")
	if sp.Label == "" {
		sp.Label = sp.FromDay + " – " + sp.ToDay
	}
	_ = weekStart
	return sp
}

// bounds are the bucket starts of a range plus its end.
func bounds(from, to time.Time, interval string, weekStart int) []int64 {
	t := from
	switch interval {
	case "week":
		wd := int(t.Weekday())
		start := weekStart % 7 // 1 = Monday, 7 → 0 = Sunday, 6 = Saturday
		back := (wd - start + 7) % 7
		t = t.AddDate(0, 0, -back)
	case "month":
		t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	}
	out := []int64{}
	for t.Before(to) && len(out) < 2000 {
		out = append(out, max(t.Unix(), from.Unix()))
		switch interval {
		case "hour":
			t = t.Add(time.Hour)
		case "day":
			t = t.AddDate(0, 0, 1)
		case "week":
			t = t.AddDate(0, 0, 7)
		default:
			t = t.AddDate(0, 1, 0)
		}
	}
	return append(out, to.Unix())
}

// parseFilters reads ?f=dim:value (and ?f=!dim:value for "is not").
func parseFilters(r *http.Request) []store.Filter {
	out := []store.Filter{}
	for _, f := range r.URL.Query()["f"] {
		not := strings.HasPrefix(f, "!")
		f = strings.TrimPrefix(f, "!")
		dim, val, ok := strings.Cut(f, ":")
		if !ok || !store.ValidDim(dim) || len(out) >= 12 {
			continue
		}
		out = append(out, store.Filter{Dim: dim, Value: clip(val, 500), Not: not})
	}
	return out
}

// queryFor is the site's query for the request's range and filters.
func (s *Server) queryFor(r *http.Request, site *store.Site) (store.Query, span) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, site.UserID)
	sp := parseSpan(r, s.zoneOf(ctx, site), site.FirstHit, st.WeekStart)
	q := store.Query{SiteID: site.ID, From: sp.From.Unix(), To: sp.To.Unix(), Filters: parseFilters(r)}
	for _, f := range q.Filters {
		if f.Dim == "goal" {
			goals, _ := s.store.ListGoals(ctx, site.UserID, site.ID)
			q.Goals = map[int64]store.Goal{}
			for _, g := range goals {
				q.Goals[g.ID] = g
			}
			break
		}
	}
	return q, sp
}

// handleStats is the top of a site's page: the numbers and the chart, with
// the period before to compare.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, site.UserID)
	q, sp := s.queryFor(r, site)
	totals, err := s.store.Totals(ctx, q)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	pq := q
	pq.From, pq.To = sp.PFrom.Unix(), sp.PTo.Unix()
	prev, err := s.store.Totals(ctx, pq)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	series, err := s.store.Series(ctx, q, bounds(sp.From, sp.To, sp.Interval, st.WeekStart))
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	// the period before, bucket for bucket (for the dashed line)
	pb := bounds(sp.PFrom, sp.PTo, sp.Interval, st.WeekStart)
	prevSeries, _ := s.store.Series(ctx, pq, pb)
	// what happened on the days in the range, with the moment each day starts (for the chart)
	type noteAt struct {
		store.Note
		TS int64 `json:"ts"`
	}
	notes := []noteAt{}
	list, _ := s.store.ListNotes(ctx, site.UserID, site.ID, sp.FromDay, sp.ToDay)
	for _, n := range list {
		if d, err := time.ParseInLocation("2006-01-02", n.Day, sp.From.Location()); err == nil {
			notes = append(notes, noteAt{n, d.Unix()})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"site": s.siteView(ctx, site), "range": sp, "filters": q.Filters, "totals": totals, "previous": prev, "notes": notes,
		"series": series, "previous_series": prevSeries, "now": time.Now().Unix(), "live": s.store.LiveNow(ctx, site.ID, time.Now().Unix()-300),
	})
}

// handleBreakdown is one top list (sources, pages, countries…).
func (s *Server) handleBreakdown(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	q.Path = clip(r.URL.Query().Get("path"), 500)
	dim := r.URL.Query().Get("dim")
	limit := queryInt(r, "limit")
	rows, err := s.store.Breakdown(r.Context(), q, dim, r.URL.Query().Get("event"), r.URL.Query().Get("key"), limit, queryInt(r, "offset"))
	if err != nil {
		s.fail(w, err, "breakdown")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dim": dim, "rows": rows})
}

// handleConversions is how each goal did.
func (s *Server) handleConversions(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q, _ := s.queryFor(r, site)
	goals, _ := s.store.ListGoals(ctx, site.UserID, site.ID)
	t, err := s.store.Totals(ctx, q)
	if err != nil {
		s.fail(w, err, "goals")
		return
	}
	conv, err := s.store.Conversions(ctx, q, goals, t.Visitors)
	if err != nil {
		s.fail(w, err, "goals")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"goals": conv, "visitors": t.Visitors})
}

func (s *Server) handleFunnelCounts(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	f, err := s.store.GetFunnel(r.Context(), site.UserID, idParam(r, "fid"))
	if err != nil || f.SiteID != site.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	q, _ := s.queryFor(r, site)
	steps, err := s.store.FunnelCounts(r.Context(), q, f.Steps)
	if err != nil {
		s.fail(w, err, "funnel")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"funnel": f, "steps": steps})
}

func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	l, err := s.store.LiveView(r.Context(), site.ID, time.Now().Unix())
	if err != nil {
		s.fail(w, err, "live")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (s *Server) handleVisits(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	visits, total, err := s.store.ListVisits(r.Context(), q, limit, max(queryInt(r, "offset"), 0))
	if err != nil {
		s.fail(w, err, "visits")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"visits": visits, "total": total})
}

func (s *Server) handleVisit(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	v, hits, err := s.store.GetVisit(r.Context(), site.ID, idParam(r, "vid"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"visit": v, "hits": hits})
}

// handleEventNames lists the site's own events and, for one, its property names.
func (s *Server) handleEventNames(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	since := time.Now().AddDate(0, 0, -90).Unix()
	names, err := s.store.EventNames(r.Context(), site.ID, since)
	if err != nil {
		s.fail(w, err, "events")
		return
	}
	out := map[string]any{"events": names}
	if ev := r.URL.Query().Get("event"); ev != "" {
		keys, _ := s.store.PropKeys(r.Context(), site.ID, ev, since)
		out["keys"] = keys
	}
	writeJSON(w, http.StatusOK, out)
}

// handleExportCSV downloads a top list (or every visit) as a spreadsheet.
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q, sp := s.queryFor(r, site)
	dim := r.URL.Query().Get("dim")
	name := fmt.Sprintf("%s-%s-%s-%s.csv", site.Domain, firstNonEmpty(dim, "visits"), sp.FromDay, sp.ToDay)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if dim == "" {
		visits, _, err := s.store.ListVisits(ctx, q, 100000, 0)
		if err != nil {
			return
		}
		loc := s.zoneOf(ctx, site)
		_ = cw.Write([]string{"started", "seconds", "pages", "events", "entry", "exit", "source", "channel", "referrer", "utm_source", "utm_medium", "utm_campaign", "country", "region", "city", "device", "browser", "os", "language"})
		for _, v := range visits {
			_ = cw.Write([]string{time.Unix(v.Started, 0).In(loc).Format("2006-01-02 15:04:05"), strconv.FormatInt(v.Last-v.Started, 10), strconv.Itoa(v.Pageviews), strconv.Itoa(v.Events),
				v.Entry, v.Exit, v.Source, v.Channel, v.Referrer, v.UTMSource, v.UTMMedium, v.UTMCampaign, v.Country, v.Region, v.City, v.Device, v.Browser, v.OS, v.Language})
		}
		return
	}
	rows, err := s.store.Breakdown(ctx, q, dim, r.URL.Query().Get("event"), r.URL.Query().Get("key"), 1000, 0)
	if err != nil {
		return
	}
	_ = cw.Write([]string{dim, "country", "visitors", "visits", "pageviews", "events", "bounce_rate", "visit_seconds", "seconds_on_page", "scroll_percent"})
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
	for _, row := range rows {
		_ = cw.Write([]string{row.Value, row.Country, strconv.Itoa(row.Visitors), strconv.Itoa(row.Visits), strconv.Itoa(row.Pageviews), strconv.Itoa(row.Events), f(row.BounceRate), f(row.Duration), f(row.TimeOnPage), f(row.Scroll)})
	}
}

// periodTotals counts a site between two moments (reports, summaries).
func (s *Server) periodTotals(ctx context.Context, site *store.Site, from, to time.Time) store.Totals {
	t, _ := s.store.Totals(ctx, store.Query{SiteID: site.ID, From: from.Unix(), To: to.Unix()})
	return t
}
