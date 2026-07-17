# Code Review Summary — tempestwx-exporter

**Date:** 2026-07-17
**Reviewed commit:** `017a852` (branch `main`) + the one uncommitted local change (`.gitignore`)
**Reviewer:** Claude Code, via 7 parallel senior-engineer review agents (file-by-file, line-by-line)
**Scope:** Every tracked Go file in the `main` working tree (20 `.go` files, ~2,760 LoC), plus all build/CI/infra/docs, plus the uncommitted `.gitignore` edit.

---

## 1. How this review was done

The repository was partitioned across 7 background review agents, each reading **every line** of its assigned files (no skimming, no sampling), cross-checking symbols in neighbouring packages, and running the critical-thinking verification gate before reporting:

| Agent | Area | Files |
|---|---|---|
| A | Entry point + fan-out | `main.go`, `internal/sink/` |
| B | Config + DB schema | `internal/config/`, `internal/postgres/schema.go` |
| C | Postgres batch writer | `internal/postgres/writer.go` (+test) |
| D | Prometheus layer | `internal/tempest/`, `internal/prometheus/` |
| E | UDP parsing + wet-bulb | `internal/tempestudp/` |
| F | REST API client | `internal/tempestapi/` |
| G | Build / CI / infra / docs / deps | Dockerfile, `.github/`, go.mod, README, etc. |

I also gathered independent ground-truth evidence (below) to cross-check agent claims rather than take them on faith.

### Scope notes (verified, not assumed)

- **Uncommitted local change:** the only uncommitted change on `main` is `.gitignore` (adds `# Auto Claude data directory` / `.auto-claude/`). Trivial and correct. **Included in scope as requested.**
- **The 16.9 MB `tempestwx-utilities` binary** in the repo root is a local build artifact — **not tracked** (confirmed via `git ls-files --error-unmatch` → "not tracked" and `git check-ignore` → matched by `.gitignore`). Not a finding.
- **Stale worktree:** `.worktrees/uuid-raw-values/` (branch `feature/uuid-raw-values-v1.0.0`, `bd3cf99`) is a *prunable, gitignored, untracked* worktree whose git link is broken and whose code has **diverged** from `main` (it lacks the Prometheus scrape endpoint that `main` has; `main` lacks some of its commits). It is committed work on a pushed branch — **not** "local-only uncommitted code" — so it was **not** deep-reviewed. Recommend `git worktree prune` / `git worktree remove` to remove dead clutter that also pollutes repo-wide greps.

---

## 2. Objective ground-truth (independently verified)

```
go version   : go1.26.4 (go.mod targets go 1.24.0, toolchain go1.25.5)
go build ./... : clean ✅
go vet ./...   : clean ✅
gofmt        : internal/tempest/metrics.go is NOT gofmt-clean ✗
```

**Test coverage — total 35.2%** (`go test -cover ./...`):

| Package | Coverage | Notes |
|---|---:|---|
| `internal/config` | 100.0% | but tautological (asserts `Sprintf` output; no round-trip) |
| `internal/sink` | 93.8% | happy paths only; no `-race`, no panic/partial-failure |
| `internal/tempestapi` | 90.5% | broad on filtering; **blind to status/timeout paths** |
| `internal/prometheus` | 90.1% | smoke-level; misses bind-fail + concurrency |
| `internal/tempestudp` | 82.5% | good obs coverage; **the CRITICAL panic path untested** |
| **`internal/postgres`** | **9.2%** | 4 of 5 tests are `t.Skip`; SQL/retry/drain untested |
| `internal/tempest` | 0.0% | descriptors only |
| `main.go` | 0.0% | untested (mode-selection logic, where a HIGH bug lives) |

The postgres writer's highest-risk functions — **every** `batch*`/`flush*`/`insert*` path for rapid_wind, hub_status, and events, plus `flushWithRetry`, `isRetryable`, `Close`, and the shutdown drain — are at **0%**. Only `handleObservationReport` (88.9%) is meaningfully exercised.

---

## 3. Findings roll-up

| Severity | Count |
|---|---:|
| 🔴 CRITICAL | 1 |
| 🟠 HIGH | 17 |
| 🟡 MEDIUM | 31 |
| 🔵 LOW | 23 |
| ⚪ NIT | 14 |
| 🟢 POSITIVE | 37 |

**The single most important structural fact:** the codebase has **multiple unguarded panic paths reachable from untrusted network input**, and **the fan-out layer has no `recover()`** — so any one of them takes down the whole exporter. These interlock (see §4). Second-most-important: **graceful shutdown is broken in several independent ways**, so buffered PostgreSQL data is lost on a normal container stop.

---

## 4. Cross-cutting themes (the issues that reinforce each other)

These matter more together than as isolated line items:

1. **Remote-crash chain.** `report.go:263-264` indexes `RadioStats[1]/[2]` with **no length guard** (CRITICAL) → panics on a short/malformed `hub_status` UDP packet from anyone on the LAN. The UDP receive path (`sink.go`) has **no panic recovery** (Review A, MEDIUM) → the panic is not contained → the process dies. Trivial remote DoS. Fix both: guard the index *and* add `defer recover()` in the fan-out goroutines.

2. **Send-on-closed-channel panics on shutdown, in two packages.** Both `postgres/writer.go` (Review C, HIGH) and `prometheus/writer.go` (Review D, HIGH) close channels in `Close()` while producers may still `select`-send — and a `default:` case does **not** prevent a send on a closed channel from being chosen. A late in-flight UDP write during shutdown crashes the process. Neither `Close()` is idempotent (double-close panic). Same root cause, same fix pattern (`sync.Once` + a `done` channel gating sends).

3. **Graceful shutdown loses buffered Postgres data — three independent causes.**
   - `main.go:29` traps only SIGINT, **not SIGTERM** → `docker stop`/k8s never trigger graceful shutdown at all (Review A, HIGH).
   - Even if it did, cleanup runs under the **already-cancelled** signal context (`main.go:33-34` + `sink.go`), so flushes abort immediately (Review A, MEDIUM).
   - And the writer derives flush timeouts from that same cancelled ctx, and its `ctx.Done()` worker branches flush only the local slice, not the 1000-deep channel buffer (Review C, HIGH).
   All three must be fixed for shutdown to actually persist buffered rows.

4. **Errors that can't fire / can't be seen.** `MetricsServer.Start()` swallows bind failures in a goroutine and always returns nil (Review D, HIGH), so `main.go`'s error check is **dead code** (Review A, HIGH) — a failed `/metrics` endpoint runs silently. Similarly, `sink.SendReport/SendMetrics` always return nil, so the caller's error checks are dead (Review A, LOW).

5. **HTTP-client footguns, all at once, in the API client** (Review F): no request timeout, no status-code check, token in the URL query string (leaks to proxy logs *and* into the `*url.Error` the caller `log.Fatalf`s), and a `log.Fatalf` inside a library method. Each is independently HIGH.

6. **Docs ↔ code drift & inert knobs.** README documents an obsolete env-var scheme (`PUSH_URL`, `DATABASE_*`) so the headline quickstart cannot work (Review G, HIGH); and the documented `POSTGRES_BATCH_SIZE`/`FLUSH_INTERVAL`/`MAX_RETRIES` tunables are hardcoded and never read from the environment (Review C, HIGH).

7. **The highest-risk code has the lowest test coverage.** The postgres writer (9.2%) and `main.go` mode-selection (0%) are exactly where the CRITICAL-adjacent lifecycle bugs live.

---

## 5. 🔴 CRITICAL

### C-1 · `RadioStats` indexed without a length guard → remote panic/DoS
`internal/tempestudp/report.go:263-264`
```go
prometheus.MustNewConstMetric(tempest.Reboots,   prometheus.CounterValue, r.RadioStats[1], ...),
prometheus.MustNewConstMetric(tempest.BusErrors, prometheus.CounterValue, r.RadioStats[2], ...),
```
`RadioStats []float64` comes straight from the untrusted `radio_stats` JSON of a `hub_status` UDP packet. Every other array path in the file is length-guarded; this one is not. A packet with `radio_stats` missing / `null` / `[]` / `<3` elements panics with index-out-of-range. Combined with the missing `recover()` in `sink.go`, **any host on the LAN can crash the exporter with a single datagram.** Fix: `if len(r.RadioStats) >= 3 { … }` before indexing (guard shown in the agent report), and add panic recovery in the sink fan-out.

---

## 6. 🟠 HIGH (17)

**main.go / sink (Review A)**
- **A-H1 · SIGTERM ignored** — `main.go:29` `signal.NotifyContext(..., os.Interrupt)` only. `docker stop`/k8s send SIGTERM → graceful shutdown never fires → buffered Postgres rows lost. Fix: add `syscall.SIGTERM`.
- **A-H2 · gzip-only API export mode is unreachable** — `main.go:86-95`. In `TOKEN` mode only Postgres can register as a writer, so `TOKEN + KEEP_EXPORT_FILES` with no Postgres dies on "no writers configured" despite being a documented mode. Enforce the writer-count invariant only in UDP mode.
- **A-H3 · metrics-server start error check is dead code** — `main.go:60-64`. `Start()` always returns nil; real bind failures surface only inside a goroutine. (Root cause: D-H2.)

**postgres/writer.go (Review C)**
- **C-H1 · buffered data lost on shutdown** — `writer.go:179,270,346,402`. Flush inserts use `context.WithTimeout(w.ctx, …)`; if the app cancels `w.ctx` before/while `Close()` drains, every insert fails `context.Canceled`, `isRetryable`→false, batch dropped. Also the `ctx.Done()` worker branches flush only the local slice, not the buffered channel. Fix: drain under a fresh `context.Background()`-derived context, driven by `Close()` only.
- **C-H2 · documented tunables are inert** — `writer.go:133-135`. `POSTGRES_BATCH_SIZE`/`FLUSH_INTERVAL`/`MAX_RETRIES` are hardcoded; grep confirms nothing reads them. Wire them into the constructor or remove from docs.
- **C-H3 · producer/Close race → send-on-closed / double-close panic** — `writer.go:857-860` + all send sites. No `sync.Once`, no shutdown gate. Fix: `sync.Once` + `done` channel gating sends; make `Close()` idempotent.

**prometheus (Review D)**
- **D-H1 · send on closed channel panics** — `writer.go:60-61,71,82-83,91-92`. `Close()` closes `outbox`/`more` while `WriteMetrics`/`Flush` may still send; `default:` does not protect a send case. Crash on shutdown. Fix: `done` channel / mutex+closed-flag; don't close a channel producers still use.
- **D-H2 · `Start()` swallows bind errors** — `server.go:64-74`. `ListenAndServe` errors only logged in the goroutine; `Start()` always returns nil → `/metrics` fails silently on an occupied/invalid port. Fix: `net.Listen` synchronously, return the bind error, then `go Serve(ln)`.

**tempestapi/client.go (Review F)** — all four are independent and each production-blocking:
- **F-H1 · TOKEN in URL query string** — `client.go:34,90`. Leaks to intermediary access logs and into Go's `*url.Error` (which redacts userinfo, *not* query params), then logged verbatim by `main.go:171/202` `log.Fatalf`. Move to `Authorization: Bearer` header (verify against current WeatherFlow API docs) and scrub error URLs.
- **F-H2 · no HTTP status-code check** — `client.go:39-43,96-100`. 401/403/429/5xx decoded/parsed as success → auth failures and rate limits abort the export with a wrong diagnosis. Fix: check `resp.StatusCode` before decode.
- **F-H3 · no request timeout** — `client.go:39,96` use `http.DefaultClient` (Timeout 0) and the caller ctx has no deadline → a stalled endpoint hangs indefinitely. Fix: dedicated `&http.Client{Timeout: 30*time.Second}`.
- **F-H4 · `log.Fatalf` inside a library method** — `client.go:116`. An unexpected report type `os.Exit(1)`s the whole process. Fix: return an error.

**Infra / CI / docs (Review G)**
- **G-H1 · Docker action references undeclared `inputs.push`** — `.github/actions/docker/action.yml:50`. Commit `e722581` deleted the `push`/`latest`/`tag-strategy` input declarations but left the `fromJSON(inputs.push)` reference → `fromJSON('')` throws on every push/tag; workflows still pass three now-dead inputs. Image publish is very likely broken — verify against an Actions run.
- **G-H2 · README quickstart is broken** — documents `PUSH_URL`/`DATABASE_*` env vars that no longer exist (renamed in PR #26 to `ENABLE_PROMETHEUS_*`/`POSTGRES_*`). The headline `docker run` cannot start the tool. CLAUDE.md is accurate; README must be brought in line.
- **G-H3 · no `permissions:` block in any workflow** — inherits default `GITHUB_TOKEN` scope (not least-privilege; also a latent functional risk for ghcr push + GoReleaser release creation). Set explicit least-privilege per workflow.

---

## 7. 🟡 MEDIUM (31)

**main.go / sink (A):** cleanup uses already-cancelled context (data loss) `main.go:33-34`; gzip export file opened without `O_TRUNC` (corrupt overwrite) `main.go:243`; `strconv.ParseBool` errors discarded — a typo (`ENABLE_POSTGRES=yes`) silently disables a sink `main.go:39,54,69,99,187`; a slow/blocked writer stalls the single UDP read loop (no per-writer timeout) `sink.go:58-101`; no panic recovery in fan-out goroutines `sink.go:66-125` (see C-1 chain).

**config / schema (B):** no migration path — `CREATE TABLE IF NOT EXISTS` silently won't apply column changes to existing DBs, and the "safe on every startup" comment overstates evolution safety `schema.go:10-31`; tautological config tests with no special-char or round-trip case `database_test.go`.

**postgres/writer.go (C):** "critical" events silently dropped under backpressure (non-blocking send drops discrete lightning/rain-start rows when the DB is slow) `writer.go:382,514-637`; `Flush()` is a no-op that misrepresents durability `writer.go:765-769`; `isRetryable` defaults unknown errors to *retryable* and the batch retry is non-transactional `writer.go:812-852`; retry backoff runs synchronously in the ingesting worker (backpressure amplification) `writer.go:772-810`.

**prometheus (D):** every Desc uses the **reserved `instance` label** → renamed to `exported_instance` on scrape, breaking dashboards `metrics.go:32-50`; `tempest_wind_ms` ambiguous unit (m/s vs ms) `metrics.go:40`; explicit UDP timestamps re-served on the scrape endpoint → Prometheus "sample too old" rejection when a station goes quiet `server.go:135-141`; failed push permanently loses the drained batch `writer.go:98-105`; uptime modeled as a counter with `_total` though it resets on reboot `metrics.go:32`; `RainTotal` declared/registered/described but never emitted (dead) `metrics.go:45,66`.

**tempestudp (E):** `wetBulb` `panic("failed to converge")` on a network-fed calculation `wetbulb.go:47`; `evt_precip`/`evt_strike`/`device_status` emit no metrics — silent drop on the metrics path (confirm intentional) `report.go:64,82,222`; missing obs values unmarshal to `0.0`, indistinguishable from a true zero → skews averages/`rate()` `report.go:134,147`.

**tempestapi (F):** API `status.status_code` decoded but never checked (API-level errors read as "0 stations") `client.go:56-60`; token not URL-escaped `client.go:34,90`; full response body logged unbounded on parse error `client.go:108`.

**Infra / CI (G):** third-party Actions pinned to floating tags not SHAs; `go clean -modcache` + `go mod tidy` in CI (no caching, masks module drift); `setup-go@v4` stale/inconsistent with `@v6`; CI Go `>=1.20` vs module `go 1.24.0`; base images floating tags (non-reproducible builds); GoReleaser runs with no `.goreleaser.yml` in the repo; `github.ref_name` interpolated into a `run:` script (injection antipattern); `.dockerignore` misses the local binary + `docs/`.

---

## 8. 🔵 LOW & ⚪ NIT (highlights)

- **A:** `SendReport`/`SendMetrics` always return nil → caller error-checks are dead; single transient API fetch error `log.Fatalf`s a multi-hour backfill; gzip/file `Close()` errors ignored; dead "removed" comments.
- **B:** integer counts stored as `DOUBLE PRECISION` (`lightning_strike_count`, `reboot_count`, `bus_errors`); overlapping unique+DESC indexes double write cost; use `t.Setenv`; `cmp.Or` for defaults.
- **C:** `batchSize` dead for obs/events (batching only half-applied); `WriteMetrics` reconstruction brittle (`strings.Contains` matching, lossy fields, 1970 timestamps); ctx stored in struct field; duplicated batch-execute procedure across four `insertX`; drop logs lack serial/timestamp context.
- **D:** missing `IdleTimeout`/`ReadHeaderTimeout` on the HTTP server; `Collect` holds `RLock` while sending; lossy `metricKey` fallback; `/health` ignores write error and accepts any method; `metrics.go` not gofmt-clean; `init()`+`var`+`All` triple-sync surface.
- **E:** uptime counter/`_total` vs reset-on-reboot; pre-1.22 loop (`for range 10000`); `if/else` indent-error-flow; param `bytes` shadows stdlib; double JSON unmarshal.
- **F:** unbounded `io.ReadAll`; `time.Unix` local-zone timestamps; value receivers / no owned `*http.Client`.
- **G:** `go build main.go` vs `go build .`; no `HEALTHCHECK`/`EXPOSE`; `.gitignore` lists tracked `CLAUDE.md`; unused `SOURCE_COMMIT` bake var; YAML trailing whitespace (add `actionlint`).

---

## 9. What's genuinely well done (🟢 POSITIVE — 37 total)

- **UDP field-index mapping is correct.** Every `obs_st` and `rapid_wind` index was verified against the Tempest UDP v143 field order and the metric descriptors; length guards on those two paths are tight and complete (Review E). The inline field-index docs are excellent.
- **SQL is correct and safe.** All four INSERTs have fully-aligned column/placeholder/value mappings (verified field-by-field), are entirely parameterized (no injection surface), and retries are **idempotent** via UUIDv7-generated-once + `ON CONFLICT DO NOTHING` (Review C). The CRITICAL-class "swapped column" risk this file was flagged for is **not present**.
- **The UDP listener's shutdown is subtly correct** — buffered `readErr` channel raced against `ctx.Done()`, `defer sock.Close()` unblocks the parked read; no goroutine leak (Review A).
- **The scrape collector `latestMetricsCollector` is genuinely concurrency-correct** (RWMutex, last-value-wins), and HTTP graceful shutdown distinguishes `http.ErrServerClosed` (Review D).
- **Wet-bulb math is legitimate** — standard Bolton (1980) + psychrometric with correct units and a correct shrinking-step solver (Review E).
- **Container hardening is strong** — non-root `USER 65532`, static Chainguard base, `CGO_ENABLED=0`, stripped binary; tests genuinely gate merges/releases; deps current; only `GITHUB_TOKEN` used; ghcr login gated to non-PR (Review G).
- **Config layer is clean and appropriately KISS/YAGNI**, schema uses correct UNIQUE constraints backing the dedup and correct nullable value columns (Review B).

---

## 10. Recommended priority order

1. **Stop the remote crashes (do first):** guard `RadioStats` (C-1) **and** add `recover()` in the sink fan-out; fix both send-on-closed-channel panics (C-H3, D-H1) with `sync.Once`+`done`.
2. **Fix graceful shutdown / data durability:** SIGTERM (A-H1), cleanup context (A-MEDIUM), writer drain context + channel-buffer drain (C-H1). Reconsider dropping "critical" events under backpressure (C-MEDIUM).
3. **Make the API client production-grade:** timeout, status check, token-in-header, remove `log.Fatalf` (F-H1..H4).
4. **Surface silent failures:** `Start()` bind error (D-H2 → A-H3); wire the Postgres tunables or delete the docs (C-H2); fix the unreachable gzip export mode (A-H2).
5. **Fix the release pipeline & docs:** Docker action input mismatch (G-H1), README env vars (G-H2), workflow `permissions:` (G-H3).
6. **Close the test gap where risk is highest:** `isRetryable` and routing/insert unit tests + a testcontainers integration test for the postgres writer (currently 9.2%); a `hub_status` short-`radio_stats` test (would have caught C-1); status-code/timeout tests for the API client; real DDL test for the schema.
7. **Metric hygiene:** rename `instance`→`serial`, fix `_ms`/`_total`/dead `RainTotal`, `gofmt` `metrics.go`.
8. **Housekeeping:** `git worktree prune` the stale `.worktrees/uuid-raw-values/`.

---

*Detailed per-file findings (with every `file:line`, failure scenario, and suggested fix) were produced by the seven review agents; this document consolidates and cross-references them. Coverage/build/vet figures above were independently re-run and verified.*
