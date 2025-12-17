package main

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	"tempestwx-utilities/internal/config"
	"tempestwx-utilities/internal/prometheus"
	"tempestwx-utilities/internal/postgres"
	"tempestwx-utilities/internal/sink"
	"tempestwx-utilities/internal/tempestapi"
	"tempestwx-utilities/internal/tempestudp"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// Old collector implementation removed - now using MetricsSink

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt)
	defer done()

	// Initialize sink for both modes
	metricsSink := sink.NewMetricsSink(ctx)
	defer metricsSink.Close()

	// Configure Prometheus writer (UDP mode only)
	token := os.Getenv("TOKEN")
	if token == "" {
		pushURL := os.Getenv("PUSH_URL")
		if pushURL != "" {
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
			metricsAddr := ":" + port
			metricsServer := prometheus.NewMetricsServer(metricsAddr)
			if err := metricsServer.Start(); err != nil {
				log.Fatalf("failed to start metrics server: %v", err)
			}
			metricsSink.AddWriter(metricsServer)
		}
	}

	// Configure Postgres writer (both modes)
	dbConfig, err := config.GetDatabaseConfig()
	if err != nil {
		log.Fatalf("database configuration error: %v", err)
	}
	if dbConfig != "" {
		pgWriter, err := postgres.NewPostgresWriter(ctx, dbConfig)
		if err != nil {
			log.Fatalf("failed to initialize postgres: %v", err)
		}
		metricsSink.AddWriter(pgWriter)
	}

	// Require at least one writer
	if metricsSink.WriterCount() == 0 {
		log.Fatal("no writers configured - set PUSH_URL, ENABLE_PROMETHEUS_METRICS, and/or DATABASE_HOST/DATABASE_URL")
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
	defer sock.Close()
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

	keepFiles, _ := strconv.ParseBool(os.Getenv("KEEP_EXPORT_FILES"))
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
					log.Fatalf("error fetching %#v for %d-%d: %v", station, cur.Unix(), next.Unix(), err)
				}
				metrics = append(metrics, stationMetrics...)
			}
		}

		if len(metrics) == 0 {
			break
		}

		// Send to sink (Postgres)
		log.Printf("sending %d metrics to sink", len(metrics))
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
