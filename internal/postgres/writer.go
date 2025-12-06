package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"tempestwx-utilities/internal/tempestudp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	io_prometheus_client "github.com/prometheus/client_model/go"
)

// Row types for each table
type observationRow struct {
	id                   uuid.UUID
	serialNumber         string
	timestamp            time.Time
	windLull             float64
	windAvg              float64
	windGust             float64
	windDirection        float64
	windSampleInterval   *float64
	pressure             float64
	tempAir              float64
	tempWetbulb          float64
	humidity             float64
	illuminance          float64
	uvIndex              float64
	irradiance           float64
	rainRate             float64
	precipType           *int
	lightningDistance    *float64
	lightningStrikeCount *float64
	battery              *float64
	reportInterval       *float64
}

type rapidWindRow struct {
	id            uuid.UUID
	serialNumber  string
	timestamp     time.Time
	windSpeed     float64
	windDirection float64
}

type hubStatusRow struct {
	id           uuid.UUID
	serialNumber string
	timestamp    time.Time
	uptime       float64
	rssi         float64
	rebootCount  float64
	busErrors    float64
}

type eventRow struct {
	id           uuid.UUID
	serialNumber string
	timestamp    time.Time
	eventType    string
	distanceKm   *float64
	energy       *float64
}

// PostgresWriter writes metrics to PostgreSQL with batching and retry logic.
type PostgresWriter struct {
	pool *pgxpool.Pool

	// Batch channels per table
	obsBatch   chan observationRow
	windBatch  chan rapidWindRow
	hubBatch   chan hubStatusRow
	eventBatch chan eventRow

	// Configuration
	batchSize     int
	flushInterval time.Duration
	maxRetries    int

	ctx context.Context
	wg  sync.WaitGroup
}

// NewPostgresWriter creates a new PostgreSQL writer with connection pooling.
func NewPostgresWriter(ctx context.Context, databaseURL string) (*PostgresWriter, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	// Connection pool configuration
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 10 * time.Minute
	config.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Auto-create schema
	if err := CreateSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	log.Printf("postgres: connected, schema ready")

	// Initialize writer
	w := &PostgresWriter{
		pool:          pool,
		obsBatch:      make(chan observationRow, 1000),
		windBatch:     make(chan rapidWindRow, 1000),
		hubBatch:      make(chan hubStatusRow, 1000),
		eventBatch:    make(chan eventRow, 1000),
		batchSize:     100,
		flushInterval: 10 * time.Second,
		maxRetries:    3,
		ctx:           ctx,
	}

	// Start background batch workers
	w.wg.Add(4)
	go w.batchObservations()
	go w.batchRapidWind()
	go w.batchHubStatus()
	go w.batchEvents()

	return w, nil
}

// batchObservations handles observation rows with immediate flush (1 row batches)
func (w *PostgresWriter) batchObservations() {
	defer w.wg.Done()

	for {
		select {
		case row, ok := <-w.obsBatch:
			if !ok {
				return // Channel closed
			}
			// Immediate flush for observations
			w.flushObservations([]observationRow{row})

		case <-w.ctx.Done():
			return
		}
	}
}

func (w *PostgresWriter) flushObservations(batch []observationRow) {
	w.flushWithRetry(func() error {
		return w.insertObservations(batch)
	}, "tempest_observations", len(batch))
}

func (w *PostgresWriter) insertObservations(batch []observationRow) error {
	if len(batch) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	b := &pgx.Batch{}

	for _, row := range batch {
		b.Queue(`
			INSERT INTO tempest_observations (
				serial_number, timestamp, wind_lull, wind_avg, wind_gust,
				wind_direction, pressure, temp_air, temp_wetbulb, humidity,
				illuminance, uv_index, irradiance, rain_rate,
				lightning_distance, lightning_strike_count,
				battery, report_interval
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
			ON CONFLICT (serial_number, timestamp) DO NOTHING
		`, row.serialNumber, row.timestamp, row.windLull, row.windAvg, row.windGust,
			row.windDirection, row.pressure, row.tempAir, row.tempWetbulb, row.humidity,
			row.illuminance, row.uvIndex, row.irradiance, row.rainRate,
			row.lightningDistance, row.lightningStrikeCount,
			row.battery, row.reportInterval)
	}

	br := w.pool.SendBatch(ctx, b)
	defer br.Close()

	for i := 0; i < len(batch); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("insert observation %d: %w", i, err)
		}
	}

	return nil
}

func (w *PostgresWriter) batchRapidWind() {
	defer w.wg.Done()

	batch := make([]rapidWindRow, 0, w.batchSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case row, ok := <-w.windBatch:
			if !ok {
				// Channel closed - flush remaining
				if len(batch) > 0 {
					w.flushRapidWind(batch)
				}
				return
			}

			batch = append(batch, row)

			// Flush when batch is full
			if len(batch) >= w.batchSize {
				w.flushRapidWind(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Periodic flush
			if len(batch) > 0 {
				w.flushRapidWind(batch)
				batch = batch[:0]
			}

		case <-w.ctx.Done():
			// Shutdown - flush remaining
			if len(batch) > 0 {
				w.flushRapidWind(batch)
			}
			return
		}
	}
}

func (w *PostgresWriter) flushRapidWind(batch []rapidWindRow) {
	w.flushWithRetry(func() error {
		return w.insertRapidWind(batch)
	}, "tempest_rapid_wind", len(batch))
}

func (w *PostgresWriter) insertRapidWind(batch []rapidWindRow) error {
	if len(batch) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	b := &pgx.Batch{}

	for _, row := range batch {
		b.Queue(`
			INSERT INTO tempest_rapid_wind (
				serial_number, timestamp, wind_speed, wind_direction
			) VALUES ($1, $2, $3, $4)
			ON CONFLICT (serial_number, timestamp) DO NOTHING
		`, row.serialNumber, row.timestamp, row.windSpeed, row.windDirection)
	}

	br := w.pool.SendBatch(ctx, b)
	defer br.Close()

	for i := 0; i < len(batch); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("insert rapid_wind %d: %w", i, err)
		}
	}

	return nil
}

func (w *PostgresWriter) batchHubStatus() {
	defer w.wg.Done()

	batch := make([]hubStatusRow, 0, w.batchSize)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case row, ok := <-w.hubBatch:
			if !ok {
				if len(batch) > 0 {
					w.flushHubStatus(batch)
				}
				return
			}

			batch = append(batch, row)
			if len(batch) >= w.batchSize {
				w.flushHubStatus(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				w.flushHubStatus(batch)
				batch = batch[:0]
			}

		case <-w.ctx.Done():
			if len(batch) > 0 {
				w.flushHubStatus(batch)
			}
			return
		}
	}
}

func (w *PostgresWriter) flushHubStatus(batch []hubStatusRow) {
	w.flushWithRetry(func() error {
		return w.insertHubStatus(batch)
	}, "tempest_hub_status", len(batch))
}

func (w *PostgresWriter) insertHubStatus(batch []hubStatusRow) error {
	if len(batch) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	b := &pgx.Batch{}

	for _, row := range batch {
		b.Queue(`
			INSERT INTO tempest_hub_status (
				serial_number, timestamp, uptime, rssi, reboot_count, bus_errors
			) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (serial_number, timestamp) DO NOTHING
		`, row.serialNumber, row.timestamp, row.uptime, row.rssi, row.rebootCount, row.busErrors)
	}

	br := w.pool.SendBatch(ctx, b)
	defer br.Close()

	for i := 0; i < len(batch); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("insert hub_status %d: %w", i, err)
		}
	}

	return nil
}

func (w *PostgresWriter) batchEvents() {
	defer w.wg.Done()

	for {
		select {
		case row, ok := <-w.eventBatch:
			if !ok {
				return
			}
			// Immediate flush for events (critical)
			w.flushEvents([]eventRow{row})

		case <-w.ctx.Done():
			return
		}
	}
}

func (w *PostgresWriter) flushEvents(batch []eventRow) {
	w.flushWithRetry(func() error {
		return w.insertEvents(batch)
	}, "tempest_events", len(batch))
}

func (w *PostgresWriter) insertEvents(batch []eventRow) error {
	if len(batch) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	b := &pgx.Batch{}

	for _, row := range batch {
		b.Queue(`
			INSERT INTO tempest_events (
				serial_number, timestamp, event_type, distance_km, energy
			) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (serial_number, timestamp, event_type) DO NOTHING
		`, row.serialNumber, row.timestamp, row.eventType, row.distanceKm, row.energy)
	}

	br := w.pool.SendBatch(ctx, b)
	defer br.Close()

	for i := 0; i < len(batch); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("insert event %d: %w", i, err)
		}
	}

	return nil
}

// WriteReport implements MetricsWriter interface
func (w *PostgresWriter) WriteReport(ctx context.Context, report tempestudp.Report) error {
	switch r := report.(type) {
	case *tempestudp.TempestObservationReport:
		return w.handleObservationReport(ctx, r)

	case *tempestudp.RapidWindReport:
		return w.handleRapidWindReport(ctx, r)

	case *tempestudp.HubStatusReport:
		return w.handleHubStatusReport(ctx, r)

	case *tempestudp.RainStartReport:
		return w.handleRainStartReport(ctx, r)

	case *tempestudp.LightningStrikeReport:
		return w.handleLightningStrikeReport(ctx, r)

	default:
		// Unknown report type (e.g., device_status) - not an error
		return nil
	}
}

func (w *PostgresWriter) handleObservationReport(ctx context.Context, r *tempestudp.TempestObservationReport) error {
	for _, ob := range r.Obs {
		if len(ob) < 13 {
			continue
		}

		ts := time.Unix(int64(ob[0]), 0)

		// Calculate wet bulb temperature (from tempestudp package)
		wetBulb := tempestudp.WetBulbTemperatureC(ob[7], ob[8], ob[6])

		row := observationRow{
			id:            uuid.Must(uuid.NewV7()), // Generate UUIDv7
			serialNumber:  r.SerialNumber,
			timestamp:     ts,
			windLull:      ob[1],
			windAvg:       ob[2],
			windGust:      ob[3],
			windDirection: ob[4],
			pressure:      ob[6], // Raw mb value (no conversion)
			tempAir:       ob[7],
			tempWetbulb:   wetBulb,
			humidity:      ob[8],
			illuminance:   ob[9],
			uvIndex:       ob[10],
			irradiance:    ob[11],
			rainRate:      ob[12],
		}

		// Field 5: wind_sample_interval (seconds)
		if len(ob) >= 6 {
			interval := ob[5]
			row.windSampleInterval = &interval
		}

		// Field 13: precip_type (0=none, 1=rain, 2=hail, 3=rain+hail)
		if len(ob) >= 14 {
			precipType := int(ob[13])
			row.precipType = &precipType
		}

		// Lightning fields (14 and 15)
		if len(ob) >= 16 {
			distance := ob[14]
			count := ob[15]
			row.lightningDistance = &distance
			row.lightningStrikeCount = &count
		}

		// Field 16: battery
		if len(ob) >= 17 {
			battery := ob[16]
			row.battery = &battery
		}

		// Field 17: report_interval (minutes - raw value)
		if len(ob) >= 18 {
			interval := ob[17] // Raw minutes value (no conversion)
			row.reportInterval = &interval
		}

		// Send to batch channel (non-blocking)
		select {
		case w.obsBatch <- row:
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Printf("postgres: observation batch channel full, dropping")
		}
	}

	return nil
}

func (w *PostgresWriter) handleRapidWindReport(ctx context.Context, r *tempestudp.RapidWindReport) error {
	if len(r.Ob) != 3 {
		return nil // Invalid data
	}

	ts := time.Unix(int64(r.Ob[0]), 0)

	row := rapidWindRow{
		id:            uuid.Must(uuid.NewV7()), // Generate UUIDv7
		serialNumber:  r.SerialNumber,
		timestamp:     ts,
		windSpeed:     r.Ob[1],
		windDirection: r.Ob[2],
	}

	// Send to batch channel (non-blocking)
	select {
	case w.windBatch <- row:
	case <-ctx.Done():
		return ctx.Err()
	default:
		log.Printf("postgres: rapid_wind batch channel full, dropping")
	}

	return nil
}

func (w *PostgresWriter) handleHubStatusReport(ctx context.Context, r *tempestudp.HubStatusReport) error {
	if len(r.RadioStats) < 3 {
		return nil // Invalid data
	}

	ts := time.Unix(r.Timestamp, 0)

	row := hubStatusRow{
		id:           uuid.Must(uuid.NewV7()), // Generate UUIDv7
		serialNumber: r.SerialNumber,
		timestamp:    ts,
		uptime:       r.Uptime,
		rssi:         r.Rssi,
		rebootCount:  r.RadioStats[1],
		busErrors:    r.RadioStats[2],
	}

	// Send to batch channel (non-blocking)
	select {
	case w.hubBatch <- row:
	case <-ctx.Done():
		return ctx.Err()
	default:
		log.Printf("postgres: hub_status batch channel full, dropping")
	}

	return nil
}

func (w *PostgresWriter) handleRainStartReport(ctx context.Context, r *tempestudp.RainStartReport) error {
	if len(r.Evt) < 1 {
		return nil // Invalid data
	}

	ts := time.Unix(int64(r.Evt[0]), 0)

	row := eventRow{
		id:           uuid.Must(uuid.NewV7()), // Generate UUIDv7
		serialNumber: r.SerialNumber,
		timestamp:    ts,
		eventType:    "rain_start",
		distanceKm:   nil, // Not applicable for rain
		energy:       nil, // Not applicable for rain
	}

	// Send to batch channel (non-blocking)
	select {
	case w.eventBatch <- row:
	case <-ctx.Done():
		return ctx.Err()
	default:
		log.Printf("postgres: event batch channel full, dropping")
	}

	return nil
}

func (w *PostgresWriter) handleLightningStrikeReport(ctx context.Context, r *tempestudp.LightningStrikeReport) error {
	if len(r.Evt) < 3 {
		return nil // Invalid data
	}

	ts := time.Unix(int64(r.Evt[0]), 0)
	distance := r.Evt[1]
	energy := r.Evt[2]

	row := eventRow{
		id:           uuid.Must(uuid.NewV7()), // Generate UUIDv7
		serialNumber: r.SerialNumber,
		timestamp:    ts,
		eventType:    "lightning_strike",
		distanceKm:   &distance,
		energy:       &energy,
	}

	// Send to batch channel (non-blocking)
	select {
	case w.eventBatch <- row:
	case <-ctx.Done():
		return ctx.Err()
	default:
		log.Printf("postgres: event batch channel full, dropping")
	}

	return nil
}

// WriteMetrics implements MetricsWriter interface
// This is used by the API export mode to write historical data
func (w *PostgresWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	// Group metrics by timestamp and serial number to reconstruct observations
	type metricKey struct {
		serialNumber string
		timestamp    time.Time
	}

	observations := make(map[metricKey]*observationRow)

	for _, metric := range metrics {
		var dto io_prometheus_client.Metric
		if err := metric.Write(&dto); err != nil {
			log.Printf("postgres: failed to write metric: %v", err)
			continue
		}

		// Extract serial number from instance label
		var serialNumber string
		for _, label := range dto.GetLabel() {
			if label.GetName() == "instance" {
				serialNumber = label.GetValue()
				break
			}
		}
		if serialNumber == "" {
			continue
		}

		// Extract timestamp
		ts := time.UnixMilli(dto.GetTimestampMs())
		key := metricKey{serialNumber: serialNumber, timestamp: ts}

		// Get or create observation row
		obs, exists := observations[key]
		if !exists {
			obs = &observationRow{
				serialNumber: serialNumber,
				timestamp:    ts,
			}
			observations[key] = obs
		}

		// Extract value
		var value float64
		if dto.GetGauge() != nil {
			value = dto.GetGauge().GetValue()
		} else if dto.GetCounter() != nil {
			value = dto.GetCounter().GetValue()
		}

		// Map metric to field based on descriptor
		desc := metric.Desc().String()
		switch {
		case strings.Contains(desc, "tempest_wind_ms"):
			// Check kind label
			for _, label := range dto.GetLabel() {
				if label.GetName() == "kind" {
					switch label.GetValue() {
					case "lull":
						obs.windLull = value
					case "avg":
						obs.windAvg = value
					case "gust":
						obs.windGust = value
					}
					break
				}
			}
		case strings.Contains(desc, "tempest_wind_direction_degrees"):
			obs.windDirection = value
		case strings.Contains(desc, "tempest_pressure_pa"):
			obs.pressure = value
		case strings.Contains(desc, "tempest_temperature_c"):
			// Check kind label
			for _, label := range dto.GetLabel() {
				if label.GetName() == "kind" {
					switch label.GetValue() {
					case "air":
						obs.tempAir = value
					case "wetbulb":
						obs.tempWetbulb = value
					}
					break
				}
			}
		case strings.Contains(desc, "tempest_humidity_percent"):
			obs.humidity = value
		case strings.Contains(desc, "tempest_illuminance_lux"):
			obs.illuminance = value
		case strings.Contains(desc, "tempest_uv_index"):
			obs.uvIndex = value
		case strings.Contains(desc, "tempest_irradiance_w_m2"):
			obs.irradiance = value
		case strings.Contains(desc, "tempest_rain_rate_mm_min"):
			obs.rainRate = value
		case strings.Contains(desc, "tempest_lightning_distance_km"):
			obs.lightningDistance = &value
		case strings.Contains(desc, "tempest_lightning_strike_count"):
			obs.lightningStrikeCount = &value
		case strings.Contains(desc, "tempest_battery_volts"):
			obs.battery = &value
		case strings.Contains(desc, "tempest_report_interval_s"):
			obs.reportInterval = &value
		}
	}

	// Send all observations to the batch channel
	for _, obs := range observations {
		select {
		case w.obsBatch <- *obs:
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Printf("postgres: observation batch channel full during WriteMetrics, dropping")
		}
	}

	return nil
}

// Flush implements MetricsWriter interface
func (w *PostgresWriter) Flush(ctx context.Context) error {
	// Flush is handled by the batch workers
	// When Close() is called, channels are closed and workers flush remaining data
	return nil
}

// flushWithRetry implements exponential backoff retry logic
func (w *PostgresWriter) flushWithRetry(flushFn func() error, tableName string, batchSize int) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for attempt := 1; attempt <= w.maxRetries; attempt++ {
		err := flushFn()
		if err == nil {
			return
		}

		log.Printf("postgres: failed to write %d rows to %s (attempt %d/%d): %v",
			batchSize, tableName, attempt, w.maxRetries, err)

		if !isRetryable(err) {
			log.Printf("postgres: non-retryable error for %s, dropping batch: %v",
				tableName, err)
			return
		}

		if attempt == w.maxRetries {
			log.Printf("postgres: max retries exceeded for %s, dropping %d rows", tableName, batchSize)
			return
		}

		// Check if context is still valid before sleeping
		select {
		case <-w.ctx.Done():
			log.Printf("postgres: context cancelled during retry for %s, dropping batch", tableName)
			return
		case <-time.After(backoff):
			// Continue to next attempt
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for context errors (not retryable - parent cancelled)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Check for pgconn errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		// Retryable: connection failures (Class 08)
		case "08000", "08003", "08006":
			return true
		// Retryable: deadlock (Class 40)
		case "40001", "40P01":
			return true
		// Not retryable: constraint violations (Class 23)
		case "23505", "23503", "23502":
			return false
		// Not retryable: undefined objects (Class 42)
		case "42P01", "42703":
			return false
		default:
			return true
		}
	}

	// Network errors
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable") {
		return true
	}

	return true
}

// Close implements MetricsWriter interface
func (w *PostgresWriter) Close() error {
	// Close channels to stop workers
	close(w.obsBatch)
	close(w.windBatch)
	close(w.hubBatch)
	close(w.eventBatch)

	// Wait for workers to finish
	w.wg.Wait()

	// Close connection pool
	w.pool.Close()
	log.Printf("postgres: closed")

	return nil
}
