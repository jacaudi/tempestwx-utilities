# Provenance

This directory (`web/`) is vendored from the owned fork
[`jacaudi/tempest-display`](https://github.com/jacaudi/tempest-display).

- **Source commit:** `49892063dab064a7864c4810b4d266b9ea3f4985`
- **Commit date:** 2026-03-05T16:34:13-08:00
- **Commit subject:** `Add renovate.json (#2)`
- **Vendored on:** 2026-07-19

## What was carried over

The full `src/`, `package.json`, `package-lock.json`, `vite.config.ts`,
`eslint.config.js`, `tsconfig*.json`, `index.html`, `public/`, and `LICENSE`
from the pinned commit, unmodified.

## What was intentionally dropped

- `server/` — upstream's 63-line Go static-file server. Replaced by the
  Go HTTP server built in a later task of this repository (embeds
  `web/dist` via `go:embed` directly).
- `Dockerfile` / `.dockerignore` — built the standalone `server/` binary;
  superseded by this repo's own multi-stage Dockerfile task.
- `renovate.json` — this monorepo already has a root-level Renovate
  config (`.github/renovate.json`) that covers `web/package.json`.
- upstream `.gitignore` — replaced by `web/.gitignore`, tailored to this
  monorepo's layout (keeps `web/dist/.gitkeep` tracked while ignoring
  built `dist/` output and `node_modules/`).

## Upgrading

To pull a newer upstream revision, re-run the vendoring process against
the new commit and update this file's `Source commit` fields. There is no
automated sync; `web/` is a vendored, editable copy, not a submodule.
