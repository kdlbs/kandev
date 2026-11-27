package persistence

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func bootStagingTestLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return log, logs
}

func bootStagingWarnings(t *testing.T, logs *observer.ObservedLogs) []map[string]any {
	t.Helper()
	var warnings []map[string]any
	for _, observed := range logs.All() {
		entry := observed.ContextMap()
		entry["msg"] = observed.Message
		warnings = append(warnings, entry)
	}
	return warnings
}

// @covers AC-PLATFORM-SQLITE-FILE-HYGIENE-002.4
func TestSweepStaleBootStagingListingFailureAndAbsence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backups")
	if err := os.WriteFile(path, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	log, observed := bootStagingTestLogger(t)
	SweepStaleBootStaging(path, log)
	warnings := bootStagingWarnings(t, observed)
	if len(warnings) != 1 || warnings[0]["msg"] != "persistence: failed to list boot backup staging directories" || warnings[0]["path"] != path || warnings[0]["error"] == nil {
		t.Fatalf("listing failure lacks warning context: %v", warnings)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "preserve" {
		t.Fatalf("listing failure changed the file: %q, %v", data, err)
	}
	SweepStaleBootStaging(path+"-missing", log)
	if warnings := bootStagingWarnings(t, observed); len(warnings) != 1 {
		t.Fatalf("missing directory logged a warning: %v", warnings)
	}
}

// @covers AC-PLATFORM-SQLITE-FILE-HYGIENE-002.4
func TestSweepStaleBootStagingWarnings(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires non-root POSIX directory permissions")
	}
	for _, failure := range []string{"listing", "inspection", "removal"} {
		t.Run(failure, func(t *testing.T) {
			backupDir := t.TempDir()
			stale := seedBootStaging(t, backupDir, bootStagingDirPrefix+"stale", 2*staleBootStagingAge)
			blocked, mode := stale, os.FileMode(0)
			message := "persistence: failed to inspect boot backup staging directory"
			switch failure {
			case "listing":
				blocked = backupDir
				message = "persistence: failed to list boot backup staging directories"
			case "removal":
				blocked, mode = backupDir, 0o500
				message = "persistence: failed to reclaim stale boot backup staging directory"
			}
			if err := os.Chmod(blocked, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
			log, observed := bootStagingTestLogger(t)
			SweepStaleBootStaging(backupDir, log)
			warnings := bootStagingWarnings(t, observed)
			if len(warnings) != 1 {
				t.Fatalf("warnings = %v, want one %q", warnings, message)
			}
			wantPath := stale
			if failure == "listing" {
				wantPath = backupDir
			}
			if warnings[0]["msg"] != message || warnings[0]["path"] != wantPath || warnings[0]["error"] == nil {
				t.Fatalf("warning lacks failure context: %v", warnings[0])
			}
			if err := os.Chmod(blocked, 0o700); err != nil {
				t.Fatal(err)
			}
			requireExists(t, stale, true)
			old := time.Now().Add(-2 * staleBootStagingAge)
			if err := os.Chtimes(stale, old, old); err != nil {
				t.Fatal(err)
			}
			SweepStaleBootStaging(backupDir, nil)
			requireExists(t, stale, false)
		})
	}
}

// @covers AC-PLATFORM-SQLITE-FILE-HYGIENE-002.3, AC-PLATFORM-SQLITE-FILE-HYGIENE-002.4
func TestSweepStaleBootStagingConcurrentSweeps(t *testing.T) {
	backupDir := t.TempDir()
	stale := seedBootStaging(t, backupDir, bootStagingDirPrefix+"stale", 2*staleBootStagingAge)
	live := seedBootStaging(t, backupDir, bootStagingDirPrefix+"live", staleBootStagingAge/2)
	log, observed := bootStagingTestLogger(t)
	var sweeps sync.WaitGroup
	start := make(chan struct{})
	for range 16 {
		sweeps.Go(func() { <-start; SweepStaleBootStaging(backupDir, log) })
	}
	close(start)
	sweeps.Wait()
	requireExists(t, stale, false)
	requireExists(t, live, true)
	if warnings := bootStagingWarnings(t, observed); len(warnings) != 0 {
		t.Fatalf("benign concurrent removal logged warnings: %v", warnings)
	}
}

// @covers AC-PLATFORM-SQLITE-FILE-HYGIENE-002.3
func TestSweepStaleBootStagingPreservesSymlinks(t *testing.T) {
	backupDir := t.TempDir()
	outside := seedBootStaging(t, t.TempDir(), "outside", 2*staleBootStagingAge)
	link := filepath.Join(backupDir, bootStagingDirPrefix+"link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	SweepStaleBootStaging(backupDir, nil)
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("staging-prefix symlink changed: %v", err)
	}
	requireExists(t, outside, true)
}
