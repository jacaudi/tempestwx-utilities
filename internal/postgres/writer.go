package postgres

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"tempestwx-exporter/internal/tempestudp"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// Row types for each table
type observationRow struct {
	serialNumber   string
	timestamp      time.Time
	windLull       float64
	windAvg        float64
	windGust       float64
	windDirection  float64
	pressure       float64
	tempAir        float64
	tempWetbulb    float64
	humidity       float64
	illuminance    float64
	uvIndex        float64
	irradiance     float64
	rainRate       float64
	battery        *float64
	reportInterval *float64
}

type rapidWindRow struct {
	serialNumber  string
	timestamp     time.Time
	windSpeed     float64
	windDirection float64
}

type hubStatusRow struct {
	serialNumber string
	timestamp    time.Time
	uptime       float64
	rssi         float64
	rebootCount  float64
	busErrors    float64
}

type eventRow struct {
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
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	for _, row := range batch {
		_, err := w.pool.Exec(ctx, `
			INSERT INTO tempest_observations (
				serial_number, timestamp, wind_lull, wind_avg, wind_gust,
				wind_direction, pressure, temp_air, temp_wetbulb, humidity,
				illuminance, uv_index, irradiance, rain_rate, battery, report_interval
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
			ON CONFLICT (serial_number, timestamp) DO NOTHING
		`, row.serialNumber, row.timestamp, row.windLull, row.windAvg, row.windGust,
			row.windDirection, row.pressure, row.tempAir, row.tempWetbulb, row.humidity,
			row.illuminance, row.uvIndex, row.irradiance, row.rainRate, row.battery, row.reportInterval)

		if err != nil {
			return fmt.Errorf("insert observation: %w", err)
		}
	}

	return nil
}

func (w *PostgresWriter) batchRapidWind() {
	defer w.wg.Done()
	// TODO: implement
}

func (w *PostgresWriter) batchHubStatus() {
	defer w.wg.Done()
	// TODO: implement
}

func (w *PostgresWriter) batchEvents() {
	defer w.wg.Done()
	// TODO: implement
}

// WriteReport implements MetricsWriter interface
func (w *PostgresWriter) WriteReport(ctx context.Context, report tempestudp.Report) error {
	switch r := report.(type) {
	case *tempestudp.TempestObservationReport:
		return w.handleObservationReport(ctx, r)

	// TODO: other report types in next tasks
	default:
		// Unknown report type - not an error
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
			serialNumber:  r.SerialNumber,
			timestamp:     ts,
			windLull:      ob[1],
			windAvg:       ob[2],
			windGust:      ob[3],
			windDirection: ob[4],
			pressure:      ob[6] * 100, // MB to Pascals
			tempAir:       ob[7],
			tempWetbulb:   wetBulb,
			humidity:      ob[8],
			illuminance:   ob[9],
			uvIndex:       ob[10],
			irradiance:    ob[11],
			rainRate:      ob[12],
		}

		// Optional fields
		if len(ob) >= 17 {
			battery := ob[16]
			row.battery = &battery
		}
		if len(ob) >= 18 {
			interval := ob[17] * 60 // minutes to seconds
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

// WriteMetrics implements MetricsWriter interface
func (w *PostgresWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	// TODO: implement later
	return nil
}

// Flush implements MetricsWriter interface
func (w *PostgresWriter) Flush(ctx context.Context) error {
	// TODO: implement
	return nil
}

// flushWithRetry implements exponential backoff retry logic
func (w *PostgresWriter) flushWithRetry(flushFn func() error, tableName string, batchSize int) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for attempt := 1; attempt <= w.maxRetries; attempt++ {
		err := flushFn()
		if err == nil {
			return // Success
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

		time.Sleep(backoff)
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

	errStr := err.Error()

	// Retryable: connection, timeout, deadlock
	if contains(errStr, "connection") ||
		contains(errStr, "timeout") ||
		contains(errStr, "deadlock") {
		return true
	}

	// Not retryable: constraint violations, schema errors
	if contains(errStr, "duplicate key") ||
		contains(errStr, "does not exist") ||
		contains(errStr, "constraint") {
		return false
	}

	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) && containsSlow(s, substr))
}

func containsSlow(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
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
