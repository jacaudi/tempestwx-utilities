package radar

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// sidecarFeatureCollection writes a minimal-but-valid Contract A success body
// (a GeoJSON FeatureCollection with the metadata block the proxy decodes).
func sidecarFeatureCollection(w http.ResponseWriter, site, product string) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":     "FeatureCollection",
		"features": []any{},
		"metadata": map[string]any{
			"site":      site,
			"product":   product,
			"scan_time": "2026-07-20T12:00:00Z",
			"bbox":      []float64{-98.5, 34.9, -96.5, 36.1},
		},
	})
}

func writeSidecarError(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

func TestProxy_Constants(t *testing.T) {
	if radarCacheMaxEntries != 64 {
		t.Errorf("radarCacheMaxEntries = %d, want 64", radarCacheMaxEntries)
	}
	if radarCacheTTL != 5*time.Minute {
		t.Errorf("radarCacheTTL = %v, want %v", radarCacheTTL, 5*time.Minute)
	}
}

func TestProxy_FetchesAndCaches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		sidecarFeatureCollection(w, r.URL.Query().Get("site"), r.URL.Query().Get("product"))
	}))
	defer srv.Close()

	p := NewProxy(srv.URL)
	ctx := t.Context()

	body1, meta1, err := p.Get(ctx, "TLX", "N0B")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	body2, meta2, err := p.Get(ctx, "TLX", "N0B")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("sidecar hits = %d, want 1 (second Get should be served from cache)", got)
	}
	if string(body1) != string(body2) {
		t.Errorf("cached body mismatch:\n%s\nvs\n%s", body1, body2)
	}
	if meta1.Site != "TLX" || meta1.Product != "N0B" {
		t.Errorf("meta1 = %+v, want site=TLX product=N0B", meta1)
	}
	if !meta1.ScanTime.Equal(meta2.ScanTime) {
		t.Errorf("scan_time mismatch: %v vs %v", meta1.ScanTime, meta2.ScanTime)
	}
}

func TestProxy_N0BFallsBackToN0Q(t *testing.T) {
	var n0bHits, n0qHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("product") {
		case "N0B":
			n0bHits.Add(1)
			writeSidecarError(w, http.StatusServiceUnavailable, "no_recent_scan")
		case "N0Q":
			n0qHits.Add(1)
			sidecarFeatureCollection(w, "TLX", "N0Q")
		}
	}))
	defer srv.Close()

	p := NewProxy(srv.URL)
	_, meta, err := p.Get(t.Context(), "TLX", "N0B")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if meta.Product != "N0Q" {
		t.Errorf("meta.Product = %q, want N0Q (fallback)", meta.Product)
	}
	if n0bHits.Load() != 1 {
		t.Errorf("N0B hits = %d, want 1", n0bHits.Load())
	}
	if n0qHits.Load() != 1 {
		t.Errorf("N0Q hits = %d, want 1", n0qHits.Load())
	}
}

func TestProxy_ErrorEnvelope(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		writeSidecarError(w, http.StatusBadGateway, "decode_failed")
	}))
	defer srv.Close()

	p := NewProxy(srv.URL)
	_, _, err := p.Get(t.Context(), "TLX", "N0B")
	if !errors.Is(err, ErrDecodeFailed) {
		t.Errorf("Get error = %v, want errors.Is ErrDecodeFailed", err)
	}
	// decode_failed is not no_recent_scan, so no N0B->N0Q fallback attempt.
	if hits.Load() != 1 {
		t.Errorf("sidecar hits = %d, want 1 (no fallback for decode_failed)", hits.Load())
	}
}
