package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- how fast the pages are ----

// handleSpeed is how fast the site was for the people who came: over all
// measured page loads, and by page, device, browser, system or country.
func (s *Server) handleSpeed(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	q, _ := s.queryFor(r, site)
	dim := r.URL.Query().Get("dim")
	if !store.ValidSpeedDim(dim) {
		dim = "page"
	}
	all, rows, err := s.store.Speed(r.Context(), q, dim, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "speed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dim": dim, "all": all, "rows": rows, "on": site.Options.Speed})
}

// ---- when they come ----

// handleWhen is visits by day of the week and hour, in the site's time
// zone: cells[weekday][hour], weekday 0 = Sunday.
func (s *Server) handleWhen(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q, _ := s.queryFor(r, site)
	starts, err := s.store.VisitStarts(ctx, q)
	if err != nil {
		s.fail(w, err, "when")
		return
	}
	loc := s.zoneOf(ctx, site)
	var cells [7][24]int
	top, total := 0, 0
	for _, ts := range starts {
		t := time.Unix(ts, 0).In(loc)
		c := &cells[int(t.Weekday())][t.Hour()]
		*c++
		total++
		if *c > top {
			top = *c
		}
	}
	st, _ := s.store.GetSettings(ctx, site.UserID)
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells, "max": top, "visits": total, "week_start": st.WeekStart, "tz": loc.String()})
}

// ---- the way through a page ----

// handlePaths is where people were before a page and where they went next.
func (s *Server) handlePaths(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	page := clip(strings.TrimSpace(r.URL.Query().Get("page")), 500)
	if page == "" {
		writeErr(w, http.StatusBadRequest, "say which page (?page=/menu)")
		return
	}
	q, _ := s.queryFor(r, site)
	p, err := s.store.PagePaths(r.Context(), q, page, queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "paths")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---- notes on the chart ----

type noteIn struct {
	Day  string `json:"day"`
	Text string `json:"text"`
}

func (in noteIn) check() (store.Note, string) {
	n := store.Note{Day: strings.TrimSpace(in.Day), Text: clip(strings.TrimSpace(in.Text), 200)}
	if !validDate(n.Day) {
		return n, "the day is YYYY-MM-DD"
	}
	if n.Text == "" {
		return n, "say what happened that day"
	}
	return n, ""
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	notes, err := s.store.ListNotes(r.Context(), site.UserID, site.ID, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.fail(w, err, "notes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	var in noteIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	n, msg := in.check()
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	n.SiteID = site.ID
	if err := s.store.CreateNote(r.Context(), site.UserID, &n); err != nil {
		s.fail(w, err, "note")
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	uid := actorID(r.Context())
	cur, err := s.store.GetNote(r.Context(), uid, idParam(r, "nid"))
	if err != nil {
		s.fail(w, err, "note")
		return
	}
	var in noteIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	n, msg := in.check()
	if msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	n.ID, n.SiteID, n.CreatedAt = cur.ID, cur.SiteID, cur.CreatedAt
	if err := s.store.UpdateNote(r.Context(), uid, &n); err != nil {
		s.fail(w, err, "note")
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteNote(r.Context(), actorID(r.Context()), idParam(r, "nid")); err != nil {
		s.fail(w, err, "note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
