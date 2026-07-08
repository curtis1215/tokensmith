package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestLockExcludesSecondHolder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "daemon.lock")
	rel, err := AcquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(p); err == nil {
		t.Fatal("second AcquireLock should fail while first is held")
	}
	if err := rel(); err != nil {
		t.Fatal(err)
	}
	// after release, a new holder can acquire
	rel2, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("acquire after release failed: %v", err)
	}
	rel2()
}

func TestLockStealsStale(t *testing.T) {
	p := filepath.Join(t.TempDir(), "daemon.lock")
	// a dead PID that is very unlikely to be running
	os.WriteFile(p, []byte(strconv.Itoa(2_000_000_000)), 0o644)
	rel, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("should steal a stale lock, got %v", err)
	}
	rel()
}
