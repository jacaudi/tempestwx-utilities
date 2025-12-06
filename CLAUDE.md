# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Multi-backend data utilities for Tempest weather stations. The application operates in two modes:

1. **UDP Listener Mode** (default): Listens for Tempest UDP broadcasts on port 50222 and writes to Prometheus push gateway and/or PostgreSQL in real-time
2. **API Export Mode**: Fetches historical observation data via REST API when `TOKEN` env var is set, writes to PostgreSQL and/or compressed files

## Development Commands

### Testing
```bash
# Run all tests
go test ./...

# Run tests with JSON output (CI format)
go test -json ./...

# Prepare dependencies
go mod tidy
```

### Building
```bash
# Build local Docker image
task build-local

# Direct Docker build
docker buildx bake image-local
```

### Running Locally
```bash
# UDP mode (requires host network for broadcast reception)
docker run -it --rm --net=host \
  -e PUSH_URL=http://localhost:9091 \
  tempestwx-utilities:latest

# API export mode
docker run -it --rm \
  -e TOKEN=your_token \
  tempestwx-utilities:latest
```

## Architecture

### Operational Modes

The application switches modes based on presence of `TOKEN` environment variable:
- **No TOKEN**: Runs `listenAndPush()` - UDP listener with push gateway
- **With TOKEN**: Runs `export()` - Historical data export to gzipped files

### Internal Package Structure

- **`internal/tempest/`**: Defines all Prometheus metric descriptors (`prometheus.Desc`)
- **`internal/tempestudp/`**: Parses UDP broadcast messages into metrics, includes wet bulb temperature calculations
- **`internal/tempestapi/`**: REST API client for fetching historical observations

### Data Flow (UDP Mode)

1. UDP packets received on port 50222 → `listen()`
2. Raw bytes → `tempestudp.ParseReport()` → Report struct
3. Report → `Report.Metrics()` → `[]prometheus.Metric`
4. Metrics buffered in `outbox` channel (cap: 1000)
5. `collector` drains `outbox` when `pusher.Add()` called
6. Metrics pushed to gateway in Prometheus text format

### Key Design Patterns

- Uses Prometheus push pattern (not pull/scrape) because weather stations broadcast sporadically
- Custom `collector` implementation drains buffered metrics non-blockingly
- UDP broadcasts are link-local, requiring `--net=host` in Docker
- Background goroutine handles pushing, triggered by `more` channel

## Configuration

### Environment Variables

- `PUSH_URL`: Required for UDP mode. URL of Prometheus Pushgateway or compatible service (e.g., VictoriaMetrics)
- `JOB_NAME`: Job label for pushed metrics (default: "tempest")
- `LOG_UDP`: Optional. Set to "true" or "1" to log all UDP broadcasts received (default: false)
- `TOKEN`: Optional. When set, switches to API export mode for historical data

## PostgreSQL Storage (Optional)

The exporter can write metrics to PostgreSQL in addition to (or instead of) Prometheus.

### Configuration

**Option 1: Full connection string**
```bash
DATABASE_URL=postgresql://user:pass@localhost:5432/weather
```

**Option 2: Individual components**
```bash
DATABASE_HOST=postgres
DATABASE_PORT=5432              # optional, default: 5432
DATABASE_USERNAME=tempest
DATABASE_PASSWORD=secret
DATABASE_NAME=weather
DATABASE_SSLMODE=disable        # optional: disable, require, verify-ca, verify-full
```

**Optional tuning:**
```bash
POSTGRES_BATCH_SIZE=100         # default: 100
POSTGRES_FLUSH_INTERVAL=10s     # default: 10s
POSTGRES_MAX_RETRIES=3          # default: 3
```

### Database Schema

Four typed tables are automatically created on startup:
- `tempest_observations` - Main weather data (~1/minute)
- `tempest_rapid_wind` - High-frequency wind readings (~3 seconds)
- `tempest_hub_status` - Device health metrics
- `tempest_events` - Rain start and lightning strike events

### Operational Modes

| PUSH_URL | DATABASE_URL/HOST | TOKEN | Behavior |
|----------|-------------------|-------|----------|
| Yes | No | No | Prometheus only (current behavior) |
| Yes | Yes | No | Both Prometheus + Postgres |
| No | Yes | No | Postgres only |
| N/A | No | Yes | API export to .gz files |
| N/A | Yes | Yes | API export to Postgres (+ optional .gz files) |

### Docker Compose Example

```yaml
services:
  tempest-utilities:
    image: tempestwx-utilities:latest
    network_mode: host
    environment:
      PUSH_URL: http://pushgateway:9091
      DATABASE_HOST: postgres
      DATABASE_USERNAME: tempest
      DATABASE_PASSWORD: ${DB_PASSWORD}
      DATABASE_NAME: weather
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

volumes:
  pgdata:
```

### API Export with Backfill

To backfill historical data into Postgres:

```bash
TOKEN=your_api_token DATABASE_URL=postgresql://... go run .
```

Optionally keep .gz files:

```bash
TOKEN=your_api_token DATABASE_URL=postgresql://... KEEP_EXPORT_FILES=true go run .
```

## Testing Notes

Test files located alongside implementation:
- `internal/tempestudp/report_test.go`: UDP message parsing
- `internal/tempestudp/wetbulb_test.go`: Wet bulb calculations
- `internal/tempestapi/client_test.go`: API client

Go 1.23.0+ required (see go.mod).
