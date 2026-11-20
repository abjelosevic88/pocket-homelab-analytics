package server

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ---- host switch ----

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

func urlHost(u string) string {
	if p, err := url.Parse(u); err == nil {
		return strings.ToLower(p.Hostname())
	}
	return ""
}

// publicPath is what websites may reach: the script and the collector.
func publicPath(p string) bool {
	return p == "/api/event" || p == "/api/robot" || p == "/favicon.svg" || (strings.HasPrefix(p, "/js/") && strings.HasSuffix(p, ".js"))
}

// hostSwitch serves only the script and the collector on the public
// hostname; every other name (BASE_URL, host.docker.internal for Core and
// the apps acting through it, a LAN name) is the whole app.
func (s *Server) hostSwitch(next http.Handler) http.Handler {
	publicHost := urlHost(s.cfg.PublicURL)
	if publicHost == "" || publicHost == urlHost(s.cfg.BaseURL) {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hostOnly(r.Host) == publicHost && !publicPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// collectBase is the address websites send to (PUBLIC_URL, else BASE_URL).
func (s *Server) collectBase() string {
	return strings.TrimRight(firstNonEmpty(s.cfg.PublicURL, s.cfg.BaseURL), "/")
}

type originKey struct{}

// withOrigin remembers the address the request came in on, for when
// neither PUBLIC_URL nor BASE_URL is set (a quick local try).
func (s *Server) withOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.collectBase() == "" && r.Host != "" {
			scheme := "http"
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			r = r.WithContext(context.WithValue(r.Context(), originKey{}, scheme+"://"+r.Host))
		}
		next.ServeHTTP(w, r)
	})
}

// collectFor is collectBase, else the request's own address.
func (s *Server) collectFor(ctx context.Context) string {
	if b := s.collectBase(); b != "" {
		return b
	}
	o, _ := ctx.Value(originKey{}).(string)
	return o
}

// base is where people open the app.
func (s *Server) base() string { return strings.TrimRight(s.cfg.BaseURL, "/") }
