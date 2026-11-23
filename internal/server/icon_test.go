package server

import (
	"net/url"
	"testing"
)

func TestIconLinks(t *testing.T) {
	base, _ := url.Parse("https://example.com/blog/")
	page := `<head>
<link rel="stylesheet" href="/a.css">
<link rel="apple-touch-icon" href="/apple.png">
<link rel='icon' type='image/png' sizes='16x16' href='fav16.png'>
<link href="/fav.svg" rel="icon" type="image/svg+xml">
<link rel="shortcut icon" href="https://cdn.example.net/f.ico">
<link rel="icon" href="javascript:alert(1)">
</head>`
	got := iconLinks(page, base)
	want := []string{"https://example.com/fav.svg", "https://example.com/apple.png", "https://cdn.example.net/f.ico", "https://example.com/blog/fav16.png"}
	if len(got) != len(want) {
		t.Fatalf("got %d links: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].href != w {
			t.Errorf("link %d = %s, want %s", i, got[i].href, w)
		}
	}
}

func TestIconType(t *testing.T) {
	cases := map[string]string{
		"<svg xmlns='http://www.w3.org/2000/svg'></svg>":     "image/svg+xml",
		"<?xml version=\"1.0\"?>\n<svg></svg>":               "image/svg+xml",
		"\x89PNG\r\n\x1a\n0000":                              "image/png",
		"\x00\x00\x01\x00\x01\x00":                           "image/x-icon",
		"<!doctype html><html><body>not found</body></html>": "",
	}
	for in, want := range cases {
		if got := iconType([]byte(in)); got != want {
			t.Errorf("iconType(%q) = %q, want %q", in[:min(len(in), 12)], got, want)
		}
	}
}
