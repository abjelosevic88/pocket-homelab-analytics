package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
)

// ---- how people use the site: the pages they land on, the ways they
// take, how far they read and what the AI assistants do ----

// goalsCond is "the hit reached one of these goals" (robot goals left out:
// no hit is one); "" when there are none. A page goal counts only on a
// page after the first: landing on it is where the visit started, not
// where it led.
func goalsCond(alias string, goals []Goal) (string, []any) {
	var parts []string
	var args []any
	for _, g := range goals {
		if g.Kind == "robot" {
			continue
		}
		c, a := goalCond(alias, g)
		if g.Kind == "page" {
			c += " AND " + alias + ".id <> (SELECT MIN(hf.id) FROM hits hf WHERE hf.visit_id = " + alias + ".visit_id AND hf.name = 'pageview')"
		}
		parts = append(parts, "("+c+")")
		args = append(args, a...)
	}
	if len(parts) == 0 {
		return "", nil
	}
	return strings.Join(parts, " OR "), args
}

// Landing is how the visits that came in on one page went.
type Landing struct {
	Page      string  `json:"page"`
	Visits    int     `json:"visits"`
	Visitors  int     `json:"visitors"`
	Bounce    float64 `json:"bounce"`    // percent who saw only this page and did nothing
	Engaged   float64 `json:"engaged"`   // seconds in front, average per visit
	Pages     float64 `json:"pages"`     // pages per visit, average
	Converted int     `json:"converted"` // visits that reached a goal
	Rate      float64 `json:"rate"`      // percent of the visits
}

// LandingPages lists the pages visits started on, with how each went; a
// visit "converted" when it reached any of the goals.
func (s *Store) LandingPages(ctx context.Context, q Query, goals []Goal, limit int) ([]Landing, bool, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	w, a := q.visitRange()
	conv, cargs := "0", []any{}
	gc, ga := goalsCond("h2", goals)
	if gc != "" {
		conv, cargs = "CASE WHEN EXISTS (SELECT 1 FROM hits h2 WHERE h2.visit_id = v.id AND ("+gc+")) THEN 1 ELSE 0 END", ga
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT v.entry, COUNT(*), COUNT(DISTINCT v.visitor), SUM(`+bounceExpr+`), AVG(v.engaged), AVG(v.pageviews), SUM(`+conv+`)
		FROM visits v WHERE `+w+` GROUP BY v.entry ORDER BY 2 DESC, 1 LIMIT ?`, append(append(cargs, a...), limit)...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Landing{}
	for rows.Next() {
		var l Landing
		var bounced int
		var eng, pages float64
		if err := rows.Scan(&l.Page, &l.Visits, &l.Visitors, &bounced, &eng, &pages, &l.Converted); err != nil {
			return nil, false, err
		}
		if l.Visits > 0 {
			l.Bounce = round1(float64(bounced) * 100 / float64(l.Visits))
			l.Rate = round1(float64(l.Converted) * 100 / float64(l.Visits))
		}
		l.Engaged, l.Pages = round1(eng/1000), round1(pages)
		out = append(out, l)
	}
	return out, gc != "", rows.Err()
}

// Journey is a way through the site: the first pages of a visit in order.
type Journey struct {
	Pages     []string `json:"pages"`
	Visits    int      `json:"visits"`
	More      int      `json:"more"`      // of these, visits that went on to more pages
	Converted int      `json:"converted"` // of these, visits that reached a goal
}

// Journeys groups visits by their first pages (up to steps; a reload is
// the same page still), most common first.
func (s *Store) Journeys(ctx context.Context, q Query, goals []Goal, steps, limit int) ([]Journey, int, error) {
	if steps < 2 || steps > 6 {
		steps = 3
	}
	if limit <= 0 || limit > 100 {
		limit = 15
	}
	hw, ha := q.hitRange()
	reached := map[int64]bool{}
	if gc, ga := goalsCond("h", goals); gc != "" {
		rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT h.visit_id FROM hits h JOIN visits v ON v.id = h.visit_id WHERE (`+gc+`) AND `+hw, append(ga, ha...)...)
		if err != nil {
			return nil, 0, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return nil, 0, err
			}
			reached[id] = true
		}
		rows.Close()
	}
	// visits that started in the range, all their pages in order
	vw, va := q.visitRange()
	rows, err := s.DB.QueryContext(ctx, `SELECT h.visit_id, h.path FROM hits h JOIN visits v ON v.id = h.visit_id
		WHERE h.name = 'pageview' AND `+vw+` ORDER BY h.visit_id, h.ts, h.id`, va...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	type agg struct {
		pages            []string
		n, more, reached int
	}
	byKey := map[string]*agg{}
	total := 0
	var cur int64 = -1
	var seq []string
	done := func() {
		if len(seq) == 0 {
			return
		}
		total++
		head := seq
		if len(head) > steps {
			head = head[:steps]
		}
		k := strings.Join(head, "\x00")
		a := byKey[k]
		if a == nil {
			a = &agg{pages: append([]string{}, head...)}
			byKey[k] = a
		}
		a.n++
		if len(seq) > steps {
			a.more++
		}
		if reached[cur] {
			a.reached++
		}
		seq = seq[:0]
	}
	for rows.Next() {
		var vid int64
		var path string
		if err := rows.Scan(&vid, &path); err != nil {
			return nil, 0, err
		}
		if vid != cur {
			done()
			cur = vid
		}
		if n := len(seq); n == 0 || seq[n-1] != path {
			seq = append(seq, path)
		}
	}
	done()
	out := make([]Journey, 0, len(byKey))
	for _, a := range byKey {
		out = append(out, Journey{Pages: a.pages, Visits: a.n, More: a.more, Converted: a.reached})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Visits != out[j].Visits {
			return out[i].Visits > out[j].Visits
		}
		if len(out[i].Pages) != len(out[j].Pages) {
			return len(out[i].Pages) > len(out[j].Pages)
		}
		return strings.Join(out[i].Pages, " ") < strings.Join(out[j].Pages, " ")
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, total, rows.Err()
}

// SectionSeen is how many page views had one part of the page on screen.
type SectionSeen struct {
	Name    string  `json:"name"`
	Reached int     `json:"reached"`
	Rate    float64 `json:"rate"` // percent of the page views that report parts
}

// Depth is how many page views got at least this far down (percent).
type Depth struct {
	At   int     `json:"at"`
	Rate float64 `json:"rate"`
}

// Reading is how far down one page people read.
type Reading struct {
	Page     string        `json:"page"`
	Views    int           `json:"views"`    // page views in the range
	Measured int           `json:"measured"` // of these, how many reported how far down they got
	Depth    []Depth       `json:"depth"`
	Marked   int           `json:"marked"` // page views that reported parts of the page
	Sections []SectionSeen `json:"sections"`
}

// PageReading is how far down a page (a path, * for any part) people read:
// by scroll depth, and by the parts the site marked with data-pocket-section.
func (s *Store) PageReading(ctx context.Context, q Query, page string) (*Reading, error) {
	out := &Reading{Page: page, Depth: []Depth{}, Sections: []SectionSeen{}}
	hw, ha := q.hitRange()
	pc, pa := PathMatch("h.path", page)
	rows, err := s.DB.QueryContext(ctx, `SELECT h.scroll, h.engaged, h.sections FROM hits h JOIN visits v ON v.id = h.visit_id
		WHERE h.name = 'pageview' AND `+pc+` AND `+hw, append([]any{pa}, ha...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	marks := []int{25, 50, 75, 100}
	depth := make([]int, len(marks))
	type sec struct {
		n   int
		pos float64 // sum of each time's place in the page, 0–1
	}
	secs := map[string]*sec{}
	for rows.Next() {
		var scroll int
		var engaged int64
		var sections string
		if err := rows.Scan(&scroll, &engaged, &sections); err != nil {
			return nil, err
		}
		out.Views++
		if scroll > 0 {
			out.Measured++
			for i, m := range marks {
				if scroll >= m {
					depth[i]++
				}
			}
		}
		var names []string
		if sections != "" && json.Unmarshal([]byte(sections), &names) == nil && len(names) > 0 {
			out.Marked++
			for i, n := range names {
				x := secs[n]
				if x == nil {
					x = &sec{}
					secs[n] = x
				}
				x.n++
				x.pos += float64(i+1) / float64(len(names)+1)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, m := range marks {
		d := Depth{At: m}
		if out.Measured > 0 {
			d.Rate = round1(float64(depth[i]) * 100 / float64(out.Measured))
		}
		out.Depth = append(out.Depth, d)
	}
	type ord struct {
		name string
		at   float64
	}
	var order []ord
	for n, x := range secs {
		order = append(order, ord{n, x.pos / float64(x.n)})
	}
	// page order: where each part usually sits among those seen; the ones
	// seen most come first when that is a tie
	sort.Slice(order, func(i, j int) bool {
		a, b := secs[order[i].name], secs[order[j].name]
		if a.n != b.n && (a.n*10 > b.n*13 || b.n*10 > a.n*13) {
			return a.n > b.n
		}
		if order[i].at != order[j].at {
			return order[i].at < order[j].at
		}
		return order[i].name < order[j].name
	})
	for _, o := range order {
		x := secs[o.name]
		out.Sections = append(out.Sections, SectionSeen{Name: o.name, Reached: x.n, Rate: round1(float64(x.n) * 100 / float64(out.Marked))})
	}
	return out, nil
}

// MarkedPages are the pages that report their parts, most viewed first.
func (s *Store) MarkedPages(ctx context.Context, siteID, from, to int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT path FROM hits WHERE site_id = ? AND ts >= ? AND ts < ? AND name = 'pageview' AND sections <> ''
		GROUP BY path ORDER BY COUNT(*) DESC LIMIT 30`, siteID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AIPage is one page: how often AI robots read it and how many visits AI
// assistants sent to it.
type AIPage struct {
	Page   string `json:"page"`
	Reads  int    `json:"reads"`  // by AI robots
	Robots int    `json:"robots"` // different AI robots
	Visits int    `json:"visits"` // people who came from an AI assistant and landed here
}

// AICount is a name and how many.
type AICount struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// AIView is what AI assistants do with a site: their robots read pages
// (to learn, to search, to answer someone) and people follow the answers.
type AIView struct {
	Reads   int       `json:"reads"`
	Visits  int       `json:"visits"`
	Pages   []AIPage  `json:"pages"`
	Robots  []AICount `json:"robots"`  // AI robots by reads
	Sources []AICount `json:"sources"` // AI assistants by visits sent
}

// AI is the AI picture of a site in a range (visits pass the filters; the
// robots have none).
func (s *Store) AI(ctx context.Context, q Query, channel string, limit int) (*AIView, error) {
	if limit <= 0 || limit > 200 {
		limit = 15
	}
	out := &AIView{Pages: []AIPage{}, Robots: []AICount{}, Sources: []AICount{}}
	pages := map[string]*AIPage{}
	get := func(p string) *AIPage {
		x := pages[p]
		if x == nil {
			x = &AIPage{Page: p}
			pages[p] = x
		}
		return x
	}
	rr := []any{q.SiteID, q.From / 3600 * 3600, q.To}
	rows, err := s.DB.QueryContext(ctx, `SELECT path, SUM(hits), COUNT(DISTINCT name) FROM robots WHERE site_id = ? AND ts >= ? AND ts < ? AND kind = 'ai' GROUP BY path`, rr...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p string
		var n, k int
		if err := rows.Scan(&p, &n, &k); err != nil {
			rows.Close()
			return nil, err
		}
		x := get(p)
		x.Reads, x.Robots = n, k
		out.Reads += n
	}
	rows.Close()
	w, a := q.visitRange()
	rows, err = s.DB.QueryContext(ctx, `SELECT v.entry, COUNT(*) FROM visits v WHERE v.channel = ? AND `+w+` GROUP BY v.entry`, append([]any{channel}, a...)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p string
		var n int
		if err := rows.Scan(&p, &n); err != nil {
			rows.Close()
			return nil, err
		}
		get(p).Visits = n
		out.Visits += n
	}
	rows.Close()
	counts := func(query string, args ...any) ([]AICount, error) {
		rows, err := s.DB.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		list := []AICount{}
		for rows.Next() {
			var c AICount
			if err := rows.Scan(&c.Name, &c.N); err != nil {
				return nil, err
			}
			list = append(list, c)
		}
		return list, rows.Err()
	}
	if out.Robots, err = counts(`SELECT name, SUM(hits) FROM robots WHERE site_id = ? AND ts >= ? AND ts < ? AND kind = 'ai' GROUP BY name ORDER BY 2 DESC, 1 LIMIT 20`, rr...); err != nil {
		return nil, err
	}
	if out.Sources, err = counts(`SELECT v.source, COUNT(*) FROM visits v WHERE v.channel = ? AND `+w+` GROUP BY v.source ORDER BY 2 DESC, 1 LIMIT 20`, append([]any{channel}, a...)...); err != nil {
		return nil, err
	}
	for _, x := range pages {
		out.Pages = append(out.Pages, *x)
	}
	sort.Slice(out.Pages, func(i, j int) bool {
		a, b := out.Pages[i], out.Pages[j]
		if a.Visits != b.Visits {
			return a.Visits > b.Visits
		}
		if a.Reads != b.Reads {
			return a.Reads > b.Reads
		}
		return a.Page < b.Page
	})
	if len(out.Pages) > limit {
		out.Pages = out.Pages[:limit]
	}
	return out, nil
}

// EventPages is where an event happened: by page, most visitors first.
func (s *Store) EventPages(ctx context.Context, q Query, event string, limit int) ([]Row, error) {
	if limit <= 0 || limit > 200 {
		limit = 10
	}
	hw, ha := q.hitRange()
	rows, err := s.DB.QueryContext(ctx, `SELECT h.path, COUNT(DISTINCT v.visitor), COUNT(*) FROM hits h JOIN visits v ON v.id = h.visit_id
		WHERE h.name = ? AND `+hw+` GROUP BY h.path ORDER BY 2 DESC, 3 DESC, 1 LIMIT ?`, append(append([]any{event}, ha...), limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Row{}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Value, &r.Visitors, &r.Events); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SearchRow is one thing people searched for on the site.
type SearchRow struct {
	Term     string `json:"term"`
	Visitors int    `json:"visitors"`
	Times    int    `json:"times"`
	Results  *int   `json:"results"` // the fewest results it got (nil = the site did not say)
	Where    string `json:"where,omitempty"`
}

// Searches lists what people searched for, one row per words and search box
// (where); empty = only the searches that found nothing.
func (s *Store) Searches(ctx context.Context, q Query, empty bool, limit int) ([]SearchRow, int, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	hw, ha := q.hitRange()
	cond := ""
	if empty {
		cond = " AND json_extract(h.props, '$.results') = '0'"
	}
	var none int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM hits h JOIN visits v ON v.id = h.visit_id WHERE h.name = 'Search' AND json_extract(h.props, '$.results') = '0' AND `+hw, ha...).Scan(&none); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT COALESCE(NULLIF(json_extract(h.props, '$.term'), ''), '(none)'), COUNT(DISTINCT v.visitor), COUNT(*),
		MIN(CAST(json_extract(h.props, '$.results') AS INTEGER)), COALESCE(json_extract(h.props, '$.where'), '')
		FROM hits h JOIN visits v ON v.id = h.visit_id WHERE h.name = 'Search'`+cond+` AND `+hw+` GROUP BY 1, 5 ORDER BY 2 DESC, 3 DESC, 1 LIMIT ?`, append(ha, limit)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []SearchRow{}
	for rows.Next() {
		var r SearchRow
		if err := rows.Scan(&r.Term, &r.Visitors, &r.Times, &r.Results, &r.Where); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, none, rows.Err()
}
