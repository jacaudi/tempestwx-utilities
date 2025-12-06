# Schema Enhancements: Missing Fields + UUIDv7 Migration

**Date**: 2025-12-06
**Status**: Approved
**Author**: Claude (with jacaudi)

## Overview

Enhance the PostgreSQL schema to capture complete observation data and migrate from BIGSERIAL to UUIDv7 primary keys while preserving existing data.

## Objectives

1. **Data Completeness**: Add missing fields from Tempest UDP API (wind_sample_interval, precip_type)
2. **Modern IDs**: Migrate from BIGSERIAL to UUIDv7 for all primary keys
3. **Zero Data Loss**: Preserve all existing records during migration
4. **No Extensions**: Generate UUIDs in Go code, avoid PostgreSQL extension dependencies

## Background

### Current State
- Tables use `BIGSERIAL` for auto-incrementing primary keys
- Missing fields from `obs_st` UDP messages:
  - Field 5: `wind_sample_interval` (seconds)
  - Field 13: `precip_type` (0=none, 1=rain, 2=hail, 3=rain+hail)
- UUID support would enable better distributed system compatibility

### Constraints
- Must preserve existing production data
- Minimize application downtime
- Avoid PostgreSQL extensions (keep deployment simple)

## Design Decisions

### UUID Generation Strategy
**Decision**: Generate UUIDv7 in Go code using `github.com/google/uuid`

**Rationale**:
- No PostgreSQL extension dependencies (works on any PG 9.4+)
- Full control over UUID generation in application
- Testable and mockable
- Consistent with Go idioms

**Alternatives Considered**:
- ❌ PostgreSQL `pg_uuidv7` extension - adds deployment complexity
- ❌ Database default `gen_random_uuid()` - loses time-ordering benefit of v7
- ✅ **Go-generated UUIDv7** - chosen for simplicity and control

### Migration Approach
**Decision**: Two-phase migration (add columns → swap keys)

**Rationale**:
- Backward compatible during transition
- Can backfill UUIDs while app is running
- Rollback possible at each step
- Minimal downtime

## Architecture

### Schema Changes

#### Phase 1: Add Missing Fields (Backward Compatible)
```sql
ALTER TABLE tempest_observations
  ADD COLUMN wind_sample_interval DOUBLE PRECISION,
  ADD COLUMN precip_type INTEGER;
```

#### Phase 2: UUID Migration
```sql
-- Step 1: Add UUID column (nullable initially)
ALTER TABLE tempest_observations
  ADD COLUMN new_id UUID;

-- Step 2: Backfill UUIDs for existing rows
UPDATE tempest_observations SET new_id = gen_random_uuid() WHERE new_id IS NULL;

-- Step 3: Make UUID non-null and primary key
ALTER TABLE tempest_observations
  ALTER COLUMN new_id SET NOT NULL,
  DROP CONSTRAINT tempest_observations_pkey,
  ADD PRIMARY KEY (new_id),
  DROP COLUMN id,
  RENAME COLUMN new_id TO id;
```

(Repeat for all tables: rapid_wind, hub_status, events)

### Go Code Changes

#### Dependencies
```go
import "github.com/google/uuid"
```

#### Row Structs
```go
type observationRow struct {
    id                   uuid.UUID  // Changed from not included
    serialNumber         string
    timestamp            time.Time
    windLull             float64
    windAvg              float64
    windGust             float64
    windDirection        float64
    windSampleInterval   *float64   // NEW: field 5
    pressure             float64
    tempAir              float64
    tempWetbulb          float64
    humidity             float64
    illuminance          float64
    uvIndex              float64
    irradiance           float64
    rainRate             float64
    precipType           *int       // NEW: field 13
    lightningDistance    *float64
    lightningStrikeCount *float64
    battery              *float64
    reportInterval       *float64
}
```

#### UUID Generation
```go
func (w *PostgresWriter) handleObservationReport(ctx context.Context, r *tempestudp.TempestObservationReport) error {
    for _, ob := range r.Obs {
        // ... field extraction ...

        row := observationRow{
            id:           uuid.Must(uuid.NewV7()),  // Generate UUIDv7
            serialNumber: r.SerialNumber,
            timestamp:    ts,
            // ... populate all fields ...
        }

        // ... send to batch channel ...
    }
}
```

#### INSERT Statements
```go
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
`, row.id, row.serialNumber, row.timestamp, ...)
```

## Data Model

### Updated Field Mappings

#### tempest_observations
| Column | UDP Field | Unit | Type | Notes |
|--------|-----------|------|------|-------|
| id | N/A | N/A | UUID | **NEW**: UUIDv7 generated in Go |
| serial_number | N/A | text | TEXT | Device serial |
| timestamp | 0 | seconds | TIMESTAMPTZ | Unix epoch UTC |
| wind_lull | 1 | m/s | DOUBLE PRECISION | |
| wind_avg | 2 | m/s | DOUBLE PRECISION | |
| wind_gust | 3 | m/s | DOUBLE PRECISION | |
| wind_direction | 4 | degrees | DOUBLE PRECISION | |
| **wind_sample_interval** | **5** | **seconds** | **DOUBLE PRECISION** | **NEW** |
| pressure | 6 | mb | DOUBLE PRECISION | Raw UDP value |
| temp_air | 7 | °C | DOUBLE PRECISION | |
| temp_wetbulb | calculated | °C | DOUBLE PRECISION | |
| humidity | 8 | % | DOUBLE PRECISION | |
| illuminance | 9 | lux | DOUBLE PRECISION | |
| uv_index | 10 | index | DOUBLE PRECISION | |
| irradiance | 11 | W/m² | DOUBLE PRECISION | |
| rain_rate | 12 | mm | DOUBLE PRECISION | |
| **precip_type** | **13** | **enum** | **INTEGER** | **NEW: 0=none, 1=rain, 2=hail, 3=both** |
| lightning_distance | 14 | km | DOUBLE PRECISION | |
| lightning_strike_count | 15 | count | DOUBLE PRECISION | |
| battery | 16 | volts | DOUBLE PRECISION | |
| report_interval | 17 | minutes | DOUBLE PRECISION | Raw UDP value |

#### tempest_rapid_wind, tempest_hub_status, tempest_events
All tables get same UUID migration (BIGSERIAL → UUID with Go-generated UUIDv7).

## Complete Units Reference Table

### Stored Data Units by Table

#### tempest_observations
| Field | Unit | PostgreSQL Storage | Conversion Notes |
|-------|------|-------------------|------------------|
| id | N/A | UUID | UUIDv7 generated in Go |
| serial_number | text | TEXT | e.g., "ST-00019709" |
| timestamp | seconds (Unix epoch) | TIMESTAMPTZ | UTC timezone |
| wind_lull | m/s (meters/second) | DOUBLE PRECISION | No conversion |
| wind_avg | m/s (meters/second) | DOUBLE PRECISION | No conversion |
| wind_gust | m/s (meters/second) | DOUBLE PRECISION | No conversion |
| wind_direction | degrees (0-360°) | DOUBLE PRECISION | No conversion |
| wind_sample_interval | seconds | DOUBLE PRECISION | No conversion |
| pressure | mb (millibars) | DOUBLE PRECISION | Raw UDP value |
| temp_air | °C (Celsius) | DOUBLE PRECISION | No conversion |
| temp_wetbulb | °C (Celsius) | DOUBLE PRECISION | Calculated from temp/RH/pressure |
| humidity | % (0-100) | DOUBLE PRECISION | No conversion |
| illuminance | lux | DOUBLE PRECISION | No conversion |
| uv_index | index (0-16+) | DOUBLE PRECISION | No conversion |
| irradiance | W/m² (watts/meter²) | DOUBLE PRECISION | No conversion |
| rain_rate | mm (millimeters) | DOUBLE PRECISION | No conversion |
| precip_type | enum (0-3) | INTEGER | 0=none, 1=rain, 2=hail, 3=rain+hail |
| lightning_distance | km (kilometers) | DOUBLE PRECISION | No conversion |
| lightning_strike_count | count | DOUBLE PRECISION | No conversion |
| battery | volts | DOUBLE PRECISION | No conversion |
| report_interval | minutes | DOUBLE PRECISION | Raw UDP value |

#### tempest_rapid_wind
| Field | Unit | PostgreSQL Storage | Conversion Notes |
|-------|------|-------------------|------------------|
| id | N/A | UUID | UUIDv7 generated in Go |
| serial_number | text | TEXT | Device serial |
| timestamp | seconds (Unix epoch) | TIMESTAMPTZ | UTC timezone |
| wind_speed | m/s (meters/second) | DOUBLE PRECISION | No conversion |
| wind_direction | degrees (0-360°) | DOUBLE PRECISION | No conversion |

#### tempest_hub_status
| Field | Unit | PostgreSQL Storage | Conversion Notes |
|-------|------|-------------------|------------------|
| id | N/A | UUID | UUIDv7 generated in Go |
| serial_number | text | TEXT | Hub serial (HB-XXXXXXXX) |
| timestamp | seconds (Unix epoch) | TIMESTAMPTZ | UTC timezone |
| uptime | seconds | DOUBLE PRECISION | No conversion |
| rssi | dBm (decibel-milliwatts) | DOUBLE PRECISION | WiFi signal strength |
| reboot_count | count | DOUBLE PRECISION | From radio_stats[1] |
| bus_errors | count | DOUBLE PRECISION | From radio_stats[2], I2C errors |

#### tempest_events
| Field | Unit | PostgreSQL Storage | Conversion Notes |
|-------|------|-------------------|------------------|
| id | N/A | UUID | UUIDv7 generated in Go |
| serial_number | text | TEXT | Device serial |
| timestamp | seconds (Unix epoch) | TIMESTAMPTZ | Event occurrence time |
| event_type | text | TEXT | "rain_start" or "lightning_strike" |
| distance_km | km (kilometers) | DOUBLE PRECISION | Lightning only, NULL for rain |
| energy | (unspecified) | DOUBLE PRECISION | Lightning only, NULL for rain |

### Data Storage Policy: Raw Values

**All UDP fields stored exactly as received** - no unit conversions applied.

**Rationale:**
- Preserves original data integrity
- Simpler code with fewer bugs
- Easier to verify against UDP spec
- Unit conversions can be applied in queries/dashboards
- Future-proof if UDP API changes units

**Calculated Fields:**
- **Wet Bulb Temperature**: Derived field calculated from air temp, humidity, and pressure using psychrometric formula (not in UDP message)

**Example Raw Values:**
- Pressure: `987.81` mb (not converted to Pascals)
- Report Interval: `1` minute (not converted to seconds)
- Temperature: `19.0` °C
- Wind Speed: `0.49` m/s

## Implementation Notes

### Version: v1.0.0

This is a **clean implementation** for the v1.0.0 release. No migration needed - all changes are part of the initial stable schema.

### Prerequisites
1. Add `github.com/google/uuid` to go.mod
2. Update schema constants in `internal/postgres/schema.go`
3. Update row structs in `internal/postgres/writer.go`

### Schema Changes

**1. Update `internal/postgres/schema.go`**:
- Change all `id BIGSERIAL PRIMARY KEY` to `id UUID PRIMARY KEY`
- Add `wind_sample_interval DOUBLE PRECISION` column
- Add `precip_type INTEGER` column
- Remove pressure/interval conversion comments

**2. Update `internal/postgres/writer.go`**:
- Add `id uuid.UUID` field to all row structs
- Generate UUID in all handler methods: `uuid.Must(uuid.NewV7())`
- Add `wind_sample_interval` and `precip_type` to `observationRow`
- Extract fields 5 and 13 from UDP observation messages
- Include `id` in all INSERT statements
- **Remove conversions**: Store pressure as `ob[6]` (not `ob[6]*100`)
- **Remove conversions**: Store report_interval as `ob[17]` (not `ob[17]*60`)

**3. Update `internal/tempestudp/report.go`**:
- **Remove conversions** from Prometheus metrics:
  - Pressure: `ob[6]` instead of `ob[6]*100`
  - Report interval: `ob[17]` instead of `ob[17]*60`

**4. Update `go.mod`**:
```bash
go get github.com/google/uuid
```

## Testing Checklist

### Unit Tests
- ✅ Test UUID generation in row creation
- ✅ Test INSERT statements include id field
- ✅ Test field extraction for wind_sample_interval and precip_type
- ✅ Verify raw values stored (no conversions)

### Integration Tests
- ✅ Verify schema creation with new columns and UUID PKs
- ✅ Test data insertion with UUIDs
- ✅ Verify UNIQUE constraint still works on (serial_number, timestamp)
- ✅ Verify pressure stored as mb, not Pascals
- ✅ Verify report_interval stored as minutes, not seconds

## Success Criteria

- ✅ New fields (`wind_sample_interval`, `precip_type`) captured from UDP messages
- ✅ All primary keys use UUIDv7 generated in Go
- ✅ Application writes data without errors
- ✅ UNIQUE constraints on (serial_number, timestamp) enforced
- ✅ No PostgreSQL extensions required
- ✅ **Raw UDP values stored** (pressure in mb, interval in minutes)

## Future Considerations

### Out of Scope (Not Included)
- Legacy device support (`obs_air`, `obs_sky`) - only Tempest devices supported
- Device status storage - currently parsed but not persisted (by design)

### Potential Enhancements
- Add indexes on new fields if frequently queried
- Consider partitioning large tables by timestamp
- Add precipitation type enum/check constraint for data validation
