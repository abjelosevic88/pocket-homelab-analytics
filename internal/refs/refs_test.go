package refs

import (
	"net/url"
	"testing"
)

func TestClassify(t *testing.T) {
	own := func(h string) bool { return h == "example.com" || h == "www.example.com" }
	cases := []struct {
		page, ref, name, channel string
	}{
		{"https://example.com/", "", Direct, Direct},
		{"https://example.com/a", "https://www.example.com/", Direct, Direct},
		{"https://example.com/", "https://www.google.de/", "Google", Search},
		{"https://example.com/", "https://news.ycombinator.com/item?id=1", "Hacker News", Social},
		{"https://example.com/", "https://old.reddit.com/r/selfhosted/", "Reddit", Social},
		{"https://example.com/", "https://chatgpt.com/", "ChatGPT", AI},
		{"https://example.com/?utm_source=newsletter&utm_medium=email", "", "newsletter", Email},
		{"https://example.com/?utm_source=google&utm_medium=cpc", "", "Google", Paid},
		{"https://example.com/", "https://someblog.net/post/1", "someblog.net", Referral},
		{"https://example.com/?ref=producthunt", "", "Product Hunt", Social},
		{"https://example.com/", "https://mail.google.com/", "Gmail", Email},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.page)
		got := Classify(u, c.ref, own)
		if got.Name != c.name || got.Channel != c.channel {
			t.Errorf("%s from %q: got %q/%q, want %q/%q", c.page, c.ref, got.Name, got.Channel, c.name, c.channel)
		}
	}
}
