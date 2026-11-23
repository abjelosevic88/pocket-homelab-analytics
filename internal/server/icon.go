package server

// A website's own icon, for the list of websites. The box reads the site's
// home page once a week, takes the icon it names (<link rel="icon">, else
// /favicon.ico) and keeps it, so the dashboard shows it without asking the
// site from the browser each time.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	iconKeep    = 7 * 24 * time.Hour // a found icon
	iconRetry   = 6 * time.Hour      // a site without one
	iconMaxPage = 512 << 10
	iconMaxFile = 256 << 10
	// not a robot name, so a site's own robot counting leaves it out
	iconAgent = "Mozilla/5.0 (compatible; PocketAnalytics/1.0; +site icon)"
)

// iconHTTP reaches public addresses only: a website named here can't make
// the box read something on its own network.
var iconHTTP = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || isCGNAT(ip) {
				return errors.New("not a public address")
			}
			return nil
		}}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" && req.URL.Scheme != "http" {
			return errors.New("not a web address")
		}
		return nil
	},
}

// 100.64.0.0/10: carrier and Tailscale addresses
func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

func (s *Server) handleSiteIcon(w http.ResponseWriter, r *http.Request) {
	site, ok := s.mySite(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	key := "icon:" + site.Domain
	v, at, cached := s.store.CacheGet(ctx, key)
	if cached {
		t, err := time.Parse(time.RFC3339Nano, at)
		keep := iconKeep
		if v == "" {
			keep = iconRetry
		}
		cached = err == nil && time.Since(t) < keep
	}
	if !cached {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		ctype, body := findIcon(fctx, site.Domain)
		cancel()
		v = ""
		if body != nil {
			v = ctype + ";" + base64.StdEncoding.EncodeToString(body)
		}
		_ = s.store.CachePut(ctx, key, v)
	}
	ctype, enc, _ := strings.Cut(v, ";")
	body, err := base64.StdEncoding.DecodeString(enc)
	if v == "" || err != nil || len(body) == 0 {
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.Error(w, "no icon", http.StatusNotFound)
		return
	}
	h := w.Header()
	h.Set("Content-Type", ctype)
	h.Set("Content-Length", strconv.Itoa(len(body)))
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("X-Content-Type-Options", "nosniff")
	// an SVG opened on its own must not run anything
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	_, _ = w.Write(body)
}

var (
	reLinkTag = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	reAttr    = regexp.MustCompile(`(?is)\b(rel|href|sizes|type)\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
)

type iconLink struct {
	href  string
	score int
}

// iconLinks reads the icons a page names, best first: drawn (SVG), then the
// size nearest 64 px, then whatever else.
func iconLinks(page string, base *url.URL) []iconLink {
	var out []iconLink
	for _, tag := range reLinkTag.FindAllString(page, -1) {
		a := map[string]string{}
		for _, m := range reAttr.FindAllStringSubmatch(tag, -1) {
			a[strings.ToLower(m[1])] = strings.Trim(m[2], `"'`)
		}
		rel := " " + strings.ToLower(a["rel"]) + " "
		if !strings.Contains(rel, " icon ") && !strings.Contains(rel, " apple-touch-icon ") && !strings.Contains(rel, " apple-touch-icon-precomposed ") {
			continue
		}
		u, err := base.Parse(strings.TrimSpace(a["href"]))
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		score := 50
		typ := strings.ToLower(a["type"])
		if typ == "image/svg+xml" || strings.HasSuffix(strings.ToLower(u.Path), ".svg") {
			score = 100
		} else if sz := strings.ToLower(a["sizes"]); sz != "" && sz != "any" {
			n, _ := strconv.Atoi(strings.SplitN(sz, "x", 2)[0])
			d := n - 64
			if d < 0 {
				d = -d * 4 // smaller than wanted looks blurry; larger shrinks well
			}
			score = 90 - min(d/4, 60)
		} else if strings.Contains(rel, "apple-touch-icon") {
			score = 60
		}
		out = append(out, iconLink{u.String(), score})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}

// findIcon returns the site's icon, or nil when it has none.
func findIcon(ctx context.Context, domain string) (string, []byte) {
	var tries []string
	home := "https://" + domain + "/"
	if page, final, err := iconGet(ctx, home, iconMaxPage); err == nil {
		for _, l := range iconLinks(string(page), final) {
			tries = append(tries, l.href)
		}
		tries = append(tries, final.ResolveReference(&url.URL{Path: "/favicon.ico"}).String())
	}
	tries = append(tries, "https://"+domain+"/favicon.ico")
	seen := map[string]bool{}
	for i, u := range tries {
		if seen[u] || i > 6 {
			continue
		}
		seen[u] = true
		body, _, err := iconGet(ctx, u, iconMaxFile)
		if err != nil || len(body) == 0 {
			continue
		}
		if ct := iconType(body); ct != "" {
			return ct, body
		}
	}
	return "", nil
}

func iconGet(ctx context.Context, u string, max int64) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", iconAgent)
	req.Header.Set("Accept", "text/html,image/*;q=0.9,*/*;q=0.5")
	res, err := iconHTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return nil, nil, err
	}
	if int64(len(body)) > max {
		if max == iconMaxPage { // the icons are named near the top
			body = body[:max]
		} else {
			return nil, nil, errors.New("too large")
		}
	}
	return body, res.Request.URL, nil
}

// iconType is the picture's kind from its first bytes; "" when it isn't one.
func iconType(b []byte) string {
	head := strings.ToLower(strings.TrimSpace(string(b[:min(len(b), 512)])))
	if strings.HasPrefix(head, "<svg") || (strings.HasPrefix(head, "<?xml") && strings.Contains(head, "<svg")) || (strings.HasPrefix(head, "<!--") && strings.Contains(head, "<svg")) {
		return "image/svg+xml"
	}
	if len(b) >= 4 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0 {
		return "image/x-icon"
	}
	switch ct := http.DetectContentType(b); ct {
	case "image/png", "image/gif", "image/jpeg", "image/webp", "image/x-icon", "image/bmp":
		return ct
	}
	return ""
}
