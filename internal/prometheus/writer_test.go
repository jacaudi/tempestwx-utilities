package prometheus

import (
	"context"
	"testing"
	"time"

	"tempestwx-utilities/internal/tempestudp"
)

func TestPrometheusWriter_WriteReport(t *testing.T) {
	ctx := context.Background()

	// Mock push gateway - use httptest
	// For now, just test construction
	writer := NewPrometheusWriter("http://localhost:9091", "test-job")

	report := &tempestudp.TempestObservationReport{
		SerialNumber: "ST-00001",
		Obs: [][]float64{
			{1234567890, 1.5, 2.0, 2.5, 180, 0, 1013.25, 20.5, 75, 50000, 3, 500, 0.5},
		},
	}

	err := writer.WriteReport(ctx, report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that metrics were queued
	// This is a basic test - full integration test would verify push gateway
	time.Sleep(100 * time.Millisecond)
}

func TestPrometheusWriter_Close(t *testing.T) {
	writer := NewPrometheusWriter("http://localhost:9091", "test-job")

	err := writer.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
