# Code Review Summary — tempest-display (UI)

**Repository:** [`github.com/jacaudi/tempest-display`](https://github.com/jacaudi/tempest-display)
**Reviewed commit:** [`49892063`](https://github.com/jacaudi/tempest-display/tree/49892063dab064a7864c4810b4d266b9ea3f4985) (branch `main`)
**Date:** 2026-07-17
**Reviewer:** Claude Code, via 6 parallel senior-engineer review agents (file-by-file, line-by-line)
**Scope:** All 24 tracked source files (~3,480 LoC frontend + a 63-line Go static server), plus build/config/container/docs.

> All `file:line` references below link to the exact line at commit `49892063`. Permalink base:
> `https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/`

---

## 1. What this project is (as-built)

`tempest-display` (`package.json` name `tempest-app`) is a **React 19.2 + TypeScript 5.9 + Vite 7** glassmorphism weather dashboard for WeatherFlow Tempest stations, served by a **zero-dependency Go static-file server** (`server/main.go`) that embeds the built `dist/` via `//go:embed` and serves it with an SPA fallback. Seven themes, unit toggles, ~15 weather cards.

**The single most important architectural fact:** the app currently runs **entirely on stub data**. Every function in `src/api/tempestApi.ts` returns fixtures from `stubData.ts`; the real network code exists only as commented-out `// TODO` blocks. The Go server is **not a proxy** — it never talks to WeatherFlow. So the whole review splits into two questions: *is the shell well-built?* (largely yes) and *is the not-yet-written data path safe?* (there is a latent HIGH security trap baked into the commented design).

### Review partition

| Agent | Area | Files |
|---|---|---|
| A | App shell + global CSS | `index.html`, `src/main.tsx`, `src/App.tsx`, `src/App.css` (1177 lines), `src/index.css` |
| B | Data layer | `src/api/tempestApi.ts`, `src/api/stubData.ts`, `src/hooks/useWeatherData.ts`, `src/hooks/useUnits.ts`, `src/types/weather.ts` |
| C | Components 1 | GlassCard, Header, TemperatureHero, Wind, Humidity, Pressure, Rain |
| D | Components 2 | Lightning, SolarUV, StationHealth, ForecastStrip, Almanac, WeatherIcon, SettingsPanel, `themes/themes.ts` |
| E | Go backend | `server/main.go`, `server/go.mod` |
| F | Build / config / container / deps / docs | Dockerfile, `vite.config.ts`, 3× tsconfig, ESLint, `package.json`, README, etc. |

---

## 2. Findings roll-up

| Severity | Count |
|---|---:|
| 🔴 CRITICAL | 0 |
| 🟠 HIGH | 8 |
| 🟡 MEDIUM | 25 |
| 🔵 LOW | 20 |
| ⚪ NIT | 18 |
| 🟢 POSITIVE | 27 |

**Verified non-issues (good news):** no committed secrets; **no `VITE_`-embedded token** (token is runtime user-entered, not build-baked); no client-side token *persistence* today (the token input stores nothing); path traversal is structurally impossible (read-only `embed.FS` + `fs.ValidPath`); the moon-phase geometry and **all 10 unit conversions are mathematically correct**; WeatherIcon has a safe default fallback (no crash on unknown conditions).

---

## 3. Cross-cutting themes

1. **The app ships fixtures as production data.** The entire API client is stub-only (B-H2); a deployed container shows a hard-coded Seattle station with randomized wind and no indication the data is fake. Everything below about "the data path" is about code that doesn't exist yet but is partly designed in comments.

2. **The commented-out live design is a token-exposure trap.** `API_BASE` points straight at `swd.weatherflow.com`, and every reference fetch appends `?token=${_apiToken}` — in the URL query string, from browser JS, with **no backend proxy** to hold the secret (E confirmed the Go server is static-only). Uncommenting as written ships a WeatherFlow Personal Access Token to every visitor and leaks it via server/proxy logs and `Referer`. This is flagged three times from three angles (B-H1, E token assessment, F-M token). **The fix — add a proxy endpoint to the Go server and fetch tokenless relative URLs — should be decided before the data path is implemented.**

3. **The shell is not production-hardened for a 24/7 display.** No error boundary (A-H1) → one card's render error blanks the whole screen; the loading/error/Retry CSS is *referenced but never defined* (A-H2) → the first screen users see is broken; zero media queries (A-H3) → non-responsive on the tablets/phones such displays actually run on; no `prefers-reduced-motion` (A-H4) despite many infinite animations.

4. **Both container/server layers miss basic hardening.** The Go server has no HTTP timeouts (E-H1, Slowloris) and no graceful shutdown; the Docker image runs as **root** (F-H1). Both are one-line-ish fixes.

5. **No tests, no CI.** No test runner is configured and there's no `.github/` at all (F-M) — so the pure functions most worth testing (unit conversions, moon phase, battery curve, day-name derivation) are unguarded, and lint/typecheck/build are never enforced.

6. **Small civil-calendar / i18n correctness bugs.** Hard-coded `°N/°W` hemisphere suffixes (C-M) render wrong coordinates for non-US stations; `ForecastStrip.getDayName` uses the current year (D-M) → wrong weekday labels across a Dec→Jan boundary; sunrise/sunset shown in the viewer's timezone, not the station's (C/D).

---

## 4. 🟠 HIGH (8)

### App shell (Review A)

- **A-H1 · No error boundary anywhere** — [`src/App.tsx:51-88`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.tsx#L51-L88). ~10 cards render with no `ErrorBoundary`/`getDerivedStateFromError` anywhere in `src/`. A render-time exception in any one card unmounts the entire app to a blank page — the worst outcome for a wall display. Fix: wrap the dashboard (or each card) in an error boundary with a fallback.
- **A-H2 · Loading/error/Retry styles used but never defined** — [`src/App.tsx:30-47`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.tsx#L30-L47). `.loading-screen`, `.loading-spinner`, `.error-screen`, `.glass-btn` have no CSS in either stylesheet, so the initial spinner is an invisible zero-size div and the failure screen + Retry button are unstyled. This is literally the first screen every visitor sees.
- **A-H3 · No responsive design** — [`src/App.css:240-244`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.css#L240-L244). `grid-template-columns: repeat(3, 1fr)` is fixed and there is **not one `@media` rule** in the codebase; span-2/span-3 cards force a rigid 3-up layout on phones/tablets.
- **A-H4 · No `prefers-reduced-motion`** — [`src/App.css:57-62`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.css#L57-L62). Continuous `orbFloat`/`pulse`/`rainfall`/`flashBadge` animations with no reduce-motion opt-out (WCAG 2.3.3 / vestibular safety).

### Data layer (Review B)

- **B-H1 · Commented live-API design embeds the token in the frontend; no proxy exists** — [`src/api/tempestApi.ts:69-72`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/api/tempestApi.ts#L69-L72). PAT in `?token=` query string, from browser JS, static-only backend. Route through a Go proxy that injects the token server-side; frontend uses tokenless relative URLs. (See §3.2.)
- **B-H2 · Entire API client returns stub data — no real data path** — [`src/api/tempestApi.ts:49-133`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/api/tempestApi.ts#L49-L133). The app renders fixtures in production with no error and no "fake data" indication. Must not ship as a release without a loud dev-only/offline banner until the real fetch layer (with `response.ok`/timeout/validation) lands.

### Backend (Review E)

- **E-H1 · Missing HTTP server timeouts → Slowloris** — [`server/main.go:59`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/server/main.go#L59). `http.ListenAndServe` uses a zero-value `http.Server` (all timeouts 0). The scratch image binds `0.0.0.0:3000` and ships standalone, so slow-header connections can exhaust goroutines/FDs with no auth. Fix: explicit `http.Server` with `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`.

### Container (Review F)

- **F-H1 · Final container runs as root (UID 0)** — [`Dockerfile:42`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/Dockerfile#L42). `FROM scratch` with no `USER`. The Go binary needs no root and PodSecurity `restricted`/OpenShift reject UID-0 containers. Fix: `USER 65532:65532` (or switch to `distroless/static:nonroot`, which also ships CA certs the planned proxy will need).

---

## 5. 🟡 MEDIUM (25)

**App shell (A):** no `:focus-visible`/outline anywhere (WCAG 2.4.7) [`index.css`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/index.css#L57-L64); blank-screen when a successful fetch yields null `current` [`App.tsx:49`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.tsx#L49); all ~10 cards re-render on every ~3s tick (no `React.memo`) [`App.tsx:66-78`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.tsx#L66-L78); external Google Fonts dependency, no CSP, breaks offline/air-gapped kiosks [`index.html:8-10`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/index.html#L8-L10); `transition: all` on blur-heavy cards [`App.css:73`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/App.css#L73).

**Data layer (B):** `loadData` has no AbortController/overlap guard → stale-response race once real latency exists [`useWeatherData.ts:44-75`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/hooks/useWeatherData.ts#L44-L75); `savePrefs` calls `localStorage.setItem` with no try/catch (throws in private mode / quota) [`useUnits.ts:26-28`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/hooks/useUnits.ts#L26-L28); no runtime validation of untrusted input (localStorage prefs spread unchecked; future API JSON `as`-cast) [`useUnits.ts:18-24`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/hooks/useUnits.ts#L18-L24); token-in-URL smell even server-side.

**Components 1 (C):** `GlassCard` `role="button"`+`tabIndex` with **no `onKeyDown`** → focusable but not keyboard-activatable (WCAG 2.1.1), baked into the shared wrapper [`GlassCard.tsx:14-20`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/GlassCard.tsx#L14-L20); hard-coded `°N/°W` renders wrong coordinates for southern/eastern-hemisphere stations [`Header.tsx:28`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/Header.tsx#L28); systemic missing-field handling — `current.uvIndex.toFixed(1)` **throws** (unmounts dashboard) while `formatX`/`Math.round` sites degrade to visible `NaN` [`TemperatureHero.tsx:62`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/TemperatureHero.tsx#L62).

**Components 2 (D):** SettingsPanel Station ID / API Token inputs are **dead UI** — uncontrolled `defaultValue=""`, no wiring, no matching pref field [`SettingsPanel.tsx:104-119`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/SettingsPanel.tsx#L104-L119); settings modal has no `role="dialog"`/`aria-modal`, no Escape, no focus trap/restore [`SettingsPanel.tsx:17-18`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/SettingsPanel.tsx#L17-L18); `themes.ts` `desert-sunset` omits `--text-shadow` and `applyTheme` never clears prior vars → stale shadow leaks across theme switches [`themes.ts:223-233`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/themes/themes.ts#L223-L233); `ForecastStrip.getDayName` uses the current year → wrong weekday across a year boundary, ignoring the entry's own `sunrise` epoch [`ForecastStrip.tsx:16-20`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/ForecastStrip.tsx#L16-L20).

**Backend (E):** no graceful shutdown / signal handling → truncated responses on every restart [`server/main.go:59-61`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/server/main.go#L59-L61); `Cache-Control: immutable` set before the existence check → a missing `/assets/*` returns `index.html` cached for a year (cache poisoning + wrong 404 semantics) [`server/main.go:34-36`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/server/main.go#L34-L36); no security response headers (nosniff/CSP/frame/referrer) [`server/main.go:32-55`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/server/main.go#L32-L55).

**Build / config (F):** base images float on mutable tags (no digest pin) [`Dockerfile:4`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/Dockerfile#L4); README documents phantom build args (`VITE_ENABLE_RADAR`, `VITE_RADAR_TILE_HOST`) and dead `docs/*.md` links [`README.md:37`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/README.md#L37); **no CI** (no `.github/`) → lint/typecheck/build never enforced [`package.json:6`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/package.json#L6); **no test tooling / no `test` script**; TS strictness omits `noUncheckedIndexedAccess` despite index-heavy code [`tsconfig.app.json:20`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/tsconfig.app.json#L20); the live-API token-in-URL design (same root as B-H1) [`tempestApi.ts:35`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/api/tempestApi.ts#L35).

---

## 6. 🔵 LOW & ⚪ NIT (highlights)

- **A:** default Vite favicon still shipped; `100vh` vs `100dvh` (mobile viewport bug); `background-attachment: fixed` janky on iOS; `App` ignores the `hourly` data the hook fetches (dead round-trip); unsafe `as ThemeName` cast; ~7 near-identical card-label styles (DRY); a single `!important`.
- **C:** `RainCard` recomputes `Math.random()` raindrops every render (reshuffle on each poll); WindCard has no "Calm" handling (0 wind shows a direction); Header time in viewer TZ not station TZ; decorative SVGs lack `aria-hidden`; `GlassCard` className double-space; magic pressure/solar thresholds.
- **D:** Almanac `daylightDuration` mishandles polar day/night (negative durations); WeatherIcon mapping incomplete (`foggy`/`sleet`/`possibly-*` silently render "partly cloudy"); decorative header SVGs not `aria-hidden`; forecast `key={i}` has a natural stable id; WeatherIcon uses UMD `React` namespace; `UserPreferences.theme` typed `string`; `signalBars` `number[]` used as booleans; `timeSince` returns negative strings for future timestamps.
- **E:** `PORT` parsed with `fmt.Sscanf` (ignored error, silent fallback); cache policy keyed on URL prefix not file identity; no server tests; double `stat` on happy path; log advertises `http://0.0.0.0:3000`; `go.mod` pins only minor Go version.
- **F:** no HEALTHCHECK (constrained by scratch); `.gitignore` doesn't ignore `.env`; Google-CDN fonts (offline/privacy); no Prettier; ESLint `ecmaVersion 2020` lags tsconfig target; README says "six themes" but ships seven; `package.json` name `tempest-app` ≠ repo, version `0.0.0`; `renovate.json` is schema-only (no `config:recommended`).

---

## 7. What's genuinely well done (🟢 27 positives)

- **All 10 unit conversions are mathematically correct** (°C↔°F, m/s↔mph/kph/kts, mb↔inHg, mm↔in — each verified against reference constants) — [`useUnits.ts:32-57`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/hooks/useUnits.ts#L32-L57). The strongest part of the data layer.
- **Moon-phase geometry is correct and elegantly derived**, verified across all phases with a clean quarter-degeneracy guard — [`AlmanacCard.tsx:11-49`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/AlmanacCard.tsx#L11-L49).
- **Wind compass shortest-arc animation** (normalizes delta to `[-180,180]`) and correct `degToCompass` wrap — [`WindCard.tsx:20-31`](https://github.com/jacaudi/tempest-display/blob/49892063dab064a7864c4810b4d266b9ea3f4985/src/components/WindCard.tsx#L20-L31).
- **Every displayed measurement routes through the units hook** — no raw-unit leakage into any card.
- **Backend is minimal and safe by construction:** zero third-party deps (stdlib-only), embedded assets on `scratch`, path traversal structurally prevented by read-only `embed.FS` + `fs.ValidPath`, no hardcoded secrets, correct KISS/YAGNI scope — Review E.
- **Strong build/toolchain posture:** `strict: true` + correct TS project references, `eslint-plugin-react-hooks` recommended (exhaustive-deps on), `npm ci --ignore-scripts`, `CGO_ENABLED=0`/`-trimpath`/`-ldflags="-s -w"`, thorough `.dockerignore`, **no `VITE_`-embedded secret** (token runtime-entered) — Review F.
- **Clean, idiomatic types** (const-object enums, per-field unit comments, no `any`), correct effect/interval cleanup in the WS stub, correct staleness semantics (retain prior data on failure) — Review B.
- **Consistent numeric clamping** where it matters (battery %, UV/lightning bar positions), correct cross-type field selection (`status.batteryLevel` vs `current.battery`), WeatherIcon safe default — Reviews C/D.

---

## 8. Recommended priority order

1. **Decide the data-path security model before writing it:** add a proxy endpoint to `server/main.go` (token from env, injected server-side) and have the frontend fetch tokenless relative URLs (B-H1). Do not uncomment the token-in-URL design.
2. **Harden the two infra layers (one-liners):** HTTP server timeouts (E-H1) and non-root container `USER` (F-H1); add graceful shutdown + a `/healthz` route while there.
3. **Make the shell production-safe for a 24/7 display:** error boundary (A-H1), define the missing loading/error/Retry CSS (A-H2), add breakpoints (A-H3) and `prefers-reduced-motion` + `:focus-visible` (A-H4 / A-MEDIUM).
4. **Fix the correctness bugs users will actually see:** hemisphere suffixes (C-M), forecast weekday across year boundary (D-M), theme-var leak (D-M), and route UV through a `formatX` helper so a missing field can't hard-crash (C-M).
5. **Wire or remove the dead SettingsPanel config** (D-M) and add dialog semantics/Escape/focus-trap.
6. **Establish the safety net:** add Vitest + a `test` script and cover the pure math (conversions, moon phase, battery, day-name); add a CI workflow running lint/typecheck/build/`docker build` (F-M); turn on `noUncheckedIndexedAccess`.
7. **Before shipping stub-only:** add a loud "demo/offline data" banner so fixtures are never mistaken for live readings (B-H2).
8. **Supply-chain/docs polish:** digest-pin base images, fix README drift (phantom args, dead links, theme count), self-host fonts, enrich `renovate.json`.

---

*Detailed per-file findings (every `file:line`, GitHub permalink, failure scenario, and suggested fix) were produced by the six review agents and consolidated here. Every finding in this document links to the exact line at commit [`49892063`](https://github.com/jacaudi/tempest-display/tree/49892063dab064a7864c4810b4d266b9ea3f4985).*
