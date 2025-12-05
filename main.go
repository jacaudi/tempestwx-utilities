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

	"tempestwx-exporter/internal/config"
	"tempestwx-exporter/internal/prometheus"
	"tempestwx-exporter/internal/postgres"
	"tempestwx-exporter/internal/sink"
	"tempestwx-exporter/internal/tempest"
	"tempestwx-exporter/internal/tempestapi"
	"tempestwx-exporter/internal/tempestudp"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/prometheus/common/expfmt"
)

type collector struct {
	outbox <-chan promclient.Metric
}

func (c collector) Describe(descs chan<- *promclient.Desc) {
	for _, desc := range tempest.All {
		descs <- desc
	}
}

func (c collector) Collect(metrics chan<- promclient.Metric) {
	for {
		select {
		case m := <-c.outbox:
			metrics <- m
		default:
			return
		}
	}
}

func main() {
	ctx, done := signal.NotifyContext(context.Background(), os.Interrupt)
	defer done()

	// Check operational mode
	token := os.Getenv("TOKEN")
	if token != "" {
		// API export mode - keep existing implementation for now
		export(ctx, token)
		return
	}

	// UDP listener mode with sink
	metricsSink := sink.NewMetricsSink(ctx)
	defer metricsSink.Close()

	// Configure Prometheus writer
	pushURL := os.Getenv("PUSH_URL")
	if pushURL != "" {
		jobName := os.Getenv("JOB_NAME")
		if jobName == "" {
			jobName = "tempest"
		}
		promWriter := prometheus.NewPrometheusWriter(pushURL, jobName)
		metricsSink.AddWriter(promWriter)
	}

	// Configure Postgres writer
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
		log.Fatal("no writers configured - set PUSH_URL and/or DATABASE_HOST/DATABASE_URL")
	}

	// Start UDP listener
	listenAndPushWithSink(ctx, metricsSink)
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

func listenAndPush(ctx context.Context) {
	pushUrl := os.Getenv("PUSH_URL")
	if pushUrl == "" {
		log.Fatal("PUSH_URL must be specified")
	}
	jobName := os.Getenv("JOB_NAME")
	if jobName == "" {
		jobName = "tempest"
	}
	logUDP, _ := strconv.ParseBool(os.Getenv("LOG_UDP"))
	log.Printf("pushing to %q with job name %q", pushUrl, jobName)

	more := make(chan bool, 1)
	outbox := make(chan promclient.Metric, 1000)
	go func() {
		c := &collector{outbox}
		pusher := push.New(pushUrl, jobName).Collector(c).Format(expfmt.FmtText)

		for {
			select {
			case <-ctx.Done():
				return
			case <-more:
				if err := pusher.Add(); err != nil {
					log.Printf("error pushing: %v", err)
				}
			}
		}
	}()

	if err := listen(ctx, func(b []byte, addr *net.UDPAddr) error {
		if logUDP {
			log.Printf("UDP in: %s", string(b))
		}
		report, err := tempestudp.ParseReport(b)
		if err != nil {
			log.Printf("error parsing report from %s: %s", addr, err)
		} else {
			for _, m := range report.Metrics() {
				outbox <- m
			}

			select {
			case more <- true:
				// success
			default:
				// already busy sending
			}
		}

		return nil
	}); err != nil {
		log.Fatal(err)
	}
}

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

func export(ctx context.Context, token string) {
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

	n := 1

	var next time.Time
	cur := startAt
	for {
		var c dumpCollector

		for ; cur.Before(time.Now()) && len(c.metrics) < 200_000; cur = next {
			next = cur.AddDate(0, 0, 1) // for 1-minute observation frequency

			for _, station := range stations {
				log.Printf("fetching %s starting %s", station.Name, cur.Format(time.RFC3339))
				metrics, err := client.GetObservations(ctx, station, cur, next)
				if err != nil {
					log.Fatalf("error fetching %#v for %d-%d: %v", station, cur.Unix(), next.Unix(), err)
				}
				c.metrics = append(c.metrics, metrics...)
			}
		}

		if len(c.metrics) == 0 {
			break
		}

		r := promclient.NewRegistry()
		r.MustRegister(&c)
		families, err := r.Gather()
		if err != nil {
			log.Fatalf("error gathering metrics: %v", err)
		}

		filename := fmt.Sprintf("tempest_%03d.txt.gz", n)
		log.Printf("writing %s", filename)
		f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("error opening output file: %v", err)
		}
		gzw := gzip.NewWriter(f)
		enc := expfmt.NewEncoder(gzw, expfmt.FmtText)
		for _, family := range families {
			if err := enc.Encode(family); err != nil {
				log.Fatalf("error encoding metrics: %v", err)
			}
		}
		if c, ok := enc.(io.Closer); ok {
			err = c.Close()
			if err != nil {
				log.Fatalf("error closing metric encoder: %v", err)
			}
		}
		if err := gzw.Close(); err != nil {
			log.Fatalf("error closing gzip writer: %v", err)
		}
		if err := f.Close(); err != nil {
			log.Fatalf("error closing output file: %v", err)
		}

		n = n + 1
	}
}

type dumpCollector struct {
	metrics []promclient.Metric
}

func (d dumpCollector) Describe(descs chan<- *promclient.Desc) {
}

func (d dumpCollector) Collect(metrics chan<- promclient.Metric) {
	for _, metric := range d.metrics {
		metrics <- metric
	}
}
