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
	"net/netip"
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
// By default nothing identifies a person: the visitor is a hash of the
// site, the IP address and the browser with a salt that changes every day
// and is then thrown away, and the IP address is not stored. A site that
// switches on "Know who visits" (options.identify) also keeps what reaches
// the server anyway: the IP address, who owns it, the place on the map, the
// whole User-Agent, referrer and Accept-Language — unless the browser asks
// not to be tracked (GPC, Do Not Track). Nothing is read from the browser.
// Only with options.browser_id (behind a consent banner) does the browser
// keep a random ID (data-identify: localStorage, returning visitors) and
// tell its screen, settings and hardware.

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
	Vitals   *vitalsIn       `json:"v"`   // engagement: how fast the page was
	Revenue  json.RawMessage `json:"$"`   // an event's money: { amount, currency }
	Sections []string        `json:"sec"` // engagement: the page's parts that were on screen (data-pocket-section)
	ID       string          `json:"id"`  // data-identify: the browser's own random ID
	X        json.RawMessage `json:"x"`   // data-device / data-identify: what the browser says about itself (first page of a load)
	NoTrack  int             `json:"np"`  // the browser asks not to be tracked (Global Privacy Control, Do Not Track)
}

// vitalsIn is how fast the page was, as the script measured it: t = the
// server answered, f = something showed, l = the main content showed,
// i = the slowest press (all milliseconds), c = how much the layout moved.
type vitalsIn struct {
	T *float64 `json:"t"`
	F *float64 `json:"f"`
	L *float64 `json:"l"`
	I *float64 `json:"i"`
	C *float64 `json:"c"`
}

// clean keeps what can be true: up to two minutes, a layout shift up to 10.
func (v *vitalsIn) clean() store.Vitals {
	var out store.Vitals
	if v == nil {
		return out
	}
	ms := func(f *float64) *int64 {
		if f == nil || *f <= 0 || *f > 120000 {
			return nil
		}
		n := int64(*f + 0.5)
		if n == 0 {
			n = 1
		}
		return &n
	}
	out.TTFB, out.FCP, out.LCP, out.INP = ms(v.T), ms(v.F), ms(v.L), ms(v.I)
	if v.C != nil && *v.C >= 0 && *v.C <= 10 {
		c := float64(int64(*v.C*1000+0.5)) / 1000
		out.CLS = &c
	}
	return out
}

// The events the script sends by itself.
const (
	evOutbound = "Outbound Link: Click"
	evDownload = "File Download"
	evClick    = "Click"
	evContact  = "Contact Link: Click" // an email or a phone link
	evForm     = "Form: Submission"
	evNotFound = "404"
	evSearch   = "Search"
)

// cleanRevenue reads an event's money: { amount: 12.5 | "12.50", currency: "EUR" }.
func cleanRevenue(raw json.RawMessage) (float64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, ""
	}
	var m struct {
		Amount   any    `json:"amount"`
		Currency string `json:"currency"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return 0, ""
	}
	var amount float64
	switch t := m.Amount.(type) {
	case float64:
		amount = t
	case string:
		amount, _ = strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(t, ",", ".")), 64)
	}
	if amount <= 0 || amount > 1e9 || amount != amount {
		return 0, ""
	}
	cur := strings.ToUpper(strings.TrimSpace(m.Currency))
	if len(cur) != 3 || strings.Trim(cur, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		cur = ""
	}
	return float64(int64(amount*100+0.5)) / 100, cur
}

// searchTerm is what someone searched for on the site: the first of the
// site's search names in the address that has words.
func searchTerm(q url.Values, names []string) string {
	for _, n := range names {
		if n = strings.TrimSpace(n); n == "" {
			continue
		}
		if t := strings.Join(strings.Fields(strings.ToLower(q.Get(n))), " "); t != "" {
			return clip(t, 80)
		}
	}
	return ""
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
	page, err := url.Parse(in.URL)
	if ua.IsBot(agent) {
		// a robot that runs the script is never a visitor; it is counted with the robots
		if err == nil && page.Host != "" && strings.EqualFold(strings.TrimSpace(in.Name), "pageview") {
			for _, d := range strings.Split(in.Domain, ",") {
				// counted once: a read the site's server already reported is not counted again
				if st := s.siteFor(ctx, strings.TrimSpace(d)); st != nil && s.ownHost(st, page.Hostname()) && !s.wasServed(st, agent, page) {
					s.sawRobot(st, agent, page)
				}
			}
		}
		return "robot"
	}
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
	addr, _ := netip.ParseAddr(strings.Trim(ip, "[]"))
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
		if ignoredAddr(site.Options.IgnoreIPs, addr) {
			kept = "ignored address"
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
			if site.Options.Speed {
				hit.Vitals = in.Vitals.clean()
			}
			hit.Sections = cleanSections(in.Sections)
		} else {
			hit.Props = cleanProps(in.Props)
			if name == evSearch {
				hit.Props = cleanSearchProps(hit.Props)
			}
			if name != "pageview" {
				hit.Revenue, hit.Currency = cleanRevenue(in.Revenue)
			}
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
			if site.Options.Identify && asksNotToBeTracked(r, in) {
				// only the fact, so the visit can say why nothing about the person is there
				hit.Visit.Details = json.RawMessage(`{"not_tracked":true}`)
			} else if site.Options.Identify {
				s.who(r, &hit.Visit, ip, place, agent, ref, in, site.Options.BrowserID)
			}
		}
		if !s.enqueue(hit) {
			return "busy"
		}
		// a page of search results: what was searched for, as an event of its own
		if name == "pageview" && site.Options.Search {
			if term := searchTerm(page.Query(), site.Options.SearchParams); term != "" {
				sh := hit
				sh.Name, sh.Props = evSearch, toJSONString(map[string]string{"term": term})
				s.enqueue(sh)
			}
		}
		kept = "ok"
	}
	return kept
}

// ignoredAddr reports whether an address is one the site does not count
// (an IP, or a range: 203.0.113.0/24, 2001:db8:1:2::/64).
func ignoredAddr(list []string, addr netip.Addr) bool {
	if !addr.IsValid() || len(list) == 0 {
		return false
	}
	addr = addr.Unmap()
	for _, e := range list {
		if p, err := netip.ParsePrefix(e); err == nil {
			if p.Contains(addr) {
				return true
			}
		} else if a, err := netip.ParseAddr(e); err == nil && a.Unmap() == addr {
			return true
		}
	}
	return false
}

// CleanIgnore checks the "don't count these addresses" list: IPs or
// ranges, as they will be matched; "" for each line that is neither.
func cleanIgnore(in []string) ([]string, string) {
	out := []string{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if p, err := netip.ParsePrefix(e); err == nil {
			e = p.Masked().String()
		} else if a, err := netip.ParseAddr(strings.Trim(e, "[]")); err == nil {
			e = a.Unmap().String()
		} else {
			return nil, "“" + clip(e, 60) + "” is not an address (203.0.113.7) or a range (203.0.113.0/24)"
		}
		if len(out) < 50 {
			out = append(out, e)
		}
	}
	return out, ""
}

// cleanSections keeps up to 40 section names of up to 60 characters, in order, once each.
func cleanSections(in []string) string {
	seen := map[string]bool{}
	out := []string{}
	for _, n := range in {
		n = clip(strings.Join(strings.Fields(n), " "), 60)
		if n == "" || seen[n] || len(out) >= 40 {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		return ""
	}
	return toJSONString(out)
}

// cleanSearchProps keeps a site search the way searches are compared: the
// words lowercased and single-spaced; results stays a whole number.
func cleanSearchProps(props string) string {
	if props == "" {
		return ""
	}
	var m map[string]string
	if json.Unmarshal([]byte(props), &m) != nil {
		return props
	}
	if t, ok := m["term"]; ok {
		m["term"] = clip(strings.Join(strings.Fields(strings.ToLower(t)), " "), 80)
	}
	if r, ok := m["results"]; ok {
		if n, err := strconv.Atoi(strings.TrimSpace(r)); err == nil && n >= 0 {
			m["results"] = strconv.Itoa(n)
		} else {
			delete(m, "results")
		}
	}
	return toJSONString(m)
}

func toJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
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
	on := true
	switch name {
	case evOutbound, evContact:
		on = site.Options.Outbound
	case evDownload:
		on = site.Options.Downloads
	case evClick:
		on = site.Options.Clicks
	case evForm:
		on = site.Options.Forms
	case evNotFound:
		on = site.Options.NotFound
	case evSearch:
		on = site.Options.Search
	}
	if !on {
		return "off"
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
	g := s.geo.Lookup(ip)
	if p.Country == "" {
		p = placeT{Country: g.Country, Region: g.Region, City: g.City}
	} else if g.Country == p.Country {
		p.Region, p.City = firstNonEmpty(p.Region, g.Region), firstNonEmpty(p.City, g.City)
	}
	if g.Country == p.Country && (g.City == "" || strings.EqualFold(g.City, p.City)) {
		p.Lat, p.Lon, p.Postal = g.Lat, g.Lon, g.Postal
	}
	return ip, p
}

type placeT struct {
	Country, Region, City string
	Lat, Lon              *float64
	Postal                string
}

// ---- who it was (sites that know who visits) ----

// who fills in who a new visit is: the address and who owns it, the place
// on the map, the browser's own ID and what it says about itself, and what
// the proxy in front saw of the connection (a Cloudflare Worker).
func (s *Server) who(r *http.Request, v *store.Visit, ip string, place placeT, agent, ref string, in eventIn, browserID bool) {
	h := r.Header
	proxied := h.Get("X-Pocket-Proxy-Key") != "" && h.Get("X-Pocket-Client-IP") != ""
	addr, err := netip.ParseAddr(strings.Trim(ip, "[]"))
	if err == nil {
		v.IP = addr.Unmap().String()
	}
	v.Agent = clip(agent, 500)
	if browserID {
		v.ClientID = cleanClientID(in.ID)
	}
	if ref != "" {
		v.RefURL = clip(ref, 800)
	}
	v.Postal, v.Lat, v.Lon = place.Postal, place.Lat, place.Lon
	if proxied {
		if pc := clip(strings.TrimSpace(h.Get("X-Pocket-Postal")), 16); pc != "" {
			v.Postal = pc
		}
		if lat, e1 := strconv.ParseFloat(h.Get("X-Pocket-Lat"), 64); e1 == nil {
			if lon, e2 := strconv.ParseFloat(h.Get("X-Pocket-Lon"), 64); e2 == nil && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 {
				v.Lat, v.Lon = &lat, &lon
			}
		}
		if n, err := strconv.ParseInt(h.Get("X-Pocket-ASN"), 10, 64); err == nil && n > 0 {
			v.ASN = n
		}
		v.Network = clip(strings.TrimSpace(h.Get("X-Pocket-Org")), 120)
	}
	if v.Network == "" || v.ASN == 0 {
		if n := s.asn.Network(v.IP); n.ASN > 0 {
			v.ASN, v.Network = int64(n.ASN), firstNonEmpty(v.Network, clip(n.Org, 120))
		}
	}
	d := cleanDetails(in.X)
	if !browserID {
		// without the visitor's consent nothing is read from the browser:
		// only what reaches the server anyway is kept
		clear(d)
	}
	if al := strings.TrimSpace(h.Get("Accept-Language")); al != "" {
		d["accept_language"] = clip(al, 200)
	}
	if proxied {
		for k, hdr := range map[string]string{"edge": "X-Pocket-Colo", "http": "X-Pocket-HTTP", "tls": "X-Pocket-TLS", "net_rtt": "X-Pocket-RTT",
			"ip_timezone": "X-Pocket-Timezone", "continent": "X-Pocket-Continent", "metro": "X-Pocket-Metro"} {
			if val := clip(strings.TrimSpace(h.Get(hdr)), 80); val != "" {
				d[k] = val
			}
		}
	}
	if len(d) > 0 {
		v.Details = json.RawMessage(toJSONString(d))
	}
}

// asksNotToBeTracked: Global Privacy Control or Do Not Track, from the
// browser's headers (passed on by a proxy) or the script.
func asksNotToBeTracked(r *http.Request, in eventIn) bool {
	return in.NoTrack == 1 || r.Header.Get("Sec-Gpc") == "1" || r.Header.Get("Dnt") == "1"
}

// cleanClientID keeps a browser's ID when it looks like one (8–64 letters, digits, - or _).
func cleanClientID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) < 8 || len(id) > 64 {
		return ""
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return ""
		}
	}
	return id
}

// cleanDetails keeps up to 40 short facts a browser told about itself:
// strings (200 characters at most), numbers and yes/no.
func cleanDetails(raw json.RawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return out
	}
	for k, v := range m {
		if len(out) >= 40 {
			break
		}
		k = clip(strings.TrimSpace(k), 40)
		if k == "" {
			continue
		}
		switch t := v.(type) {
		case string:
			if t = clip(strings.TrimSpace(t), 200); t != "" {
				out[k] = t
			}
		case float64:
			if t == t && t > -1e12 && t < 1e12 {
				out[k] = t
			}
		case bool:
			out[k] = t
		case []any: // a list of words (languages)
			parts := []string{}
			for _, x := range t {
				if sv, ok := x.(string); ok && len(parts) < 10 {
					parts = append(parts, clip(sv, 40))
				}
			}
			if len(parts) > 0 {
				out[k] = strings.Join(parts, ", ")
			}
		}
	}
	return out
}

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
			s.flushRobots(ctx)
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
			s.flushRobots(ctx)
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
