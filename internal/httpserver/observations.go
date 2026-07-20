package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"tempestwx-utilities/internal/sqlite"
	"tempestwx-utilities/internal/tempestudp"
)

// pressureTrendWindow is how far back HistoryPoints is queried to derive
// pressureTrend for the current observation. 3 hours is long enough to
// smooth past a single noisy sample yet short enough to reflect a
// short-term trend rather than a full daily cycle -- a judgment call with no
// authoritative source; see the task report for the rationale.
const pressureTrendWindow = 3 * time.Hour

// pressureTrendThresholdMB is the minimum |delta| (millibars, over
// pressureTrendWindow) required to call the trend "rising"/"falling" rather
// than "steady". Below this, sensor noise could otherwise flip the label
// sample-to-sample.
const pressureTrendThresholdMB = 0.5

// windChillMaxTempC and windChillMinWindKmh are the NWS/Environment Canada
// wind chill formula's validity bounds: below the temperature and above the
// wind speed. Outside this regime the formula does not apply and windChill
// returns the air temperature unchanged.
const (
	windChillMaxTempC   = 10.0
	windChillMinWindKmh = 4.8
)

// heatIndexMinTempC is the NWS heat-index formula's validity floor (80°F).
// Below it, feelsLike falls through to the wind-chill/plain-temperature
// branches instead.
const heatIndexMinTempC = 26.7

// ObservationReader is the read-side dependency registerObservations needs,
// defined at the consumer site per go-standards §12: production wires
// *sqlite.Writer (which satisfies it), tests use fakeObservationReader.
type ObservationReader interface {
	// LatestObservationAny returns the newest tempest_observations row across
	// all serials, or sqlite.ErrObservationNotFound if the table is empty.
	LatestObservationAny(ctx context.Context) (sqlite.Observation, error)
	// HistoryPoints returns the allowlisted field's samples in [from, to];
	// an unknown field is rejected with a non-nil error before any query runs.
	HistoryPoints(ctx context.Context, field string, from, to int64) ([]sqlite.Point, error)
}

// currentObservation is the wire shape for GET /api/observations/current.
// Field names and units are Contract C (design §11), pinned to
// web/src/types/weather.ts's CurrentObservation interface -- every field
// there must have a same-named counterpart here.
type currentObservation struct {
	Timestamp                  int64   `json:"timestamp"`
	WindLull                   float64 `json:"windLull"`
	WindAvg                    float64 `json:"windAvg"`
	WindGust                   float64 `json:"windGust"`
	WindDirection              float64 `json:"windDirection"`
	WindSampleInterval         int64   `json:"windSampleInterval"`
	StationPressure            float64 `json:"stationPressure"`
	AirTemperature             float64 `json:"airTemperature"`
	RelativeHumidity           float64 `json:"relativeHumidity"`
	Illuminance                float64 `json:"illuminance"`
	UVIndex                    float64 `json:"uvIndex"`
	SolarRadiation             float64 `json:"solarRadiation"`
	RainAccumulated            float64 `json:"rainAccumulated"`
	PrecipitationType          int64   `json:"precipitationType"`
	LightningStrikeAvgDistance float64 `json:"lightningStrikeAvgDistance"`
	LightningStrikeCount       int64   `json:"lightningStrikeCount"`
	Battery                    float64 `json:"battery"`
	ReportInterval             int64   `json:"reportInterval"`
	LocalDayRainAccumulation   float64 `json:"localDayRainAccumulation"`
	FeelsLike                  float64 `json:"feelsLike"`
	DewPoint                   float64 `json:"dewPoint"`
	WetBulbTemperature         float64 `json:"wetBulbTemperature"`
	HeatIndex                  float64 `json:"heatIndex"`
	WindChill                  float64 `json:"windChill"`
	PressureTrend              string  `json:"pressureTrend"`
}

// historyResponse is the wire shape for GET /api/observations/history:
// Contract C's {"points":[{"t":..,"v":..}]}. sqlite.Point already carries the
// t/v json tags (single-sourced there), so this just wraps it.
type historyResponse struct {
	Points []sqlite.Point `json:"points"`
}

// registerObservations registers the Contract C JSON API handlers reading
// SQLite. Additive per the Deps seam server.go documents: a new field
// (Observations) plus this one register call, siblings (registerHealthz,
// registerStatic) untouched.
func registerObservations(mux *http.ServeMux, deps Deps) {
	mux.HandleFunc("GET /api/observations/current", func(w http.ResponseWriter, r *http.Request) {
		handleCurrentObservation(w, r, deps.Observations)
	})
	mux.HandleFunc("GET /api/observations/history", func(w http.ResponseWriter, r *http.Request) {
		handleHistory(w, r, deps.Observations)
	})
}

// handleCurrentObservation serves the newest observation (across all
// serials -- see sqlite.Writer.LatestObservationAny's doc comment for why a
// single-station appliance has no serial to scope by) as Contract C's
// currentObservation, including the server-computed derived fields.
func handleCurrentObservation(w http.ResponseWriter, r *http.Request, reader ObservationReader) {
	ctx := r.Context()

	obs, err := reader.LatestObservationAny(ctx)
	if err != nil {
		if errors.Is(err, sqlite.ErrObservationNotFound) {
			writeJSONError(w, http.StatusNotFound, "no observation available")
			return
		}
		slog.ErrorContext(ctx, "httpserver: latest observation", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load current observation")
		return
	}

	now := time.Now().Unix()
	trendPoints, err := reader.HistoryPoints(ctx, "pressure", now-int64(pressureTrendWindow.Seconds()), now)
	if err != nil {
		// Pressure trend is a nice-to-have derived field, not the reason this
		// request exists -- degrade to "steady" rather than failing the whole
		// response over a history-query error.
		slog.WarnContext(ctx, "httpserver: pressure history for trend", "error", err)
		trendPoints = nil
	}

	writeJSON(w, http.StatusOK, toCurrentObservation(obs, pressureTrendFromHistory(trendPoints)))
}

// handleHistory serves HistoryPoints for the requested field, wrapped in
// Contract C's {"points": [...]} shape. field is passed straight through to
// HistoryPoints, whose allowlist rejects an unknown field -- that rejection
// is mapped to 400 here, not re-validated.
func handleHistory(w http.ResponseWriter, r *http.Request, reader ObservationReader) {
	ctx := r.Context()
	q := r.URL.Query()

	from := parseEpochOrDefault(q.Get("from"), 0)
	to := parseEpochOrDefault(q.Get("to"), time.Now().Unix())

	points, err := reader.HistoryPoints(ctx, q.Get("field"), from, to)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid field")
		return
	}

	writeJSON(w, http.StatusOK, historyResponse{Points: points})
}

// parseEpochOrDefault parses s as a base-10 unix-epoch-seconds integer,
// returning def for an empty or unparseable string. Query-string from/to are
// bind-parameter integers to HistoryPoints (never formatted into SQL text),
// so a malformed value degrading to the default is safe -- not a boundary a
// 400 needs to guard.
func parseEpochOrDefault(s string, def int64) int64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// toCurrentObservation maps a raw sqlite.Observation to Contract C's wire
// shape, filling in the server-computed derived fields. trend is the already
// -computed pressureTrend (kept as a parameter so this stays a pure function
// -- fetching the trend history is handleCurrentObservation's job, not
// this one's).
func toCurrentObservation(obs sqlite.Observation, trend string) currentObservation {
	return currentObservation{
		Timestamp:                  obs.Timestamp,
		WindLull:                   obs.WindLull,
		WindAvg:                    obs.WindAvg,
		WindGust:                   obs.WindGust,
		WindDirection:              obs.WindDirection,
		WindSampleInterval:         deref(obs.WindSampleInterval),
		StationPressure:            obs.Pressure,
		AirTemperature:             obs.TempAir,
		RelativeHumidity:           obs.Humidity,
		Illuminance:                obs.Illuminance,
		UVIndex:                    obs.UVIndex,
		SolarRadiation:             obs.Irradiance,
		RainAccumulated:            obs.RainRate,
		PrecipitationType:          deref(obs.PrecipType),
		LightningStrikeAvgDistance: deref(obs.LightningDistance),
		LightningStrikeCount:       deref(obs.LightningStrikeCount),
		Battery:                    deref(obs.Battery),
		ReportInterval:             deref(obs.ReportInterval),
		// Tempest UDP field 18 (local-day rain accumulation) is not persisted
		// by internal/sqlite's writer -- zero-filled rather than invented; see
		// the task report's DONE_WITH_CONCERNS note.
		LocalDayRainAccumulation: 0,
		FeelsLike:                feelsLikeC(obs.TempAir, obs.Humidity, obs.WindAvg),
		DewPoint:                 tempestudp.DewPointC(obs.TempAir, obs.Humidity),
		WetBulbTemperature:       wetBulbTemperatureC(obs),
		HeatIndex:                tempestudp.HeatIndexC(obs.TempAir, obs.Humidity),
		WindChill:                windChillC(obs.TempAir, obs.WindAvg),
		PressureTrend:            trend,
	}
}

// deref returns *p, or the zero value of T if p is nil. The single "nil
// pointer means absent SQL column -> report as zero" mapping every Contract
// C integer/float field below needs, shared knowledge Contract C's spec
// states once ("nil -> 0") for every one of these optional columns.
func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

// wetBulbTemperatureC returns obs's wet bulb temperature: the writer's own
// computed value when present (TempWetbulb is non-nil), else recomputed here
// from the stored raw fields (stored pressure is mb, which equals hPa, so no
// conversion is needed). WetBulbTemperatureC returns NaN for non-convergent
// inputs; since TempWetbulb being nil already means the writer's own attempt
// didn't converge, there is no second path to fall back to -- 0 is reported
// in that case, matching this handler's nil-pointer-as-absent convention.
func wetBulbTemperatureC(obs sqlite.Observation) float64 {
	if obs.TempWetbulb != nil {
		return *obs.TempWetbulb
	}
	wb := tempestudp.WetBulbTemperatureC(obs.TempAir, obs.Humidity, obs.Pressure)
	if math.IsNaN(wb) {
		return 0
	}
	return wb
}

// windChillC computes the NWS/Environment Canada metric wind chill:
//
//	WC = 13.12 + 0.6215*T - 11.37*V^0.16 + 0.3965*T*V^0.16
//
// (T in °C, V in km/h), valid only for T <= windChillMaxTempC and
// V > windChillMinWindKmh; outside that regime it returns T unchanged, since
// the formula is not defined (and not meaningful) outside it.
func windChillC(tempC, windMS float64) float64 {
	windKmh := windMS * 3.6
	if tempC > windChillMaxTempC || windKmh <= windChillMinWindKmh {
		return tempC
	}
	v := math.Pow(windKmh, 0.16)
	return 13.12 + 0.6215*tempC - 11.37*v + 0.3965*tempC*v
}

// feelsLikeC composites the three "how it actually feels" formulas into one
// Contract C value: heat index above heatIndexMinTempC, wind chill in the
// cold-and-windy regime it's valid for, otherwise the plain air temperature.
func feelsLikeC(tempC, humidityPercent, windMS float64) float64 {
	switch {
	case tempC >= heatIndexMinTempC:
		return tempestudp.HeatIndexC(tempC, humidityPercent)
	case tempC <= windChillMaxTempC && windMS*3.6 > windChillMinWindKmh:
		return windChillC(tempC, windMS)
	default:
		return tempC
	}
}

// pressureTrendFromHistory derives Contract C's pressureTrend enum from an
// ordered (by timestamp ascending, per HistoryPoints) slice of pressure
// samples: fewer than two points can't show a trend, so "steady"; otherwise
// compare the newest sample to the oldest against pressureTrendThresholdMB.
func pressureTrendFromHistory(points []sqlite.Point) string {
	if len(points) < 2 {
		return "steady"
	}
	delta := points[len(points)-1].V - points[0].V
	switch {
	case delta > pressureTrendThresholdMB:
		return "rising"
	case delta < -pressureTrendThresholdMB:
		return "falling"
	default:
		return "steady"
	}
}

// writeJSON encodes v as the JSON response body with status and the
// application/json Content-Type -- the shared "how a JSON API response is
// written" knowledge both observation handlers need (would change together
// if, e.g., the encoding needed a shared envelope added later).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("httpserver: encode json response", "error", err)
	}
}

// writeJSONError writes a {"error": message} JSON body.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
