package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// testDeps returns a Deps wired to an in-memory static filesystem, so tests
// are hermetic and never depend on a real `web/dist` build.
func testDeps() Deps {
	return Deps{
		StaticFS: fstest.MapFS{
			"index.html":    {Data: []byte("<html>fake index</html>")},
			"assets/app.js": {Data: []byte("console.log('app')")},
		},
	}
}

func TestServer_ServesIndex(t *testing.T) {
	srv := New(testDeps())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "fake index") {
		t.Fatalf("GET / body = %q, want embedded index.html content", rec.Body.String())
	}
}

func TestServer_SPAFallback(t *testing.T) {
	srv := New(testDeps())

	t.Run("unknown non-asset route serves index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some/spa/route", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /some/spa/route = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "fake index") {
			t.Fatalf("GET /some/spa/route body = %q, want index.html content", rec.Body.String())
		}
	})

	t.Run("missing static asset 404s, not index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /assets/missing.js = %d, want 404", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "fake index") {
			t.Fatalf("GET /assets/missing.js served SPA index instead of 404")
		}
	})

	t.Run("existing asset served with immutable cache-control", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /assets/app.js = %d, want 200", rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Fatalf("GET /assets/app.js Cache-Control = %q, want to contain immutable", cc)
		}
	})

	t.Run("index.html itself is not stamped immutable", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Fatalf("GET / Cache-Control = %q, must not be immutable", cc)
		}
	})

	t.Run("api namespace reserved, never serves SPA index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/observations", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /api/observations = %d, want 404 (reserved namespace, no handler yet)", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "fake index") {
			t.Fatalf("GET /api/observations served SPA index instead of 404")
		}
	})
}

func TestServer_SecurityHeaders(t *testing.T) {
	srv := New(testDeps())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	const wantCSP = "default-src 'self'; img-src 'self' data: blob:; worker-src 'self' blob:; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'self'"

	gotCSP := rec.Header().Get("Content-Security-Policy")
	if gotCSP != wantCSP {
		t.Fatalf("Content-Security-Policy = %q, want %q", gotCSP, wantCSP)
	}
	if strings.Contains(gotCSP, "http://") || strings.Contains(gotCSP, "https://") {
		t.Fatalf("Content-Security-Policy contains an external host: %q", gotCSP)
	}

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
}

func TestServer_Timeouts(t *testing.T) {
	srv := New(testDeps())

	if srv.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout is zero, want non-zero")
	}
	if srv.ReadTimeout == 0 {
		t.Error("ReadTimeout is zero, want non-zero")
	}
	if srv.WriteTimeout == 0 {
		t.Error("WriteTimeout is zero, want non-zero")
	}
	if srv.IdleTimeout == 0 {
		t.Error("IdleTimeout is zero, want non-zero")
	}
}

// TestServer_EmitsHTTPSpan proves the server's handler is wrapped in otelhttp:
// a request through srv.Handler must produce one ended span, using an
// injected TracerProvider (tracetest.SpanRecorder) rather than touching OTel
// globals, so the test is hermetic and can run in parallel with the rest of
// the suite.
func TestServer_EmitsHTTPSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	deps := testDeps()
	deps.TracerProvider = tp
	srv := New(deps)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("got %d ended spans, want 1", len(ended))
	}
	if got := ended[0].Name(); got != "http.server" {
		t.Fatalf("span name = %q, want %q", got, "http.server")
	}
}

func TestServer_Healthz(t *testing.T) {
	srv := New(testDeps())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %q, want {\"status\":\"ok\"}", got)
	}
}
