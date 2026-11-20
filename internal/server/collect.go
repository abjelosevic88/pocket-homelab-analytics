package server

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/refs"
	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
	"github.com/abjelosevic88/pocket-homelab-analytics/internal/ua"
)

// ---- the tracking script and the collector (public) ----
//
// A website loads /js/script.js and sends each page and event to
// /api/event (the same fields as Plausible's, so its script works too).
// Nothing identifies a person: the visitor is a hash of the site, the IP
// address and the browser with a salt that changes every day and is then
// thrown away. The IP address itself is never stored.

//go:embed tracker/script.js
var trackerSource string

// trackerJS is the script without comments and indentation.
var trackerJS = func() string {
	var b strings.Builder
	for _, line := range strings.Split(trackerSource, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "//") {
			continue
		}
		b.WriteString(t)
		b.WriteByte('\n')
	}
	return b.String()
}()

func (s *Server) handleScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	io.WriteString(w, trackerJS)
}

// eventIn is what the script sends.
type eventIn struct {
	Name     string          `json:"n"`
	URL      string          `json:"u"`
	Domain   string          `json:"d"`
	Referrer *string         `json:"r"`
	Width    int             `json:"w"`
	Props    json.RawMessage `json:"p"`
	Hash     any             `json:"h"`
	Engaged  int64           `json:"e"`
	Scroll   int             `json:"sd"`
}

func cors(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type")
	h.Set("Access-Control-Max-Age", "86400")
}

func (s *Server) handleEventOptions(w http.ResponseWriter, r *http.Request) {
	cors(w)
	w.WriteHeader(http.StatusNoContent)
}

// handleEvent takes one hit. It always answers 202 to a well-formed hit:
// a website learns nothing about what was kept.
func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request) {
	cors(w)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8<<10))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "too large")
		return
	}
	var in eventIn
	if err := json.Unmarshal(body, &in); err != nil || in.Name == "" || in.URL == "" {
		writeErr(w, http.StatusBadRequest, "need n (name), u (url) and d (domain)")
		return
	}
	reason := s.collect(r, in)
	s.counter(reason)
	if s.cfg.DemoData || s.cfg.LogLevel == "debug" {
		w.Header().Set("X-Pocket-Result", reason)
	}
	w.WriteHeader(http.StatusAccepted)
	io.WriteString(w, "ok")
}

// ---- turning a hit into a stored one ----

// collect checks and enriches a hit and queues it; it says what happened.
func (s *Server) collect(r *http.Request, in eventIn) string {
	ctx := r.Context()
	agent := r.UserAgent()
	if ua.IsBot(agent) {
		return "robot"
	}
	page, err := url.Parse(in.URL)
	if err != nil || page.Host == "" {
		return "bad url"
	}
	name := clip(strings.TrimSpace(in.Name), 120)
	if name == "" {
		return "no name"
	}
	if strings.EqualFold(name, "pageview") {
		name = "pageview"
	}
	host := strings.TrimPrefix(strings.ToLower(page.Hostname()), "www.")
	sites := []*store.Site{}
	for _, d := range strings.Split(in.Domain, ",") {
		if st := s.siteFor(ctx, strings.TrimSpace(d)); st != nil {
			sites = append(sites, st)
		}
	}
	if len(sites) == 0 {
		return "unknown site"
	}
	ip, place := s.clientPlace(r, sites[0])
	if !s.allow(ip) {
		return "too many"
	}
	info := ua.Parse(agent, in.Width)
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
	now := time.Now().Unix()
	kept := ""
	for _, site := range sites {
		if why := s.refuse(site, host, path, name); why != "" {
			kept = why
			continue
		}
		p := path
		if site.Options.Hash && page.Fragment != "" {
			p = path + "#" + page.Fragment
		}
		hit := store.Incoming{SiteID: site.ID, TS: now, Name: name, Path: clip(p, 500)}
		hit.Visitor, hit.PrevVisitor = s.visitorIDs(ctx, site.ID, ip, agent)
		if name == "engagement" {
			hit.Engaged = min(max(in.Engaged, 0), 30*60*1000)
			hit.Scroll = min(max(in.Scroll, 0), 100)
		} else {
			hit.Props = cleanProps(in.Props)
			ref := ""
			if in.Referrer != nil {
				ref = *in.Referrer
			}
			src := refs.Classify(page, ref, func(h string) bool { return s.ownHost(site, h) })
			hit.Visit = store.Visit{Hostname: host, Referrer: src.Referrer, Source: src.Name, Channel: src.Channel,
				UTMSource: src.UTMSource, UTMMedium: src.UTMMedium, UTMCampaign: src.UTMCampaign, UTMContent: src.UTMContent, UTMTerm: src.UTMTerm,
				Country: place.Country, Region: clip(place.Region, 80), City: clip(place.City, 80),
				Device: info.Device, Browser: info.Browser, BrowserVersion: info.BrowserVersion, OS: info.OS, OSVersion: info.OSVersion,
				Language: language(r.Header.Get("Accept-Language"))}
		}
		if !s.enqueue(hit) {
			return "busy"
		}
		kept = "ok"
	}
	return kept
}

// refuse says why a site does not keep this hit ("" = it does).
func (s *Server) refuse(site *store.Site, host, path, name string) string {
	if host == "localhost" || host == "127.0.0.1" || strings.HasSuffix(host, ".localhost") || host == "[::1]" {
		if !site.Options.Localhost {
			return "localhost"
		}
	} else if !s.ownHost(site, host) {
		return "other host"
	}
	for _, ex := range site.Options.Exclude {
		if ex != "" && store.GlobMatch(ex, path) {
			return "excluded"
		}
	}
	switch name {
	case "Outbound Link: Click":
		if !site.Options.Outbound {
			return "off"
		}
	case "File Download":
		if !site.Options.Downloads {
			return "off"
		}
	case "Click":
		if !site.Options.Clicks {
			return "off"
		}
	}
	return ""
}

// ownHost reports whether a host name is the site's (the domain, any name
// under it, or one of its other names).
func (s *Server) ownHost(site *store.Site, host string) bool {
	host = strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(host, ".")), "www.")
	for _, d := range append([]string{site.Domain}, site.Also...) {
		d = strings.TrimPrefix(strings.ToLower(d), "www.")
		if d != "" && (host == d || strings.HasSuffix(host, "."+d)) {
			return true
		}
	}
	return false
}

func cleanProps(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		// Plausible's script sends props as a JSON string
		var str string
		if json.Unmarshal(raw, &str) != nil || json.Unmarshal([]byte(str), &m) != nil {
			return ""
		}
	}
	out := map[string]string{}
	for k, v := range m {
		if len(out) >= 30 {
			break
		}
		k = clip(strings.TrimSpace(k), 60)
		if k == "" || v == nil {
			continue
		}
		var sv string
		switch t := v.(type) {
		case string:
			sv = t
		case float64:
			sv = strconv.FormatFloat(t, 'f', -1, 64)
		case bool:
			sv = strconv.FormatBool(t)
		default:
			b, _ := json.Marshal(t)
			sv = string(b)
		}
		out[k] = clip(sv, 300)
	}
	if len(out) == 0 {
		return ""
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func language(h string) string {
	l := strings.TrimSpace(strings.Split(strings.Split(h, ",")[0], ";")[0])
	if l == "" || l == "*" {
		return ""
	}
	return clip(strings.ToLower(strings.Split(l, "-")[0]), 8)
}

// ---- where the visitor is ----

// clientPlace finds the visitor's address and place: from a proxy in
// front that proves itself with the site's proxy key (a Cloudflare
// Worker), else from Cloudflare's own headers, else from the connection;
// what is missing comes from the offline database.
func (s *Server) clientPlace(r *http.Request, site *store.Site) (string, placeT) {
	h := r.Header
	var p placeT
	ip := ""
	if k := h.Get("X-Pocket-Proxy-Key"); k != "" && site.ProxyKey != "" && k == site.ProxyKey {
		ip = h.Get("X-Pocket-Client-IP")
		p = placeT{Country: strings.ToUpper(h.Get("X-Pocket-Country")), Region: h.Get("X-Pocket-Region"), City: h.Get("X-Pocket-City")}
	}
	if ip == "" {
		ip = h.Get("Cf-Connecting-Ip")
		if p.Country == "" {
			p.Country = strings.ToUpper(h.Get("Cf-Ipcountry"))
			p.Region = firstNonEmpty(h.Get("Cf-Region"), p.Region)
			p.City = firstNonEmpty(h.Get("Cf-Ipcity"), p.City)
		}
	}
	if ip == "" {
		ip = r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
	}
	if p.Country == "XX" || p.Country == "T1" {
		p.Country = ""
	}
	if p.City == "" || p.Country == "" {
		g := s.geo.Lookup(ip)
		if p.Country == "" {
			p = placeT{Country: g.Country, Region: g.Region, City: g.City}
		} else if g.Country == p.Country {
			p.Region, p.City = firstNonEmpty(p.Region, g.Region), firstNonEmpty(p.City, g.City)
		}
	}
	return ip, p
}

type placeT struct{ Country, Region, City string }

// ---- the daily salt ----

// visitorIDs are the visitor hash under today's salt and yesterday's
// (UTC days): a visit over midnight goes on, the next day it is someone new.
func (s *Server) visitorIDs(ctx context.Context, siteID int64, ip, agent string) (string, string) {
	day := time.Now().UTC()
	today := s.salt(ctx, day.Format("2006-01-02"))
	yesterday := s.salt(ctx, day.AddDate(0, 0, -1).Format("2006-01-02"))
	id := func(salt string) string {
		if salt == "" {
			return ""
		}
		h := sha256.Sum256([]byte(salt + "|" + strconv.FormatInt(siteID, 10) + "|" + ip + "|" + agent))
		return hex.EncodeToString(h[:12])
	}
	return id(today), id(yesterday)
}

// salt is a day's random salt; today's is made when first needed, an old
// one is only read (and deleted after two days by the janitor).
func (s *Server) salt(ctx context.Context, day string) string {
	s.saltMu.Lock()
	defer s.saltMu.Unlock()
	if v, ok := s.salts[day]; ok {
		return v
	}
	v, _, ok := s.store.CacheGet(ctx, "salt:"+day)
	if !ok && day == time.Now().UTC().Format("2006-01-02") {
		v = store.RandomToken(24)
		_ = s.store.CachePut(ctx, "salt:"+day, v)
		ok = true
	}
	if ok {
		s.salts[day] = v
	}
	return v
}

// dropOldSalts forgets salts older than yesterday: from then on nobody
// can tell whether two hashes were the same person.
func (s *Server) dropOldSalts(ctx context.Context) {
	keep := map[string]bool{}
	now := time.Now().UTC()
	for _, d := range []time.Time{now, now.AddDate(0, 0, -1)} {
		keep["salt:"+d.Format("2006-01-02")] = true
	}
	rows, err := s.store.DB.QueryContext(ctx, `SELECT key FROM cache WHERE key LIKE 'salt:%'`)
	if err != nil {
		return
	}
	var old []string
	for rows.Next() {
		var k string
		if rows.Scan(&k) == nil && !keep[k] {
			old = append(old, k)
		}
	}
	rows.Close()
	for _, k := range old {
		_, _ = s.store.DB.ExecContext(ctx, `DELETE FROM cache WHERE key = ?`, k)
	}
	s.saltMu.Lock()
	for d := range s.salts {
		if !keep["salt:"+d] {
			delete(s.salts, d)
		}
	}
	s.saltMu.Unlock()
}

// ---- which site, quickly ----

// siteFor finds a site by domain from a small cache (a minute).
func (s *Server) siteFor(ctx context.Context, domain string) *store.Site {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), "www.")
	if domain == "" {
		return nil
	}
	s.sitesMu.Lock()
	if time.Since(s.sitesAt) > time.Minute {
		s.sitesMu.Unlock()
		s.reloadSites(ctx)
		s.sitesMu.Lock()
	}
	st := s.sitesByDomain[domain]
	s.sitesMu.Unlock()
	return st
}

// reloadSites refreshes the domain table (after a site changes, and each minute).
func (s *Server) reloadSites(ctx context.Context) {
	all, err := s.store.AllSites(ctx)
	if err != nil {
		return
	}
	m := map[string]*store.Site{}
	for _, st := range all {
		m[st.Domain] = st
	}
	s.sitesMu.Lock()
	s.sitesByDomain, s.sitesAt = m, time.Now()
	s.sitesMu.Unlock()
}

// ---- a little protection ----

type bucket struct {
	n  int
	at int64
}

// allow lets one address send up to 300 hits a minute.
func (s *Server) allow(ip string) bool {
	now := time.Now().Unix() / 60
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	if len(s.rate) > 50000 {
		s.rate = map[string]*bucket{}
	}
	b := s.rate[ip]
	if b == nil || b.at != now {
		b = &bucket{at: now}
		s.rate[ip] = b
	}
	b.n++
	return b.n <= 300
}

// ---- the writer ----

// enqueue hands a hit to the writer; false when the queue is full.
func (s *Server) enqueue(h store.Incoming) bool {
	select {
	case s.hits <- h:
		return true
	default:
		return false
	}
}

// writer stores queued hits in batches (each half second or 500 hits).
func (s *Server) writer(ctx context.Context) {
	cache := store.VisitCache{}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	sweep := time.NewTicker(5 * time.Minute)
	defer sweep.Stop()
	var batch []store.Incoming
	flush := func() {
		if len(batch) == 0 {
			return
		}
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		n, err := s.store.SaveHits(wctx, batch, cache)
		cancel()
		if err != nil {
			s.log.Error("collector: saving hits failed", "hits", len(batch), "err", err)
			clear(cache)
		} else {
			s.stored.Add(int64(n))
		}
		batch = batch[:0]
	}
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case h := <-s.hits:
					batch = append(batch, h)
				default:
					flush()
					return
				}
			}
		case h := <-s.hits:
			batch = append(batch, h)
			if len(batch) >= 500 {
				flush()
			}
		case <-tick.C:
			flush()
		case done := <-s.flushReq:
			for drained := false; !drained; {
				select {
				case h := <-s.hits:
					batch = append(batch, h)
				default:
					drained = true
				}
			}
			flush()
			close(done)
		case <-sweep.C:
			cache.Sweep(time.Now().Unix())
		}
	}
}

// waitFlushed waits until everything queued is written (tests, demo).
func (s *Server) waitFlushed() {
	done := make(chan struct{})
	select {
	case s.flushReq <- done:
	case <-time.After(5 * time.Second):
		return
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
	}
}

// counter counts what happened to hits, for /metrics.
func (s *Server) counter(reason string) {
	s.countMu.Lock()
	s.counts[reason]++
	s.countMu.Unlock()
}
