package prometheus

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestDeprecationWarning_EmitsOnce proves the deprecation warning fires
// exactly once even when the warn func is invoked twice (the "both
// pushgateway and scrape enabled" case). A fresh warner is created per test
// via newDeprecationWarner so this assertion doesn't depend on the
// package-level sync.OnceFunc's state, which persists across the whole test
// binary.
func TestDeprecationWarning_EmitsOnce(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	warn := newDeprecationWarner(logger)

	warn()
	warn()

	output := buf.String()
	count := strings.Count(output, deprecationMsg)
	if count != 1 {
		t.Fatalf("expected deprecation message exactly once, got %d occurrence(s) in output: %q", count, output)
	}
	if !strings.Contains(output, "level=WARN") {
		t.Errorf("expected WARN level in output, got: %q", output)
	}
}
