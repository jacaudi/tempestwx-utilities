package main

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strconv"
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

// TestRunHealthcheck_HealthyServer asserts runHealthcheck's success path: a
// real listening server answering 200 on /healthz yields exit code 0. Uses
// httptest.NewServer (a real net.Listener + http.Server) rather than a mock,
// per the docker HEALTHCHECK contract: the binary is exec'd as
// `tempestwx-utilities healthcheck` inside the same container as the running
// server, so it must actually dial the loopback address.
func TestRunHealthcheck_HealthyServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse httptest URL: %v", err)
	}
	// runHealthcheck builds "http://127.0.0.1" + HTTP_ADDR + "/healthz", so
	// HTTP_ADDR must be in the ":port" shape it uses in production (srv.Addr
	// = cmp.Or(os.Getenv("HTTP_ADDR"), ":8080") in main), not "host:port".
	t.Setenv("HTTP_ADDR", ":"+u.Port())

	if got := runHealthcheck(); got != 0 {
		t.Fatalf("runHealthcheck() = %d, want 0 for a healthy /healthz", got)
	}
}

// TestRunHealthcheck_HostPortShape asserts runHealthcheck works when
// HTTP_ADDR is set in "host:port" shape (e.g. "0.0.0.0:8080" or, as here,
// "127.0.0.1:<port>"), not just the ":port" shape used elsewhere in this
// file. runHealthcheck must always probe 127.0.0.1 regardless of the host
// component in HTTP_ADDR, since the healthcheck runs inside the same
// container as the server it's probing.
func TestRunHealthcheck_HostPortShape(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse httptest URL: %v", err)
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:"+u.Port())

	if got := runHealthcheck(); got != 0 {
		t.Fatalf("runHealthcheck() = %d, want 0 for a healthy /healthz with host:port HTTP_ADDR", got)
	}
}

// TestRunHealthcheck_Unreachable asserts the failure path: nothing listening
// on the configured address yields a non-zero exit code. A listener is
// opened and immediately closed to obtain a port number that is (briefly)
// guaranteed free, rather than hardcoding a port that might be in use.
func TestRunHealthcheck_Unreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	if err := ln.Close(); err != nil {
		t.Fatalf("close reserved listener: %v", err)
	}

	t.Setenv("HTTP_ADDR", ":"+strconv.Itoa(addr.Port))

	if got := runHealthcheck(); got == 0 {
		t.Fatalf("runHealthcheck() = 0, want non-zero when nothing is listening")
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
