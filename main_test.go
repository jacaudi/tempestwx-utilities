package main

import (
	"context"
	"os"
	"slices"
	"syscall"
	"testing"
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
