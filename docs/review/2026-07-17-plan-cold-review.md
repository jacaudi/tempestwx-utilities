# Cold SGE Review — Unified Weather Platform Plan vs Design

**Reviewer:** cold, independent senior-Go pass. No prior context; conclusions reached fresh from the two documents, sanity-checked against the live tree (`main.go`, `internal/**`, `go.mod`).
**Inputs:** `docs/designs/2026-07-17-unified-weather-platform-design.md`, `docs/plans/2026-07-17-unified-weather-platform-implementation.md`.
**Skills applied:** `superpowers:writing-plans` (the plan's own stated bar), `superpowers:test-driven-development`.

---

## Verdict

**Ready-after-fixes.** The plan is architecturally faithful to the design — I found **zero drift** on the three revised decisions (O1 Python Py-ART sidecar, O3 contoured GeoJSON, O5 app-ships-OTLP-only), every review CRITICAL/HIGH is traceably assigned to a task, the three cross-task contracts are single-sourced, and the WS0-first sequencing genuinely lands the lifecycle fixes that are the whole premise (SIGTERM, drain, panic recovery, send-on-closed gate) before anything depends on them. That is strong senior work and I want to be clear it is not the problem.

The problem is **Axis 3 (executability), which is weighted heavily and where the plan is systematically thin.** Its GREEN (implementation) steps are almost universally *prose* ("Implement `Writer` satisfying `sink.MetricsWriter`... single writer goroutine... `ON CONFLICT DO NOTHING`") rather than the concrete code blocks the `writing-plans` skill mandates ("Complete code in every step — if a step changes code, show the code"; "Steps that describe what to do without showing how" is listed as a *plan failure*). A skilled human fills that gap; a Sonnet/Haiku executor reading only one task's text will stall or produce low-confidence work on the harder tasks. Layered on top are a handful of concrete blockers: tests that depend on binary fixtures a weak model cannot fabricate (Task 2.1 NIDS), unstated constants (backpressure timeout, channel capacities, LRU size), `main.go` refactor seams the executor must invent (`signalContext`/`requireWriters`/`selectStore`), one test that contradicts its design requirement and risks being tautological (Task 4.1), and one oversized task (Task 1.7). **Must fix before a weaker model executes: (1) show concrete code in the GREEN steps of the concurrency/DSN/writer tasks; (2) resolve the fixture-dependent RED tests so they can fail for the right reason; (3) repair Task 4.1 to actually exercise the Collector name-translation per design §8 and pin the unstated constants.**

---

## Axis 1 — Conformance to design

### Coverage checklist

| Design element | Covered by | Verdict |
|---|---|---|
| **W0** Foundation (lifecycle/review-fix bedrock) | Tasks 0.1–0.12 | Covered |
| **W1** Embedded UI + backend JSON API | Tasks 1.1–1.8 | Covered |
| **W2** NEXRAD radar overlay | Tasks 2.1–2.6 | Covered |
| **W3** SQLite + Litestream | Tasks 3.1–3.6 | Covered |
| **W4** Grafana dashboard | Tasks 4.1–4.2 | Covered |
| **W5** UX scan | P0/P1 folded into 1.2/1.7/2.6; P2 in UX.1; P3 recorded in DOC.2 | Covered |
| **W6** OTLP backbone | Tasks 6.1–6.5 | Covered |
| **R1** unified OTLP, app ships signals only | Global Constraints + 6.1 + DOC.1 | Covered |
| **R2** SQLite default + Postgres optional | Task 3.5 (`selectStore`) | Covered |
| **R3** radar server-side via Py-ART sidecar | Tasks 2.1–2.6 | Covered |
| **O1** Python Py-ART sidecar (NOT pure-Go) | Task 2.1 (FastAPI + Py-ART) | **Conforms — no drift** |
| **O2** N0B default, N0Q fallback | Global Constraints + Task 2.4 | Covered |
| **O3** contoured GeoJSON isobands (NOT PNG-primary) | Contract A + Task 2.1/2.6 | **Conforms — no drift** |
| **O4** deprecate-then-remove prometheus | Global Constraints + Task 6.5 | Covered |
| **O5** app ships OTLP; backends operator's choice | Global Constraints + DOC.1 (Tempo/Loki commented) | **Conforms — no drift** |
| **O6** `modernc.org/sqlite` CGO-free | Global Constraints + Task 3.1/3.2 | Covered |
| CRITICAL/HIGH fix map (both reviews) | see below | All assigned |

**CRITICAL/HIGH assignment audit (verified each):** C-1→0.2; A-H1→0.8; A-H2→0.11; A-H3→0.10; C-H1→0.8; C-H2→3.2; C-H3→0.9; D-H1→0.9; D-H2→0.10; F-H1..H4→0.5; G-H1..H3→0.12; B-HIGH→0.4. UI: A-H1..H4→1.7; B-H1→1.5; B-H2→1.4/1.7; E-H1→1.3; F-H1→1.8. **Every promised fix has a home.** Good.

### Findings

- **[MINOR] Axis 1 — Task 4.1 waters down a design §8 requirement.** Design §8 Risks explicitly demands "an **assertion test that the Collector emits** the exact `tempest_*` names WS4 queries" — i.e. exercise the real OTLP→Prometheus translation. Task 4.1 offers an "OR" branch: "assert the instrument-name→expected-prom-name **mapping table** equals Contract B exactly (a data-driven table test comparing every row)." That branch compares a hand-written map in the test to a hand-written map in code, both copied from Contract B — it asserts nothing about actual translation and is tautological. This is also an Axis 2 (TDD) finding. **Fix:** require the test to drive the OTel writer through the `sdk/metric` Prometheus exporter (or a real Collector in an integration test) and assert the emitted names, deleting the table-equality escape hatch.

- **[MINOR] Axis 1 — Litestream "restore-safety" proof is integration-gated and may silently skip.** Design §10 Risks names "misconfigured PRAGMAs silently lose Litestream data" as a top risk and §18 mandates a restore-from-replica test. Task 3.6 provides it but tags it `//go:build integration` with a fallback to "a local filesystem replica if S3 is unavailable in CI." The design's whole point is that WAL/checkpoint semantics are proven; if CI routinely skips the integration tag, the guard is decorative. **Fix:** make the filesystem-replica variant the *default* (non-integration) test so the PRAGMA/restore contract is proven on every run.

- **[MINOR] Axis 1 — no task verifies the `instance`→`serial` break is actually the *only* metric break.** Global Constraints and DoD assert `tempest_*` names are "preserved," but the design renames several (`_ms`→`_meters_per_second`, uptime `_total` dropped, RainTotal wired). These are intended, but there is no task asserting the *full* old-vs-new name set so a consumer migration doc is complete. Contract B is the source but nothing diffs it against the current `internal/tempest/metrics.go` descriptors. **Fix:** add a step in Task 6.2/4.1 that diffs Contract B against the existing descriptor names and emits the migration list DOC.2 consumes.

No scope-invention drift found: the plan adds nothing the design didn't call for. P3 items are correctly recorded-not-built (YAGNI-clean).

---

## Axis 2 — SGE standards / skills / rules

- **[MAJOR] TDD — GREEN steps are prose, not code, across the whole plan.** The `writing-plans` skill lists "Steps that describe what to do without showing how (code blocks required for code steps)" as a **plan failure**, and `test-driven-development` requires the *minimal implementation* be concrete enough to know the test drove it. Nearly every GREEN step here is a paragraph of intent: 0.7 (panic recovery + `errors.Join` aggregation), 0.9 (the `sync.Once`+`done`+`select{case ch<-x: case <-done}` gate), 3.3 (single-writer batch loop), 6.2 (instrument registration). The RED tests are mostly concrete and good; the implementations are hand-waved. A skilled human bridges it; a weak executor guesses. **Fix:** for at least the concurrency-sensitive tasks (0.7, 0.8, 0.9, 3.3) and the DSN task (0.4), inline the actual code the step should produce.

- **[MAJOR] Concurrency — the load-bearing send-on-closed gate (Task 0.9) is specified once in prose and told to "apply the identical pattern to both writers."** This is the exact fix for the design's C-H3/D-H1 premise, and it is subtle (never `close()` a channel producers may still send to; signal via `done`; drain). The plan neither shows the code nor accounts for the two writers having *different* internal channel shapes (`postgres/writer.go` is 870 lines with a 4-goroutine design per the review; `prometheus/writer.go` is ~2.7 KB). "Identical pattern" is not "identical code." A weak executor applying a copy-paste to structurally different writers is a regression risk on the very bug being fixed. **Fix:** show the concrete gate per writer, or split into 0.9a/0.9b with each writer's actual code.

- **[MAJOR] `main.go` refactor seams are invented by the executor, not specified.** `main.go` is a flat 281-line file (verified). Tasks 0.8 (`signalContext`), 0.11 (`requireWriters`), 3.5 (`selectStore`) each say "extract a testable helper `func X(...)`" but never show the current code being extracted or the resulting signatures' bodies. Task 0.8 itself admits `TestMain_TrapsSIGTERM` "is impractical" and hand-waves "unit-test that the helper lists both signals **via a seam**" — an unresolved judgment call handed to the weakest reader. **Fix:** show the before/after of each extraction with the concrete helper body; resolve the SIGTERM-test approach to one named technique (e.g. table-assert the signal set passed to a thin injectable `notify` func).

- **[MINOR] KISS/right-sizing — Task 1.7 bundles five independent concerns.** It does: replace stub data layer + AbortController + ErrorBoundary + define all missing CSS + responsive breakpoints + reduced-motion/focus-visible + `formatX` NaN guard + remove SettingsPanel inputs + self-host fonts. The `writing-plans` "Task Right-Sizing" rule: a reviewer should be able to reject one task while approving its neighbor — impossible here. **Fix:** split into 1.7a (data layer + AbortController + stale indicator), 1.7b (ErrorBoundary + missing CSS + responsive/a11y), 1.7c (settings-input removal + font self-host).

- **[MINOR] Modern-Go — good, with one gap.** Global Constraints correctly mandate 1.24 idioms (`cmp.Or`, `for range n`, `errors.Join`, `slices`/`maps`). Task 0.3 modernizes the wetbulb loop to `for range 10000`. One miss: Task 0.7 says "mutex-guarded slice + `errors.Join`" for aggregation but the current fan-out uses `sync.WaitGroup` with per-goroutine `log.Printf` (verified in `sink.go`) — the plan should note that collecting errors from goroutines needs the mutex *around the append*, which the prose says, but should show it to avoid a data race the `-race` test would catch late.

- **[MINOR] Go↔Python contract — Contract A is well-specified but the error-transport is ambiguous.** Contract A says errors come "HTTP 200 with error envelope OR non-200" — allowing both. A weak executor implementing the Go client (Task 2.4) and the Python side (Task 2.1) independently may disagree on which. **Fix:** pick one (recommend: non-200 status + JSON envelope) and state it once in Contract A.

---

## Axis 3 — Executability by a Sonnet/Haiku agent

### Per-task verdict

| Task | Verdict | Gap (if not Executable) |
|---|---|---|
| 0.1 golangci/gofmt | Executable | — |
| 0.2 RadioStats guard | Executable | Cited symbols verified in `report.go` (`RadioStats[1]/[2]` at ~263). |
| 0.3 wetBulb NaN | Executable | `panic("failed to converge")` verified at `wetbulb.go:47`. |
| 0.4 DSN via net/url | Needs-tightening | GREEN gives the `url.URL` skeleton (good) but not the full function replacing `database.go:52`'s `fmt.Sprintf`; show the whole body. |
| 0.5 API client hardening | Needs-tightening | Large multi-assertion task; `NewClient` returns value receiver (verified `client.go:21`) — plan says "change to pointer receiver as needed" without showing the ripple to `main.go` call sites. Bearer-header format left to "verify against docs." |
| 0.6 bool env parsing | Executable | — |
| 0.7 sink Close(ctx)+recover+aggregate | Needs-tightening | Interface change verified feasible (`Close() error` at `sink.go:25`, `Flush(ctx)` already exists). Prose-only GREEN for panic-recovery + `errors.Join`; show code. |
| 0.8 SIGTERM + pg drain | Ambiguous | Admits its own test is "impractical," hand-waves "via a seam"; `signalContext` extraction unshown; pg drain over "an injected fake `pgxpool`-like interface" that doesn't exist and isn't defined. |
| 0.9 idempotent Close both writers | Ambiguous | The subtle send-on-closed gate shown only in prose; "apply identical pattern to both" ignores structural differences between the two writers. |
| 0.10 metrics server bind error | Executable | `Start()`/`ListenAndServe` in goroutine verified (`server.go:64-69`); the `net.Listen` fix is concrete. |
| 0.11 writer invariant + O_TRUNC | Needs-tightening | `requireWriters` extraction + `Mode` type unshown; `writeMetricsToFile` open flags verified (`main.go:243`, missing `O_TRUNC`) — that half is concrete. |
| 0.12 CI/supply-chain | Needs-tightening | "Re-declare the inputs the action references OR remove `fromJSON(inputs.push)`" requires reading `.github/actions/docker/action.yml` + caller to disambiguate; no concrete diff. |
| 3.1 sqlite schema/migrations | Executable | DDL is fully specified in design §12; `Migrate` contract clear. |
| 3.2 sqlite Open/PRAGMAs | Needs-tightening | Exact PRAGMAs given (good) but modernc `_pragma` DSN syntax flagged "verify via Context7" — an unresolved lookup mid-task. |
| 3.3 sqlite writer | Needs-tightening | Column mappings deferred to "mirror the postgres writer's verified mappings" (870-line file); backpressure "block-with-bounded-timeout" has **no timeout value or channel capacity**. |
| 3.4 drain + read methods | Executable | Field allowlist concept clear; signatures given. |
| 3.5 wire default store | Needs-tightening | `selectStore` extraction unshown; `storeChoice` type undefined. |
| 3.6 Litestream restore | Needs-tightening | "MinIO testcontainer OR local filesystem replica … document which" — unresolved choice; spawning litestream subprocess unspecified. |
| 6.1 otel setup | Needs-tightening | Correctly defers to Context7 (logs API experimental) — acceptable, but the whole provider wiring is prose. |
| 6.2 otel writer | Needs-tightening | Contract B fully enumerates names/kinds (good); the in-memory manual-reader assertion is real; instrument registration code unshown. |
| 6.3 tracing spans | Executable | `tracetest.SpanRecorder` + span-name assertions concrete. |
| 6.4 slog→OTel bridge | Needs-tightening | slog.Handler-over-LoggerProvider is nontrivial and prose-only; depends on experimental API. |
| 6.5 prometheus deprecation | Executable | Single `slog.Warn`; trivial. |
| 1.1 vendor UI | Ambiguous | Depends on cloning `tempest-display@49892063` — an external repo not in this tree; a weak model cannot fabricate it. RED-less scaffolding task. |
| 1.2 vitest + math | Needs-tightening | Depends on file names inside the not-yet-present vendored app (`ForecastStrip.getDayName`, hemisphere logic) — unknowable until 1.1 lands. |
| 1.3 Go HTTP server/embed | Needs-tightening | Good detail on `//go:embed` cross-dir pitfall; SPA/headers concrete-ish but middleware code unshown. |
| 1.4 observation handlers | Needs-tightening | `CurrentWeather` shape deferred to `web/src/types/weather.ts` (arrives in 1.1) — must read a vendored file. |
| 1.5 WF proxy handlers | Executable | httptest-driven, bearer assertion concrete. |
| 1.6 otelhttp + wire main | Executable | — |
| 1.7 data layer + boundary + CSS | Ambiguous | Five concerns; oversized (Axis 2). Depends on vendored file internals. |
| 1.8 multi-stage Dockerfile | Executable | Concrete stages + `USER 65532` + healthcheck. |
| 2.1 python sidecar | **BLOCKER** | RED test `test_decode_nids_to_isobands` requires `radar/tests/fixtures/<captured_nids_file>` — a **binary NEXRAD L3 fixture** a weak model cannot generate. The TDD loop cannot fail-for-the-right-reason without it. |
| 2.2 sidecar Dockerfile | Executable | — |
| 2.3 radar site table | Needs-tightening | "~160 CONUS sites: code + lat/lon" — the actual table data is not provided; "reuse DRAS conventions" points at an external repo. |
| 2.4 radar proxy + LRU | Needs-tightening | LRU size ("bounded") and TTL ("~5 min") are approximate; concrete cap unset. |
| 2.5 radar handler | Executable | Error→status mapping enumerated (503/502). |
| 2.6 UI radar card | Needs-tightening | MapLibre tile source ("self-hostable tiles") unspecified; depends on vendored app. |
| 4.1 prom-name assertion | Ambiguous | Tautology risk (Axis 1/2) — the "OR table-equality" branch asserts nothing about real translation. |
| 4.2 grafana dashboard | Needs-tightening | 8 rows enumerated in §13 (good); authoring full Grafana JSON by hand from prose is large and error-prone for a weak model. |
| UX.1 P2 polish | Needs-tightening | Depends on vendored component internals. |
| DOC.1 compose | Needs-tightening | Service inventory in §15a is detailed; exact YAML unshown. |
| DOC.2 README/CI | Executable | — |

### BLOCKER / MAJOR write-ups

- **[BLOCKER] Axis 3 — Task 2.1 RED test depends on a binary NIDS fixture that cannot be authored from task text.** `test_decode.py::test_decode_nids_to_isobands` needs a real captured NEXRAD Level 3 object at `radar/tests/fixtures/`. A Sonnet/Haiku executor cannot synthesize a valid binary NIDS file, so the test cannot be written or watched-fail per TDD. **Fix:** add a pre-task step that specifies *how* the fixture is obtained (e.g. a documented `aws s3 cp --no-sign-request s3://unidata-nexrad-level3/TLX_N0B_... radar/tests/fixtures/` command with a pinned key, or commit the fixture and reference it), and give `decode.to_geojson` a smaller deterministic RED case (e.g. malformed-bytes→`DecodeError`) that *can* be written first.

- **[MAJOR] Axis 3 — vendored-UI dependency chain (Tasks 1.1, 1.2, 1.4, 1.7, 2.6, UX.1) rests on an external repo and its internal file names.** Task 1.1 clones `tempest-display@49892063` (not in this tree, verified). Every downstream UI task references symbols inside it (`ForecastStrip.getDayName`, `tempestApi.ts` stubs, `types/weather.ts`, `SettingsPanel.tsx`, `themes.ts`). A weak executor running 1.2+ in isolation cannot know these exist or their shapes. **Fix:** Task 1.1 must emit a concrete manifest (the actual file tree + the exact `weather.ts` type block) into the plan or a committed doc so downstream tasks cite real, present names instead of forward-referencing a repo that must be fetched.

- **[MAJOR] Axis 3 — unstated constants block several GREEN steps.** Task 3.3 "block-with-bounded-timeout" has no timeout and no channel capacity; Task 2.4 LRU is "bounded" with no size; Task 1.6 default addr `:8080` is stated (good) but Task 3.3/3.2 `SQLITE_*` defaults are stated (good) — the gap is specifically the backpressure timeout, event-channel cap, and LRU cap. A weak model will invent divergent values, and the backpressure one directly governs the C-MEDIUM "events not dropped" guarantee. **Fix:** pin exact values in the tasks (e.g. events channel cap N, block timeout T, LRU max entries M) and assert them in the RED tests.

- **[MAJOR] Axis 3 — Task 0.8's own admission of test impracticality is an unresolved judgment call.** The plan literally says the SIGTERM test "is impractical" and offers "(or, simpler, unit-test that the helper lists both signals via a seam)" without defining the seam. This is the acceptance criterion for the design's #1 HIGH (A-H1). **Fix:** specify one concrete testable shape — extract `func signalContext(ctx, sigs ...os.Signal)` and assert the call site passes `[os.Interrupt, syscall.SIGTERM]`, or inject a `notifyFunc` and assert its arguments.

---

## Prioritized fix list (most important first)

1. **Resolve Task 2.1's fixture blocker** — specify how the binary NIDS fixture is obtained/committed, and add a deterministic malformed-bytes RED case so TDD can start. (BLOCKER)
2. **Make Task 1.1 emit a concrete UI manifest** (file tree + `weather.ts` types) so Tasks 1.2/1.4/1.7/2.6/UX.1 reference present, real symbols instead of forward-referencing an external repo. (MAJOR)
3. **Inline concrete code in the concurrency/DSN GREEN steps** — 0.4, 0.7, 0.8, 0.9 (per-writer gate code), 3.3. This is the single biggest lift for weak-model executability and the `writing-plans` bar. (MAJOR)
4. **Pin the unstated constants** — 3.3 backpressure timeout + event channel cap, 2.4 LRU size/TTL — and assert them in RED tests. (MAJOR)
5. **Repair Task 4.1** to exercise real OTLP→Prometheus translation (delete the table-equality escape hatch) per design §8. (MINOR but corrects a design-requirement dilution)
6. **Specify the `main.go` extractions** (`signalContext`, `requireWriters`, `selectStore`, `storeChoice`) with before/after and helper bodies. (MAJOR, folded with #3)
7. **Split Task 1.7** into 3 right-sized tasks. (MINOR)
8. **Disambiguate Contract A error transport** (non-200 + envelope) and **make Task 3.6's replica choice concrete** (default to filesystem replica, non-integration). (MINOR)
9. Provide the **radar site table data** (Task 2.3) inline rather than "reuse DRAS." (MINOR)

---

## What's genuinely good (brief)

- **Zero conformance drift on the three revised decisions** (O1/O3/O5) and full CRITICAL/HIGH traceability — the hard part of faithfulness is done.
- **Correct sequencing:** WS0 lands the lifecycle bedrock first; interface-breaking changes (Task 0.7) are explicitly confined to one commit with all implementers updated — the plan even flags the compile-ripple. This genuinely fixes the shutdown/drain/panic premise rather than reintroducing it.
- **Cross-task contracts (A/B/C) are single-sourced** and cited by dependents — textbook No-Wall/DRY for the variant boundaries.
- **Contract B is fully enumerated** (every instrument → Prometheus name + kind), which makes the OTel writer and the dashboard genuinely buildable.
- **RED tests are mostly concrete and behavior-asserting** (temp DBs, `httptest`, `tracetest.SpanRecorder`, `-race` on the concurrency tasks) — the test-first discipline is real where it counts, even where the GREEN side is thin.
- **CGO-free constraint is treated as load-bearing** and repeated at the points it matters (driver choice, Dockerfile) — the static-image property is protected.
