package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"tempestwx-utilities/internal/otel"

	sdklog "go.opentelemetry.io/otel/sdk/log"
)

func TestSignalContext_RegistersInterruptAndSIGTERM(t *testing.T) {
	var gotSigs []os.Signal
	fakeNotify := func(parent context.Context, sig ...os.Signal) (context.Context, context.CancelFunc) {
		gotSigs = sig
		return context.WithCancel(parent)
	}

	_, cancel := signalContext(context.Background(), fakeNotify)
	defer cancel()

	// os.Signal(syscall.SIGTERM): slices.Contains infers E from both arguments;
	// without this explicit conversion to the slice's element type, Go's generic
	// type inference fails to unify os.Signal (interface, from gotSigs) with
	// syscall.Signal (concrete, from syscall.SIGTERM) and the call doesn't compile.
	if !slices.Contains(gotSigs, os.Interrupt) || !slices.Contains(gotSigs, os.Signal(syscall.SIGTERM)) {
		t.Fatalf("signalContext must register SIGINT+SIGTERM, got %v", gotSigs)
	}
}

func TestSelectStore(t *testing.T) {
	tests := []struct {
		name         string
		enablePG     bool
		sqlitePath   string
		wantPostgres bool
		wantSQLite   bool
		wantPath     string
	}{
		{"default sqlite", false, "", false, true, "/data/tempest.db"},
		{"postgres only", true, "", true, false, ""},
		{"both fan-out", true, "/tmp/x.db", true, true, "/tmp/x.db"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := selectStore(tc.enablePG, tc.sqlitePath)
			if c.postgres != tc.wantPostgres || c.sqlite != tc.wantSQLite {
				t.Fatalf("got %+v", c)
			}
			if tc.wantSQLite && c.sqlitePath != tc.wantPath {
				t.Fatalf("path %q want %q", c.sqlitePath, tc.wantPath)
			}
		})
	}
}

// capturingExporter is a real sdklog.Exporter (not a mock) that appends every
// exported Record to a slice, guarded by a mutex per Export's documented
// concurrency contract.
type capturingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *capturingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.records = append(e.records, records...)
	return nil
}

func (e *capturingExporter) Shutdown(context.Context) error   { return nil }
func (e *capturingExporter) ForceFlush(context.Context) error { return nil }

func (e *capturingExporter) captured() []sdklog.Record {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.records
}

// TestTeeHandler_FansOutToAllHandlers asserts newTeeHandler's fix for a real
// stdlib behavior: slog.SetDefault(l) calls log.SetOutput(&handlerWriter{l.Handler(),...})
// whenever l.Handler() isn't the unexported *defaultHandler type (see
// log/slog/logger.go's SetDefault) — so wiring ENABLE_OTEL's log bridge as the
// sole slog default would silently redirect ALL of main's existing
// log.Printf/log.Fatal output away from stderr and into the OTel pipeline
// only. newTeeHandler fans every record out to both the OTel bridge and a
// plain stderr handler, so container log visibility is preserved.
func TestTeeHandler_FansOutToAllHandlers(t *testing.T) {
	exporter := &capturingExporter{}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	t.Cleanup(func() { _ = lp.Shutdown(t.Context()) })

	var stderrBuf bytes.Buffer
	stderrHandler := slog.NewTextHandler(&stderrBuf, nil)
	otelHandler := otel.NewSlogHandler(lp)

	logger := slog.New(newTeeHandler(otelHandler, stderrHandler))
	logger.InfoContext(t.Context(), "tee test message", "key", "val")

	if !strings.Contains(stderrBuf.String(), "tee test message") {
		t.Errorf("stderr buffer = %q, want it to contain the log message (visibility must be preserved)", stderrBuf.String())
	}

	records := exporter.captured()
	if len(records) != 1 {
		t.Fatalf("got %d records captured by OTel exporter, want 1: %+v", len(records), records)
	}
	if got := records[0].Body().AsString(); got != "tee test message" {
		t.Errorf("OTel record Body() = %q, want %q", got, "tee test message")
	}
}

func TestRequireWriters(t *testing.T) {
	tests := []struct {
		name        string
		mode        Mode
		writerCount int
		keepFiles   bool
		wantErr     bool
	}{
		{"udp no writers", ModeUDP, 0, false, true},
		{"udp one writer", ModeUDP, 1, false, false},
		{"api no writers no files", ModeAPIExport, 0, false, true},
		{"api no writers keep files", ModeAPIExport, 0, true, false},
		{"api db writer", ModeAPIExport, 1, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := requireWriters(tc.mode, tc.writerCount, tc.keepFiles)
			if (err != nil) != tc.wantErr {
				t.Fatalf("requireWriters(%v,%d,%v) err=%v want err=%v", tc.mode, tc.writerCount, tc.keepFiles, err, tc.wantErr)
			}
		})
	}
}
