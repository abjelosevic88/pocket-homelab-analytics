package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
	"github.com/abjelosevic88/pocket-homelab-analytics/internal/ua"
)

// ---- the robots that read a site ----
//
// Search engines, AI assistants and link previews fetch pages without
// running the script, so the script cannot see them. A site's own server
// can: a proxy that proves itself with the site's proxy key (the Cloudflare
// Worker from Install) posts each robot request to /api/robot. The few
// robots that do run the script are counted from there. A robot is never a
// visitor; only how often each read which page is kept, by the hour.

type robotKey struct {
	site       int64
	ts         int64
	name, kind string
	path       string
}

// agentKey is a robot without a name on a site; agentSeen is how often and where.
type agentKey struct {
	site  int64
	agent string
}

type agentSeen struct {
	hits int
	path string
	last int64
}

// cleanPath is a page's path the way pages are kept: unescaped, no trailing slash.
func cleanPath(page *url.URL) string {
	path := page.EscapedPath()
	if p, err := url.PathUnescape(path); err == nil {
		path = p
	}
	if path == "" {
		path = "/"
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return clip(path, 300)
}

// sawRobot notes one page read by a robot (nothing when the agent is a person's browser).
func (s *Server) sawRobot(site *store.Site, agent string, page *url.URL) bool {
	name, kind, ok := ua.Robot(agent)
	if !ok {
		return false
	}
	now := time.Now().Unix()
	k := robotKey{site: site.ID, ts: now / 3600 * 3600, name: name, kind: kind, path: cleanPath(page)}
	s.robotMu.Lock()
	if len(s.robotBuf) < 50000 {
		s.robotBuf[k]++
	}
	// a robot without a name: keep what it calls itself, to tell them apart
	if name == ua.UnnamedRobot {
		ak := agentKey{site: site.ID, agent: clip(strings.TrimSpace(agent), 300)}
		if a, ok := s.agentBuf[ak]; ok || len(s.agentBuf) < 5000 {
			s.agentBuf[ak] = agentSeen{hits: a.hits + 1, path: k.path, last: now}
		}
	}
	s.robotMu.Unlock()
	return true
}

// flushRobots writes what was noted since the last time.
func (s *Server) flushRobots(ctx context.Context) {
	s.robotMu.Lock()
	if len(s.robotBuf) == 0 && len(s.agentBuf) == 0 {
		s.robotMu.Unlock()
		return
	}
	buf, agents := s.robotBuf, s.agentBuf
	s.robotBuf, s.agentBuf = map[robotKey]int{}, map[agentKey]agentSeen{}
	s.robotMu.Unlock()
	hits := make([]store.RobotHit, 0, len(buf))
	for k, n := range buf {
		hits = append(hits, store.RobotHit{SiteID: k.site, TS: k.ts, Name: k.name, Kind: k.kind, Path: k.path, Hits: n})
	}
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := s.store.AddRobotHits(wctx, hits); err != nil {
		s.log.Error("robots: saving failed", "rows", len(hits), "err", err)
	}
	if len(agents) > 0 {
		list := make([]store.RobotAgent, 0, len(agents))
		for k, a := range agents {
			list = append(list, store.RobotAgent{SiteID: k.site, Agent: k.agent, Hits: a.hits, Path: a.path, Last: a.last})
		}
		if err := s.store.AddRobotAgents(wctx, list); err != nil {
			s.log.Error("robots: saving agents failed", "rows", len(list), "err", err)
		}
	}
}

// handleRobot takes one robot request from a site's own server: { d: the
// site's domain, u: the page's address, ua: the robot's User-Agent }, with
// the site's proxy key in X-Pocket-Proxy-Key. It always answers 202.
func (s *Server) handleRobot(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	var in struct {
		Domain string `json:"d"`
		URL    string `json:"u"`
		Agent  string `json:"ua"`
	}
	if err != nil || json.Unmarshal(body, &in) != nil || in.Domain == "" || in.URL == "" {
		writeErr(w, http.StatusBadRequest, "need d (domain), u (url) and ua (the robot's User-Agent)")
		return
	}
	reason := s.robotHit(r, in.Domain, in.URL, in.Agent)
	s.counter("robot: " + reason)
	if s.cfg.DemoData || s.cfg.LogLevel == "debug" {
		w.Header().Set("X-Pocket-Result", reason)
	}
	w.WriteHeader(http.StatusAccepted)
	io.WriteString(w, "ok")
}

func (s *Server) robotHit(r *http.Request, domain, address, agent string) string {
	site := s.siteFor(r.Context(), domain)
	if site == nil {
		return "unknown site"
	}
	// only the site's own server may say who read it: a browser cannot know
	if k := r.Header.Get("X-Pocket-Proxy-Key"); k == "" || site.ProxyKey == "" || k != site.ProxyKey {
		return "no key"
	}
	page, err := url.Parse(address)
	if err != nil || page.Host == "" || !s.ownHost(site, page.Hostname()) {
		return "other host"
	}
	if !s.sawRobot(site, agent, page) {
		return "a person"
	}
	return "ok"
}

var robotKinds = map[string]bool{ua.RobotAI: true, ua.RobotSearch: true, ua.RobotPreview: true, ua.RobotSEO: true, ua.RobotMonitor: true, ua.RobotOther: true}

// handleRobots lists the robots that read a site in a range: by=name (who),
// by=path (what they read) or by=kind; kind=ai,search… and name=… narrow it.
func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	_, sp := s.queryFor(r, site)
	q := store.RobotQuery{SiteID: site.ID, From: sp.From.Unix(), To: sp.To.Unix(), By: r.URL.Query().Get("by"), Name: clip(r.URL.Query().Get("name"), 80), Limit: queryInt(r, "limit")}
	for _, k := range strings.Split(r.URL.Query().Get("kind"), ",") {
		if robotKinds[k] {
			q.Kinds = append(q.Kinds, k)
		}
	}
	rows, total, err := s.store.Robots(r.Context(), q)
	if err != nil {
		s.fail(w, err, "robots")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "total": total, "seen": s.store.HasRobots(r.Context(), site.ID)})
}

// handleRobotAgents lists what the robots without a name call themselves
// (their User-Agent), most first.
func (s *Server) handleRobotAgents(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	_, sp := s.queryFor(r, site)
	rows, err := s.store.RobotAgents(r.Context(), site.ID, sp.From.Unix(), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "robots")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
}
