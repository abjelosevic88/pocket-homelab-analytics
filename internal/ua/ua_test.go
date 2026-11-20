package ua

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		ua, browser, ver, os, device string
		width                        int
	}{
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36", "Chrome", "131", "macOS", "Desktop", 1512 * 2},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1", "Safari", "18", "iOS", "Phone", 390},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0", "Edge", "131", "Windows", "Laptop", 1366},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:133.0) Gecko/20100101 Firefox/133.0", "Firefox", "133", "Linux", "Desktop", 0},
		{"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/26.0 Chrome/122.0.0.0 Mobile Safari/537.36", "Samsung Internet", "26", "Android", "Phone", 0},
	}
	for _, c := range cases {
		got := Parse(c.ua, c.width)
		if got.Browser != c.browser || got.BrowserVersion != c.ver || got.OS != c.os || got.Device != c.device || got.Bot {
			t.Errorf("%s: got %+v", c.ua, got)
		}
	}
	for _, b := range []string{"", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "curl/8.4.0", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36"} {
		if !IsBot(b) {
			t.Errorf("not a bot: %q", b)
		}
	}
}
