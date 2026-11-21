package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/refs"
	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- how people use the site ----

// goalsFor are the goals a report counts as "got somewhere": one goal
// (?goal=id), else all of them.
func (s *Server) goalsFor(r *http.Request, site *store.Site) []store.Goal {
	goals, _ := s.store.ListGoals(r.Context(), site.UserID, site.ID)
	if id := queryInt(r, "goal"); id > 0 {
		for _, g := range goals {
			if g.ID == int64(id) {
				return []store.Goal{g}
			}
		}
	}
	return goals
}

// handleLanding is the pages visits start on and how each went.
func (s *Server) handleLanding(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	rows, hasGoals, err := s.store.LandingPages(r.Context(), q, s.goalsFor(r, site), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "landing pages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "goals": hasGoals})
}

// handleJourneys is the ways people take through the site: the first pages of each visit.
func (s *Server) handleJourneys(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	rows, total, err := s.store.Journeys(r.Context(), q, s.goalsFor(r, site), queryInt(r, "steps"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "journeys")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "visits": total})
}

// handleReading is how far down a page people read (?page=/; without one,
// the page most viewed among those that mark their parts, else "/").
func (s *Server) handleReading(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q, _ := s.queryFor(r, site)
	marked, err := s.store.MarkedPages(ctx, site.ID, q.From, q.To)
	if err != nil {
		s.fail(w, err, "reading")
		return
	}
	page := clip(strings.TrimSpace(r.URL.Query().Get("page")), 500)
	if page == "" {
		page = "/"
		if len(marked) > 0 {
			page = marked[0]
		}
	}
	rd, err := s.store.PageReading(ctx, q, page)
	if err != nil {
		s.fail(w, err, "reading")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reading": rd, "marked": marked})
}

// handleAI is what AI assistants do with the site: pages their robots read
// and the visits their answers sent.
func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	v, err := s.store.AI(r.Context(), q, refs.AI, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "ai")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// handleEventPages is the pages an event happened on (?event=…).
func (s *Server) handleEventPages(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ev := clip(strings.TrimSpace(r.URL.Query().Get("event")), 120)
	if ev == "" {
		writeErr(w, http.StatusBadRequest, "say which event (?event=…)")
		return
	}
	q, _ := s.queryFor(r, site)
	rows, err := s.store.EventPages(r.Context(), q, ev, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "event pages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// handleSearches is what people searched for on the site (?empty=1: only
// what found nothing).
func (s *Server) handleSearches(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	rows, none, err := s.store.Searches(r.Context(), q, r.URL.Query().Get("empty") == "1", queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "searches")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "nothing_found": none})
}

// handleAddress tells a browser the address this server sees it on, so a
// person can add their own to a site's "don't count" list. Public; nothing
// is kept.
func (s *Server) handleAddress(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-store")
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	writeJSON(w, http.StatusOK, map[string]string{"ip": ip})
}
