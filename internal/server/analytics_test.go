package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const chromeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// hit sends one hit the way the tracking script does.
func (a *app) hit(body map[string]any, ip, agent string, headers map[string]string) string {
	a.t.Helper()
	j, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", a.url+"/api/event", bytes.NewReader(j))
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("User-Agent", agent)
	req.Header.Set("X-Forwarded-For", ip)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(res.Body)
		a.t.Fatalf("event → %d %s", res.StatusCode, raw)
	}
	return res.Header.Get("X-Pocket-Result")
}

func demoApp(t *testing.T) *app {
	a := newApp(t, nil)
	a.s.cfg.DemoData = true // X-Pocket-Result on hits
	return a
}

func TestCollectAndCount(t *testing.T) {
	a := demoApp(t)
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "https://www.Example.com/blog", "name": "Example"})
	if site["domain"] != "example.com" {
		t.Fatalf("domain = %v", site["domain"])
	}
	sid := id(site)
	if !strings.Contains(str(site, "snippet"), `data-domain="example.com"`) {
		t.Fatalf("snippet: %v", site["snippet"])
	}
	a.expect(409, a.bob, "POST", "/api/v1/sites", map[string]any{"domain": "example.com"})

	// two people, one reads two pages, the other bounces; a robot and another host are not counted
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://example.com/?utm_source=newsletter&utm_medium=email", "d": "example.com", "r": nil, "w": 1400}, "1.2.3.4", chromeUA, nil); r != "ok" {
		t.Fatalf("first hit: %s", r)
	}
	a.hit(map[string]any{"n": "pageview", "u": "https://example.com/pricing/", "d": "example.com", "w": 1400}, "1.2.3.4", chromeUA, nil)
	a.hit(map[string]any{"n": "engagement", "u": "https://example.com/pricing", "d": "example.com", "e": 42000, "sd": 80}, "1.2.3.4", chromeUA, nil)
	a.hit(map[string]any{"n": "Signed up", "u": "https://example.com/pricing", "d": "example.com", "p": map[string]any{"plan": "pro"}}, "1.2.3.4", chromeUA, nil)
	a.hit(map[string]any{"n": "pageview", "u": "https://blog.example.com/post", "d": "example.com", "r": "https://news.ycombinator.com/item?id=1", "w": 390}, "5.6.7.8",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1", map[string]string{"Cf-Ipcountry": "DE"})
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://example.com/", "d": "example.com"}, "9.9.9.9", "Mozilla/5.0 (compatible; Googlebot/2.1)", nil); r != "robot" {
		t.Fatalf("robot: %s", r)
	}
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://evil.test/", "d": "example.com"}, "9.9.9.9", chromeUA, nil); r != "other host" {
		t.Fatalf("other host: %s", r)
	}
	if r := a.hit(map[string]any{"n": "pageview", "u": "http://localhost:3000/", "d": "example.com"}, "9.9.9.9", chromeUA, nil); r != "localhost" {
		t.Fatalf("localhost: %s", r)
	}
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://example.com/", "d": "nobody.test"}, "9.9.9.9", chromeUA, nil); r != "unknown site" {
		t.Fatalf("unknown: %s", r)
	}
	a.s.waitFlushed()

	st := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/stats?range=today", nil)
	tot := st["totals"].(map[string]any)
	if num(tot, "visitors") != 2 || num(tot, "visits") != 2 || num(tot, "pageviews") != 3 || num(tot, "events") != 1 || num(tot, "bounce_rate") != 50 {
		t.Fatalf("totals: %v", tot)
	}
	rows := func(dim string) []any {
		return a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/breakdown?range=today&dim="+dim, nil)["rows"].([]any)
	}
	src := map[string]bool{}
	for _, r := range rows("source") {
		src[str(r.(map[string]any), "value")] = true
	}
	if !src["newsletter"] || !src["Hacker News"] {
		t.Fatalf("sources: %v", src)
	}
	pages := rows("page")
	for _, p := range pages {
		m := p.(map[string]any)
		if str(m, "value") == "/pricing" && (num(m, "time_on_page") != 42 || num(m, "scroll") != 80) {
			t.Fatalf("pricing engagement: %v", m)
		}
	}
	if c := rows("country"); len(c) != 1 || str(c[0].(map[string]any), "value") != "DE" {
		t.Fatalf("countries: %v", c)
	}
	if d := rows("device"); len(d) != 2 {
		t.Fatalf("devices: %v", d)
	}
	props := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/breakdown?range=today&dim=prop&event=Signed+up&key=plan", nil)["rows"].([]any)
	if len(props) != 1 || str(props[0].(map[string]any), "value") != "pro" {
		t.Fatalf("props: %v", props)
	}
	// filters: only the visit from Hacker News
	f := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/stats?range=today&f=source:Hacker+News", nil)["totals"].(map[string]any)
	if num(f, "visitors") != 1 || num(f, "pageviews") != 1 {
		t.Fatalf("filtered: %v", f)
	}
	// a goal and a funnel
	g := a.ok(a.alice, "POST", "/api/v1/sites/"+sid+"/goals", map[string]any{"kind": "event", "match": "Signed up"})
	conv := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/conversions?range=today", nil)["goals"].([]any)
	if len(conv) != 1 || num(conv[0].(map[string]any), "visitors") != 1 || num(conv[0].(map[string]any), "rate") != 50 {
		t.Fatalf("conversions: %v", conv)
	}
	fu := a.ok(a.alice, "POST", "/api/v1/sites/"+sid+"/funnels", map[string]any{"name": "Sign up", "steps": []map[string]any{{"kind": "page", "match": "/"}, {"kind": "page", "match": "/pricing"}, {"kind": "event", "match": "Signed up"}}})
	steps := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/funnels/"+id(fu)+"/counts?range=today", nil)["steps"].([]any)
	if num(steps[0].(map[string]any), "visitors") != 1 || num(steps[2].(map[string]any), "visitors") != 1 {
		t.Fatalf("funnel: %v", steps)
	}
	visits := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/visits?range=today", nil)
	if num(visits, "total") != 2 {
		t.Fatalf("visits: %v", visits)
	}
	live := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/live", nil)
	if num(live, "now") != 2 {
		t.Fatalf("live: %v", live)
	}

	// Bob sees none of it and cannot touch it
	for _, p := range []string{"", "/stats", "/breakdown?dim=page", "/visits", "/live", "/conversions", "/funnels/" + id(fu) + "/counts"} {
		a.expect(404, a.bob, "GET", "/api/v1/sites/"+sid+p, nil)
	}
	a.expect(404, a.bob, "PUT", "/api/v1/sites/"+sid, map[string]any{"name": "mine"})
	a.expect(404, a.bob, "DELETE", "/api/v1/sites/"+sid, nil)
	a.expect(404, a.bob, "POST", "/api/v1/sites/"+sid+"/reset", nil)
	a.expect(404, a.bob, "DELETE", "/api/v1/goals/"+id(g), nil)
	a.expect(404, a.bob, "DELETE", "/api/v1/funnels/"+id(fu), nil)
	a.expect(404, a.bob, "POST", "/api/v1/sites/"+sid+"/goals", map[string]any{"kind": "event", "match": "x"})
	if l := a.ok(a.bob, "GET", "/api/v1/sites", nil)["sites"].([]any); len(l) != 0 {
		t.Fatalf("bob sees sites: %v", l)
	}
	if s := a.ok(a.bob, "GET", "/api/v1/summary", nil)["sites"].([]any); len(s) != 0 {
		t.Fatalf("bob's summary: %v", s)
	}
	// excluding a path, then reset
	a.ok(a.alice, "PUT", "/api/v1/sites/"+sid, map[string]any{"options": map[string]any{"exclude": []string{"/admin*"}, "outbound": true}})
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://example.com/admin/users", "d": "example.com"}, "1.2.3.4", chromeUA, nil); r != "excluded" {
		t.Fatalf("excluded: %s", r)
	}
	a.ok(a.alice, "POST", "/api/v1/sites/"+sid+"/reset", nil)
	if v := a.ok(a.alice, "GET", "/api/v1/sites/"+sid+"/stats?range=today", nil)["totals"].(map[string]any); num(v, "visitors") != 0 {
		t.Fatalf("after reset: %v", v)
	}
}

func TestPublicHostOnlyCollects(t *testing.T) {
	a := newApp(t, nil)
	a.s.cfg.PublicURL = "https://collect.example.net"
	h := a.s.hostSwitch(a.s.Router())
	for path, want := range map[string]int{"/js/script.js": 200, "/api/v1/sites": 404, "/": 404, "/api/v1/auth/status": 404} {
		req := httptest.NewRequest("GET", "https://collect.example.net"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%s → %d, want %d", path, rec.Code, want)
		}
	}
	req := httptest.NewRequest("OPTIONS", "https://collect.example.net/api/event", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("preflight → %d %v", rec.Code, rec.Header())
	}
}

func TestProxyKeyPlace(t *testing.T) {
	a := demoApp(t)
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "proxied.test"})
	key := str(site, "proxy_key")
	hdr := map[string]string{"X-Pocket-Proxy-Key": key, "X-Pocket-Client-IP": "203.0.113.9", "X-Pocket-Country": "ba", "X-Pocket-City": "Sarajevo"}
	a.hit(map[string]any{"n": "pageview", "u": "https://proxied.test/", "d": "proxied.test"}, "10.0.0.1", chromeUA, hdr)
	// a wrong key: the headers are not believed
	a.hit(map[string]any{"n": "pageview", "u": "https://proxied.test/", "d": "proxied.test"}, "10.0.0.2", chromeUA, map[string]string{"X-Pocket-Proxy-Key": "nope", "X-Pocket-Country": "US"})
	a.s.waitFlushed()
	rows := a.ok(a.alice, "GET", "/api/v1/sites/"+id(site)+"/breakdown?range=today&dim=city", nil)["rows"].([]any)
	if len(rows) != 1 || str(rows[0].(map[string]any), "value") != "Sarajevo" || str(rows[0].(map[string]any), "country") != "BA" {
		t.Fatalf("cities: %v", rows)
	}
}

func TestLinkScopes(t *testing.T) {
	cases := []struct {
		method, path string
		scopes       []string
		ok           bool
	}{
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/summary", []string{"analytics:read"}, true},
		{"GET", "/api/v1/sites/3/stats", []string{"analytics:read"}, true},
		{"GET", "/api/v1/sites/3/breakdown", []string{"analytics:read"}, true},
		{"GET", "/api/v1/day/2026-11-20", []string{"analytics:read"}, true},
		{"GET", "/api/v1/sites/3/visits", []string{"analytics:read"}, false},
		{"GET", "/api/v1/sites/3/visits/9", []string{"analytics:read"}, false},
		{"GET", "/api/v1/summary", nil, false},
		{"POST", "/api/v1/sites", []string{"analytics:read"}, false},
		{"DELETE", "/api/v1/sites/3", []string{"analytics:read"}, false},
		{"GET", "/api/v1/settings", []string{"analytics:read"}, false},
		{"GET", "/api/v1/export.json", []string{"analytics:read"}, false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, nil)
		if got := linkAllows(c.scopes, r); got != c.ok {
			t.Errorf("%s %s %v: %v, want %v", c.method, c.path, c.scopes, got, c.ok)
		}
	}
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "shop.test", "name": "Shop"})
	a.ok(a.alice, "POST", "/api/v1/sites/"+id(site)+"/goals", map[string]any{"name": "Bought", "kind": "event", "match": "Purchase"})
	a.ok(a.alice, "POST", "/api/v1/sites/"+id(site)+"/funnels", map[string]any{"name": "Buy", "steps": []map[string]any{{"kind": "page", "match": "/cart"}, {"kind": "event", "match": "Purchase"}}})
	exp := a.ok(a.alice, "GET", "/api/v1/export.json", nil)
	// back into the same account: nothing new
	res := a.ok(a.alice, "POST", "/api/v1/import", exp)
	if num(res, "sites") != 0 || num(res, "goals") != 0 || num(res, "funnels") != 0 {
		t.Fatalf("re-import added: %v", res)
	}
	// into Bob's: the site is Alice's on this box, so it is skipped
	res = a.ok(a.bob, "POST", "/api/v1/import", exp)
	if num(res, "skipped") != 1 {
		t.Fatalf("bob import: %v", res)
	}
	// after deleting, it comes back whole
	a.ok(a.alice, "DELETE", "/api/v1/sites/"+id(site), nil)
	res = a.ok(a.alice, "POST", "/api/v1/import", exp)
	if num(res, "sites") != 1 || num(res, "goals") != 1 || num(res, "funnels") != 1 {
		t.Fatalf("import: %v", res)
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"tz": "Europe/Sarajevo", "weekly_report": false, "notify_url": "ntfy://x/y"})
	b := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if b["weekly_report"] != true || b["notify_url"] != "" {
		t.Fatalf("bob got alice's settings: %v", b)
	}
	al := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	if al["weekly_report"] != false || al["tz_set"] != "Europe/Sarajevo" {
		t.Fatalf("alice's settings: %v", al)
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(t.Context(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	sites := a.ok(a.alice, "GET", "/api/v1/sites", nil)["sites"].([]any)
	if len(sites) != 2 {
		t.Fatalf("demo sites: %d", len(sites))
	}
	st := a.ok(a.alice, "GET", "/api/v1/sites/"+id(sites[0].(map[string]any))+"/stats?range=30d", nil)["totals"].(map[string]any)
	if num(st, "visitors") < 1000 {
		t.Fatalf("demo totals: %v", st)
	}
}
