package main

import (
	"cmp"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tempestwx-utilities/internal/config"
	"tempestwx-utilities/internal/postgres"
	"tempestwx-utilities/internal/prometheus"
	"tempestwx-utilities/internal/sink"
	"tempestwx-utilities/internal/sqlite"
	"tempestwx-utilities/internal/tempestapi"
	"tempestwx-utilities/internal/tempestudp"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// Old collector implementation removed - now using MetricsSink

// notifyFunc matches signal.NotifyContext's signature so tests can inject a
// fake and assert on the exact signal set without sending real signals.
type notifyFunc func(parent context.Context, sig ...os.Signal) (context.Context, context.CancelFunc)

// signalContext derives a context that is canceled on SIGINT or SIGTERM,
// giving deferred cleanup (e.g. sink.Close, PostgresWriter.Close) a chance to
// run on graceful shutdown (resolves A-H1: SIGTERM was not handled).
func signalContext(parent context.Context, notify notifyFunc) (context.Context, context.CancelFunc) {
	return notify(parent, os.Interrupt, syscall.SIGTERM)
}

// Mode identifies which operational mode main is running in, since the
// "at least one writer" invariant differs between them (see requireWriters).
type Mode int

const (
	ModeUDP       Mode = iota // UDP listener (no TOKEN)
	ModeAPIExport             // historical export (TOKEN set)
)

// requireWriters enforces the "at least one writer" invariant, but only where
// it applies: UDP mode always needs a writer; API-export mode is satisfied by a
// DB writer OR KEEP_EXPORT_FILES (fixes A-H2 — gzip-only export was unreachable).
func requireWriters(mode Mode, writerCount int, keepFiles bool) error {
	if writerCount > 0 {
		return nil
	}
	if mode == ModeAPIExport && keepFiles {
		return nil
	}
	return fmt.Errorf("no writers configured: set ENABLE_POSTGRES / ENABLE_OTEL / ENABLE_PROMETHEUS_* (or KEEP_EXPORT_FILES in API-export mode)")
}

// storeChoice is the result of selectStore: which persistence backends to
// register, and (when sqlite is selected) the path to open it at.
type storeChoice struct {
	postgres   bool
	sqlite     bool
	sqlitePath string
}

// selectStore: SQLite is the default store (R2); Postgres is opt-in via
// ENABLE_POSTGRES. Both may run concurrently (fan-out). SQLite is disabled only
// when Postgres is the sole configured store AND no SQLITE_PATH override is set.
func selectStore(enablePostgres bool, sqlitePathEnv string) storeChoice {
	c := storeChoice{postgres: enablePostgres}
	if !enablePostgres || sqlitePathEnv != "" {
		c.sqlite = true
		c.sqlitePath = cmp.Or(sqlitePathEnv, "/data/tempest.db")
	}
	return c
}

func main() {
	ctx, done := signalContext(context.Background(), signal.NotifyContext)
	defer done()

	// Initialize sink for both modes
	metricsSink := sink.NewMetricsSink()

	// sqliteDB is set below (UDP mode only) when the sqlite store is
	// selected. It must be closed AFTER the sink drains the sqlite writer
	// (sink.Close flushes buffered writes) — hence it is closed inside the
	// same deferred cleanup, after metricsSink.Close returns, rather than via
	// its own defer (which LIFO ordering would run BEFORE the sink drains).
	var sqliteDB *sql.DB
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := metricsSink.Close(cleanupCtx); err != nil {
			slog.Error("sink close", "err", err)
		}
		if sqliteDB != nil {
			if err := sqliteDB.Close(); err != nil {
				slog.Error("sqlite db close", "err", err)
			}
		}
	}()

	// Configure Postgres opt-in and select the store(s) (R2: sqlite default,
	// postgres opt-in; see selectStore).
	enablePostgres, err := config.ParseBoolEnv("ENABLE_POSTGRES")
	if err != nil {
		log.Fatal(err) //nolint:gocritic // log.Fatal on a startup config error exits before any writer buffers data, so the skipped deferred sink Close is harmless
	}
	choice := selectStore(enablePostgres, os.Getenv("SQLITE_PATH"))

	// Configure Prometheus + SQLite writers (UDP mode only)
	token := os.Getenv("TOKEN")
	if token == "" {
		enablePushgateway, err := config.ParseBoolEnv("ENABLE_PROMETHEUS_PUSHGATEWAY")
		if err != nil {
			log.Fatal(err) //nolint:gocritic // log.Fatal on a startup config error exits before any writer buffers data, so the skipped deferred sink Close is harmless
		}
		if enablePushgateway {
			pushURL := os.Getenv("PROMETHEUS_PUSHGATEWAY_URL")
			if pushURL == "" {
				log.Fatal("PROMETHEUS_PUSHGATEWAY_URL is required when ENABLE_PROMETHEUS_PUSHGATEWAY is true") //nolint:gocritic // log.Fatal on a startup config error exits before any writer buffers data, so the skipped deferred sink Close is harmless
			}
			jobName := os.Getenv("JOB_NAME")
			if jobName == "" {
				jobName = "tempest"
			}
			promWriter := prometheus.NewPrometheusWriter(pushURL, jobName)
			metricsSink.AddWriter(promWriter)
		}

		// Configure Prometheus metrics server (scrape endpoint)
		enableMetrics, err := config.ParseBoolEnv("ENABLE_PROMETHEUS_METRICS")
		if err != nil {
			log.Fatal(err)
		}
		if enableMetrics {
			port := os.Getenv("PROMETHEUS_METRICS_PORT")
			if port == "" {
				port = "9000"
			}
			metricsServer := prometheus.NewMetricsServer(port)
			if err := metricsServer.Start(); err != nil {
				log.Fatalf("failed to start metrics server: %v", err)
			}
			metricsSink.AddWriter(metricsServer)
		}

		// Configure SQLite writer. UDP-mode only: SQLite.WriteMetrics is a
		// no-op (design §10 / operational-modes table routes API-export to
		// Postgres/gz, not sqlite), so registering it in API-export mode
		// would spuriously satisfy requireWriters while silently writing
		// nothing. selectStore itself stays mode-agnostic; only this
		// registration is UDP-gated.
		if choice.sqlite {
			sqliteCfg := sqlite.LoadConfig(os.Getenv)
			db, err := sqlite.Open(ctx, choice.sqlitePath, sqliteCfg)
			if err != nil {
				log.Fatalf("failed to open sqlite: %v", err)
			}
			sqliteDB = db
			metricsSink.AddWriter(sqlite.NewWriter(ctx, db, sqliteCfg))
		}
	}

	// Configure Postgres writer (both modes)
	if choice.postgres {
		dbConfig, err := config.GetDatabaseConfig()
		if err != nil {
			log.Fatalf("database configuration error: %v", err)
		}
		if dbConfig == "" {
			log.Fatal("POSTGRES_URL or POSTGRES_HOST is required when ENABLE_POSTGRES is true")
		}
		pgWriter, err := postgres.NewPostgresWriter(ctx, dbConfig)
		if err != nil {
			log.Fatalf("failed to initialize postgres: %v", err)
		}
		metricsSink.AddWriter(pgWriter)
	}

	// Require at least one writer (relaxed for gzip-only API-export mode; see requireWriters)
	mode := ModeUDP
	if token != "" {
		mode = ModeAPIExport
	}
	keepFiles, err := config.ParseBoolEnv("KEEP_EXPORT_FILES")
	if err != nil {
		log.Fatal(err)
	}
	if err := requireWriters(mode, metricsSink.WriterCount(), keepFiles); err != nil {
		log.Fatal(err)
	}

	// Choose operational mode
	if token != "" {
		exportWithSink(ctx, token, metricsSink)
	} else {
		listenAndPushWithSink(ctx, metricsSink)
	}
}

func listenAndPushWithSink(ctx context.Context, metricsSink *sink.MetricsSink) {
	logUDP, err := config.ParseBoolEnv("LOG_UDP")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("starting UDP listener mode")

	if err := listen(ctx, func(b []byte, addr *net.UDPAddr) error {
		if logUDP {
			log.Printf("UDP in: %s", string(b))
		}

		report, err := tempestudp.ParseReport(b)
		if err != nil {
			log.Printf("error parsing report from %s: %s", addr, err)
			return nil
		}

		// Send report to all configured writers
		if err := metricsSink.SendReport(ctx, report); err != nil {
			log.Printf("error sending report: %v", err)
		}

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}

// Old listenAndPush implementation removed - now using listenAndPushWithSink

func listen(ctx context.Context, rx func([]byte, *net.UDPAddr) error) error {
	sock, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   nil,
		Port: 50222,
	})
	if err != nil {
		return err
	}
	defer sock.Close() //nolint:errcheck // UDP listener teardown revisited under graceful shutdown in Task 0.8
	log.Printf("listening on UDP :50222")

	readErr := make(chan error, 1)

	// Start reading in the background
	go func() {
		buffer := make([]byte, 1500)
		for {
			n, addr, err := sock.ReadFromUDP(buffer)
			if err != nil {
				readErr <- err
				break
			}
			err = rx(buffer[:n], addr)
			if err != nil {
				readErr <- err
				break
			}
		}
		close(readErr)
	}()

	// Wait for reading to finish, or for our context to finish
	select {
	case err := <-readErr:
		return err

	case <-ctx.Done():
		return nil
	}
}

func exportWithSink(ctx context.Context, token string, metricsSink *sink.MetricsSink) {
	client := tempestapi.NewClient(token)
	stations, err := client.ListStations(ctx)
	if err != nil {
		log.Fatalf("error listing stations: %v", err)
	}

	if len(stations) == 0 {
		log.Fatalf("no stations found")
	}

	log.Printf("found stations:")
	var startAt time.Time
	for _, station := range stations {
		log.Printf("  - %s (station #%d)", station.Name, station.StationID)
		if startAt.IsZero() || startAt.Before(station.CreatedAt) {
			startAt = station.CreatedAt
		}
	}

	keepFiles, err := config.ParseBoolEnv("KEEP_EXPORT_FILES")
	if err != nil {
		log.Fatal(err)
	}
	fileNum := 1

	var next time.Time
	cur := startAt
	for {
		var metrics []promclient.Metric

		for ; cur.Before(time.Now()) && len(metrics) < 200_000; cur = next {
			next = cur.AddDate(0, 0, 1)

			for _, station := range stations {
				log.Printf("fetching %s starting %s", station.Name, cur.Format(time.RFC3339))
				stationMetrics, err := client.GetObservations(ctx, station, cur, next)
				if err != nil {
					log.Fatalf("error fetching %#v for %d-%d: %v", station, cur.Unix(), next.Unix(), err) //nolint:gosec // pre-existing: station/error data logged unsanitized; deferred as follow-up hardening, not owned by a current task
				}
				metrics = append(metrics, stationMetrics...)
			}
		}

		if len(metrics) == 0 {
			break
		}

		// Send to sink (Postgres)
		log.Printf("sending %d metrics to sink", len(metrics)) //nolint:gosec // pre-existing: logs a count derived from tainted input, not raw content; deferred as follow-up hardening, not owned by a current task
		if err := metricsSink.SendMetrics(ctx, metrics); err != nil {
			log.Printf("error sending metrics: %v", err)
		}

		// Optionally write to .gz files
		if keepFiles {
			filename := fmt.Sprintf("tempest_%03d.txt.gz", fileNum)
			if err := writeMetricsToFile(filename, metrics); err != nil {
				log.Fatalf("error writing file: %v", err)
			}
			fileNum++
		}
	}

	log.Printf("export complete")
}

func writeMetricsToFile(filename string, metrics []promclient.Metric) error {
	// Create collector for metrics
	collector := &staticCollector{metrics: metrics}

	r := promclient.NewRegistry()
	r.MustRegister(collector)
	families, err := r.Gather()
	if err != nil {
		return fmt.Errorf("gather metrics: %w", err)
	}

	log.Printf("writing %s", filename)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644) //nolint:gosec // G302/G304: 0o644 perms and export filename are intentional, not user-controlled in a way that risks traversal
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close() //nolint:errcheck // Close handling for export writers revisited in Task 0.11

	gzw := gzip.NewWriter(f)
	defer gzw.Close() //nolint:errcheck // Close handling for export writers revisited in Task 0.11

	enc := expfmt.NewEncoder(gzw, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, family := range families {
		if err := enc.Encode(family); err != nil {
			return fmt.Errorf("encode metrics: %w", err)
		}
	}

	if c, ok := enc.(io.Closer); ok {
		if err := c.Close(); err != nil {
			return fmt.Errorf("close encoder: %w", err)
		}
	}

	return nil
}

// staticCollector holds a static list of metrics
type staticCollector struct {
	metrics []promclient.Metric
}

func (c *staticCollector) Describe(descs chan<- *promclient.Desc) {
	// Not needed
}

func (c *staticCollector) Collect(metrics chan<- promclient.Metric) {
	for _, m := range c.metrics {
		metrics <- m
	}
}
