# Schema Enhancements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete v1.0.0 schema with UUIDv7 primary keys, missing fields (wind_sample_interval, precip_type), and raw UDP value storage (no unit conversions).

**Architecture:** Store raw UDP values without conversions, use Go-generated UUIDv7 for all primary keys (no PostgreSQL extensions), add complete field coverage from Tempest UDP API specification.

**Tech Stack:** Go 1.23+, PostgreSQL 9.4+, github.com/google/uuid

---

## Task 1: Add UUID Dependency

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add google/uuid dependency**

Run:
```bash
go get github.com/google/uuid@latest
```

Expected: Dependency added to go.mod and go.sum

**Step 2: Verify dependency**

Run:
```bash
go mod tidy
go list -m github.com/google/uuid
```

Expected: Shows version like `github.com/google/uuid v1.x.x`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add github.com/google/uuid for UUIDv7 generation"
```

---

## Task 2: Update Schema - Add Missing Fields

**Files:**
- Modify: `internal/postgres/schema.go:33-63`
- Read: `docs/plans/2025-12-06-schema-enhancements-design.md` (reference for field specs)

**Step 1: Add missing columns to observations table**

In `internal/postgres/schema.go`, update `createObservationsTable` constant:

```go
const createObservationsTable = `
CREATE TABLE IF NOT EXISTS tempest_observations (
    id            BIGSERIAL PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,

    wind_lull     DOUBLE PRECISION,
    wind_avg      DOUBLE PRECISION,
    wind_gust     DOUBLE PRECISION,
    wind_direction DOUBLE PRECISION,
    wind_sample_interval DOUBLE PRECISION,

    pressure      DOUBLE PRECISION,
    temp_air      DOUBLE PRECISION,
    temp_wetbulb  DOUBLE PRECISION,
    humidity      DOUBLE PRECISION,

    illuminance   DOUBLE PRECISION,
    uv_index      DOUBLE PRECISION,
    irradiance    DOUBLE PRECISION,

    rain_rate     DOUBLE PRECISION,
    precip_type   INTEGER,

    lightning_distance DOUBLE PRECISION,
    lightning_strike_count DOUBLE PRECISION,

    battery       DOUBLE PRECISION,
    report_interval DOUBLE PRECISION,

    UNIQUE(serial_number, timestamp)
);
`
```

**Step 2: Verify syntax**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 3: Commit**

```bash
git add internal/postgres/schema.go
git commit -m "schema: add wind_sample_interval and precip_type columns"
```

---

## Task 3: Update Schema - Change to UUID Primary Keys

**Files:**
- Modify: `internal/postgres/schema.go:33-102`

**Step 1: Change observations table to UUID**

In `internal/postgres/schema.go`, update all tables to use UUID:

```go
const createObservationsTable = `
CREATE TABLE IF NOT EXISTS tempest_observations (
    id            UUID PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    -- ... rest of fields unchanged ...
);
`

const createRapidWindTable = `
CREATE TABLE IF NOT EXISTS tempest_rapid_wind (
    id            UUID PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    wind_speed    DOUBLE PRECISION,
    wind_direction DOUBLE PRECISION,

    UNIQUE(serial_number, timestamp)
);
`

const createHubStatusTable = `
CREATE TABLE IF NOT EXISTS tempest_hub_status (
    id            UUID PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    uptime        DOUBLE PRECISION,
    rssi          DOUBLE PRECISION,
    reboot_count  DOUBLE PRECISION,
    bus_errors    DOUBLE PRECISION,

    UNIQUE(serial_number, timestamp)
);
`

const createEventsTable = `
CREATE TABLE IF NOT EXISTS tempest_events (
    id            UUID PRIMARY KEY,
    serial_number TEXT NOT NULL,
    timestamp     TIMESTAMPTZ NOT NULL,
    event_type    TEXT NOT NULL,
    distance_km   DOUBLE PRECISION,
    energy        DOUBLE PRECISION,

    UNIQUE(serial_number, timestamp, event_type)
);
`
```

**Step 2: Verify syntax**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 3: Commit**

```bash
git add internal/postgres/schema.go
git commit -m "schema: change primary keys from BIGSERIAL to UUID"
```

---

## Task 4: Update Row Structs - Add UUID and Missing Fields

**Files:**
- Modify: `internal/postgres/writer.go:21-62`

**Step 1: Add import for uuid**

At top of `internal/postgres/writer.go`, add to imports:

```go
import (
    "context"
    "errors"
    "fmt"
    "log"
    "strings"
    "sync"
    "time"

    "tempestwx-utilities/internal/tempestudp"

    "github.com/google/uuid"  // ADD THIS
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/prometheus/client_golang/prometheus"
    io_prometheus_client "github.com/prometheus/client_model/go"
)
```

**Step 2: Update observationRow struct**

```go
type observationRow struct {
    id                   uuid.UUID  // CHANGED from implicit
    serialNumber         string
    timestamp            time.Time
    windLull             float64
    windAvg              float64
    windGust             float64
    windDirection        float64
    windSampleInterval   *float64   // NEW
    pressure             float64
    tempAir              float64
    tempWetbulb          float64
    humidity             float64
    illuminance          float64
    uvIndex              float64
    irradiance           float64
    rainRate             float64
    precipType           *int       // NEW
    lightningDistance    *float64
    lightningStrikeCount *float64
    battery              *float64
    reportInterval       *float64
}
```

**Step 3: Update rapidWindRow struct**

```go
type rapidWindRow struct {
    id            uuid.UUID  // CHANGED from implicit
    serialNumber  string
    timestamp     time.Time
    windSpeed     float64
    windDirection float64
}
```

**Step 4: Update hubStatusRow struct**

```go
type hubStatusRow struct {
    id           uuid.UUID  // CHANGED from implicit
    serialNumber string
    timestamp    time.Time
    uptime       float64
    rssi         float64
    rebootCount  float64
    busErrors    float64
}
```

**Step 5: Update eventRow struct**

```go
type eventRow struct {
    id           uuid.UUID  // CHANGED from implicit
    serialNumber string
    timestamp    time.Time
    eventType    string
    distanceKm   *float64
    energy       *float64
}
```

**Step 6: Verify builds**

Run:
```bash
go build .
```

Expected: Builds successfully (INSERT statements will fail at runtime until updated)

**Step 7: Commit**

```bash
git add internal/postgres/writer.go
git commit -m "writer: add UUID field and missing fields to row structs"
```

---

## Task 5: Update Observation Handler - Generate UUID and Extract Fields

**Files:**
- Modify: `internal/postgres/writer.go:437-487`

**Step 1: Update handleObservationReport to generate UUID and extract new fields**

Replace the handler function:

```go
func (w *PostgresWriter) handleObservationReport(ctx context.Context, r *tempestudp.TempestObservationReport) error {
    for _, ob := range r.Obs {
        if len(ob) < 13 {
            continue
        }

        ts := time.Unix(int64(ob[0]), 0)

        // Calculate wet bulb temperature (from tempestudp package)
        wetBulb := tempestudp.WetBulbTemperatureC(ob[7], ob[8], ob[6])

        row := observationRow{
            id:           uuid.Must(uuid.NewV7()),  // Generate UUIDv7
            serialNumber: r.SerialNumber,
            timestamp:    ts,
            windLull:     ob[1],
            windAvg:      ob[2],
            windGust:     ob[3],
            windDirection: ob[4],
            pressure:     ob[6],      // Raw mb value (no conversion)
            tempAir:      ob[7],
            tempWetbulb:  wetBulb,
            humidity:     ob[8],
            illuminance:  ob[9],
            uvIndex:      ob[10],
            irradiance:   ob[11],
            rainRate:     ob[12],
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
            interval := ob[17]  // Raw minutes value (no conversion)
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
```

**Step 2: Verify builds**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 3: Commit**

```bash
git add internal/postgres/writer.go
git commit -m "writer: generate UUIDs and extract all observation fields

- Generate UUIDv7 for each observation row
- Extract wind_sample_interval (field 5)
- Extract precip_type (field 13)
- Store raw pressure value (no mb→Pa conversion)
- Store raw report_interval value (no min→sec conversion)"
```

---

## Task 6: Update Other Report Handlers - Generate UUIDs

**Files:**
- Modify: `internal/postgres/writer.go:488-596`

**Step 1: Update handleRapidWindReport**

```go
func (w *PostgresWriter) handleRapidWindReport(ctx context.Context, r *tempestudp.RapidWindReport) error {
    if len(r.Ob) != 3 {
        return nil // Invalid data
    }

    ts := time.Unix(int64(r.Ob[0]), 0)

    row := rapidWindRow{
        id:            uuid.Must(uuid.NewV7()),  // Generate UUIDv7
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
```

**Step 2: Update handleHubStatusReport**

```go
func (w *PostgresWriter) handleHubStatusReport(ctx context.Context, r *tempestudp.HubStatusReport) error {
    if len(r.RadioStats) < 3 {
        return nil // Invalid data
    }

    ts := time.Unix(r.Timestamp, 0)

    row := hubStatusRow{
        id:           uuid.Must(uuid.NewV7()),  // Generate UUIDv7
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
```

**Step 3: Update handleRainStartReport**

```go
func (w *PostgresWriter) handleRainStartReport(ctx context.Context, r *tempestudp.RainStartReport) error {
    if len(r.Evt) < 1 {
        return nil // Invalid data
    }

    ts := time.Unix(int64(r.Evt[0]), 0)

    row := eventRow{
        id:           uuid.Must(uuid.NewV7()),  // Generate UUIDv7
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
```

**Step 4: Update handleLightningStrikeReport**

```go
func (w *PostgresWriter) handleLightningStrikeReport(ctx context.Context, r *tempestudp.LightningStrikeReport) error {
    if len(r.Evt) < 3 {
        return nil // Invalid data
    }

    ts := time.Unix(int64(r.Evt[0]), 0)
    distance := r.Evt[1]
    energy := r.Evt[2]

    row := eventRow{
        id:           uuid.Must(uuid.NewV7()),  // Generate UUIDv7
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
```

**Step 5: Verify builds**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 6: Commit**

```bash
git add internal/postgres/writer.go
git commit -m "writer: generate UUIDs in all report handlers

- RapidWindReport: UUIDv7 generation
- HubStatusReport: UUIDv7 generation
- RainStartReport: UUIDv7 generation
- LightningStrikeReport: UUIDv7 generation"
```

---

## Task 7: Update INSERT Statements - Include UUID and New Fields

**Files:**
- Modify: `internal/postgres/writer.go:164-198`

**Step 1: Update observations INSERT statement**

```go
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
                id, serial_number, timestamp,
                wind_lull, wind_avg, wind_gust, wind_direction, wind_sample_interval,
                pressure, temp_air, temp_wetbulb, humidity,
                illuminance, uv_index, irradiance, rain_rate, precip_type,
                lightning_distance, lightning_strike_count,
                battery, report_interval
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
            ON CONFLICT (serial_number, timestamp) DO NOTHING
        `, row.id, row.serialNumber, row.timestamp,
            row.windLull, row.windAvg, row.windGust, row.windDirection, row.windSampleInterval,
            row.pressure, row.tempAir, row.tempWetbulb, row.humidity,
            row.illuminance, row.uvIndex, row.irradiance, row.rainRate, row.precipType,
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
```

**Step 2: Update rapid_wind INSERT statement**

Find `insertRapidWind` around line 249:

```go
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
                id, serial_number, timestamp, wind_speed, wind_direction
            ) VALUES ($1, $2, $3, $4, $5)
            ON CONFLICT (serial_number, timestamp) DO NOTHING
        `, row.id, row.serialNumber, row.timestamp, row.windSpeed, row.windDirection)
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
```

**Step 3: Update hub_status INSERT statement**

Find `insertHubStatus` around line 325:

```go
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
                id, serial_number, timestamp, uptime, rssi, reboot_count, bus_errors
            ) VALUES ($1, $2, $3, $4, $5, $6, $7)
            ON CONFLICT (serial_number, timestamp) DO NOTHING
        `, row.id, row.serialNumber, row.timestamp, row.uptime, row.rssi, row.rebootCount, row.busErrors)
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
```

**Step 4: Update events INSERT statement**

Find `insertEvents` around line 381:

```go
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
                id, serial_number, timestamp, event_type, distance_km, energy
            ) VALUES ($1, $2, $3, $4, $5, $6)
            ON CONFLICT (serial_number, timestamp, event_type) DO NOTHING
        `, row.id, row.serialNumber, row.timestamp, row.eventType, row.distanceKm, row.energy)
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
```

**Step 5: Verify builds**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 6: Commit**

```bash
git add internal/postgres/writer.go
git commit -m "writer: update INSERT statements for UUID and new fields

- Include id (UUID) in all INSERT statements
- Add wind_sample_interval and precip_type columns
- All tables now write UUID primary keys"
```

---

## Task 8: Remove Unit Conversions - Prometheus Metrics

**Files:**
- Modify: `internal/tempestudp/report.go:139-179`

**Step 1: Remove pressure and interval conversions from Prometheus metrics**

In `TempestObservationReport.Metrics()` method:

```go
func (r TempestObservationReport) Metrics() []prometheus.Metric {
    var out []prometheus.Metric

    for _, ob := range r.Obs {
        if len(ob) < 13 {
            continue
        }

        wetBulb := WetBulbTemperatureC(ob[7], ob[8], ob[6])

        metrics := []prometheus.Metric{
            prometheus.MustNewConstMetric(tempest.Wind, prometheus.GaugeValue, ob[1], r.SerialNumber, "lull"),
            prometheus.MustNewConstMetric(tempest.Wind, prometheus.GaugeValue, ob[2], r.SerialNumber, "avg"),
            prometheus.MustNewConstMetric(tempest.Wind, prometheus.GaugeValue, ob[3], r.SerialNumber, "gust"),
            prometheus.MustNewConstMetric(tempest.WindDirection, prometheus.GaugeValue, ob[4], r.SerialNumber),
            prometheus.MustNewConstMetric(tempest.Pressure, prometheus.GaugeValue, ob[6], r.SerialNumber),  // CHANGED: removed *100
            prometheus.MustNewConstMetric(tempest.Temperature, prometheus.GaugeValue, ob[7], r.SerialNumber, "air"),
            prometheus.MustNewConstMetric(tempest.Temperature, prometheus.GaugeValue, wetBulb, r.SerialNumber, "wetbulb"),
            prometheus.MustNewConstMetric(tempest.Humidity, prometheus.GaugeValue, ob[8], r.SerialNumber),
            prometheus.MustNewConstMetric(tempest.Illuminance, prometheus.GaugeValue, ob[9], r.SerialNumber),
            prometheus.MustNewConstMetric(tempest.UV, prometheus.GaugeValue, ob[10], r.SerialNumber),
            prometheus.MustNewConstMetric(tempest.Irradiance, prometheus.GaugeValue, ob[11], r.SerialNumber),
            prometheus.MustNewConstMetric(tempest.RainRate, prometheus.GaugeValue, ob[12], r.SerialNumber),
        }

        // Lightning metrics (fields 14 and 15)
        if len(ob) >= 16 {
            metrics = append(metrics,
                prometheus.MustNewConstMetric(tempest.LightningDistance, prometheus.GaugeValue, ob[14], r.SerialNumber),
                prometheus.MustNewConstMetric(tempest.LightningStrikeCount, prometheus.GaugeValue, ob[15], r.SerialNumber),
            )
        }

        if len(ob) >= 17 {
            metrics = append(metrics,
                prometheus.MustNewConstMetric(tempest.Battery, prometheus.GaugeValue, ob[16], r.SerialNumber),
            )
        }
        if len(ob) >= 18 {
            metrics = append(metrics,
                prometheus.MustNewConstMetric(tempest.ReportInterval, prometheus.GaugeValue, ob[17], r.SerialNumber),  // CHANGED: removed *60
            )
        }

        out = append(out, withTime(int64(ob[0]), metrics)...)
    }

    return out
}
```

**Step 2: Verify builds**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 3: Commit**

```bash
git add internal/tempestudp/report.go
git commit -m "udp: remove unit conversions from Prometheus metrics

BREAKING CHANGE: Store raw UDP values

- Pressure: mb (was Pascals, removed ×100 conversion)
- Report interval: minutes (was seconds, removed ×60 conversion)"
```

---

## Task 9: Update Metric Descriptions for Raw Units

**Files:**
- Modify: `internal/tempest/metrics.go:44-46`

**Step 1: Update metric descriptions to reflect raw units**

```go
Pressure = prometheus.NewDesc("tempest_pressure_mb", "Station pressure in millibars", []string{"instance"}, nil)  // CHANGED: mb not pa
// ... other metrics ...
ReportInterval = prometheus.NewDesc("tempest_report_interval_minutes", "Report interval in minutes", []string{"instance"}, nil)  // CHANGED: minutes not s
```

**Step 2: Verify builds**

Run:
```bash
go build .
```

Expected: Builds successfully

**Step 3: Commit**

```bash
git add internal/tempest/metrics.go
git commit -m "metrics: update descriptions for raw units

- Pressure metric now describes millibars (not Pascals)
- Report interval metric now describes minutes (not seconds)"
```

---

## Task 10: Update Tests for Raw Values

**Files:**
- Modify: `internal/tempestudp/report_test.go:36-107`

**Step 1: Update test expectations for raw pressure and interval**

In `Test_tempestObservationReport_Metrics`:

```go
{
    desc:  tempest.Pressure,
    value: 987.81,  // CHANGED: was 98781 (Pascals), now raw mb value
},
// ... later in test ...
{
    desc:  tempest.ReportInterval,
    value: 1,  // CHANGED: was 60 (seconds), now raw minutes value
},
```

**Step 2: Run tests**

Run:
```bash
go test ./internal/tempestudp -v
```

Expected: All tests pass

**Step 3: Commit**

```bash
git add internal/tempestudp/report_test.go
git commit -m "test: update expectations for raw UDP values

- Pressure: 987.81 mb (not 98781 Pa)
- Report interval: 1 minute (not 60 seconds)"
```

---

## Task 11: Run Full Test Suite

**Files:**
- None (verification step)

**Step 1: Run all tests**

Run:
```bash
go test ./...
```

Expected: All tests pass

**Step 2: If failures occur**

Debug and fix any failing tests. Common issues:
- PostgreSQL schema mismatch (drop test database and recreate)
- Missing field extractions
- Incorrect INSERT parameter counts

**Step 3: Build application**

Run:
```bash
go build .
```

Expected: Binary created successfully

**Step 4: Commit if fixes were needed**

```bash
git add <fixed-files>
git commit -m "fix: resolve test failures from schema changes"
```

---

## Task 12: Verify Database Schema Creation

**Files:**
- None (integration test)

**Step 1: Start test PostgreSQL database**

Run:
```bash
docker run --name tempest-test-db -e POSTGRES_PASSWORD=test -p 5432:5432 -d postgres:16
```

**Step 2: Run application with test database**

Run:
```bash
DATABASE_URL="postgresql://postgres:test@localhost:5432/postgres" \
  PUSH_URL="http://localhost:9091" \
  ./tempestwx-utilities
```

Expected: Application starts, schema created successfully

**Step 3: Verify schema in database**

Run:
```bash
docker exec -it tempest-test-db psql -U postgres -c "\d tempest_observations"
```

Expected: Shows table with UUID id column and all fields

**Step 4: Stop test database**

Run:
```bash
docker stop tempest-test-db && docker rm tempest-test-db
```

**Step 5: Document verification**

No commit needed - verification complete.

---

## Task 13: Update CLAUDE.md Documentation

**Files:**
- Modify: `CLAUDE.md:26-62`

**Step 1: Update field documentation with raw units**

Update the configuration section to document raw value storage:

```markdown
## PostgreSQL Storage (Optional)

The exporter can write metrics to PostgreSQL in addition to (or instead of) Prometheus.

### Data Storage

**All UDP values stored as raw** - no unit conversions:
- Pressure: `mb` (millibars) from field 6
- Report Interval: `minutes` from field 17
- All other fields: stored exactly as received

### Database Schema

Four typed tables are automatically created on startup:
- `tempest_observations` - Main weather data (~1/minute) with UUID primary keys
- `tempest_rapid_wind` - High-frequency wind readings (~3 seconds)
- `tempest_hub_status` - Device health metrics
- `tempest_events` - Rain start and lightning strike events

All tables use UUIDv7 primary keys (generated in Go, no PostgreSQL extensions required).
```

**Step 2: Verify documentation is accurate**

Review changes against implemented code.

**Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for UUIDs and raw values

- Document UUIDv7 primary keys
- Document raw value storage (no conversions)
- Update schema description"
```

---

## Task 14: Final Verification and Tag v1.0.0

**Files:**
- None (git operations)

**Step 1: Review all changes**

Run:
```bash
git log --oneline -20
```

Expected: Shows clean commit history with descriptive messages

**Step 2: Run final build and test**

Run:
```bash
go mod tidy
go test ./...
go build .
```

Expected: All pass, binary created

**Step 3: Tag v1.0.0**

Run:
```bash
git tag -a v1.0.0 -m "Release v1.0.0: Complete schema with UUIDs and raw values

- UUIDv7 primary keys (Go-generated, no extensions)
- Complete UDP field coverage (wind_sample_interval, precip_type)
- Raw value storage (pressure in mb, interval in minutes)
- Lightning metrics in Prometheus and PostgreSQL
- All UDP message types supported
- PostgreSQL and Prometheus backends
- API export mode for historical data"
```

**Step 4: Push tag to remote**

Run:
```bash
git push origin v1.0.0
```

Expected: Tag pushed successfully

**Step 5: Verify tag**

Run:
```bash
git tag -l v1.0.0 --format='%(refname:short): %(contents:subject)'
```

Expected: Shows v1.0.0 with release message

---

## Success Criteria

- ✅ All tables use UUID primary keys (no BIGSERIAL)
- ✅ UUIDs generated in Go with `uuid.NewV7()`
- ✅ Complete field coverage: wind_sample_interval, precip_type
- ✅ Raw values stored: pressure (mb), report_interval (minutes)
- ✅ All tests pass
- ✅ Application builds successfully
- ✅ Schema creates correctly in PostgreSQL
- ✅ Documentation updated
- ✅ Tagged as v1.0.0
