# Focused Cold Re-Review — Unified Weather Platform Plan v2

**Reviewer:** cold, independent senior-Go pass. No prior context; this is a *targeted* re-review against the prior cold review's findings (`docs/review/2026-07-17-plan-cold-review.md`), not a full re-read. Conclusions reached fresh and checked against the live tree (`internal/sink/sink.go`, `internal/postgres/writer.go`, `internal/prometheus/writer.go`, `internal/prometheus/server.go`, `internal/config/database.go`, `internal/tempestapi/client.go`, `main.go`) and the design (`docs/designs/2026-07-17-unified-weather-platform-design.md`).
**Skills applied:** `superpowers:test-driven-development`, `modern-go-guidelines:use-modern-go` (Go 1.24). Critical-thinking gate run before finalizing.

---

## Verdict

**READY-WITH-MINORS.** Ready for a Haiku/Sonnet executor. **No remaining BLOCKER, no remaining MAJOR.**

Every defect the prior cold review flagged is genuinely fixed in v2 — and, critically, the newly-**inlined GREEN code is correct**, not merely present. I verified each inlined block against the real signatures and line numbers it claims to change:

- The two send-on-closed gates (0.9a postgres, 0.9b prometheus) are the *textbook-correct* fix and are shaped to each writer's **actual** channel topology, which I confirmed in source. This is the load-bearing check — a wrong gate here would reintroduce the C-H3/D-H1 panic — and it passes.
- The `errors.Join` fan-out aggregation (0.7) is race-free (mutex around the append; `recover()` inside each goroutine).
- The DSN `net/url` construction (0.4) genuinely percent-encodes credentials and round-trips.
- The SIGTERM seam (0.8) is now a concrete, injectable, testable helper.
- The pinned constants (3.3, 2.4) are present AND asserted in the RED tests.
- Task 4.1's tautological table-equality branch is deleted and replaced with a real translation check.
- B1/B2/B3 all landed in both docs.

Remaining items are four **MINOR** cosmetic/narrative defects (below) that do not block execution. Fix them opportunistically; none warrant another revision cycle.

---

## BLOCKER 2.1 — Radar fixture — **RESOLVED**

Both halves the prior review demanded are present:

- **(a) Deterministic, fixture-independent RED case:** Task 2.1 Step 1 now leads with `test_decode.py::test_decode_bad_bytes_raises` — `decode.to_geojson(b"not-nids", "N0B")` must raise a typed `decode.DecodeError`. A weak model can write this and watch it fail (module→function→wrong-exception) with zero binary input. TDD is startable.
- **(b) Concrete, runnable fixture-acquisition step:** Step 0 gives a pinned, commit-then-freeze recipe. It correctly handles the real-time bucket's rotating flat keys (`SSS_PPP_YYYY_MM_DD_HH_MM_SS`) by **listing newest → copying → committing the bytes** (`aws s3 ls --no-sign-request ... | grep 'TLX_N0B_' | sort | tail -1`), rather than hardcoding a key that would later 404. PROVENANCE.md records the source. The binary golden test (`test_decode_nids_to_isobands`) is second, gated on the committed file.

The TDD loop no longer requires fabricating a binary. Evidence: plan lines 1421–1453.

**Residual (not a defect):** Step 0 needs network + `aws` CLI *once* to seed the fixture; after commit, CI never touches S3. Acceptable and documented.

---

## MAJOR M1–M4

### M1 — Vendored-UI forward references — **RESOLVED**

Task 1.1 now emits a concrete UI manifest: the real `web/` file tree (lines 1156–1187) plus the **verbatim `web/src/types/weather.ts`** (lines 1189–1242) declared the single source of truth for Contract C. Downstream tasks cite present symbols: 1.2 (`ForecastStrip.getDayName`, `AlmanacCard`), 1.7a (`tempestApi.ts` stub fns, `stubData.ts`, `useWeatherData.ts`), 1.7b (`App.tsx`, `SolarUVCard.tsx`, named CSS classes), 1.7c (`SettingsPanel.tsx:103/:112`), 2.6 (`App.tsx`, `RadarCard.tsx`), UX.1 (`themes.ts`, `SettingsPanel.tsx`). The manifest header instructs STOP-and-reconcile if the clone diverges. Correctness spot-check: the `CurrentObservation` interface fields exactly match the Contract C key list on line 160 (SI units). Resolved — with one stale cross-reference noted under NEW issues.

### M2 — Prose-only GREEN → inlined code — **RESOLVED** (correctness spot-checked)

All named tasks now contain actual code, and the code is correct:

- **0.9a (postgres gate) — correct.** Verified against source: the current `Close` unconditionally `close()`s all four batch channels (`writer.go:855–866`) and producers use `select { case w.xBatch <- row: … default: log-drop }` (e.g. `:543–550`). The v2 fix adds `done chan struct{}` + `closeOnce sync.Once`, **stops closing** the producer channels, makes producers `select` on `<-w.done`, makes workers exit on `<-w.done`, and closes only `done` once. This genuinely prevents send-on-closed and never closes a channel a producer still sends to. Correct for the 4-channel shape. (lines 500–533)
- **0.9b (prometheus gate) — correct.** Verified: current `Close` closes both `outbox` and `more` (`writer.go:91–92`); `pushWorker` is `for range w.more` (`:100`); `WriteMetrics` sends into `outbox` then signals `more` (`:59–74`). The v2 fix adds `done`+`Once`, gates the metric send and the `more` signal on `done`, converts `pushWorker` to `select { case <-w.more … case <-w.done: final Add(); return }`, and closes only `done`. Correct for the `outbox`+`more` shape. The retained `default:` drop branch in `WriteMetrics` is fine (non-blocking, pre-existing behavior). (lines 553–609)
- **The split itself is right.** The plan explicitly justifies 0.9→0.9a/0.9b because the two writers have structurally different channels and warns against "apply an identical pattern" (lines 483–484). This directly answers the prior review's central M2/concurrency concern.
- **0.7 (`errors.Join` aggregation) — correct and race-free.** The append happens under `mu.Lock()` in both the error path and the `recover()` path; `recover()` is `defer`-installed per goroutine so a panicking writer is contained and the others still run; `wg.Wait()` then `errors.Join(errs...)` (nil when empty). Matches the current fan-out shape in `sink.go`. Modern idiom (`slices.Clone`, `log/slog`). (lines 341–403)
- **0.4 (DSN via `net/url`) — correct.** `url.UserPassword(username, password)` percent-encodes `@ : / ? # &`; `net.JoinHostPort`; `u.RawQuery = url.Values{...}.Encode()`; `u.String()` round-trips through `url.Parse`, which is exactly the RED assertion. Replaces the raw `fmt.Sprintf` at `database.go:52–53` (verified present). Note is accurate that `pgxpool.ParseConfig` accepts this form. (lines 240–265)
- **0.8 (SIGTERM seam) — concrete and testable.** The prior review's "impractical/via a seam" hand-wave is gone. v2 injects a `notifyFunc` matching `signal.NotifyContext`'s signature; `TestSignalContext_RegistersInterruptAndSIGTERM` asserts the exact signal set (`os.Interrupt`, `syscall.SIGTERM`) with no real signals. Replaces the real `signal.NotifyContext(ctx, os.Interrupt)` at `main.go:29` (verified — SIGTERM is currently absent). (lines 440–476)
- **3.3 (single-writer batch loop) — inlined**, with the backpressure `enqueue`/`enqueueEvent` split shown (continuous = non-blocking drop-with-log; discrete events = bounded-block on a timer). Column mappings are pinned to the verified postgres field indices. (lines 886–939)

### M3 — Unstated constants — **RESOLVED**

- **3.3:** `rowChanCap = 1000`, `eventBlockTimeout = 5 * time.Second` — pinned as named consts AND asserted by `TestWriter_EventsNotDroppedUnderBackpressure` ("Assert the pinned constants exist: `rowChanCap == 1000`, `eventBlockTimeout == 5*time.Second`", line 884).
- **2.4:** `radarCacheMaxEntries = 64`, `radarCacheTTL = 5*time.Minute` (+ `radarSidecarTimeout = 30s`) — pinned AND asserted by `TestProxy_Constants` (line 1509).

The backpressure timeout that governs the C-MEDIUM "events not dropped" guarantee is now concrete. (One garbled comment noted under NEW issues — cosmetic.)

### M4 — Task 4.1 dilution — **RESOLVED** (design-conformant)

The tautological "OR table-equality" branch is deleted. `TestPrometheusExporterEmitsContractBNames` now drives the OTel writer through the **real** in-process `go.opentelemetry.io/otel/exporters/prometheus`, gathers the registry, and asserts the emitted family names equal Contract B's right column — plus **negative guards** that the old names (`tempest_wind_ms`, `tempest_uptime_seconds_total`, `tempest_rainfall_total`) are NOT emitted. This exercises real OTLP→Prometheus normalization (dotted→underscore, `_total`, unit suffixes) and catches a wrong instrument name/kind. (lines 1583–1638)

Design-conformance: design §8 (line 294) asks for "an assertion test that the Collector emits the exact `tempest_*` names." The plan substitutes the in-process exporter — a faithful, deterministic stand-in that shares the Collector's normalization rules — and explicitly flags an optional `//go:build integration` real-Collector test. This is a defensible, in-spirit substitution, not a dilution. Step 5 (`TestMetricMigrationList`) additionally emits the complete old→new break set for DOC.2 (addresses the prior review's Axis-1 migration-diff MINOR).

---

## B1 / B2 / B3 — landed & conformant

- **B1 (Postgres tunables) — landed, conformant.** New **Task 0.13** adds a pure `postgresTunables(getenv)` helper; `TestPostgresTunables` asserts a **non-default** value takes effect (`BATCH_SIZE=7`, `FLUSH_INTERVAL=2s`, `MAX_RETRIES=5`) AND that defaults hold when unset (100/10s/3). Verified against source: the values are currently hardcoded-and-inert at `writer.go:133–135` (`batchSize:100, flushInterval:10*time.Second, maxRetries:3`) — exactly the C-H2-for-Postgres defect. Design updated: §-refs at design lines 84, 164–166, 178 credit C-H2 for the Postgres path. (plan lines 737–806)
- **B2 (basemap) — landed, conformant.** Basemap = OpenStreetMap via same-origin Protomaps `.pmtiles`; **CSP is self-only** with the exact directive pinned in Global Constraints (line 48) and asserted in `TestServer_SecurityHeaders` (1.3, line 1282) — including a positive assertion that the CSP names **no external host**. `blob:`/`wasm-unsafe-eval` are scoped to MapLibre's worker/WASM, adding no network origin. OSM attribution required and rendered (Task 2.6, line 1567–1574: visible `AttributionControl` "© OpenStreetMap contributors"). Both docs updated (design §7/§9/§15, lines 211, 378–381, 672–675). No API key, no external tile host.
- **B3 (`/api/almanac`) — landed, conformant.** Contract C (line 164) and Task 1.5 make `/api/almanac` a WeatherFlow REST proxy (Bearer), not computed. Design §11 (line 494) updated to "v2/B3: proxy, not computed." DOC.2 documents it (line 1702).

---

## NEW issues introduced by the v2 edits

All **MINOR** — none block execution:

- **[MINOR] Stale cross-reference in Task 1.4.** Step 1 (line 1302) says the current-observation endpoint returns "Contract C **`CurrentWeather`**", but the Task 1.1 manifest and Contract C table name the type **`CurrentObservation`** — there is no `CurrentWeather` interface in `weather.ts`. The field list is unambiguous, so an executor will likely infer the right type, but the manifest edit left this dangling reference unreconciled. Fix: rename `CurrentWeather`→`CurrentObservation` on line 1302.
- **[MINOR] 0.8 ↔ 0.9a forward reference.** Task 0.8 (line 476) tells the executor to "stop accepting new sends (the Task 0.9a `done` gate)" — but 0.9a *depends on* 0.8 and lands later. 0.8's own concrete GREEN keys the drain off the existing `w.ctx.Done()`/channel path, so it is executable in isolation, but the parenthetical invites a Haiku agent to implement the `done` gate early and collide with 0.9a. The plan acknowledges the shared shutdown path (line 487); tightening 0.8's wording to "drain off `w.ctx` here; the `done` gate is added in 0.9a" would remove the ambiguity.
- **[MINOR] 0.8 inserter-interface ripple under-shown.** The `TestPostgresWriter_DrainOnClose` seam introduces `interface{ insertObservations(ctx, []observationRow) error }`, but the current `insertObservations` has **no** `ctx` param (`writer.go:174`, derives its 5s timeout from `w.ctx` at `:179`). The plan does say to "route `insertObservations` et al. to accept the passed `ctx`" (line 476), so the refactor is named — but the signature change ripples to `flushObservations`/`flushWithRetry` and the three sibling insert paths more than the text shows. Tractable (the row types and call sites exist), just larger than it reads.
- **[MINOR/cosmetic] Garbled comment in 3.3.** The `eventBlockTimeout` doc comment contains "`5s > the 10s? No —`" (line 897), an editing artifact. Harmless to the code; clean up the prose.

No new dependency-order violation from the task splits (0.9a/0.9b, 1.7a/b/c, +0.13) beyond the 0.8/0.9a wording above. No new broken symbol reference in the split UI tasks — all cite the manifest.

---

## Residual watch-items (inherent uncertainties, not defects)

These are genuine execution-environment dependencies the plan already flags; listing them so the executor is not surprised:

- **Litestream on PATH (Task 3.6):** the now-default (non-integration) file-replica restore test `t.Skip`s cleanly if `litestream` is absent. Good — but it means the PRAGMA/restore contract is only *actually* proven where the binary is installed (CI must install it). The plan states CI installs it; verify that holds.
- **OTel logs API is experimental (WS6):** `otelslog`/`otlploggrpc` at v0.8.0 move. Plan mandates Context7 confirmation before coding (lines 74, 1122) and isolates the bridge to one file (No-Wall). Correct mitigation; still a live risk.
- **Upstream UI clone must match the manifest (Task 1.1):** the manifest is captured from `tempest-display@49892063`; if the clone diverges, the STOP-and-reconcile instruction fires. This is a manual gate, not an automated one.
- **NOAA HOMR awk column indices (Task 2.3 Step 0):** the plan itself says to verify `$(NF-4)`/`$(NF-3)` against the current file header once. A weak model may not catch a column shift; the `IsValidSite`/`NearestSite` RED tests (`TLX` near OKC) will catch a grossly wrong table, which is a reasonable backstop.
- **Grafana JSON authoring (Task 4.2):** hand-authoring the 8-row dashboard JSON from prose remains large and error-prone for a weak model, but the validation test (parse + expr-names-in-Contract-B + `$serial`/`$__range` vars) constrains it. Acceptable.

---

## Bottom line

The v2 edits did the hard thing the prior review asked for: they inlined **correct** concurrency-sensitive code, not just prose, and I confirmed each block against the real writer topology. The BLOCKER and all four MAJORs are resolved; B1/B2/B3 are landed and design-conformant. The only open items are four minor cosmetic/narrative defects. **A Sonnet/Haiku agent can execute this plan.**
