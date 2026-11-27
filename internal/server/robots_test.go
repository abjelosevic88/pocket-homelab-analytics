package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

const gptBot = "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)"
const googleBot = "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"

// A site's own server tells who read it: robots are counted on their own,
// never as visitors, and only with the site's proxy key.
func TestRobots(t *testing.T) {
	a := demoApp(t)
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "shop.test"})
	sid, key := id(site), str(site, "proxy_key")
	base := "/api/v1/sites/" + sid
	report := func(k, address, agent string) string {
		t.Helper()
		j, _ := json.Marshal(map[string]string{"d": "shop.test", "u": address, "ua": agent})
		req, _ := http.NewRequest("POST", a.url+"/api/robot", bytes.NewReader(j))
		req.Header.Set("X-Pocket-Proxy-Key", k)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 202 {
			t.Fatalf("robot → %d", res.StatusCode)
		}
		return res.Header.Get("X-Pocket-Result")
	}
	if st := a.ok(a.alice, "GET", base+"/stats?range=today", nil); st["robots"] != false {
		t.Fatalf("no robots yet: %v", st["robots"])
	}
	for _, c := range []struct{ key, address, agent, want string }{
		{key, "https://shop.test/cakes/", gptBot, "ok"},
		{key, "https://shop.test/cakes", gptBot, "ok"},
		{key, "https://shop.test/", gptBot, "ok"},
		{key, "https://www.shop.test/?utm_source=x", googleBot, "ok"},
		{key, "https://shop.test/menu", "WhatsApp/2.23.20.0 A", "ok"},
		{key, "https://shop.test/menu", chromeUA, "a person"},
		{"wrong", "https://shop.test/menu", gptBot, "no key"},
		{"", "https://shop.test/menu", gptBot, "no key"},
		{key, "https://evil.test/", gptBot, "other host"},
	} {
		if got := report(c.key, c.address, c.agent); got != c.want {
			t.Fatalf("report(%s, %s) = %q, want %q", c.address, c.agent[:12], got, c.want)
		}
	}
	// a robot that runs the script is counted here too, and never as a visitor
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://shop.test/about", "d": "shop.test"}, "9.9.9.9", googleBot, nil); r != "robot" {
		t.Fatalf("a robot through the script: %s", r)
	}
	// the same read reported by the site's server and then by the script is counted once
	if r := a.hit(map[string]any{"n": "pageview", "u": "https://shop.test/", "d": "shop.test"}, "9.9.9.9", gptBot, nil); r != "robot" {
		t.Fatalf("a reported robot through the script: %s", r)
	}
	a.s.waitFlushed()

	who := a.ok(a.alice, "GET", base+"/robots?range=today", nil)
	rows := who["rows"].([]any)
	if num(who, "total") != 6 || len(rows) != 3 || who["seen"] != true {
		t.Fatalf("robots: %v", who)
	}
	first := rows[0].(map[string]any)
	if str(first, "value") != "OpenAI (learning)" || str(first, "kind") != "ai" || num(first, "hits") != 3 || num(first, "pages") != 2 {
		t.Fatalf("first robot: %v", first)
	}
	if ai := a.ok(a.alice, "GET", base+"/robots?range=today&kind=ai,nonsense", nil); num(ai, "total") != 3 || len(ai["rows"].([]any)) != 1 {
		t.Fatalf("only AI: %v", ai)
	}
	pages := a.ok(a.alice, "GET", base+"/robots?range=today&by=path&name=Google", nil)["rows"].([]any)
	if len(pages) != 2 {
		t.Fatalf("what Google read: %v", pages)
	}
	if kinds := a.ok(a.alice, "GET", base+"/robots?range=today&by=kind", nil)["rows"].([]any); len(kinds) != 3 {
		t.Fatalf("kinds: %v", kinds)
	}
	// nobody visited
	st := a.ok(a.alice, "GET", base+"/stats?range=today", nil)
	if tot := st["totals"].(map[string]any); num(tot, "visitors") != 0 || num(tot, "pageviews") != 0 || st["robots"] != true {
		t.Fatalf("robots are not visitors: %v", st)
	}
	a.expect(404, a.bob, "GET", base+"/robots", nil)
	// forgetting the visits forgets the robots
	a.expect(204, a.alice, "POST", base+"/reset", nil)
	if after := a.ok(a.alice, "GET", base+"/robots?range=today", nil); num(after, "total") != 0 || after["seen"] != false {
		t.Fatalf("after a reset: %v", after)
	}
}
