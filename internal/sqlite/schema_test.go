package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrate_CreatesTablesAndVersion(t *testing.T) {
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	wantTables := []string{
		"tempest_observations",
		"tempest_rapid_wind",
		"tempest_hub_status",
		"tempest_events",
	}
	for _, table := range wantTables {
		assertTableExists(t, db, table)
	}
	assertIndexExists(t, db, "idx_obs_serial_time")
	// idx_obs_time (0002_add_timestamp_index.sql) leads with timestamp alone
	// so the read hot-path (LatestObservationAny's ORDER BY timestamp DESC
	// LIMIT 1 and HistoryPoints' WHERE timestamp BETWEEN ?) can use an index
	// instead of a full table scan + sort -- idx_obs_serial_time can't serve
	// either query since it leads with serial_number (SGE review I1).
	assertIndexExists(t, db, "idx_obs_time")
	assertSchemaVersion(t, db, 2)

	// Idempotent: running Migrate again must not fail and must leave the
	// schema at the same version.
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}
	assertSchemaVersion(t, db, 2)
}

func assertTableExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master for table %q: %v", name, err)
	}
	if count != 1 {
		t.Errorf("table %q missing after Migrate()", name)
	}
}

func assertIndexExists(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count)
	if err != nil {
		t.Fatalf("query sqlite_master for index %q: %v", name, err)
	}
	if count != 1 {
		t.Errorf("index %q missing after Migrate()", name)
	}
}

func assertSchemaVersion(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	err := db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&got)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if got != want {
		t.Errorf("schema_version = %d, want %d", got, want)
	}
}
