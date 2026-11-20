package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- export and import: sites, goals and funnels (not the visits) ----

type exportSite struct {
	Domain  string            `json:"domain"`
	Name    string            `json:"name"`
	TZ      string            `json:"tz"`
	Also    []string          `json:"also"`
	Options store.SiteOptions `json:"options"`
	Goals   []store.Goal      `json:"goals"`
	Funnels []*store.Funnel   `json:"funnels"`
}

type exportDoc struct {
	App      string       `json:"app"`
	Version  string       `json:"version"`
	Exported string       `json:"exported"`
	Sites    []exportSite `json:"sites"`
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	sites, err := s.store.ListSites(ctx, uid)
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	doc := exportDoc{App: "pocket-analytics", Version: Version, Exported: store.Now(), Sites: []exportSite{}}
	for _, st := range sites {
		g, _ := s.store.ListGoals(ctx, uid, st.ID)
		f, _ := s.store.ListFunnels(ctx, uid, st.ID)
		doc.Sites = append(doc.Sites, exportSite{Domain: st.Domain, Name: st.Name, TZ: st.TZ, Also: st.Also, Options: st.Options, Goals: g, Funnels: f})
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="pocket-analytics-%s.json"`, time.Now().Format("2006-01-02")))
	writeJSON(w, http.StatusOK, doc)
}

// handleImport brings sites, goals and funnels back from an export. A site
// already there keeps its settings; goals and funnels with the same name
// are not added twice.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "too large")
		return
	}
	var doc exportDoc
	if err := json.Unmarshal(b, &doc); err != nil || doc.App != "pocket-analytics" {
		writeErr(w, http.StatusBadRequest, "that is not a Pocket Analytics export")
		return
	}
	res := map[string]int{"sites": 0, "goals": 0, "funnels": 0, "skipped": 0}
	for _, es := range doc.Sites {
		domain, ok := cleanDomain(es.Domain)
		if !ok {
			res["skipped"]++
			continue
		}
		site, err := s.store.SiteByDomain(ctx, domain)
		switch {
		case err == store.ErrNotFound:
			site = &store.Site{UserID: uid, Domain: domain, Name: clip(es.Name, 80), TZ: es.TZ, Also: es.Also, Options: es.Options, ProxyKey: store.RandomToken(24)}
			if site.Also == nil {
				site.Also = []string{}
			}
			if err := s.store.CreateSite(ctx, site); err != nil {
				res["skipped"]++
				continue
			}
			res["sites"]++
		case err != nil:
			s.fail(w, err, "import")
			return
		case site.UserID != uid:
			res["skipped"]++ // someone else counts it on this box
			continue
		}
		goals, _ := s.store.ListGoals(ctx, uid, site.ID)
		have := map[string]bool{}
		for _, g := range goals {
			have[strings.ToLower(g.Name)] = true
		}
		for _, g := range es.Goals {
			ng, msg := goalIn{Name: g.Name, Kind: g.Kind, Match: g.Match}.check()
			if msg != "" || have[strings.ToLower(ng.Name)] {
				continue
			}
			ng.SiteID = site.ID
			if s.store.CreateGoal(ctx, uid, &ng) == nil {
				res["goals"]++
				have[strings.ToLower(ng.Name)] = true
			}
		}
		funnels, _ := s.store.ListFunnels(ctx, uid, site.ID)
		haveF := map[string]bool{}
		for _, f := range funnels {
			haveF[strings.ToLower(f.Name)] = true
		}
		for _, f := range es.Funnels {
			if f == nil {
				continue
			}
			nf, msg := funnelIn{Name: f.Name, Steps: f.Steps}.check()
			if msg != "" || haveF[strings.ToLower(nf.Name)] {
				continue
			}
			nf.SiteID = site.ID
			if s.store.CreateFunnel(ctx, uid, &nf) == nil {
				res["funnels"]++
				haveF[strings.ToLower(nf.Name)] = true
			}
		}
	}
	s.reloadSites(ctx)
	writeJSON(w, http.StatusOK, res)
}
