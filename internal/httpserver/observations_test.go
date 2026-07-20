package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"tempestwx-utilities/internal/sqlite"
)

// fakeObservationReader is a hand-written test double for ObservationReader
// (per go-standards §8.4, prefer interface-based fakes over mock-generation
// libraries). It never touches a real database.
type fakeObservationReader struct {
	obs    sqlite.Observation
	obsErr error

	// history maps a field name to the points HistoryPoints returns for it.
	// A field absent from the map yields an empty (non-nil) slice, mirroring
	// the real writer's "no matches" behavior.
	history map[string][]sqlite.Point
	// historyErr, when non-nil, is returned by HistoryPoints unconditionally
	// -- simulating the real writer's allowlist rejecting an unknown field
	// before running any query.
	historyErr error
}

func (f *fakeObservationReader) LatestObservationAny(context.Context) (sqlite.Observation, error) {
	return f.obs, f.obsErr
}

func (f *fakeObservationReader) HistoryPoints(_ context.Context, field string, _, _ int64) ([]sqlite.Point, error) {
	if f.historyErr != nil {
		return nil, f.historyErr
	}
	return f.history[field], nil
}

func testDepsWithObservations(reader ObservationReader) Deps {
	return Deps{
		StaticFS:     fstest.MapFS{"index.html": {Data: []byte("<html>fake index</html>")}},
		Observations: reader,
	}
}

// TestAPI_CurrentObservation proves GET /api/observations/current marshals
// every Contract C field (web/src/types/weather.ts CurrentObservation) with
// SI values sourced from the newest sqlite observation, including at least
// one derived (server-computed) field, and 404s with the sentinel error.
func TestAPI_CurrentObservation(t *testing.T) {
	t.Run("returns_contract_c_shape", func(t *testing.T) {
		windSample := int64(3)
		precip := int64(1)
		lightningDist := 2.1
		lightningCount := int64(4)
		battery := 3.6
		reportInterval := int64(5)

		reader := &fakeObservationReader{
			obs: sqlite.Observation{
				SerialNumber:         "ST-00001",
				Timestamp:            1700000000,
				WindLull:             1.5,
				WindAvg:              2.0,
				WindGust:             2.5,
				WindDirection:        180,
				WindSampleInterval:   &windSample,
				Pressure:             1013.25,
				TempAir:              20.5,
				Humidity:             55.0,
				Illuminance:          50000,
				UVIndex:              3.0,
				Irradiance:           500.0,
				RainRate:             0.5,
				PrecipType:           &precip,
				LightningDistance:    &lightningDist,
				LightningStrikeCount: &lightningCount,
				Battery:              &battery,
				ReportInterval:       &reportInterval,
			},
			// Rising pressure trend: last (1014.0) - first (1013.0) = +1.0 > 0.5.
			history: map[string][]sqlite.Point{
				"pressure": {{T: 1699999000, V: 1013.0}, {T: 1700000000, V: 1014.0}},
			},
		}

		srv := New(testDepsWithObservations(reader))
		req := httptest.NewRequest(http.MethodGet, "/api/observations/current", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/observations/current = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}

		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal response: %v (body: %s)", err, rec.Body.String())
		}

		// Every Contract C key from web/src/types/weather.ts CurrentObservation
		// must be present.
		wantKeys := []string{
			"timestamp", "windLull", "windAvg", "windGust", "windDirection",
			"windSampleInterval", "stationPressure", "airTemperature",
			"relativeHumidity", "illuminance", "uvIndex", "solarRadiation",
			"rainAccumulated", "precipitationType", "lightningStrikeAvgDistance",
			"lightningStrikeCount", "battery", "reportInterval",
			"localDayRainAccumulation", "feelsLike", "dewPoint",
			"wetBulbTemperature", "heatIndex", "windChill", "pressureTrend",
		}
		for _, k := range wantKeys {
			if _, ok := got[k]; !ok {
				t.Errorf("response missing Contract C key %q: %v", k, got)
			}
		}

		// Spot-check raw field mapping.
		if got["airTemperature"] != 20.5 {
			t.Errorf("airTemperature = %v, want 20.5", got["airTemperature"])
		}
		if got["stationPressure"] != 1013.25 {
			t.Errorf("stationPressure = %v, want 1013.25", got["stationPressure"])
		}
		if got["solarRadiation"] != 500.0 {
			t.Errorf("solarRadiation = %v, want 500.0 (irradiance)", got["solarRadiation"])
		}
		if got["rainAccumulated"] != 0.5 {
			t.Errorf("rainAccumulated = %v, want 0.5 (rain_rate)", got["rainAccumulated"])
		}
		// A field never persisted by the writer (Tempest field 18); zero-filled
		// with the gap noted in the task report, not invented.
		if got["localDayRainAccumulation"] != 0.0 {
			t.Errorf("localDayRainAccumulation = %v, want 0 (not stored)", got["localDayRainAccumulation"])
		}

		// Derived field: pressureTrend from the 3h history window.
		if got["pressureTrend"] != "rising" {
			t.Errorf("pressureTrend = %v, want rising", got["pressureTrend"])
		}
		// Derived field: dewPoint must differ from airTemperature (proves it
		// was actually computed, not just echoed).
		if got["dewPoint"] == got["airTemperature"] {
			t.Errorf("dewPoint = %v, expected a computed value distinct from airTemperature", got["dewPoint"])
		}
	})

	t.Run("not_found_returns_404_json", func(t *testing.T) {
		reader := &fakeObservationReader{obsErr: sqlite.ErrObservationNotFound}
		srv := New(testDepsWithObservations(reader))

		req := httptest.NewRequest(http.MethodGet, "/api/observations/current", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /api/observations/current = %d, want 404", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
	})
}

// TestAPI_History proves GET /api/observations/history returns the sqlite
// Point shape wrapped in {"points": [...]}  for a valid field, and 400s when
// the field is rejected by the writer's allowlist.
func TestAPI_History(t *testing.T) {
	t.Run("valid_field_returns_points", func(t *testing.T) {
		reader := &fakeObservationReader{
			history: map[string][]sqlite.Point{
				"temp_air": {{T: 1700000100, V: 15}, {T: 1700000200, V: 20}},
			},
		}
		srv := New(testDepsWithObservations(reader))

		req := httptest.NewRequest(http.MethodGet, "/api/observations/history?field=temp_air&from=&to=", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/observations/history = %d, want 200, body: %s", rec.Code, rec.Body.String())
		}

		var got struct {
			Points []sqlite.Point `json:"points"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		want := []sqlite.Point{{T: 1700000100, V: 15}, {T: 1700000200, V: 20}}
		if len(got.Points) != len(want) {
			t.Fatalf("points = %+v, want %+v", got.Points, want)
		}
		for i, p := range want {
			if got.Points[i] != p {
				t.Errorf("point %d = %+v, want %+v", i, got.Points[i], p)
			}
		}
	})

	t.Run("invalid_field_returns_400", func(t *testing.T) {
		reader := &fakeObservationReader{historyErr: errUnknownFieldForTest}
		srv := New(testDepsWithObservations(reader))

		req := httptest.NewRequest(http.MethodGet, "/api/observations/history?field=not_a_real_field", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET /api/observations/history?field=not_a_real_field = %d, want 400", rec.Code)
		}
	})
}

// TestAPI_ObservationsNilStore proves both observation handlers 503 (not
// panic) when Deps.Observations is nil -- the postgres-only edge case (Task
// 1.6) where main.go starts the JSON API server without a sqlite writer.
func TestAPI_ObservationsNilStore(t *testing.T) {
	srv := New(testDepsWithObservations(nil))

	t.Run("current", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/observations/current", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET /api/observations/current = %d, want 503", rec.Code)
		}
	})

	t.Run("history", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/observations/history?field=temp_air", nil)
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("GET /api/observations/history = %d, want 503", rec.Code)
		}
	})
}

// errUnknownFieldForTest simulates the sqlite package's own "unknown history
// field" error without importing sqlite's unexported error construction --
// the handler must map ANY HistoryPoints error to 400, not just a specific
// sentinel, since HistoryPoints has no exported sentinel for this case.
var errUnknownFieldForTest = errors.New("unknown history field")
