package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────
// Tests are organized by the seven Go test categories:
//   1. Happy path       2. Table-driven     3. Edge/boundary
//   4. Error handling    5. Concurrency/race 6. Integration
//   7. Benchmark (+ Fuzz)
// ─────────────────────────────────────────────────────────────────────────

// Category 1 — Happy path: New returns a usable logger and Init installs a
// non-nil default without panicking.
func TestNew_ReturnsLogger(t *testing.T) {
	if got := New("info", "text"); got == nil {
		t.Fatal("New returned nil")
	}
	if got := Init(); got == nil {
		t.Fatal("Init returned nil")
	}
	if slog.Default() == nil {
		t.Fatal("Init did not set a default logger")
	}
}

// Category 2 — Table-driven: level names map to the expected slog.Level.
func TestParseLevel_Table(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"Info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"nonsense", slog.LevelInfo},
	}
	for _, tc := range cases {
		if got := ParseLevel(tc.in); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Category 3 — Edge/boundary: surrounding whitespace and mixed case are
// tolerated, and a below-threshold record is filtered out.
func TestParseLevel_WhitespaceAndFiltering(t *testing.T) {
	if got := ParseLevel("  Warn  "); got != slog.LevelWarn {
		t.Errorf("whitespace/case not normalized: got %v", got)
	}

	var buf bytes.Buffer
	logger := newLogger(&buf, "error", "text")
	logger.Info("should be filtered") // info < error → dropped
	logger.Error("should appear")
	out := buf.String()
	if strings.Contains(out, "should be filtered") {
		t.Errorf("info record leaked at error level: %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("error record missing at error level: %q", out)
	}
}

// Category 4 — Error handling: the public API has no error return paths (level
// and format are best-effort with safe fallbacks). This test documents and
// locks the fallback contract: unrecognized inputs must not panic and must not
// silently disable logging.
func TestNew_UnknownInputsFallBackSafely(t *testing.T) {
	var buf bytes.Buffer
	logger := newLogger(&buf, "bogus-level", "bogus-format")
	logger.Info("hello") // unknown level → info default, unknown format → text
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("unknown level/format did not fall back to a working logger: %q", buf.String())
	}
}

// Category 5 — Concurrency/race: New and ParseLevel are pure and must be safe to
// call from many goroutines (run under -race).
func TestConcurrentConstruction(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ParseLevel("warn")
			if New("debug", "json") == nil {
				t.Error("New returned nil under concurrency")
			}
		}()
	}
	wg.Wait()
}

// Category 6 — Integration: the text and json formats each produce output in the
// expected shape through the full New → handler → write path.
func TestFormats_Integration(t *testing.T) {
	t.Run("json is valid JSON with our attrs", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newLogger(&buf, "info", "json")
		logger.Info("startup", "component", "test", "count", 3)
		var m map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
			t.Fatalf("json format did not emit valid JSON: %v (%q)", err, buf.String())
		}
		if m["msg"] != "startup" || m["component"] != "test" {
			t.Errorf("json output missing expected fields: %v", m)
		}
	})
	t.Run("text is not JSON but contains the message", func(t *testing.T) {
		var buf bytes.Buffer
		logger := newLogger(&buf, "info", "text")
		logger.Info("startup", "component", "test")
		out := buf.String()
		if !strings.Contains(out, "msg=startup") || !strings.Contains(out, "component=test") {
			t.Errorf("text output missing key=value fields: %q", out)
		}
		var m map[string]any
		if json.Unmarshal([]byte(out), &m) == nil {
			t.Errorf("text format unexpectedly parsed as JSON: %q", out)
		}
	})
}

// Category 7a — Benchmark.
func BenchmarkParseLevel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ParseLevel("warning")
	}
}

// Category 7b — Fuzz: ParseLevel must never panic and must return one of the
// four valid slog levels for arbitrary input.
func FuzzParseLevel(f *testing.F) {
	for _, s := range []string{"debug", "INFO", " warn ", "error", "", "??"} {
		f.Add(s)
	}
	valid := map[slog.Level]bool{
		slog.LevelDebug: true, slog.LevelInfo: true,
		slog.LevelWarn: true, slog.LevelError: true,
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got := ParseLevel(s); !valid[got] {
			t.Errorf("ParseLevel(%q) returned invalid level %v", s, got)
		}
	})
}
