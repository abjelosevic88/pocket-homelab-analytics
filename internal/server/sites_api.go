package server

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

var reDomain = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$|^localhost$`)

// cleanDomain turns what someone typed ("https://www.Example.com/blog") into example.com.
func cleanDomain(v string) (string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.Index(v, "://"); i >= 0 {
		v = v[i+3:]
	}
	v = strings.SplitN(v, "/", 2)[0]
	v = strings.SplitN(v, "?", 2)[0]
	if h, _, ok := strings.Cut(v, ":"); ok {
		v = h
	}
	v = strings.TrimPrefix(strings.TrimSuffix(v, "."), "www.")
	return v, reDomain.MatchString(v)
}

// zoneOf is a site's time zone (the person's, else the box's, when unset).
func (s *Server) zoneOf(ctx context.Context, site *store.Site) *time.Location {
	if site.TZ != "" {
		if loc, err := time.LoadLocation(site.TZ); err == nil {
			return loc
		}
	}
	st, _ := s.store.GetSettings(ctx, site.UserID)
	return zone(st)
}

// zone is a person's time zone.
func zone(st store.Settings) *time.Location {
	if st.TZ != "" {
		if loc, err := time.LoadLocation(st.TZ); err == nil {
			return loc
		}
	}
	return time.Local
}

// mySite is the site in the path when it is the caller's (else 404).
func (s *Server) mySite(w http.ResponseWriter, r *http.Request) (*store.Site, bool) {
	site, err := s.store.GetSite(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "site")
		return nil, false
	}
	return site, true
}

// siteView is a site as the app shows it: with the snippet and the addresses.
func (s *Server) siteView(ctx context.Context, site *store.Site) map[string]any {
	base := s.collectFor(ctx)
	script := base + "/js/script.js"
	return map[string]any{
		"id": site.ID, "domain": site.Domain, "name": site.Name, "label": site.Label(), "tz": s.zoneOf(ctx, site).String(), "tz_set": site.TZ,
		"also": site.Also, "options": site.Options, "proxy_key": site.ProxyKey, "position": site.Position,
		"first_hit": site.FirstHit, "last_hit": site.LastHit, "installed": site.LastHit > 0, "created_at": site.CreatedAt,
		"script_url": script, "event_url": base + "/api/event", "snippet": snippet(site, script, ""),
		"ignore_url": "https://" + site.Domain + "/?pocket_ignore=1",
	}
}

// snippet is the line to put in a site's <head>.
func snippet(site *store.Site, script, api string) string {
	attrs := []string{`defer`, `data-domain="` + html.EscapeString(site.Domain) + `"`}
	if api != "" {
		attrs = append(attrs, `data-api="`+html.EscapeString(api)+`"`)
	}
	if site.Options.Hash {
		attrs = append(attrs, "data-hash")
	}
	if site.Options.Clicks {
		attrs = append(attrs, "data-clicks")
	}
	if !site.Options.Outbound {
		attrs = append(attrs, "data-no-outbound")
	}
	if !site.Options.Downloads {
		attrs = append(attrs, "data-no-downloads")
	}
	return fmt.Sprintf(`<script %s src="%s"></script>`, strings.Join(attrs, " "), html.EscapeString(script))
}

// ---- sites ----

func (s *Server) handleListSites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sites, err := s.store.ListSites(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "sites")
		return
	}
	out := make([]map[string]any, 0, len(sites))
	now := time.Now()
	for _, site := range sites {
		v := s.siteView(ctx, site)
		loc := s.zoneOf(ctx, site)
		today := dayStart(now.In(loc))
		bounds := make([]int64, 0, 31)
		for i := 29; i >= 0; i-- {
			bounds = append(bounds, today.AddDate(0, 0, -i).Unix())
		}
		bounds = append(bounds, today.AddDate(0, 0, 1).Unix())
		pts, _ := s.store.Series(ctx, store.Query{SiteID: site.ID, From: bounds[0], To: bounds[len(bounds)-1]}, bounds)
		spark := make([]int, len(pts))
		for i, p := range pts {
			spark[i] = p.Visitors
		}
		v["spark"] = spark
		v["today"] = s.store.VisitorsIn(ctx, site.ID, today.Unix(), today.AddDate(0, 0, 1).Unix())
		v["live"] = s.store.LiveNow(ctx, site.ID, now.Unix()-300)
		t30, _ := s.store.Totals(ctx, store.Query{SiteID: site.ID, From: today.AddDate(0, 0, -29).Unix(), To: today.AddDate(0, 0, 1).Unix()})
		p30, _ := s.store.Totals(ctx, store.Query{SiteID: site.ID, From: today.AddDate(0, 0, -59).Unix(), To: today.AddDate(0, 0, -29).Unix()})
		v["visitors_30d"], v["visitors_prev_30d"] = t30.Visitors, p30.Visitors
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"sites": out, "collect_url": s.collectFor(ctx), "geo": s.geo.State()})
}

type siteIn struct {
	Domain  *string            `json:"domain"`
	Name    *string            `json:"name"`
	TZ      *string            `json:"tz"`
	Also    *[]string          `json:"also"`
	Options *store.SiteOptions `json:"options"`
}

// apply checks and copies what was sent onto a site.
func (in siteIn) apply(site *store.Site) string {
	if in.Domain != nil {
		d, ok := cleanDomain(*in.Domain)
		if !ok {
			return "that does not look like a website address (example.com)"
		}
		site.Domain = d
	}
	if in.Name != nil {
		site.Name = clip(strings.TrimSpace(*in.Name), 80)
	}
	if in.TZ != nil {
		tz := strings.TrimSpace(*in.TZ)
		if tz != "" {
			if _, err := time.LoadLocation(tz); err != nil {
				return "unknown time zone"
			}
		}
		site.TZ = tz
	}
	if in.Also != nil {
		site.Also = []string{}
		for _, a := range *in.Also {
			if strings.TrimSpace(a) == "" {
				continue
			}
			d, ok := cleanDomain(a)
			if !ok {
				return "“" + clip(a, 60) + "” does not look like a website address"
			}
			if d != site.Domain && len(site.Also) < 20 {
				site.Also = append(site.Also, d)
			}
		}
	}
	if in.Options != nil {
		o := *in.Options
		ex := []string{}
		for _, p := range o.Exclude {
			if p = strings.TrimSpace(p); p != "" && len(ex) < 50 {
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
				ex = append(ex, clip(p, 200))
			}
		}
		o.Exclude = ex
		if o.KeepDays < 0 || o.KeepDays > 36500 {
			return "keep visits for 0 (always) to 36500 days"
		}
		site.Options = o
	}
	return ""
}

func (s *Server) handleCreateSite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in siteIn
	if err := decode(r, &in); err != nil || in.Domain == nil {
		writeErr(w, http.StatusBadRequest, "the website address is needed")
		return
	}
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	site := &store.Site{UserID: actorID(ctx), Options: store.DefaultOptions(), Also: []string{}, ProxyKey: store.RandomToken(24), TZ: st.TZ}
	if msg := in.apply(site); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.CreateSite(ctx, site); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, site.Domain+" is already counted on this box")
			return
		}
		s.fail(w, err, "add site")
		return
	}
	s.reloadSites(ctx)
	writeJSON(w, http.StatusCreated, s.siteView(ctx, site))
}

func (s *Server) handleGetSite(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	v := s.siteView(r.Context(), site)
	goals, _ := s.store.ListGoals(r.Context(), site.UserID, site.ID)
	funnels, _ := s.store.ListFunnels(r.Context(), site.UserID, site.ID)
	v["goals"], v["funnels"] = goals, funnels
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleUpdateSite(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	var in siteIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(site); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateSite(r.Context(), site); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, site.Domain+" is already counted on this box")
			return
		}
		s.fail(w, err, "save site")
		return
	}
	s.reloadSites(r.Context())
	writeJSON(w, http.StatusOK, s.siteView(r.Context(), site))
}

func (s *Server) handleDeleteSite(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteSite(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete site")
		return
	}
	s.reloadSites(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetSite(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ResetSite(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "reset site")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleNewProxyKey(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	site.ProxyKey = store.RandomToken(24)
	if err := s.store.UpdateSite(r.Context(), site); err != nil {
		s.fail(w, err, "proxy key")
		return
	}
	s.reloadSites(r.Context())
	writeJSON(w, http.StatusOK, s.siteView(r.Context(), site))
}

func (s *Server) handleOrderSites(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.OrderSites(r.Context(), actorID(r.Context()), in.IDs); err != nil {
		s.fail(w, err, "order")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCheckSite says whether hits arrive (the install check).
func (s *Server) handleCheckSite(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	s.waitFlushed()
	site, _ = s.store.GetSite(r.Context(), site.UserID, site.ID)
	writeJSON(w, http.StatusOK, map[string]any{"installed": site.LastHit > 0, "last_hit": site.LastHit})
}

// ---- goals ----

type goalIn struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Match string `json:"match"`
}

func (in goalIn) check() (store.Goal, string) {
	g := store.Goal{Name: clip(strings.TrimSpace(in.Name), 80), Kind: in.Kind, Match: clip(strings.TrimSpace(in.Match), 300)}
	if g.Kind != "page" && g.Kind != "event" {
		return g, "a goal is a page or an event"
	}
	if g.Match == "" {
		return g, "say which page or event"
	}
	if g.Kind == "page" && !strings.HasPrefix(g.Match, "/") {
		g.Match = "/" + g.Match
	}
	if g.Name == "" {
		g.Name = g.Match
		if g.Kind == "page" {
			g.Name = "Visited " + g.Match
		}
	}
	return g, ""
}

func (s *Server) handleCreateGoal(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	var in goalIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	g, msg := in.check()
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	g.SiteID = site.ID
	if err := s.store.CreateGoal(r.Context(), site.UserID, &g); err != nil {
		s.fail(w, err, "goal")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) handleUpdateGoal(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	cur, err := s.store.GetGoal(r.Context(), uid, idParam(r, "gid"))
	if err != nil {
		s.fail(w, err, "goal")
		return
	}
	var in goalIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	g, msg := in.check()
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	g.ID, g.SiteID = cur.ID, cur.SiteID
	if err := s.store.UpdateGoal(r.Context(), uid, &g); err != nil {
		s.fail(w, err, "goal")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleDeleteGoal(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteGoal(r.Context(), actorID(r.Context()), idParam(r, "gid")); err != nil {
		s.fail(w, err, "goal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- funnels ----

type funnelIn struct {
	Name  string       `json:"name"`
	Steps []store.Step `json:"steps"`
}

func (in funnelIn) check() (store.Funnel, string) {
	f := store.Funnel{Name: clip(strings.TrimSpace(in.Name), 80), Steps: []store.Step{}}
	for _, st := range in.Steps {
		g, msg := goalIn{Name: st.Label, Kind: st.Kind, Match: st.Match}.check()
		if msg != "" {
			return f, msg
		}
		label := clip(strings.TrimSpace(st.Label), 80)
		f.Steps = append(f.Steps, store.Step{Kind: g.Kind, Match: g.Match, Label: label})
	}
	if len(f.Steps) < 2 || len(f.Steps) > 8 {
		return f, "a funnel has 2 to 8 steps"
	}
	if f.Name == "" {
		return f, "give the funnel a name"
	}
	return f, ""
}

func (s *Server) handleCreateFunnel(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	var in funnelIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f, msg := in.check()
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	f.SiteID = site.ID
	if err := s.store.CreateFunnel(r.Context(), site.UserID, &f); err != nil {
		s.fail(w, err, "funnel")
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (s *Server) handleUpdateFunnel(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	cur, err := s.store.GetFunnel(r.Context(), uid, idParam(r, "fid"))
	if err != nil {
		s.fail(w, err, "funnel")
		return
	}
	var in funnelIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	f, msg := in.check()
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	f.ID, f.SiteID = cur.ID, cur.SiteID
	if err := s.store.UpdateFunnel(r.Context(), uid, &f); err != nil {
		s.fail(w, err, "funnel")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleDeleteFunnel(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteFunnel(r.Context(), actorID(r.Context()), idParam(r, "fid")); err != nil {
		s.fail(w, err, "funnel")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func dayStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
