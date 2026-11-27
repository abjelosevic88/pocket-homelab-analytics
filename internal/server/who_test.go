package server

import (
	"strings"
	"testing"
)

// 0.9.0: a site that knows who visits keeps the address, who owns it, the
// place on the map, the browser's own ID (returning visitors) and what the
// browser says about itself; other sites keep none of it.
func TestKnowWhoVisits(t *testing.T) {
	a := demoApp(t)
	site := a.ok(a.alice, "POST", "/api/v1/sites", map[string]any{"domain": "who.test"})
	sid, key := id(site), str(site, "proxy_key")
	base := "/api/v1/sites/" + sid
	if strings.Contains(str(site, "snippet"), "data-identify") {
		t.Fatal("a new site does not know who visits")
	}
	// off: nothing about the person is kept
	a.hit(map[string]any{"n": "pageview", "u": "https://who.test/", "d": "who.test", "id": "abcdef0123456789", "x": map[string]any{"screen": "1920×1080"}}, "9.9.9.9", chromeUA, nil)
	a.s.waitFlushed()
	v0 := a.ok(a.alice, "GET", base+"/visits", nil)["visits"].([]any)[0].(map[string]any)
	if v0["ip"] != nil || v0["client_id"] != nil || v0["details"] != nil {
		t.Fatalf("kept without identify: %v", v0)
	}

	up := a.ok(a.alice, "PUT", base, map[string]any{"options": map[string]any{"identify": true, "forget_days": 90}})
	if !strings.Contains(str(up, "snippet"), "data-identify") {
		t.Fatalf("snippet: %s", str(up, "snippet"))
	}
	a.expect(400, a.alice, "PUT", base, map[string]any{"options": map[string]any{"forget_days": -1}})

	proxy := map[string]string{"X-Pocket-Proxy-Key": key, "X-Pocket-Client-IP": "203.0.113.9", "X-Pocket-Country": "CA", "X-Pocket-City": "Creston",
		"X-Pocket-ASN": "6327", "X-Pocket-Org": "Shaw Communications", "X-Pocket-Lat": "49.1", "X-Pocket-Lon": "-116.5", "X-Pocket-Postal": "V0B",
		"X-Pocket-Colo": "SEA", "Accept-Language": "en-CA,en;q=0.9"}
	x := map[string]any{"screen": "1440×900", "timezone": "America/Vancouver", "cores": 8, "languages": []string{"en-CA", "en"}, "gpu": "Apple M1", "evil": map[string]any{"no": 1}}
	// day one, then the same browser comes back from another address
	a.hit(map[string]any{"n": "pageview", "u": "https://who.test/docs?x=1", "d": "who.test", "r": "https://www.google.com/search?q=pocket", "id": "browser-0001", "x": x}, "1.1.1.1", chromeUA, proxy)
	a.s.waitFlushed()
	a.s.store.DB.Exec(`UPDATE visits SET started = started - 86400, last = last - 86400 WHERE client_id = 'browser-0001'`)
	proxy["X-Pocket-Client-IP"] = "203.0.113.50"
	a.hit(map[string]any{"n": "pageview", "u": "https://who.test/pricing", "d": "who.test", "id": "browser-0001"}, "1.1.1.1", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0", proxy)
	a.s.waitFlushed()

	list := a.ok(a.alice, "GET", base+"/visits?range=7d", nil)["visits"].([]any)
	latest := list[0].(map[string]any)
	vid := int64(latest["id"].(float64))
	one := a.ok(a.alice, "GET", base+"/visits/"+itoa(vid), nil)
	v := one["visit"].(map[string]any)
	if v["ip"] != "203.0.113.50" || v["network"] != "Shaw Communications" || v["asn"].(float64) != 6327 || v["postal"] != "V0B" || v["lat"].(float64) != 49.1 {
		t.Fatalf("who: %v", v)
	}
	if one["others_total"].(float64) != 1 || one["others"].([]any)[0].(map[string]any)["by"] != "browser" {
		t.Fatalf("same browser before: %v", one)
	}
	var first map[string]any
	for _, x := range list {
		if x.(map[string]any)["entry"] == "/docs" {
			first = x.(map[string]any)
		}
	}
	d := first["details"].(map[string]any)
	if d["screen"] != "1440×900" || d["languages"] != "en-CA, en" || d["edge"] != "SEA" || d["accept_language"] != "en-CA,en;q=0.9" || d["evil"] != nil || d["gpu"] != "Apple M1" {
		t.Fatalf("details: %v", d)
	}
	if first["ref_url"] != "https://www.google.com/search?q=pocket" {
		t.Fatalf("whole referrer: %v", first["ref_url"])
	}
	// lists: networks, addresses, new or returning, screens
	for dim, want := range map[string]string{"network": "Shaw Communications", "ip": "203.0.113.50", "returning": "Returning", "screen": "1440×900", "timezone": "America/Vancouver"} {
		rows := a.ok(a.alice, "GET", base+"/breakdown?range=7d&dim="+dim, nil)["rows"].([]any)
		found := false
		for _, r := range rows {
			if r.(map[string]any)["value"] == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: %v", dim, rows)
		}
	}
	// forgetting: who they were goes, the visit stays
	if n, err := a.s.store.ForgetWho(t.Context(), int64(up["id"].(float64)), 1<<40); err != nil || n != 2 {
		t.Fatalf("forget: %d %v", n, err)
	}
	v = a.ok(a.alice, "GET", base+"/visits/"+itoa(vid), nil)["visit"].(map[string]any)
	if v["ip"] != nil || v["network"] != nil || v["entry"] != "/pricing" {
		t.Fatalf("after forgetting: %v", v)
	}
}
