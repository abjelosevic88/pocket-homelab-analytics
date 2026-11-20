package server

import (
	"strings"
	"testing"
	"time"
)

const phoneUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1"

// What the script counts since 0.2.0: how fast pages are, pages that do not
// exist, searches, forms, email and phone links, money — and what is read
// from the visits: the busy hours, the way through a page, notes.
func TestMoreThatIsCounted(t *testing.T) {
	a := demoApp(t)
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "shop.test"})
	sid := id(site)
	opts := site["options"].(map[string]any)
	for _, k := range []string{"forms", "not_found", "search", "speed", "outbound"} {
		if opts[k] != true {
			t.Fatalf("a new site counts %s: %v", k, opts)
		}
	}
	base := "/api/v1/sites/" + sid
	hit := func(ip, agent string, body map[string]any) {
		t.Helper()
		body["d"] = "shop.test"
		if r := a.hit(body, ip, agent, nil); r != "ok" {
			t.Fatalf("hit %v: %s", body["n"], r)
		}
	}

	// Ann (laptop): home → a search → the menu → orders for 24.50 EUR; fast pages
	hit("1.1.1.1", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/", "r": nil, "w": 1400})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "engagement", "u": "https://shop.test/", "e": 9000, "sd": 60, "v": map[string]any{"t": 120, "f": 480, "l": 900.4, "i": 80, "c": 0.02}})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/search?q=%20Gluten%20%20FREE", "w": 1400})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "pageview", "u": "https://shop.test/menu", "w": 1400})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "Form: Submission", "u": "https://shop.test/menu", "p": map[string]any{"form": "Order form"}})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "Order placed", "u": "https://shop.test/menu", "$": map[string]any{"amount": "24.50", "currency": "eur"}})
	hit("1.1.1.1", chromeUA, map[string]any{"n": "Contact Link: Click", "u": "https://shop.test/menu", "p": map[string]any{"kind": "phone", "to": "+387 33 555 010"}})
	// Ben (phone): lands on a page that does not exist, goes home, then the menu; a slow page
	hit("2.2.2.2", phoneUA, map[string]any{"n": "pageview", "u": "https://shop.test/old-offer", "r": "https://blog.other.test/post", "w": 390})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "404", "u": "https://shop.test/old-offer", "p": map[string]any{"from": "blog.other.test/post"}})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "engagement", "u": "https://shop.test/old-offer", "e": 4000, "sd": 100, "v": map[string]any{"t": 900, "l": 5200, "c": 0.31, "i": -4, "f": 999999}})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "pageview", "u": "https://shop.test/", "w": 390})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "pageview", "u": "https://shop.test/", "w": 390}) // a reload
	hit("2.2.2.2", phoneUA, map[string]any{"n": "pageview", "u": "https://shop.test/menu", "w": 390})
	hit("2.2.2.2", phoneUA, map[string]any{"n": "Order placed", "u": "https://shop.test/menu", "$": map[string]any{"amount": 10, "currency": "EUR"}})
	a.s.waitFlushed()

	rows := func(q string) []any {
		t.Helper()
		return a.ok(a.alice, "GET", base+"/breakdown?range=today&"+q, nil)["rows"].([]any)
	}
	one := func(q, want string) {
		t.Helper()
		r := rows(q)
		if len(r) != 1 || str(r[0].(map[string]any), "value") != want {
			t.Fatalf("%s: want %q, got %v", q, want, r)
		}
	}
	one("dim=search", "gluten free")
	one("dim=form", "Order form")
	one("dim=contact", "+387 33 555 010")
	one("dim=notfound", "/old-offer")
	one("dim=prop&event=404&key=from&path=/old-offer", "blog.other.test/post")
	if r := rows("dim=prop&event=404&key=from&path=/nowhere"); len(r) != 0 {
		t.Fatalf("404s of another page: %v", r)
	}

	// speed: two measured loads; the bad values (-4 ms, 999999 ms) were dropped
	sp := a.ok(a.alice, "GET", base+"/speed?range=today&dim=device", nil)
	all := sp["all"].(map[string]any)
	if num(all, "loads") != 2 || num(all["lcp"].(map[string]any), "p75") != 5200 || num(all["lcp"].(map[string]any), "n") != 2 {
		t.Fatalf("speed, all: %v", all)
	}
	if num(all["inp"].(map[string]any), "n") != 1 || num(all["fcp"].(map[string]any), "n") != 1 || num(all["cls"].(map[string]any), "p75") != 0.31 {
		t.Fatalf("speed, cleaned: %v", all)
	}
	if by := sp["rows"].([]any); len(by) != 2 {
		t.Fatalf("speed by device: %v", by)
	}
	pg := a.ok(a.alice, "GET", base+"/speed?range=today&f=device:Phone", nil)
	if r := pg["rows"].([]any); len(r) != 1 || str(r[0].(map[string]any), "value") != "/old-offer" || num(r[0].(map[string]any)["lcp"].(map[string]any), "p75") != 5200 {
		t.Fatalf("speed by page on phones: %v", pg)
	}

	// money on a goal
	a.ok(a.alice, "POST", base+"/goals", map[string]any{"kind": "event", "match": "Order placed"})
	conv := a.ok(a.alice, "GET", base+"/conversions?range=today", nil)["goals"].([]any)[0].(map[string]any)
	rev := conv["revenue"].([]any)
	if num(conv, "visitors") != 2 || len(rev) != 1 || str(rev[0].(map[string]any), "currency") != "EUR" || num(rev[0].(map[string]any), "amount") != 34.5 {
		t.Fatalf("money: %v", conv)
	}

	// when: two visits in this hour
	wh := a.ok(a.alice, "GET", base+"/when?range=today", nil)
	if num(wh, "visits") != 2 || num(wh, "max") != 2 || len(wh["cells"].([]any)) != 7 {
		t.Fatalf("when: %v", wh)
	}

	// the way through the home page: Ann came in on it, Ben came from the missing page; a reload counts once
	paths := a.ok(a.alice, "GET", base+"/paths?range=today&page=/", nil)
	if num(paths, "views") != 2 {
		t.Fatalf("paths: %v", paths)
	}
	steps := func(k string) map[string]float64 {
		out := map[string]float64{}
		for _, s := range paths[k].([]any) {
			out[str(s.(map[string]any), "value")] = num(s.(map[string]any), "times")
		}
		return out
	}
	if b, af := steps("before"), steps("after"); b[""] != 1 || b["/old-offer"] != 1 || af["/search"] != 1 || af["/menu"] != 1 {
		t.Fatalf("paths: before %v, after %v", b, af)
	}
	a.expect(400, a.alice, "GET", base+"/paths?range=today", nil)

	// one visit shows its money and its speed
	visits := a.ok(a.alice, "GET", base+"/visits?range=today&f=device:Phone", nil)["visits"].([]any)
	one1 := a.ok(a.alice, "GET", base+"/visits/"+id(visits[0].(map[string]any)), nil)["hits"].([]any)
	var sawMoney, sawSpeed bool
	for _, h := range one1 {
		m := h.(map[string]any)
		sawMoney = sawMoney || (num(m, "revenue") == 10 && str(m, "currency") == "EUR")
		sawSpeed = sawSpeed || num(m, "lcp") == 5200
	}
	if !sawMoney || !sawSpeed {
		t.Fatalf("one visit: %v", one1)
	}

	// notes on the chart
	today := time.Now().Format("2006-01-02")
	a.expect(400, a.alice, "POST", base+"/notes", map[string]any{"day": "soon", "text": "x"})
	a.expect(400, a.alice, "POST", base+"/notes", map[string]any{"day": today, "text": " "})
	note := a.ok(a.alice, "POST", base+"/notes", map[string]any{"day": today, "text": "The newsletter went out"})
	if st := a.ok(a.alice, "GET", base+"/stats?range=today", nil)["notes"].([]any); len(st) != 1 {
		t.Fatalf("notes in stats: %v", st)
	}
	a.ok(a.alice, "PUT", "/api/v1/notes/"+id(note), map[string]any{"day": "2020-01-01", "text": "Opened"})
	if st := a.ok(a.alice, "GET", base+"/stats?range=today", nil)["notes"].([]any); len(st) != 0 {
		t.Fatalf("a note of another day: %v", st)
	}
	if all := a.ok(a.alice, "GET", base+"/notes", nil)["notes"].([]any); len(all) != 1 || str(all[0].(map[string]any), "text") != "Opened" {
		t.Fatalf("notes: %v", all)
	}
	a.expect(404, a.bob, "PUT", "/api/v1/notes/"+id(note), map[string]any{"day": today, "text": "mine"})
	a.expect(404, a.bob, "DELETE", "/api/v1/notes/"+id(note), nil)
	for _, p := range []string{"/speed", "/when", "/paths?page=/", "/notes"} {
		a.expect(404, a.bob, "GET", base+p, nil)
	}
	a.expect(204, a.alice, "DELETE", "/api/v1/notes/"+id(note), nil)

	// switching things off: only what is sent changes, and the line on the pages follows
	off := a.ok(a.alice, "PUT", base, map[string]any{"options": map[string]any{"forms": false, "speed": false, "search": false, "not_found": false, "search_params": []string{"find", "a b", ""}}})
	o := off["options"].(map[string]any)
	if o["forms"] != false || o["outbound"] != true || o["downloads"] != true || o["weekly"] != true {
		t.Fatalf("options after a partial save: %v", o)
	}
	if sp := o["search_params"].([]any); len(sp) != 1 || sp[0] != "find" {
		t.Fatalf("search names: %v", sp)
	}
	for _, attr := range []string{"data-no-forms", "data-no-404", "data-no-speed"} {
		if !strings.Contains(str(off, "snippet"), attr) {
			t.Fatalf("snippet lacks %s: %v", attr, off["snippet"])
		}
	}
	if r := a.hit(map[string]any{"n": "Form: Submission", "u": "https://shop.test/", "d": "shop.test"}, "3.3.3.3", chromeUA, nil); r != "off" {
		t.Fatalf("forms are off: %s", r)
	}
	if r := a.hit(map[string]any{"n": "404", "u": "https://shop.test/x", "d": "shop.test"}, "3.3.3.3", chromeUA, nil); r != "off" {
		t.Fatalf("404s are off: %s", r)
	}
	a.hit(map[string]any{"n": "pageview", "u": "https://shop.test/search?q=cake&find=bread", "d": "shop.test"}, "3.3.3.3", chromeUA, nil)
	a.hit(map[string]any{"n": "engagement", "u": "https://shop.test/search", "d": "shop.test", "e": 3000, "sd": 10, "v": map[string]any{"l": 100}}, "3.3.3.3", chromeUA, nil)
	a.s.waitFlushed()
	one("dim=search", "gluten free") // nothing new: searches are off
	if all := a.ok(a.alice, "GET", base+"/speed?range=today", nil)["all"].(map[string]any); num(all, "loads") != 2 {
		t.Fatalf("speed is off, yet: %v", all)
	}
	a.ok(a.alice, "PUT", base, map[string]any{"options": map[string]any{"search": true}})
	a.hit(map[string]any{"n": "pageview", "u": "https://shop.test/search?q=cake&find=Bread", "d": "shop.test"}, "4.4.4.4", chromeUA, nil)
	a.s.waitFlushed()
	if r := rows("dim=search"); len(r) != 2 {
		t.Fatalf("the site's own search name: %v", r)
	}
}

// The script the websites load says everything it measures.
func TestScriptHasTheNewParts(t *testing.T) {
	for _, part := range []string{"largest-contentful-paint", "layout-shift", "Form: Submission", "Contact Link: Click", "responseStatus", "data-pocket-404", "p.$ = opts.rev"} {
		if !strings.Contains(trackerJS, part) {
			t.Fatalf("the script lacks %q", part)
		}
	}
	if strings.Contains(trackerJS, "\n//") || len(trackerJS) > 12000 {
		t.Fatalf("the script is %d bytes or has comment lines", len(trackerJS))
	}
}
