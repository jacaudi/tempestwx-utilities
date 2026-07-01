// Package apiserver exposes a small read-only HTTP JSON API that serves the
// latest Tempest observation and station metadata to the tempest-display SPA
// (see jacaudi/tempest-display and issue #35).
//
// It participates in the existing UDP pipeline by implementing
// sink.MetricsWriter: every parsed report is fed to WriteReport, where the most
// recent obs_st observation is cached in memory and then served as JSON.
//
// Scope: this implements the two core endpoints called out in the issue title —
// GET /api/station and GET /api/observation — plus a GET /health probe. The
// remaining endpoints from the issue (/api/status, /api/forecast, /api/almanac,
// and the /ws relay) depend on external data sources (NWS API, Postgres history)
// and are intentionally left for follow-up work.
package apiserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"tempestwx-utilities/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
)

// StationMeta is the static station metadata served at GET /api/station. The
// live fields (SerialNumber, FirmwareRevision) are filled in from incoming
// reports; the descriptive fields come from configuration because they are not
// present in the UDP broadcast.
type StationMeta struct {
	StationID        int     `json:"station_id,omitempty"`
	Name             string  `json:"name,omitempty"`
	Latitude         float64 `json:"latitude,omitempty"`
	Longitude        float64 `json:"longitude,omitempty"`
	Elevation        float64 `json:"elevation,omitempty"`
	Timezone         string  `json:"timezone,omitempty"`
	FirmwareRevision string  `json:"firmware_revision,omitempty"`
	SerialNumber     string  `json:"serial_number,omitempty"`
	DeviceID         int     `json:"device_id,omitempty"`
}

// CurrentObservation is the latest observation served at GET /api/observation.
// Field names match the shape the tempest-display SPA consumes.
type CurrentObservation struct {
	Timestamp                  int64   `json:"timestamp"`
	WindLull                   float64 `json:"windLull"`
	WindAvg                    float64 `json:"windAvg"`
	WindGust                   float64 `json:"windGust"`
	WindDirection              float64 `json:"windDirection"`
	WindSampleInterval         float64 `json:"windSampleInterval"`
	StationPressure            float64 `json:"stationPressure"`
	AirTemperature             float64 `json:"airTemperature"`
	RelativeHumidity           float64 `json:"relativeHumidity"`
	Illuminance                float64 `json:"illuminance"`
	UVIndex                    float64 `json:"uvIndex"`
	SolarRadiation             float64 `json:"solarRadiation"`
	RainAccumulated            float64 `json:"rainAccumulated"`
	PrecipitationType          int     `json:"precipitationType"`
	LightningStrikeAvgDistance float64 `json:"lightningStrikeAvgDistance"`
	LightningStrikeCount       float64 `json:"lightningStrikeCount"`
	Battery                    float64 `json:"battery"`
	ReportInterval             float64 `json:"reportInterval"`
	FeelsLike                  float64 `json:"feelsLike"`
	DewPoint                   float64 `json:"dewPoint"`
	WetBulbTemperature         float64 `json:"wetBulbTemperature"`
	HeatIndex                  float64 `json:"heatIndex"`
	WindChill                  float64 `json:"windChill"`
}

// Server is an HTTP JSON API server that caches the latest observation and
// serves it, alongside station metadata, to the display SPA. It implements
// sink.MetricsWriter.
type Server struct {
	port    string
	server  *http.Server
	handler http.Handler
	wg      sync.WaitGroup

	mu       sync.RWMutex
	meta     StationMeta
	obs      *CurrentObservation
	serial   string
	firmware int
}

// NewServer constructs an API server listening on the given port, seeded with
// the supplied static station metadata.
func NewServer(port string, meta StationMeta) *Server {
	s := &Server{port: port, meta: meta}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/station", s.handleStation)
	mux.HandleFunc("/api/observation", s.handleObservation)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	s.handler = mux

	s.server = &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	return s
}

// StationMetaFromEnv builds StationMeta from optional environment variables.
// All fields are optional; unset values are simply omitted from the response.
//
//	STATION_ID, STATION_NAME, STATION_LATITUDE, STATION_LONGITUDE,
//	STATION_ELEVATION, STATION_TIMEZONE, DEVICE_ID
func StationMetaFromEnv() StationMeta {
	atoi := func(key string) int {
		n, _ := strconv.Atoi(os.Getenv(key))
		return n
	}
	atof := func(key string) float64 {
		f, _ := strconv.ParseFloat(os.Getenv(key), 64)
		return f
	}
	return StationMeta{
		StationID: atoi("STATION_ID"),
		Name:      os.Getenv("STATION_NAME"),
		Latitude:  atof("STATION_LATITUDE"),
		Longitude: atof("STATION_LONGITUDE"),
		Elevation: atof("STATION_ELEVATION"),
		Timezone:  os.Getenv("STATION_TIMEZONE"),
		DeviceID:  atoi("DEVICE_ID"),
	}
}

// Handler exposes the underlying HTTP handler for testing without binding a
// TCP port.
func (s *Server) Handler() http.Handler { return s.handler }

// Start begins serving in a background goroutine.
func (s *Server) Start() error {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		slog.Info("api: starting HTTP server", "port", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("api: server error", "err", err)
		}
	}()
	return nil
}

// WriteReport caches state from the latest obs_st report. Other report types are
// ignored (they carry no data these two endpoints serve).
func (s *Server) WriteReport(_ context.Context, report tempestudp.Report) error {
	obsReport, ok := report.(*tempestudp.TempestObservationReport)
	if !ok {
		return nil
	}
	obs, ok := observationFromReport(obsReport)
	if !ok {
		return nil
	}

	s.mu.Lock()
	s.obs = obs
	if obsReport.SerialNumber != "" {
		s.serial = obsReport.SerialNumber
	}
	if obsReport.FirmwareRevision != 0 {
		s.firmware = obsReport.FirmwareRevision
	}
	s.mu.Unlock()
	return nil
}

// WriteMetrics is a no-op: the API server serves typed observations, not
// Prometheus metrics.
func (s *Server) WriteMetrics(_ context.Context, _ []prometheus.Metric) error { return nil }

// Flush is a no-op.
func (s *Server) Flush(_ context.Context) error { return nil }

// Close shuts the HTTP server down gracefully.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		slog.Error("api: shutdown error", "err", err)
		return err
	}
	s.wg.Wait()
	slog.Info("api: server closed")
	return nil
}

func (s *Server) handleStation(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	meta := s.meta
	meta.SerialNumber = s.serial
	if s.firmware != 0 {
		meta.FirmwareRevision = strconv.Itoa(s.firmware)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, meta)
}

func (s *Server) handleObservation(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	obs := s.obs
	s.mu.RUnlock()

	if obs == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "no observation received yet",
		})
		return
	}
	writeJSON(w, http.StatusOK, obs)
}

// observationFromReport converts the most recent obs row of an obs_st report
// into a CurrentObservation, computing the derived temperature quantities. It
// returns false when the report carries no usable observation row.
func observationFromReport(r *tempestudp.TempestObservationReport) (*CurrentObservation, bool) {
	if len(r.Obs) == 0 {
		return nil, false
	}
	ob := r.Obs[len(r.Obs)-1]
	if len(ob) < 13 {
		return nil, false
	}

	obs := &CurrentObservation{
		Timestamp:          int64(ob[0]),
		WindLull:           ob[1],
		WindAvg:            ob[2],
		WindGust:           ob[3],
		WindDirection:      ob[4],
		WindSampleInterval: ob[5],
		StationPressure:    ob[6],
		AirTemperature:     ob[7],
		RelativeHumidity:   ob[8],
		Illuminance:        ob[9],
		UVIndex:            ob[10],
		SolarRadiation:     ob[11],
		RainAccumulated:    ob[12],
	}
	if len(ob) >= 14 {
		obs.PrecipitationType = int(ob[13])
	}
	if len(ob) >= 16 {
		obs.LightningStrikeAvgDistance = ob[14]
		obs.LightningStrikeCount = ob[15]
	}
	if len(ob) >= 17 {
		obs.Battery = ob[16]
	}
	if len(ob) >= 18 {
		obs.ReportInterval = ob[17]
	}

	temp, rh, pressure, wind := ob[7], ob[8], ob[6], ob[2]
	obs.DewPoint = round1(DewPointC(temp, rh))
	obs.WetBulbTemperature = round1(safeWetBulbC(temp, rh, pressure))
	obs.HeatIndex = round1(HeatIndexC(temp, rh))
	obs.WindChill = round1(WindChillC(temp, wind))
	obs.FeelsLike = round1(FeelsLikeC(temp, rh, wind))
	return obs, true
}

// safeWetBulbC computes the wet-bulb temperature without ever panicking. The
// upstream iterative solver can panic ("failed to converge") on physically
// implausible inputs — e.g. a corrupt UDP packet carrying garbage floats. Since
// WriteReport runs on a sink goroutine, an unrecovered panic there would take
// the whole process down, so on failure we fall back to the air temperature.
func safeWetBulbC(tempC, rh, pressureMb float64) (out float64) {
	defer func() {
		if recover() != nil {
			out = tempC
		}
	}()
	return tempestudp.WetBulbTemperatureC(tempC, rh, pressureMb)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("api: failed to encode response", "err", err)
	}
}
