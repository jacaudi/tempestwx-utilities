package otel

import (
	"math"
	"testing"

	"tempestwx-utilities/internal/tempestudp"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// findMetric locates a recorded instrument by its exact Contract B name.
func findMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

// dataPointFor returns the data point recorded with serial=serial, from
// either a Gauge's or a Sum's DataPoints slice.
func dataPointFor(points []metricdata.DataPoint[float64], serial string) (metricdata.DataPoint[float64], bool) {
	for _, dp := range points {
		if v, ok := dp.Attributes.Value("serial"); ok && v.AsString() == serial {
			return dp, true
		}
	}
	return metricdata.DataPoint[float64]{}, false
}

// gaugePointFor returns the Gauge data point recorded with serial=serial, if
// the instrument's aggregation is a Gauge.
func gaugePointFor(t *testing.T, m metricdata.Metrics, serial string) (metricdata.DataPoint[float64], bool) {
	t.Helper()
	g, ok := m.Data.(metricdata.Gauge[float64])
	if !ok {
		t.Fatalf("%s: data is %T, want metricdata.Gauge[float64]", m.Name, m.Data)
	}
	return dataPointFor(g.DataPoints, serial)
}

// sumPointFor returns the Sum (Counter) data point recorded with
// serial=serial, if the instrument's aggregation is a Sum.
func sumPointFor(t *testing.T, m metricdata.Metrics, serial string) (metricdata.DataPoint[float64], bool) {
	t.Helper()
	s, ok := m.Data.(metricdata.Sum[float64])
	if !ok {
		t.Fatalf("%s: data is %T, want metricdata.Sum[float64]", m.Name, m.Data)
	}
	return dataPointFor(s.DataPoints, serial)
}

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 0.001 }

// TestWriter_RecordsInstruments feeds one TempestObservationReport, one
// RapidWindReport, and one HubStatusReport through the writer and asserts
// the collected metricdata carries the EXACT Contract B instrument names
// with the right kinds (Gauge vs Sum), the recorded values, "serial" (not
// "instance") as the identifying attribute, and "kind" where required.
func TestWriter_RecordsInstruments(t *testing.T) {
	ctx := t.Context()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	w, err := NewWriter(mp)
	if err != nil {
		t.Fatalf("NewWriter() returned unexpected error: %v", err)
	}

	const serial = "TEST-001"
	obs := &tempestudp.TempestObservationReport{
		SerialNumber: serial,
		Obs: [][]float64{
			{
				0,       // 0 time epoch
				1.5,     // 1 wind lull
				2.5,     // 2 wind avg
				3.5,     // 3 wind gust
				180,     // 4 wind direction
				60,      // 5 wind sample interval
				1013.25, // 6 station pressure mb
				25,      // 7 air temp C
				50,      // 8 humidity %
				1000,    // 9 illuminance lux
				5,       // 10 uv index
				400,     // 11 irradiance w/m2
				0.2,     // 12 rain amount mm (previous minute)
				0,       // 13 precip type
				2.0,     // 14 lightning distance km
				3,       // 15 lightning strike count
				2.75,    // 16 battery volts
				1,       // 17 report interval minutes
			},
		},
	}
	if err := w.WriteReport(ctx, obs); err != nil {
		t.Fatalf("WriteReport(observation) returned unexpected error: %v", err)
	}

	rapid := &tempestudp.RapidWindReport{
		SerialNumber: serial,
		Ob:           []float64{0, 4.2, 90},
	}
	if err := w.WriteReport(ctx, rapid); err != nil {
		t.Fatalf("WriteReport(rapid wind) returned unexpected error: %v", err)
	}

	hub := &tempestudp.HubStatusReport{
		SerialNumber: serial,
		Uptime:       12345,
		Rssi:         -60,
		RadioStats:   []float64{1, 4, 2},
	}
	if err := w.WriteReport(ctx, hub); err != nil {
		t.Fatalf("WriteReport(hub status) returned unexpected error: %v", err)
	}

	// Second observation report, a different serial, engineered so
	// WetBulbTemperatureC never converges (humidity=-500, physically
	// impossible — see wetbulb_test.go's TestWetBulb_NonConvergentReturnsNaN
	// for the same input) — asserts the NaN wetbulb is skipped rather than
	// recorded.
	const nanSerial = "NAN-TEST"
	nanObs := &tempestudp.TempestObservationReport{
		SerialNumber: nanSerial,
		Obs: [][]float64{
			{0, 1, 2, 3, 180, 60, 900, 25, -500, 100, 1, 50, 0.1},
		},
	}
	if err := w.WriteReport(ctx, nanObs); err != nil {
		t.Fatalf("WriteReport(nan observation) returned unexpected error: %v", err)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("reader.Collect() returned unexpected error: %v", err)
	}

	wetBulbWant := tempestudp.WetBulbTemperatureC(25, 50, 1013.25)
	dewPointWant := tempestudp.DewPointC(25, 50)
	heatIndexWant := tempestudp.HeatIndexC(25, 50)

	gaugeCases := []struct {
		instrument string
		want       float64
		kind       string // "" means no kind attribute asserted
	}{
		{"tempest.temperature.c", 25, "air"},
		{"tempest.dewpoint.c", dewPointWant, ""},
		{"tempest.heat_index.c", heatIndexWant, ""},
		{"tempest.wetbulb.c", wetBulbWant, ""},
		{"tempest.humidity.percent", 50, ""},
		{"tempest.pressure.mb", 1013.25, ""},
		// tempest.wind.direction.degrees carries no "kind" attribute in
		// Contract B, so the observation report's value (180) and the
		// later rapid-wind report's value (90) share the same {serial}
		// attribute set — the Gauge's last recorded value (90, from the
		// rapid-wind report written after the observation report above)
		// wins, which is the correct "current direction" semantics for a
		// Gauge fed by two sources.
		{"tempest.wind.direction.degrees", 90, ""},
		{"tempest.uv.index", 5, ""},
		{"tempest.irradiance.w_m2", 400, ""},
		{"tempest.illuminance.lux", 1000, ""},
		{"tempest.rain_rate.mm_min", 0.2, ""},
		{"tempest.lightning.distance.km", 2.0, ""},
		{"tempest.battery.volts", 2.75, ""},
		{"tempest.rssi.dbm", -60, ""},
		{"tempest.uptime.seconds", 12345, ""},
	}
	for _, tc := range gaugeCases {
		t.Run("gauge/"+tc.instrument, func(t *testing.T) {
			m, ok := findMetric(rm, tc.instrument)
			if !ok {
				t.Fatalf("instrument %q not found in collected metrics", tc.instrument)
			}
			dp, ok := gaugePointFor(t, m, serial)
			if !ok {
				t.Fatalf("instrument %q: no data point for serial=%q", tc.instrument, serial)
			}
			if !almostEqual(dp.Value, tc.want) {
				t.Errorf("instrument %q value = %v, want %v", tc.instrument, dp.Value, tc.want)
			}
			if _, hasInstance := dp.Attributes.Value("instance"); hasInstance {
				t.Errorf("instrument %q: unexpected legacy 'instance' attribute present", tc.instrument)
			}
			if tc.kind != "" {
				v, ok := dp.Attributes.Value("kind")
				if !ok || v.AsString() != tc.kind {
					t.Errorf("instrument %q: kind attribute = %v (ok=%v), want %q", tc.instrument, v, ok, tc.kind)
				}
			}
		})
	}

	// Wind is recorded three times for the observation report (lull, avg,
	// gust) and once more for the rapid-wind report (rapid) — all under the
	// single tempest.wind.meters_per_second instrument, disambiguated by kind.
	t.Run("gauge/tempest.wind.meters_per_second", func(t *testing.T) {
		m, ok := findMetric(rm, "tempest.wind.meters_per_second")
		if !ok {
			t.Fatal("instrument tempest.wind.meters_per_second not found")
		}
		g, ok := m.Data.(metricdata.Gauge[float64])
		if !ok {
			t.Fatalf("tempest.wind.meters_per_second: data is %T, want Gauge[float64]", m.Data)
		}
		want := map[string]float64{"lull": 1.5, "avg": 2.5, "gust": 3.5, "rapid": 4.2}
		got := map[string]float64{}
		for _, dp := range g.DataPoints {
			if v, ok := dp.Attributes.Value("serial"); !ok || v.AsString() != serial {
				continue
			}
			kind, ok := dp.Attributes.Value("kind")
			if !ok {
				t.Fatalf("wind data point missing 'kind' attribute: %+v", dp.Attributes)
			}
			got[kind.AsString()] = dp.Value
		}
		for kind, want := range want {
			gotV, ok := got[kind]
			if !ok || !almostEqual(gotV, want) {
				t.Errorf("wind kind=%q value = %v (ok=%v), want %v", kind, gotV, ok, want)
			}
		}
	})

	counterCases := []struct {
		instrument string
		want       float64
	}{
		{"tempest.rainfall.mm", 0.2},
		{"tempest.lightning.strike_count", 3},
		{"tempest.reboots", 4},
		{"tempest.bus_errors", 2},
	}
	for _, tc := range counterCases {
		t.Run("counter/"+tc.instrument, func(t *testing.T) {
			m, ok := findMetric(rm, tc.instrument)
			if !ok {
				t.Fatalf("instrument %q not found in collected metrics", tc.instrument)
			}
			dp, ok := sumPointFor(t, m, serial)
			if !ok {
				t.Fatalf("instrument %q: no data point for serial=%q", tc.instrument, serial)
			}
			if !almostEqual(dp.Value, tc.want) {
				t.Errorf("instrument %q value = %v, want %v", tc.instrument, dp.Value, tc.want)
			}
		})
	}

	t.Run("wetbulb NaN input is skipped", func(t *testing.T) {
		m, ok := findMetric(rm, "tempest.wetbulb.c")
		if !ok {
			t.Fatal("instrument tempest.wetbulb.c not found")
		}
		if _, ok := gaugePointFor(t, m, nanSerial); ok {
			t.Errorf("tempest.wetbulb.c: expected no data point for serial=%q (NaN wetbulb), but found one", nanSerial)
		}
	})
}
