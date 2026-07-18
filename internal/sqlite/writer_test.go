package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"tempestwx-utilities/internal/tempestudp"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// newTestDB opens a fresh, migrated SQLite DB in a t.TempDir() for use by
// writer tests. FlushInterval is set to an hour so the ticker never fires
// during a test; tests force flushes explicitly via Writer.Flush.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(t.Context(), dbPath, Config{BusyTimeout: 5000 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	return db
}

// newTestWriter constructs a Writer backed by newTestDB, with a large
// batch size and a dormant ticker so tests control flushing explicitly.
func newTestWriter(t *testing.T) *Writer {
	t.Helper()
	db := newTestDB(t)
	w := NewWriter(t.Context(), db, Config{BatchSize: 100, FlushInterval: time.Hour})
	t.Cleanup(func() {
		if err := w.Close(t.Context()); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return w
}

// TestWriter_InsertsObservation proves WriteReport on an observation report
// produces a row with exact column values (field-by-field) and a UUIDv7 PK,
// and separately that fields absent from a short obs slice are stored as SQL
// NULL rather than a zero value (the task brief's explicit NULL-handling
// requirement).
func TestWriter_InsertsObservation(t *testing.T) {
	t.Run("full_fields", func(t *testing.T) {
		w := newTestWriter(t)
		ctx := t.Context()

		report := &tempestudp.TempestObservationReport{
			SerialNumber: "ST-00001",
			Obs: [][]float64{
				{1700000000, 1.5, 2.0, 2.5, 180, 3, 1013.25, 20.5, 55.0, 50000, 3, 500, 0.5, 1, 2.1, 4, 3.6, 5},
			},
		}

		if err := w.WriteReport(ctx, report); err != nil {
			t.Fatalf("WriteReport: %v", err)
		}
		if err := w.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		var (
			id                                         string
			serial                                     string
			ts                                         int64
			windLull, windAvg, windGust, windDirection float64
			windSampleInterval                         int64
			pressure, tempAir, tempWetbulb, humidity   float64
			illuminance, uvIndex, irradiance, rainRate float64
			precipType                                 int64
			lightningDistance                          float64
			lightningStrikeCount                       int64
			battery                                    float64
			reportInterval                             int64
		)
		row := w.db.QueryRowContext(ctx, `SELECT
			id, serial_number, timestamp,
			wind_lull, wind_avg, wind_gust, wind_direction, wind_sample_interval,
			pressure, temp_air, temp_wetbulb, humidity,
			illuminance, uv_index, irradiance, rain_rate, precip_type,
			lightning_distance, lightning_strike_count,
			battery, report_interval
			FROM tempest_observations`)
		if err := row.Scan(
			&id, &serial, &ts,
			&windLull, &windAvg, &windGust, &windDirection, &windSampleInterval,
			&pressure, &tempAir, &tempWetbulb, &humidity,
			&illuminance, &uvIndex, &irradiance, &rainRate, &precipType,
			&lightningDistance, &lightningStrikeCount,
			&battery, &reportInterval,
		); err != nil {
			t.Fatalf("scan observation row: %v", err)
		}

		parsedID, err := uuid.Parse(id)
		if err != nil {
			t.Fatalf("id %q is not a valid UUID: %v", id, err)
		}
		if parsedID.Version() != 7 {
			t.Errorf("id version = %d, want 7 (UUIDv7)", parsedID.Version())
		}

		if serial != "ST-00001" {
			t.Errorf("serial = %q, want ST-00001", serial)
		}
		if ts != 1700000000 {
			t.Errorf("timestamp = %d, want 1700000000", ts)
		}
		if windLull != 1.5 {
			t.Errorf("windLull = %v, want 1.5", windLull)
		}
		if windAvg != 2.0 {
			t.Errorf("windAvg = %v, want 2.0", windAvg)
		}
		if windGust != 2.5 {
			t.Errorf("windGust = %v, want 2.5", windGust)
		}
		if windDirection != 180.0 {
			t.Errorf("windDirection = %v, want 180.0", windDirection)
		}
		if windSampleInterval != 3 {
			t.Errorf("windSampleInterval = %v, want 3", windSampleInterval)
		}
		if pressure != 1013.25 {
			t.Errorf("pressure = %v, want 1013.25", pressure)
		}
		if tempAir != 20.5 {
			t.Errorf("tempAir = %v, want 20.5", tempAir)
		}
		if humidity != 55.0 {
			t.Errorf("humidity = %v, want 55.0", humidity)
		}
		if illuminance != 50000.0 {
			t.Errorf("illuminance = %v, want 50000.0", illuminance)
		}
		if uvIndex != 3.0 {
			t.Errorf("uvIndex = %v, want 3.0", uvIndex)
		}
		if irradiance != 500.0 {
			t.Errorf("irradiance = %v, want 500.0", irradiance)
		}
		if rainRate != 0.5 {
			t.Errorf("rainRate = %v, want 0.5", rainRate)
		}
		if precipType != 1 {
			t.Errorf("precipType = %v, want 1", precipType)
		}
		if lightningDistance != 2.1 {
			t.Errorf("lightningDistance = %v, want 2.1", lightningDistance)
		}
		if lightningStrikeCount != 4 {
			t.Errorf("lightningStrikeCount = %v, want 4", lightningStrikeCount)
		}
		if battery != 3.6 {
			t.Errorf("battery = %v, want 3.6", battery)
		}
		if reportInterval != 5 {
			t.Errorf("reportInterval = %v, want 5", reportInterval)
		}

		// Wet bulb is computed, not a literal input; just assert it's a
		// plausible convergent value (non-zero, less than air temp).
		if tempWetbulb <= 0 || tempWetbulb >= tempAir {
			t.Errorf("tempWetbulb = %v, want a plausible convergent value below tempAir=%v", tempWetbulb, tempAir)
		}
	})

	t.Run("nullable_fields_absent", func(t *testing.T) {
		w := newTestWriter(t)
		ctx := t.Context()

		// Minimum valid length (13, indices 0-12): fields 13-17 (precip_type,
		// lightning distance/count, battery, report_interval) are absent ->
		// must read back as SQL NULL, not zero.
		report := &tempestudp.TempestObservationReport{
			SerialNumber: "ST-SHORT",
			Obs: [][]float64{
				{1700000001, 1, 1, 1, 1, 1, 1013, 20, 50, 100, 1, 10, 0.5},
			},
		}

		if err := w.WriteReport(ctx, report); err != nil {
			t.Fatalf("WriteReport: %v", err)
		}
		if err := w.Flush(ctx); err != nil {
			t.Fatalf("Flush: %v", err)
		}

		var (
			windSampleInterval                               int64
			precipType, lightningStrikeCount, reportInterval sql.NullInt64
			lightningDistance, battery                       sql.NullFloat64
		)
		row := w.db.QueryRowContext(ctx, `SELECT
			wind_sample_interval, precip_type, lightning_distance, lightning_strike_count,
			battery, report_interval
			FROM tempest_observations WHERE serial_number = ?`, "ST-SHORT")
		if err := row.Scan(
			&windSampleInterval, &precipType, &lightningDistance, &lightningStrikeCount,
			&battery, &reportInterval,
		); err != nil {
			t.Fatalf("scan observation row: %v", err)
		}

		// wind_sample_interval (index 5) IS present at the minimum valid
		// length of 13 -> must NOT be NULL.
		if windSampleInterval != 1 {
			t.Errorf("wind_sample_interval = %d, want 1 (present at len=13)", windSampleInterval)
		}
		if precipType.Valid {
			t.Errorf("precip_type should be NULL, got %v", precipType.Int64)
		}
		if lightningDistance.Valid {
			t.Errorf("lightning_distance should be NULL, got %v", lightningDistance.Float64)
		}
		if lightningStrikeCount.Valid {
			t.Errorf("lightning_strike_count should be NULL, got %v", lightningStrikeCount.Int64)
		}
		if battery.Valid {
			t.Errorf("battery should be NULL, got %v", battery.Float64)
		}
		if reportInterval.Valid {
			t.Errorf("report_interval should be NULL, got %v", reportInterval.Int64)
		}
	})
}

// TestWriter_OnConflictIdempotent proves writing the same (serial_number,
// timestamp) observation twice yields exactly one row (ON CONFLICT DO
// NOTHING on the UNIQUE(serial_number, timestamp) constraint).
func TestWriter_OnConflictIdempotent(t *testing.T) {
	w := newTestWriter(t)
	ctx := t.Context()

	report := &tempestudp.TempestObservationReport{
		SerialNumber: "ST-DUP",
		Obs: [][]float64{
			{1700000002, 1, 1, 1, 1, 1, 1013, 20, 50, 100, 1, 10, 0},
		},
	}

	if err := w.WriteReport(ctx, report); err != nil {
		t.Fatalf("first WriteReport: %v", err)
	}
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("first Flush: %v", err)
	}
	if err := w.WriteReport(ctx, report); err != nil {
		t.Fatalf("second WriteReport: %v", err)
	}
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("second Flush: %v", err)
	}

	var count int
	if err := w.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tempest_observations WHERE serial_number = ?`, "ST-DUP",
	).Scan(&count); err != nil {
		t.Fatalf("count observations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after duplicate write, got %d", count)
	}
}

// TestWriter_RoutesReportTypes proves each of the five report types lands in
// its corresponding table.
func TestWriter_RoutesReportTypes(t *testing.T) {
	w := newTestWriter(t)
	ctx := t.Context()

	obs := &tempestudp.TempestObservationReport{
		SerialNumber: "ST-ROUTE",
		Obs:          [][]float64{{1700000010, 1, 1, 1, 1, 1, 1013, 20, 50, 100, 1, 10, 0}},
	}
	rapidWind := &tempestudp.RapidWindReport{
		SerialNumber: "ST-ROUTE",
		Ob:           []float64{1700000011, 5.5, 90},
	}
	hub := &tempestudp.HubStatusReport{
		SerialNumber: "ST-ROUTE",
		Timestamp:    1700000012,
		Uptime:       12345,
		Rssi:         -60,
		RadioStats:   []float64{17, 2, 0},
	}
	rainStart := &tempestudp.RainStartReport{
		SerialNumber: "ST-ROUTE",
		Evt:          []float64{1700000013},
	}
	lightning := &tempestudp.LightningStrikeReport{
		SerialNumber: "ST-ROUTE",
		Evt:          []float64{1700000014, 3.2, 100},
	}

	for _, report := range []tempestudp.Report{obs, rapidWind, hub, rainStart, lightning} {
		if err := w.WriteReport(ctx, report); err != nil {
			t.Fatalf("WriteReport(%T): %v", report, err)
		}
	}
	if err := w.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	assertCount := func(table, where string, want int) {
		t.Helper()
		var got int
		q := `SELECT COUNT(*) FROM ` + table + ` WHERE serial_number = 'ST-ROUTE'`
		if where != "" {
			q += " AND " + where
		}
		if err := w.db.QueryRowContext(ctx, q).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Errorf("%s count = %d, want %d", table, got, want)
		}
	}

	assertCount("tempest_observations", "", 1)
	assertCount("tempest_rapid_wind", "", 1)
	assertCount("tempest_hub_status", "", 1)
	assertCount("tempest_events", "event_type = 'rain_start'", 1)
	assertCount("tempest_events", "event_type = 'lightning_strike'", 1)
}

// TestWriter_EventsNotDroppedUnderBackpressure proves a discrete event
// (lightning/rain-start) is never silently dropped just because the row
// channel is momentarily saturated: enqueueEvent blocks up to
// eventBlockTimeout and succeeds once space frees up, whereas a continuous
// row (enqueue) is dropped immediately once the channel is full (C-MEDIUM).
// Also pins the exact constants the design requires.
func TestWriter_EventsNotDroppedUnderBackpressure(t *testing.T) {
	if rowChanCap != 1000 {
		t.Fatalf("rowChanCap = %d, want 1000", rowChanCap)
	}
	if eventBlockTimeout != 5*time.Second {
		t.Fatalf("eventBlockTimeout = %v, want 5s", eventBlockTimeout)
	}

	// Construct the Writer directly (bypassing NewWriter) so no background
	// goroutine drains w.rows concurrently -- that would make "fill the
	// channel to capacity" a race instead of a deterministic setup.
	w := &Writer{
		rows: make(chan rowEnvelope, rowChanCap),
		done: make(chan struct{}),
	}

	for range rowChanCap {
		w.rows <- rowEnvelope{kind: kindObservation, payload: observationRow{}}
	}

	// A continuous row must be dropped (non-blocking) once the channel is
	// full, not block.
	enqueueDone := make(chan error, 1)
	go func() {
		enqueueDone <- w.enqueue(t.Context(), rowEnvelope{kind: kindObservation, payload: observationRow{}})
	}()
	select {
	case err := <-enqueueDone:
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("enqueue blocked instead of dropping when channel full")
	}
	if len(w.rows) != rowChanCap {
		t.Fatalf("channel length changed after dropped continuous row: got %d, want %d", len(w.rows), rowChanCap)
	}

	// A discrete event must block rather than drop, and succeed once a slot
	// frees up (simulating the writer goroutine draining one row).
	freed := make(chan struct{})
	go func() {
		<-w.rows // free one slot, simulating the writer goroutine draining
		close(freed)
	}()

	eventDone := make(chan error, 1)
	go func() {
		eventDone <- w.enqueueEvent(t.Context(), rowEnvelope{kind: kindEvent, payload: eventRow{eventType: "lightning_strike"}})
	}()

	select {
	case <-freed:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the simulated drain")
	}

	select {
	case err := <-eventDone:
		if err != nil {
			t.Fatalf("enqueueEvent: %v", err)
		}
	case <-time.After(eventBlockTimeout):
		t.Fatal("enqueueEvent did not unblock once the channel had room")
	}
}
