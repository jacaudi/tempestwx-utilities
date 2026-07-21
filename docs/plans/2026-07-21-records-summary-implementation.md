# Records Summary Card Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **For Claude:** REQUIRED EXECUTION WORKFLOW (follow in order):
> 1. `superpowers:using-git-worktrees` — already in worktree `.claude/worktrees/records-summary` (branch `worktree-records-summary`, off `origin/main` eee1153).
> 2. `superpowers:subagent-driven-development` — fresh subagent per task; model topology: Go tasks → **sr-go-engineer/Sonnet**, TS tasks → **general-purpose/Sonnet**; per-task review **general-purpose/Opus**; final cold whole-branch review **general-purpose/Fable**.
> 3. `superpowers:test-driven-development` — every task RED-first.
> 4. `superpowers:verification-before-completion` — per task.
> 5. After all tasks: cold whole-branch review + **in-browser gate** (build the binary, seed observations across a window, confirm the card renders/aggregates/switches; re-run the theme matrix).
> 6. `superpowers:finishing-a-development-branch` — USER-GATED (jacaudi's own repo; merge-commit precedent).
> Skills carry their own model/effort settings. Do not override them.

**Goal:** Add a full-width "Records" card above the 7-day forecast showing windowed observation aggregates (max/min temp/humidity/pressure, combined sustained+gust wind, rain total, lightning total) over a 7/30/180/365-day rolling window selectable in Settings.

**Architecture:** A new server-side SQL aggregation endpoint (`GET /api/observations/summary?days=N`) reading SQLite through a **dedicated read-only `*sql.DB`** (WAL enables readers concurrent with the single ingest writer, decoupling the heavy scan from the write connection); the timestamp index `idx_obs_time` (migration `0002`, already on main) serves the range. Contract-C SI-unit JSON, formatted to the user's unit prefs client-side; a memoized `RecordsCard` React component + a Settings window selector.

**Tech Stack:** Go 1.25 (`modernc.org/sqlite`, `database/sql`, `net/http`, `CGO_ENABLED=0`); Vite + React 18 + TypeScript + Vitest/RTL.

## Global Constraints

- CGO-free: `CGO_ENABLED=0` stays buildable — pure-Go `modernc.org/sqlite` only, never `mattn/go-sqlite3`.
- Contract C: `web/src/types/weather.ts` is the **single source** of JSON field names; the Go wire struct's `json:` tags mirror it exactly. Backend emits **SI units** (°C, mb, m/s, mm); the frontend formats to prefs via `useUnits`. New type is `RecordsSummary`, kept distinct from the existing `StationAlmanac`/`TempRecord` almanac types.
- Real DB columns (NOT Contract-C names): `temp_air`, `humidity`, `pressure`, `wind_avg`, `wind_gust`, `rain_rate` (per-interval mm accumulation), `lightning_strike_count`. Timestamps are UTC epoch-second integers.
- `days` allowlist: exactly `{7, 30, 180, 365}`; missing/invalid → HTTP 400 (deliberately stricter than `/history`).
- NULL discipline: `MIN/MAX/SUM` over 0 rows (or all-NULL columns) return SQL `NULL`, not 0; scan with `sql.Null*`; `COUNT(*)==0` is the empty signal; absent aggregates serialize as JSON `null` and render as em-dash.
- Records-window labels (`High`/`Low`/`Sustained`/`Gust`) use `var(--text-secondary)`, never `--danger`/`--info` (theme-contrast requirement; verified across all 7 themes).
- The read-only handle uses `mode=ro` and **must not** set `journal_mode` (a read-only connection can't run that write PRAGMA; the DB is already WAL). It is opened AFTER the write handle (whose `Open` runs migrations and creates the `-wal`/`-shm` files).

## File Structure

- `internal/sqlite/db.go` — add `OpenReadOnly`.
- `internal/sqlite/writer.go` — add `readDB` field + `WithReadDB` option; route reads through `readDB`; add `Summary` + `SummarizeObservations`.
- `internal/sqlite/summary_test.go` — new; `SummarizeObservations` tests.
- `internal/httpserver/observations.go` — extend `ObservationReader`; add `summaryResponse` wire shape + `handleSummary`; register route.
- `internal/httpserver/observations_test.go` — extend `fakeObservationReader`; handler tests.
- `main.go` — open the read-only handle, pass `WithReadDB`, close it on shutdown.
- `web/src/types/weather.ts` — add `RecordsSummary`; add `recordsWindowDays` to `UserPreferences`.
- `web/src/api/tempestApi.ts` — add `ENDPOINTS.summary` + `fetchRecordsSummary`.
- `web/src/hooks/useWeatherData.ts` — fetch the summary keyed on the window pref.
- `web/src/hooks/usePreferences.ts` (or wherever prefs default lives) — default `recordsWindowDays: 7`.
- `web/src/components/RecordsCard.tsx` (+ `RecordsCard.test.tsx`) — new component.
- `web/src/components/SettingsPanel.tsx` (+ test) — window selector.
- `web/src/App.tsx` — render `<RecordsCard>` before `<ForecastStrip>`.
- `web/src/App.css` — Records card CSS.

---

## Task 1: Read-only SQLite handle + route reads through it

**Files:**
- Modify: `internal/sqlite/db.go` (add `OpenReadOnly`), `internal/sqlite/writer.go` (`readDB` field, `WithReadDB`, route reads), `main.go` (wire + close).
- Test: `internal/sqlite/db_test.go` (or `writer_test.go`).

**Interfaces:**
- Produces: `func OpenReadOnly(ctx context.Context, path string, cfg Config) (*sql.DB, error)`; `type WriterOption func(*Writer)`; `func WithReadDB(db *sql.DB) WriterOption`; `NewWriter(ctx, db, cfg, opts ...WriterOption)` (variadic — existing callers unchanged).
- Consumes: existing `Open`, `Config` (has `BusyTimeout`), `driverName`.

- [ ] **Step 1: Write the failing test** — `internal/sqlite/db_test.go`:
```go
func TestOpenReadOnly_ReadsWriterData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.db")
	cfg := Config{BusyTimeout: 5000, BatchSize: 1, FlushInterval: time.Second}
	ctx := context.Background()

	wdb, err := Open(ctx, path, cfg) // runs migrations, creates WAL
	if err != nil { t.Fatal(err) }
	defer wdb.Close()

	// seed one observation row directly
	_, err = wdb.ExecContext(ctx, `INSERT INTO tempest_observations
		(id, serial_number, timestamp, temp_air) VALUES ('a','ST-1',100,21.5)`)
	if err != nil { t.Fatal(err) }

	rdb, err := OpenReadOnly(ctx, path, cfg)
	if err != nil { t.Fatalf("OpenReadOnly: %v", err) }
	defer rdb.Close()

	var got float64
	if err := rdb.QueryRowContext(ctx, `SELECT temp_air FROM tempest_observations WHERE id='a'`).Scan(&got); err != nil {
		t.Fatalf("read via read-only handle: %v", err)
	}
	if got != 21.5 { t.Fatalf("got %v want 21.5", got) }
}
```
- [ ] **Step 2: Run — expect FAIL** (`OpenReadOnly` undefined). `go test ./internal/sqlite/ -run TestOpenReadOnly -v`
- [ ] **Step 3: Implement `OpenReadOnly`** in `db.go`:
```go
// OpenReadOnly opens a read-only handle to an existing SQLite database for
// query-side work. It must be opened AFTER Open (which creates/migrates the
// DB and its WAL files). WAL journaling lets these read connections run
// concurrently with the single ingest writer without SQLITE_BUSY. It does
// NOT set journal_mode (a read-only connection can't run that write PRAGMA;
// the DB is already WAL from Open).
func OpenReadOnly(ctx context.Context, path string, cfg Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(%d)&_pragma=foreign_keys(1)", path, cfg.BusyTimeout)
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}
	db.SetMaxOpenConns(4) // concurrent readers alongside the writer (WAL)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite read-only: %w", err)
	}
	return db, nil
}
```
- [ ] **Step 4: Add `readDB` + `WithReadDB`** in `writer.go`. Add field `readDB *sql.DB` to the `Writer` struct. Add:
```go
// WriterOption configures a Writer at construction.
type WriterOption func(*Writer)

// WithReadDB routes read methods (LatestObservation*, HistoryPoints,
// SummarizeObservations) through a dedicated read-only handle instead of the
// single write connection, so heavy reads never queue behind ingest.
func WithReadDB(db *sql.DB) WriterOption {
	return func(w *Writer) { w.readDB = db }
}
```
In `NewWriter`, change signature to `func NewWriter(ctx context.Context, db *sql.DB, cfg Config, opts ...WriterOption) *Writer`; after building `w`, apply `for _, o := range opts { o(w) }`, then `if w.readDB == nil { w.readDB = db }`. Change the read methods `LatestObservation`, `LatestObservationAny`, `HistoryPoints` to use `w.readDB` instead of `w.db` (writes stay on `w.db`).
- [ ] **Step 5: Run — expect PASS.** Also `go test ./internal/sqlite/ ./internal/httpserver/ -race` (existing reads still pass; `readDB` defaults to `db`).
- [ ] **Step 6: Wire in `main.go`.** After `db, err := sqlite.Open(ctx, choice.sqlitePath, sqliteCfg)` (~line 296) and its error check, add:
```go
rdb, err := sqlite.OpenReadOnly(ctx, choice.sqlitePath, sqliteCfg)
if err != nil {
	return fmt.Errorf("open sqlite read handle: %w", err)
}
```
Change the `sw = sqlite.NewWriter(ctx, db, sqliteCfg)` call to `sw = sqlite.NewWriter(ctx, db, sqliteCfg, sqlite.WithReadDB(rdb))`. Ensure `rdb.Close()` is called on shutdown wherever `db.Close()` is (close `rdb` before/after `db` — order doesn't matter, both after the writer drains).
- [ ] **Step 7: Verify build + run-the-binary smoke.** `CGO_ENABLED=0 go build ./... && go vet ./...`; then start the binary with a temp `SQLITE_PATH`, confirm it boots + `/healthz` 200 (read handle opens cleanly).
- [ ] **Step 8: Commit** `feat(sqlite): read-only handle for query-side reads (decouple from ingest writer)`.

---

## Task 2: `SummarizeObservations` aggregation

**Files:**
- Modify: `internal/sqlite/writer.go`.
- Test: `internal/sqlite/summary_test.go` (new).

**Interfaces:**
- Produces: `type Summary struct {…}` and `func (w *Writer) SummarizeObservations(ctx context.Context, from, to int64) (Summary, error)`.
- Consumes: `w.readDB` (Task 1).

`Summary` (all aggregates nullable; `Count` drives emptiness):
```go
type Summary struct {
	Count          int64
	CoveredFrom    sql.NullInt64
	CoveredTo      sql.NullInt64
	TempMax        sql.NullFloat64
	TempMin        sql.NullFloat64
	HumidityMax    sql.NullFloat64
	HumidityMin    sql.NullFloat64
	PressureMax    sql.NullFloat64
	PressureMin    sql.NullFloat64
	WindMax        sql.NullFloat64
	GustMax        sql.NullFloat64
	RainTotal      sql.NullFloat64
	LightningTotal sql.NullInt64
}
```

- [ ] **Step 1: Write the failing test** — `summary_test.go`. Seed rows spanning [100,300]; assert aggregates; assert empty window (no rows) → `Count==0` and all `Valid==false`; assert a window with a NULL `lightning_strike_count` row still sums the non-null ones:
```go
func TestSummarizeObservations(t *testing.T) {
	w, db := newTestWriter(t) // helper: Open + NewWriter (readDB defaults to db)
	ctx := context.Background()
	seed := func(id string, ts int64, temp, hum, pres, wavg, wgust, rain float64, ls sql.NullInt64) {
		_, err := db.ExecContext(ctx, `INSERT INTO tempest_observations
			(id, serial_number, timestamp, temp_air, humidity, pressure, wind_avg, wind_gust, rain_rate, lightning_strike_count)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			id, "ST-1", ts, temp, hum, pres, wavg, wgust, rain, ls)
		if err != nil { t.Fatal(err) }
	}
	seed("a", 100, 10, 40, 1000, 3, 5, 0.5, sql.NullInt64{Int64: 2, Valid: true})
	seed("b", 200, 25, 90, 1020, 8, 12, 1.0, sql.NullInt64{}) // NULL lightning
	seed("c", 300, 18, 60, 1010, 5, 9, 0.25, sql.NullInt64{Int64: 3, Valid: true})

	s, err := w.SummarizeObservations(ctx, 100, 300)
	if err != nil { t.Fatal(err) }
	if s.Count != 3 { t.Fatalf("count=%d", s.Count) }
	if s.TempMax.Float64 != 25 || s.TempMin.Float64 != 10 { t.Fatalf("temp %v/%v", s.TempMax, s.TempMin) }
	if s.WindMax.Float64 != 8 || s.GustMax.Float64 != 12 { t.Fatalf("wind %v/%v", s.WindMax, s.GustMax) }
	if math.Abs(s.RainTotal.Float64-1.75) > 1e-9 { t.Fatalf("rain %v", s.RainTotal) }
	if s.LightningTotal.Int64 != 5 { t.Fatalf("lightning=%d (want 5, NULL row skipped)", s.LightningTotal.Int64) }
	if s.CoveredFrom.Int64 != 100 || s.CoveredTo.Int64 != 300 { t.Fatalf("coverage %v/%v", s.CoveredFrom, s.CoveredTo) }

	empty, err := w.SummarizeObservations(ctx, 1000, 2000)
	if err != nil { t.Fatal(err) }
	if empty.Count != 0 || empty.TempMax.Valid || empty.RainTotal.Valid || empty.CoveredFrom.Valid {
		t.Fatalf("empty window not all-null: %+v", empty)
	}
}
```
- [ ] **Step 2: Run — expect FAIL** (`SummarizeObservations` undefined).
- [ ] **Step 3: Implement.** Add the SQL const and method (uses `w.readDB`):
```go
const summarizeObservationsSQL = `
SELECT
  COUNT(*),
  MIN(timestamp), MAX(timestamp),
  MAX(temp_air),  MIN(temp_air),
  MAX(humidity),  MIN(humidity),
  MAX(pressure),  MIN(pressure),
  MAX(wind_avg),  MAX(wind_gust),
  SUM(rain_rate),                 -- rain_rate = obs_st[12], per-interval mm accumulation → SUM = total mm
  SUM(lightning_strike_count)
FROM tempest_observations
WHERE timestamp BETWEEN ? AND ?`

// SummarizeObservations aggregates the observations in [from, to] into one
// Summary. Aggregates over 0 rows (or all-NULL columns) are SQL NULL, surfaced
// via sql.Null*; Count==0 signals an empty window. Runs on the read-only
// handle so a wide scan never queues behind the ingest writer.
func (w *Writer) SummarizeObservations(ctx context.Context, from, to int64) (Summary, error) {
	var s Summary
	err := w.readDB.QueryRowContext(ctx, summarizeObservationsSQL, from, to).Scan(
		&s.Count, &s.CoveredFrom, &s.CoveredTo,
		&s.TempMax, &s.TempMin, &s.HumidityMax, &s.HumidityMin,
		&s.PressureMax, &s.PressureMin, &s.WindMax, &s.GustMax,
		&s.RainTotal, &s.LightningTotal,
	)
	if err != nil {
		return Summary{}, fmt.Errorf("summarize observations: %w", err)
	}
	return s, nil
}
```
- [ ] **Step 4: Run — expect PASS.** `go test ./internal/sqlite/ -run TestSummarizeObservations -v`
- [ ] **Step 5: Commit** `feat(sqlite): SummarizeObservations windowed aggregate`.

---

## Task 3: `GET /api/observations/summary` handler

**Files:**
- Modify: `internal/httpserver/observations.go`.
- Test: `internal/httpserver/observations_test.go`.

**Interfaces:**
- Consumes: `Summary`, `SummarizeObservations` (Task 2); the `Deps.Observations` seam.
- Produces: `GET /api/observations/summary?days=N` → `summaryResponse` JSON.

Add `SummarizeObservations(ctx, from, to int64) (sqlite.Summary, error)` to the `ObservationReader` interface. Wire shape (mirrors `web/src/types/weather.ts` `RecordsSummary` from Task 4):
```go
type summaryMinMax struct {
	Max *float64 `json:"max"`
	Min *float64 `json:"min"`
}
type summaryWindow struct {
	Days int   `json:"days"`
	From int64 `json:"from"`
	To   int64 `json:"to"`
}
type summaryResponse struct {
	Window         summaryWindow `json:"window"`
	Count          int64         `json:"count"`
	CoveredFrom    *int64        `json:"coveredFrom"`
	CoveredTo      *int64        `json:"coveredTo"`
	Temperature    summaryMinMax `json:"temperature"`
	Humidity       summaryMinMax `json:"humidity"`
	Pressure       summaryMinMax `json:"pressure"`
	WindMax        *float64      `json:"windMax"`
	GustMax        *float64      `json:"gustMax"`
	RainTotal      *float64      `json:"rainTotal"`
	LightningTotal *int64        `json:"lightningTotal"`
}

const summaryQueryTimeout = 5 * time.Second
var allowedSummaryDays = map[int]bool{7: true, 30: true, 180: true, 365: true}

func f64(n sql.NullFloat64) *float64 { if n.Valid { v := n.Float64; return &v }; return nil }
func i64(n sql.NullInt64) *int64     { if n.Valid { v := n.Int64;   return &v }; return nil }
```

- [ ] **Step 1: Write the failing test** — extend `fakeObservationReader` with a `SummarizeObservations` field/func, then:
```go
func TestHandleSummary_OK(t *testing.T) {
	reader := &fakeObservationReader{summary: sqlite.Summary{
		Count: 5, CoveredFrom: sql.NullInt64{Int64: 10, Valid: true}, CoveredTo: sql.NullInt64{Int64: 90, Valid: true},
		TempMax: sql.NullFloat64{Float64: 25, Valid: true}, TempMin: sql.NullFloat64{Float64: 5, Valid: true},
		WindMax: sql.NullFloat64{Float64: 8, Valid: true}, GustMax: sql.NullFloat64{Float64: 12, Valid: true},
		RainTotal: sql.NullFloat64{Float64: 3.5, Valid: true}, LightningTotal: sql.NullInt64{Int64: 4, Valid: true},
	}}
	rr := httptest.NewRecorder()
	newTestServer(reader).ServeHTTP(rr, httptest.NewRequest("GET", "/api/observations/summary?days=7", nil))
	if rr.Code != 200 { t.Fatalf("code=%d", rr.Code) }
	var got summaryResponse
	json.Unmarshal(rr.Body.Bytes(), &got)
	if got.Window.Days != 7 || got.Count != 5 || *got.Temperature.Max != 25 || *got.RainTotal != 3.5 || *got.LightningTotal != 4 {
		t.Fatalf("bad body: %+v", got)
	}
}
func TestHandleSummary_BadDays(t *testing.T) {
	for _, q := range []string{"", "?days=1", "?days=abc", "?days=8"} {
		rr := httptest.NewRecorder()
		newTestServer(&fakeObservationReader{}).ServeHTTP(rr, httptest.NewRequest("GET", "/api/observations/summary"+q, nil))
		if rr.Code != 400 { t.Fatalf("days=%q code=%d want 400", q, rr.Code) }
	}
}
func TestHandleSummary_NilReader(t *testing.T) {
	rr := httptest.NewRecorder()
	newTestServerNilObs().ServeHTTP(rr, httptest.NewRequest("GET", "/api/observations/summary?days=7", nil))
	if rr.Code != 503 { t.Fatalf("code=%d want 503", rr.Code) }
}
```
- [ ] **Step 2: Run — expect FAIL.**
- [ ] **Step 3: Implement `handleSummary`** and register it in `registerObservations`:
```go
mux.HandleFunc("GET /api/observations/summary", func(w http.ResponseWriter, r *http.Request) {
	handleSummary(w, r, deps.Observations)
})
```
```go
func handleSummary(w http.ResponseWriter, r *http.Request, reader ObservationReader) {
	if reader == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "observation store not configured")
		return
	}
	days, err := strconv.Atoi(r.URL.Query().Get("days"))
	if err != nil || !allowedSummaryDays[days] {
		writeJSONError(w, http.StatusBadRequest, "days must be one of 7, 30, 180, 365")
		return
	}
	to := time.Now().Unix()
	from := to - int64(days)*86400
	ctx, cancel := context.WithTimeout(r.Context(), summaryQueryTimeout)
	defer cancel()
	s, err := reader.SummarizeObservations(ctx, from, to)
	if err != nil {
		slog.ErrorContext(ctx, "httpserver: summarize observations", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to load records summary")
		return
	}
	writeJSON(w, http.StatusOK, summaryResponse{
		Window:      summaryWindow{Days: days, From: from, To: to},
		Count:       s.Count,
		CoveredFrom: i64(s.CoveredFrom), CoveredTo: i64(s.CoveredTo),
		Temperature: summaryMinMax{Max: f64(s.TempMax), Min: f64(s.TempMin)},
		Humidity:    summaryMinMax{Max: f64(s.HumidityMax), Min: f64(s.HumidityMin)},
		Pressure:    summaryMinMax{Max: f64(s.PressureMax), Min: f64(s.PressureMin)},
		WindMax:     f64(s.WindMax), GustMax: f64(s.GustMax),
		RainTotal:   f64(s.RainTotal), LightningTotal: i64(s.LightningTotal),
	})
}
```
(Use the existing `writeJSON`/`writeJSONError` helpers; add imports `strconv`, `context`, `time`, `database/sql` as needed.)
- [ ] **Step 4: Run — expect PASS.** `go test ./internal/httpserver/ -race -v`
- [ ] **Step 5: Full backend gate.** `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l . && go test ./... -race`.
- [ ] **Step 6: Commit** `feat(httpserver): GET /api/observations/summary endpoint`.

---

## Task 4: Contract-C `RecordsSummary` type + prefs field + API client

**Files:**
- Modify: `web/src/types/weather.ts`, `web/src/api/tempestApi.ts`, the prefs default (search for the `UserPreferences` default object, likely `web/src/hooks/usePreferences.ts` or `App.tsx`).
- Test: `web/src/api/tempestApi.test.ts`.

**Interfaces:**
- Produces: `RecordsSummary`, `RecordsWindowDays`, `fetchRecordsSummary(days, signal)`, `UserPreferences.recordsWindowDays`.

- [ ] **Step 1: Add types** to `weather.ts` (mirror the Go wire tags exactly):
```ts
export type RecordsWindowDays = 7 | 30 | 180 | 365;

export interface RecordsMinMax { max: number | null; min: number | null; }

export interface RecordsSummary {
  window: { days: RecordsWindowDays; from: number; to: number };
  count: number;
  coveredFrom: number | null;
  coveredTo: number | null;
  temperature: RecordsMinMax;   // °C (SI)
  humidity: RecordsMinMax;      // %
  pressure: RecordsMinMax;      // mb (SI)
  windMax: number | null;       // m/s (SI)
  gustMax: number | null;       // m/s (SI)
  rainTotal: number | null;     // mm (SI)
  lightningTotal: number | null;
}
```
Add `recordsWindowDays: RecordsWindowDays;` to `UserPreferences`.
- [ ] **Step 2: Failing api test** — `tempestApi.test.ts`: mock `fetch` returning a `RecordsSummary`; assert `fetchRecordsSummary(7, signal)` GETs `/api/observations/summary?days=7` and returns the parsed object.
- [ ] **Step 3: Run — FAIL.**
- [ ] **Step 4: Implement** in `tempestApi.ts`: add `summary: '/api/observations/summary'` to `ENDPOINTS`, and:
```ts
export async function fetchRecordsSummary(days: RecordsWindowDays, signal?: AbortSignal): Promise<RecordsSummary> {
  return getJSON<RecordsSummary>(`${ENDPOINTS.summary}?days=${days}`, signal);
}
```
Set the default `recordsWindowDays: 7` in the `UserPreferences` default object (and any localStorage merge/validation so an old stored prefs object without the field falls back to 7).
- [ ] **Step 5: Run — PASS**; `npx tsc --noEmit`.
- [ ] **Step 6: Commit** `feat(web): RecordsSummary type + fetchRecordsSummary + recordsWindowDays pref`.

---

## Task 5: Fetch the summary in `useWeatherData`

**Files:** Modify `web/src/hooks/useWeatherData.ts`; Test: `web/src/hooks/useWeatherData.test.ts`.

**Interfaces:** Produces `summary: RecordsSummary | null` (and its loading/error state) on the hook's return, fetched with `fetchRecordsSummary(prefs.recordsWindowDays)`; re-fetches when `recordsWindowDays` changes.

- [ ] **Step 1: Failing test** — render the hook with a mocked `fetchRecordsSummary`; assert it's called with the current `recordsWindowDays` and exposes the resolved `summary`; change the window → assert a re-fetch with the new value.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** — add a `summary` state + an effect keyed on `recordsWindowDays` calling `fetchRecordsSummary` with an `AbortController`, following the existing `applySettled`/stale-retain pattern in this hook (retain last good `summary` on transient failure). Thread `recordsWindowDays` in from wherever prefs enter the hook.
- [ ] **Step 4: Run — PASS**; `npx tsc --noEmit`.
- [ ] **Step 5: Commit** `feat(web): fetch records summary keyed on the window pref`.

---

## Task 6: `RecordsCard` component + CSS

**Files:** Create `web/src/components/RecordsCard.tsx`, `web/src/components/RecordsCard.test.tsx`; Modify `web/src/App.css`.

**Interfaces:** Consumes `RecordsSummary | null` + `UserPreferences` units + `recordsWindowDays`. Produces `export const RecordsCard = memo(...)`.

Props: `{ summary: RecordsSummary | null; prefs: UserPreferences }`. Format via `useUnits` helpers (`formatTemp`/`formatWind`/`formatPressure`/`formatRain` — match the exact names used in the existing cards, e.g. `RainCard` uses `formatRain(value, unit)`, `PressureCard` uses `formatPressure`). Render a full-width `.glass-card.records-card` with a header (title + window pill "Last {days} days") and a 6-tile grid: Temperature, Humidity, Pressure (paired High/Low), Wind (paired Sustained/Gust), Rain total, Lightning (single). Null aggregates or `count===0` → em-dash. Labels use the `--text-secondary` classes (see CSS below); values right-aligned with `tabular-nums`.

- [ ] **Step 1: Failing test** — `RecordsCard.test.tsx`: render with a full `RecordsSummary` + prefs (F/mph/inHg/in); assert the tile labels/values appear (e.g. `getByText('High')`, formatted temp appears, "Last 7 days" pill); render with `count: 0` (or null aggregates) → em-dashes; render with `summary={null}` → a loading/empty placeholder (no crash).
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement `RecordsCard.tsx`.** Structure (paired tile = two `.rpair-row`s, label left / value right; single tile = one big value + muted unit). Wrap `export const RecordsCard = memo(function RecordsCard({summary, prefs}: Props) {…})`. Guard nulls with a small `fmt(value, formatter)` that returns `—` when the value is null. Window pill reads `summary?.window.days ?? prefs.recordsWindowDays`.
- [ ] **Step 4: Add CSS to `App.css`** — the classes validated across all 7 themes (reuse theme custom properties; labels `--text-secondary`; enumerated transitions; no `transition: all`):
```css
.records-card { grid-column: 1 / -1; }
.records-header { display: flex; align-items: center; gap: 10px; margin-bottom: 18px; }
.records-title { font-size: .82rem; letter-spacing: .08em; text-transform: uppercase; color: var(--text-secondary); font-weight: 600; }
.records-window { margin-left: auto; font-size: .8rem; color: var(--text-muted); display: inline-flex; align-items: center; gap: 6px; padding: 4px 12px; border: 1px solid var(--border-color); border-radius: 999px; }
.records-grid { display: grid; gap: 14px; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); align-items: stretch; }
.rstat { padding: 14px 16px; border-radius: 14px; border: 1px solid var(--border-color); background: rgba(255,255,255,.05); display: flex; flex-direction: column; }
.rstat-label { font-size: .68rem; letter-spacing: .06em; text-transform: uppercase; color: var(--text-muted); margin-bottom: 10px; }
.rstat-body { flex: 1; display: flex; flex-direction: column; justify-content: center; }
.rstat-value { font-size: 1.6rem; font-weight: 650; line-height: 1; font-variant-numeric: tabular-nums; }
.rstat-unit { font-size: .82rem; color: var(--text-muted); font-weight: 500; margin-left: 3px; }
.rstat-pair { display: flex; flex-direction: column; gap: 8px; }
.rpair-row { display: flex; align-items: baseline; justify-content: space-between; gap: 14px; }
.rpair-tag { font-size: .7rem; letter-spacing: .04em; text-transform: uppercase; color: var(--text-secondary); font-weight: 600; }
.rpair-val { font-size: 1.32rem; font-weight: 650; line-height: 1; font-variant-numeric: tabular-nums; text-align: right; }
.rpair-val .rstat-unit { font-size: .78rem; }
```
- [ ] **Step 5: Run — PASS**; `npx tsc --noEmit`.
- [ ] **Step 6: Commit** `feat(web): RecordsCard component + theme-safe CSS`.

---

## Task 7: Settings — Records window selector

**Files:** Modify `web/src/components/SettingsPanel.tsx`; Test: `web/src/components/SettingsPanel.test.tsx`.

**Interfaces:** Consumes `prefs.recordsWindowDays` + `onPrefsChange`. Produces a "Records window" `.toggle-group` (7/30/180/365) that calls `onPrefsChange({ recordsWindowDays: N })`.

- [ ] **Step 1: Failing test** — render `SettingsPanel` open; assert a "Records window" section with buttons `7 days`/`30 days`/`180 days`/`365 days`; the current pref's button has `active`; clicking `30 days` calls `onPrefsChange` with `{ recordsWindowDays: 30 }`.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** — add a new `.settings-section` after the existing Units section, mirroring the existing unit toggle-group markup:
```tsx
<div className="settings-section">
  <h3>Records</h3>
  <div className="setting-row">
    <label>Window</label>
    <div className="toggle-group">
      {([7, 30, 180, 365] as const).map((d) => (
        <button key={d}
          className={prefs.recordsWindowDays === d ? 'active' : ''}
          onClick={() => onPrefsChange({ recordsWindowDays: d })}>{d}d</button>
      ))}
    </div>
  </div>
</div>
```
- [ ] **Step 4: Run — PASS** (existing SettingsPanel dialog-a11y/theme tests still green); `npx tsc --noEmit`.
- [ ] **Step 5: Commit** `feat(web): Settings records-window selector`.

---

## Task 8: Wire `RecordsCard` into the dashboard

**Files:** Modify `web/src/App.tsx`; Test: extend an `App`-level or integration test if one exists, else add a focused render test.

**Interfaces:** Consumes `summary` (Task 5) + `prefs`. Renders `<RecordsCard summary={summary} prefs={prefs} />` immediately before `<ForecastStrip …>` inside `.dashboard-grid`.

- [ ] **Step 1: Failing test** — render `App` (or the dashboard) with a mocked `useWeatherData` returning a `summary`; assert the Records card renders and appears **before** the 7-Day Forecast in DOM order.
- [ ] **Step 2: Run — FAIL.**
- [ ] **Step 3: Implement** — import `RecordsCard`; pull `summary` from `useWeatherData()`; insert `<RecordsCard summary={summary} prefs={prefs} />` on the line directly above `<ForecastStrip forecast={forecast} unit={prefs.temperatureUnit} />`.
- [ ] **Step 4: Run — PASS.** Full web gate: `cd web && npm test && npm run build && npx tsc --noEmit`.
- [ ] **Step 5: Commit** `feat(web): render RecordsCard above the 7-day forecast`.

---

## Final verification (bundle gate, not a task)

- `CGO_ENABLED=0 go build ./... && go vet ./... && gofmt -l . && golangci-lint run && go test ./... -race` — all clean.
- `cd web && npm test && npm run build && npx tsc --noEmit` — clean; test count up.
- **In-browser gate:** build the binary embedding the UI; run with a temp `SQLITE_PATH`, `SQLITE_BATCH_SIZE=1`, `SQLITE_FLUSH_INTERVAL=1s`, `HTTP_ADDR=127.0.0.1:8087`; feed several `obs_st` UDP packets (varied temp/humidity/pressure/wind/gust/rain/lightning) to `127.0.0.1:50222`; open `:8087`; confirm the Records card shows real aggregates, switching the Settings window re-fetches, and labels are legible (re-run the theme matrix). Confirm `/api/observations/summary?days=7` returns correct JSON and `?days=8`/missing → 400.
- Cold whole-branch review (Fable) over `git diff origin/main..HEAD`; address findings; then `superpowers:finishing-a-development-branch` (USER-GATED).

## Self-review notes
- Spec coverage: read-path decoupling (T1), aggregation + NULL/empty (T2), endpoint + validation + deadline + error mapping (T3), Contract-C single-source (T4), data fetch keyed on window (T5), card + theme-safe labels + units (T6), settings selector (T7), placement (T8). Migration: the timestamp index already exists (`0002_add_timestamp_index.sql` on main) — no new migration; T1 relies on it.
- Type consistency: `SummarizeObservations`/`Summary` names identical across T2/T3; `RecordsSummary` JSON tags (T4) mirror `summaryResponse` (T3) field-for-field; `recordsWindowDays` consistent T4/T5/T7/T8.
- Flag for the implementer to verify at T1: `modernc.org/sqlite` honoring `mode=ro` + read-only over WAL (covered by T1's test); if `mode=ro` misbehaves, fall back to a normal (read-write-capable) second handle used only for reads — WAL still decouples it from the writer's single-conn pool.
