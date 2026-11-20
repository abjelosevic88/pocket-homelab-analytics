package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/abjelosevic88/pocket-homelab-analytics/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.withOrigin)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("analytics"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	// what websites load and send to (collect.go); the only paths on PUBLIC_URL
	r.Get("/js/script.js", s.handleScript)
	r.Get("/js/pa.js", s.handleScript)
	r.Get("/js/plausible.js", s.handleScript)
	r.Post("/api/event", s.handleEvent)
	r.Options("/api/event", s.handleEventOptions)

	r.Route("/api/v1", func(r chi.Router) {
		// imports and AI can take longer than a request may
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/import", s.handleImport)
			r.Post("/sites/{id}/explain", s.handleExplain)
			r.Post("/sites/{id}/make", s.handleMake)
			r.Get("/sites/{id}/export.csv", s.handleExportCSV)
		})

		r.Group(func(r chi.Router) {
			r.Use(middleware.Timeout(120 * time.Second))
			r.Post("/auth/login", s.handleLogin)
			r.Post("/auth/setup", s.handleSetup)
			r.Get("/auth/status", s.handleAuthStatus)
			r.Get("/auth/oidc/start", s.handleOIDCStart)
			r.Get("/auth/oidc/callback", s.handleOIDCCallback)

			r.Group(func(r chi.Router) {
				r.Use(s.requireAuth)
				r.Post("/auth/logout", s.handleLogout)
				r.Get("/auth/me", s.handleMe)
				r.Put("/auth/me", s.handleUpdateMe)

				// websites
				r.Get("/sites", s.handleListSites)
				r.Post("/sites", s.handleCreateSite)
				r.Post("/sites/order", s.handleOrderSites)
				r.Get("/sites/{id}", s.handleGetSite)
				r.Put("/sites/{id}", s.handleUpdateSite)
				r.Delete("/sites/{id}", s.handleDeleteSite)
				r.Post("/sites/{id}/reset", s.handleResetSite)
				r.Post("/sites/{id}/proxy-key", s.handleNewProxyKey)
				r.Get("/sites/{id}/check", s.handleCheckSite)

				// the numbers
				r.Get("/sites/{id}/stats", s.handleStats)
				r.Get("/sites/{id}/breakdown", s.handleBreakdown)
				r.Get("/sites/{id}/conversions", s.handleConversions)
				r.Get("/sites/{id}/live", s.handleLive)
				r.Get("/sites/{id}/visits", s.handleVisits)
				r.Get("/sites/{id}/visits/{vid}", s.handleVisit)
				r.Get("/sites/{id}/events", s.handleEventNames)

				// goals and funnels
				r.Post("/sites/{id}/goals", s.handleCreateGoal)
				r.Put("/goals/{gid}", s.handleUpdateGoal)
				r.Delete("/goals/{gid}", s.handleDeleteGoal)
				r.Post("/sites/{id}/funnels", s.handleCreateFunnel)
				r.Get("/sites/{id}/funnels/{fid}/counts", s.handleFunnelCounts)
				r.Put("/funnels/{fid}", s.handleUpdateFunnel)
				r.Delete("/funnels/{fid}", s.handleDeleteFunnel)

				// the other Pocket apps
				r.Get("/apps", s.handleApps)

				// for the rest of the box: Pocket Day, Search, dashboards, AI
				r.Get("/summary", s.handleSummary)
				r.Get("/day/{date}", s.handleDay)
				r.Get("/search", s.handleSearch)

				// AI
				r.Get("/ai/status", s.handleAIStatus)
				r.Get("/ai/models", s.handleAIModels)

				r.Post("/integrations/test", s.handleIntegrationTest)
				r.Post("/notify/test", s.handleNotifyTest)
				r.Post("/notify/report", s.handleReportNow)

				r.Get("/users", s.handleListUsers)
				r.Post("/users", s.handleCreateUser)
				r.Put("/users/{id}", s.handleUpdateUser)
				r.Delete("/users/{id}", s.handleDeleteUser)

				r.Get("/settings", s.handleGetSettings)
				r.Put("/settings", s.handlePutSettings)
				r.Get("/settings/system", s.handleSystemInfo)

				r.Get("/tokens", s.handleListTokens)
				r.Post("/tokens", s.handleCreateToken)
				r.Delete("/tokens/{id}", s.handleDeleteToken)

				r.Get("/backup", s.handleBackup)
				r.Get("/export.json", s.handleExportJSON)
			})
		})
	})

	r.NotFound(s.handleSPA)
	return s.hostSwitch(r)
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_analytics_info Build info.\n# TYPE pocket_analytics_info gauge\npocket_analytics_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_analytics_uptime_seconds Process uptime.\n# TYPE pocket_analytics_uptime_seconds gauge\npocket_analytics_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_analytics_http_requests_total HTTP requests served.\n# TYPE pocket_analytics_http_requests_total counter\npocket_analytics_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_analytics_http_errors_total HTTP 5xx responses.\n# TYPE pocket_analytics_http_errors_total counter\npocket_analytics_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_analytics_rows Rows per table.\n# TYPE pocket_analytics_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_analytics_rows{table=%q} %d\n", k, v)
	}
	fmt.Fprintf(w, "# HELP pocket_analytics_hits_received_total Hits from websites since start, by what happened to them.\n# TYPE pocket_analytics_hits_received_total counter\n")
	s.countMu.Lock()
	for k, v := range s.counts {
		fmt.Fprintf(w, "pocket_analytics_hits_received_total{result=%q} %d\n", k, v)
	}
	s.countMu.Unlock()
	fmt.Fprintf(w, "# HELP pocket_analytics_hits_stored_total Hits stored since start.\n# TYPE pocket_analytics_hits_stored_total counter\npocket_analytics_hits_stored_total %d\n", s.stored.Load())
	fmt.Fprintf(w, "# HELP pocket_analytics_queue Hits waiting to be stored.\n# TYPE pocket_analytics_queue gauge\npocket_analytics_queue %d\n", len(s.hits))
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}
