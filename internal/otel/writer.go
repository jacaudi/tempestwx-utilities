// Package otel's Writer implements sink.MetricsWriter by recording each
// Tempest weather field onto a pre-registered OTel instrument. The
// instrument names are Contract B: chosen so the Collector's OTLP→Prometheus
// translation reproduces the exact existing tempest_* metric names that
// WS4's PromQL depends on. See the instrument-name constants below for the
// full table.
package otel

import (
	"context"
	"errors"
	"fmt"
	"math"

	"tempestwx-utilities/internal/tempest"
	"tempestwx-utilities/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// meterName is the instrumentation scope name for all instruments this
// writer registers.
const meterName = "tempestwx"

// Contract B instrument names — the law WS4's PromQL depends on. A wrong
// dot/underscore/suffix here breaks WS4, so these are defined once and
// referenced both at instrument-creation time and (indirectly, via the
// pointer-identity switch in WriteMetrics) at the old-Prometheus-descriptor
// mapping site.
const (
	instTemperatureC         = "tempest.temperature.c"
	instDewPointC            = "tempest.dewpoint.c"
	instHeatIndexC           = "tempest.heat_index.c"
	instWetBulbC             = "tempest.wetbulb.c"
	instHumidityPercent      = "tempest.humidity.percent"
	instPressureMb           = "tempest.pressure.mb"
	instWindMetersPerSecond  = "tempest.wind.meters_per_second"
	instWindDirectionDegrees = "tempest.wind.direction.degrees"
	instUVIndex              = "tempest.uv.index"
	instIrradianceWM2        = "tempest.irradiance.w_m2"
	instIlluminanceLux       = "tempest.illuminance.lux"
	instRainRateMmMin        = "tempest.rain_rate.mm_min"
	instRainfallMm           = "tempest.rainfall.mm"
	instLightningDistanceKm  = "tempest.lightning.distance.km"
	instLightningStrikeCount = "tempest.lightning.strike_count"
	instBatteryVolts         = "tempest.battery.volts"
	instRssiDbm              = "tempest.rssi.dbm"
	instUptimeSeconds        = "tempest.uptime.seconds"
	instReboots              = "tempest.reboots"
	instBusErrors            = "tempest.bus_errors"
)

// Writer implements sink.MetricsWriter, recording each Tempest report field
// onto its Contract B instrument. Instruments are created once at
// construction time and reused for the writer's lifetime.
type Writer struct {
	temperatureC         metric.Float64Gauge
	dewPointC            metric.Float64Gauge
	heatIndexC           metric.Float64Gauge
	wetBulbC             metric.Float64Gauge
	humidityPercent      metric.Float64Gauge
	pressureMb           metric.Float64Gauge
	windMetersPerSecond  metric.Float64Gauge
	windDirectionDegrees metric.Float64Gauge
	uvIndex              metric.Float64Gauge
	irradianceWM2        metric.Float64Gauge
	illuminanceLux       metric.Float64Gauge
	rainRateMmMin        metric.Float64Gauge
	lightningDistanceKm  metric.Float64Gauge
	batteryVolts         metric.Float64Gauge
	rssiDbm              metric.Float64Gauge
	uptimeSeconds        metric.Float64Gauge

	rainfallMm           metric.Float64Counter
	lightningStrikeCount metric.Float64Counter
	reboots              metric.Float64Counter
	busErrors            metric.Float64Counter
}

// NewWriter pre-registers every Contract B instrument on a Meter obtained
// from mp, returning an error if any instrument fails to register (e.g. a
// malformed name).
func NewWriter(mp metric.MeterProvider) (*Writer, error) {
	meter := mp.Meter(meterName)

	var errs []error
	newGauge := func(name string) metric.Float64Gauge {
		g, err := meter.Float64Gauge(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("create gauge %s: %w", name, err))
		}
		return g
	}
	newCounter := func(name string) metric.Float64Counter {
		c, err := meter.Float64Counter(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("create counter %s: %w", name, err))
		}
		return c
	}

	w := &Writer{
		temperatureC:         newGauge(instTemperatureC),
		dewPointC:            newGauge(instDewPointC),
		heatIndexC:           newGauge(instHeatIndexC),
		wetBulbC:             newGauge(instWetBulbC),
		humidityPercent:      newGauge(instHumidityPercent),
		pressureMb:           newGauge(instPressureMb),
		windMetersPerSecond:  newGauge(instWindMetersPerSecond),
		windDirectionDegrees: newGauge(instWindDirectionDegrees),
		uvIndex:              newGauge(instUVIndex),
		irradianceWM2:        newGauge(instIrradianceWM2),
		illuminanceLux:       newGauge(instIlluminanceLux),
		rainRateMmMin:        newGauge(instRainRateMmMin),
		lightningDistanceKm:  newGauge(instLightningDistanceKm),
		batteryVolts:         newGauge(instBatteryVolts),
		rssiDbm:              newGauge(instRssiDbm),
		uptimeSeconds:        newGauge(instUptimeSeconds),

		rainfallMm:           newCounter(instRainfallMm),
		lightningStrikeCount: newCounter(instLightningStrikeCount),
		reboots:              newCounter(instReboots),
		busErrors:            newCounter(instBusErrors),
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return w, nil
}

// serialAttrs builds the per-data-point attribute set: "serial" (the Tempest
// station serial number, replacing the old reserved "instance" label) plus
// any instrument-specific attributes such as "kind".
func serialAttrs(serial string, extra ...attribute.KeyValue) []attribute.KeyValue {
	return append([]attribute.KeyValue{attribute.String("serial", serial)}, extra...)
}

// WriteReport implements sink.MetricsWriter. It type-switches on the
// concrete report type (matching internal/postgres and internal/sqlite's
// writers) and reads raw obs fields directly — Contract B restructures the
// metric set relative to Report.Metrics()'s Prometheus output, so that
// output cannot be translated 1:1; see WriteMetrics for the API-export path,
// which does translate Report.Metrics() output.
func (w *Writer) WriteReport(ctx context.Context, report tempestudp.Report) error {
	switch r := report.(type) {
	case *tempestudp.TempestObservationReport:
		w.handleObservationReport(ctx, r)
	case *tempestudp.RapidWindReport:
		w.handleRapidWindReport(ctx, r)
	case *tempestudp.HubStatusReport:
		w.handleHubStatusReport(ctx, r)
	case *tempestudp.RainStartReport, *tempestudp.LightningStrikeReport:
		// No instruments — matches report.go, which returns nil Metrics()
		// for evt_precip/evt_strike.
	}
	return nil
}

func (w *Writer) handleObservationReport(ctx context.Context, r *tempestudp.TempestObservationReport) {
	for _, ob := range r.Obs {
		if len(ob) < 13 {
			continue
		}
		serial := r.SerialNumber

		w.gauge(ctx, w.windMetersPerSecond, ob[1], serial, attribute.String("kind", "lull"))
		w.gauge(ctx, w.windMetersPerSecond, ob[2], serial, attribute.String("kind", "avg"))
		w.gauge(ctx, w.windMetersPerSecond, ob[3], serial, attribute.String("kind", "gust"))
		w.gauge(ctx, w.windDirectionDegrees, ob[4], serial)
		w.gauge(ctx, w.pressureMb, ob[6], serial)
		w.gauge(ctx, w.temperatureC, ob[7], serial, attribute.String("kind", "air"))

		// WetBulbTemperatureC returns NaN for non-convergent inputs (e.g.
		// physically impossible humidity/pressure from a malformed report);
		// skip emitting the point rather than publishing NaN (mirrors the
		// same guard in tempestudp/report.go's Prometheus metrics path).
		wetBulb := tempestudp.WetBulbTemperatureC(ob[7], ob[8], ob[6])
		if !math.IsNaN(wetBulb) {
			w.gauge(ctx, w.wetBulbC, wetBulb, serial)
		}

		w.gauge(ctx, w.dewPointC, tempestudp.DewPointC(ob[7], ob[8]), serial)
		w.gauge(ctx, w.heatIndexC, tempestudp.HeatIndexC(ob[7], ob[8]), serial)

		w.gauge(ctx, w.humidityPercent, ob[8], serial)
		w.gauge(ctx, w.illuminanceLux, ob[9], serial)
		w.gauge(ctx, w.uvIndex, ob[10], serial)
		w.gauge(ctx, w.irradianceWM2, ob[11], serial)
		w.gauge(ctx, w.rainRateMmMin, ob[12], serial)
		w.counter(ctx, w.rainfallMm, ob[12], serial)

		// Lightning metrics (fields 14 and 15).
		if len(ob) >= 16 {
			w.gauge(ctx, w.lightningDistanceKm, ob[14], serial)
			w.counter(ctx, w.lightningStrikeCount, ob[15], serial)
		}
		if len(ob) >= 17 {
			w.gauge(ctx, w.batteryVolts, ob[16], serial)
		}
		// ob[13] (precip type) and ob[17] (report interval) are not in
		// Contract B — dropped, matching the brief's field mapping.
	}
}

func (w *Writer) handleRapidWindReport(ctx context.Context, r *tempestudp.RapidWindReport) {
	if len(r.Ob) != 3 {
		return
	}
	w.gauge(ctx, w.windMetersPerSecond, r.Ob[1], r.SerialNumber, attribute.String("kind", "rapid"))
	w.gauge(ctx, w.windDirectionDegrees, r.Ob[2], r.SerialNumber)
}

func (w *Writer) handleHubStatusReport(ctx context.Context, r *tempestudp.HubStatusReport) {
	w.gauge(ctx, w.uptimeSeconds, r.Uptime, r.SerialNumber)
	w.gauge(ctx, w.rssiDbm, r.Rssi, r.SerialNumber)

	// radio_stats[1] and [2] (reboots, bus errors) are only present on a
	// well-formed hub_status broadcast; a malformed/short array must not
	// panic (mirrors the same guard in postgres/sqlite's writers).
	if len(r.RadioStats) >= 3 {
		w.counter(ctx, w.reboots, r.RadioStats[1], r.SerialNumber)
		w.counter(ctx, w.busErrors, r.RadioStats[2], r.SerialNumber)
	}
}

// gauge and counter are the small internal helpers that DRY "record a
// gauge/counter with serial(+extra) attributes", per the writer's one
// required design note.
func (w *Writer) gauge(ctx context.Context, g metric.Float64Gauge, value float64, serial string, extra ...attribute.KeyValue) {
	g.Record(ctx, value, metric.WithAttributes(serialAttrs(serial, extra...)...))
}

func (w *Writer) counter(ctx context.Context, c metric.Float64Counter, value float64, serial string, extra ...attribute.KeyValue) {
	c.Add(ctx, value, metric.WithAttributes(serialAttrs(serial, extra...)...))
}

// WriteMetrics implements sink.MetricsWriter for API-export mode: it
// translates each incoming Prometheus metric (built against the OLD
// internal/tempest descriptors) to its Contract B instrument by matching on
// the exact *prometheus.Desc pointer (tempest.Wind, tempest.Temperature,
// etc. are package-level vars, so m.Desc() is the same pointer that
// Report.Metrics() used to build m) and reading its label/value via
// m.Write(&dto.Metric{}). The "instance" label value becomes the "serial"
// attribute. Metrics with no Contract B counterpart (tempest.ReportInterval,
// dropped in Contract B; tempest.RainTotal, never emitted by any
// Report.Metrics() implementation) are skipped via the default case.
//
// Known gap: tempest.dewpoint.c and tempest.heat_index.c are never
// populated via this path. Deriving them requires the SAME observation's
// temperature AND humidity together, but Report.Metrics() emits temperature
// and humidity as separate, independently-labeled prometheus.Metric values
// with no reliable correlation key across a flat []prometheus.Metric slice
// (especially once merged across multiple stations/timestamps in
// API-export mode). WriteReport is the primary, fully correct path — it has
// both raw fields together from the same ob row.
func (w *Writer) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	for _, m := range metrics {
		var d dto.Metric
		if err := m.Write(&d); err != nil {
			return fmt.Errorf("write prometheus metric %s: %w", m.Desc(), err)
		}

		serial := labelValue(&d, "instance")
		kind := labelValue(&d, "kind")
		value := metricValue(&d)

		switch m.Desc() {
		case tempest.Wind:
			w.gauge(ctx, w.windMetersPerSecond, value, serial, attribute.String("kind", kind))
		case tempest.WindDirection:
			w.gauge(ctx, w.windDirectionDegrees, value, serial)
		case tempest.Pressure:
			w.gauge(ctx, w.pressureMb, value, serial)
		case tempest.Temperature:
			switch kind {
			case "air":
				w.gauge(ctx, w.temperatureC, value, serial, attribute.String("kind", "air"))
			case "wetbulb":
				w.gauge(ctx, w.wetBulbC, value, serial)
			}
		case tempest.Humidity:
			w.gauge(ctx, w.humidityPercent, value, serial)
		case tempest.Illuminance:
			w.gauge(ctx, w.illuminanceLux, value, serial)
		case tempest.UV:
			w.gauge(ctx, w.uvIndex, value, serial)
		case tempest.Irradiance:
			w.gauge(ctx, w.irradianceWM2, value, serial)
		case tempest.RainRate:
			w.gauge(ctx, w.rainRateMmMin, value, serial)
		case tempest.LightningDistance:
			w.gauge(ctx, w.lightningDistanceKm, value, serial)
		case tempest.LightningStrikeCount:
			w.counter(ctx, w.lightningStrikeCount, value, serial)
		case tempest.Battery:
			w.gauge(ctx, w.batteryVolts, value, serial)
		case tempest.Uptime:
			w.gauge(ctx, w.uptimeSeconds, value, serial)
		case tempest.Rssi:
			w.gauge(ctx, w.rssiDbm, value, serial)
		case tempest.Reboots:
			w.counter(ctx, w.reboots, value, serial)
		case tempest.BusErrors:
			w.counter(ctx, w.busErrors, value, serial)
		}
	}
	return nil
}

// labelValue returns the value of the named label pair, or "" if absent.
func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

// metricValue extracts the numeric value from a dto.Metric regardless of
// whether it was built as a Gauge or a Counter.
func metricValue(m *dto.Metric) float64 {
	if g := m.GetGauge(); g != nil {
		return g.GetValue()
	}
	if c := m.GetCounter(); c != nil {
		return c.GetValue()
	}
	return 0
}

// Flush is a no-op: OTel's SDK-side push (via the PeriodicReader configured
// in Setup) owns export timing, not this writer.
func (w *Writer) Flush(ctx context.Context) error { return nil }

// Close is a no-op: provider lifecycle (including final flush) is owned by
// Setup's returned shutdown function (Task 6.1), not the writer.
func (w *Writer) Close(ctx context.Context) error { return nil }
