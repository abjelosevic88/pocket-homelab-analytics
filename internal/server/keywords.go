package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/gsc"
	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- the words people searched on Google (Search Console) ----
//
// Google does not tell a website which words led to a visit. It tells the
// site's owner, in Search Console. With the person's service account key
// (Settings → Google Search) the words show next to the other sources.

// keywordsOut is what a site's "Search words" list shows.
type keywordsOut struct {
	State    string    `json:"state"` // ok | off | no_property | error
	By       string    `json:"by"`    // query (the words) | page (the pages Google sent people to)
	Error    string    `json:"error,omitempty"`
	Email    string    `json:"email,omitempty"`    // the key's address, to add in Search Console
	Property string    `json:"property,omitempty"` // the Search Console property read
	Seen     []string  `json:"seen,omitempty"`     // what the key can read, when the site is not among it
	From     string    `json:"from,omitempty"`
	To       string    `json:"to,omitempty"`
	Rows     []gsc.Row `json:"rows"`
	Totals   *gsc.Row  `json:"totals,omitempty"`
	Partial  bool      `json:"partial,omitempty"` // filters Google cannot follow were left out
	Demo     bool      `json:"demo,omitempty"`
}

type kwEntry struct {
	out   keywordsOut
	props []string
	until time.Time
}

func (s *Server) kwGet(key string) (kwEntry, bool) {
	s.kwMu.Lock()
	defer s.kwMu.Unlock()
	e, ok := s.kwCache[key]
	return e, ok && time.Now().Before(e.until)
}

func (s *Server) kwPut(key string, e kwEntry, ttl time.Duration) {
	s.kwMu.Lock()
	defer s.kwMu.Unlock()
	if len(s.kwCache) > 500 {
		s.kwCache = map[string]kwEntry{}
	}
	e.until = time.Now().Add(ttl)
	s.kwCache[key] = e
}

// kwForget drops what was kept for a person's key (after it changes).
func (s *Server) kwForget() {
	s.kwMu.Lock()
	s.kwCache = map[string]kwEntry{}
	s.kwMu.Unlock()
}

// properties are what a key can read (kept ten minutes).
func (s *Server) properties(ctx context.Context, k *gsc.Key) ([]string, error) {
	if e, ok := s.kwGet("props:" + k.Email); ok {
		return e.props, nil
	}
	props, err := s.gsc.Properties(ctx, k)
	if err != nil {
		return nil, err
	}
	s.kwPut("props:"+k.Email, kwEntry{props: props}, 10*time.Minute)
	return props, nil
}

// pageRE turns a path (with * for any part) into what Google matches page
// addresses with: that path on any host of the property.
func pageRE(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "**", "*"), "*")
	for i, p := range parts {
		parts[i] = regexp.QuoteMeta(p)
	}
	re := strings.TrimSuffix(strings.Join(parts, ".*"), "/")
	return `^https?://[^/]+` + re + `/?(\?.*)?(#.*)?$`
}

var gscDevice = map[string]string{"Phone": "MOBILE", "Tablet": "TABLET", "Laptop": "DESKTOP", "Desktop": "DESKTOP"}

// keywords reads a site's search words (by = "query") or the pages Google
// sent people to (by = "page") for a range and its filters.
func (s *Server) keywords(ctx context.Context, site *store.Site, q store.Query, sp span, by string, limit int) keywordsOut {
	if by != "page" {
		by = "query"
	}
	out := keywordsOut{State: "off", By: by, Rows: []gsc.Row{}}
	st, _ := s.store.GetSettings(ctx, site.UserID)
	if st.GoogleKey == "" {
		if s.cfg.DemoData {
			return demoKeywords(site, sp, by, limit)
		}
		return out
	}
	key, err := gsc.ParseKey(st.GoogleKey)
	if err != nil {
		out.State, out.Error = "error", err.Error()
		return out
	}
	out.Email = key.Email
	props, err := s.properties(ctx, key)
	if err != nil {
		out.State, out.Error = "error", err.Error()
		return out
	}
	prop := ""
	for _, d := range append([]string{site.Domain}, site.Also...) {
		if prop = gsc.PropertyFor(d, props); prop != "" {
			break
		}
	}
	if prop == "" {
		out.State, out.Seen = "no_property", props
		return out
	}
	// Google keeps 16 months and knows nothing of today yet
	today := time.Now().In(sp.From.Location())
	from, to := sp.From, sp.To.AddDate(0, 0, -1)
	if to.After(today) {
		to = today
	}
	if oldest := today.AddDate(0, -16, 1); from.Before(oldest) {
		from = oldest
	}
	if from.After(to) {
		from = to
	}
	gq := gsc.Query{From: from.Format("2006-01-02"), To: to.Format("2006-01-02"), By: by, Limit: limit}
	for _, f := range q.Filters {
		switch {
		case f.Dim == "page" && !f.Not:
			gq.PageRE = pageRE(f.Value)
		case f.Dim == "device" && !f.Not && gscDevice[f.Value] != "":
			gq.Device = gscDevice[f.Value]
		case (f.Dim == "source" && f.Value == "Google" || f.Dim == "channel" && f.Value == "Search") && !f.Not:
			// the same people
		default:
			out.Partial = true
		}
	}
	out.State, out.Property, out.From, out.To = "ok", prop, gq.From, gq.To
	ck := fmt.Sprintf("kw:%s|%s|%s|%s|%s|%s|%s|%d", key.Email, prop, by, gq.From, gq.To, gq.PageRE, gq.Device, gq.Limit)
	if e, ok := s.kwGet(ck); ok {
		e.out.Partial = out.Partial
		return e.out
	}
	rows, err := s.gsc.Search(ctx, key, prop, gq)
	if err != nil {
		out.State, out.Error = "error", err.Error()
		return out
	}
	if by == "page" {
		// Google gives whole addresses; the pages here are paths
		for i := range rows {
			if u, err := url.Parse(rows[i].Words); err == nil && u.Host != "" {
				rows[i].Words = cleanPath(u)
			}
		}
	}
	out.Rows = rows
	tq := gq
	tq.By = ""
	if tot, err := s.gsc.Search(ctx, key, prop, tq); err == nil && len(tot) > 0 {
		out.Totals = &tot[0]
	}
	s.kwPut(ck, kwEntry{out: out}, 30*time.Minute)
	return out
}

// handleKeywords is the words people searched on Google before they came.
func (s *Server) handleKeywords(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, sp := s.queryFor(r, site)
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 500 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.keywords(ctx, site, q, sp, r.URL.Query().Get("by"), limit))
}

// handleGoogleTest tries a key (the one typed, else the saved one) and says
// which of the person's sites it can read.
func (s *Server) handleGoogleTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Key string `json:"key"`
	}
	_ = decode(r, &in)
	raw := strings.TrimSpace(in.Key)
	if raw == "" || strings.Contains(raw, "••") {
		st, _ := s.store.GetSettings(ctx, uid)
		raw = st.GoogleKey
	}
	if raw == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "paste the key file first"})
		return
	}
	key, err := gsc.ParseKey(raw)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	props, err := s.gsc.Properties(cctx, key)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "email": key.Email, "error": err.Error()})
		return
	}
	sites, _ := s.store.ListSites(ctx, uid)
	found := []map[string]any{}
	for _, site := range sites {
		prop := ""
		for _, d := range append([]string{site.Domain}, site.Also...) {
			if prop = gsc.PropertyFor(d, props); prop != "" {
				break
			}
		}
		found = append(found, map[string]any{"site": site.Label(), "domain": site.Domain, "property": prop})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "email": key.Email, "properties": props, "sites": found})
}

// googleEmail is the address of a saved key ("" when none or unreadable).
func googleEmail(raw string) string {
	if raw == "" {
		return ""
	}
	if k, err := gsc.ParseKey(raw); err == nil {
		return k.Email
	}
	return ""
}

// demoKeywords are made-up search words for the demo sites.
func demoKeywords(site *store.Site, sp span, by string, limit int) keywordsOut {
	out := keywordsOut{State: "off", By: by, Rows: []gsc.Row{}}
	var words []gsc.Row
	switch {
	case site.Domain == "maplebakery.example" && by == "page":
		words = []gsc.Row{{Words: "/", Clicks: 590, Impressions: 6100, Position: 2.9}, {Words: "/cakes/birthday", Clicks: 201, Impressions: 2600, Position: 3.1},
			{Words: "/blog/best-burek-in-town", Clicks: 139, Impressions: 5900, Position: 6.0}, {Words: "/blog/sourdough-at-home", Clicks: 118, Impressions: 9400, Position: 9.2},
			{Words: "/menu", Clicks: 104, Impressions: 480, Position: 1.4}, {Words: "/cakes/wedding", Clicks: 47, Impressions: 2200, Position: 7.6}, {Words: "/catering", Clicks: 28, Impressions: 1500, Position: 10.8},
			{Words: "/visit-us", Clicks: 16, Impressions: 300, Position: 2.2}}
	case site.Domain == "fieldnotes.example" && by == "page":
		words = []gsc.Row{{Words: "/posts/sqlite-is-enough", Clicks: 149, Impressions: 2100, Position: 2.4}, {Words: "/posts/self-hosting-on-a-raspberry-pi", Clicks: 131, Impressions: 7000, Position: 5.3},
			{Words: "/posts/one-binary-apps", Clicks: 70, Impressions: 1700, Position: 4.1}, {Words: "/posts/backups-that-work", Clicks: 41, Impressions: 800, Position: 3.0}, {Words: "/", Clicks: 24, Impressions: 90, Position: 1.2}}
	case site.Domain == "maplebakery.example":
		words = []gsc.Row{{Words: "maple bakery", Clicks: 412, Impressions: 760, Position: 1.1}, {Words: "bakery sarajevo", Clicks: 238, Impressions: 3900, Position: 3.4},
			{Words: "birthday cake sarajevo", Clicks: 171, Impressions: 2100, Position: 2.8}, {Words: "best burek in sarajevo", Clicks: 124, Impressions: 5200, Position: 6.2},
			{Words: "sourdough recipe at home", Clicks: 96, Impressions: 8100, Position: 9.7}, {Words: "maple bakery menu", Clicks: 88, Impressions: 140, Position: 1.0},
			{Words: "wedding cake prices", Clicks: 41, Impressions: 1900, Position: 7.9}, {Words: "gluten free bakery near me", Clicks: 37, Impressions: 950, Position: 4.6},
			{Words: "catering sarajevo", Clicks: 22, Impressions: 1300, Position: 11.3}, {Words: "baklava order online", Clicks: 14, Impressions: 610, Position: 8.8}}
	case site.Domain == "fieldnotes.example":
		words = []gsc.Row{{Words: "sqlite is enough", Clicks: 133, Impressions: 900, Position: 1.6}, {Words: "self hosting raspberry pi", Clicks: 121, Impressions: 6400, Position: 5.1},
			{Words: "one binary web app go", Clicks: 64, Impressions: 1500, Position: 3.9}, {Words: "backups that work", Clicks: 38, Impressions: 700, Position: 2.7},
			{Words: "tailscale home server", Clicks: 31, Impressions: 4800, Position: 12.4}, {Words: "field notes blog", Clicks: 19, Impressions: 60, Position: 1.0}}
	default:
		return out
	}
	days := sp.To.Sub(sp.From).Hours() / 24
	scale := days / 30
	tot := gsc.Row{}
	for i := range words {
		w := &words[i]
		w.Clicks, w.Impressions = float64(int(w.Clicks*scale+0.5)), float64(int(w.Impressions*scale+0.5))
		if w.Impressions > 0 {
			w.CTR = float64(int(w.Clicks*1000/w.Impressions+0.5)) / 10
		}
		tot.Clicks += w.Clicks
		tot.Impressions += w.Impressions
	}
	tot.Clicks, tot.Impressions = float64(int(tot.Clicks*1.35)), float64(int(tot.Impressions*1.6)) // Google leaves rare searches out of the list
	tot.CTR, tot.Position = float64(int(tot.Clicks*1000/max(tot.Impressions, 1)+0.5))/10, 6.4
	if limit > 0 && len(words) > limit {
		words = words[:limit]
	}
	out.State, out.Demo, out.Rows, out.Totals = "ok", true, words, &tot
	out.Property, out.From, out.To = "sc-domain:"+site.Domain, sp.FromDay, sp.ToDay
	return out
}
