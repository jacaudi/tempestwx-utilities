# Tempest Weather Station Utilities

Multi-backend data utilities for [Tempest weather stations](https://weatherflow.com/tempest-home-weather-system/).

This tool provides:
- **UDP Mode**: Listens for [Tempest UDP broadcasts](https://weatherflow.github.io/Tempest/api/udp.html) and forwards to Prometheus push gateway and/or PostgreSQL
- **API Export Mode**: Fetches historical data via REST API and stores to PostgreSQL and/or compressed files

## Quickstart

Container images are available at [GitHub
Container Registry](https://github.com/jacaudi/tempestwx-utilities/pkgs/container/tempestwx-utilities).

```bash
$ docker run -it --rm --net=host \
  -e PUSH_URL=http://victoriametrics:8429/api/v1/import/prometheus \
  ghcr.io/jacaudi/tempestwx-utilities

2023/07/06 20:18:55 pushing to "0.0.0.0" with job name "tempest"
2023/07/06 20:18:55 listening on UDP :50222
```

Note that `--net=host` is used here because UDP broadcasts are link-local and therefore cannot be received from typical
(routed) container networks.

## Exporter configuration

Minimal, via environment variables:

* `PUSH_URL`: the URL of the [Prometheus Pushgateway](https://github.com/prometheus/pushgateway) or other [compatible
  service](https://docs.victoriametrics.com/?highlight=exposition#how-to-import-data-in-prometheus-exposition-format)

* `JOB_NAME`: the value for the `job` label, defaulting to `"tempest"`

### PostgreSQL Storage (Optional)

The exporter can optionally write metrics to PostgreSQL in addition to (or instead of) Prometheus. Configure using either:

* `DATABASE_URL`: Full PostgreSQL connection string (e.g., `postgresql://user:pass@host:5432/dbname`)
* Or individual components: `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_USERNAME`, `DATABASE_PASSWORD`, `DATABASE_NAME`

When configured, the exporter automatically creates and maintains typed tables for observations, rapid wind data, hub status, and events.

See `CLAUDE.md` for detailed configuration options and Docker Compose examples

## Source Credit

- [tempest-exporter](https://github.com/willglynn/tempest_exporter) - Started as a fork of this project.
