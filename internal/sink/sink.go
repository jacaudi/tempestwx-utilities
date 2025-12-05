package sink

import (
	"context"
	"log"
	"sync"

	"tempestwx-exporter/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsWriter is the interface that all metric backends must implement.
type MetricsWriter interface {
	// WriteReport writes a parsed Tempest report (UDP mode - typed structs)
	WriteReport(ctx context.Context, report tempestudp.Report) error

	// WriteMetrics writes Prometheus metrics (API export mode)
	WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error

	// Flush ensures any buffered data is written
	Flush(ctx context.Context) error

	// Close performs cleanup
	Close() error
}

// MetricsSink coordinates sending metrics to multiple backends.
type MetricsSink struct {
	writers []MetricsWriter
	ctx     context.Context
	mu      sync.RWMutex
}

// NewMetricsSink creates a new metrics sink.
func NewMetricsSink(ctx context.Context) *MetricsSink {
	return &MetricsSink{
		writers: make([]MetricsWriter, 0),
		ctx:     ctx,
	}
}

// AddWriter registers a new metrics writer.
func (s *MetricsSink) AddWriter(writer MetricsWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writers = append(s.writers, writer)
}

// WriterCount returns the number of registered writers.
func (s *MetricsSink) WriterCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.writers)
}

// SendReport sends a typed report to all writers.
func (s *MetricsSink) SendReport(ctx context.Context, report tempestudp.Report) error {
	s.mu.RLock()
	writers := make([]MetricsWriter, len(s.writers))
	copy(writers, s.writers)
	s.mu.RUnlock()

	// Fan out to all writers concurrently
	var wg sync.WaitGroup
	for _, w := range writers {
		wg.Add(1)
		go func(writer MetricsWriter) {
			defer wg.Done()
			if err := writer.WriteReport(ctx, report); err != nil {
				log.Printf("writer error (report): %v", err)
			}
		}(w)
	}
	wg.Wait()

	return nil
}

// SendMetrics sends Prometheus metrics to all writers.
func (s *MetricsSink) SendMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	s.mu.RLock()
	writers := make([]MetricsWriter, len(s.writers))
	copy(writers, s.writers)
	s.mu.RUnlock()

	// Fan out to all writers concurrently
	var wg sync.WaitGroup
	for _, w := range writers {
		wg.Add(1)
		go func(writer MetricsWriter) {
			defer wg.Done()
			if err := writer.WriteMetrics(ctx, metrics); err != nil {
				log.Printf("writer error (metrics): %v", err)
			}
		}(w)
	}
	wg.Wait()

	return nil
}

// Close flushes and closes all writers.
func (s *MetricsSink) Close() error {
	s.mu.RLock()
	writers := make([]MetricsWriter, len(s.writers))
	copy(writers, s.writers)
	s.mu.RUnlock()

	var wg sync.WaitGroup
	for _, w := range writers {
		wg.Add(1)
		go func(writer MetricsWriter) {
			defer wg.Done()

			// Flush first
			if err := writer.Flush(s.ctx); err != nil {
				log.Printf("writer flush error: %v", err)
			}

			// Then close
			if err := writer.Close(); err != nil {
				log.Printf("writer close error: %v", err)
			}
		}(w)
	}
	wg.Wait()

	return nil
}
