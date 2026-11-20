package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Analytics
// as the target: another Pocket app the person allowed in Core may act here
// for them with a pcx_ token, only within these scopes. Every route not
// listed in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "analytics:read", Name: "See who visits your websites", Description: "Read your websites and their numbers — visitors, pages, where people came from, countries, devices, goals, the people on a site right now — for your day, search, dashboards, a blog's view counts and Pocket AI. Never anything that tells one visitor from another."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(summary|sites|search|day/\d{4}-\d{2}-\d{2})$`), "analytics:read"},
	{"GET", regexp.MustCompile(`^/api/v1/sites/\d+$`), "analytics:read"},
	{"GET", regexp.MustCompile(`^/api/v1/sites/\d+/(stats|breakdown|conversions|live|events)$`), "analytics:read"},
	{"GET", regexp.MustCompile(`^/api/v1/sites/\d+/funnels/\d+/counts$`), "analytics:read"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "analytics" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
