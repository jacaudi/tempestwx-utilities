# Unified Weather Platform — Implementation Plan

## Revisions

- **v2 2026-07-17 — cold-review remediation + user decisions.** Applied the cold review's prioritized fix list (`docs/review/2026-07-17-plan-cold-review.md`): resolved the Task 2.1 fixture BLOCKER; Task 1.1 now emits a concrete UI manifest (real file tree + `weather.ts` types) so downstream UI tasks cite present symbols; inlined concrete Go code in the GREEN steps of the concurrency/DSN/lifecycle tasks (0.4, 0.7, 0.8, 0.9a/0.9b, 3.3) and the `main.go` extraction seams (`signalContext`, `requireWriters`, `selectStore`); pinned previously-unstated constants and asserted them in RED tests (3.3 backpressure timeout + channel caps; 2.4 LRU size/TTL); repaired Task 4.1 to drive the real OTel Prometheus exporter; split oversized Task 1.7 into 1.7a/1.7b/1.7c; disambiguated Contract A error transport (non-200 + JSON envelope); made Task 3.6 a default (non-integration) filesystem-replica test; provided the radar site-table data acquisition inline. Applied user decisions: **B1** wire legacy Postgres tunables (new Task 0.13, resolves C-H2 for Postgres); **B2** basemap = OpenStreetMap via Protomaps `.pmtiles` served same-origin (CSP now self-only + MapLibre worker allowance); **B3** `/api/almanac` = proxy WeatherFlow. Design doc updated in lockstep.
- **v2.1 2026-07-17 — focused re-review cleanup.** Patched the 4 MINOR nits from `docs/review/2026-07-17-plan-cold-review-v2.md`: fixed the stale `CurrentWeather`→`CurrentObservation` type reference (Task 1.4); reworded Task 0.8 so it no longer forward-references the Task 0.9a `done` gate (which lands later); made the `insertObservations`/`insertRapidWind`/`insertHubStatus`/`insertEvents` context-parameter ripple explicit in Task 0.8; fixed a garbled comment in the 3.3 `eventBlockTimeout` const. No structural changes; re-review verdict was READY-WITH-MINORS.

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Every task uses TDD (`superpowers:test-driven-development`): write the failing test, watch it fail for the right reason, then make it pass.

**Goal:** Evolve `tempestwx-utilities` from a headless UDP/API exporter into a single-binary weather platform — embedded React UI, server-side WeatherFlow token proxy, NEXRAD Level 3 radar overlay (opt-in Python Py-ART sidecar → contoured GeoJSON), SQLite+Litestream default storage (Postgres retained), a unified OpenTelemetry (OTLP) backbone, a Grafana dashboard, and a prioritized UX overhaul — while fixing the CRITICAL/HIGH lifecycle findings from the two source reviews.

**Architecture:** All new sinks are additive on the existing `internal/sink` fan-out seam; a new HTTP surface serves the embedded UI + a tokenless JSON API. Foundation lifecycle fixes (Workstream 0) land first because every later workstream depends on correct shutdown, panic isolation, and context handling. **Rationale for every decision lives in the design doc — do not duplicate it here. Read it first:** `docs/designs/2026-07-17-unified-weather-platform-design.md` (§ references throughout this plan point there).

**Source reviews this plan resolves:**
- Exporter: `docs/review/2026-07-17-code-review-summary.md` (1 CRITICAL, 17 HIGH). Finding IDs: `C-1`, `A-H1..H3`, `C-H1..H3`, `D-H1..H2`, `F-H1..H4`, `G-H1..H3`, `B-HIGH`, plus MEDIUMs cited per task.
- UI: `docs/review/2026-07-17-ui-code-review-summary.md` (0 CRITICAL, 8 HIGH). Finding IDs: `A-H1..H4`, `B-H1..H2`, `E-H1`, `F-H1`. (UI findings referenced with a `UI-` prefix below to disambiguate from exporter findings that reuse the same letters.)

**Tech Stack:** Go 1.24 (toolchain 1.25), `modernc.org/sqlite` (pure Go, CGO-free), OpenTelemetry Go SDK + OTLP exporters, `otelhttp`; React 19 + TypeScript 5.9 + Vite 7 + Vitest + MapLibre GL JS; Python 3.12 + FastAPI + Py-ART + boto3 + geojsoncontour (radar sidecar); Litestream, OTel Collector, Prometheus, Grafana, MinIO (compose stack).

---

## Execution Workflow

> **For Claude:** REQUIRED EXECUTION WORKFLOW (follow in order):
> 1. `superpowers:using-git-worktrees` — Isolate work in a dedicated worktree
> 2. `superpowers:subagent-driven-development` — Dispatch a fresh subagent per task
> 3. `superpowers:test-driven-development` — All subagents use TDD
> 4. `superpowers:verification-before-completion` — Verify all tests pass per task
> 5. `superpowers:requesting-code-review` — Code review after each task (built in)
> 6. After all tasks: comprehensive code review on full diff from branch point (automatic)
> 7. `superpowers:finishing-a-development-branch` — Complete the branch
>
> Skills carry their own model and effort settings. Do not override them.

---

## Global Constraints

Every task's requirements implicitly include this section. Values are copied verbatim from the design (§ noted).

- **Go version floor:** module `go 1.24.0`, toolchain `go1.25.5`. Use modern Go idioms up to 1.24 (`any`, `slices`/`maps`/`cmp`, `min`/`max`, `for range n`, `errors.Is/As`, `t.Context()` in tests, `omitzero` JSON tags for time/duration/struct/slice/map fields, `b.Loop()` in benchmarks). (go.mod)
- **CGO-free static build is load-bearing:** `CGO_ENABLED=0` MUST hold. SQLite driver MUST be `modernc.org/sqlite` — never `mattn/go-sqlite3` (CGO). This preserves the praised static Chainguard image. (design §10, O6)
- **Radar product default:** `N0B` (super-res base reflectivity), fall back to `N0Q` (legacy) if a site lacks a recent N0B object. (design §9, O2)
- **Bespoke Prometheus path:** deprecate-then-remove (keep for one release, log a deprecation warning) — do NOT remove-now. (design §8, O4)
- **Telemetry scope:** the app only *ships* OTLP to `OTEL_EXPORTER_OTLP_ENDPOINT`; it does NOT bundle or prescribe trace/log backends. Example compose = Collector + Prometheus + Grafana only; Tempo/Loki commented. (design §8, R1, O5)
- **Metric names preserved:** OTel instruments MUST translate (via the Collector) to the exact existing `tempest_*` Prometheus names (see Contract B). The `instance`→`serial` label rename is the one intentional break. (design §12)
- **API/JSON units:** all backend JSON and OTel instruments use SI units (°C, m/s, mb, mm) so the vendored UI's unit hook handles display conversion. (design §11)
- **HTTP hardening:** every `http.Server` sets `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`; graceful shutdown; containers run non-root `USER 65532`. (design §15)
- **CSP is self-only (B2):** the map basemap is an OpenStreetMap Protomaps `.pmtiles` file served **same-origin** — there is no external tile server and no API key. The Content-Security-Policy therefore names **no external hosts**. Exact directive: `default-src 'self'; img-src 'self' data: blob:; worker-src 'self' blob:; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'self'`. The `blob:`/`wasm-unsafe-eval` allowances exist solely for MapLibre GL JS's Web Worker + WebAssembly renderer; they add no network origin. Plus `X-Content-Type-Options: nosniff` and `Referrer-Policy: no-referrer`. (design §7/§9/§15, B2)
- **Logging:** standardize new packages on `log/slog` (structured, stdout). Do not add zerolog/logrus. (go-standards §7)
- **Testing/verification per task:** `go build ./...` clean; `go test -race ./<touched-pkg>/...` passes; `go vet ./...` clean; `gofmt -l` reports nothing; `golangci-lint run` clean (add `.golangci.yml` in Task 0.1 if absent). Report real command output — never "looks correct". (go-standards §8, §8.5)
- **Secrets:** WeatherFlow `TOKEN`, `LITESTREAM_*` creds, and any S3 creds live only in env; never in the bundle, `localStorage`, logs, or committed files. Scrub `*url.Error` before logging. (design §15)
- **Commit discipline:** one commit per task at minimum; conventional-commit messages; end commit messages with the Co-Authored-By trailer.

---

## Dependencies to Add (pinned)

Add these as tasks reach them (do not add all up front — YAGNI; add per consuming task). Versions are floors; run `go mod tidy` after each `go get`.

**Go modules:**
| Module | Version | Consuming workstream |
|---|---|---|
| `modernc.org/sqlite` | `v1.34.4` | WS3 (SQLite writer) |
| `go.opentelemetry.io/otel` | `v1.32.0` | WS6 |
| `go.opentelemetry.io/otel/sdk` | `v1.32.0` | WS6 |
| `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` | `v1.32.0` | WS6 |
| `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc` | `v1.32.0` | WS6 |
| `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc` | `v0.8.0` (experimental) | WS6 |
| `go.opentelemetry.io/otel/log` + `.../sdk/log` | `v0.8.0` | WS6 |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | `v0.57.0` | WS6/WS1 |
| `go.opentelemetry.io/otel/exporters/prometheus` | `v0.54.0` | WS4 (Task 4.1 name-translation assertion) |
| `github.com/google/uuid` | `v1.6.0` (already present) | WS3 |

> Before writing any OTel code, confirm current APIs via Context7 (`resolve-library-id` → `query-docs` for `go.opentelemetry.io/otel`) — the OTLP log signal is experimental and its API moves. Do not code OTel from memory.

**Python (radar sidecar, `radar/requirements.txt`):**
| Package | Version | Purpose |
|---|---|---|
| `fastapi` | `>=0.115,<1` | HTTP framework |
| `uvicorn[standard]` | `>=0.32,<1` | ASGI server |
| `arm-pyart` | `>=1.19` | NEXRAD L3 decode (`pyart.io.read_nexrad_level3`) |
| `boto3` | `>=1.35` | anonymous S3 (`s3://unidata-nexrad-level3`) |
| `geojsoncontour` | `>=0.4` | contour → GeoJSON isobands |
| `matplotlib` | `>=3.9` | contourf backend (headless `Agg`) |
| `pytest` | `>=8.3` | sidecar tests |

**JS (UI, `web/package.json` — vendored from `tempest-display@49892063`):**
| Package | Version | Purpose |
|---|---|---|
| `maplibre-gl` | `^4.7` | radar map card |
| `pmtiles` | `^3.2` | read the same-origin OSM `.pmtiles` basemap (MapLibre protocol) |
| `vitest` | `^2.1` | unit tests (currently none) |
| `@testing-library/react` | `^16` | component/error-boundary tests |

**Compose images (digest-pin at task time):** `otel/opentelemetry-collector-contrib`, `prom/prometheus`, `grafana/grafana`, `litestream/litestream`, `minio/minio`, and this repo's `radar-sidecar` image.

---

## Cross-Task Contracts (single source of truth — dependents cite these by name)

These three contracts have consumers on both sides of a task boundary. They are defined once here; tasks that produce or consume them reference **Contract A/B/C** rather than re-deriving. Changing a contract is a one-edit change here.

### Contract A — Go ↔ Radar Sidecar HTTP contract (WS2)

- **Request:** `GET /radar?site={SSS}&product={N0B|N0Q}` on the sidecar (default sidecar addr `http://radar-sidecar:8081`). `{SSS}` = 3-char uppercase WSR-88D site (no leading `K`), validated by the Go side against the static site table before the call (SSRF guard).
- **Success (HTTP 200), `Content-Type: application/json`:**
  ```json
  {
    "type": "FeatureCollection",
    "features": [ { "type": "Feature", "properties": { "dbz_min": 20, "dbz_max": 25 }, "geometry": { "type": "Polygon", "coordinates": [ ... ] } } ],
    "metadata": { "site": "TLX", "product": "N0B", "scan_time": "2026-07-17T18:42:10Z", "bbox": [minLon, minLat, maxLon, maxLat] }
  }
  ```
  Isobands are 5-dBZ steps over the NWS reflectivity scale; each `Feature.properties` carries `dbz_min`/`dbz_max` (band bounds) for client-side data-driven styling.
- **Error — single transport, no ambiguity: a non-200 status code + a JSON error envelope**, `Content-Type: application/json`. (Chosen over "HTTP 200 + envelope" so the Go client can branch on `resp.StatusCode` alone; the Python side MUST NOT return 200 on error.) Status→error mapping the sidecar emits:
  - `503` → `{ "error": "no_recent_scan" }` (no decodable NIDS object in S3 for the requested `site`/`product`)
  - `502` → `{ "error": "decode_failed" }` (object found but Py-ART/contour raised)
  - `500` → `{ "error": "internal" }` (any other server-side failure)
  ```json
  { "error": "no_recent_scan" }
  ```
  `error` ∈ {`no_recent_scan`, `decode_failed`, `internal`}. The Go side reads `resp.StatusCode != 200` → decode the envelope → map to `/api/radar/{site}` responses (see Contract C) and degrade gracefully (soft-fail, mirroring DRAS).
- **Cache key (Go side):** `(site, product, scan_time)`. TTL ≈ one scan cycle (~5 min). Bounded in-memory LRU.

### Contract B — OTel instrument → Prometheus name map (WS6, consumed by WS4)

Instruments are named with dotted OTel convention; the Collector's OTLP→Prometheus translation yields the underscore names on the right. WS4 PromQL uses ONLY the right column. `serial` is a metric label (resource attribute `tempest.serial`); `kind` distinguishes air/gust/avg etc.

| OTel instrument | Kind | Prometheus name | Notes |
|---|---|---|---|
| `tempest.temperature.c` | Gauge | `tempest_temperature_c` | label `kind="air"` |
| `tempest.dewpoint.c` | Gauge | `tempest_dewpoint_c` | derived |
| `tempest.heat_index.c` | Gauge | `tempest_heat_index_c` | derived |
| `tempest.wetbulb.c` | Gauge | `tempest_wetbulb_c` | derived (skip if NaN) |
| `tempest.humidity.percent` | Gauge | `tempest_humidity_percent` | |
| `tempest.pressure.mb` | Gauge | `tempest_pressure_mb` | |
| `tempest.wind.meters_per_second` | Gauge | `tempest_wind_meters_per_second` | fixes `_ms` (D-MEDIUM); `kind` ∈ {avg,gust,lull} |
| `tempest.wind.direction.degrees` | Gauge | `tempest_wind_direction_degrees` | |
| `tempest.uv.index` | Gauge | `tempest_uv_index` | |
| `tempest.irradiance.w_m2` | Gauge | `tempest_irradiance_w_m2` | |
| `tempest.illuminance.lux` | Gauge | `tempest_illuminance_lux` | |
| `tempest.rain_rate.mm_min` | Gauge | `tempest_rain_rate_mm_min` | |
| `tempest.rainfall.mm` | Counter | `tempest_rainfall_mm_total` | wires dead `RainTotal` (D-MEDIUM) |
| `tempest.lightning.distance.km` | Gauge | `tempest_lightning_distance_km` | |
| `tempest.lightning.strike_count` | Counter | `tempest_lightning_strike_count_total` | |
| `tempest.battery.volts` | Gauge | `tempest_battery_volts` | |
| `tempest.rssi.dbm` | Gauge | `tempest_rssi_dbm` | |
| `tempest.uptime.seconds` | Gauge | `tempest_uptime_seconds` | fixes `_total` (D-MEDIUM) |
| `tempest.reboots` | Counter | `tempest_reboots_total` | |
| `tempest.bus_errors` | Counter | `tempest_bus_errors_total` | |

Resource attributes: `service.name=tempestwx`, `service.version`, `host.name`, `tempest.serial` (replaces reserved `instance` label — fixes D-MEDIUM).

### Contract C — Backend JSON API shapes (WS1, consumed by UI data layer)

All endpoints are tokenless from the browser and `otelhttp`-traced. Shapes match the UI's existing `web/src/types/weather.ts` (SI units). The exact TypeScript interfaces are reproduced verbatim in **Task 1.1's UI manifest** (below) — that manifest is the single source for these field names; do NOT invent names. The Go handlers marshal JSON whose keys match those interface field names exactly (e.g. `airTemperature`, `stationPressure`, `windAvg`, `dewPoint`).

| Endpoint | Source | Response (interface from `web/src/types/weather.ts`) |
|---|---|---|
| `GET /api/observations/current` | sqlite latest row | one `CurrentObservation` object (SI; keys `timestamp,windLull,windAvg,windGust,windDirection,windSampleInterval,stationPressure,airTemperature,relativeHumidity,illuminance,uvIndex,solarRadiation,rainAccumulated,precipitationType,lightningStrikeAvgDistance,lightningStrikeCount,battery,reportInterval,localDayRainAccumulation,feelsLike,dewPoint,wetBulbTemperature,heatIndex,windChill,pressureTrend`). Derived fields (`feelsLike`,`dewPoint`,`wetBulbTemperature`,`heatIndex`,`windChill`,`pressureTrend`) are computed server-side from the stored row. Resolves UI B-H2. |
| `GET /api/observations/history?field=&from=&to=` | sqlite range query | `{ "points": [ { "t": <epoch>, "v": <float> } ] }` |
| `GET /api/station` | WeatherFlow REST proxy (`Bearer`) | `StationMeta` (`station_id,name,latitude,longitude,elevation,timezone,firmware_revision,serial_number,device_id`) |
| `GET /api/forecast` | WeatherFlow REST proxy (`Bearer`) | Better-Forecast → `ForecastDay[]`/`HourlyForecast[]` passthrough |
| `GET /api/almanac` | **WeatherFlow REST proxy (`Bearer`)** — B3: proxy, not computed | `StationAlmanac` (`today/week/month/year: TempRecord`, `sunrise,sunset,moonPhase,moonPhaseName,moonIllumination`) proxied from the WeatherFlow Better-Forecast/station endpoints |
| `GET /api/radar/{site}?product=N0B` | `internal/radar` → sidecar (Contract A) | GeoJSON + metadata, or `{ "error": "..." }` with HTTP 502/503 mapping. Opt-in `ENABLE_RADAR`. |
| `GET /healthz` | — | `200 {"status":"ok"}` |
| `GET /metrics` | legacy prometheus scrape | deprecated; kept one release (O4) |

---

# Task List

Grouped by workstream in the design's §19 execution order. Task IDs are `W<workstream>.<n>`. Each task ends with an independently testable deliverable.

---

## Workstream 0 — Foundation: lifecycle & review-fix bedrock

> Design §6. Do FIRST. Early tasks (0.1–0.6) are self-contained and do NOT depend on the interface change, so they keep the tree green. Task 0.7 changes the `MetricsWriter` interface and MUST update all existing implementers in the same task.

### Task 0.1: Add project tooling — `.golangci.yml`, slog baseline, gofmt fix — no deps

**Files:**
- Create: `.golangci.yml`
- Modify: `internal/tempest/metrics.go` (gofmt only)

**Steps:**
- [ ] **Step 1:** Add `.golangci.yml` enabling `govet, errcheck, staticcheck, gosec, unused, gocritic` (go-standards §8.5).
- [ ] **Step 2:** Run `gofmt -w internal/tempest/metrics.go` (fixes the review's gofmt-clean finding).
- [ ] **Step 3 (verify):** `gofmt -l ./...` → prints nothing; `golangci-lint run ./...` → exits clean (fix or `//nolint`-with-justification any pre-existing hits, documenting each).
- [ ] **Step 4:** Commit `chore: add golangci config, gofmt metrics.go`.

**Acceptance:** `gofmt -l ./...` empty; `golangci-lint run ./...` clean; `go build ./...` clean.
**Implements:** review NIT (gofmt), go-standards §8.5.

### Task 0.2: Guard `RadioStats` indexing (CRITICAL C-1) — no deps

**Files:**
- Modify: `internal/tempestudp/report.go` (`HubStatusReport.Metrics`, ~line 258-266)
- Test: `internal/tempestudp/report_test.go`

**Steps:**
- [ ] **Step 1 (RED):** Add `TestHubStatusReport_ShortRadioStats` to `internal/tempestudp/report_test.go`: parse a `hub_status` packet whose `radio_stats` is `[]` (and separately `null`, and `[1]`), call `.Metrics()`, assert it returns without panicking and omits the reboots/bus_errors metrics (len reflects only uptime+rssi).
- [ ] **Step 2:** Run `go test ./internal/tempestudp/ -run TestHubStatusReport_ShortRadioStats` → FAIL with index-out-of-range panic.
- [ ] **Step 3 (GREEN):** Wrap the `RadioStats[1]`/`[2]` metrics in `if len(r.RadioStats) >= 3 { ... }`; build the slice conditionally.
- [ ] **Step 4:** Run the test → PASS.
- [ ] **Step 5:** Commit `fix: guard RadioStats index to prevent remote panic (C-1)`.

**Acceptance:** `go test -race ./internal/tempestudp/ -run TestHubStatusReport_ShortRadioStats` passes; `go build ./...` clean.
**Implements:** design §6; resolves **C-1** (CRITICAL).

### Task 0.3: `wetBulb` returns NaN instead of panicking (E-MEDIUM) — no deps

**Files:**
- Modify: `internal/tempestudp/wetbulb.go`
- Test: `internal/tempestudp/wetbulb_test.go`

**Steps:**
- [ ] **Step 1 (RED):** Add `TestWetBulb_NonConvergentReturnsNaN`: call `WetBulbTemperatureC` with inputs that do not converge, assert `math.IsNaN(result)` (not a panic). Keep an existing convergent-case assertion.
- [ ] **Step 2:** Run → FAIL (panic "failed to converge").
- [ ] **Step 3 (GREEN):** Replace `panic("failed to converge")` with `return math.NaN()`. Modernize the loop to `for range 10000`.
- [ ] **Step 4:** In `internal/tempestudp/report.go`, ensure the caller skips emitting the wet-bulb metric when the value is NaN (guard `if !math.IsNaN(wb)`).
- [ ] **Step 5:** Run `go test -race ./internal/tempestudp/` → PASS.
- [ ] **Step 6:** Commit `fix: wetBulb returns NaN instead of panicking (E-MEDIUM)`.

**Acceptance:** `go test -race ./internal/tempestudp/` passes.
**Implements:** design §6; resolves **E-MEDIUM** (wet-bulb panic).

### Task 0.4: Postgres DSN via `net/url` (B-HIGH credential escaping) — no deps

**Files:**
- Modify: `internal/config/database.go` (`GetDatabaseConfig`)
- Test: `internal/config/database_test.go`

**Steps:**
- [ ] **Step 1 (RED):** Add `TestGetDatabaseConfig_EscapesCredentials`: set `POSTGRES_HOST/USERNAME/PASSWORD/NAME` via `t.Setenv`, with a password containing `@ : / ? # &` special chars; assert the returned DSN round-trips through `net/url.Parse` and `url.User.Password()` returns the exact original password.
- [ ] **Step 2:** Run → FAIL (raw `fmt.Sprintf` corrupts special chars).
- [ ] **Step 3 (GREEN):** Replace the raw `fmt.Sprintf` at `internal/config/database.go:52-53` with a `net/url`-built DSN. The full function body after the change (the `POSTGRES_URL` precedence branch and the required-field checks above line 47 are unchanged — only the construction at the bottom changes):

```go
import (
	"cmp"
	"fmt"
	"net"
	"net/url"
	"os"
)

// ... (POSTGRES_URL precedence + host/username/password/dbname required-field
//      checks unchanged from the current file) ...

	port := cmp.Or(os.Getenv("POSTGRES_PORT"), "5432")
	sslmode := cmp.Or(os.Getenv("POSTGRES_SSLMODE"), "disable")

	u := url.URL{
		Scheme: "postgresql",
		User:   url.UserPassword(username, password), // percent-encodes @ : / ? # &
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + dbname,
	}
	u.RawQuery = url.Values{"sslmode": {sslmode}}.Encode()
	return u.String(), nil
```

  Note: `url.UserPassword` percent-encodes the credentials, and `u.String()` round-trips cleanly through `url.Parse`, which is exactly what the RED test asserts. `pgxpool.ParseConfig` (used by the postgres writer) accepts this URL form.
- [ ] **Step 4:** Run `go test ./internal/config/` → PASS.
- [ ] **Step 5:** Commit `fix: build postgres DSN with net/url to escape credentials (B-HIGH)`.

**Acceptance:** `go test ./internal/config/` passes.
**Implements:** design §6; resolves **B-HIGH**.

### Task 0.5: Harden the WeatherFlow API client (F-H1..H4, F-MEDIUM) — no deps

**Files:**
- Modify: `internal/tempestapi/client.go`
- Test: `internal/tempestapi/client_test.go`

**Steps:**
- [ ] **Step 1 (RED):** Add tests using `httptest.Server`:
  - `TestClient_UsesBearerHeader_NotURLToken`: assert the request `Authorization` header == `"Bearer "+token` and the request URL query has NO `token=` param.
  - `TestClient_ReturnsErrorOn401`: server returns 401 → `ListStations`/`GetObservations` return a non-nil error mentioning the status, do NOT decode as success.
  - `TestClient_HasTimeout`: assert the client's `Timeout` is `30 * time.Second` (via an exported field or a slow server + short ctx that returns a deadline error).
  - `TestClient_UnhandledReportType_ReturnsError`: feed an observation the switch can't handle → returns error (NOT `log.Fatalf`).
  - `TestClient_ChecksStatusCode` (F-MEDIUM): API JSON `status.status_code != 0` → error.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Rework `internal/tempestapi/client.go`:
  - `Client` gains an owned client: `type Client struct { token string; http *http.Client }`; `NewClient` returns `*Client` with `http: &http.Client{Timeout: 30 * time.Second}`. Change both methods (`ListStations`, `GetObservations`) to **pointer receiver** `func (c *Client) ...` to match.
  - **Bearer, not URL token:** drop the `?token=`/`&token=` from both request URLs (`client.go:34` and `client.go:90`); after building the request, `req.Header.Set("Authorization", "Bearer "+c.token)`. (WeatherFlow's REST API accepts `Authorization: Bearer <PAT>` — design §20.) The `GetObservations` URL becomes `fmt.Sprintf("https://swd.weatherflow.com/swd/rest/observations/device/%d?time_start=%d&time_end=%d", station.deviceID, startAt.Unix(), endAt.Unix())`.
  - Use `c.http.Do(req)` (not `http.DefaultClient`).
  - After `Do`: `if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("weatherflow API status %d", resp.StatusCode) }` **before** decode.
  - Bound the body: `body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))` (10 MB).
  - After decode, check the API envelope: `if data.Status.StatusCode != 0 { return nil, fmt.Errorf("weatherflow status_code %d: %s", data.Status.StatusCode, data.Status.StatusMessage) }` (F-MEDIUM).
  - Replace `log.Fatalf("unhandled report type")` at `client.go:116` with `return nil, fmt.Errorf("unhandled report type %T", report)`.
  - The URL no longer contains the token, so a `*url.Error` from `Do` cannot leak it; do not log raw request URLs elsewhere.
- [ ] **Step 4:** Update `main.go` call sites for the pointer-receiver ripple: `client := tempestapi.NewClient(token)` now yields `*tempestapi.Client` (line ~168 in `exportWithSink`); the `client.ListStations`/`client.GetObservations` calls are unchanged (method set is identical on the pointer). Confirm `go build ./...` — no other call sites exist.
- [ ] **Step 5:** Run `go test -race ./internal/tempestapi/` → PASS; `go build ./...` clean.
- [ ] **Step 6:** Commit `fix: harden API client — bearer header, status check, timeout, no log.Fatalf (F-H1..H4)`.

**Acceptance:** `go test -race ./internal/tempestapi/` passes; `go build ./...` clean.
**Implements:** design §6; resolves **F-H1, F-H2, F-H3, F-H4, F-MEDIUM**.

### Task 0.6: Explicit boolean env parsing (A-MEDIUM silent-typo) — no deps

**Files:**
- Create: `internal/config/env.go`
- Test: `internal/config/env_test.go`
- Modify: `main.go` (replace `strconv.ParseBool(os.Getenv(...))` discards)

**Steps:**
- [ ] **Step 1 (RED):** Add `TestParseBoolEnv`: `ParseBoolEnv("KEY")` where the env is unset → `false, nil`; `"true"`/`"1"` → `true, nil`; `"yes"`/`"nonsense"` → `false, error` naming the key and value.
- [ ] **Step 2:** Run → FAIL (function missing).
- [ ] **Step 3 (GREEN):** Implement `func ParseBoolEnv(key string) (bool, error)` — empty ⇒ false; otherwise `strconv.ParseBool`, wrapping the error with the key.
- [ ] **Step 4:** In `main.go`, replace each discarded `strconv.ParseBool` with `config.ParseBoolEnv`, logging and treating a parse error as fatal-at-startup (fail fast, go-standards §15.3). Keep behavior identical for valid values.
- [ ] **Step 5:** Run `go test ./internal/config/`; `go build ./...` → clean.
- [ ] **Step 6:** Commit `fix: fail loud on unparseable boolean env vars (A-MEDIUM)`.

**Acceptance:** `go test ./internal/config/` passes; `go build ./...` clean.
**Implements:** design §6; resolves **A-MEDIUM** (silent-typo-disables-a-sink).

### Task 0.7: Rework the sink seam — `Close(ctx)`, panic recovery, error aggregation; update all writers — depends on: Task 0.2

**Files:**
- Modify: `internal/sink/sink.go` (interface + `MetricsSink`)
- Modify: `internal/postgres/writer.go` (implement new `Close(ctx)`)
- Modify: `internal/prometheus/writer.go`, `internal/prometheus/server.go` (implement new `Close(ctx)`)
- Modify: `main.go` (call `Close(cleanupCtx)`)
- Test: `internal/sink/sink_test.go`

> **Compile-breaking ripple — do it all in this one task:** changing `MetricsWriter.Close() error` → `Close(ctx context.Context) error` breaks every implementer. Update postgres AND prometheus AND server writers AND main.go in this task or `go build ./...` fails and acceptance cannot pass.

**Steps:**
- [ ] **Step 1 (RED):** Add to `internal/sink/sink_test.go`:
  - `TestSink_RecoversPanickingWriter`: a fake writer whose `WriteReport` panics; `SendReport` must not propagate the panic, must log/count it, and must still call the other (non-panicking) writer. Run with `-race`.
  - `TestSink_SendReportAggregatesErrors`: two fake writers both returning errors → `SendReport` returns a non-nil aggregated error (`errors.Join`); one failing + one ok → still non-nil naming the failure; all ok → nil.
  - `TestSink_ClosePassesContext`: a fake writer records the ctx passed to `Close`; assert it is the caller-supplied cleanup ctx (not a stored/cancelled one).
- [ ] **Step 2:** Run `go test ./internal/sink/` → FAIL.
- [ ] **Step 3 (GREEN):**
  - **Interface** (`internal/sink/sink.go`): change `Close() error` → `Close(ctx context.Context) error`. Drop the stored `ctx context.Context` field from `MetricsSink` and `NewMetricsSink` (Review A NIT / C-LOW) — keep `NewMetricsSink()` taking no ctx (update the `main.go` call). Switch the package's `log` import to `log/slog`.
  - **`SendReport` / `SendMetrics`** — the current fan-out swallows errors and returns nil. Replace with panic-recovery + mutex-guarded aggregation. `SendReport` becomes (SendMetrics is identical with `WriteMetrics(ctx, metrics)`):

```go
func (s *MetricsSink) SendReport(ctx context.Context, report tempestudp.Report) error {
	s.mu.RLock()
	writers := slices.Clone(s.writers)
	s.mu.RUnlock()

	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for _, w := range writers {
		wg.Add(1)
		go func(writer MetricsWriter) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("writer panic recovered", "panic", r, "writer", fmt.Sprintf("%T", writer))
					mu.Lock()
					errs = append(errs, fmt.Errorf("writer %T panicked: %v", writer, r))
					mu.Unlock()
				}
			}()
			if err := writer.WriteReport(ctx, report); err != nil {
				mu.Lock() // mutex guards the append — required, else -race flags the concurrent slice write
				errs = append(errs, fmt.Errorf("writer %T: %w", writer, err))
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return errors.Join(errs...) // nil when errs is empty
}
```

  - **`Close(ctx)`** fans out `Flush(ctx)` then `Close(ctx)` per writer using the **passed** ctx (never a stored one), aggregating errors the same way:

```go
func (s *MetricsSink) Close(ctx context.Context) error {
	s.mu.RLock()
	writers := slices.Clone(s.writers)
	s.mu.RUnlock()

	var (
		mu   sync.Mutex
		errs []error
		wg   sync.WaitGroup
	)
	for _, w := range writers {
		wg.Add(1)
		go func(writer MetricsWriter) {
			defer wg.Done()
			if err := writer.Flush(ctx); err != nil {
				mu.Lock(); errs = append(errs, err); mu.Unlock()
			}
			if err := writer.Close(ctx); err != nil {
				mu.Lock(); errs = append(errs, err); mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	return errors.Join(errs...)
}
```

  - **Implementer signature updates (compile ripple — all in this task):**
    - `internal/postgres/writer.go`: `func (w *PostgresWriter) Close(ctx context.Context) error` (body still `close`s the four batch channels + `wg.Wait()` + `pool.Close()` for now; the drain rework is Task 0.8, the gate is Task 0.9a).
    - `internal/prometheus/writer.go` and `internal/prometheus/server.go`: `func (...) Close(ctx context.Context) error` — accept and ignore `ctx` for now (`MetricsServer.Close` already builds its own 5s ctx internally; keep that, the param is unused until 0.9b). Real gate is Task 0.9b.
  - **`main.go`** (`main`, lines ~28-34): keep the ingest `signal.NotifyContext` (SIGTERM is added in Task 0.8), change the sink construction + deferred close:

```go
	metricsSink := sink.NewMetricsSink()
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := metricsSink.Close(cleanupCtx); err != nil {
			slog.Error("sink close", "err", err)
		}
	}()
```
- [ ] **Step 4:** Run `go test -race ./internal/sink/ ./internal/postgres/ ./internal/prometheus/`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `refactor: sink Close(ctx) + panic recovery + error aggregation (A-MEDIUM, A-LOW)`.

**Acceptance:** `go test -race ./internal/sink/...` passes; `go build ./...` clean.
**Interfaces produced:** `MetricsWriter.Close(ctx context.Context) error`, `MetricsSink.Close(ctx context.Context) error` — WS3/WS6 writers implement this signature.
**Implements:** design §6; resolves **A-MEDIUM** (panic recovery, cleanup ctx), **A-LOW** (dead nil returns).

### Task 0.8: SIGTERM + drain-on-shutdown for postgres writer (A-H1, cleanup ctx, C-H1) — depends on: Task 0.7

**Files:**
- Modify: `main.go` (`signal.NotifyContext`)
- Modify: `internal/postgres/writer.go` (drain the channel buffer under the cleanup ctx)
- Test: `internal/postgres/writer_test.go`

> **Test seam decision (resolves the cold-review ambiguity):** do NOT try to test `main` sending itself a signal. Instead extract `signalContext` with an **injectable `notify` function** so the test asserts the exact signal set passed to `signal.NotifyContext` without any real signals. This is the acceptance criterion for A-H1 (the design's #1 HIGH).

**Steps:**
- [ ] **Step 1 (RED):** Add `main_test.go`:

```go
func TestSignalContext_RegistersInterruptAndSIGTERM(t *testing.T) {
	var gotSigs []os.Signal
	fakeNotify := func(parent context.Context, sig ...os.Signal) (context.Context, context.CancelFunc) {
		gotSigs = sig
		return context.WithCancel(parent)
	}
	_, cancel := signalContext(context.Background(), fakeNotify)
	defer cancel()
	if !slices.Contains(gotSigs, os.Interrupt) || !slices.Contains(gotSigs, syscall.SIGTERM) {
		t.Fatalf("signalContext must register SIGINT+SIGTERM, got %v", gotSigs)
	}
}
```

  Also add `TestPostgresWriter_DrainOnClose` in `internal/postgres/writer_test.go`: enqueue N observation rows into `w.obsBatch`, call `Close(ctx)` with a fresh `context.Background()`-derived ctx, assert all N reach the insert path (buffered channel drained, not just a local slice). Because the current writer needs a live pool, extract the drain loop over an injectable `inserter` interface (`interface{ insertObservations(ctx, []observationRow) error }`) so a fake inserter counts rows; keep a `//go:build integration` real-DB variant that exercises the pool.
- [ ] **Step 2:** Run `go test ./ -run TestSignalContext` and `go test ./internal/postgres/ -run TestPostgresWriter_DrainOnClose` → FAIL (helper/behavior missing).
- [ ] **Step 3 (GREEN):**
  - **`main.go`** — extract the injectable helper and call it from `main`:

```go
// notifyFunc matches signal.NotifyContext so tests can inject a fake.
type notifyFunc func(parent context.Context, sig ...os.Signal) (context.Context, context.CancelFunc)

func signalContext(parent context.Context, notify notifyFunc) (context.Context, context.CancelFunc) {
	return notify(parent, os.Interrupt, syscall.SIGTERM)
}

func main() {
	ctx, done := signalContext(context.Background(), signal.NotifyContext)
	defer done()
	// ... rest of main unchanged (sink construction from Task 0.7) ...
}
```

  (Add `"syscall"` and `"slices"` to imports; `signal.NotifyContext` satisfies `notifyFunc` directly.) This replaces the current `signal.NotifyContext(context.Background(), os.Interrupt)` at `main.go:29` — SIGTERM is the fix for A-H1.
  - **`internal/postgres/writer.go`** — on `Close(ctx)`, **drain the full buffered channels and flush using the passed `ctx`**, not the constructor `w.ctx`. (The idempotent send-blocking `done` gate is added *next*, in Task 0.9a; in 0.8 the drain simply reads whatever is already buffered — do not reference a `done` field here, it does not exist yet.) Concretely, the batch workers' `case <-w.ctx.Done():` branches (e.g. `writer.go:162`, `:249`) currently flush only the local `batch` slice and return — change the shutdown path so `Close(ctx)` drains any rows still queued in `w.obsBatch`/`w.windBatch`/`w.hubBatch`/`w.eventBatch` before returning, e.g. a final non-blocking drain loop per channel that calls the batch insert with the cleanup `ctx`.
    **Signature ripple (explicit):** the four insert helpers `insertObservations`/`insertRapidWind`/`insertHubStatus`/`insertEvents` are currently param-less w.r.t. context (they derive a 5s timeout from `w.ctx` at `writer.go:179` et al., ~`writer.go:174/216/258/300`). Add an explicit `ctx context.Context` first parameter to **all four** and update their existing call sites in the batch workers to pass the worker/normal ctx, and the new drain path to pass the `Close` ctx. This is a mechanical but file-wide change within `internal/postgres/writer.go`; `go build ./internal/postgres/` must stay green after it.
- [ ] **Step 4:** Run `go test -race ./internal/postgres/`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `fix: SIGTERM + drain buffered postgres rows on shutdown (A-H1, C-H1)`.

**Acceptance:** `go test -race ./internal/postgres/` passes; `go build ./...` clean.
**Implements:** design §6; resolves **A-H1**, **C-H1** (context+drain).

> **Why 0.9 is split into 0.9a / 0.9b:** the two writers have **structurally different channel shapes** — postgres has FOUR producer channels (`obsBatch/windBatch/hubBatch/eventBatch`) whose workers close-signal by `close(channel)`; prometheus has ONE `outbox` chan plus a `more` signal chan and closes both in `Close`. The send-on-closed gate is the *same idea* but *different code* per writer. Do NOT "apply an identical pattern" — each subtask shows its own concrete code.

### Task 0.9a: Idempotent, panic-free `Close` for the **postgres** writer (C-H3, D-H1) — depends on: Task 0.8

> Sequenced after Task 0.8 because both touch the postgres writer's shutdown path: 0.8 adds the buffered-channel drain, 0.9a adds the `done` gate + idempotent `Close` around it. Doing them in one order avoids a merge conflict on `Close`.

**Files:**
- Modify: `internal/postgres/writer.go`
- Test: `internal/postgres/writer_test.go`

**Steps:**
- [ ] **Step 1 (RED):** In `internal/postgres/writer_test.go`:
  - `TestPostgresClose_Idempotent`: calling `Close(ctx)` twice does not panic and returns nil both times.
  - `TestPostgresWriteDuringClose_NoPanic`: spawn goroutines calling `WriteReport` (which sends into `obsBatch` etc.) while `Close(ctx)` runs; run under `-race`; assert no send-on-closed-channel panic and `Close` returns.
- [ ] **Step 2:** Run `go test -race ./internal/postgres/` → FAIL (double-close at `writer.go:857-860` / send-on-closed from the `default:` producer branches).
- [ ] **Step 3 (GREEN):** The current `Close` (`writer.go:855`) unconditionally `close()`s the four batch channels — a second `Close` double-closes (panic), and a producer's `select { case w.obsBatch <- row: ... default: ... }` (e.g. `writer.go:~545`) can send on an already-closed channel (panic). Add a `done` gate + `sync.Once`, stop closing the batch channels from `Close`, and make workers exit on `<-w.done`:

```go
// add to the PostgresWriter struct:
	done      chan struct{}
	closeOnce sync.Once

// in NewPostgresWriter, initialize:
	done: make(chan struct{}),

// every producer send changes from `default: log-drop` to gate on done, e.g.:
	select {
	case w.obsBatch <- row:
	case <-w.done: // Close in progress — stop producing, no send-on-closed
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}

// each batch worker's shutdown branch selects on w.done instead of a closed channel:
	case <-w.done:
		// drain whatever is buffered, then return (see Task 0.8 drain)
		return

// Close becomes idempotent and never closes the producer channels:
func (w *PostgresWriter) Close(ctx context.Context) error {
	w.closeOnce.Do(func() {
		close(w.done)      // single close — signals producers AND workers
		w.wg.Wait()        // workers drain (Task 0.8) then return
		w.pool.Close()
	})
	return nil
}
```

  Remove the four `close(w.xBatch)` calls. The batch channels are now never closed (GC reclaims them); `done` is the sole shutdown signal.
- [ ] **Step 4:** Run `go test -race ./internal/postgres/` → PASS.
- [ ] **Step 5:** Commit `fix: idempotent panic-free Close in postgres writer via done-gate (C-H3, D-H1)`.

**Acceptance:** `go test -race ./internal/postgres/` passes.
**Implements:** design §6; resolves **C-H3**, **D-H1** (postgres).

### Task 0.9b: Idempotent, panic-free `Close` for the **prometheus** writer (C-H3, D-H1) — depends on: Task 0.7

**Files:**
- Modify: `internal/prometheus/writer.go`
- Test: `internal/prometheus/writer_test.go`

**Steps:**
- [ ] **Step 1 (RED):** In `internal/prometheus/writer_test.go`:
  - `TestPrometheusClose_Idempotent`: calling `Close(ctx)` twice does not panic and returns nil both times.
  - `TestPrometheusWriteDuringClose_NoPanic`: spawn goroutines calling `WriteMetrics` (sends into `outbox`, signals `more`) while `Close(ctx)` runs; run under `-race`; assert no send-on-closed panic and `Close` returns.
- [ ] **Step 2:** Run `go test -race ./internal/prometheus/` → FAIL (double-close of `outbox`/`more` at `writer.go:91-92`; send-on-closed from `WriteMetrics` `writer.go:61` and `Flush`/`WriteMetrics` `more` sends).
- [ ] **Step 3 (GREEN):** Replace the close-the-channels shutdown with a `done` gate + `sync.Once`. The `pushWorker` currently loops `for range w.more`; change it to select on `done`. Concrete diff:

```go
// add to the PrometheusWriter struct:
	done      chan struct{}
	closeOnce sync.Once

// in NewPrometheusWriter, initialize:
	done: make(chan struct{}),

// WriteMetrics — the metric send and the `more` signal both gate on done:
func (w *PrometheusWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	for _, m := range metrics {
		select {
		case w.outbox <- m:
		case <-w.done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Printf("prometheus: outbox full, dropping metric")
		}
	}
	select {
	case w.more <- true:
	case <-w.done:
	default:
	}
	return nil
}

// pushWorker drains until done, then does one final drain:
func (w *PrometheusWriter) pushWorker() {
	defer w.wg.Done()
	for {
		select {
		case <-w.more:
			if err := w.pusher.Add(); err != nil {
				log.Printf("prometheus: push error: %v", err)
			}
		case <-w.done:
			_ = w.pusher.Add() // final flush of whatever the collector can drain
			return
		}
	}
}

// Close is idempotent and closes NO producer channel:
func (w *PrometheusWriter) Close(ctx context.Context) error {
	w.closeOnce.Do(func() {
		close(w.done)
		w.wg.Wait()
	})
	log.Printf("prometheus: closed")
	return nil
}
```

  Remove `close(w.outbox)` and `close(w.more)`. `Flush` keeps its non-blocking `more` send but also gates on `done` (mirror the `select` above).
- [ ] **Step 4:** Run `go test -race ./internal/prometheus/` → PASS.
- [ ] **Step 5:** Commit `fix: idempotent panic-free Close in prometheus writer via done-gate (C-H3, D-H1)`.

**Acceptance:** `go test -race ./internal/prometheus/` passes.
**Implements:** design §6; resolves **C-H3**, **D-H1** (prometheus).

### Task 0.10: Metrics server returns bind errors synchronously (D-H2, A-H3) — depends on: Task 0.7

**Files:**
- Modify: `internal/prometheus/server.go` (`MetricsServer.Start`)
- Test: `internal/prometheus/server_test.go`

**Steps:**
- [ ] **Step 1 (RED):** Add `TestMetricsServer_StartReturnsBindError`: start one server on a port, then start a second `MetricsServer` on the same port → `Start()` returns a non-nil error (currently always nil).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** In `Start()`, call `net.Listen("tcp", addr)` synchronously; on error return it; on success `go srv.Serve(ln)`. Add `ReadHeaderTimeout`/`IdleTimeout` to the `http.Server` (Global Constraints; also review LOW).
- [ ] **Step 4:** Run `go test -race ./internal/prometheus/` → PASS. Confirm `main.go`'s existing `if err := metricsServer.Start(); err != nil` is now live.
- [ ] **Step 5:** Commit `fix: metrics server surfaces bind errors synchronously (D-H2, A-H3)`.

**Acceptance:** `go test -race ./internal/prometheus/` passes.
**Implements:** design §6; resolves **D-H2**, **A-H3**.

### Task 0.11: Fix API-export "at least one writer" invariant + gzip `O_TRUNC` (A-H2, A-MEDIUM) — depends on: Task 0.6

**Files:**
- Modify: `main.go` (writer-count check; `writeMetricsToFile` open flags)
- Test: `main_test.go` (create)

**Steps:**
- [ ] **Step 1 (RED):** Add to `main_test.go`:

```go
func TestRequireWriters(t *testing.T) {
	tests := []struct {
		name        string
		mode        Mode
		writerCount int
		keepFiles   bool
		wantErr     bool
	}{
		{"udp no writers", ModeUDP, 0, false, true},
		{"udp one writer", ModeUDP, 1, false, false},
		{"api no writers no files", ModeAPIExport, 0, false, true},
		{"api no writers keep files", ModeAPIExport, 0, true, false},
		{"api db writer", ModeAPIExport, 1, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireWriters(tc.mode, tc.writerCount, tc.keepFiles)
			if (err != nil) != tc.wantErr {
				t.Fatalf("requireWriters(%v,%d,%v) err=%v want err=%v", tc.mode, tc.writerCount, tc.keepFiles, err, tc.wantErr)
			}
		})
	}
}
```

- [ ] **Step 2:** Run `go test ./ -run TestRequireWriters` → FAIL (`Mode`/`requireWriters` undefined).
- [ ] **Step 3 (GREEN):** Add the `Mode` type and helper to `main.go`, and call it in place of the current `if metricsSink.WriterCount() == 0` block (`main.go:86-88`):

```go
type Mode int

const (
	ModeUDP Mode = iota // UDP listener (no TOKEN)
	ModeAPIExport       // historical export (TOKEN set)
)

// requireWriters enforces the "at least one writer" invariant, but only where
// it applies: UDP mode always needs a writer; API-export mode is satisfied by a
// DB writer OR KEEP_EXPORT_FILES (fixes A-H2 — gzip-only export was unreachable).
func requireWriters(mode Mode, writerCount int, keepFiles bool) error {
	if writerCount > 0 {
		return nil
	}
	if mode == ModeAPIExport && keepFiles {
		return nil
	}
	return fmt.Errorf("no writers configured: set ENABLE_POSTGRES / ENABLE_OTEL / ENABLE_PROMETHEUS_* (or KEEP_EXPORT_FILES in API-export mode)")
}
```

  In `main`, compute `mode` from `token` (`ModeAPIExport` when `token != ""`, else `ModeUDP`), read `keepFiles` via `config.ParseBoolEnv("KEEP_EXPORT_FILES")` (from Task 0.6), and `if err := requireWriters(mode, metricsSink.WriterCount(), keepFiles); err != nil { log.Fatal(err) }`. Also add `os.O_TRUNC` to the gzip open flags at `main.go:243`: `os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)` (A-MEDIUM corrupt-overwrite when a shorter export overwrites a longer file).
- [ ] **Step 4:** Run `go test ./... -run TestWriterInvariant`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `fix: reachable gzip-only export mode + O_TRUNC (A-H2, A-MEDIUM)`.

**Acceptance:** `go test ./ -run TestWriterInvariant` passes; `go build ./...` clean.
**Implements:** design §6/§16; resolves **A-H2**, **A-MEDIUM** (file truncation).

### Task 0.12: CI/supply-chain fixes (G-H1..H3) — no deps

**Files:**
- Modify: `.github/actions/docker/action.yml` (undeclared `inputs.push`)
- Modify: `.github/workflows/*.yml` (add `permissions:` blocks; pin actions to SHAs; align Go to 1.24)
- Create: `.goreleaser.yaml`
- Modify: `.dockerignore` (add local binary + `docs/`)

**Steps:**
- [ ] **Step 1:** Fix the input mismatch. Verified against the tree: `.github/actions/docker/action.yml` declares only `inputs.token` but its last step uses `push: ${{ fromJSON(inputs.push) }}`, while `.github/workflows/on-push-main.yml` passes `latest: true`, `push: true`, `tag-strategy: "latest"` — three undeclared inputs. Add the missing declarations to `action.yml`'s `inputs:` block so the caller's inputs resolve:

```yaml
inputs:
  token:
    description: Github token
    required: true
  push:
    description: Whether to push the built image
    required: false
    default: "false"
  latest:
    description: Also tag the image as latest
    required: false
    default: "false"
  tag-strategy:
    description: Tag strategy (e.g. latest, semver)
    required: false
    default: "latest"
```

  (`fromJSON("true")`/`fromJSON("false")` yields the boolean the `if:`/`push:` fields need.) Confirm `on-release.yml` and `on-pull-request.yml` callers pass a matching subset.
- [ ] **Step 2:** Add least-privilege `permissions:` to every workflow; pin third-party actions to commit SHAs; set CI Go to `1.24`; commit a minimal `.goreleaser.yaml`; add `actionlint` to CI.
- [ ] **Step 3 (verify):** `actionlint` clean; `docker buildx bake image-local` still builds (Task depends on Dockerfile as-is; WS1 rewrites it later).
- [ ] **Step 4:** Commit `ci: fix docker action inputs, add permissions + goreleaser (G-H1..H3)`.

**Acceptance:** `actionlint` reports no errors; existing image build succeeds.
**Implements:** design §15; resolves **G-H1, G-H2 (partial — README in Task DOC.1), G-H3**.

### Task 0.13: Wire the legacy Postgres tunables from env (C-H2 for the Postgres path) — no deps

> **User decision B1.** `POSTGRES_BATCH_SIZE` / `POSTGRES_FLUSH_INTERVAL` / `POSTGRES_MAX_RETRIES` are documented (CLAUDE.md) but currently **hardcoded and inert** in `NewPostgresWriter` (`writer.go:133-135` — `batchSize:100, flushInterval:10*time.Second, maxRetries:3`). This is C-H2 for the legacy Postgres writer (the design previously applied the C-H2 *lesson* only to the new SQLite store). Wire them so a set env value takes effect.

**Files:**
- Create: `internal/postgres/tunables.go`
- Modify: `internal/postgres/writer.go` (`NewPostgresWriter` reads tunables)
- Test: `internal/postgres/tunables_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `internal/postgres/tunables_test.go`:

```go
func TestPostgresTunables(t *testing.T) {
	getenv := func(k string) string {
		return map[string]string{
			"POSTGRES_BATCH_SIZE":     "7",
			"POSTGRES_FLUSH_INTERVAL": "2s",
			"POSTGRES_MAX_RETRIES":    "5",
		}[k]
	}
	tn := postgresTunables(getenv)
	if tn.batchSize != 7 || tn.flushInterval != 2*time.Second || tn.maxRetries != 5 {
		t.Fatalf("tunables not applied: %+v", tn)
	}
	// defaults when unset:
	def := postgresTunables(func(string) string { return "" })
	if def.batchSize != 100 || def.flushInterval != 10*time.Second || def.maxRetries != 3 {
		t.Fatalf("defaults wrong: %+v", def)
	}
}
```

- [ ] **Step 2:** Run `go test ./internal/postgres/ -run TestPostgresTunables` → FAIL (`postgresTunables` undefined).
- [ ] **Step 3 (GREEN):** Add the pure helper (testable without a live DB), then call it in `NewPostgresWriter`:

```go
type tunables struct {
	batchSize     int
	flushInterval time.Duration
	maxRetries    int
}

func postgresTunables(getenv func(string) string) tunables {
	atoiOr := func(s string, def int) int {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
		return def
	}
	durOr := func(s string, def time.Duration) time.Duration {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
		return def
	}
	return tunables{
		batchSize:     atoiOr(getenv("POSTGRES_BATCH_SIZE"), 100),
		flushInterval: durOr(getenv("POSTGRES_FLUSH_INTERVAL"), 10*time.Second),
		maxRetries:    atoiOr(getenv("POSTGRES_MAX_RETRIES"), 3),
	}
}
```

  In `NewPostgresWriter`, replace the hardcoded literals with `tn := postgresTunables(os.Getenv)` and set `batchSize: tn.batchSize, flushInterval: tn.flushInterval, maxRetries: tn.maxRetries`.
- [ ] **Step 4:** Run `go test ./internal/postgres/ -run TestPostgresTunables`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `fix: honor POSTGRES_BATCH_SIZE/FLUSH_INTERVAL/MAX_RETRIES env (C-H2, B1)`.

**Acceptance:** `go test ./internal/postgres/ -run TestPostgresTunables` passes; `go build ./...` clean.
**Implements:** design §6/§16; resolves **C-H2** for the Postgres path (B1).

---

## Workstream 3 — SQLite + Litestream (default store)

> Design §10, §12. Depends on WS0 (needs `Close(ctx)` + drain pattern). The UI (WS1) reads this DB, so it lands before WS1.

### Task 3.1: SQLite schema + migrations package (embedded DDL, `schema_version`) — depends on: Task 0.7

**Files:**
- Create: `internal/sqlite/schema.go`, `internal/sqlite/migrations/0001_init.sql`
- Test: `internal/sqlite/schema_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestMigrate_CreatesTablesAndVersion`: open a temp `modernc.org/sqlite` DB (`t.TempDir()`), run `Migrate(ctx, db)`, assert the four tables + index exist (query `sqlite_master`) and `schema_version` == 1; run `Migrate` again → idempotent, still version 1.
- [ ] **Step 2:** Run `go test ./internal/sqlite/` → FAIL (package/functions missing). First `go get modernc.org/sqlite@v1.34.4` + `go mod tidy`.
- [ ] **Step 3 (GREEN):** `//go:embed migrations/*.sql` the DDL (exactly the design §12 DDL — `tempest_observations`, `tempest_rapid_wind`, `tempest_hub_status`, `tempest_events`, `idx_obs_serial_time`, `schema_version`; integer counts as `INTEGER` not float — fixes B-LOW). Implement `Migrate(ctx, *sql.DB) error` applying unapplied versions in a transaction and recording `schema_version`.
- [ ] **Step 4:** Run `go test ./internal/sqlite/` → PASS.
- [ ] **Step 5:** Commit `feat: sqlite schema + embedded migrations (B-MEDIUM)`.

**Acceptance:** `go test ./internal/sqlite/` passes; `go build ./...` clean.
**Interfaces produced:** `sqlite.Migrate(ctx, *sql.DB) error`; table/column names per design §12.
**Implements:** design §10/§12; resolves **B-MEDIUM** (migration path), **B-LOW** (integer types).

### Task 3.2: SQLite connection + PRAGMAs — depends on: Task 3.1

**Files:**
- Create: `internal/sqlite/db.go`
- Test: `internal/sqlite/db_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestOpen_SetsPragmas`: `Open(ctx, path, cfg)` then query `PRAGMA journal_mode` == `wal`, `PRAGMA busy_timeout` == `5000`, `PRAGMA synchronous` == `1` (NORMAL), `PRAGMA foreign_keys` == `1`. Assert `wal_autocheckpoint` is left at the SQLite default (do NOT set it aggressively — Litestream owns checkpointing).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** `func Open(ctx context.Context, path string, cfg Config) (*sql.DB, error)` using driver name `"sqlite"` (modernc registers under that name). **Confirmed modernc DSN pragma syntax** (Context7, `gitlab.com/cznic/sqlite`): each pragma is a separate `_pragma=` query param, value in `name(value)` form, joined by `&`. Build the DSN as:

```go
const driverName = "sqlite" // modernc.org/sqlite registers this name

func Open(ctx context.Context, path string, cfg Config) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(%d)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)",
		path, cfg.BusyTimeout.Milliseconds(),
	)
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // single writer — serializes writes, avoids SQLITE_BUSY (design §10)
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := Migrate(ctx, db); err != nil {
		return nil, err
	}
	return db, nil
}
```

  Note `foreign_keys(1)` (not `ON`) per the confirmed modernc example, and **do NOT** append a `_pragma=wal_autocheckpoint(...)` — leave it at the SQLite default so Litestream owns checkpointing (design §10). `Config` carries `BatchSize`, `FlushInterval`, `BusyTimeout` from env (`SQLITE_BATCH_SIZE` default 100, `SQLITE_FLUSH_INTERVAL` default 10s, `SQLITE_BUSY_TIMEOUT` default 5000ms) via `cmp.Or`/parse-with-default.
- [ ] **Step 4:** Run `go test ./internal/sqlite/` → PASS.
- [ ] **Step 5:** Commit `feat: sqlite Open with exact PRAGMAs (design §10)`.

**Acceptance:** `go test ./internal/sqlite/` passes.
**Interfaces produced:** `sqlite.Open(ctx, path, cfg)`, `sqlite.Config{BatchSize, FlushInterval, BusyTimeout}`.
**Implements:** design §10 (PRAGMAs, Litestream-owned checkpointing, honored tunables — avoids C-H2).

### Task 3.3: SQLite writer — single serialized goroutine, `ON CONFLICT DO NOTHING`, UUIDv7 — depends on: Task 3.2

**Files:**
- Create: `internal/sqlite/writer.go`
- Test: `internal/sqlite/writer_test.go`

**Steps:**
- [ ] **Step 1 (RED):** Against a real temp DB:
  - `TestWriter_InsertsObservation`: `WriteReport` an observation → row present with exact column values (field-by-field), UUIDv7 PK.
  - `TestWriter_OnConflictIdempotent`: write the same `(serial, timestamp)` twice → exactly one row (`ON CONFLICT DO NOTHING`).
  - `TestWriter_RoutesReportTypes`: obs → observations, rapid_wind → rapid_wind, hub_status → hub_status, precip/strike events → events.
  - `TestWriter_EventsNotDroppedUnderBackpressure`: fill the channel to capacity, then submit a discrete lightning/rain-start row; assert it is **not** silently dropped — it blocks up to `eventBlockTimeout` and succeeds once the writer drains (avoids C-MEDIUM). Assert the pinned constants exist: `rowChanCap == 1000`, `eventBlockTimeout == 5*time.Second`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Implement `Writer` satisfying `sink.MetricsWriter`. **Pinned constants** (asserted in RED):

```go
const (
	// rowChanCap bounds the single writer's inbound queue. 1000 mirrors the
	// postgres writer's per-channel buffer — at ~1 obs/min plus 3s rapid-wind,
	// 1000 is minutes of headroom before backpressure engages.
	rowChanCap = 1000
	// eventBlockTimeout is how long a DISCRETE event (lightning/rain-start)
	// will block when the channel is full before it is logged-and-dropped.
	// Chosen shorter than the 30s shutdown budget yet long enough for the
	// single writer to complete at least one batch flush; a discrete event is
	// rare, so blocking briefly is correct, not silent loss.
	eventBlockTimeout = 5 * time.Second
)
```

  Structure: a **single** writer goroutine consumes one buffered `chan rowEnvelope` (cap `rowChanCap`) and batch-inserts per `BatchSize`/`FlushInterval`. Continuous rows (observation/rapid_wind/hub) use a non-blocking send with `done`-gate (WS0 pattern); **discrete events** use a bounded-block send so they are not dropped:

```go
// continuous rows — non-blocking, drop-with-log only when truly saturated:
func (w *Writer) enqueue(ctx context.Context, r rowEnvelope) error {
	select {
	case w.rows <- r:
		return nil
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		slog.Warn("sqlite: row channel full, dropping continuous row", "kind", r.kind)
		return nil
	}
}

// discrete events (lightning, rain-start) — block up to eventBlockTimeout:
func (w *Writer) enqueueEvent(ctx context.Context, r rowEnvelope) error {
	t := time.NewTimer(eventBlockTimeout)
	defer t.Stop()
	select {
	case w.rows <- r:
		return nil
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		slog.Warn("sqlite: event channel full after block timeout, dropping event", "kind", r.kind)
		return nil
	}
}
```

  Generate one UUIDv7 per row (`uuid.Must(uuid.NewV7())`). Parameterized inserts, `INSERT ... ON CONFLICT DO NOTHING` on the `(serial_number, timestamp)` unique key (events on `(serial_number, timestamp, event_type)`). `sync.Once`+`done` gate for `Close` (mirror Task 0.9a's postgres shape — never close `w.rows`). Column mappings mirror the postgres writer's verified field indices (`internal/postgres/writer.go` `handleObservationReport` at `writer.go:453` — `ob[0]`=timestamp, `ob[1]`=windLull, `ob[2]`=windAvg, `ob[3]`=windGust, `ob[4]`=windDirection, `ob[5]`=windSampleInterval, `ob[6]`=pressure, `ob[7]`=tempAir, `ob[8]`=humidity, `ob[9]`=illuminance, `ob[10]`=uvIndex, `ob[11]`=irradiance, `ob[12]`=rainRate, `ob[13]`=precipType, `ob[14]`=lightningDistance, `ob[15]`=lightningStrikeCount, `ob[16]`=battery, `ob[17]`=reportInterval; wet-bulb via `tempestudp.WetBulbTemperatureC(ob[7], ob[8], ob[6])`). Timestamps stored as unix-epoch INTEGER (design §12).
- [ ] **Step 4:** Run `go test -race ./internal/sqlite/` → PASS.
- [ ] **Step 5:** Commit `feat: sqlite writer (single-writer, idempotent, backpressure-safe)`.

**Acceptance:** `go test -race ./internal/sqlite/` passes.
**Interfaces produced:** `sqlite.NewWriter(ctx, db, cfg) *Writer` implementing `sink.MetricsWriter`.
**Implements:** design §10; applies lessons of **C-H1/C-H3/C-MEDIUM**.

### Task 3.4: SQLite drain-on-Close test + history query method — depends on: Task 3.3

**Files:**
- Modify: `internal/sqlite/writer.go` (add `LatestObservation`, `HistoryPoints` read methods)
- Test: `internal/sqlite/writer_test.go`

**Steps:**
- [ ] **Step 1 (RED):**
  - `TestWriter_DrainOnClose`: enqueue N reports, `Close(ctx)` with fresh ctx, reopen DB, assert all N persisted.
  - `TestReader_LatestObservation`: insert 3 obs, `LatestObservation(ctx, serial)` returns the newest.
  - `TestReader_HistoryPoints`: `HistoryPoints(ctx, field, from, to)` returns `[]Point{T,V}` in range for a field (e.g. `temp_air`), with an allowlist of queryable field names (no SQL injection from `field`).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Add drain in `Close(ctx)`; add read methods. `field` is validated against a static allowlist mapping API field → column.
- [ ] **Step 4:** Run `go test -race ./internal/sqlite/` → PASS.
- [ ] **Step 5:** Commit `feat: sqlite drain-on-close + read methods for the JSON API`.

**Acceptance:** `go test -race ./internal/sqlite/` passes.
**Interfaces produced:** `(*Writer).LatestObservation(ctx, serial)`, `(*Writer).HistoryPoints(ctx, field, from, to)` — consumed by Contract C handlers (WS1).
**Implements:** design §10/§11; resolves class of **C-H1**.

### Task 3.5: Wire SQLite as default store in `main.go` — depends on: Task 3.4, Task 0.11

**Files:**
- Modify: `main.go`
- Test: `main_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestSelectStore` in `main_test.go`:

```go
func TestSelectStore(t *testing.T) {
	tests := []struct {
		name         string
		enablePG     bool
		sqlitePath   string
		wantPostgres bool
		wantSQLite   bool
		wantPath     string
	}{
		{"default sqlite", false, "", false, true, "/data/tempest.db"},
		{"postgres only", true, "", true, false, ""},
		{"both fan-out", true, "/tmp/x.db", true, true, "/tmp/x.db"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := selectStore(tc.enablePG, tc.sqlitePath)
			if c.postgres != tc.wantPostgres || c.sqlite != tc.wantSQLite {
				t.Fatalf("got %+v", c)
			}
			if tc.wantSQLite && c.sqlitePath != tc.wantPath {
				t.Fatalf("path %q want %q", c.sqlitePath, tc.wantPath)
			}
		})
	}
}
```

- [ ] **Step 2:** Run `go test ./ -run TestSelectStore` → FAIL (`selectStore`/`storeChoice` undefined).
- [ ] **Step 3 (GREEN):** Add the pure decision helper to `main.go` and use it to register writers:

```go
type storeChoice struct {
	postgres   bool
	sqlite     bool
	sqlitePath string
}

// selectStore: SQLite is the default store (R2); Postgres is opt-in via
// ENABLE_POSTGRES. Both may run concurrently (fan-out). SQLite is disabled only
// when Postgres is the sole configured store AND no SQLITE_PATH override is set.
func selectStore(enablePostgres bool, sqlitePathEnv string) storeChoice {
	c := storeChoice{postgres: enablePostgres}
	if !enablePostgres || sqlitePathEnv != "" {
		c.sqlite = true
		c.sqlitePath = cmp.Or(sqlitePathEnv, "/data/tempest.db")
	}
	return c
}
```

  In `main`, call `choice := selectStore(enablePostgres, os.Getenv("SQLITE_PATH"))`; when `choice.sqlite`, `sqlite.Open` + register `sqlite.NewWriter`; when `choice.postgres`, register the postgres writer (existing path). Both register → fan-out. The SQLite writer/reader handle is retained for the WS1 observation handlers.
- [ ] **Step 4:** Run `go test ./ -run TestSelectStore`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `feat: default to sqlite store, postgres opt-in (R2)`.

**Acceptance:** `go test ./ -run TestSelectStore` passes; `go build ./...` clean.
**Implements:** design §10 (R2).

### Task 3.6: Litestream replicate+restore round-trip test (default: filesystem replica) — depends on: Task 3.5

> **Replica choice is concrete (resolves the cold-review ambiguity): the DEFAULT test uses a Litestream `file:` replica to a `t.TempDir()` path — NOT integration-gated — so the PRAGMA/WAL/restore contract is proven on every `go test` run** (design §10 names "misconfigured PRAGMAs silently lose Litestream data" as the top risk; a routinely-skipped integration test would leave it unproven). The test requires the `litestream` binary on PATH; if absent, `t.Skip` with a clear message (CI installs it). A separate `//go:build integration` MinIO/S3 variant is optional and exercises the object-storage path.

**Files:**
- Create: `internal/sqlite/litestream_test.go` (default build, filesystem replica)
- Create: `internal/sqlite/litestream_integration_test.go` (`//go:build integration`, MinIO/S3 — optional)

**Steps:**
- [ ] **Step 1 (RED):** `TestLitestreamRestore_FileReplica` (default build):
  - open a SQLite DB at `db := t.TempDir()+"/tempest.db"` via `sqlite.Open`, write N observation rows through the WS3 writer, `Close`.
  - write a Litestream config with a `file:` replica pointing at `replica := t.TempDir()+"/replica"`; run `litestream replicate -config <cfg>` briefly (or `litestream replicate -exec` / a one-shot `litestream snapshot`), then `litestream restore -config <cfg> -o <fresh.db> <db>`.
  - open `fresh.db`, assert the N rows match the original field-for-field.
  - Guard with `if _, err := exec.LookPath("litestream"); err != nil { t.Skip("litestream not installed") }`.
- [ ] **Step 2:** Run `go test ./internal/sqlite/ -run TestLitestreamRestore_FileReplica` → FAIL (harness missing) or SKIP (binary absent).
- [ ] **Step 3 (GREEN):** Implement the harness: template the Litestream YAML config (`dbs: [{path: <db>, replicas: [{type: file, path: <replica>}]}]`), spawn `litestream` via `os/exec` with the test's context, assert restore equality. Keep the optional `//go:build integration` MinIO variant thin (swap the replica block for `type: s3` + testcontainer endpoint).
- [ ] **Step 4:** Run `go test ./internal/sqlite/ -run TestLitestreamRestore_FileReplica` (litestream installed) → PASS.
- [ ] **Step 5:** Commit `test: litestream file-replica replicate+restore round-trip (default) + optional S3 (integration)`.

**Acceptance:** `go test ./internal/sqlite/ -run TestLitestreamRestore_FileReplica` passes locally with `litestream` installed (skips cleanly without it).
**Implements:** design §10/§18 (restore-from-replica test proven on every run; PRAGMA-safety guard).

---

## Workstream 6 — Unified OpenTelemetry (OTLP) backbone

> Design §8, §12. Depends on WS0. WS4 (Grafana) depends on this. **Confirm all OTel APIs via Context7 before coding — logs signal is experimental.**

### Task 6.1: `internal/otel` setup — providers + OTLP exporters + resource — depends on: Task 0.7

**Files:**
- Create: `internal/otel/setup.go`
- Test: `internal/otel/setup_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestSetup_ReturnsShutdown`: `Setup(ctx, cfg)` with a fake/no-op endpoint returns a non-nil `shutdown func(context.Context) error` and no error; calling `shutdown(ctx)` is clean and idempotent. `TestResourceAttributes`: the built `Resource` carries `service.name=tempestwx`, `service.version`, `tempest.serial`.
- [ ] **Step 2:** Run → FAIL. First `go get` the OTel modules (versions in Dependencies table) + `go mod tidy`.
- [ ] **Step 3 (GREEN):** `func Setup(ctx context.Context, cfg Config) (func(context.Context) error, error)` building a `Resource`, `MeterProvider`, `TracerProvider`, and `LoggerProvider`, each with an OTLP gRPC exporter → `OTEL_EXPORTER_OTLP_ENDPOINT`. Keep the log signal isolated in this file (No-Wall — an API bump is one-file). Return a `shutdown` that flushes+stops all three.
- [ ] **Step 4:** Run `go test ./internal/otel/` → PASS.
- [ ] **Step 5:** Commit `feat: internal/otel setup — meter/tracer/logger providers + OTLP (R1)`.

**Acceptance:** `go test ./internal/otel/` passes; `go build ./...` clean.
**Interfaces produced:** `otel.Setup(ctx, cfg) (shutdown, error)`, `otel.Config{Endpoint, ServiceVersion}`.
**Implements:** design §8 (R1).

### Task 6.2: OTel sink writer — instruments per Contract B — depends on: Task 6.1

**Files:**
- Create: `internal/otel/writer.go`
- Test: `internal/otel/writer_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestWriter_RecordsInstruments`: use an in-memory metric reader (`sdk/metric` manual reader / `metricdata`); `WriteReport` an observation → assert the recorded instrument names/kinds/values EXACTLY match Contract B (e.g. `tempest.temperature.c` Gauge with `kind="air"`, `tempest.wind.meters_per_second` Gauge, `tempest.uptime.seconds` Gauge, `tempest.reboots` Counter). Assert wet-bulb NaN is skipped. Assert `serial` is an attribute, not `instance`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Implement `otel.Writer` satisfying `sink.MetricsWriter`; pre-register every instrument in Contract B; on `WriteReport` record each field to its instrument with the `serial`/`kind` attributes; derived dewpoint/heat-index/wetbulb as Gauges. `WriteMetrics` maps the API-mode prom metrics similarly. `Close(ctx)`/`Flush(ctx)` no-op (provider shutdown handled by Setup).
- [ ] **Step 4:** Run `go test -race ./internal/otel/` → PASS.
- [ ] **Step 5:** Commit `feat: otel sink writer with tempest_* instrument names (D-MEDIUM hygiene)`.

**Acceptance:** `go test -race ./internal/otel/` passes.
**Interfaces produced:** `otel.NewWriter(mp) *Writer` implementing `sink.MetricsWriter`.
**Implements:** design §8/§12; resolves **D-MEDIUM** metric-hygiene set (instance→serial, `_ms`, `_total`, dead RainTotal).

### Task 6.3: Tracing spans around ingest + HTTP — depends on: Task 6.2

**Files:**
- Modify: `main.go` (ingest spans), `internal/otel/tracing.go` (helpers)
- Test: `internal/otel/tracing_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestSpans_UDPIngestChain`: with a `tracetest.SpanRecorder`, run a report through the traced ingest path; assert spans `udp.receive` → `report.parse` → `sink.write` exist with parent/child links.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Add span helpers; wrap UDP ingest and the API export loop. (HTTP `otelhttp` middleware is added in WS1 where the server lives.)
- [ ] **Step 4:** Run `go test ./internal/otel/` → PASS; `go build ./...` clean.
- [ ] **Step 5:** Commit `feat: tracing spans for udp ingest + export loop`.

**Acceptance:** `go test ./internal/otel/` passes.
**Implements:** design §8/§12 (spans).

### Task 6.4: slog→OTel log bridge + wire OTel into main — depends on: Task 6.3, Task 3.5

**Files:**
- Modify: `main.go` (call `otel.Setup`, register writer, install log bridge, defer shutdown)
- Create: `internal/otel/logbridge.go`
- Test: `internal/otel/logbridge_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestLogBridge_EmitsRecords`: emit a `slog` record through the bridge handler → assert it reaches an in-memory OTel log exporter with the message + attributes (and trace/span IDs when a span is active).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Use the **official OTel slog bridge** rather than hand-writing a handler: `go.opentelemetry.io/contrib/bridges/otelslog` provides `otelslog.NewHandler(name string, opts ...Option) *otelslog.Handler` that emits `slog.Record`s to a `LoggerProvider` (pass `otelslog.WithLoggerProvider(lp)`); it carries the active span's trace/span IDs automatically. `internal/otel/logbridge.go` is a thin wrapper: `func NewSlogHandler(lp log.LoggerProvider) slog.Handler { return otelslog.NewHandler("tempestwx", otelslog.WithLoggerProvider(lp)) }` (keep it one file so an experimental-API bump is contained — No-Wall). Add `go.opentelemetry.io/contrib/bridges/otelslog` to the deps table (`v0.8.0`). In `main.go`: when `ENABLE_OTEL=true` + `OTEL_EXPORTER_OTLP_ENDPOINT` set, call `otel.Setup`, register `otel.NewWriter`, `slog.SetDefault(slog.New(otel.NewSlogHandler(lp)))`, and `defer shutdown(cleanupCtx)`. Confirm the `otelslog`/`log` API via Context7 before coding (experimental).
- [ ] **Step 4:** Run `go test ./internal/otel/`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `feat: slog→OTel log bridge + wire OTel sink (ENABLE_OTEL)`.

**Acceptance:** `go test ./internal/otel/` passes; `go build ./...` clean.
**Implements:** design §8 (structured logs with trace correlation).

### Task 6.5: Deprecate the bespoke Prometheus path (deprecate-then-remove, O4) — depends on: Task 6.4

**Files:**
- Modify: `internal/prometheus/writer.go`, `internal/prometheus/server.go`, `main.go`

**Steps:**
- [ ] **Step 1:** Add a one-time `slog.Warn("ENABLE_PROMETHEUS_* is deprecated; migrate to ENABLE_OTEL — removal in the next release")` when either prometheus writer is registered. (No behavior change — kept one release per O4.)
- [ ] **Step 2 (verify):** `go build ./...` clean; existing prometheus tests still pass.
- [ ] **Step 3:** Commit `chore: deprecation warning on bespoke prometheus path (O4)`.

**Acceptance:** `go build ./...` clean; `go test ./internal/prometheus/` passes.
**Implements:** design §8/§16 (O4 deprecate-then-remove).

---

## Workstream 1 — Embedded UI + backend JSON API (folds in WS5 UX P0/P1)

> Design §7, §11, §14. Depends on WS3 (data) + WS0 (token proxy). UX P0/P1 items (design §14) are folded into these tasks as noted; P2 polish is Task UX.1 at the end.

### Task 1.1: Vendor the `tempest-display` UI into `web/` + build wiring + emit UI manifest — depends on: Task 0.1

> **UI manifest (below) is authoritative for all downstream UI tasks (1.2, 1.3, 1.4, 1.7a–c, 2.6, UX.1).** It is captured here from the pinned upstream commit `49892063` so those tasks reference **present, real symbol names** instead of forward-referencing an external repo. Verify the tree matches after the clone; if upstream differs, STOP and reconcile before proceeding.

**Files:**
- Create: `web/` (vendored from `tempest-display@49892063`, retaining `LICENSE` + provenance header), `web/dist/.gitkeep`, `web/PROVENANCE.md`
- Modify: `taskfile.yml` (add `ui-build` target)

**UI manifest — file tree under `web/` after vendoring (upstream `server/` removed):**

```
web/
  index.html
  package.json           # React 19 + TS 5.9 + Vite 7; add: maplibre-gl, pmtiles, vitest, @testing-library/react
  vite.config.ts
  eslint.config.js
  tsconfig.json  tsconfig.app.json  tsconfig.node.json
  public/vite.svg
  src/
    main.tsx
    App.tsx                       # top-level dashboard; wrap in ErrorBoundary (1.7b), mount RadarCard (2.6)
    App.css  index.css            # missing .loading-screen/.loading-spinner/.error-screen/.glass-btn defined in 1.7b
    api/
      tempestApi.ts               # STUB client — fetchStationMeta, fetchCurrentObservation, fetchForecast,
                                   #   fetchHourlyForecast, fetchStationStatus, fetchStationAlmanac,
                                   #   setApiToken/getApiToken, connectWebSocket (replace stubs → real /api/* in 1.7a)
      stubData.ts                 # deleted in 1.7a
    hooks/
      useUnits.ts                 # convertTemp/convertWind/convertPressure/convertRain + format* + useUnits() (CORRECT — cover in 1.2)
      useWeatherData.ts           # useWeatherData(stationId?) → WeatherData; add AbortController in 1.7a
    components/
      AlmanacCard.tsx             # MoonPhase(), formatTime(), RecordColumn(), daylightDuration(), AlmanacCard() (moon geometry CORRECT)
      ForecastStrip.tsx           # getDayName(dayNum, monthNum) — Dec→Jan boundary BUG, fix in 1.2
      GlassCard.tsx  Header.tsx   # Header.tsx line 28: hardcoded "°N … °W" hemisphere BUG, fix in 1.2
      HumidityCard.tsx  LightningCard.tsx  PressureCard.tsx  RainCard.tsx
      SettingsPanel.tsx           # dead Station ID (line 103) + API Token (line 112) inputs — REMOVE in 1.7c
      SolarUVCard.tsx  StationHealth.tsx  TemperatureHero.tsx  WeatherIcon.tsx  WindCard.tsx
    themes/themes.ts              # themes: Record<ThemeName,…>; applyTheme(name); getThemeList() — theme-var leak fix in UX.1
    types/weather.ts              # SINGLE SOURCE for Contract C shapes — reproduced verbatim below
```

**UI manifest — `web/src/types/weather.ts` (verbatim, the Contract C source of truth):**

```ts
export interface StationMeta {
  station_id: number; name: string; latitude: number; longitude: number;
  elevation: number; timezone: string; firmware_revision: string;
  serial_number: string; device_id: number;
}
export interface CurrentObservation {
  timestamp: number;
  windLull: number; windAvg: number; windGust: number; windDirection: number;
  windSampleInterval: number; stationPressure: number; airTemperature: number;
  relativeHumidity: number; illuminance: number; uvIndex: number;
  solarRadiation: number; rainAccumulated: number;
  precipitationType: PrecipitationType;
  lightningStrikeAvgDistance: number; lightningStrikeCount: number;
  battery: number; reportInterval: number; localDayRainAccumulation: number;
  feelsLike: number; dewPoint: number; wetBulbTemperature: number;
  heatIndex: number; windChill: number; pressureTrend: PressureTrend;
}
export const PrecipitationType = { None:0, Rain:1, Hail:2, RainAndHail:3 } as const;
export type PrecipitationType = typeof PrecipitationType[keyof typeof PrecipitationType];
export const PressureTrend = { Falling:'falling', Steady:'steady', Rising:'rising' } as const;
export type PressureTrend = typeof PressureTrend[keyof typeof PressureTrend];
export interface ForecastDay {
  dayNum: number; monthNum: number; conditions: string; icon: string;
  airTempHigh: number; airTempLow: number; precipProbability: number;
  precipType: string; sunrise: number; sunset: number;
}
export interface HourlyForecast {
  timestamp: number; conditions: string; icon: string; airTemperature: number;
  feelsLike: number; relativeHumidity: number; windAvg: number;
  windDirection: number; windGust: number; precipProbability: number; uvIndex: number;
}
export interface StationStatus {
  isOnline: boolean; lastReport: number; batteryLevel: number;
  signalStrength: number; firmwareVersion: string;
}
export type TemperatureUnit = 'C' | 'F';
export type WindUnit = 'ms' | 'mph' | 'kph' | 'kts';
export type PressureUnit = 'mb' | 'inHg' | 'hPa';
export type RainUnit = 'mm' | 'in';
export interface TempRecord { high: number; highDate: string; low: number; lowDate: string; }
export interface StationAlmanac {
  today: TempRecord; week: TempRecord; month: TempRecord; year: TempRecord;
  sunrise: number; sunset: number; moonPhase: number;
  moonPhaseName: string; moonIllumination: number;
}
export interface UserPreferences {
  temperatureUnit: TemperatureUnit; windUnit: WindUnit;
  pressureUnit: PressureUnit; rainUnit: RainUnit; theme: string;
}
export type ThemeName = 'liquid-glass' | 'midnight-aurora' | 'desert-sunset' | 'nord' | 'tokyo-night' | 'catppuccin-mocha' | 'the-grid';
```

**Steps:**
- [ ] **Step 1:** Clone the pinned upstream and copy it in: `git clone https://github.com/jacaudi/tempest-display && (cd tempest-display && git checkout 49892063)` then copy its tree into `web/`. Record the exact source commit `49892063` in `web/PROVENANCE.md`. **Delete the upstream `web/server/`** (its 63-line static server is absorbed by the Go server in Task 1.3). Keep `src/`, `package.json`, `vite.config.ts`, tsconfigs, ESLint.
- [ ] **Step 2 (verify manifest):** confirm the vendored tree matches the manifest above and that `web/src/types/weather.ts` is byte-identical to the reproduction (it is Contract C's source). If not, STOP and reconcile.
- [ ] **Step 3:** Add `task ui-build` → `cd web && npm ci --ignore-scripts && npm run build` producing `web/dist`. Add `web/dist/.gitkeep` and document the `go:embed`-needs-non-empty-dir build-order requirement in `web/README.md`.
- [ ] **Step 4 (verify):** `task ui-build` produces `web/dist/index.html`.
- [ ] **Step 5:** Commit `feat: vendor tempest-display UI into web/ (owned fork @49892063) + UI manifest`.

**Acceptance:** `task ui-build` succeeds; `web/dist/index.html` exists; vendored tree matches the manifest.
**Implements:** design §7. The manifest above is the reference for all downstream UI tasks.

### Task 1.2: Vitest harness + cover the pure math (UI-F test gap) — depends on: Task 1.1

**Files:**
- Modify: `web/package.json` (add `vitest`, `@testing-library/react`, `test` script)
- Create: `web/src/hooks/useUnits.test.ts`, `web/src/components/AlmanacCard.test.tsx`, `web/src/components/ForecastStrip.test.tsx`

**Steps:**
- [ ] **Step 1 (RED):** Write tests for the already-correct pure functions (10 unit conversions, moon-phase, day-name) AND for the bugs to be fixed: `getDayName` across a Dec→Jan boundary (currently wrong — UI D-MEDIUM), hemisphere suffix `°S/°E` (UI C-MEDIUM). The boundary/hemisphere tests FAIL against current code.
- [ ] **Step 2:** Run `cd web && npm test` → conversions/moon PASS, boundary/hemisphere FAIL.
- [ ] **Step 3 (GREEN):** Fix `ForecastStrip.getDayName` to use the entry's own `sunrise` epoch; fix hemisphere suffix logic to honor sign of lat/lon.
- [ ] **Step 4:** Run `npm test` → all PASS.
- [ ] **Step 5:** Commit `test: add vitest + fix day-name and hemisphere bugs (UI D/C-MEDIUM)`.

**Acceptance:** `cd web && npm test` passes; `npm run build` clean.
**Implements:** design §14 P1.9, §18; resolves **UI D-MEDIUM**, **UI C-MEDIUM**.

### Task 1.3: Go HTTP server — embed UI, SPA fallback, timeouts, headers, graceful shutdown — depends on: Task 1.1, Task 0.7

**Files:**
- Create: `internal/httpserver/server.go`, `internal/httpserver/embed.go` (`//go:embed web/dist` — note: embed path must be relative; place the embed directive appropriately or use an embed sub-package under `web/`)
- Test: `internal/httpserver/server_test.go`

> Embed note: `//go:embed` cannot cross out of a package dir upward. Put the embed directive in a small `web/embed.go` (`package web`, `//go:embed dist`) and import it, OR generate into the httpserver package. Choose the `web` sub-package approach and document it.

**Steps:**
- [ ] **Step 1 (RED):**
  - `TestServer_ServesIndex`: `GET /` → 200 + the embedded index.html.
  - `TestServer_SPAFallback`: `GET /some/spa/route` → 200 index.html; `GET /assets/missing.js` → 404 (NOT index — fixes UI immutable-cache bug).
  - `TestServer_SecurityHeaders`: responses carry `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and the **self-only CSP** (B2) exactly: `default-src 'self'; img-src 'self' data: blob:; worker-src 'self' blob:; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'self'`. Assert the CSP contains **no** external host (no `http`/`https` scheme with a domain) — the basemap is same-origin `.pmtiles`.
  - `TestServer_Timeouts`: the `http.Server` has non-zero Read/ReadHeader/Write/Idle timeouts.
  - `TestServer_Healthz`: `GET /healthz` → 200 `{"status":"ok"}`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Build a `net/http.ServeMux`-based server (Go 1.22 method+path patterns) serving the embedded FS with `fs.ValidPath` traversal safety (preserve the praised safety), SPA fallback, security-headers middleware emitting the self-only CSP above (single `const cspPolicy = "..."`), explicit timeouts, `/healthz`, and `Shutdown(ctx)` on the cleanup ctx. Only set `Cache-Control: immutable` after a successful asset stat. The OSM `.pmtiles` basemap is served **same-origin** as a static asset (e.g. `web/dist/basemap/osm.pmtiles`, embedded or bind-mounted — the asset itself is produced in Task 2.6's basemap step); MapLibre reads it via byte-range requests to `'self'`, so no external tile host and no CSP host entry.
- [ ] **Step 4:** Run `go test -race ./internal/httpserver/`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `feat: embedded UI HTTP server (timeouts, headers, SPA fallback, /healthz)`.

**Acceptance:** `go test -race ./internal/httpserver/` passes.
**Interfaces produced:** `httpserver.New(cfg, deps) *http.Server`; mux registration seam for API handlers.
**Implements:** design §7/§11/§15; resolves **UI E-H1**, **UI E-MEDIUM** (cache/headers/shutdown), UI F-LOW (health).

### Task 1.4: Observation API handlers (current/history) reading sqlite — depends on: Task 1.3, Task 3.4

**Files:**
- Create: `internal/httpserver/observations.go`
- Test: `internal/httpserver/observations_test.go`

**Steps:**
- [ ] **Step 1 (RED):** With a temp sqlite DB seeded via the WS3 writer:
  - `TestAPI_CurrentObservation`: `GET /api/observations/current` → 200 JSON matching Contract C `CurrentObservation` (SI units, field names from `web/src/types/weather.ts`).
  - `TestAPI_History`: `GET /api/observations/history?field=temp_air&from=&to=` → `{"points":[{"t":..,"v":..}]}`; an invalid `field` → 400 (allowlist).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Handlers call `(*sqlite.Writer).LatestObservation`/`HistoryPoints`; marshal to Contract C shapes; register on the mux.
- [ ] **Step 4:** Run `go test -race ./internal/httpserver/` → PASS.
- [ ] **Step 5:** Commit `feat: /api/observations current+history from sqlite (UI B-H2)`.

**Acceptance:** `go test -race ./internal/httpserver/` passes.
**Implements:** design §11 (Contract C); resolves **UI B-H2** (real data path).

### Task 1.5: WeatherFlow proxy handlers (station/forecast/almanac) with server-side token — depends on: Task 1.3, Task 0.5

**Files:**
- Create: `internal/httpserver/proxy.go`
- Test: `internal/httpserver/proxy_test.go`

**Steps:**
- [ ] **Step 1 (RED):** With an `httptest.Server` standing in for WeatherFlow:
  - `TestProxy_InjectsBearerToken`: `GET /api/station` → the upstream request carries `Authorization: Bearer <TOKEN>` and the browser-facing request needs no token.
  - `TestProxy_NoTokenInResponseOrLogs`: the token never appears in the response body.
  - `TestProxy_ForecastAndAlmanac`: both endpoints proxy and pass through JSON.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Implement handlers that call the WeatherFlow REST endpoints with the server-held `TOKEN` via `Authorization: Bearer` (reuse the hardened client from Task 0.5), timeouts, status checks; register on the mux.
- [ ] **Step 4:** Run `go test -race ./internal/httpserver/` → PASS.
- [ ] **Step 5:** Commit `feat: server-side WeatherFlow proxy (UI B-H1, exporter F-H1)`.

**Acceptance:** `go test -race ./internal/httpserver/` passes.
**Implements:** design §7/§11/§15; resolves **UI B-H1**, exporter **F-H1** (token off the wire).

### Task 1.6: `otelhttp` middleware + wire the server into main — depends on: Task 1.5, Task 6.4

**Files:**
- Modify: `internal/httpserver/server.go` (wrap handler in `otelhttp.NewHandler`), `main.go` (start server, run in both modes)
- Test: `internal/httpserver/server_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestServer_EmitsHTTPSpan`: with a `tracetest.SpanRecorder`, a request produces an `http.server.request` span.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Wrap the mux in `otelhttp.NewHandler(mux, "http.server")`. In `main.go`, start the HTTP server (default addr `:8080`, `HTTP_ADDR` override) in UDP mode alongside ingest; graceful shutdown via cleanup ctx.
- [ ] **Step 4:** Run `go test -race ./internal/httpserver/`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `feat: otelhttp middleware + start UI/API server from main`.

**Acceptance:** `go test -race ./internal/httpserver/` passes; `go build ./...` clean.
**Implements:** design §8/§11.

> **Task 1.7 was split into 1.7a/1.7b/1.7c** (cold review: the original bundled five independent concerns and a reviewer could not accept one while rejecting another). Each subtask is independently testable. All three depend on the UI manifest in Task 1.1 for symbol names.

### Task 1.7a: UI data layer — replace stubs with backend fetches + AbortController + stale indicator — depends on: Task 1.4, Task 1.5

**Files:**
- Modify: `web/src/api/tempestApi.ts` (real fetches to relative `/api/*`; delete `stubData` import), `web/src/hooks/useWeatherData.ts` (AbortController + stale tracking)
- Delete: `web/src/api/stubData.ts`
- Create: `web/src/api/tempestApi.test.ts`

**Steps:**
- [ ] **Step 1 (RED):** `tempestApi.test.ts`: mock global `fetch`; assert `fetchCurrentObservation()` GETs the tokenless relative URL `/api/observations/current` and returns a typed `CurrentObservation` (from the manifest's `weather.ts`), with no `stubData` import. Add a `useWeatherData` test: a failed refetch retains the prior `WeatherData` and flips a `isStale`/`lastUpdated` field (§14 P1.6). Both FAIL.
- [ ] **Step 2:** Run `cd web && npm test` → FAIL.
- [ ] **Step 3 (GREEN):** In `tempestApi.ts`, replace every stub return in `fetchStationMeta`/`fetchCurrentObservation`/`fetchForecast`/`fetchHourlyForecast`/`fetchStationStatus`/`fetchStationAlmanac` with `fetch` to the tokenless relative Contract C endpoints (`/api/station`, `/api/observations/current`, `/api/forecast`, `/api/almanac`, …); drop `setApiToken`/`getApiToken` (token is server-side). Add an `AbortController` to `useWeatherData` (cancel in-flight on unmount/refetch — UI B-MEDIUM race); retain prior data on failure and expose `isStale` + `lastUpdated` age.
- [ ] **Step 4:** Run `npm test && npm run build && npx tsc --noEmit` → clean.
- [ ] **Step 5:** Commit `feat: real UI data layer (Contract C) + AbortController + stale indicator (UI B-H2, B-MEDIUM, §14 P1.6)`.

**Acceptance:** `cd web && npm test && npm run build && npx tsc --noEmit` clean.
**Implements:** design §11/§14 P1.6; resolves **UI B-H2**, **UI B-MEDIUM**.

### Task 1.7b: Error boundary + missing CSS + responsive/a11y (UX P0) — depends on: Task 1.1

**Files:**
- Create: `web/src/components/ErrorBoundary.tsx`, `web/src/components/ErrorBoundary.test.tsx`
- Modify: `web/src/App.tsx` (wrap dashboard), `web/src/App.css` / `web/src/index.css` (define `.loading-screen/.loading-spinner/.error-screen/.glass-btn`, `@media` breakpoints, `prefers-reduced-motion`, `:focus-visible`), the UV card (`SolarUVCard.tsx`) for the `formatX` NaN guard

**Steps:**
- [ ] **Step 1 (RED):** `ErrorBoundary.test.tsx`: a child that throws renders the fallback UI, not a blank page. Add a small test that a UV value of `undefined` routed through the `formatX` helper renders `"NaN"` (or an em-dash) rather than throwing. Both FAIL.
- [ ] **Step 2:** Run `cd web && npm test` → FAIL.
- [ ] **Step 3 (GREEN):**
  - Add `ErrorBoundary` (class component with `componentDidCatch`) and wrap the dashboard in `App.tsx` (§14 P0.1 / UI A-H1) — one bad card must not blank a 24/7 display.
  - Define the missing `.loading-screen/.loading-spinner/.error-screen/.glass-btn` CSS (§14 P0.2 / UI A-H2 — the literal first screen).
  - Add responsive `@media` breakpoints collapsing the 3-col grid to 2/1 (P0.3 / UI A-H3); add `@media (prefers-reduced-motion: reduce)` disabling animations and a global `:focus-visible` outline (P0.4 / UI A-H4).
  - Route UV (and other optional fields) through a `formatX(value, fmt)` helper so a missing field renders `NaN`/`—` not a hard crash (P0.5 / UI C-MEDIUM).
- [ ] **Step 4:** Run `npm test && npm run build && npx tsc --noEmit` → clean.
- [ ] **Step 5:** Commit `feat: error boundary + missing CSS + responsive/a11y + NaN-safe formatX (UI A-H1..H4, C-MEDIUM)`.

**Acceptance:** `cd web && npm test && npm run build && npx tsc --noEmit` clean.
**Implements:** design §14 P0; resolves **UI A-H1, A-H2, A-H3, A-H4, C-MEDIUM**.

### Task 1.7c: Remove dead SettingsPanel token inputs + self-host fonts — depends on: Task 1.7a

**Files:**
- Modify: `web/src/components/SettingsPanel.tsx` (remove Station-ID/Token inputs), `web/index.html` / `web/src/index.css` (self-host Inter, drop Google Fonts CDN)
- Create: `web/src/components/SettingsPanel.test.tsx`; add the Inter font files under `web/src/assets/fonts/` (or `web/public/fonts/`)

**Steps:**
- [ ] **Step 1 (RED):** `SettingsPanel.test.tsx`: render `SettingsPanel`; assert there is **no** "Station ID" text input and **no** "API Token" password input (they are dead now the token is server-side — §14 P1.10 / UI D-MEDIUM). FAILs against current code (inputs at `SettingsPanel.tsx:103` and `:112`).
- [ ] **Step 2:** Run `cd web && npm test` → FAIL.
- [ ] **Step 3 (GREEN):** Remove the Station-ID and API-Token inputs and their handlers/state from `SettingsPanel.tsx` (keep the real settings: units, theme, kiosk toggle). Self-host the Inter font (§14 P1.12 / UI A-MEDIUM): add the woff2 files, `@font-face` in `index.css`, and **remove the Google Fonts `<link>`** from `index.html` — the self-only CSP (Task 1.3) has no font CDN, so the CDN link would otherwise be blocked and the appliance must work offline/air-gapped.
- [ ] **Step 4:** Run `npm test && npm run build && npx tsc --noEmit` → clean.
- [ ] **Step 5:** Commit `feat: remove dead token inputs + self-host Inter font (UI D-MEDIUM, A-MEDIUM)`.

**Acceptance:** `cd web && npm test && npm run build && npx tsc --noEmit` clean; no external font/host references remain.
**Implements:** design §7/§14 P1.10/P1.12; resolves **UI D-MEDIUM**, **UI A-MEDIUM**.

### Task 1.8: Multi-stage Dockerfile — Vite build → Go embed → non-root static image — depends on: Task 1.6, Task 1.7a, Task 1.7b, Task 1.7c

**Files:**
- Modify: `Dockerfile`, `docker-bake.hcl`, `.dockerignore`

**Steps:**
- [ ] **Step 1:** Rewrite `Dockerfile` as multi-stage: (1) `node:22-alpine` → `npm ci --ignore-scripts` + `vite build` → `web/dist`; (2) `golang:1.25-alpine` → `CGO_ENABLED=0 go build` embedding `web/dist`; (3) `cgr.dev/chainguard/static` non-root `USER 65532`, `-trimpath -ldflags="-s -w"`, version injection (`-X main.version=...`). Digest-pin base images. Add `EXPOSE 8080` + `HEALTHCHECK` hitting `/healthz`.
- [ ] **Step 2 (verify):** `docker buildx bake image-local` builds; `docker run` the image → `GET /healthz` returns 200; container runs as UID 65532.
- [ ] **Step 3:** Commit `build: multi-stage Dockerfile embedding UI, non-root static image (UI F-H1)`.

**Acceptance:** `docker buildx bake image-local` succeeds; healthcheck passes; non-root confirmed.
**Implements:** design §7/§15; resolves **UI F-H1** (root→non-root).

---

## Workstream 2 — NEXRAD Level 3 radar overlay (opt-in Python sidecar + Go proxy)

> Design §9. Depends on WS1 (HTTP surface + UI map card). Radar is opt-in (`ENABLE_RADAR`). Contract A governs the Go↔sidecar boundary.

### Task 2.1: Python radar sidecar — FastAPI + Py-ART decode → contoured GeoJSON — no deps (parallel-safe; separable dir)

> **Fixture BLOCKER resolved (cold review #1).** The binary NIDS fixture cannot be authored from text, so the TDD loop starts with a **deterministic malformed-bytes RED case that needs no binary** (`test_decode_bad_bytes_raises`). The binary golden-file test comes second and its fixture is obtained by a **pinned acquire-then-commit step** (Step 0) — fetched once from the real-time S3 bucket and committed, so it is stable thereafter (the bucket is real-time with rotating flat keys `SSS_PPP_YYYY_MM_DD_HH_MM_SS`, so a hardcoded key would 404 later; committing the fetched bytes pins it by git, not by S3).

**Files:**
- Create: `radar/app.py`, `radar/decode.py`, `radar/requirements.txt`, `radar/Dockerfile`, `radar/tests/test_decode.py`, `radar/tests/fixtures/TLX_N0B.nids` (committed binary), `radar/tests/fixtures/PROVENANCE.md`

**Steps:**
- [ ] **Step 0 (acquire + commit the fixture — one-time, documented):** requires `aws` CLI (anonymous S3 works with `--no-sign-request`). The bucket is real-time, so **list the newest object then copy it**, and commit the bytes:

```bash
# newest TLX super-res base-reflectivity object (bucket is us-east-1, anonymous):
KEY=$(aws s3 ls --no-sign-request s3://unidata-nexrad-level3/ \
      | grep -E 'TLX_N0B_' | sort | tail -1 | awk '{print $4}')
aws s3 cp --no-sign-request "s3://unidata-nexrad-level3/${KEY}" \
      radar/tests/fixtures/TLX_N0B.nids
# record what we fetched so the golden file is traceable:
printf 'source: s3://unidata-nexrad-level3/%s\nfetched: %s\n' "$KEY" "$(date -u +%FT%TZ)" \
      > radar/tests/fixtures/PROVENANCE.md
git add radar/tests/fixtures/TLX_N0B.nids radar/tests/fixtures/PROVENANCE.md
```

  The committed `TLX_N0B.nids` is now the stable golden input; CI never touches S3 for tests.
- [ ] **Step 1 (RED, pytest — deterministic, no binary needed first):**
  - `test_decode.py::test_decode_bad_bytes_raises`: `decode.to_geojson(b"not-nids", "N0B")` raises a typed `decode.DecodeError`. This can be written and watched-fail immediately (module → function → wrong-exception), unblocking TDD.
  - `test_decode.py::test_decode_nids_to_isobands` (uses the committed fixture from Step 0): `decode.to_geojson(Path("tests/fixtures/TLX_N0B.nids").read_bytes(), "N0B")` returns a FeatureCollection whose features carry `dbz_min`/`dbz_max` band properties and a `metadata` block with `site`/`product`/`scan_time`/`bbox` (Contract A). Assert `>0` features and that `dbz_min`/`dbz_max` are 5-dBZ-aligned.
- [ ] **Step 2:** Run `cd radar && pytest` → FAIL (module missing). Set matplotlib to headless `Agg` (`MPLBACKEND=Agg` / `matplotlib.use("Agg")`).
- [ ] **Step 3 (GREEN):** `decode.py`: define `class DecodeError(Exception)`; `to_geojson(nids: bytes, product: str)` writes the bytes to a temp file, `pyart.io.read_nexrad_level3` it (wrap any exception → `DecodeError`), grid base reflectivity, `geojsoncontour`/`contourf` into 5-dBZ isobands over the NWS reflectivity scale, emit Contract A GeoJSON (each `Feature.properties` has `dbz_min`/`dbz_max`; top-level `metadata`). `app.py`: FastAPI `GET /radar?site=&product=` — fetch newest NIDS from `s3://unidata-nexrad-level3` via anonymous `boto3` (`ListObjectsV2` prefix `SSS_PPP_`, pick largest `YYYY_MM_DD_HH_MM_SS`, `GetObject`), decode, return GeoJSON; on failure return the Contract A **non-200 + error envelope** (`503 no_recent_scan` / `502 decode_failed` / `500 internal`). `GET /healthz` → 200.
- [ ] **Step 4:** Run `cd radar && pytest` → PASS.
- [ ] **Step 5:** Commit `feat: python radar sidecar (Py-ART → contoured GeoJSON, Contract A) + committed NIDS fixture`.

**Acceptance:** `cd radar && pytest` passes (both the no-binary malformed case and the committed-fixture isoband case).
**Implements:** design §9 (O1 Py-ART sidecar, O3 contoured GeoJSON, Contract A).

### Task 2.2: Sidecar Dockerfile + healthcheck — depends on: Task 2.1

**Files:**
- Modify: `radar/Dockerfile`

**Steps:**
- [ ] **Step 1:** Multi-stage/ slim Python image installing Py-ART/SciPy/matplotlib/boto3/geojsoncontour/FastAPI/uvicorn; run uvicorn on `:8081`; non-root user; `HEALTHCHECK` → `/healthz`; headless matplotlib `MPLBACKEND=Agg`.
- [ ] **Step 2 (verify):** `docker build radar/` succeeds; `docker run` → `GET /healthz` 200.
- [ ] **Step 3:** Commit `build: radar sidecar Dockerfile`.

**Acceptance:** `docker build radar/` succeeds; healthcheck passes.
**Implements:** design §9/§15a.

### Task 2.3: Go `internal/radar` — site table + nearest-site selection + `{site}` allowlist — depends on: Task 0.7

> **Site-table data is concrete (cold review #9).** Do NOT "reuse DRAS." Generate the table from NOAA's authoritative WSR-88D station list (fixed-width text, verified reachable). This is a one-time `go generate` step whose output is committed.

**Files:**
- Create: `internal/radar/sites.go` (committed static table `var sites = []Site{...}`), `internal/radar/select.go`
- Test: `internal/radar/select_test.go`

**Steps:**
- [ ] **Step 0 (data acquisition — pinned, one-time, commit the OUTPUT):** the NOAA HOMR list is fixed-width; verified format: `ICAO` (e.g. `KTLX`), `LAT`/`LON` decimal degrees, `STNTYPE` == `NEXRAD`. The site code is the ICAO **without the leading `K`** (`KTLX`→`TLX`). Produce the committed table with a documented one-liner (no generator program to maintain — the table changes maybe once a decade):

```bash
{ echo 'package radar'; echo; echo 'var sites = []Site{'; \
  curl -s "https://www.ncei.noaa.gov/access/homr/file/nexrad-stations.txt" \
  | awk 'NR>2 && /NEXRAD/ { icao=$2; code=substr(icao,2); printf "\t{%q, %s, %s},\n", code, $(NF-4), $(NF-3) }'; \
  echo '}'; } > internal/radar/sites.go
gofmt -w internal/radar/sites.go
```

  Commit `internal/radar/sites.go` (the ~160-row output). Re-run the one-liner only if NOAA adds a site. (Verify the `awk` column indices against the current file header once — HOMR is stable fixed-width; adjust `$(NF-4)`/`$(NF-3)` if the trailing columns shift.)
- [ ] **Step 1 (RED):** `TestNearestSite`: for a lat/lon near Oklahoma City (`35.47, -97.51`), `NearestSite(lat, lon)` returns `"TLX"`. `TestValidSite`: `IsValidSite("TLX")` true; `IsValidSite("../etc")`, `IsValidSite("ZZZ")`, `IsValidSite("tlx")` (case) all false (SSRF allowlist guard).
- [ ] **Step 2:** Run `go test ./internal/radar/` → FAIL (`Site`/`NearestSite`/`IsValidSite` missing).
- [ ] **Step 3 (GREEN):** `select.go`: `type Site struct{ Code string; Lat, Lon float64 }`; `NearestSite(lat, lon float64) string` via haversine over `sites` (return the min-distance `Code`); `IsValidSite(code string) bool` = exact-match membership in `sites` (uppercase, 3-char).
- [ ] **Step 4:** Run `go test ./internal/radar/` → PASS.
- [ ] **Step 5:** Commit `feat: radar site table (generated from NOAA HOMR) + nearest-site + allowlist (SSRF guard)`.

**Acceptance:** `go test ./internal/radar/` passes.
**Interfaces produced:** `radar.NearestSite(lat, lon) string`, `radar.IsValidSite(code) bool`.
**Implements:** design §9/§15 (SSRF guard).

### Task 2.4: Go radar proxy + LRU cache (Contract A client) — depends on: Task 2.3

**Files:**
- Create: `internal/radar/proxy.go` (sidecar HTTP client + `(site,product,scanTime)` LRU)
- Test: `internal/radar/proxy_test.go`

**Steps:**
- [ ] **Step 1 (RED):** With an `httptest.Server` mimicking the sidecar (Contract A):
  - `TestProxy_FetchesAndCaches`: two calls for the same `(site,product,scanTime)` hit the sidecar once (cache hit second time).
  - `TestProxy_N0BFallsBackToN0Q`: sidecar returns `503 no_recent_scan` for N0B → proxy retries with N0Q (O2 default+fallback).
  - `TestProxy_ErrorEnvelope`: `502 decode_failed` → proxy surfaces a typed error mapped for the HTTP layer.
  - `TestProxy_Constants`: assert the pinned cache constants `radarCacheMaxEntries == 64` and `radarCacheTTL == 5*time.Minute`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Implement the sidecar client (stdlib `http.Client` with a 30s timeout) and a bounded LRU keyed on `(site,product,scanTime)`. **Pinned constants** (asserted in RED):

```go
const (
	// radarCacheMaxEntries bounds the LRU. ~160 sites but any one deployment
	// tracks 1 station → a handful of sites × 2 products (N0B/N0Q) × the last
	// few scan times; 64 is generous headroom and a hard memory bound.
	radarCacheMaxEntries = 64
	// radarCacheTTL ≈ one WSR-88D volume-scan cycle (VCP ~4–6 min); 5 min keeps
	// a scan cached for its useful life without serving stale reflectivity.
	radarCacheTTL = 5 * time.Minute
	// radarSidecarTimeout bounds the HTTP call to the Python sidecar.
	radarSidecarTimeout = 30 * time.Second
)
```

  N0B→N0Q fallback (per O2): on a `503 no_recent_scan` for `N0B`, retry once with `N0Q`. Map Contract A non-200 envelopes to typed Go errors (`ErrNoRecentScan`, `ErrDecodeFailed`, `ErrInternal`) for the HTTP layer.
- [ ] **Step 4:** Run `go test -race ./internal/radar/` → PASS.
- [ ] **Step 5:** Commit `feat: radar proxy + LRU cache + N0B→N0Q fallback (O2, Contract A)`.

**Acceptance:** `go test -race ./internal/radar/` passes.
**Interfaces produced:** `radar.NewProxy(sidecarURL, cfg)`; `(*Proxy).Get(ctx, site, product) (GeoJSON, meta, error)`.
**Implements:** design §9 (O2, Contract A).

### Task 2.5: `/api/radar/{site}` handler (opt-in) — depends on: Task 2.4, Task 1.6

**Files:**
- Create: `internal/httpserver/radar.go`
- Test: `internal/httpserver/radar_test.go`

**Steps:**
- [ ] **Step 1 (RED):**
  - `TestRadarHandler_ServesGeoJSON`: `GET /api/radar/TLX?product=N0B` → 200 GeoJSON (via a fake proxy).
  - `TestRadarHandler_RejectsInvalidSite`: `GET /api/radar/../etc` → 400 (allowlist).
  - `TestRadarHandler_ErrorMapping`: proxy `no_recent_scan`→503, `decode_failed`→502, `internal`→502 with `{"error":...}` (Contract C).
  - `TestRadarHandler_DisabledWhenNotEnabled`: with `ENABLE_RADAR` off, the route is not registered / returns 404.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3 (GREEN):** Register the handler only when `ENABLE_RADAR=true`; validate `{site}` via `radar.IsValidSite`; call the proxy; map errors per Contract C.
- [ ] **Step 4:** Run `go test -race ./internal/httpserver/`; `go build ./...` → clean, PASS.
- [ ] **Step 5:** Commit `feat: /api/radar/{site} handler, opt-in ENABLE_RADAR (Contract C)`.

**Acceptance:** `go test -race ./internal/httpserver/` passes.
**Implements:** design §9/§11; resolves **UI F-MEDIUM** (phantom radar → real feature).

### Task 2.6: UI radar map card (MapLibre GL JS + same-origin OSM `.pmtiles` basemap, GeoJSON isoband fill) — depends on: Task 2.5, Task 1.7a, Task 1.7b

> **Basemap = OpenStreetMap via Protomaps `.pmtiles`, served same-origin (user decision B2).** No external tile server, no API key, offline-capable. MapLibre reads the `.pmtiles` file with byte-range requests to `'self'` via the `pmtiles` protocol plugin. **OSM attribution is required** and MUST be visible on the map. The public `tile.openstreetmap.org` raster server is an online-only, light-use fallback only — not the default.

**Files:**
- Create: `web/src/components/RadarCard.tsx`, `web/src/components/RadarCard.test.tsx`, `web/public/basemap/osm.pmtiles` (built/downloaded — Step 0)
- Modify: `web/src/App.tsx` (mount the card when radar is available), `web/package.json` (add `maplibre-gl`, `pmtiles`)

**Steps:**
- [ ] **Step 0 (basemap asset — pinned, one-time, committed or build-fetched):** obtain a single OSM vector-tile `.pmtiles` file and place it at `web/public/basemap/osm.pmtiles` so Vite copies it into `web/dist/basemap/` (served same-origin under `'self'`). Two documented options — pick one and record it in `web/public/basemap/PROVENANCE.md`:
  - **Regional extract (recommended, small):** build from a region extract with the Protomaps tooling — `pmtiles extract https://build.protomaps.com/<DATE>.pmtiles web/public/basemap/osm.pmtiles --bbox=<minLon,minLat,maxLon,maxLat>` (bbox around the deployment region), or download a prebuilt regional `.pmtiles` from `build.protomaps.com`.
  - **Whole-planet (large):** download the daily planet build `https://build.protomaps.com/<DATE>.pmtiles` (only if a global basemap is genuinely wanted — it is hundreds of MB; prefer the extract).
  Attribution string to render: `© OpenStreetMap contributors`.
- [ ] **Step 1 (RED):** `RadarCard.test.tsx`: renders a map container; registers the `pmtiles://` protocol pointing at the same-origin `/basemap/osm.pmtiles`; on a mocked `/api/radar/...` GeoJSON response it adds a fill layer styled by `dbz_min`/`dbz_max`; on an `{error}` response it renders a graceful "radar unavailable" state (no crash); the OSM attribution control is present. Respects `prefers-reduced-motion` (no auto-pan). (Mock MapLibre GL in jsdom.)
- [ ] **Step 2:** Run `cd web && npm test` → FAIL.
- [ ] **Step 3 (GREEN):** Implement `RadarCard` with MapLibre GL JS. Register the pmtiles protocol: `import { Protocol } from 'pmtiles'; const p = new Protocol(); maplibregl.addProtocol('pmtiles', p.tile);` and use a style whose vector source is `url: 'pmtiles:///basemap/osm.pmtiles'` (same-origin — matches the self-only CSP from Task 1.3; no external host, no API key). Add a visible `AttributionControl` showing `© OpenStreetMap contributors`. Center on the station lat/lon; add the radar GeoJSON as a fill layer with a data-driven color ramp keyed on `dbz_min`; auto-refresh on the scan cadence; graceful error state.
- [ ] **Step 4:** Run `npm test && npm run build && npx tsc --noEmit` → clean.
- [ ] **Step 5:** Commit `feat: UI radar map card (MapLibre + same-origin OSM pmtiles basemap, dBZ isobands) — §14 P1.8, B2`.

**Acceptance:** `cd web && npm test && npm run build` clean; basemap loads from `'self'`; OSM attribution visible.
**Implements:** design §9/§14 P1.8 (B2 pmtiles basemap, self-only CSP, OSM attribution).

---

## Workstream 4 — Grafana "Weather Nerd" dashboard

> Design §13. Depends on WS6 (metrics via Collector→Prometheus). PromQL uses ONLY Contract B / design §13 names + `serial` label.

### Task 4.1: OTel→Prometheus name-translation assertion test (real exporter) — depends on: Task 6.2

> **Repaired per cold review:** the previous "OR table-equality" branch compared a hand-written map to a hand-written map and asserted nothing about actual translation — deleted. This test drives the OTel writer through the **real** in-process Prometheus exporter (`go.opentelemetry.io/otel/exporters/prometheus`), which performs the exact OTLP→Prometheus normalization the Collector uses (dotted→underscore, unit suffixes, `_total` for monotonic counters). Gathering the registry and asserting the emitted metric family names is a genuine translation check, deterministic on every `go test` run — no Collector container needed. (An optional `//go:build integration` real-Collector test may be added later but is not required.)

**Files:**
- Create: `internal/otel/promnames_test.go`

**Steps:**
- [ ] **Step 1 (RED):** `TestPrometheusExporterEmitsContractBNames`:

```go
func TestPrometheusExporterEmitsContractBNames(t *testing.T) {
	reg := prom.NewRegistry() // github.com/prometheus/client_golang/prometheus
	exporter, err := otelprom.New(otelprom.WithRegisterer(reg)) // otel exporters/prometheus
	if err != nil { t.Fatal(err) }
	mp := metric.NewMeterProvider(metric.WithReader(exporter))
	w := otel.NewWriter(mp)

	// record one of every field so every instrument is emitted:
	w.WriteReport(t.Context(), fullSampleReport())

	mfs, err := reg.Gather()
	if err != nil { t.Fatal(err) }
	got := map[string]bool{}
	for _, mf := range mfs { got[mf.GetName()] = true }

	// wantNames = Contract B right column (the exact tempest_* names):
	wantNames := []string{
		"tempest_temperature_c", "tempest_dewpoint_c", "tempest_heat_index_c",
		"tempest_wetbulb_c", "tempest_humidity_percent", "tempest_pressure_mb",
		"tempest_wind_meters_per_second", "tempest_wind_direction_degrees",
		"tempest_uv_index", "tempest_irradiance_w_m2", "tempest_illuminance_lux",
		"tempest_rain_rate_mm_min", "tempest_rainfall_mm_total",
		"tempest_lightning_distance_km", "tempest_lightning_strike_count_total",
		"tempest_battery_volts", "tempest_rssi_dbm", "tempest_uptime_seconds",
		"tempest_reboots_total", "tempest_bus_errors_total",
	}
	for _, n := range wantNames {
		if !got[n] { t.Errorf("missing Prometheus metric %q (got %v)", n, slices.Sorted(maps.Keys(got))) }
	}
	// negative guards: the OLD names must NOT appear (the intended breaks):
	for _, old := range []string{"tempest_wind_ms", "tempest_uptime_seconds_total", "tempest_rainfall_total"} {
		if got[old] { t.Errorf("stale metric name %q still emitted", old) }
	}
}
```

  (The Prometheus exporter may append `_total` to monotonic counters and reconcile units itself — the assertion is on the **emitted** family names, so it catches a wrong instrument name or wrong kind.)
- [ ] **Step 2:** Run `go test ./internal/otel/ -run TestPrometheusExporterEmitsContractBNames` → FAIL if any instrument name/kind drifts from Contract B. First `go get go.opentelemetry.io/otel/exporters/prometheus@v0.54.0`.
- [ ] **Step 3 (GREEN):** Reconcile the Task 6.2 instrument names/kinds until the emitted family names match Contract B exactly.
- [ ] **Step 4:** Run the test → PASS.
- [ ] **Step 5 (metric-migration diff, item 11):** Add `TestMetricMigrationList`: diff Contract B's right column against the **current** descriptor names in `internal/tempest/metrics.go` (`tempest_uptime_seconds_total`, `tempest_rssi_dbm`, `tempest_reboots_total`, `tempest_bus_errors_total`, `tempest_illuminance_lux`, `tempest_uv_index`, `tempest_rain_rate_mm_min`, `tempest_wind_ms`, `tempest_wind_direction_degrees`, `tempest_battery_volts`, `tempest_report_interval_minutes`, `tempest_irradiance_w_m2`, `tempest_rainfall_total`, `tempest_pressure_mb`, `tempest_temperature_c`, `tempest_humidity_percent`, `tempest_lightning_distance_km`, `tempest_lightning_strike_count`). The test computes and prints (via `t.Log`) the renamed/added/dropped set — `tempest_wind_ms`→`tempest_wind_meters_per_second`, `tempest_uptime_seconds_total`→`tempest_uptime_seconds`, `tempest_rainfall_total`→`tempest_rainfall_mm_total`, `tempest_lightning_strike_count`→`tempest_lightning_strike_count_total`, plus the `instance`→`serial` label rename — and asserts this is the COMPLETE break set (fails if an unlisted name changes). DOC.2 consumes this list for the migration note.
- [ ] **Step 6:** Commit `test: assert OTel instruments translate to exact tempest_* names via real prom exporter + emit migration list (Contract B)`.

**Acceptance:** `go test ./internal/otel/ -run 'TestPrometheusExporterEmitsContractBNames|TestMetricMigrationList'` passes.
**Implements:** design §8/§13 (name-translation guard via the real exporter; complete metric-migration list for DOC.2).

### Task 4.2: Grafana dashboard JSON + provisioning — depends on: Task 4.1

**Files:**
- Create: `deploy/grafana/dashboards/weather-nerd.json`, `deploy/grafana/provisioning/datasources/prometheus.yaml`, `deploy/grafana/provisioning/dashboards/dashboards.yaml`
- Test: `deploy/grafana/validate_test.go` (or a `Makefile`/`task` target running `jsonnet`/`promtool`)

**Steps:**
- [ ] **Step 1 (RED):** Add a validation check: a small Go test (or `task grafana-validate`) that (a) `json.Unmarshal`s the dashboard without error, (b) asserts every panel `expr` references only names present in Contract B / design §13, and (c) asserts the `$serial` + `$__range` template variables exist. FAIL before the JSON exists.
- [ ] **Step 2:** Run the validation → FAIL.
- [ ] **Step 3 (GREEN):** Author `weather-nerd.json` implementing all 8 rows exactly as design §13 enumerates (Current conditions stats; Trends incl. `deriv(tempest_pressure_mb[3h])`; Wind rose + gust factor; Rain rate/accumulation; Lightning rate/nearest; Solar/UV; Station health; Records/almanac with `max_over_time`/`min_over_time` over `$__range`). Provision the Prometheus datasource + dashboard loader.
- [ ] **Step 4:** Run the validation → PASS.
- [ ] **Step 5:** Commit `feat: Grafana Weather Nerd dashboard + provisioning (§13)`.

**Acceptance:** dashboard JSON parses; validation passes; (manual) dashboard loads in the compose stack against live metrics.
**Implements:** design §13.

---

## Workstream 5 — UX polish (P2) + records the deferred P3 follow-ups

> Design §14. P0/P1 landed inside WS1/WS2. This is the P2 batch. P3 items are NOT built (YAGNI) — recorded as follow-ups in DOC.1.

### Task UX.1: UI P2 polish batch — depends on: Task 1.7b, Task 1.7c

**Files:**
- Modify: leaf card components (`web/src/components/*`), `web/src/App.css`, `web/src/components/SettingsPanel.tsx`, `web/src/themes/themes.ts`
- Test: extend `web/src/**/*.test.tsx`

**Steps:**
- [ ] **Step 1 (RED where testable):** Add tests for the behavior-bearing P2 items: settings modal dialog semantics (`role="dialog"`, Escape closes, focus restore — §14 P2.14), theme-var leak fix (`applyTheme` clears prior vars; `desert-sunset` `--text-shadow` — §14 P2.15). These FAIL initially.
- [ ] **Step 2:** Run `cd web && npm test` → FAIL.
- [ ] **Step 3 (GREEN):** Implement the P2 set: `React.memo` leaf cards + `useMemo` RainCard raindrops (§14 P2.13); dialog semantics/focus-trap (P2.14); theme-var leak fix + `aria-hidden` on decorative SVGs (P2.15); `100dvh` over `100vh`, drop `background-attachment: fixed`, enumerate `transition` props (P2.16). (P2.17 Vitest+CI already established in Task 1.2 / DOC.2.)
- [ ] **Step 4:** Run `npm test && npm run build && npx tsc --noEmit` → clean.
- [ ] **Step 5:** Commit `feat: UI P2 polish — memoization, dialog a11y, theme leak, viewport (§14 P2)`.

**Acceptance:** `cd web && npm test && npm run build` clean.
**Implements:** design §14 P2.13–P2.16.

---

## Documentation, Compose, CI

### Task DOC.1: Full-stack docker-compose + `.env.example` — depends on: Task 1.8, Task 2.2, Task 4.2

**Files:**
- Create: `deploy/docker-compose.yml`, `deploy/.env.example` (gitignored `.env`), `deploy/otel-collector-config.yaml`, `deploy/prometheus.yml`, `deploy/litestream.yml`

**Steps:**
- [ ] **Step 1:** Author compose per design §15a: `tempestwx` (`network_mode: host`, `OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317`, `SQLITE_PATH=/data/tempest.db`, `TOKEN`, volume `/data`); `litestream` (same `/data` → `minio`); `otel-collector` (OTLP `:4317`, metrics→Prometheus required, traces/logs→debug by default, **Tempo/Loki commented**); `prometheus`; `grafana` (provisioned datasource + WS4 dashboard); `radar-sidecar` **opt-in** (profile/commented, enabled with `ENABLE_RADAR`); `minio`. Document the `ENABLE_POSTGRES` variant (swap litestream+minio for `postgres`).
- [ ] **Step 2 (verify):** `docker compose -f deploy/docker-compose.yml config` validates; a smoke bring-up reaches Grafana with the dashboard and (with `ENABLE_RADAR`) the radar card.
- [ ] **Step 3:** Commit `feat: full-stack docker-compose (Collector+Prometheus+Grafana+Litestream+MinIO; radar opt-in) — §15a`.

**Acceptance:** `docker compose config` validates; smoke bring-up healthy.
**Implements:** design §15a (R1/O5 — no bundled Tempo/Loki as required services).

### Task DOC.2: README/docs rewrite + UI CI — depends on: Task DOC.1

**Files:**
- Modify: `README.md`, `CLAUDE.md` (env-var matrix), `.github/workflows/` (add UI lint/typecheck/build/`docker build` job)
- Create: `docs/CHANGELOG` note or follow-up list capturing the P3 items

**Steps:**
- [ ] **Step 1:** Rewrite README to reality: new env vars (`ENABLE_OTEL`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `SQLITE_PATH`, `SQLITE_*`, `ENABLE_RADAR`, `RADAR_SITE`, `LITESTREAM_*`, `HTTP_ADDR`); new default store (SQLite); updated operational-mode matrix (design §16); remove obsolete `PUSH_URL`/`DATABASE_*` (G-H2); note the `ENABLE_PROMETHEUS_*` deprecation and consume the **complete metric-migration list** emitted by Task 4.1's `TestMetricMigrationList` (`instance`→`serial` label break, `tempest_wind_ms`→`tempest_wind_meters_per_second`, `tempest_uptime_seconds_total`→`tempest_uptime_seconds`, `tempest_rainfall_total`→`tempest_rainfall_mm_total`, `tempest_lightning_strike_count`→`tempest_lightning_strike_count_total`). Document that **`POSTGRES_BATCH_SIZE`/`POSTGRES_FLUSH_INTERVAL`/`POSTGRES_MAX_RETRIES` are now honored** (Task 0.13, B1 — previously inert). Document **`/api/almanac` proxies WeatherFlow** (B3) and that the **map basemap is a same-origin OSM `.pmtiles`** with required **OSM attribution** and no external tile server/API key (B2). Record P3 follow-ups (in-UI alerting, multi-station switcher, PWA, unit/theme profiles — design §14 P3) as a clearly-labeled "Future work (not implemented)" list.
- [ ] **Step 2:** Add a UI CI job: `npm ci --ignore-scripts && npm run lint && npx tsc --noEmit && npm test && npm run build`, plus `docker build` for both images; turn on `noUncheckedIndexedAccess` in `web/tsconfig.app.json`.
- [ ] **Step 3 (verify):** CI green locally (`actionlint`; run the UI steps).
- [ ] **Step 4:** Commit `docs: rewrite README for new architecture + add UI CI (G-H2, UI F-MEDIUM)`.

**Acceptance:** `actionlint` clean; UI CI steps pass; README quickstart is runnable.
**Implements:** design §16; resolves **G-H2**, **UI F-MEDIUM**; records design §14 P3 as follow-ups.

---

## Risks & Rollback

- **Interface-change ripple (Task 0.7):** `Close(ctx)` breaks all writers at once. Mitigation: the task updates every implementer + main in one commit; `go build ./...` gates it. Rollback: revert the single commit.
- **OTel logs API is experimental:** isolated in `internal/otel` (No-Wall). If a breaking bump lands, fall back to `slog`-only (drop the log bridge, keep metrics/traces). Confirm APIs via Context7 before coding.
- **Metric-name drift breaks the dashboard:** Task 4.1 asserts Contract B before Task 4.2 authors PromQL. If the Collector's translation differs from assumption, fix the assertion + instrument names before shipping the dashboard.
- **SQLite PRAGMA misconfig silently loses Litestream data:** Task 3.2 pins exact PRAGMAs and leaves checkpointing to Litestream; Task 3.6 proves restore. Do not set `wal_autocheckpoint`.
- **Sidecar image size / second moving part:** radar is opt-in; core stays a single static Go binary when `ENABLE_RADAR` is off. Sidecar failures soft-fail per Contract A. Rollback: disable `ENABLE_RADAR`.
- **Vendored UI drift:** `web/` is an owned fork pinned at `49892063` (recorded in `web/PROVENANCE.md`); upstream changes are cherry-picked deliberately.
- **`go:embed` empty-dir build failure:** `web/dist/.gitkeep` + documented build order (`task ui-build` before `go build`); the Dockerfile enforces the order.

## Definition of Done (whole effort)

- [ ] `go build ./...` clean; `go vet ./...` clean; `gofmt -l ./...` empty; `golangci-lint run ./...` clean.
- [ ] `go test -race ./...` passes (this includes the **default** Litestream file-replica restore test, Task 3.6 — proven on every run, not integration-gated); `go test -tags integration ./...` passes locally (optional S3/MinIO Litestream variant).
- [ ] `cd web && npm ci && npm run lint && npx tsc --noEmit && npm test && npm run build` all clean.
- [ ] `cd radar && pytest` passes (malformed-bytes case + committed-fixture isoband case).
- [ ] `docker buildx bake image-local` and `docker build radar/` succeed; both images run non-root; `/healthz` responds on each.
- [ ] `docker compose -f deploy/docker-compose.yml config` validates; smoke bring-up reaches Grafana with the Weather Nerd dashboard populated from live metrics; radar card renders (same-origin OSM `.pmtiles` basemap, OSM attribution visible) when `ENABLE_RADAR=true`.
- [ ] Every review CRITICAL/HIGH is resolved or superseded: exporter **C-1, A-H1..H3, C-H1, C-H2 (both SQLite and Postgres paths — Task 0.13/3.2/3.3), C-H3, D-H1..H2, F-H1..H4, G-H1..H3, B-HIGH**; UI **A-H1..H4, B-H1..H2, E-H1, F-H1**. (Each traced to a task above.)
- [ ] `instance`→`serial` label rename documented; `tempest_*` names preserved and asserted via the **real Prometheus exporter** (Task 4.1, Contract B); the complete metric-migration list emitted for the README.
- [ ] CSP is **self-only** (no external hosts); the map basemap is a same-origin OSM `.pmtiles` with visible OSM attribution; `/api/almanac` proxies WeatherFlow (B3).
- [ ] `POSTGRES_BATCH_SIZE`/`POSTGRES_FLUSH_INTERVAL`/`POSTGRES_MAX_RETRIES` take effect (Task 0.13); `SQLITE_*` tunables take effect.
- [ ] README quickstart is runnable; operational-mode matrix matches reality; P3 items recorded as explicit follow-ups (not built).
- [ ] Fresh cold-review on the full diff from branch point completed; findings addressed.

**Task count:** 46 tasks (was 42; +4 net). WS0: 14 (0.1–0.8, 0.9a, 0.9b, 0.10, 0.11, 0.12, 0.13 — the 0.9→0.9a/0.9b split and new 0.13); WS3: 6 (3.1–3.6); WS6: 5 (6.1–6.5); WS1: 10 (1.1–1.6, 1.7a, 1.7b, 1.7c, 1.8 — the 1.7→1.7a/b/c split); WS2: 6 (2.1–2.6); WS4: 2 (4.1–4.2); WS5: 1 (UX.1); DOC: 2 (DOC.1–DOC.2). Tasks added/split: **+0.13** (B1 Postgres tunables), **0.9→0.9a/0.9b** (per-writer gate), **1.7→1.7a/1.7b/1.7c** (right-sizing).
