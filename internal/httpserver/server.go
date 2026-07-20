// Package httpserver builds the HTTP server that serves the embedded React
// UI (from tempestwx-utilities/web) alongside the JSON API added by later
// tasks (observations from SQLite, WeatherFlow proxy — see Deps).
package httpserver

import (
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"
)

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second

	// apiPrefix is reserved for the JSON API handlers Tasks 1.4/1.5 register.
	// Requests under this prefix never fall through to the SPA fallback, even
	// before those handlers exist, so the namespace boundary holds from day one.
	apiPrefix = "/api/"

	indexPage = "index.html"
)

// cspPolicy is the self-only Content-Security-Policy for the UI: every
// resource (scripts, styles, images, connections) must come from the same
// origin the page was served from. No external host is ever added here —
// the OSM basemap is served same-origin as a static asset (Task 2.6).
const cspPolicy = "default-src 'self'; img-src 'self' data: blob:; worker-src 'self' blob:; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'self'"

// Deps carries the dependencies New's handlers need. It is the seam future
// tasks extend: Task 1.4 adds a SQLite reader field + a registerObservations
// call in New; Task 1.5 adds a WeatherFlow client field + its own register
// call. Adding a field and a register call is additive — it does not require
// touching the handlers already registered here.
type Deps struct {
	// StaticFS serves the embedded UI build (web.DistFS() in production; an
	// in-memory fstest.MapFS in tests, so tests never need a real UI build).
	StaticFS fs.FS

	// Observations backs GET /api/observations/current and .../history
	// (Task 1.4). Production passes a *sqlite.Writer (which satisfies
	// ObservationReader); tests pass a fake.
	Observations ObservationReader
}

// New builds the HTTP server: the embedded UI with SPA fallback, security
// headers, /healthz, explicit timeouts, and a mux ready for later tasks to
// register /api/* handlers onto. The caller owns starting and stopping it
// (Serve/ListenAndServe, then Shutdown(ctx) for graceful shutdown).
func New(deps Deps) *http.Server {
	mux := http.NewServeMux()
	registerHealthz(mux)
	registerObservations(mux, deps)
	registerStatic(mux, deps)

	return &http.Server{
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// securityHeaders wraps next with the response headers required for the UI:
// a self-only CSP, MIME-sniffing protection, and no referrer leakage.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", cspPolicy)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// registerHealthz registers the liveness endpoint used by container
// orchestration to confirm the process is up.
func registerHealthz(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			slog.ErrorContext(r.Context(), "httpserver: healthz write response", "error", err)
		}
	})
}

// registerStatic registers the catch-all handler that serves the embedded
// UI, with SPA fallback for client-side routes and a reserved /api/
// namespace for future JSON API handlers.
//
// Rules:
//   - /api/* never falls back to the SPA index (reserved for Tasks 1.4/1.5).
//   - A path that resolves to a real static file is served with an immutable
//     cache-control header (content-hashed assets never change in place).
//   - A missing path with a file extension (a static asset request) 404s —
//     it must not silently serve index.html for a stale/renamed asset URL.
//   - A missing path without an extension (a client-side route) serves
//     index.html for the SPA router to handle, uncached.
func registerStatic(mux *http.ServeMux, deps Deps) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			http.NotFound(w, r)
			return
		}

		assetPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if assetPath == "" || assetPath == "." {
			assetPath = indexPage
		}

		if assetPath != indexPage && fs.ValidPath(assetPath) {
			if f, err := deps.StaticFS.Open(assetPath); err == nil {
				_ = f.Close()
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeFileFS(w, r, deps.StaticFS, assetPath)
				return
			}
			if path.Ext(assetPath) != "" {
				http.NotFound(w, r)
				return
			}
		}

		http.ServeFileFS(w, r, deps.StaticFS, indexPage)
	})
}
