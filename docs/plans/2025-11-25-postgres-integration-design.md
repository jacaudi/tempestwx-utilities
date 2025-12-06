# PostgreSQL Integration Design

**Date:** 2025-11-25
**Status:** Design Approved
**Author:** Claude Code with jacaudi

## Overview

Add PostgreSQL storage capability to complement the existing Prometheus push gateway integration. This enables long-term historical analysis, structured queries, and data persistence while maintaining real-time operational monitoring in Prometheus.

## Requirements

### Goals
- **Complement Prometheus**: Dual-write to both systems when configured
- **Historical backfill**: Support API export mode for loading historical data
- **Type-safe storage**: Use typed tables matching Tempest message structure
- **Production-ready**: Connection pooling, retry logic, graceful degradation

### Non-Goals
- Replace Prometheus entirely
- Real-time alerting (Prometheus handles this)
- Schema migrations (auto-create on startup only)

## Data Volume Analysis

**Tempest broadcast frequencies:**
- `obs_st` (main observations): ~1/minute, 13+ metrics per broadcast
- `rapid_wind`: ~every 3 seconds, 2 metrics
- `hub_status`: periodic, 4 metrics
- `evt_precip`, `evt_strike`: event-driven (sporadic)

**Estimated volume:** ~50-100 data points/minute/station, ~75K inserts/day/station

**Conclusion:** Easily manageable by PostgreSQL with proper indexing and batching.

## Architecture

### High-Level Design

```
UDP Broadcast → ParseReport() → Report (typed structs)
                                   ↓
                         MetricsSink (central coordinator)
                              ↓           ↓
                    PrometheusWriter   PostgresWriter
                         ↓                  ↓
                  Push Gateway         PostgreSQL


API Response → GetObservations() → []prometheus.Metric
                                        ↓
                              MetricsSink (same!)
                                   ↓         ↓
                          PrometheusWriter  PostgresWriter
```

### Core Components

#### 1. MetricsSink

Central coordinator that routes metrics to multiple backends.

```go
type MetricsSink struct {
    writers []MetricsWriter
    ctx     context.Context
    wg      sync.WaitGroup
}

type MetricsWriter interface {
    WriteReport(ctx context.Context, report tempestudp.Report) error
    WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error
    Flush(ctx context.Context) error
    Close() error
}
```

**Responsibilities:**
- Accept metrics from both operational modes (UDP listener and API export)
- Fan out to all configured writers concurrently
- Handle graceful shutdown with flush coordination
- Log errors per writer but continue processing

**Writer registration:**
```go
sink := NewMetricsSink(ctx)

if pushUrl := os.Getenv("PUSH_URL"); pushUrl != "" {
    sink.AddWriter(NewPrometheusWriter(pushUrl, jobName))
}

if dbConfig, err := getDatabaseConfig(); dbConfig != "" {
    pgWriter, err := NewPostgresWriter(ctx, dbConfig)
    sink.AddWriter(pgWriter)
}
```

#### 2. PrometheusWriter

Encapsulates existing push logic, no functional changes.

```go
func (w *PrometheusWriter) WriteReport(ctx context.Context, report tempestudp.Report) error {
    metrics := report.Metrics()
    return w.WriteMetrics(ctx, metrics)
}

func (w *PrometheusWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
    for _, m := range metrics {
        w.outbox <- m
    }
    return nil
}
```

#### 3. PostgresWriter

New backend with batching, retry logic, and connection pooling.

```go
type PostgresWriter struct {
    pool          *pgxpool.Pool

    // Separate batch channels per table type
    obsBatch      chan observationRow
    windBatch     chan rapidWindRow
    hubBatch      chan hubStatusRow
    eventBatch    chan eventRow

    // Configuration
    batchSize     int           // default: 100
    flushInterval time.Duration // default: 10s
    maxRetries    int           // default: 3

    ctx           context.Context
    wg            sync.WaitGroup
}
```

### Hybrid Input Pattern

**Key design decision:** Support both `Report` objects (UDP mode) and `prometheus.Metric` arrays (API mode).

**Rationale:**
- UDP mode already has typed `Report` structs from `ParseReport()`
- API mode already produces `[]prometheus.Metric` from `GetObservations()`
- Forcing both through single abstraction adds complexity without benefit

**PostgresWriter implementation:**

```go
func (w *PostgresWriter) WriteReport(ctx context.Context, report tempestudp.Report) error {
    switch r := report.(type) {
    case *tempestudp.TempestObservationReport:
        // Direct struct-to-table mapping - type-safe!
        for _, ob := range r.Obs {
            w.obsBatch <- observationRow{
                serialNumber: r.SerialNumber,
                timestamp:    time.Unix(int64(ob[0]), 0),
                windLull:     ob[1],
                windAvg:      ob[2],
                // ... direct field access
            }
        }
    case *tempestudp.rapidWindReport:
        w.windBatch <- rapidWindRow{...}
    // ... other report types
    }
    return nil
}

func (w *PostgresWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
    // Parse prometheus.Metric descriptors and route to tables
    // More complex but only used for historical backfill
    // ...
}
```

**Benefits:**
- Simple, type-safe real-time path (Report → Postgres)
- No metric descriptor parsing in hot path
- API export mode still works for both writers
- Each writer handles natural data format

## Database Schema

### Typed Table Design

Use separate tables per message type for optimal query performance and type safety.

```sql
-- Main weather observations (~1/minute)
CREATE TABLE IF NOT EXISTS tempest_observations (
    id            BIGSERIAL PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,

    -- Wind measurements
    wind_lull     DOUBLE PRECISION,  -- m/s
    wind_avg      DOUBLE PRECISION,  -- m/s
    wind_gust     DOUBLE PRECISION,  -- m/s
    wind_direction DOUBLE PRECISION, -- degrees

    -- Atmospheric
    pressure      DOUBLE PRECISION,  -- pascals
    temp_air      DOUBLE PRECISION,  -- celsius
    temp_wetbulb  DOUBLE PRECISION,  -- celsius (calculated)
    humidity      DOUBLE PRECISION,  -- percent

    -- Solar
    illuminance   DOUBLE PRECISION,  -- lux
    uv_index      DOUBLE PRECISION,
    irradiance    DOUBLE PRECISION,  -- w/m²

    -- Precipitation
    rain_rate     DOUBLE PRECISION,  -- mm/min

    -- Device
    battery       DOUBLE PRECISION,  -- volts
    report_interval DOUBLE PRECISION, -- seconds

    UNIQUE(serial_number, timestamp)
);

-- Rapid wind readings (~3 seconds)
CREATE TABLE IF NOT EXISTS tempest_rapid_wind (
    id            BIGSERIAL PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    wind_speed    DOUBLE PRECISION,  -- m/s
    wind_direction DOUBLE PRECISION, -- degrees

    UNIQUE(serial_number, timestamp)
);

-- Hub status (periodic)
CREATE TABLE IF NOT EXISTS tempest_hub_status (
    id            BIGSERIAL PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    uptime        DOUBLE PRECISION,  -- seconds
    rssi          DOUBLE PRECISION,  -- dbm
    reboot_count  DOUBLE PRECISION,
    bus_errors    DOUBLE PRECISION,

    UNIQUE(serial_number, timestamp)
);

-- Events (rain start, lightning strikes)
CREATE TABLE IF NOT EXISTS tempest_events (
    id            BIGSERIAL PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    event_type    TEXT NOT NULL,     -- 'rain_start', 'lightning_strike'
    distance_km   DOUBLE PRECISION,  -- for lightning
    energy        DOUBLE PRECISION,  -- for lightning

    UNIQUE(serial_number, timestamp, event_type)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_obs_time ON tempest_observations(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_obs_serial_time ON tempest_observations(serial_number, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_wind_time ON tempest_rapid_wind(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_wind_serial_time ON tempest_rapid_wind(serial_number, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_hub_time ON tempest_hub_status(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_events_time ON tempest_events(timestamp DESC);
```

### Schema Rationale

**Why typed tables vs. generic metric table?**

1. **Query performance**: Most queries want "all conditions at time X" (one row) or "temperature over time" (indexed column)
2. **Type safety**: Each metric has specific units and meaning
3. **Storage efficiency**: No NULL columns for unrelated metrics
4. **Natural fit**: Matches Tempest message structure
5. **Indexing flexibility**: Optimize per table's query patterns

**Why UNIQUE constraints?**
- Handle retries gracefully (idempotent inserts)
- Prevent duplicate data from UDP broadcast overlaps
- `ON CONFLICT DO NOTHING` for upsert behavior

## Batching Strategy

### Per-Table Batching Rules

| Message Type | Frequency | Strategy | Rationale |
|--------------|-----------|----------|-----------|
| `obs_st` | ~1/minute | **Immediate flush** | Important snapshots, low frequency |
| `events` | Sporadic | **Immediate flush** | Critical events, need visibility |
| `rapid_wind` | ~20/minute | Batch (100 or 10s) | High frequency, batching reduces load |
| `hub_status` | Periodic | Batch (100 or 10s) | Operational data, less time-sensitive |

### Implementation

Each table has a dedicated background goroutine:

```go
// Observations - immediate flush
func (w *PostgresWriter) batchObservations() {
    for {
        select {
        case row := <-w.obsBatch:
            w.flushObservations([]observationRow{row})
        case <-w.ctx.Done():
            return
        }
    }
}

// Rapid wind - batched for efficiency
func (w *PostgresWriter) batchRapidWind() {
    batch := make([]rapidWindRow, 0, w.batchSize)
    ticker := time.NewTicker(w.flushInterval)

    for {
        select {
        case row := <-w.windBatch:
            batch = append(batch, row)
            if len(batch) >= w.batchSize {
                w.flushRapidWind(batch)
                batch = batch[:0]
            }

        case <-ticker.C:
            if len(batch) > 0 {
                w.flushRapidWind(batch)
                batch = batch[:0]
            }

        case <-w.ctx.Done():
            // Final flush on shutdown
            if len(batch) > 0 {
                w.flushRapidWind(batch)
            }
            return
        }
    }
}
```

### Batch Insert with pgx

```go
func (w *PostgresWriter) flushObservations(batch []observationRow) {
    w.flushWithRetry(func() error {
        b := &pgx.Batch{}

        for _, row := range batch {
            b.Queue(`
                INSERT INTO tempest_observations (
                    serial_number, timestamp, wind_lull, wind_avg, wind_gust,
                    wind_direction, pressure, temp_air, temp_wetbulb, humidity,
                    illuminance, uv_index, irradiance, rain_rate, battery, report_interval
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
                ON CONFLICT (serial_number, timestamp) DO NOTHING
            `, row.serialNumber, row.timestamp, row.windLull, /* ... */)
        }

        br := w.pool.SendBatch(w.ctx, b)
        defer br.Close()

        for i := 0; i < len(batch); i++ {
            _, err := br.Exec()
            if err != nil {
                return fmt.Errorf("batch exec failed at row %d: %w", i, err)
            }
        }

        return nil
    }, "tempest_observations")
}
```

## Error Handling and Retry Logic

### Retry with Exponential Backoff

```go
func (w *PostgresWriter) flushWithRetry(flushFn func() error, tableName string) {
    backoff := time.Second
    maxBackoff := 30 * time.Second

    for attempt := 1; attempt <= w.maxRetries; attempt++ {
        err := flushFn()
        if err == nil {
            return // Success
        }

        log.Printf("postgres: failed to write to %s (attempt %d/%d): %v",
            tableName, attempt, w.maxRetries, err)

        // Check if error is retryable
        if !isRetryable(err) {
            log.Printf("postgres: non-retryable error for %s, dropping batch: %v",
                tableName, err)
            return
        }

        // Last attempt - give up
        if attempt == w.maxRetries {
            log.Printf("postgres: max retries exceeded for %s, dropping batch", tableName)
            return
        }

        // Exponential backoff
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

    // Retryable: network, connection, timeout, deadlocks
    if strings.Contains(errStr, "connection") ||
       strings.Contains(errStr, "timeout") ||
       strings.Contains(errStr, "deadlock") {
        return true
    }

    // Not retryable: constraint violations, schema errors
    if strings.Contains(errStr, "duplicate key") ||
       strings.Contains(errStr, "does not exist") ||
       strings.Contains(errStr, "constraint") {
        return false
    }

    return true // Default: retry
}
```

### Error Isolation

- Each writer fails independently
- Postgres errors don't block Prometheus push
- Non-retryable errors log and drop batch (don't crash)
- Transient errors retry with backoff

## Connection Pooling

Using `pgxpool` for production-grade connection management:

```go
func NewPostgresWriter(ctx context.Context, databaseURL string) (*PostgresWriter, error) {
    config, err := pgxpool.ParseConfig(databaseURL)
    if err != nil {
        return nil, fmt.Errorf("parse database url: %w", err)
    }

    // Connection pool configuration
    config.MaxConns = 10                        // Max connections
    config.MinConns = 2                         // Keep-alive connections
    config.MaxConnLifetime = time.Hour          // Rotate connections
    config.MaxConnIdleTime = 10 * time.Minute   // Close idle
    config.HealthCheckPeriod = 30 * time.Second // Health checks

    pool, err := pgxpool.NewWithConfig(ctx, config)
    if err != nil {
        return nil, fmt.Errorf("create connection pool: %w", err)
    }

    // Verify connectivity
    if err := pool.Ping(ctx); err != nil {
        pool.Close()
        return nil, fmt.Errorf("ping database: %w", err)
    }

    // Auto-create schema
    if err := createSchema(ctx, pool); err != nil {
        pool.Close()
        return nil, fmt.Errorf("create schema: %w", err)
    }

    // Initialize writer with batch channels
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
```

## Configuration

### Environment Variables

Support both full connection string and individual components:

```bash
# Option 1: Full connection string (simple, good for local dev)
DATABASE_URL=postgresql://user:pass@localhost:5432/weather

# Option 2: Individual components (better for production/secrets)
DATABASE_HOST=postgres
DATABASE_PORT=5432              # optional, default: 5432
DATABASE_USERNAME=tempest
DATABASE_PASSWORD=secret
DATABASE_NAME=weather
DATABASE_SSLMODE=disable        # optional: disable, require, verify-ca, verify-full

# Batching configuration (optional)
POSTGRES_BATCH_SIZE=100         # default: 100
POSTGRES_FLUSH_INTERVAL=10s     # default: 10s
POSTGRES_MAX_RETRIES=3          # default: 3

# API export mode (optional)
KEEP_EXPORT_FILES=true          # Keep .gz files in addition to Postgres
```

### Configuration Precedence

```go
func getDatabaseConfig() (string, error) {
    // Option 1: Full connection string (takes precedence)
    if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
        return dbURL, nil
    }

    // Option 2: Build from components
    host := os.Getenv("DATABASE_HOST")
    if host == "" {
        return "", nil // No database configured
    }

    port := os.Getenv("DATABASE_PORT")
    if port == "" {
        port = "5432"
    }

    username := os.Getenv("DATABASE_USERNAME")
    if username == "" {
        return "", fmt.Errorf("DATABASE_USERNAME required when using DATABASE_HOST")
    }

    password := os.Getenv("DATABASE_PASSWORD")
    if password == "" {
        return "", fmt.Errorf("DATABASE_PASSWORD required when using DATABASE_HOST")
    }

    dbname := os.Getenv("DATABASE_NAME")
    if dbname == "" {
        return "", fmt.Errorf("DATABASE_NAME required when using DATABASE_HOST")
    }

    sslmode := os.Getenv("DATABASE_SSLMODE")
    if sslmode == "" {
        sslmode = "disable"
    }

    return fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
        username, password, host, port, dbname, sslmode), nil
}
```

### Operational Modes Matrix

| Mode | PUSH_URL | DATABASE_URL/HOST | Behavior |
|------|----------|-------------------|----------|
| UDP | ✓ | ✗ | Prometheus only (current) |
| UDP | ✓ | ✓ | Both Prometheus + Postgres |
| UDP | ✗ | ✓ | Postgres only |
| API | N/A | ✗ | .gz files only (current) |
| API | N/A | ✓ | Postgres + optional .gz files |

### Startup Logic

```go
func main() {
    ctx, done := signal.NotifyContext(context.Background(), os.Interrupt)
    defer done()

    // Initialize metrics sink
    sink := NewMetricsSink(ctx)

    // Configure Prometheus writer
    if pushUrl := os.Getenv("PUSH_URL"); pushUrl != "" {
        jobName := os.Getenv("JOB_NAME")
        if jobName == "" {
            jobName = "tempest"
        }
        log.Printf("configuring prometheus push to %q", pushUrl)
        sink.AddWriter(NewPrometheusWriter(pushUrl, jobName))
    }

    // Configure Postgres writer
    if dbConfig, err := getDatabaseConfig(); err != nil {
        log.Fatalf("database configuration error: %v", err)
    } else if dbConfig != "" {
        log.Printf("configuring postgres writer")
        pgWriter, err := NewPostgresWriter(ctx, dbConfig)
        if err != nil {
            log.Fatalf("failed to initialize postgres: %v", err)
        }
        sink.AddWriter(pgWriter)
    }

    // Require at least one writer
    if sink.WriterCount() == 0 {
        log.Fatal("no writers configured - set PUSH_URL and/or DATABASE_HOST/DATABASE_URL")
    }

    // Choose operational mode
    token := os.Getenv("TOKEN")
    if token != "" {
        exportWithSink(ctx, token, sink)
    } else {
        listenAndPushWithSink(ctx, sink)
    }
}
```

## Deployment

### Docker Compose Example

```yaml
version: '3.8'

services:
  tempest-exporter:
    image: tempestwx-exporter:latest
    network_mode: host  # Required for UDP broadcasts
    environment:
      # Prometheus
      PUSH_URL: http://pushgateway:9091
      JOB_NAME: tempest

      # Postgres - using component approach for secrets
      DATABASE_HOST: postgres
      DATABASE_PORT: 5432
      DATABASE_USERNAME: tempest
      DATABASE_PASSWORD: ${DB_PASSWORD}  # From .env or secrets
      DATABASE_NAME: weather
      DATABASE_SSLMODE: require

      # Optional tuning
      POSTGRES_BATCH_SIZE: 100
      POSTGRES_FLUSH_INTERVAL: 10s

    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: weather
      POSTGRES_USER: tempest
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U tempest"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  pgdata:
```

### Kubernetes Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tempest-exporter
spec:
  template:
    spec:
      hostNetwork: true  # Required for UDP broadcasts
      containers:
      - name: exporter
        image: tempestwx-exporter:latest
        env:
        - name: PUSH_URL
          value: "http://prometheus-pushgateway:9091"
        - name: DATABASE_HOST
          value: "postgres-service"
        - name: DATABASE_USERNAME
          value: "tempest"
        - name: DATABASE_NAME
          value: "weather"
        - name: DATABASE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgres-credentials
              key: password
        - name: DATABASE_SSLMODE
          value: "require"
```

## Testing Strategy

### Unit Tests
- Schema creation logic
- Metric parsing and routing
- Retry logic with mock errors
- Batch flushing behavior

### Integration Tests
- Real Postgres container (testcontainers)
- Verify table creation
- Insert various message types
- Verify UNIQUE constraints work
- Test connection pool behavior
- Test graceful shutdown and flush

### Load Tests
- Simulate high-frequency rapid_wind messages
- Verify batching reduces DB load
- Monitor connection pool utilization
- Confirm no memory leaks in channels

## Future Enhancements

### Potential Additions
- Partitioning for large datasets (by timestamp)
- Materialized views for common aggregations
- TimescaleDB support for better time-series performance
- Data retention policies (delete old data)
- Metrics about Postgres writer health (inserts/sec, error rates)

### Out of Scope (Initially)
- Schema migrations (use manual SQL or tools like golang-migrate)
- Multiple database support (focus on Postgres)
- Real-time streaming to clients (use Prometheus for this)

## Dependencies

### New Go Modules
```
github.com/jackc/pgx/v5
github.com/jackc/pgx/v5/pgxpool
```

### Dockerfile Changes
```dockerfile
RUN go get github.com/jackc/pgx/v5 \
    github.com/jackc/pgx/v5/pgxpool
```

## Migration Path

### For Existing Deployments

1. **Add Postgres to infrastructure** (Docker Compose or K8s)
2. **Set DATABASE_* environment variables**
3. **Restart exporter** - tables auto-create on startup
4. **Optionally backfill historical data** using API export mode
5. **Verify both Prometheus and Postgres are receiving data**

### Rollback Plan

If issues occur:
1. **Remove DATABASE_* environment variables**
2. **Restart exporter** - falls back to Prometheus-only mode
3. **Postgres data remains intact** for later investigation

## Success Criteria

- ✅ Dual-write to Prometheus and Postgres when both configured
- ✅ API export mode can backfill historical data to Postgres
- ✅ No performance degradation in UDP listener mode
- ✅ Postgres failures don't crash exporter or block Prometheus
- ✅ Schema auto-creates on startup
- ✅ Connection pooling prevents resource exhaustion
- ✅ Batching reduces database load for high-frequency messages
- ✅ Immediate flush for critical events and observations

---

**End of Design Document**
