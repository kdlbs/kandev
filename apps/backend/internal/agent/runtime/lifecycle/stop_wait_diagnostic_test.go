package lifecycle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"
	"time"
)

func TestStopWaitWatchReportsLatestPhase(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reported := make(chan stopWaitPhase, 1)
		watch := newStopWaitWatch(time.Second, func(phase stopWaitPhase) {
			reported <- phase
		})
		watch.advance(stopWaitRuntime)

		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case phase := <-reported:
			if phase != stopWaitRuntime {
				t.Fatalf("reported phase = %q, want %q", phase, stopWaitRuntime)
			}
		default:
			t.Fatal("watch did not report after its timeout")
		}
		watch.finish()
	})
}

func TestStopWaitWatchFinishesBeforeTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reported := make(chan stopWaitPhase, 1)
		watch := newStopWaitWatch(time.Second, func(phase stopWaitPhase) {
			reported <- phase
		})
		watch.finish()

		time.Sleep(2 * time.Second)
		synctest.Wait()
		if len(reported) != 0 {
			t.Fatal("finished watch reported a stop wait")
		}
	})
}

func TestDefaultStopWaitSnapshotDirUsesUserCache(t *testing.T) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("user cache directory unavailable: %v", err)
	}
	dir, err := defaultStopWaitSnapshotDir()
	if err != nil {
		t.Fatalf("default snapshot directory: %v", err)
	}
	want := filepath.Join(cacheDir, "kandev", "stop-wait")
	if dir != want {
		t.Fatalf("snapshot directory = %q, want %q", dir, want)
	}
}

func TestStopWaitLimitedWriterMarksOnlyTruncatedOutput(t *testing.T) {
	t.Run("exact limit", func(t *testing.T) {
		var output bytes.Buffer
		writer := &stopWaitLimitedWriter{writer: &output, remaining: 4}
		n, err := writer.Write([]byte("four"))
		if err != nil || n != 4 {
			t.Fatalf("Write() = (%d, %v), want (4, nil)", n, err)
		}
		if writer.truncated {
			t.Fatal("exactly full output was marked truncated")
		}
	})

	t.Run("over limit", func(t *testing.T) {
		var output bytes.Buffer
		writer := &stopWaitLimitedWriter{writer: &output, remaining: 4}
		n, err := writer.Write([]byte("fifth"))
		if err != nil || n != 5 {
			t.Fatalf("Write() = (%d, %v), want (5, nil)", n, err)
		}
		if output.String() != "fift" || !writer.truncated {
			t.Fatalf("output = %q, truncated = %v", output.String(), writer.truncated)
		}
	})
}

func TestReportStopWaitDiagnosticLogsEveryExpiredWatch(t *testing.T) {
	resetStopWaitSnapshotCooldown(t)
	log, logs := observedLogger(t)
	dir := filepath.Join(t.TempDir(), "snapshots")

	reportStopWaitDiagnosticInDir(log, "exec-one", stopWaitRuntime, dir)
	reportStopWaitDiagnosticInDir(log, "exec-two", stopWaitActivity, dir)

	if got := logs.FilterMessage("agent stop is taking longer than expected").Len(); got != 2 {
		t.Fatalf("phase warnings = %d, want 2", got)
	}
	snapshots := logs.FilterMessage("stop wait goroutine snapshot").All()
	if len(snapshots) != 1 {
		t.Fatalf("snapshot logs = %d, want 1 during cooldown", len(snapshots))
	}
	path, _ := snapshots[0].ContextMap()["goroutine_snapshot"].(string)
	if path == "" {
		t.Fatal("snapshot log does not contain its file path")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if len(content) == 0 || len(content) > stopWaitSnapshotLimit {
		t.Fatalf("snapshot size = %d, want 1..%d bytes", len(content), stopWaitSnapshotLimit)
	}
}

func TestStopWaitSnapshotRetriesAfterCreateFailure(t *testing.T) {
	resetStopWaitSnapshotCooldown(t)
	log, logs := observedLogger(t)
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}

	captureStopWaitSnapshotInDir(log, "exec-one", stopWaitRuntime, blocker)
	captureStopWaitSnapshotInDir(log, "exec-two", stopWaitRuntime, root)

	if got := logs.FilterMessage("stop wait snapshot unavailable").Len(); got != 1 {
		t.Fatalf("unavailable snapshot logs = %d, want 1", got)
	}
	if got := logs.FilterMessage("stop wait goroutine snapshot").Len(); got != 1 {
		t.Fatalf("successful snapshot logs = %d, want 1", got)
	}
	paths, err := filepath.Glob(filepath.Join(root, "kandev-stop-wait-*.goroutines"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("snapshot files = %v, err = %v; want one file", paths, err)
	}
}

func TestPruneStopWaitSnapshotsKeepsRecentFilesAndOtherEntries(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 6; i++ {
		path := filepath.Join(dir, "kandev-stop-wait-test"+strconv.Itoa(i)+".goroutines")
		if err := os.WriteFile(path, []byte("snapshot"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	unrelated := filepath.Join(dir, "other.goroutines")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "kandev-stop-wait-test5.goroutines")
	if err := pruneStopWaitSnapshots(dir, current); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "kandev-stop-wait-*.goroutines"))
	if err != nil || len(paths) != maxStopWaitSnapshots {
		t.Fatalf("retained snapshots = %d, err = %v; want %d", len(paths), err, maxStopWaitSnapshots)
	}
	for i := 2; i < 6; i++ {
		path := filepath.Join(dir, "kandev-stop-wait-test"+strconv.Itoa(i)+".goroutines")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("recent snapshot %q was removed: %v", path, err)
		}
	}
	for i := 0; i < 2; i++ {
		path := filepath.Join(dir, "kandev-stop-wait-test"+strconv.Itoa(i)+".goroutines")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("old snapshot %q still exists, err = %v", path, err)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestPruneStopWaitSnapshotsContinuesAfterRemovalError(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 6; i++ {
		path := filepath.Join(dir, "kandev-stop-wait-test"+strconv.Itoa(i)+".goroutines")
		if err := os.WriteFile(path, []byte("snapshot"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	current := filepath.Join(dir, "kandev-stop-wait-test5.goroutines")
	failedPath := filepath.Join(dir, "kandev-stop-wait-test0.goroutines")
	removeCalls := 0
	err := pruneStopWaitSnapshotsWithRemove(dir, current, func(path string) error {
		removeCalls++
		if path == failedPath {
			return errors.New("permission denied")
		}
		return os.Remove(path)
	})
	if err == nil {
		t.Fatal("prune error = nil, want removal error")
	}
	if removeCalls != 2 {
		t.Fatalf("remove calls = %d, want 2", removeCalls)
	}
	if _, err := os.Stat(failedPath); err != nil {
		t.Fatalf("failed removal path should remain: %v", err)
	}
	removedPath := filepath.Join(dir, "kandev-stop-wait-test1.goroutines")
	if _, err := os.Stat(removedPath); !os.IsNotExist(err) {
		t.Fatalf("later removal should continue, stat error = %v", err)
	}
}

func resetStopWaitSnapshotCooldown(t *testing.T) {
	t.Helper()
	previous := lastStopWaitSnapshot.Swap(0)
	t.Cleanup(func() { lastStopWaitSnapshot.Store(previous) })
}
