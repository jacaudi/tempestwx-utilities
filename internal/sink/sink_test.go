package sink

import (
	"context"
	"errors"
	"sync"
	"testing"

	"tempestwx-utilities/internal/tempestudp"

	"github.com/prometheus/client_golang/prometheus"
)

// Mock writer for testing
type mockWriter struct {
	reportCalls  int
	metricCalls  int
	reportErr    error
	metricsErr   error
	flushCalled  bool
	closeCalled  bool
	mu           sync.Mutex
}

func (m *mockWriter) WriteReport(ctx context.Context, report tempestudp.Report) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reportCalls++
	return m.reportErr
}

func (m *mockWriter) WriteMetrics(ctx context.Context, metrics []prometheus.Metric) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metricCalls++
	return m.metricsErr
}

func (m *mockWriter) Flush(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushCalled = true
	return nil
}

func (m *mockWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return nil
}

func TestMetricsSink_AddWriter(t *testing.T) {
	ctx := context.Background()
	sink := NewMetricsSink(ctx)

	if sink.WriterCount() != 0 {
		t.Errorf("expected 0 writers, got %d", sink.WriterCount())
	}

	writer := &mockWriter{}
	sink.AddWriter(writer)

	if sink.WriterCount() != 1 {
		t.Errorf("expected 1 writer, got %d", sink.WriterCount())
	}
}

func TestMetricsSink_SendReport(t *testing.T) {
	ctx := context.Background()
	sink := NewMetricsSink(ctx)

	writer1 := &mockWriter{}
	writer2 := &mockWriter{}
	sink.AddWriter(writer1)
	sink.AddWriter(writer2)

	// Create a simple report
	report := &tempestudp.TempestObservationReport{
		SerialNumber: "TEST-001",
		Obs:          [][]float64{{1234567890, 1.5, 2.0, 2.5}},
	}

	err := sink.SendReport(ctx, report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both writers should be called
	if writer1.reportCalls != 1 {
		t.Errorf("writer1: expected 1 call, got %d", writer1.reportCalls)
	}
	if writer2.reportCalls != 1 {
		t.Errorf("writer2: expected 1 call, got %d", writer2.reportCalls)
	}
}

func TestMetricsSink_SendMetrics(t *testing.T) {
	ctx := context.Background()
	sink := NewMetricsSink(ctx)

	writer := &mockWriter{}
	sink.AddWriter(writer)

	metrics := []prometheus.Metric{} // Empty for now
	err := sink.SendMetrics(ctx, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if writer.metricCalls != 1 {
		t.Errorf("expected 1 call, got %d", writer.metricCalls)
	}
}

func TestMetricsSink_WriterError(t *testing.T) {
	ctx := context.Background()
	sink := NewMetricsSink(ctx)

	// Writer that returns error
	failWriter := &mockWriter{reportErr: errors.New("write failed")}
	goodWriter := &mockWriter{}

	sink.AddWriter(failWriter)
	sink.AddWriter(goodWriter)

	report := &tempestudp.TempestObservationReport{
		SerialNumber: "TEST-001",
	}

	// Should not return error - errors are logged but don't fail the send
	err := sink.SendReport(ctx, report)
	if err != nil {
		t.Errorf("expected no error when writer fails, got %v", err)
	}

	// Both writers should have been called
	if failWriter.reportCalls != 1 {
		t.Errorf("failWriter should have been called")
	}
	if goodWriter.reportCalls != 1 {
		t.Errorf("goodWriter should have been called")
	}
}

func TestMetricsSink_Close(t *testing.T) {
	ctx := context.Background()
	sink := NewMetricsSink(ctx)

	writer1 := &mockWriter{}
	writer2 := &mockWriter{}
	sink.AddWriter(writer1)
	sink.AddWriter(writer2)

	err := sink.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both writers should have flush and close called
	if !writer1.flushCalled {
		t.Error("writer1.Flush should have been called")
	}
	if !writer1.closeCalled {
		t.Error("writer1.Close should have been called")
	}
	if !writer2.flushCalled {
		t.Error("writer2.Flush should have been called")
	}
	if !writer2.closeCalled {
		t.Error("writer2.Close should have been called")
	}
}
