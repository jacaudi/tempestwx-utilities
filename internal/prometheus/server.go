package prometheus

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"tempestwx-utilities/internal/tempest"
	"tempestwx-utilities/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// MetricsServer exposes a /metrics endpoint for Prometheus scraping.
// It implements the sink.MetricsWriter interface.
type MetricsServer struct {
	addr       string
	server     *http.Server
	collector  *latestMetricsCollector
	registry   *prometheus.Registry
	shutdownWg sync.WaitGroup
}

// NewMetricsServer creates a new metrics server listening on the given address.
func NewMetricsServer(addr string) *MetricsServer {
	collector := &latestMetricsCollector{
		metrics: make(map[string]prometheus.Metric),
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return &MetricsServer{
		addr:      addr,
		server:    server,
		collector: collector,
		registry:  registry,
	}
}

// Start begins serving the metrics endpoint in a background goroutine.
func (s *MetricsServer) Start() error {
	s.shutdownWg.Add(1)
	go func() {
		defer s.shutdownWg.Done()
		log.Printf("metrics: starting HTTP server on %s", s.addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics: server error: %v", err)
		}
	}()
	return nil
}

// WriteReport converts a UDP report to metrics and stores them.
func (s *MetricsServer) WriteReport(ctx context.Context, report tempestudp.Report) error {
	metrics := report.Metrics()
	return s.WriteMetrics(ctx, metrics)
}

// WriteMetrics stores the latest metrics for scraping.
func (s *MetricsServer) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	s.collector.Update(metrics)
	return nil
}

// Flush is a no-op for the metrics server.
func (s *MetricsServer) Flush(ctx context.Context) error {
	return nil
}

// Close shuts down the HTTP server gracefully.
func (s *MetricsServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		log.Printf("metrics: shutdown error: %v", err)
		return err
	}

	s.shutdownWg.Wait()
	log.Printf("metrics: server closed")
	return nil
}

// latestMetricsCollector stores the latest value for each unique metric.
// This allows Prometheus to scrape the current values at any time.
type latestMetricsCollector struct {
	mu      sync.RWMutex
	metrics map[string]prometheus.Metric
}

// Update stores or replaces metrics with the same identity.
func (c *latestMetricsCollector) Update(metrics []prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, m := range metrics {
		key := metricKey(m)
		c.metrics[key] = m
	}
}

// Describe sends all metric descriptors to the channel.
func (c *latestMetricsCollector) Describe(descs chan<- *prometheus.Desc) {
	// Send all known metric descriptors
	for _, desc := range tempest.All {
		descs <- desc
	}
}

// Collect sends all stored metrics to the channel.
func (c *latestMetricsCollector) Collect(metrics chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, m := range c.metrics {
		metrics <- m
	}
}

// metricKey generates a unique key for a metric based on its description and labels.
func metricKey(m prometheus.Metric) string {
	var d dto.Metric
	if err := m.Write(&d); err != nil {
		// Fallback to description string if write fails
		return m.Desc().String()
	}

	// Build key from description and label values
	key := m.Desc().String()
	for _, label := range d.Label {
		key += fmt.Sprintf(",%s=%s", label.GetName(), label.GetValue())
	}
	return key
}
