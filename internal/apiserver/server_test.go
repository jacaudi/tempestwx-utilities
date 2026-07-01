package apiserver

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"tempestwx-utilities/internal/tempestudp"
)

// ─────────────────────────────────────────────────────────────────────────
// Tests are organized by the seven Go test categories:
//   1. Happy path       2. Table-driven     3. Edge/boundary
//   4. Error handling    5. Concurrency/race 6. Integration
//   7. Benchmark (+ Fuzz)
// ─────────────────────────────────────────────────────────────────────────

// A representative obs_st row (18 fields) matching the WeatherFlow field order.
func sampleReport() *tempestudp.TempestObservationReport {
	return &tempestudp.TempestObservationReport{
		Type:             "obs_st",
		SerialNumber:     "ST-00012345",
		FirmwareRevision: 143,
		Obs: [][]float64{{
			1708560000, // 0  timestamp
			0.5,        // 1  wind lull
			2.1,        // 2  wind avg
			4.2,        // 3  wind gust
			270,        // 4  wind direction
			3,          // 5  wind sample interval
			1013.2,     // 6  station pressure MB
			21.5,       // 7  air temperature C
			55,         // 8  relative humidity %
			12000,      // 9  illuminance lux
			4,          // 10 UV index
			450,        // 11 solar radiation W/m^2
			0.1,        // 12 rain over previous minute mm
			1,          // 13 precipitation type
			8,          // 14 lightning avg distance km
			2,          // 15 lightning strike count
			2.62,       // 16 battery V
			1,          // 17 report interval min
		}},
	}
}

// Category 1 — Happy path: after a report is written, /api/observation returns
// 200 with the parsed obs_st fields.
func TestObservationHappyPath(t *testing.T) {
	s := NewServer("0", StationMeta{})
	if err := s.WriteReport(context.Background(), sampleReport()); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/observation", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var obs CurrentObservation
	if err := json.Unmarshal(rec.Body.Bytes(), &obs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if obs.Timestamp != 1708560000 || obs.AirTemperature != 21.5 || obs.WindGust != 4.2 {
		t.Errorf("unexpected core fields: %+v", obs)
	}
	if obs.PrecipitationType != 1 || obs.Battery != 2.62 || obs.ReportInterval != 1 {
		t.Errorf("unexpected tail fields: %+v", obs)
	}
	if obs.DewPoint == 0 || obs.WetBulbTemperature == 0 {
		t.Errorf("derived temps not populated: %+v", obs)
	}
}

// Category 2 — Table-driven: the derived meteorological helpers against known
// reference points (within tolerance).
func TestDerivedCalcsTable(t *testing.T) {
	approx := func(got, want, tol float64) bool { return math.Abs(got-want) <= tol }
	cases := []struct {
		name           string
		got, want, tol float64
	}{
		{"dewpoint 21.5C/55%", DewPointC(21.5, 55), 12.0, 0.5},
		{"dewpoint saturated", DewPointC(20, 100), 20.0, 0.2},
		{"heatindex mild is passthrough", HeatIndexC(20, 50), 20.0, 0.01},
		{"heatindex hot 35C/60%", HeatIndexC(35, 60), 45.0, 3.0},
		{"windchill warm is passthrough", WindChillC(20, 5), 20.0, 0.01},
		{"windchill cold -5C/10m/s", WindChillC(-5, 10), -15.0, 3.0},
		{"feelslike temperate passthrough", FeelsLikeC(18, 50, 1), 18.0, 0.01},
	}
	for _, tc := range cases {
		if !approx(tc.got, tc.want, tc.tol) {
			t.Errorf("%s: got %.2f, want %.2f±%.2f", tc.name, tc.got, tc.want, tc.tol)
		}
	}
}

// Category 3 — Edge/boundary: no report yet → 503; station reflects live serial
// and firmware once a report lands; short obs rows are rejected.
func TestEdgeCases(t *testing.T) {
	s := NewServer("0", StationMeta{Name: "Shoreline", Latitude: 47.75})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/observation", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("observation before any report: status = %d, want 503", rec.Code)
	}

	// Station is served even before a report, from static config.
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/station", nil))
	var meta StationMeta
	_ = json.Unmarshal(rec.Body.Bytes(), &meta)
	if meta.Name != "Shoreline" || meta.SerialNumber != "" {
		t.Errorf("unexpected pre-report station: %+v", meta)
	}

	// After a report, serial + firmware appear.
	_ = s.WriteReport(context.Background(), sampleReport())
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/station", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &meta)
	if meta.SerialNumber != "ST-00012345" || meta.FirmwareRevision != "143" {
		t.Errorf("station missing live fields: %+v", meta)
	}

	// A too-short obs row must not overwrite state / must be rejected.
	if _, ok := observationFromReport(&tempestudp.TempestObservationReport{Obs: [][]float64{{1, 2, 3}}}); ok {
		t.Error("short obs row should be rejected")
	}
	if _, ok := observationFromReport(&tempestudp.TempestObservationReport{Obs: nil}); ok {
		t.Error("empty obs should be rejected")
	}
}

// Category 4 — Error handling: non-obs reports are ignored (no panic, no state
// change) and WriteMetrics/Flush are no-ops.
func TestNonObservationReportsIgnored(t *testing.T) {
	s := NewServer("0", StationMeta{})
	if err := s.WriteReport(context.Background(), &tempestudp.RapidWindReport{Type: "rapid_wind"}); err != nil {
		t.Fatalf("WriteReport(rapid_wind): %v", err)
	}
	if err := s.WriteMetrics(context.Background(), nil); err != nil {
		t.Fatalf("WriteMetrics: %v", err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/observation", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("rapid_wind should not populate observation: status = %d", rec.Code)
	}
}

// Category 5 — Concurrency/race: concurrent writers and readers under -race.
func TestConcurrentReadWrite(t *testing.T) {
	s := NewServer("0", StationMeta{})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = s.WriteReport(context.Background(), sampleReport()) }()
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/observation", nil))
		}()
	}
	wg.Wait()
}

// Category 6 — Integration: over a real httptest server, /health and the two API
// endpoints respond through the full HTTP stack.
func TestIntegrationEndpoints(t *testing.T) {
	s := NewServer("0", StationMeta{Name: "Shoreline"})
	_ = s.WriteReport(context.Background(), sampleReport())

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	for _, tc := range []struct {
		path string
		want int
	}{
		{"/health", http.StatusOK},
		{"/api/station", http.StatusOK},
		{"/api/observation", http.StatusOK},
	} {
		resp, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		if resp.StatusCode != tc.want {
			t.Errorf("GET %s: status = %d, want %d", tc.path, resp.StatusCode, tc.want)
		}
		resp.Body.Close()
	}
}

// Category 7a — Benchmark: report-to-cache conversion throughput.
func BenchmarkWriteReport(b *testing.B) {
	s := NewServer("0", StationMeta{})
	r := sampleReport()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.WriteReport(context.Background(), r)
	}
}

// Category 7b — Fuzz: observationFromReport must never panic on arbitrary obs
// rows, and derived values must be finite when it succeeds.
func FuzzObservationFromReport(f *testing.F) {
	f.Add(21.5, 55.0, 1013.2, 2.1)
	f.Add(-40.0, 0.0, 900.0, 30.0)
	f.Add(50.0, 100.0, 1100.0, 0.0)
	f.Fuzz(func(t *testing.T, temp, rh, pressure, wind float64) {
		r := &tempestudp.TempestObservationReport{
			Obs: [][]float64{{1708560000, 0, wind, 0, 0, 0, pressure, temp, rh, 0, 0, 0, 0}},
		}
		obs, ok := observationFromReport(r)
		if !ok {
			return
		}
		for _, v := range []float64{obs.DewPoint, obs.HeatIndex, obs.WindChill, obs.FeelsLike, obs.WetBulbTemperature} {
			if math.IsInf(v, 0) || math.IsNaN(v) {
				t.Errorf("non-finite derived value for temp=%v rh=%v p=%v wind=%v: %v", temp, rh, pressure, wind, v)
			}
		}
	})
}
