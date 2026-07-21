# Design — Records summary card (windowed observation aggregates)

**Type:** design
**Date:** 2026-07-21
**Feature:** Records summary card
**Status:** approved (visual validated in-browser across all 7 themes); ready for implementation planning.

## 1. Goal

Add a **Records** card to the dashboard, full-width directly above the 7-Day Forecast, showing aggregate weather records over a user-selected rolling window (7 / 30 / 180 / 365 days). Metrics:

| Tile | Aggregate | Source column |
|------|-----------|---------------|
| Temperature | max & min | `air_temperature` |
| Humidity | max & min | `relative_humidity` |
| Pressure | max & min | `station_pressure` |
| Wind | max sustained & max gust (one tile) | `wind_avg`, `wind_gust` |
| Rain total | sum | `rain_rate` (per-interval accumulation, mm) |
| Lightning | sum (total strikes) | `lightning_strike_count` |

The values are computed from the local SQLite observation store (the same store the dashboard already reads), **not** from the token-gated WeatherFlow almanac.

## 2. Why a new backend endpoint (not the existing history read)

The existing `HistoryPoints(field, from, to)` returns raw samples of **one** field, capped at `maxHistoryPoints = 10000`, with no aggregation. Computing min/max/sum over up to 365 days (~525k rows) × multiple fields client-side is both **incorrect** (the 10k cap truncates) and **infeasible** (payload). A single server-side SQL aggregation query returns one small object correctly and cheaply, so the feature adds a dedicated summary endpoint. (This does not touch or change `HistoryPoints`; storage is unaffected — no data is lost or pruned.)

## 3. Backend

### 3.1 SQLite aggregation method
New method on the SQLite reader (`internal/sqlite`), sibling to `LatestObservation`/`HistoryPoints`:

```
SummarizeObservations(ctx, from, to int64) (Summary, error)
```

One query over `tempest_observations WHERE timestamp BETWEEN ? AND ?`:
- `MIN`/`MAX` of `air_temperature`, `relative_humidity`, `station_pressure`
- `MAX` of `wind_avg`, `wind_gust`
- `SUM` of `rain_rate` (window rain total, mm) and `lightning_strike_count` (total strikes)
- `COUNT(*)` and `MIN(timestamp)`/`MAX(timestamp)` — so the UI knows actual coverage (partial-data awareness)

The range scan is index-backed by the existing `UNIQUE(serial_number, timestamp)`. Aggregation runs on the single writer connection (`SetMaxOpenConns(1)`); this is acceptable because the endpoint is called on load and on window change, **not** on the 3s/30s tick (not a hot path). `rain_rate` holds the Tempest `ob[12]` per-interval rain accumulation (mm), so `SUM` is the correct window total — documented at the query.

`Summary` fields are nullable/zero-aware: an empty window (no rows) yields a well-formed "no data" result (counts 0), not an error.

### 3.2 HTTP handler
New route in `internal/httpserver`:

```
GET /api/observations/summary?days=N
```

- Validate `N ∈ {7, 30, 180, 365}`; otherwise `400`.
- `to = now`, `from = now − N·86400` (rolling window, UTC seconds).
- Response: Contract-C JSON in **SI units** (°C, mb, m/s, mm) — consistent with `/current` and `/history`; the frontend formats to the user's unit prefs. Shape includes each metric's aggregate plus `count`, `coveredFrom`, `coveredTo`.
- Reuses the existing `Observations` dependency seam on the server (extend it with `SummarizeObservations`).

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
(Exact field names finalized in the plan; frontend `types/weather.ts` is the single source, mirrored by the Go struct's JSON tags — same discipline as Contract C elsewhere.)

## 4. Frontend

### 4.1 RecordsCard component
- New `web/src/components/RecordsCard.tsx`, `React.memo` (consistent with the W5 memoization pass).
- Full-width in the dashboard grid (`grid-column: 1 / -1`), inserted in `App.tsx` immediately **before** `<ForecastStrip>`.
- Layout (validated in-browser across all 7 themes):
  - Header: "Records" title + a right-aligned **window pill** ("Last 7 days", reflecting the selected window).
  - Responsive tile grid (`repeat(auto-fit, minmax(150px, 1fr))`), 6 tiles: **Temperature · Humidity · Pressure · Wind · Rain total · Lightning**.
  - Paired tiles (Temp/Humidity/Pressure, and Wind) render **two rows**: label on the left, value **right-aligned with `font-variant-numeric: tabular-nums`** so digits line up. No ▲▼ arrows.
  - **Labels (`High`/`Low`/`Sustained`/`Gust`) use `var(--text-secondary)`, NOT semantic `--danger`/`--info`** — the theme matrix proved red/blue labels wash out on the light themes (desert-sunset, liquid-glass). This is a hard requirement (same contrast trap as the W5 toggle bug).
  - Single-value tiles (Rain total, Lightning) show the value + a muted unit suffix. Every value carries its unit (including pressure).
  - CSS lives in `web/src/App.css` reusing the existing per-theme custom properties + `.glass-card`; enumerated transitions, `:focus-visible` where interactive, dvh where a viewport height is used — matching the W5 conventions.
- Values format to the user's unit prefs via the existing `useUnits` helpers (temp F/C, wind mph/kph/ms/kts, pressure inHg/mb/hPa, rain in/mm).

### 4.2 Data
- Fetched in `useWeatherData` (or a small dedicated hook) keyed on the selected window pref: `GET /api/observations/summary?days=<pref>`. Re-fetches when the window pref changes. AbortController + failure handling consistent with the existing fetch layer (retain prior data on transient failure).

### 4.3 Settings — window selector
- New "Records window" segmented control in `SettingsPanel` (7 / 30 / 180 / 365 days), reusing the W5 `.toggle-group` styling.
- Persisted in `UserPreferences` (localStorage), new field `recordsWindowDays`, **default 7**.

## 5. Edge cases
- **Partial data** (fresh DB with < window of history): aggregates over what exists; the header may indicate real coverage from `coveredFrom`/`coveredTo` (e.g. still labeled "Last 7 days" but data may be shorter). No error.
- **No data** (`count == 0`): tiles render em-dashes / "No data yet".
- **Loading**: placeholder/skeleton; on fetch failure, retain last good summary if present.
- **NaN/valid guards**: skip/represent missing aggregates safely (mirrors the existing null-safe formatting).

## 6. Testing
- **Go:** `SummarizeObservations` SQL correctness (seed rows spanning a window, assert min/max/sum/count/coverage; empty-window case) + handler param validation (`days` allowlist, 400s) + Contract-C JSON shape.
- **TS:** RecordsCard rendering (paired vs single tiles, unit formatting, window-pill label), Settings window control (persist + re-fetch), empty/partial states. RED-first for behavior-bearing bits.
- **In-browser gate:** build the binary embedding the UI, seed a spread of observations, confirm the card renders real aggregates and switches with the window selector — and re-run the theme matrix for label legibility.

## 7. Out of scope (YAGNI / follow-ups)
- Multi-station serial scoping (single-station assumption today; add a serial filter when a real multi-station need arrives).
- Per-metric independent windows (one global window controls the whole card).
- Calculated-value records (e.g. max wet-bulb) — could be added as tiles later; the derived-values work is tracked in issue #78.
- Historical drill-down charts (separate feature).

## 8. Principles check
- **DRY:** unit formatting reuses `useUnits`; the summary JSON shape is single-sourced (TS types ↔ Go tags); the aggregation is one query, one place.
- **KISS:** one endpoint, one SQL query, one card; no client-side aggregation, no caching layer (not needed at this call frequency).
- **No-Wall:** the summary endpoint is additive (new handler + new sqlite method + new component), siblings untouched; a future metric is a new column in the query + a new tile.
- **YAGNI:** rolling window only; no per-metric windows, no multi-station, no export.
