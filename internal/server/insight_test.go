package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/config"
)

// 0.5.0: your own addresses are not counted; the pages visits land on, the
// ways through the site, how far down a page people read (by the parts a
// site marks), what AI assistants do, searches that found nothing, robot
// goals (install.sh run by curl) and the robots without a name.
func TestHowPeopleUseTheSite(t *testing.T) {
	a := newApp(t, func(c *config.Config) { c.TrustProxy = true }) // the test's X-Forwarded-For is the visitor
	a.s.cfg.DemoData = true
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "shop.test"})
	sid, key := id(site), str(site, "proxy_key")
	base := "/api/v1/sites/" + sid

	// own addresses: a bad one is refused, ranges are tidied
	a.expect(400, a.alice, "PUT", base, map[string]any{"options": map[string]any{"ignore_ips": []string{"not an ip"}}})
	up := a.ok(a.alice, "PUT", base, map[string]any{"options": map[string]any{"ignore_ips": []string{" 7.7.7.7 ", "2001:db8:1:2:aa::1/64"}}})
	if ips := up["options"].(map[string]any)["ignore_ips"].([]any); len(ips) != 2 || ips[1] != "2001:db8:1:2::/64" {
		t.Fatalf("ignore_ips: %v", ips)
	}
	// other options stay as they were when only ignore_ips is sent
	if up["options"].(map[string]any)["search"] != true {
		t.Fatalf("options merged: %v", up["options"])
	}
	hit := func(ip, agent string, body map[string]any) string {
		t.Helper()
		body["d"] = "shop.test"
		return a.hit(body, ip, agent, nil)
	}
	if r := hit("7.7.7.7", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/"}); r != "ignored address" {
		t.Fatalf("own address: %s", r)
	}
	if r := hit("2001:db8:1:2::99", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/"}); r != "ignored address" {
		t.Fatalf("own range: %s", r)
	}

	// goals: the menu page, and install.sh read by curl
	a.ok(a.alice, "POST", base+"/goals", map[string]any{"kind": "page", "match": "/menu"})
	rg := a.ok(a.alice, "POST", base+"/goals", map[string]any{"kind": "robot", "match": "/install.sh", "robot": "Scripts and tools", "name": "Ran the install"})
	if str(rg, "robot") != "Scripts and tools" || str(rg, "kind") != "robot" {
		t.Fatalf("robot goal: %v", rg)
	}
	a.expect(400, a.alice, "POST", base+"/funnels", map[string]any{"name": "x", "steps": []map[string]any{{"kind": "robot", "match": "/install.sh"}, {"kind": "page", "match": "/"}}})
	fn := a.ok(a.alice, "POST", base+"/funnels", map[string]any{"name": "Home to install", "steps": []map[string]any{{"kind": "page", "match": "/"}, {"kind": "robot", "match": "/install.sh", "robot": "Scripts and tools"}}})

	// Ann: from ChatGPT to home, reads two parts of it (then a third), then the menu
	hit("1.1.1.1", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/", "r": "https://chatgpt.com/"})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "engagement", "u": "https://shop.test/", "e": 5000, "sd": 55, "sec": []string{"Hero", "Menu"}})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "engagement", "u": "https://shop.test/", "e": 2000, "sd": 80, "sec": []string{"Hero", "Menu", "Prices"}})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/menu"})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "Search", "u": "https://shop.test/menu", "p": map[string]any{"term": "  Vegan  CAKE", "results": "0", "where": "docs"}})
	// Ben: home only, sees the top part
	hit("2.2.2.2", phoneUA, map[string]any{"n": "pageview", "u": "https://shop.test/"})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "engagement", "u": "https://shop.test/", "e": 3000, "sd": 20, "sec": []string{"Hero"}})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "Search", "u": "https://shop.test/", "p": map[string]any{"term": "cake", "results": "4"}})

	// robots: curl runs install.sh twice, Apple reads it once, an unnamed one and one with no name at all
	robot := func(address, agent string) {
		t.Helper()
		j, _ := json.Marshal(map[string]string{"d": "shop.test", "u": address, "ua": agent})
		req, _ := http.NewRequest("POST", a.url+"/api/robot", bytes.NewReader(j))
		req.Header.Set("X-Pocket-Proxy-Key", key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
	}
	robot("https://shop.test/install.sh", "curl/8.5.0")
	robot("https://shop.test/install.sh", "curl/8.7.1")
	robot("https://shop.test/install.sh", "Mozilla/5.0 (Macintosh) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17 Safari/605.1.15 (Applebot/0.1)")
	robot("https://shop.test/menu", "WeirdCrawler/2.0 (+https://weird.test)")
	robot("https://shop.test/menu", "WeirdCrawler/2.0 (+https://weird.test)")
	robot("https://shop.test/", "")
	robot("https://shop.test/menu", gptBot)
	robot("https://shop.test/menu", gptBot)
	a.s.waitFlushed()

	// goals: the robot goal counts the curl runs only, after the visit goals
	conv := a.ok(a.alice, "GET", base+"/conversions?range=today", nil)["goals"].([]any)
	if len(conv) != 2 {
		t.Fatalf("goals: %v", conv)
	}
	last := conv[1].(map[string]any)
	if last["robot"] != true || num(last, "conversions") != 2 || num(last, "visitors") != 0 {
		t.Fatalf("robot goal: %v", last)
	}
	steps := a.ok(a.alice, "GET", base+"/funnels/"+id(fn)+"/counts?range=today", nil)["steps"].([]any)
	if s1 := steps[1].(map[string]any); num(steps[0].(map[string]any), "visitors") != 2 || num(s1, "visitors") != 2 || s1["robot"] != true || num(s1, "next") != 100 {
		t.Fatalf("funnel with a robot step: %v", steps)
	}

	// landing pages: both came in on home; one went on to the menu goal
	land := a.ok(a.alice, "GET", base+"/landing?range=today", nil)
	lr := land["rows"].([]any)
	if land["goals"] != true || len(lr) != 1 {
		t.Fatalf("landing: %v", land)
	}
	if l := lr[0].(map[string]any); str(l, "page") != "/" || num(l, "visits") != 2 || num(l, "converted") != 1 || num(l, "rate") != 50 || num(l, "bounce") != 0 {
		// Ben searched (an event), so nobody bounced
		t.Fatalf("landing row: %v", l)
	}

	// journeys
	j := a.ok(a.alice, "GET", base+"/journeys?range=today", nil)
	jr := j["rows"].([]any)
	if num(j, "visits") != 2 || len(jr) != 2 {
		t.Fatalf("journeys: %v", j)
	}
	if first := jr[0].(map[string]any); len(first["pages"].([]any)) != 2 || num(first, "converted") != 1 {
		t.Fatalf("longest way first on a tie: %v", jr)
	}

	// reading: the parts of home, in page order, merged across reports
	rd := a.ok(a.alice, "GET", base+"/reading?range=today", nil)
	reading := rd["reading"].(map[string]any)
	secs := reading["sections"].([]any)
	if str(reading, "page") != "/" || num(reading, "views") != 2 || num(reading, "marked") != 2 || len(secs) != 3 {
		t.Fatalf("reading: %v", reading)
	}
	if s0, s2 := secs[0].(map[string]any), secs[2].(map[string]any); str(s0, "name") != "Hero" || num(s0, "rate") != 100 || str(s2, "name") != "Prices" || num(s2, "rate") != 50 {
		t.Fatalf("sections: %v", secs)
	}
	if d := reading["depth"].([]any); num(d[1].(map[string]any), "rate") != 50 || num(d[0].(map[string]any), "rate") != 50 {
		// Ann got to 80 (≥25, ≥50, ≥75), Ben to 20
		t.Fatalf("depth: %v", d)
	}

	// AI: ChatGPT sent Ann to home; GPTBot read the menu twice
	ai := a.ok(a.alice, "GET", base+"/ai?range=today", nil)
	if num(ai, "visits") != 1 || num(ai, "reads") != 2 || len(ai["pages"].([]any)) != 2 || str(ai["pages"].([]any)[0].(map[string]any), "page") != "/" {
		t.Fatalf("ai: %v", ai)
	}

	// searches: the words tidied; the one that found nothing on its own
	srch := a.ok(a.alice, "GET", base+"/searches?range=today&empty=1", nil)
	if sr := srch["rows"].([]any); num(srch, "nothing_found") != 1 || len(sr) != 1 || str(sr[0].(map[string]any), "term") != "vegan cake" || str(sr[0].(map[string]any), "where") != "docs" {
		t.Fatalf("searches: %v", srch)
	}

	// event pages
	ep := a.ok(a.alice, "GET", base+"/event-pages?range=today&event=Search", nil)["rows"].([]any)
	if len(ep) != 2 {
		t.Fatalf("event pages: %v", ep)
	}

	// the robots without a name, by what they call themselves
	un := a.ok(a.alice, "GET", base+"/robots/unnamed?range=today", nil)["rows"].([]any)
	if len(un) != 2 || str(un[0].(map[string]any), "agent") != "WeirdCrawler/2.0 (+https://weird.test)" || num(un[0].(map[string]any), "hits") != 2 || str(un[1].(map[string]any), "agent") != "" {
		t.Fatalf("unnamed robots: %v", un)
	}

	// the address a browser is seen on (public)
	res, err := http.Get(a.url + "/api/address")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	_ = json.NewDecoder(res.Body).Decode(&got)
	res.Body.Close()
	if got["ip"] == "" || res.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("address: %v", got)
	}
}
