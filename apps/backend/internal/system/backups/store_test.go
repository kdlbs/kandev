package backups

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/jobs"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stderr"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

func newTestPool(t *testing.T) (*db.Pool, string) {
	t.Helper()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "kandev.db")

	writer, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := writer.Exec(`CREATE TABLE things (id TEXT PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := writer.Exec(`INSERT INTO things VALUES ('1','hello')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	pool := db.NewPool(writer, writer)
	return pool, dataDir
}

func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	pool, dataDir := newTestPool(t)
	tracker := jobs.NewTracker(nil, newTestLogger(t))
	svc := NewService(dataDir, pool, tracker, newTestLogger(t))
	return svc, dataDir
}

// waitForJob polls the tracker for a terminal state. Reused from
// jobs_test patterns to avoid sleeping forever in CI.
func waitForJob(t *testing.T, tracker *jobs.Tracker, id string, want jobs.State) *jobs.Job {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		j := tracker.Get(id)
		if j != nil && j.State == want {
			return j
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %s did not reach %s in 3s; last = %+v", id, want, tracker.Get(id))
	return nil
}

func TestList_EmptyDirReturnsEmptySlice(t *testing.T) {
	svc, _ := newTestService(t)
	got, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(got))
	}
}

func TestList_ClassifiesAutoAndManual(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := []string{
		"kandev-v0.1.0-20260101T000000Z.db",
		"manual-1700000000.db",
		"random.txt",
	}
	for _, n := range seed {
		if err := os.WriteFile(filepath.Join(backupsDir, n), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}

	got, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 snapshots (only .db), got %d: %+v", len(got), got)
	}

	kinds := map[string]string{}
	for _, s := range got {
		kinds[s.Name] = s.Kind
	}
	if kinds["kandev-v0.1.0-20260101T000000Z.db"] != "auto" {
		t.Errorf("kandev-* should be auto, got %q", kinds["kandev-v0.1.0-20260101T000000Z.db"])
	}
	if kinds["manual-1700000000.db"] != "manual" {
		t.Errorf("manual-* should be manual, got %q", kinds["manual-1700000000.db"])
	}
}

func TestCreate_ProducesManualVacuumIntoFile(t *testing.T) {
	svc, dataDir := newTestService(t)
	id := svc.Create(context.Background())
	if id == "" {
		t.Fatal("expected non-empty job id")
	}
	waitForJob(t, svc.jobs, id, jobs.StateSucceeded)

	backupsDir := filepath.Join(dataDir, "backups")
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var manualFile string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "manual-") && strings.HasSuffix(e.Name(), ".db") {
			manualFile = filepath.Join(backupsDir, e.Name())
		}
	}
	if manualFile == "" {
		t.Fatalf("manual-*.db not produced; dir contents: %+v", entries)
	}

	// Verify the snapshot is a valid SQLite file and contains seeded row.
	snap, err := sqlx.Open("sqlite3", manualFile)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = snap.Close() }()
	var name string
	if err := snap.QueryRow(`SELECT name FROM things WHERE id='1'`).Scan(&name); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if name != "hello" {
		t.Errorf("snapshot row = %q, want %q", name, "hello")
	}
}

// Regression for the truncated-size bug: the manual snapshot must be written
// to a ".tmp" sidecar and atomically renamed into place, so a concurrent
// List() never stats a half-written file. After a successful Create there
// must be exactly one "manual-*.db" and no leftover ".tmp" file, and the
// reported size must equal the final file's on-disk size.
func TestCreate_AtomicRenameLeavesNoTmpAndReportsFullSize(t *testing.T) {
	svc, dataDir := newTestService(t)
	id := svc.Create(context.Background())
	waitForJob(t, svc.jobs, id, jobs.StateSucceeded)

	backupsDir := filepath.Join(dataDir, "backups")
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	var manualFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover tmp file after Create: %s", e.Name())
		}
		if strings.HasPrefix(e.Name(), "manual-") && strings.HasSuffix(e.Name(), ".db") {
			manualFile = filepath.Join(backupsDir, e.Name())
		}
	}
	if manualFile == "" {
		t.Fatalf("manual-*.db not produced; dir contents: %+v", entries)
	}

	// The size reported in the job result must match the final file on disk,
	// not a partial VACUUM INTO write.
	info, err := os.Stat(manualFile)
	if err != nil {
		t.Fatalf("stat manual file: %v", err)
	}
	job := svc.jobs.Get(id)
	if job == nil {
		t.Fatal("job not found")
	}
	reported, ok := job.Result["size_bytes"].(int64)
	if !ok {
		t.Fatalf("size_bytes missing or wrong type in job result: %+v", job.Result)
	}
	if reported != info.Size() {
		t.Errorf("reported size %d != on-disk size %d", reported, info.Size())
	}
}

// A ".tmp" sidecar (an in-progress VACUUM INTO) must never appear in List(),
// so the UI cannot show or restore a half-written snapshot.
func TestList_IgnoresTmpSidecar(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := []string{
		"manual-1700000000.db",
		"manual-1700000001.db.tmp",
	}
	for _, n := range seed {
		if err := os.WriteFile(filepath.Join(backupsDir, n), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}

	got, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "manual-1700000000.db" {
		t.Errorf("expected only the completed .db snapshot, got %+v", got)
	}
}

// A crash between SnapshotSQLite and os.Rename leaves a ".tmp" sidecar that
// classify() hides from List()/Delete(), so it could never be cleaned up via
// the UI. Create must sweep such stale sidecars before writing a new snapshot.
func TestCreate_SweepsStaleTmpSidecar(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := filepath.Join(backupsDir, "manual-1700000000.db.tmp")
	if err := os.WriteFile(stale, []byte("crashed vacuum debris"), 0o644); err != nil {
		t.Fatalf("seed stale tmp: %v", err)
	}

	id := svc.Create(context.Background())
	waitForJob(t, svc.jobs, id, jobs.StateSucceeded)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale tmp sidecar not swept: stat err = %v", err)
	}
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover tmp file after Create: %s", e.Name())
		}
	}
}

// Core retention property: persistence.PruneBackups(dir, 0) must remove all
// auto snapshots but must NOT touch manual snapshots. This is the contract
// that lets manual snapshots survive the existing pre-migration pruning.
func TestCreate_ManualSurvivesPruneBackups(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")

	// Drop a synthetic auto-snapshot alongside the manual one.
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	autoFile := filepath.Join(backupsDir, "kandev-v0.1.0-20260101T000000Z.db")
	if err := os.WriteFile(autoFile, []byte("auto"), 0o644); err != nil {
		t.Fatalf("seed auto: %v", err)
	}

	id := svc.Create(context.Background())
	waitForJob(t, svc.jobs, id, jobs.StateSucceeded)

	if err := persistence.PruneBackups(backupsDir, 0); err != nil {
		t.Fatalf("PruneBackups: %v", err)
	}

	if _, err := os.Stat(autoFile); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("auto snapshot should have been pruned, err=%v", err)
	}

	entries, _ := os.ReadDir(backupsDir)
	var manualSurvived bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "manual-") && strings.HasSuffix(e.Name(), ".db") {
			manualSurvived = true
		}
	}
	if !manualSurvived {
		t.Errorf("manual snapshot was incorrectly pruned; remaining: %+v", entries)
	}
}

func TestRestore_WrongConfirmReturnsError(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.Restore(context.Background(), "manual-1.db", "WRONG")
	if err == nil {
		t.Fatal("expected error for wrong confirm token")
	}
}

func TestRestore_SuccessStagesFileAndFlagsRestartRequired(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Make a valid SQLite snapshot to restore.
	id := svc.Create(context.Background())
	waitForJob(t, svc.jobs, id, jobs.StateSucceeded)

	entries, _ := os.ReadDir(backupsDir)
	var snapName string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "manual-") {
			snapName = e.Name()
		}
	}
	if snapName == "" {
		t.Fatal("no manual snapshot to restore")
	}

	jobID, err := svc.Restore(context.Background(), snapName, "RESTORE")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	waitForJob(t, svc.jobs, jobID, jobs.StateSucceeded)

	// The DB must be in place at the canonical path post-restore.
	if _, err := os.Stat(filepath.Join(dataDir, "kandev.db")); err != nil {
		t.Errorf("expected kandev.db at canonical path: %v", err)
	}

	// The job result must flag restart_required=true so the frontend dialog
	// knows to prompt the user to quit and relaunch.
	job := svc.jobs.Get(jobID)
	if job == nil {
		t.Fatalf("job not found")
	}
	if got, _ := job.Result["restart_required"].(bool); !got {
		t.Errorf("restart_required missing or false in job result: %+v", job.Result)
	}
}

func TestRestore_WriteFailurePreservesOriginal(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	id := svc.Create(context.Background())
	waitForJob(t, svc.jobs, id, jobs.StateSucceeded)

	entries, _ := os.ReadDir(backupsDir)
	var snapName string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "manual-") {
			snapName = e.Name()
		}
	}

	// Capture the original kandev.db bytes (it exists because newTestPool
	// opened sqlite at that path).
	originalPath := filepath.Join(dataDir, "kandev.db")
	originalBytes, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	// Inject a failure in the staged-write step.
	svc.failWritesForTest = true

	jobID, err := svc.Restore(context.Background(), snapName, "RESTORE")
	if err != nil {
		t.Fatalf("Restore submission: %v", err)
	}
	waitForJob(t, svc.jobs, jobID, jobs.StateFailed)

	got, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read after fail: %v", err)
	}
	if string(got) != string(originalBytes) {
		t.Error("original kandev.db was modified despite failed restore")
	}
}

func TestDelete_RemovesManualFile(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(backupsDir, "manual-1.db")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.Delete("manual-1.db"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected file removed, err=%v", err)
	}
}

func TestDelete_RefusesPreResetSnapshot(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(backupsDir, "kandev-pre-reset-20260101T000000Z.db")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := svc.Delete("kandev-pre-reset-20260101T000000Z.db")
	if err == nil {
		t.Fatal("expected error refusing pre-reset deletion")
	}
	if _, statErr := os.Stat(target); statErr != nil {
		t.Errorf("pre-reset file unexpectedly removed: %v", statErr)
	}
}

func TestOpenForDownload_RejectsTraversal(t *testing.T) {
	svc, _ := newTestService(t)
	cases := []string{"../etc/passwd", "..", "foo/bar.db", "/abs/path.db"}
	for _, name := range cases {
		f, _, err := svc.OpenForDownload(name)
		if err == nil {
			_ = f.Close()
			t.Errorf("OpenForDownload(%q) succeeded; expected error", name)
		}
	}
}

func TestResolveSnapshotPath_RejectsNonSnapshotFile(t *testing.T) {
	svc, _ := newTestService(t)
	for _, name := range []string{"config.db", "secrets.db", "kandev.db"} {
		if _, err := svc.resolveSnapshotPath(name); err == nil {
			t.Errorf("resolveSnapshotPath(%q) succeeded, want rejection", name)
		}
	}
}

func TestOpenForDownload_ValidName(t *testing.T) {
	svc, dataDir := newTestService(t)
	backupsDir := filepath.Join(dataDir, "backups")
	if err := os.MkdirAll(backupsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(backupsDir, "manual-42.db")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	f, size, err := svc.OpenForDownload("manual-42.db")
	if err != nil {
		t.Fatalf("OpenForDownload: %v", err)
	}
	defer func() { _ = f.Close() }()
	if size != int64(len("hello")) {
		t.Errorf("size = %d, want %d", size, len("hello"))
	}
}
