package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Filter narrows what is counted: Dim is a visit property (source,
// country, device…), "page" (visits that saw a page), "event" or "goal".
type Filter struct {
	Dim   string `json:"dim"`
	Value string `json:"value"`
	Not   bool   `json:"not,omitempty"`
}

// Query is a site, a time range [From, To) in Unix seconds and filters.
type Query struct {
	SiteID  int64
	From    int64
	To      int64
	Filters []Filter
	Goals   map[int64]Goal // for "goal" filters
	Path    string         // event and property lists: only what happened on this page
}

// visitCols are the dimensions stored on a visit, by name.
var visitCols = map[string]string{
	"source": "source", "channel": "channel", "referrer": "referrer", "utm_source": "utm_source", "utm_medium": "utm_medium",
	"utm_campaign": "utm_campaign", "utm_content": "utm_content", "utm_term": "utm_term", "country": "country", "region": "region",
	"city": "city", "device": "device", "browser": "browser", "browser_version": "browser_version", "os": "os", "os_version": "os_version",
	"entry": "entry", "exit": "exit", "language": "language", "hostname": "hostname",
}

// ValidDim reports whether a filter dimension exists.
func ValidDim(d string) bool {
	_, ok := visitCols[d]
	return ok || d == "page" || d == "event" || d == "goal"
}

// PathMatch turns a path with * wildcards into an SQL condition on col.
func PathMatch(col, pattern string) (string, any) {
	if !strings.Contains(pattern, "*") {
		return col + " = ?", pattern
	}
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '?', '[', ']':
			b.WriteString("[" + string(r) + "]")
		default:
			b.WriteRune(r)
		}
	}
	return col + " GLOB ?", strings.ReplaceAll(b.String(), "**", "*")
}

func goalCond(alias string, g Goal) (string, []any) {
	if g.Kind == "robot" { // robots are not visits: no hit is one
		return "0", nil
	}
	if g.Kind == "event" {
		return alias + ".name = ?", []any{g.Match}
	}
	c, a := PathMatch(alias+".path", g.Match)
	return alias + ".name = 'pageview' AND " + c, []any{a}
}

// where builds the conditions on visits v (dimensions, page/event/goal) —
// the time range is added by the caller (visits by start, hits by time).
func (q Query) where() (string, []any) {
	var parts []string
	var args []any
	for _, f := range q.Filters {
		var c string
		var a []any
		if col, ok := visitCols[f.Dim]; ok {
			c, a = "v."+col+" = ?", []any{f.Value}
		} else {
			switch f.Dim {
			case "page":
				pc, pa := PathMatch("h2.path", f.Value)
				c, a = "EXISTS (SELECT 1 FROM hits h2 WHERE h2.visit_id = v.id AND h2.name = 'pageview' AND "+pc+")", []any{pa}
			case "event":
				c, a = "EXISTS (SELECT 1 FROM hits h2 WHERE h2.visit_id = v.id AND h2.name = ?)", []any{f.Value}
			case "goal":
				var id int64
				fmt.Sscan(f.Value, &id)
				g, ok := q.Goals[id]
				if !ok || g.Kind == "robot" {
					continue
				}
				gc, ga := goalCond("h2", g)
				c, a = "EXISTS (SELECT 1 FROM hits h2 WHERE h2.visit_id = v.id AND "+gc+")", ga
			default:
				continue
			}
		}
		if f.Not {
			c = "NOT (" + c + ")"
		}
		parts = append(parts, c)
		args = append(args, a...)
	}
	if len(parts) == 0 {
		return "1", nil
	}
	return strings.Join(parts, " AND "), args
}

// pageCond limits hits to the filtered page, for the page lists.
func (q Query) pageCond(alias string) (string, []any) {
	var parts []string
	var args []any
	for _, f := range q.Filters {
		if f.Dim != "page" {
			continue
		}
		c, a := PathMatch(alias+".path", f.Value)
		if f.Not {
			c = "NOT (" + c + ")"
		}
		parts = append(parts, c)
		args = append(args, a)
	}
	if len(parts) == 0 {
		return "1", nil
	}
	return strings.Join(parts, " AND "), args
}

// visitRange is "visits of the site that started in the range and pass the filters".
func (q Query) visitRange() (string, []any) {
	w, a := q.where()
	return "v.site_id = ? AND v.started >= ? AND v.started < ? AND " + w, append([]any{q.SiteID, q.From, q.To}, a...)
}

// hitRange is "hits of the site in the range whose visit passes the filters".
func (q Query) hitRange() (string, []any) {
	w, a := q.where()
	return "h.site_id = ? AND h.ts >= ? AND h.ts < ? AND " + w, append([]any{q.SiteID, q.From, q.To}, a...)
}

const bounceExpr = "CASE WHEN v.pageviews <= 1 AND v.events = 0 THEN 1 ELSE 0 END"

// Totals are the numbers at the top of a site's page.
type Totals struct {
	Visitors      int     `json:"visitors"`
	Visits        int     `json:"visits"`
	Pageviews     int     `json:"pageviews"`
	ViewsPerVisit float64 `json:"views_per_visit"`
	BounceRate    float64 `json:"bounce_rate"` // percent
	Duration      float64 `json:"duration"`    // seconds, average
	Events        int     `json:"events"`
}

// Totals counts a range.
func (s *Store) Totals(ctx context.Context, q Query) (Totals, error) {
	var t Totals
	w, a := q.visitRange()
	var bounces, dur int64
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT v.visitor), COUNT(*), COALESCE(SUM(`+bounceExpr+`), 0), COALESCE(SUM(v.last - v.started), 0) FROM visits v WHERE `+w, a...).
		Scan(&t.Visitors, &t.Visits, &bounces, &dur)
	if err != nil {
		return t, err
	}
	hw, ha := q.hitRange()
	pc, pa := q.pageCond("h")
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN h.name = 'pageview' AND `+pc+` THEN 1 ELSE 0 END), 0), COALESCE(SUM(CASE WHEN h.name <> 'pageview' THEN 1 ELSE 0 END), 0)
		FROM hits h JOIN visits v ON v.id = h.visit_id WHERE `+hw, append(pa, ha...)...).Scan(&t.Pageviews, &t.Events); err != nil {
		return t, err
	}
	if t.Visits > 0 {
		t.ViewsPerVisit = round1(float64(t.Pageviews) / float64(t.Visits))
		t.BounceRate = round1(float64(bounces) * 100 / float64(t.Visits))
		t.Duration = float64(dur) / float64(t.Visits)
	}
	return t, nil
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

// Point is one bucket of the main chart.
type Point struct {
	Start      int64   `json:"start"`
	Visitors   int     `json:"visitors"`
	Visits     int     `json:"visits"`
	Pageviews  int     `json:"pageviews"`
	BounceRate float64 `json:"bounce_rate"`
	Duration   float64 `json:"duration"`
}

// Series counts each bucket; bounds are the bucket starts plus the end.
func (s *Store) Series(ctx context.Context, q Query, bounds []int64) ([]Point, error) {
	n := len(bounds) - 1
	if n < 1 {
		return []Point{}, nil
	}
	pts := make([]Point, n)
	uniq := make([]map[string]struct{}, n)
	bounce := make([]int, n)
	dur := make([]int64, n)
	for i := range pts {
		pts[i].Start = bounds[i]
		uniq[i] = map[string]struct{}{}
	}
	at := func(ts int64) int {
		i := sort.Search(len(bounds), func(i int) bool { return bounds[i] > ts }) - 1
		if i < 0 || i >= n {
			return -1
		}
		return i
	}
	w, a := q.visitRange()
	rows, err := s.DB.QueryContext(ctx, `SELECT v.started, v.visitor, `+bounceExpr+`, v.last - v.started FROM visits v WHERE `+w, a...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var st, d int64
		var vis string
		var b int
		if err := rows.Scan(&st, &vis, &b, &d); err != nil {
			rows.Close()
			return nil, err
		}
		if i := at(st); i >= 0 {
			pts[i].Visits++
			uniq[i][vis] = struct{}{}
			bounce[i] += b
			dur[i] += d
		}
	}
	rows.Close()
	hw, ha := q.hitRange()
	pc, pa := q.pageCond("h")
	rows, err = s.DB.QueryContext(ctx, `SELECT h.ts FROM hits h JOIN visits v ON v.id = h.visit_id WHERE h.name = 'pageview' AND `+pc+` AND `+hw, append(pa, ha...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			rows.Close()
			return nil, err
		}
		if i := at(ts); i >= 0 {
			pts[i].Pageviews++
		}
	}
	rows.Close()
	for i := range pts {
		pts[i].Visitors = len(uniq[i])
		if pts[i].Visits > 0 {
			pts[i].BounceRate = round1(float64(bounce[i]) * 100 / float64(pts[i].Visits))
			pts[i].Duration = float64(dur[i]) / float64(pts[i].Visits)
		}
	}
	return pts, nil
}

// Row is one line of a top list.
type Row struct {
	Value      string  `json:"value"`
	Country    string  `json:"country,omitempty"` // regions and cities: which country
	Visitors   int     `json:"visitors"`
	Visits     int     `json:"visits,omitempty"`
	Pageviews  int     `json:"pageviews,omitempty"`
	Events     int     `json:"events,omitempty"`
	BounceRate float64 `json:"bounce_rate,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	TimeOnPage float64 `json:"time_on_page,omitempty"` // seconds, average
	Scroll     float64 `json:"scroll,omitempty"`       // percent, average
}

// Dims a top list can be made of.
var BreakdownDims = []string{"source", "channel", "referrer", "utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term",
	"country", "region", "city", "device", "browser", "browser_version", "os", "os_version", "language", "hostname",
	"page", "entry", "exit", "event", "outbound", "download", "click", "contact", "form", "search", "notfound", "prop"}

// propEvent names the events behind the link, click, form and search lists.
var propEvent = map[string][2]string{"outbound": {"Outbound Link: Click", "url"}, "download": {"File Download", "url"}, "click": {"Click", "text"},
	"contact": {"Contact Link: Click", "to"}, "form": {"Form: Submission", "form"}, "search": {"Search", "term"}}

// Breakdown is a top list: visitors (and more) per value of dim. For
// "prop", event and key say which event's which property.
func (s *Store) Breakdown(ctx context.Context, q Query, dim, event, key string, limit, offset int) ([]Row, error) {
	if limit <= 0 || limit > 1000 {
		limit = 10
	}
	out := []Row{}
	switch {
	case dim == "page":
		hw, ha := q.hitRange()
		pc, pa := q.pageCond("h")
		rows, err := s.DB.QueryContext(ctx, `SELECT h.path, COUNT(DISTINCT v.visitor), COUNT(*), AVG(h.engaged), AVG(h.scroll), SUM(CASE WHEN h.scroll > 0 THEN 1 ELSE 0 END)
			FROM hits h JOIN visits v ON v.id = h.visit_id WHERE h.name = 'pageview' AND `+pc+` AND `+hw+` GROUP BY h.path ORDER BY 2 DESC, 3 DESC, 1 LIMIT ? OFFSET ?`,
			append(append(pa, ha...), limit, offset)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r Row
			var eng, scr float64
			var scrolled int
			if err := rows.Scan(&r.Value, &r.Visitors, &r.Pageviews, &eng, &scr, &scrolled); err != nil {
				return nil, err
			}
			r.TimeOnPage = eng / 1000
			if scrolled > 0 {
				r.Scroll = round1(scr)
			}
			out = append(out, r)
		}
		return out, rows.Err()
	case dim == "event" || dim == "prop" || dim == "notfound" || propEvent[dim][0] != "":
		hw, ha := q.hitRange()
		col, cond, args := "h.name", "h.name <> 'pageview'", []any{}
		if pe, ok := propEvent[dim]; ok {
			event, key = pe[0], pe[1]
		}
		switch {
		case dim == "notfound":
			// pages that do not exist, by their address
			col, cond = "h.path", "h.name = '404'"
		case dim != "event":
			if event == "" || key == "" {
				return out, nil
			}
			col, cond = "COALESCE(NULLIF(json_extract(h.props, ?), ''), '(none)')", "h.name = ?"
			args = []any{`$."` + strings.ReplaceAll(key, `"`, ``) + `"`, event}
		}
		if q.Path != "" {
			cond += " AND h.path = ?"
			args = append(args, q.Path)
		}
		a := append(args, ha...)
		rows, err := s.DB.QueryContext(ctx, `SELECT `+col+`, COUNT(DISTINCT v.visitor), COUNT(*) FROM hits h JOIN visits v ON v.id = h.visit_id
			WHERE `+cond+` AND `+hw+` GROUP BY 1 ORDER BY 2 DESC, 3 DESC, 1 LIMIT ? OFFSET ?`, append(a, limit, offset)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var r Row
			var v any
			if err := rows.Scan(&v, &r.Visitors, &r.Events); err != nil {
				return nil, err
			}
			r.Value = fmt.Sprint(v)
			out = append(out, r)
		}
		return out, rows.Err()
	}
	col, ok := visitCols[dim]
	if !ok {
		return out, nil
	}
	w, a := q.visitRange()
	sel, group := "v."+col+", ''", "v."+col
	if dim == "region" || dim == "city" {
		sel, group = "v."+col+", v.country", "v."+col+", v.country"
	}
	empty := ""
	if dim == "region" || dim == "city" || dim == "country" {
		empty = " AND v." + col + " <> ''"
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+sel+`, COUNT(DISTINCT v.visitor), COUNT(*), SUM(`+bounceExpr+`), SUM(v.last - v.started), SUM(v.pageviews)
		FROM visits v WHERE `+w+empty+` GROUP BY `+group+` ORDER BY 3 DESC, 4 DESC, 1 LIMIT ? OFFSET ?`, append(a, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r Row
		var b, d int64
		if err := rows.Scan(&r.Value, &r.Country, &r.Visitors, &r.Visits, &b, &d, &r.Pageviews); err != nil {
			return nil, err
		}
		if r.Visits > 0 {
			r.BounceRate = round1(float64(b) * 100 / float64(r.Visits))
			r.Duration = float64(d) / float64(r.Visits)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Money is an amount in one currency ("" = the event did not say which).
type Money struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
}

// Conversion is how a goal did in a range.
type Conversion struct {
	Goal        Goal    `json:"goal"`
	Visitors    int     `json:"visitors"`          // who reached it
	Conversions int     `json:"conversions"`       // how often
	Rate        float64 `json:"rate"`              // percent of all visitors
	Revenue     []Money `json:"revenue,omitempty"` // what it brought, per currency, most first
	Robot       bool    `json:"robot,omitempty"`   // read by robots: Conversions = times, no visitors (the filters do not apply)
}

// Conversions counts each goal.
func (s *Store) Conversions(ctx context.Context, q Query, goals []Goal, total int) ([]Conversion, error) {
	out := []Conversion{}
	hw, ha := q.hitRange()
	for _, g := range goals {
		if g.Kind == "robot" {
			n, err := s.RobotReads(ctx, q.SiteID, q.From, q.To, g.Match, g.Robot)
			if err != nil {
				return nil, err
			}
			out = append(out, Conversion{Goal: g, Conversions: n, Robot: true})
			continue
		}
		gc, ga := goalCond("h", g)
		c := Conversion{Goal: g}
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT v.visitor), COUNT(*) FROM hits h JOIN visits v ON v.id = h.visit_id WHERE `+gc+` AND `+hw, append(ga, ha...)...).
			Scan(&c.Visitors, &c.Conversions); err != nil {
			return nil, err
		}
		if total > 0 {
			c.Rate = round1(float64(c.Visitors) * 100 / float64(total))
		}
		if g.Kind == "event" && c.Conversions > 0 {
			rows, err := s.DB.QueryContext(ctx, `SELECT h.currency, SUM(h.revenue) FROM hits h JOIN visits v ON v.id = h.visit_id WHERE h.revenue > 0 AND `+gc+` AND `+hw+` GROUP BY 1 ORDER BY 2 DESC LIMIT 5`, append(ga, ha...)...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var m Money
				if err := rows.Scan(&m.Currency, &m.Amount); err != nil {
					rows.Close()
					return nil, err
				}
				m.Amount = float64(int64(m.Amount*100+0.5)) / 100
				c.Revenue = append(c.Revenue, m)
			}
			rows.Close()
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Robot != out[j].Robot {
			return !out[i].Robot
		}
		return out[i].Visitors > out[j].Visitors
	})
	return out, nil
}

// RobotReads is how often robots read the pages matching a path (* = any
// part) in [from, to); robot narrows it to one robot by name.
func (s *Store) RobotReads(ctx context.Context, siteID, from, to int64, match, robot string) (int, error) {
	pc, pa := PathMatch("path", match)
	where, args := "site_id = ? AND ts >= ? AND ts < ? AND "+pc, []any{siteID, from / 3600 * 3600, to, pa}
	if robot != "" {
		where += " AND name = ?"
		args = append(args, robot)
	}
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(hits), 0) FROM robots WHERE `+where, args...).Scan(&n)
	return n, err
}

// StepCount is how many visitors reached a funnel step.
type StepCount struct {
	Step     Step    `json:"step"`
	Visitors int     `json:"visitors"`
	Dropped  int     `json:"dropped"`         // left after the step before
	Rate     float64 `json:"rate"`            // percent of the first step
	Next     float64 `json:"next"`            // percent of the step before
	Robot    bool    `json:"robot,omitempty"` // a robot step: Visitors = times read, not people
}

// FunnelCounts walks each visit's hits in order through the steps.
func (s *Store) FunnelCounts(ctx context.Context, q Query, steps []Step) ([]StepCount, error) {
	out := make([]StepCount, len(steps))
	for i := range steps {
		out[i].Step = steps[i]
	}
	if len(steps) == 0 {
		return out, nil
	}
	// a last robot step (install.sh run by curl) is counted on its own: a
	// robot is not in anyone's visit
	if last := steps[len(steps)-1]; last.Kind == "robot" {
		head, err := s.FunnelCounts(ctx, q, steps[:len(steps)-1])
		if err != nil {
			return nil, err
		}
		n, err := s.RobotReads(ctx, q.SiteID, q.From, q.To, last.Match, last.Robot)
		if err != nil {
			return nil, err
		}
		rs := StepCount{Step: last, Visitors: n, Robot: true}
		if k := len(head); k > 0 {
			prev := head[k-1].Visitors
			rs.Dropped = max(prev-n, 0)
			if prev > 0 {
				rs.Next = round1(float64(n) * 100 / float64(prev))
			}
			if head[0].Visitors > 0 {
				rs.Rate = round1(float64(n) * 100 / float64(head[0].Visitors))
			}
		}
		return append(head, rs), nil
	}
	var conds []string
	var cargs []any
	for _, st := range steps {
		c, a := goalCond("h", Goal{Kind: st.Kind, Match: st.Match})
		conds = append(conds, "("+c+")")
		cargs = append(cargs, a...)
	}
	hw, ha := q.hitRange()
	rows, err := s.DB.QueryContext(ctx, `SELECT h.visit_id, v.visitor, h.name, h.path FROM hits h JOIN visits v ON v.id = h.visit_id
		WHERE (`+strings.Join(conds, " OR ")+`) AND `+hw+` ORDER BY h.visit_id, h.ts, h.id`, append(cargs, ha...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reached := make([]map[string]struct{}, len(steps))
	for i := range reached {
		reached[i] = map[string]struct{}{}
	}
	var cur int64 = -1
	next := 0
	var visitor string
	for rows.Next() {
		var vid int64
		var name, path string
		if err := rows.Scan(&vid, &visitor, &name, &path); err != nil {
			return nil, err
		}
		if vid != cur {
			cur, next = vid, 0
		}
		if next < len(steps) && StepMatches(steps[next], name, path) {
			reached[next][visitor] = struct{}{}
			next++
		}
	}
	for i := range out {
		out[i].Visitors = len(reached[i])
		if i > 0 {
			out[i].Dropped = out[i-1].Visitors - out[i].Visitors
			if out[i-1].Visitors > 0 {
				out[i].Next = round1(float64(out[i].Visitors) * 100 / float64(out[i-1].Visitors))
			}
		}
		if out[0].Visitors > 0 {
			out[i].Rate = round1(float64(out[i].Visitors) * 100 / float64(out[0].Visitors))
		}
	}
	return out, rows.Err()
}

// StepMatches reports whether a hit is a step (the same rule as in SQL).
func StepMatches(st Step, name, path string) bool {
	if st.Kind == "event" {
		return name == st.Match
	}
	return name == "pageview" && GlobMatch(st.Match, path)
}

// GlobMatch matches a path against a pattern where * is any text.
func GlobMatch(pattern, s string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == s
	}
	parts := strings.Split(strings.ReplaceAll(pattern, "**", "*"), "*")
	if !strings.HasPrefix(s, parts[0]) {
		return false
	}
	s = s[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if i == len(parts)-1 {
			return strings.HasSuffix(s, p)
		}
		j := strings.Index(s, p)
		if j < 0 {
			return false
		}
		s = s[j+len(p):]
	}
	return true
}

// ---- right now ----

// Live is what is happening on a site at the moment.
type Live struct {
	Now       int       `json:"now"`     // visitors in the last 5 minutes
	Minutes   []int     `json:"minutes"` // pageviews per minute, the last 30, oldest first
	Pages     []Row     `json:"pages"`   // where the people on the site are
	Sources   []Row     `json:"sources"` // where they came from
	Countries []Row     `json:"countries"`
	Recent    []LiveHit `json:"recent"`
}

// LiveHit is one of the latest hits.
type LiveHit struct {
	TS      int64  `json:"ts"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Country string `json:"country"`
	City    string `json:"city"`
	Device  string `json:"device"`
	Browser string `json:"browser"`
	Source  string `json:"source"`
	VisitID int64  `json:"visit_id"`
}

// LiveNow counts visitors seen since a moment.
func (s *Store) LiveNow(ctx context.Context, siteID, since int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT visitor) FROM visits WHERE site_id = ? AND last >= ?`, siteID, since).Scan(&n)
	return n
}

// LiveView is the realtime page.
func (s *Store) LiveView(ctx context.Context, siteID, now int64) (*Live, error) {
	l := &Live{Minutes: make([]int, 30), Pages: []Row{}, Sources: []Row{}, Countries: []Row{}, Recent: []LiveHit{}}
	since := now - 5*60
	l.Now = s.LiveNow(ctx, siteID, since)
	rows, err := s.DB.QueryContext(ctx, `SELECT ts FROM hits WHERE site_id = ? AND name = 'pageview' AND ts >= ?`, siteID, now-30*60)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ts int64
		if rows.Scan(&ts) == nil {
			i := 29 - int((now-ts)/60)
			if i >= 0 && i < 30 {
				l.Minutes[i]++
			}
		}
	}
	rows.Close()
	group := func(col string) ([]Row, error) {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+col+`, COUNT(DISTINCT visitor) FROM visits WHERE site_id = ? AND last >= ? GROUP BY 1 ORDER BY 2 DESC, 1 LIMIT 8`, siteID, since)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []Row{}
		for rows.Next() {
			var r Row
			if err := rows.Scan(&r.Value, &r.Visitors); err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, rows.Err()
	}
	if l.Pages, err = group("exit"); err != nil {
		return nil, err
	}
	if l.Sources, err = group("source"); err != nil {
		return nil, err
	}
	if l.Countries, err = group("country"); err != nil {
		return nil, err
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT h.ts, h.name, h.path, v.country, v.city, v.device, v.browser, v.source, v.id FROM hits h JOIN visits v ON v.id = h.visit_id
		WHERE h.site_id = ? AND h.ts >= ? ORDER BY h.ts DESC, h.id DESC LIMIT 25`, siteID, now-30*60)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var h LiveHit
		if err := rows.Scan(&h.TS, &h.Name, &h.Path, &h.Country, &h.City, &h.Device, &h.Browser, &h.Source, &h.VisitID); err != nil {
			return nil, err
		}
		l.Recent = append(l.Recent, h)
	}
	return l, rows.Err()
}

// ---- visits, one by one ----

const visitCols2 = `v.id, v.site_id, v.started, v.last, v.hostname, v.entry, v.exit, v.pageviews, v.events, v.engaged, v.referrer, v.source, v.channel,
	v.utm_source, v.utm_medium, v.utm_campaign, v.utm_content, v.utm_term, v.country, v.region, v.city, v.device, v.browser, v.browser_version, v.os, v.os_version, v.language`

func scanVisit(row interface{ Scan(...any) error }) (*Visit, error) {
	var v Visit
	err := row.Scan(&v.ID, &v.SiteID, &v.Started, &v.Last, &v.Hostname, &v.Entry, &v.Exit, &v.Pageviews, &v.Events, &v.Engaged, &v.Referrer, &v.Source, &v.Channel,
		&v.UTMSource, &v.UTMMedium, &v.UTMCampaign, &v.UTMContent, &v.UTMTerm, &v.Country, &v.Region, &v.City, &v.Device, &v.Browser, &v.BrowserVersion, &v.OS, &v.OSVersion, &v.Language)
	if err != nil {
		return nil, notFound(err)
	}
	return &v, nil
}

// ListVisits is the latest visits in a range, newest first.
func (s *Store) ListVisits(ctx context.Context, q Query, limit, offset int) ([]*Visit, int, error) {
	w, a := q.visitRange()
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM visits v WHERE `+w, a...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+visitCols2+` FROM visits v WHERE `+w+` ORDER BY v.started DESC, v.id DESC LIMIT ? OFFSET ?`, append(a, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Visit{}
	for rows.Next() {
		v, err := scanVisit(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// GetVisit is one visit of a site with its hits in order.
func (s *Store) GetVisit(ctx context.Context, siteID, id int64) (*Visit, []Hit, error) {
	v, err := scanVisit(s.DB.QueryRowContext(ctx, `SELECT `+visitCols2+` FROM visits v WHERE v.id = ? AND v.site_id = ?`, id, siteID))
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id, visit_id, ts, name, path, props, engaged, scroll, revenue, currency, lcp, inp, cls FROM hits WHERE visit_id = ? ORDER BY ts, id LIMIT 500`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	hits := []Hit{}
	for rows.Next() {
		var h Hit
		var props string
		if err := rows.Scan(&h.ID, &h.VisitID, &h.TS, &h.Name, &h.Path, &props, &h.Engaged, &h.Scroll, &h.Revenue, &h.Currency, &h.LCP, &h.INP, &h.CLS); err != nil {
			return nil, nil, err
		}
		if props != "" {
			_ = json.Unmarshal([]byte(props), &h.Props)
		}
		hits = append(hits, h)
	}
	return v, hits, rows.Err()
}

// EventNames are the site's own events seen in a range (for goals and filters).
func (s *Store) EventNames(ctx context.Context, siteID, from int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT name FROM hits WHERE site_id = ? AND ts >= ? AND name <> 'pageview' ORDER BY name LIMIT 200`, siteID, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// PropKeys are the property names an event was sent with lately.
func (s *Store) PropKeys(ctx context.Context, siteID int64, event string, from int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT j.key FROM hits h, json_each(h.props) j WHERE h.site_id = ? AND h.name = ? AND h.ts >= ? AND h.props <> '' LIMIT 50`, siteID, event, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out, rows.Err()
}

// CountVisitsSince is visits of a site since a moment (alerts, sidebar).
func (s *Store) CountVisitsSince(ctx context.Context, siteID, since int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM visits WHERE site_id = ? AND started >= ?`, siteID, since).Scan(&n)
	return n
}

// DailyVisitors is unique visitors per UTC-day-free bucket: bounds as in Series.
func (s *Store) VisitorsIn(ctx context.Context, siteID, from, to int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(DISTINCT visitor) FROM visits WHERE site_id = ? AND started >= ? AND started < ?`, siteID, from, to).Scan(&n)
	return n
}

// ---- how fast the pages are ----

// Metric is one speed measure over many page loads: the value three out
// of four loads stayed under (the 75th percentile), and how many loads.
type Metric struct {
	P75 float64 `json:"p75"`
	N   int     `json:"n"`
}

// SpeedRow is how fast one page (or device, country, browser) was.
type SpeedRow struct {
	Value   string  `json:"value"`
	Loads   int     `json:"loads"`          // page loads measured
	LCP     *Metric `json:"lcp,omitempty"`  // ms until the main content showed
	INP     *Metric `json:"inp,omitempty"`  // ms the slowest press took
	CLS     *Metric `json:"cls,omitempty"`  // how much the layout moved
	TTFB    *Metric `json:"ttfb,omitempty"` // ms until the server answered
	FCP     *Metric `json:"fcp,omitempty"`  // ms until something showed
	samples [5][]float64
}

// speedDims are what speed can be split by, and the column for each.
var speedDims = map[string]string{"page": "h.path", "device": "v.device", "browser": "v.browser", "os": "v.os", "country": "v.country"}

// ValidSpeedDim reports whether speed can be split by dim.
func ValidSpeedDim(d string) bool { _, ok := speedDims[d]; return ok }

func p75(v []float64) *Metric {
	if len(v) == 0 {
		return nil
	}
	sort.Float64s(v)
	i := (len(v)*3 + 3) / 4
	return &Metric{P75: v[i-1], N: len(v)}
}

func (r *SpeedRow) finish() {
	r.TTFB, r.FCP, r.LCP, r.INP, r.CLS = p75(r.samples[0]), p75(r.samples[1]), p75(r.samples[2]), p75(r.samples[3]), p75(r.samples[4])
	r.samples = [5][]float64{}
}

// Speed is how fast the site was in a range: over all measured page loads,
// and for the most loaded values of dim.
func (s *Store) Speed(ctx context.Context, q Query, dim string, limit int) (SpeedRow, []SpeedRow, error) {
	all := SpeedRow{}
	col, ok := speedDims[dim]
	if !ok {
		col = "h.path"
	}
	if limit <= 0 || limit > 1000 {
		limit = 10
	}
	hw, ha := q.hitRange()
	pc, pa := q.pageCond("h")
	rows, err := s.DB.QueryContext(ctx, `SELECT `+col+`, h.ttfb, h.fcp, h.lcp, h.inp, h.cls FROM hits h JOIN visits v ON v.id = h.visit_id
		WHERE h.name = 'pageview' AND (h.lcp IS NOT NULL OR h.ttfb IS NOT NULL OR h.fcp IS NOT NULL OR h.inp IS NOT NULL OR h.cls IS NOT NULL)
		AND `+pc+` AND `+hw+` LIMIT 400000`, append(pa, ha...)...)
	if err != nil {
		return all, nil, err
	}
	defer rows.Close()
	by := map[string]*SpeedRow{}
	for rows.Next() {
		var key string
		var m [5]*float64
		if err := rows.Scan(&key, &m[0], &m[1], &m[2], &m[3], &m[4]); err != nil {
			return all, nil, err
		}
		r := by[key]
		if r == nil {
			r = &SpeedRow{Value: key}
			by[key] = r
		}
		r.Loads++
		all.Loads++
		for i, v := range m {
			if v != nil {
				r.samples[i] = append(r.samples[i], *v)
				all.samples[i] = append(all.samples[i], *v)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return all, nil, err
	}
	all.finish()
	out := make([]SpeedRow, 0, len(by))
	for _, r := range by {
		r.finish()
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Loads != out[j].Loads {
			return out[i].Loads > out[j].Loads
		}
		return out[i].Value < out[j].Value
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return all, out, nil
}

// ---- when they come ----

// VisitStarts are the start times of the visits in a range (for the
// week-by-hour picture; the caller knows the site's time zone).
func (s *Store) VisitStarts(ctx context.Context, q Query) ([]int64, error) {
	w, a := q.visitRange()
	rows, err := s.DB.QueryContext(ctx, `SELECT v.started FROM visits v WHERE `+w, a...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var t int64
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ---- the way through a page ----

// PathStep is a page people were on before or after another, and how often.
// Value "" is the edge of the visit: they came in here, or left from here.
type PathStep struct {
	Value string `json:"value"`
	Times int    `json:"times"`
}

// Paths is where people were before a page and where they went after it.
type Paths struct {
	Page   string     `json:"page"`
	Views  int        `json:"views"` // times the page was seen in a visit (reloads count once)
	Before []PathStep `json:"before"`
	After  []PathStep `json:"after"`
}

// PagePaths walks every visit that saw the page (a path, * for any part).
func (s *Store) PagePaths(ctx context.Context, q Query, page string, limit int) (*Paths, error) {
	out := &Paths{Page: page, Before: []PathStep{}, After: []PathStep{}}
	if limit <= 0 || limit > 100 {
		limit = 8
	}
	has := false
	for _, f := range q.Filters {
		if f.Dim == "page" && f.Value == page && !f.Not {
			has = true
		}
	}
	if !has {
		q.Filters = append(append([]Filter{}, q.Filters...), Filter{Dim: "page", Value: page})
	}
	hw, ha := q.hitRange()
	rows, err := s.DB.QueryContext(ctx, `SELECT h.visit_id, h.path FROM hits h JOIN visits v ON v.id = h.visit_id WHERE h.name = 'pageview' AND `+hw+` ORDER BY h.visit_id, h.ts, h.id`, ha...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	before, after := map[string]int{}, map[string]int{}
	var seq []string
	walk := func() {
		for i, p := range seq {
			if !GlobMatch(page, p) {
				continue
			}
			out.Views++
			prev, next := "", ""
			if i > 0 {
				prev = seq[i-1]
			}
			if i+1 < len(seq) {
				next = seq[i+1]
			}
			before[prev]++
			after[next]++
		}
		seq = seq[:0]
	}
	var cur int64 = -1
	for rows.Next() {
		var vid int64
		var path string
		if err := rows.Scan(&vid, &path); err != nil {
			return nil, err
		}
		if vid != cur {
			walk()
			cur = vid
		}
		if n := len(seq); n == 0 || seq[n-1] != path { // a reload is the same page still
			seq = append(seq, path)
		}
	}
	walk()
	top := func(m map[string]int) []PathStep {
		list := make([]PathStep, 0, len(m))
		for k, n := range m {
			list = append(list, PathStep{Value: k, Times: n})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Times != list[j].Times {
				return list[i].Times > list[j].Times
			}
			return list[i].Value < list[j].Value
		})
		if len(list) > limit {
			list = list[:limit]
		}
		return list
	}
	out.Before, out.After = top(before), top(after)
	return out, rows.Err()
}
