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

// Placeholder batch methods - will implement in next tasks
func (w *PostgresWriter) batchObservations() {
	defer w.wg.Done()
	// TODO: implement
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
	// TODO: implement in next task
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
