package journal

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
func TestJournalOpenFailureClassification(t *testing.T) {
	t.Run("lock timeout is typed and keeps bolt compatibility", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "delivery.bbolt")
		owner, err := Open(Config{Path: path})
		if err != nil {
			t.Fatalf("open owner journal: %v", err)
		}
		defer func() { _ = owner.Close() }()

		started := time.Now()
		_, err = Open(Config{Path: path, OpenTimeout: 50 * time.Millisecond})
		if err == nil {
			t.Fatal("second open of a locked journal = nil error, want lock timeout")
		}
		assertOpenReason(t, err, OpenReasonLockTimeout)
		if !errors.Is(err, bolt.ErrTimeout) {
			t.Fatalf("open error %v does not preserve bolt.ErrTimeout", err)
		}
		var typed *OpenError
		if !errors.As(err, &typed) {
			t.Fatalf("open error %v is not an *OpenError", err)
		}
		if typed.Path() != path || typed.ConfiguredTimeout() != 50*time.Millisecond {
			t.Fatalf("open evidence = path %q timeout %s", typed.Path(), typed.ConfiguredTimeout())
		}
		if typed.Elapsed() <= 0 || time.Since(started) > 5*time.Second {
			t.Fatalf("elapsed evidence = %s, want a bounded acquisition attempt", typed.Elapsed())
		}
		if strings.Contains(typed.Error(), path) {
			t.Fatalf("safe error projection leaked the journal path: %v", typed)
		}
	})

	t.Run("corrupt journal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "delivery.bbolt")
		if err := os.WriteFile(path, make([]byte, 8192), 0o600); err != nil {
			t.Fatalf("write corrupt journal: %v", err)
		}
		_, err := Open(Config{Path: path})
		if err == nil {
			t.Fatal("open corrupt journal = nil error, want corruption")
		}
		assertOpenReason(t, err, OpenReasonCorrupt)
	})

	t.Run("newer version", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "delivery.bbolt")
		created, err := Open(Config{Path: path})
		if err != nil {
			t.Fatalf("create journal: %v", err)
		}
		if err := created.Close(); err != nil {
			t.Fatalf("close created journal: %v", err)
		}
		db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
		if err != nil {
			t.Fatalf("open raw journal: %v", err)
		}
		if err := db.Update(func(tx *bolt.Tx) error {
			version := make([]byte, 4)
			binary.BigEndian.PutUint32(version, CurrentVersion+1)
			return tx.Bucket([]byte("meta")).Put([]byte("version"), version)
		}); err != nil {
			_ = db.Close()
			t.Fatalf("write newer version: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("close raw journal: %v", err)
		}
		_, err = Open(Config{Path: path})
		if err == nil {
			t.Fatal("open newer journal = nil error, want rejection")
		}
		assertOpenReason(t, err, OpenReasonNewerVersion)
		if !errors.Is(err, ErrJournalNewerVersion) {
			t.Fatalf("open error %v does not preserve ErrJournalNewerVersion", err)
		}
	})

	t.Run("permission denied when the platform enforces it", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "delivery.bbolt")
		created, err := Open(Config{Path: path})
		if err != nil {
			t.Fatalf("create journal: %v", err)
		}
		if err := created.Close(); err != nil {
			t.Fatalf("close created journal: %v", err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Skipf("chmod is unavailable: %v", err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
		if probe, probeErr := os.Open(path); probeErr == nil {
			_ = probe.Close()
			t.Skip("running with privileges that bypass permission bits")
		}
		_, err = Open(Config{Path: path})
		if err == nil {
			t.Fatal("open unreadable journal = nil error, want permission failure")
		}
		assertOpenReason(t, err, OpenReasonPermissionDenied)
	})
}

func assertOpenReason(t *testing.T, err error, want OpenReason) {
	t.Helper()
	var typed *OpenError
	if !errors.As(err, &typed) {
		t.Fatalf("error %v is not an *OpenError", err)
	}
	if typed.Reason() != want {
		t.Fatalf("open reason = %q, want %q (%v)", typed.Reason(), want, err)
	}
	if !strings.Contains(err.Error(), string(want)) {
		t.Fatalf("error %q does not state its safe reason %q", err, want)
	}
}

func TestOpenErrorKeepsCauseWithoutProjectingIt(t *testing.T) {
	cause := errors.New("operation failed for /secret/path with prompt sentinel")
	err := newOpenError("/secret/path", time.Second, time.Millisecond, cause)
	if strings.Contains(err.Error(), "/secret/path") || strings.Contains(err.Error(), "prompt sentinel") {
		t.Fatalf("open error leaked acquisition detail: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("open error %v does not unwrap its cause", err)
	}
	if reason, ok := openErrorReason(err); !ok || reason != OpenReasonUnavailable {
		t.Fatalf("open reason = %q/%t, want unavailable", reason, ok)
	}
}
