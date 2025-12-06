package postgres

import (
	"context"
	"testing"
	"time"

	"tempestwx-utilities/internal/tempestudp"
)

func TestNewPostgresWriter_InvalidURL(t *testing.T) {
	ctx := context.Background()

	_, err := NewPostgresWriter(ctx, "not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestNewPostgresWriter_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// TODO: implement with testcontainers
	t.Skip("TODO: implement with real Postgres container")
}

func TestPostgresWriter_WriteReport_Observation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// TODO: implement with testcontainers
	t.Skip("TODO: implement with real Postgres")
}

func TestPostgresWriter_FlushObservations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// TODO: implement with testcontainers
	t.Skip("TODO: implement with real Postgres")
}

// Unit test for routing logic
func TestPostgresWriter_RouteObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Mock writer without real DB
	w := &PostgresWriter{
		obsBatch:   make(chan observationRow, 10),
		windBatch:  make(chan rapidWindRow, 10),
		hubBatch:   make(chan hubStatusRow, 10),
		eventBatch: make(chan eventRow, 10),
		ctx:        ctx,
	}

	report := &tempestudp.TempestObservationReport{
		SerialNumber: "ST-00001",
		Obs: [][]float64{
			{
				1234567890, // timestamp
				1.5,        // wind lull
				2.0,        // wind avg
				2.5,        // wind gust
				180,        // wind direction
				0,          // wind sample interval
				1013.25,    // pressure (MB)
				20.5,       // temp air (C)
				75.0,       // humidity (%)
				50000,      // illuminance
				3,          // UV
				500,        // irradiance
				0.5,        // rain rate
				0,          // precip type
				0,          // lightning distance
				0,          // lightning count
				3.5,        // battery
				1,          // report interval (minutes)
			},
		},
		FirmwareRevision: 143,
	}

	err := w.WriteReport(ctx, report)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have queued one observation
	select {
	case row := <-w.obsBatch:
		if row.serialNumber != "ST-00001" {
			t.Errorf("wrong serial: got %q", row.serialNumber)
		}
		if row.windAvg != 2.0 {
			t.Errorf("wrong wind avg: got %f", row.windAvg)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for observation")
	}
}
