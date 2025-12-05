package prometheus

import (
	"context"
	"log"
	"sync"

	"tempestwx-exporter/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/prometheus/common/expfmt"
)

// PrometheusWriter wraps the existing Prometheus push logic.
type PrometheusWriter struct {
	pusher *push.Pusher
	outbox chan prometheus.Metric
	more   chan bool
	wg     sync.WaitGroup
}

// NewPrometheusWriter creates a new Prometheus writer.
func NewPrometheusWriter(pushURL, jobName string) *PrometheusWriter {
	outbox := make(chan prometheus.Metric, 1000)
	more := make(chan bool, 1)

	// Create collector that drains outbox
	collector := &outboxCollector{outbox: outbox}

	// Create pusher
	pusher := push.New(pushURL, jobName).
		Collector(collector).
		Format(expfmt.FmtText)

	w := &PrometheusWriter{
		pusher: pusher,
		outbox: outbox,
		more:   more,
	}

	// Start background push worker
	w.wg.Add(1)
	go w.pushWorker()

	log.Printf("prometheus: configured push to %q with job %q", pushURL, jobName)

	return w
}

// WriteReport converts report to metrics and queues for pushing.
func (w *PrometheusWriter) WriteReport(ctx context.Context, report tempestudp.Report) error {
	metrics := report.Metrics()
	return w.WriteMetrics(ctx, metrics)
}

// WriteMetrics queues metrics for pushing.
func (w *PrometheusWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	for _, m := range metrics {
		select {
		case w.outbox <- m:
		case <-ctx.Done():
			return ctx.Err()
		default:
			log.Printf("prometheus: outbox full, dropping metric")
		}
	}

	// Signal push worker
	select {
	case w.more <- true:
	default:
		// Already signaled
	}

	return nil
}

// Flush is a no-op for Prometheus (push happens in background).
func (w *PrometheusWriter) Flush(ctx context.Context) error {
	// Trigger immediate push
	select {
	case w.more <- true:
	default:
	}
	return nil
}

// Close stops the push worker.
func (w *PrometheusWriter) Close() error {
	close(w.outbox)
	close(w.more)
	w.wg.Wait() // Wait for pushWorker to finish
	log.Printf("prometheus: closed")
	return nil
}

func (w *PrometheusWriter) pushWorker() {
	defer w.wg.Done()
	for range w.more {
		if err := w.pusher.Add(); err != nil {
			log.Printf("prometheus: push error: %v", err)
		}
	}
}

// outboxCollector drains the outbox channel non-blockingly.
type outboxCollector struct {
	outbox <-chan prometheus.Metric
}

func (c *outboxCollector) Describe(descs chan<- *prometheus.Desc) {
	// Don't need to describe - metrics are already created
}

func (c *outboxCollector) Collect(metrics chan<- prometheus.Metric) {
	for {
		select {
		case m := <-c.outbox:
			metrics <- m
		default:
			return
		}
	}
}
