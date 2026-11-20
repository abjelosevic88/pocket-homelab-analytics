package server

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
)

// ---- demo data (DEMO_DATA=true) ----
//
// Two websites with four months of visits: a bakery in Sarajevo (orders,
// a menu to download, Instagram) and a small blog that once landed on
// Hacker News. Enough to try everything and for the screenshots.

type pick[T any] struct {
	v T
	w int
}

func choose[T any](rng *rand.Rand, list []pick[T]) T {
	total := 0
	for _, p := range list {
		total += p.w
	}
	n := rng.Intn(total)
	for _, p := range list {
		if n < p.w {
			return p.v
		}
		n -= p.w
	}
	return list[0].v
}

type demoSource struct{ name, channel, ref, utmSource, utmMedium, utmCampaign string }

type demoPlace struct{ country, region, city string }

type demoDevice struct{ device, os, osv, browser, bv string }

var demoPlaces = []pick[demoPlace]{
	{demoPlace{"BA", "Federation of Bosnia and Herzegovina", "Sarajevo"}, 30}, {demoPlace{"BA", "Republika Srpska", "Banja Luka"}, 5},
	{demoPlace{"BA", "Federation of Bosnia and Herzegovina", "Mostar"}, 4}, {demoPlace{"BA", "Federation of Bosnia and Herzegovina", "Tuzla"}, 3},
	{demoPlace{"HR", "City of Zagreb", "Zagreb"}, 7}, {demoPlace{"HR", "Split-Dalmatia", "Split"}, 3}, {demoPlace{"RS", "Belgrade", "Belgrade"}, 7},
	{demoPlace{"RS", "Vojvodina", "Novi Sad"}, 2}, {demoPlace{"DE", "Berlin", "Berlin"}, 4}, {demoPlace{"DE", "Bavaria", "Munich"}, 3},
	{demoPlace{"DE", "Hesse", "Frankfurt am Main"}, 2}, {demoPlace{"AT", "Vienna", "Vienna"}, 5}, {demoPlace{"AT", "Styria", "Graz"}, 1},
	{demoPlace{"US", "New York", "New York"}, 4}, {demoPlace{"US", "California", "San Francisco"}, 3}, {demoPlace{"US", "Illinois", "Chicago"}, 1},
	{demoPlace{"GB", "England", "London"}, 3}, {demoPlace{"NL", "North Holland", "Amsterdam"}, 2}, {demoPlace{"SI", "Ljubljana", "Ljubljana"}, 2},
	{demoPlace{"CH", "Zurich", "Zurich"}, 2}, {demoPlace{"SE", "Stockholm", "Stockholm"}, 1}, {demoPlace{"TR", "Istanbul", "Istanbul"}, 1},
}

var techPlaces = []pick[demoPlace]{
	{demoPlace{"US", "California", "San Francisco"}, 14}, {demoPlace{"US", "New York", "New York"}, 9}, {demoPlace{"US", "Washington", "Seattle"}, 5},
	{demoPlace{"US", "Texas", "Austin"}, 3}, {demoPlace{"GB", "England", "London"}, 8}, {demoPlace{"DE", "Berlin", "Berlin"}, 7},
	{demoPlace{"DE", "Bavaria", "Munich"}, 3}, {demoPlace{"NL", "North Holland", "Amsterdam"}, 4}, {demoPlace{"FR", "Île-de-France", "Paris"}, 4},
	{demoPlace{"CA", "Ontario", "Toronto"}, 4}, {demoPlace{"IN", "Karnataka", "Bengaluru"}, 4}, {demoPlace{"BA", "Federation of Bosnia and Herzegovina", "Sarajevo"}, 5},
	{demoPlace{"SE", "Stockholm", "Stockholm"}, 2}, {demoPlace{"PL", "Mazovia", "Warsaw"}, 2}, {demoPlace{"AU", "New South Wales", "Sydney"}, 2},
	{demoPlace{"JP", "Tokyo", "Tokyo"}, 1}, {demoPlace{"BR", "São Paulo", "São Paulo"}, 1}, {demoPlace{"CH", "Zurich", "Zurich"}, 2},
}

var demoDevices = []pick[demoDevice]{
	{demoDevice{"Phone", "iOS", "18", "Safari", "18"}, 26}, {demoDevice{"Phone", "iOS", "17", "Safari", "17"}, 6}, {demoDevice{"Phone", "iOS", "18", "Chrome", "131"}, 5},
	{demoDevice{"Phone", "Android", "14", "Chrome", "131"}, 16}, {demoDevice{"Phone", "Android", "13", "Samsung Internet", "26"}, 5}, {demoDevice{"Phone", "Android", "14", "Firefox", "133"}, 1},
	{demoDevice{"Tablet", "iPadOS", "18", "Safari", "18"}, 4}, {demoDevice{"Laptop", "macOS", "15", "Safari", "18"}, 6}, {demoDevice{"Laptop", "macOS", "15", "Chrome", "131"}, 5},
	{demoDevice{"Laptop", "Windows", "10/11", "Chrome", "131"}, 6}, {demoDevice{"Desktop", "Windows", "10/11", "Chrome", "131"}, 7}, {demoDevice{"Desktop", "Windows", "10/11", "Edge", "131"}, 4},
	{demoDevice{"Desktop", "macOS", "15", "Firefox", "133"}, 2}, {demoDevice{"Desktop", "Linux", "", "Firefox", "133"}, 2},
}

var techDevices = []pick[demoDevice]{
	{demoDevice{"Desktop", "macOS", "15", "Chrome", "131"}, 14}, {demoDevice{"Laptop", "macOS", "15", "Safari", "18"}, 10}, {demoDevice{"Desktop", "Linux", "", "Firefox", "133"}, 12},
	{demoDevice{"Desktop", "Windows", "10/11", "Chrome", "131"}, 9}, {demoDevice{"Laptop", "macOS", "15", "Firefox", "133"}, 6}, {demoDevice{"Phone", "iOS", "18", "Safari", "18"}, 12},
	{demoDevice{"Phone", "Android", "14", "Chrome", "131"}, 7}, {demoDevice{"Desktop", "Linux", "", "Chrome", "131"}, 5}, {demoDevice{"Desktop", "Windows", "10/11", "Edge", "131"}, 2},
	{demoDevice{"Tablet", "iPadOS", "18", "Safari", "18"}, 2},
}

var languages = []pick[string]{{"bs", 30}, {"en", 30}, {"hr", 10}, {"sr", 8}, {"de", 10}, {"nl", 2}, {"sl", 2}, {"fr", 2}, {"tr", 1}, {"sv", 1}}

// demoEvent is something a demo visitor does on a page; rev is money in euros.
type demoEvent struct {
	name, props string
	rev         float64
}

// demoMissing is a page that does not exist and the page that links to it.
type demoMissing struct{ path, from string }

type demoSite struct {
	domain, name string
	perDay       float64
	sources      []pick[demoSource]
	pages        []pick[string]
	next         map[string][]pick[string] // where people go next ("" = leave)
	places       []pick[demoPlace]
	devices      []pick[demoDevice]
	langs        []pick[string]
	events       func(rng *rand.Rand, path string) []demoEvent
	missing      []pick[demoMissing] // broken links people land on
	slow         map[string]float64  // pages that take longer to show (1 = as the rest)
	notes        map[int]string      // day (0 = 120 days ago) → what happened
	goals        []store.Goal
	funnels      []store.Funnel
}

func demoSites() []demoSite {
	bakery := demoSite{domain: "maplebakery.example", name: "Maple Bakery", perDay: 140, places: demoPlaces, devices: demoDevices, langs: languages,
		sources: []pick[demoSource]{
			{demoSource{"Direct", "Direct", "", "", "", ""}, 30}, {demoSource{"Google", "Search", "google.com", "", "", ""}, 30},
			{demoSource{"Instagram", "Social", "instagram.com", "", "", ""}, 11}, {demoSource{"Facebook", "Social", "facebook.com", "", "", ""}, 6},
			{demoSource{"Bing", "Search", "bing.com", "", "", ""}, 3}, {demoSource{"DuckDuckGo", "Search", "duckduckgo.com", "", "", ""}, 2},
			{demoSource{"ChatGPT", "AI assistants", "chatgpt.com", "", "", ""}, 3}, {demoSource{"Perplexity", "AI assistants", "perplexity.ai", "", "", ""}, 1},
			{demoSource{"newsletter", "Email", "", "newsletter", "email", "november-cakes"}, 5}, {demoSource{"Instagram", "Paid", "", "instagram", "paid_social", "spring-cakes"}, 3},
			{demoSource{"klix.ba", "Other sites", "klix.ba/lifestyle/hrana", "", "", ""}, 3}, {demoSource{"tripadvisor.com", "Other sites", "tripadvisor.com/Restaurant_Review", "", "", ""}, 3},
			{demoSource{"Gmail", "Email", "mail.google.com", "", "", ""}, 1},
		},
		pages: []pick[string]{{"/", 50}, {"/menu", 14}, {"/cakes", 9}, {"/order", 5}, {"/visit-us", 6}, {"/blog/sourdough-at-home", 7}, {"/blog/best-burek-in-town", 4}, {"/catering", 3}, {"/gift-cards", 2}},
		next: map[string][]pick[string]{
			"/":                        {{"", 40}, {"/menu", 25}, {"/cakes", 12}, {"/visit-us", 10}, {"/order", 6}, {"/catering", 4}, {"/about", 3}},
			"/menu":                    {{"", 45}, {"/order", 22}, {"/cakes", 13}, {"/visit-us", 12}, {"/", 8}},
			"/cakes":                   {{"", 35}, {"/cakes/birthday", 25}, {"/cakes/wedding", 12}, {"/order", 20}, {"/menu", 8}},
			"/cakes/birthday":          {{"", 40}, {"/order", 45}, {"/cakes", 15}},
			"/cakes/wedding":           {{"", 50}, {"/catering", 20}, {"/order", 30}},
			"/order":                   {{"", 45}, {"/order/thanks", 55}},
			"/order/thanks":            {{"", 85}, {"/", 15}},
			"/visit-us":                {{"", 80}, {"/menu", 20}},
			"/blog/sourdough-at-home":  {{"", 70}, {"/", 15}, {"/menu", 15}},
			"/blog/best-burek-in-town": {{"", 65}, {"/menu", 25}, {"/visit-us", 10}},
			"/catering":                {{"", 55}, {"/order", 30}, {"/menu", 15}},
			"/gift-cards":              {{"", 60}, {"/order", 40}},
			"/about":                   {{"", 70}, {"/visit-us", 30}},
		},
		events: func(rng *rand.Rand, path string) []demoEvent {
			var out []demoEvent
			add := func(n, p string) { out = append(out, demoEvent{name: n, props: p}) }
			switch path {
			case "/menu":
				if rng.Intn(100) < 18 {
					add("File Download", `{"url":"https://maplebakery.example/files/menu-autumn.pdf"}`)
				}
				if rng.Intn(100) < 9 {
					add("Search", fmt.Sprintf(`{"term":%q}`, choose(rng, []pick[string]{{"burek", 30}, {"gluten free", 22}, {"birthday cake", 18}, {"baklava", 12}, {"vegan", 10}, {"sourdough", 8}, {"opening hours", 7}, {"delivery", 6}, {"gift card", 3}})))
				}
			case "/visit-us":
				if rng.Intn(100) < 35 {
					add("Outbound Link: Click", `{"url":"https://maps.google.com/maps"}`)
				}
				if rng.Intn(100) < 16 {
					add("Contact Link: Click", `{"kind":"phone","to":"+387 33 555 010"}`)
				} else if rng.Intn(100) < 6 {
					add("Contact Link: Click", `{"kind":"email","to":"hello@maplebakery.example"}`)
				}
			case "/catering":
				if rng.Intn(100) < 14 {
					add("Form: Submission", `{"form":"Catering enquiry"}`)
				}
				if rng.Intn(100) < 8 {
					add("Contact Link: Click", `{"kind":"email","to":"catering@maplebakery.example"}`)
				}
			case "/order/thanks":
				kind := []string{"pickup", "pickup", "delivery"}[rng.Intn(3)]
				size := rng.Intn(3)
				out = append(out, demoEvent{name: "Order placed", props: fmt.Sprintf(`{"type":%q,"size":%q}`, kind, []string{"small", "medium", "large"}[size]),
					rev: []float64{18, 32, 54}[size] + float64(rng.Intn(12))})
				add("Form: Submission", `{"form":"Order form"}`)
			case "/":
				if rng.Intn(100) < 6 {
					add("Outbound Link: Click", `{"url":"https://instagram.com/maplebakery"}`)
				}
				if rng.Intn(100) < 3 {
					add("Newsletter signup", `{"where":"footer"}`)
					add("Form: Submission", `{"form":"Newsletter"}`)
				}
			case "/blog/sourdough-at-home":
				if rng.Intn(100) < 8 {
					add("Newsletter signup", `{"where":"article"}`)
					add("Form: Submission", `{"form":"Newsletter"}`)
				}
				if rng.Intn(100) < 10 {
					add("File Download", `{"url":"https://maplebakery.example/files/sourdough-recipe.pdf"}`)
				}
			}
			return out
		},
		missing: []pick[demoMissing]{{demoMissing{"/cakes/christmas-2024", "klix.ba/lifestyle/hrana"}, 40}, {demoMissing{"/menu.pdf", ""}, 22}, {demoMissing{"/blog/burek", "/blog/best-burek-in-town"}, 18},
			{demoMissing{"/cakes/christmas-2024", "facebook.com/"}, 12}, {demoMissing{"/contact", "tripadvisor.com/Restaurant_Review"}, 8}},
		slow:  map[string]float64{"/cakes": 1.9, "/cakes/birthday": 1.6, "/cakes/wedding": 2.3, "/": 1.1},
		notes: map[int]string{62: "New menu page", 108: "The November newsletter went out"},
		goals: []store.Goal{{Name: "Order placed", Kind: "event", Match: "Order placed"}, {Name: "Newsletter signup", Kind: "event", Match: "Newsletter signup"},
			{Name: "Looked at cakes", Kind: "page", Match: "/cakes*"}, {Name: "Downloaded the menu", Kind: "event", Match: "File Download"}},
		funnels: []store.Funnel{{Name: "From the menu to an order", Steps: []store.Step{{Kind: "page", Match: "/menu", Label: "Menu"}, {Kind: "page", Match: "/order", Label: "Order form"}, {Kind: "event", Match: "Order placed", Label: "Ordered"}}},
			{Name: "Cakes to order", Steps: []store.Step{{Kind: "page", Match: "/cakes", Label: "Cakes"}, {Kind: "page", Match: "/cakes/*", Label: "A cake"}, {Kind: "page", Match: "/order", Label: "Order form"}, {Kind: "event", Match: "Order placed", Label: "Ordered"}}}},
	}
	blog := demoSite{domain: "fieldnotes.example", name: "Field Notes", perDay: 45, places: techPlaces, devices: techDevices, langs: []pick[string]{{"en", 80}, {"de", 8}, {"fr", 4}, {"bs", 5}, {"nl", 3}},
		sources: []pick[demoSource]{
			{demoSource{"Google", "Search", "google.com", "", "", ""}, 30}, {demoSource{"Direct", "Direct", "", "", "", ""}, 22},
			{demoSource{"Hacker News", "Social", "news.ycombinator.com", "", "", ""}, 6}, {demoSource{"Reddit", "Social", "reddit.com/r/selfhosted", "", "", ""}, 8},
			{demoSource{"GitHub", "Other sites", "github.com", "", "", ""}, 9}, {demoSource{"DuckDuckGo", "Search", "duckduckgo.com", "", "", ""}, 6},
			{demoSource{"Lobsters", "Social", "lobste.rs", "", "", ""}, 3}, {demoSource{"Bluesky", "Social", "bsky.app", "", "", ""}, 3},
			{demoSource{"ChatGPT", "AI assistants", "chatgpt.com", "", "", ""}, 4}, {demoSource{"Perplexity", "AI assistants", "perplexity.ai", "", "", ""}, 2},
			{demoSource{"Claude", "AI assistants", "claude.ai", "", "", ""}, 2}, {demoSource{"Mastodon", "Social", "mastodon.social", "", "", ""}, 3},
		},
		pages: []pick[string]{{"/", 18}, {"/posts/self-hosting-on-a-raspberry-pi", 22}, {"/posts/sqlite-is-enough", 18}, {"/posts/one-binary-apps", 12}, {"/posts/backups-that-work", 9}, {"/posts/tailscale-at-home", 8}, {"/about", 3}, {"/now", 2}},
		next: map[string][]pick[string]{
			"/":      {{"", 45}, {"/posts/sqlite-is-enough", 15}, {"/posts/self-hosting-on-a-raspberry-pi", 15}, {"/about", 10}, {"/now", 5}, {"/posts/one-binary-apps", 10}},
			"/about": {{"", 70}, {"/now", 15}, {"/", 15}},
			"/now":   {{"", 85}, {"/", 15}},
		},
		events: func(rng *rand.Rand, path string) []demoEvent {
			var out []demoEvent
			if len(path) > 7 && path[:7] == "/posts/" {
				if rng.Intn(100) < 22 {
					out = append(out, demoEvent{name: "Outbound Link: Click", props: []string{`{"url":"https://github.com/abjelosevic88"}`, `{"url":"https://sqlite.org/whentouse.html"}`, `{"url":"https://tailscale.com/kb"}`}[rng.Intn(3)]})
				}
				if rng.Intn(100) < 4 {
					out = append(out, demoEvent{name: "Subscribed (RSS)"})
				}
			}
			if path == "/" && rng.Intn(100) < 7 {
				out = append(out, demoEvent{name: "Search", props: fmt.Sprintf(`{"term":%q}`, choose(rng, []pick[string]{{"sqlite", 30}, {"backup", 20}, {"raspberry pi", 20}, {"tailscale", 15}, {"docker", 10}, {"rss", 5}}))})
			}
			return out
		},
		missing: []pick[demoMissing]{{demoMissing{"/posts/sqlite-is-enough.html", "news.ycombinator.com/item"}, 50}, {demoMissing{"/feed", ""}, 30}, {demoMissing{"/posts/hello-world", "github.com/abjelosevic88"}, 20}},
		slow:    map[string]float64{"/posts/self-hosting-on-a-raspberry-pi": 1.5},
		notes:   map[int]string{100: "On the front page of Hacker News"},
		goals:   []store.Goal{{Name: "Read a post", Kind: "page", Match: "/posts/*"}, {Name: "Subscribed (RSS)", Kind: "event", Match: "Subscribed (RSS)"}},
	}
	return []demoSite{bakery, blog}
}

func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if sites, _ := s.store.ListSites(ctx, uid); len(sites) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	st, _ := s.store.GetSettings(ctx, uid)
	st.TZ = "Europe/Sarajevo"
	st.ReportTo = ""
	if err := s.store.SaveUserSettings(ctx, uid, st); err != nil {
		return err
	}
	loc := zone(st)
	now := time.Now()
	for si, ds := range demoSites() {
		site := &store.Site{UserID: uid, Domain: ds.domain, Name: ds.name, TZ: "Europe/Sarajevo", Also: []string{}, Options: store.DefaultOptions(), ProxyKey: store.RandomToken(24)}
		if si == 0 {
			site.Options.Clicks = true
		}
		if err := s.store.CreateSite(ctx, site); err != nil {
			return err
		}
		for _, g := range ds.goals {
			g.SiteID = site.ID
			if err := s.store.CreateGoal(ctx, uid, &g); err != nil {
				return err
			}
		}
		for _, f := range ds.funnels {
			f.SiteID = site.ID
			if err := s.store.CreateFunnel(ctx, uid, &f); err != nil {
				return err
			}
		}
		for d, text := range ds.notes {
			n := store.Note{SiteID: site.ID, Day: dayStart(now.In(loc)).AddDate(0, 0, d-120).Format("2006-01-02"), Text: text}
			if err := s.store.CreateNote(ctx, uid, &n); err != nil {
				return err
			}
		}
		lost := map[string][]pick[string]{} // a missing page → the pages with the link to it
		for _, m := range ds.missing {
			lost[m.v.path] = append(lost[m.v.path], pick[string]{m.v.from, m.w})
		}
		cache := store.VisitCache{}
		var batch []store.Incoming
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			_, err := s.store.SaveHits(ctx, batch, cache)
			batch = batch[:0]
			return err
		}
		start := dayStart(now.In(loc)).AddDate(0, 0, -120)
		var visits []store.Incoming // first hits, sorted by time later
		for d := 0; d <= 120; d++ {
			day := start.AddDate(0, 0, d)
			trend := 0.55 + 0.45*float64(d)/120
			weekday := []float64{0.75, 1.05, 1.0, 1.0, 1.05, 1.15, 0.9}[day.Weekday()]
			if si == 1 {
				weekday = []float64{0.7, 1.1, 1.1, 1.05, 1.0, 0.95, 0.7}[day.Weekday()]
			}
			n := ds.perDay * trend * weekday * (0.85 + 0.3*rng.Float64())
			spike := si == 1 && d == 100
			if spike {
				n *= 14
			}
			if si == 0 && d >= 108 && d <= 111 {
				n *= 1.6 // the newsletter went out
			}
			var today []store.Incoming
			for i := 0; i < int(n); i++ {
				var at time.Time
				if d == 120 {
					// today: up to now, a few right now
					sec := rng.Intn(int(now.Sub(day).Seconds()) + 1)
					if i < 4 {
						sec = int(now.Sub(day).Seconds()) - rng.Intn(240)
					}
					at = day.Add(time.Duration(sec) * time.Second)
				} else {
					// more in the day than at night
					h := choose(rng, []pick[int]{{7, 3}, {8, 5}, {9, 6}, {10, 7}, {11, 7}, {12, 8}, {13, 7}, {14, 6}, {15, 6}, {16, 6}, {17, 7}, {18, 8}, {19, 8}, {20, 7}, {21, 6}, {22, 4}, {23, 2}, {0, 1}, {1, 1}, {6, 2}})
					if si == 1 {
						h = (h + 6) % 24 // readers in other zones
					}
					at = day.Add(time.Duration(h)*time.Hour + time.Duration(rng.Intn(3600))*time.Second)
				}
				src := choose(rng, ds.sources)
				if spike && rng.Intn(100) < 80 {
					src = demoSource{"Hacker News", "Social", "news.ycombinator.com", "", "", ""}
				}
				if si == 0 && d >= 108 && d <= 111 && rng.Intn(100) < 40 {
					src = demoSource{"newsletter", "Email", "", "newsletter", "email", "november-cakes"}
				}
				pl := choose(rng, ds.places)
				dv := choose(rng, ds.devices)
				entry := choose(rng, ds.pages)
				if spike && src.name == "Hacker News" {
					entry = "/posts/sqlite-is-enough"
				}
				if src.utmCampaign == "spring-cakes" {
					entry = "/cakes"
				}
				if src.utmCampaign == "november-cakes" && rng.Intn(100) < 70 {
					entry = "/cakes/birthday"
				}
				if len(ds.missing) > 0 && src.utmCampaign == "" && rng.Intn(1000) < 14 {
					entry = choose(rng, ds.missing).path // an old link somewhere
				}
				v := store.Visit{Hostname: ds.domain, Referrer: src.ref, Source: src.name, Channel: src.channel, UTMSource: src.utmSource, UTMMedium: src.utmMedium, UTMCampaign: src.utmCampaign,
					Country: pl.country, Region: pl.region, City: pl.city, Device: dv.device, Browser: dv.browser, BrowserVersion: dv.bv, OS: dv.os, OSVersion: dv.osv, Language: choose(rng, ds.langs)}
				who := fmt.Sprintf("demo%d-%d-%d", si, d, i)
				if len(today) > 0 && rng.Intn(100) < 18 {
					// someone already here today comes back (same browser, same place)
					prev := today[rng.Intn(len(today))]
					who, v.Country, v.Region, v.City, v.Device, v.Browser, v.BrowserVersion, v.OS, v.OSVersion, v.Language =
						prev.Visitor, prev.Visit.Country, prev.Visit.Region, prev.Visit.City, prev.Visit.Device, prev.Visit.Browser, prev.Visit.BrowserVersion, prev.Visit.OS, prev.Visit.OSVersion, prev.Visit.Language
				}
				in := store.Incoming{SiteID: site.ID, Visitor: who, TS: at.Unix(), Name: "pageview", Path: entry, Visit: v}
				visits = append(visits, in)
				today = append(today, in)
			}
		}
		// in time order, so a returning visitor's visits follow each other
		sort.Slice(visits, func(i, j int) bool { return visits[i].TS < visits[j].TS })
		for _, first := range visits {
			if first.TS > now.Unix() {
				continue
			}
			path, t := first.Path, first.TS
			for step := 0; step < 9; step++ {
				in := first
				in.TS, in.Path = t, path
				batch = append(batch, in)
				read := int64(5 + rng.ExpFloat64()*55)
				if path == "/order" {
					read += 60
				}
				if len(path) > 7 && (path[:6] == "/blog/" || path[:7] == "/posts/") {
					read = int64(20 + rng.ExpFloat64()*150)
				}
				if t+read > now.Unix() {
					read = max(now.Unix()-t, 1)
				}
				scroll := int(math.Min(100, 25+rng.Float64()*85))
				if from, gone := lost[path]; gone {
					e := first
					e.TS, e.Name, e.Path, e.Props = t, "404", path, fmt.Sprintf(`{"from":%q}`, choose(rng, from))
					batch = append(batch, e)
				}
				for _, ev := range ds.events(rng, path) {
					e := first
					e.TS, e.Name, e.Path, e.Props = t+read/2, ev.name, path, ev.props
					if ev.rev > 0 {
						e.Revenue, e.Currency = ev.rev, "EUR"
					}
					batch = append(batch, e)
				}
				if ds.domain == "maplebakery.example" && rng.Intn(100) < 25 {
					e := first
					e.TS, e.Name, e.Path = t+read/3, "Click", path
					e.Props = choose(rng, []pick[string]{{`{"text":"Order a cake"}`, 30}, {`{"text":"See the menu"}`, 25}, {`{"text":"Opening hours"}`, 15}, {`{"text":"Call us"}`, 10}, {`{"text":"Gift cards"}`, 5}, {`{"text":"Instagram"}`, 8}, {`{"text":"(button)"}`, 4}})
					batch = append(batch, e)
				}
				eng := first
				eng.TS, eng.Name, eng.Path, eng.Engaged, eng.Scroll = t+read, "engagement", path, read*1000*int64(60+rng.Intn(40))/100, scroll
				if step == 0 {
					eng.Vitals = demoVitals(rng, first.Visit.Device, ds.slow[path])
				}
				batch = append(batch, eng)
				nexts, ok := ds.next[path]
				if !ok {
					nexts = []pick[string]{{"", 70}, {"/", 30}}
				}
				if _, gone := lost[path]; gone {
					nexts = []pick[string]{{"", 55}, {"/", 45}}
				}
				nx := choose(rng, nexts)
				if nx == "" || t+read+5 > now.Unix() {
					break
				}
				path, t = nx, t+read+int64(2+rng.Intn(8))
			}
			if len(batch) > 3000 {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if err := flush(); err != nil {
			return err
		}
	}
	s.reloadSites(ctx)
	s.log.Info("demo data ready")
	return nil
}

// demoVitals is how fast a page was for one demo visitor: phones are
// slower, some pages are heavy, and now and then something is very slow.
func demoVitals(rng *rand.Rand, device string, heavy float64) store.Vitals {
	if heavy == 0 {
		heavy = 1
	}
	f := 1.0
	if device == "Phone" || device == "Tablet" {
		f = 1.55
	}
	if rng.Intn(100) < 6 {
		f *= 2.4 // a bad connection
	}
	ms := func(v float64) *int64 { n := int64(v); return &n }
	ttfb := 90 + rng.ExpFloat64()*160*f
	fcp := ttfb + (260+rng.ExpFloat64()*240)*f
	lcp := fcp + (150+rng.ExpFloat64()*330)*f*heavy*heavy
	v := store.Vitals{TTFB: ms(ttfb), FCP: ms(fcp), LCP: ms(lcp)}
	cls := float64(int(rng.ExpFloat64()*28*heavy*heavy)) / 1000
	if rng.Intn(100) < 45 {
		cls = 0
	}
	v.CLS = &cls
	if rng.Intn(100) < 72 { // most people press something
		v.INP = ms((36 + rng.ExpFloat64()*44) * f)
	}
	return v
}
