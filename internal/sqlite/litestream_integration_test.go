//go:build integration

package sqlite

import "testing"

// TestLitestreamRestore_S3Replica is a placeholder for the optional
// S3/MinIO-replica variant of TestLitestreamRestore_FileReplica.
//
// It is deliberately NOT wired to a testcontainers-managed MinIO instance:
// the task brief marks this variant OPTIONAL and explicitly says not to sink
// time on it, and the default-build file-replica test already proves the
// actual requirement — design §10's PRAGMA/WAL/restore contract — without
// needing a second replica backend to do it. Adding a testcontainers-go +
// MinIO dependency here, with no present consumer requiring S3-replica
// coverage specifically, would be exactly the speculative generality YAGNI
// rejects.
//
// If a concrete requirement for S3-replica coverage appears (e.g. a
// production deployment starts relying on an S3-compatible replica), replace
// this skip with a real MinIO-backed round-trip mirroring
// TestLitestreamRestore_FileReplica's structure, swapping the replica URL for
// an `s3://` one pointed at the container's endpoint.
func TestLitestreamRestore_S3Replica(t *testing.T) {
	t.Skip("S3/MinIO replica round-trip not implemented: optional per task 3.6 brief; " +
		"the default-build TestLitestreamRestore_FileReplica already proves the PRAGMA/WAL/restore contract (design §10)")
}
