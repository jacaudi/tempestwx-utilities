# Basemap provenance

This directory is where an **operator-supplied** OpenStreetMap basemap lives:
`web/public/basemap/osm.pmtiles`. It is intentionally **not** committed to
this repository and **not** fetched at build/image time.

## Why

The radar card (Task 2.6, design §9/§14 P1.8, decision B2) renders NEXRAD
reflectivity on top of an OpenStreetMap basemap served same-origin as a
[Protomaps](https://protomaps.com/) `.pmtiles` file, read directly in the
browser via the `pmtiles://` MapLibre protocol (byte-range requests, no
tile server, no API key). Two properties of that basemap made "bake it into
the image" the wrong call:

- **The operator's region is unknowable at build time.** This exporter runs
  at whatever station location the operator deploys it to; a single
  baked-in basemap would either have to cover the whole planet (hundreds of
  MB, pointless for a single-station kiosk) or guess a region that's wrong
  for most installs.
- **`build.protomaps.com`'s dated builds expire in about a week.** A
  build-fetch step baked into the image build would silently start failing
  (or serve a stale/expired URL) days after the image was built --
  non-reproducible and a maintenance trap.

So the basemap is supplied by the operator at the deploy layer instead --
see the compose one-shot extract step (tracked separately, DOC.1). When
`osm.pmtiles` is absent, the map still renders (an empty/basemap-less map
under the radar overlay) rather than crashing -- **this is the intended
graceful degrade, not a bug.**

## How to produce one

Use the [`pmtiles` CLI](https://docs.protomaps.com/pmtiles/cli) to extract a
small regional slice from a Protomaps daily planet build, centered on the
station's location:

```bash
pmtiles extract https://build.protomaps.com/<DATE>.pmtiles \
  web/public/basemap/osm.pmtiles \
  --bbox=<minLon,minLat,maxLon,maxLat> \
  --maxzoom=14
```

Pick `<DATE>` from the current listing at https://build.protomaps.com/, and
a `--bbox` comfortably covering the station's location plus the NEXRAD
radar's typical ~150km coverage radius. Drop the resulting file at
`web/public/basemap/osm.pmtiles` (same-origin, served by this app's static
file server -- Vite copies anything under `public/` into `dist/` unchanged).

## Attribution

Per the OpenStreetMap license, the map **must** display:

> © OpenStreetMap contributors

`RadarCard` renders this via a visible MapLibre `AttributionControl` --
required regardless of which regional extract is in use.

## Missing file behavior

If `osm.pmtiles` is absent (e.g. the operator hasn't run the extract step
yet), MapLibre's tile requests for it 404. The radar card catches this
gracefully -- the map still renders (without basemap tiles) rather than
throwing. This is by design, not a defect to be "fixed" by baking in a
placeholder basemap.
