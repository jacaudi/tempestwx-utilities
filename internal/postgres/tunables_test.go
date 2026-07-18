package postgres

import (
	"testing"
	"time"
)

func TestPostgresTunables(t *testing.T) {
	getenv := func(k string) string {
		return map[string]string{
			"POSTGRES_BATCH_SIZE":     "7",
			"POSTGRES_FLUSH_INTERVAL": "2s",
			"POSTGRES_MAX_RETRIES":    "5",
		}[k]
	}
	tn := postgresTunables(getenv)
	if tn.batchSize != 7 || tn.flushInterval != 2*time.Second || tn.maxRetries != 5 {
		t.Fatalf("tunables not applied: %+v", tn)
	}
	// defaults when unset:
	def := postgresTunables(func(string) string { return "" })
	if def.batchSize != 100 || def.flushInterval != 10*time.Second || def.maxRetries != 3 {
		t.Fatalf("defaults wrong: %+v", def)
	}
	// non-positive/unparseable values fall back to defaults (the n > 0 / d > 0 guard):
	badEnv := func(k string) string {
		return map[string]string{
			"POSTGRES_BATCH_SIZE":     "0",
			"POSTGRES_FLUSH_INTERVAL": "abc",
			"POSTGRES_MAX_RETRIES":    "-1",
		}[k]
	}
	badTn := postgresTunables(badEnv)
	if badTn.batchSize != 100 || badTn.flushInterval != 10*time.Second || badTn.maxRetries != 3 {
		t.Fatalf("non-positive/unparseable values should fall back to defaults: %+v", badTn)
	}
}
