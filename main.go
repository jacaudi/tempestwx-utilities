package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"tempestwx-utilities/internal/config"
	"tempestwx-utilities/internal/logging"
	"tempestwx-utilities/internal/postgres"
	"tempestwx-utilities/internal/prometheus"
	"tempestwx-utilities/internal/sink"
	"tempestwx-utilities/internal/tempestapi"
	"tempestwx-utilities/internal/tempestudp"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// Old collector implementation removed - now using MetricsSink

// fatal logs msg at error level with the given structured attributes and exits
// the process. It replaces the previous log.Fatal/log.Fatalf usage.
func fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func main() {
	logging.Init()

	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt)
	defer done()

	// Initialize sink for both modes
	metricsSink := sink.NewMetricsSink(ctx)
	defer metricsSink.Close()

	// Configure Prometheus writer (UDP mode only)
	token := os.Getenv("TOKEN")
	if token == "" {
		enablePushgateway, _ := strconv.ParseBool(os.Getenv("ENABLE_PROMETHEUS_PUSHGATEWAY"))
		if enablePushgateway {
			pushURL := os.Getenv("PROMETHEUS_PUSHGATEWAY_URL")
			if pushURL == "" {
				fatal("PROMETHEUS_PUSHGATEWAY_URL is required when ENABLE_PROMETHEUS_PUSHGATEWAY is true")
			}
			jobName := os.Getenv("JOB_NAME")
			if jobName == "" {
				jobName = "tempest"
			}
			promWriter := prometheus.NewPrometheusWriter(pushURL, jobName)
			metricsSink.AddWriter(promWriter)
		}

		// Configure Prometheus metrics server (scrape endpoint)
		enableMetrics, _ := strconv.ParseBool(os.Getenv("ENABLE_PROMETHEUS_METRICS"))
		if enableMetrics {
			port := os.Getenv("PROMETHEUS_METRICS_PORT")
			if port == "" {
				port = "9000"
			}
			metricsServer := prometheus.NewMetricsServer(port)
			if err := metricsServer.Start(); err != nil {
				fatal("failed to start metrics server", "err", err)
			}
			metricsSink.AddWriter(metricsServer)
		}
	}

	// Configure Postgres writer (both modes)
	enablePostgres, _ := strconv.ParseBool(os.Getenv("ENABLE_POSTGRES"))
	if enablePostgres {
		dbConfig, err := config.GetDatabaseConfig()
		if err != nil {
			fatal("database configuration error", "err", err)
		}
		if dbConfig == "" {
			fatal("POSTGRES_URL or POSTGRES_HOST is required when ENABLE_POSTGRES is true")
		}
		pgWriter, err := postgres.NewPostgresWriter(ctx, dbConfig)
		if err != nil {
			fatal("failed to initialize postgres", "err", err)
		}
		metricsSink.AddWriter(pgWriter)
	}

	// Require at least one writer
	if metricsSink.WriterCount() == 0 {
		fatal("no writers configured - set ENABLE_PROMETHEUS_PUSHGATEWAY, ENABLE_PROMETHEUS_METRICS, and/or ENABLE_POSTGRES")
	}

	// Choose operational mode
	if token != "" {
		exportWithSink(ctx, token, metricsSink)
	} else {
		listenAndPushWithSink(ctx, metricsSink)
	}
}

func listenAndPushWithSink(ctx context.Context, metricsSink *sink.MetricsSink) {
	logUDP, _ := strconv.ParseBool(os.Getenv("LOG_UDP"))
	slog.Info("starting UDP listener mode")

	if err := listen(ctx, func(b []byte, addr *net.UDPAddr) error {
		if logUDP {
			slog.Info("received UDP packet", "payload", string(b))
		}

		report, err := tempestudp.ParseReport(b)
		if err != nil {
			slog.Warn("error parsing report", "addr", addr, "err", err)
			return nil
		}

		// Send report to all configured writers
		if err := metricsSink.SendReport(ctx, report); err != nil {
			slog.Error("error sending report", "err", err)
		}

		return nil
	}); err != nil {
		fatal("UDP listener error", "err", err)
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
	defer sock.Close()
	slog.Info("listening on UDP", "port", 50222)

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
		fatal("error listing stations", "err", err)
	}

	if len(stations) == 0 {
		fatal("no stations found")
	}

	slog.Info("found stations", "count", len(stations))
	var startAt time.Time
	for _, station := range stations {
		slog.Info("station", "name", station.Name, "station_id", station.StationID)
		if startAt.IsZero() || startAt.Before(station.CreatedAt) {
			startAt = station.CreatedAt
		}
	}

	keepFiles, _ := strconv.ParseBool(os.Getenv("KEEP_EXPORT_FILES"))
	fileNum := 1

	var next time.Time
	cur := startAt
	for {
		var metrics []promclient.Metric

		for ; cur.Before(time.Now()) && len(metrics) < 200_000; cur = next {
			next = cur.AddDate(0, 0, 1)

			for _, station := range stations {
				slog.Info("fetching observations", "station", station.Name, "start", cur.Format(time.RFC3339))
				stationMetrics, err := client.GetObservations(ctx, station, cur, next)
				if err != nil {
					fatal("error fetching observations", "station", station.Name, "from", cur.Unix(), "to", next.Unix(), "err", err)
				}
				metrics = append(metrics, stationMetrics...)
			}
		}

		if len(metrics) == 0 {
			break
		}

		// Send to sink (Postgres)
		slog.Info("sending metrics to sink", "count", len(metrics))
		if err := metricsSink.SendMetrics(ctx, metrics); err != nil {
			slog.Error("error sending metrics", "err", err)
		}

		// Optionally write to .gz files
		if keepFiles {
			filename := fmt.Sprintf("tempest_%03d.txt.gz", fileNum)
			if err := writeMetricsToFile(filename, metrics); err != nil {
				fatal("error writing file", "err", err)
			}
			fileNum++
		}
	}

	slog.Info("export complete")
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

	slog.Info("writing metrics file", "filename", filename)
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	gzw := gzip.NewWriter(f)
	defer gzw.Close()

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
