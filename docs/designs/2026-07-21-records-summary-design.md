# Design — Records summary card (windowed observation aggregates)

**Type:** design
**Date:** 2026-07-21
**Feature:** Records summary card
**Status:** approved (visual validated in-browser across all 7 themes) + SGE (Fable) backend review folded in — "ready with changes" resolved below. Ready for implementation planning.

## 1. Goal

Add a **Records** card to the dashboard, full-width directly above the 7-Day Forecast, showing aggregate weather records over a user-selected rolling window (7 / 30 / 180 / 365 days). Metrics:

| Tile | Aggregate | DB column (`tempest_observations`) |
|------|-----------|-------------------------------------|
| Temperature | max & min | `temp_air` |
| Humidity | max & min | `humidity` |
| Pressure | max & min | `pressure` |
| Wind | max sustained & max gust (one tile) | `wind_avg`, `wind_gust` |
| Rain total | sum | `rain_rate` (Tempest `obs_st[12]` = per-interval accumulation, mm) |
| Lightning | sum (total strikes) | `lightning_strike_count` |

> Column names are the **actual DB columns** (verified in `migrations/0001_init.sql` / `observationRow` in `writer.go`), not the Contract-C wire names — the SQL must use these.

The values are computed from the local SQLite observation store (the same store the dashboard already reads), **not** from the token-gated WeatherFlow almanac. (Keep the new type/naming clearly distinct from the existing `StationAlmanac`/`TempRecord` almanac types in `weather.ts`.)

## 2. Why a new backend endpoint (not the existing history read)

`HistoryPoints(field, from, to)` returns raw samples of **one** field, capped at `maxHistoryPoints = 10000`, no aggregation. Computing min/max/sum over up to 365 days (~525k rows) × multiple fields client-side is both **incorrect** (the 10k cap truncates) and **infeasible** (payload). One server-side SQL aggregation returns a small object correctly and cheaply. This does not touch `HistoryPoints`; storage is unaffected (no data lost or pruned).

## 3. Backend

### 3.0 Read path & writer-starvation mitigation (resolved from SGE review — REQUIRED)

The aggregation is a deliberately heavy read, and the design must not starve the ingest writer. Three points, in priority order:

1. **Timestamp index (required, new migration `0002`).** The existing indexes (`UNIQUE(serial_number, timestamp)` on observations and rapid_wind) lead with `serial_number`; a `WHERE timestamp BETWEEN ?` query with **no** `serial_number` predicate cannot use them, so it would full-scan the table on *every* call (even 7-day windows). Add, via the embedded `schema_version`/`Migrate` path:
   ```sql
   -- 0002_obs_timestamp_index.sql
   CREATE INDEX IF NOT EXISTS idx_obs_timestamp ON tempest_observations(timestamp);
   ```
   so 7/30-day windows scan only their slice.
2. **Dedicated read-only `*sql.DB` for reads (chosen approach).** The ingest path opens the DB with `SetMaxOpenConns(1)` (`db.go`) to serialize *writes*; reads currently share that one connection (so `/current`'s `LatestObservation` + 3h `HistoryPoints`, and this new summary, all queue behind the writer). Open a **second, read-only `*sql.DB`** (WAL journal mode — already set — supports one writer concurrent with many readers without `SQLITE_BUSY`) and route `SummarizeObservations` (and, additively, the other reads) through it. This decouples heavy reads from ingest entirely and de-risks the existing `/current`+`/history` fan-out too. It removes coupling — a clean seam, not speculative structure. *(Lighter fallback if we choose not to add the read handle now: keep the single connection but rely on the index + the deadline below. The read handle is the recommended fix.)*
3. **Context deadline (required regardless of 1–2).** `SummarizeObservations` runs under a `context` with a few-second deadline; a timeout maps to `503`/`500` so a pathological scan can never stall ingest unboundedly.

### 3.1 SQLite aggregation method
New method on the SQLite reader (`internal/sqlite`), sibling to `LatestObservation`/`HistoryPoints`:

```
SummarizeObservations(ctx, from, to int64) (Summary, error)
```

One query over `tempest_observations WHERE timestamp BETWEEN ? AND ?`:
- `MIN`/`MAX` of `temp_air`, `humidity`, `pressure`
- `MAX` of `wind_avg`, `wind_gust`
- `SUM` of `rain_rate` (window rain total, mm — correct because `obs_st[12]` is per-interval accumulation and rows are deduped by `ON CONFLICT(serial_number,timestamp) DO NOTHING`) and `lightning_strike_count` (total strikes)
- `COUNT(*)` and `MIN(timestamp)`/`MAX(timestamp)` — actual coverage (partial-data awareness)

**NULL handling is explicit, not implicit.** Over 0 rows (or all-NULL columns) `MIN/MAX/SUM` return `NULL`, not 0. The Go scan therefore uses `sql.NullFloat64`/`sql.NullInt64` for every aggregate and nullable `coveredFrom`/`coveredTo`, and treats `COUNT(*) == 0` as the empty-window signal. `lightning_strike_count` is itself a nullable column, so a partial-NULL window is a real case (covered by a test).

### 3.2 HTTP handler
New route in `internal/httpserver`, extending the existing **`ObservationReader`** interface (the `Deps.Observations` seam) with `SummarizeObservations` — additive, implemented by `*sqlite.Writer`, test fake updated (No-Wall):

```
GET /api/observations/summary?days=N
```

- Validate `N ∈ {7, 30, 180, 365}`. **Missing or invalid `days` → `400`** (deliberately stricter than `/history`, which degrades to defaults — called out so it's intentional, not an oversight).
- `to = now`, `from = now − N·86400` (rolling window; UTC epoch seconds — TZ-independent, no DST/calendar pitfalls).
- **Error/HTTP mapping** (matching existing handlers): reader `nil` → `503`; query error/timeout → `500`/`503`; empty window (`count == 0`) → `200` with a well-formed zero/no-data body.
- Response: Contract-C JSON in **SI units** (°C, mb, m/s, mm) — consistent with `/current`; the frontend formats to the user's unit prefs.

### 3.3 Contract-C shape (illustrative)
```json
{
  "window": { "days": 7, "from": 1719000000, "to": 1719604800 },
  "count": 9720,
  "coveredFrom": 1719000600, "coveredTo": 1719604700,
  "temperature": { "max": 25.6, "min": 5.0 },
  "humidity":    { "max": 94.0, "min": 38.0 },
  "pressure":    { "max": 1023.4, "min": 1003.8 },
  "windMax":     15.2,
  "gustMax":     22.1,
  "rainTotal":   31.5,
  "lightningTotal": 12
}
```
The **TS type is the single source**: the plan adds a `RecordsSummary` interface to `web/src/types/weather.ts` first, and the Go struct's JSON tags mirror it. Note the nested `{max,min}` objects are a new pattern vs `/current`'s flat fields — a deliberate choice, not accidental. Missing aggregates (empty/partial window) serialize as `null` and the UI renders them safely. Exact field names finalized in the plan.

## 4. Frontend

### 4.1 RecordsCard component
- New `web/src/components/RecordsCard.tsx`, `React.memo` (consistent with the W5 memoization pass).
- Full-width in the dashboard grid (`grid-column: 1 / -1`), inserted in `App.tsx` immediately **before** `<ForecastStrip>`.
- Layout (validated in-browser across all 7 themes):
  - Header: "Records" title + right-aligned **window pill** ("Last 7 days", reflecting the selected window).
  - Responsive tile grid (`repeat(auto-fit, minmax(150px, 1fr))`), 6 tiles: **Temperature · Humidity · Pressure · Wind · Rain total · Lightning**.
  - Paired tiles (Temp/Humidity/Pressure, and Wind) render **two rows**: label on the left, value **right-aligned with `font-variant-numeric: tabular-nums`** so digits line up. No ▲▼ arrows.
  - **Labels (`High`/`Low`/`Sustained`/`Gust`) use `var(--text-secondary)`, NOT semantic `--danger`/`--info`** — the theme matrix proved red/blue labels wash out on the light themes (desert-sunset, liquid-glass). Hard requirement (same contrast trap as the W5 toggle bug).
  - Single-value tiles (Rain total, Lightning) show value + muted unit suffix. Every value carries its unit (including pressure).
  - CSS in `web/src/App.css` reusing the per-theme custom properties + `.glass-card`; enumerated transitions, `:focus-visible` where interactive, dvh where a viewport height is used — matching W5 conventions.
- Values format to the user's unit prefs via the existing `useUnits` helpers (temp F/C, wind mph/kph/ms/kts, pressure inHg/mb/hPa, rain in/mm).

### 4.2 Data
- Fetched in `useWeatherData` (or a small dedicated hook) keyed on the selected window pref: `GET /api/observations/summary?days=<pref>`. Re-fetches when the window pref changes. AbortController + failure handling consistent with the existing fetch layer (retain prior data on transient failure).

### 4.3 Settings — window selector
- New "Records window" segmented control in `SettingsPanel` (7 / 30 / 180 / 365 days), reusing the W5 `.toggle-group` styling.
- Persisted in `UserPreferences` (localStorage), new field `recordsWindowDays`, **default 7**.

## 5. Edge cases
- **Partial data** (fresh DB with < window of history): aggregates over what exists; header may indicate real coverage from `coveredFrom`/`coveredTo`. No error.
- **No data** (`count == 0`): tiles render em-dashes / "No data yet".
- **Loading**: placeholder/skeleton; on fetch failure, retain last good summary if present.
- **NULL aggregates**: `sql.Null*` scan + `null` JSON → UI renders em-dash (mirrors existing null-safe formatting).

## 6. Testing
- **Go:** `SummarizeObservations` correctness (seed rows spanning a window; assert min/max/sum/count/coverage); **empty-window (0 rows) → all-null result**; **partial-NULL `lightning_strike_count` window** and an all-NULL-column window (prove `sql.Null*` handling); a **large-seed** case to sanity-check the `0002` index actually restricts the scan. Handler param validation (`days` allowlist incl. missing/invalid → 400) + Contract-C JSON shape + error mapping (nil reader → 503).
- **TS:** RecordsCard rendering (paired vs single tiles, unit formatting, window-pill label), Settings window control (persist + re-fetch), empty/partial/loading states. RED-first for behavior-bearing bits.
- **In-browser gate:** build the binary embedding the UI, seed a spread of observations across the window, confirm the card renders real aggregates and switches with the window selector — and re-run the theme matrix for label legibility.

## 7. Out of scope (YAGNI / follow-ups)
- Multi-station serial scoping (single-station assumption today, consistent with `LatestObservationAny`; add a serial filter when a real multi-station need arrives).
- Per-metric independent windows (one global window controls the whole card).
- Calculated-value records (e.g. max wet-bulb) — could be added as tiles later; derived-values work tracked in issue #78.
- Historical drill-down charts (separate feature).

## 8. Principles check
- **DRY:** unit formatting reuses `useUnits`; summary JSON shape single-sourced (TS types ↔ Go tags); one aggregation query, one place.
- **KISS:** one endpoint, one SQL query, one card; no client-side aggregation, no caching. The one under-KISS spot the SGE flagged — reusing the writer's single connection for a heavy read — is fixed by §3.0 (the read handle *removes* coupling, it doesn't add speculative structure).
- **No-Wall:** additive throughout (new handler + new reader method + new component + new migration); a future metric is a new column in the query + a new tile; the read-only `*sql.DB` is a clean seam.
- **YAGNI:** rolling window only; no per-metric windows, no multi-station, no export.
