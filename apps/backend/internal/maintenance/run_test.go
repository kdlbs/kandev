package maintenance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/metrics"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestRunDryRunMissingDatabaseCreatesNothing(t *testing.T) {
	databaseDir := t.TempDir()
	databasePath := filepath.Join(databaseDir, "kandev.db")

	_, err := Run(context.Background(), t.TempDir(), "sqlite", databasePath, RunOptions{Execute: false}, nil)
	if err == nil {
		t.Fatal("Run(dry-run) error = nil, want missing database error")
	}
	if _, statErr := os.Stat(databasePath); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run created missing database file; stat err = %v", statErr)
	}
}

func TestRunDryRunLegacySchemaDoesNotMigrateOrHeal(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := db.OpenSQLite(databasePath)
	if err != nil {
		t.Fatalf("open legacy sqlite: %v", err)
	}
	if _, err := conn.Exec(`CREATE TABLE legacy_marker (id TEXT PRIMARY KEY)`); err != nil {
		_ = conn.Close()
		t.Fatalf("create legacy schema: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close legacy sqlite: %v", err)
	}

	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read legacy database before dry-run: %v", err)
	}
	_, err = Run(context.Background(), t.TempDir(), "sqlite", databasePath, RunOptions{Execute: false}, nil)
	if err == nil {
		t.Fatal("Run(dry-run) error = nil, want unsupported legacy schema error")
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("read legacy database after dry-run: %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("dry-run changed legacy database bytes")
	}
}

// TestRunDryRunReportsCandidatesWithoutMutating is this wave's core dry-run
// contract test: Analyze must report every category's exact candidate
// count/byte estimate while leaving every row, and the database file
// itself, completely unchanged.
func TestRunDryRunReportsCandidatesWithoutMutating(t *testing.T) {
	f := newTestFixture(t)
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	f.seedTask("task-dryrun")
	f.seedSession("task-dryrun", "session-dryrun")
	f.seedDuplicateGitSnapshots("session-dryrun", base)
	f.seedObsoletePlanRevisions("task-dryrun", 3, base)
	f.seedOrphanedMessagePayload("task-dryrun", "session-dryrun", "turn-dryrun")

	snapshotsBefore := f.countRows(`SELECT COUNT(*) FROM task_session_git_snapshots`)
	revisionsBefore := f.countRows(`SELECT COUNT(*) FROM task_plan_revisions`)
	payloadsBefore := f.countRows(`SELECT COUNT(*) FROM task_message_payloads`)
	sizeBefore := f.fileSize()

	outcome, err := Run(context.Background(), t.TempDir(), "sqlite", f.dbPath, RunOptions{Execute: false}, nil)
	if err != nil {
		t.Fatalf("Run(dry-run): %v", err)
	}
	if outcome.Executed {
		t.Fatal("Executed = true, want false for a dry-run")
	}
	if outcome.Report.DuplicateGitSnapshots.RowCount != 1 {
		t.Fatalf("DuplicateGitSnapshots.RowCount = %d, want 1", outcome.Report.DuplicateGitSnapshots.RowCount)
	}
	if outcome.Report.ObsoletePlanRevisions.RowCount != 2 {
		t.Fatalf("ObsoletePlanRevisions.RowCount = %d, want 2 (3 revisions, HEAD protected)", outcome.Report.ObsoletePlanRevisions.RowCount)
	}
	if outcome.Report.OrphanedMessagePayloads.RowCount != 1 {
		t.Fatalf("OrphanedMessagePayloads.RowCount = %d, want 1", outcome.Report.OrphanedMessagePayloads.RowCount)
	}
	if outcome.Report.DatabaseSizeBytes <= 0 {
		t.Fatalf("DatabaseSizeBytes = %d, want positive", outcome.Report.DatabaseSizeBytes)
	}

	if got := f.countRows(`SELECT COUNT(*) FROM task_session_git_snapshots`); got != snapshotsBefore {
		t.Fatalf("git snapshot rows changed: before=%d after=%d", snapshotsBefore, got)
	}
	if got := f.countRows(`SELECT COUNT(*) FROM task_plan_revisions`); got != revisionsBefore {
		t.Fatalf("plan revision rows changed: before=%d after=%d", revisionsBefore, got)
	}
	if got := f.countRows(`SELECT COUNT(*) FROM task_message_payloads`); got != payloadsBefore {
		t.Fatalf("message payload rows changed: before=%d after=%d", payloadsBefore, got)
	}
	if got := f.fileSize(); got != sizeBefore {
		t.Fatalf("database file size changed: before=%d after=%d", sizeBefore, got)
	}

	backupDir := filepath.Join(filepath.Dir(f.dbPath), "backups")
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create a backups dir; stat err = %v", err)
	}
}

// TestRunUnsupportedDriverRefusesImmediately confirms a non-sqlite driver
// (e.g. postgres) is rejected before any connection is opened or lock
// attempted, since database maintenance is a SQLite-only capability.
func TestRunUnsupportedDriverRefusesImmediately(t *testing.T) {
	_, err := Run(context.Background(), t.TempDir(), "postgres", "unused", RunOptions{Execute: true}, nil)
	if !errors.Is(err, ErrUnsupportedDriver) {
		t.Fatalf("err = %v, want ErrUnsupportedDriver", err)
	}
}

// TestRunExecuteRefusesWhenAnotherProcessHoldsOwnership proves --execute
// refuses to run while a live backend (simulated here by acquiring the same
// ownership targets directly) already owns the database, so a maintenance
// run and a running backend can never race on destructive operations.
func TestRunExecuteRefusesWhenAnotherProcessHoldsOwnership(t *testing.T) {
	f := newTestFixture(t)
	homeDir := t.TempDir()

	targets, err := ownershiplock.Targets(homeDir, "sqlite", f.dbPath)
	if err != nil {
		t.Fatalf("ownershiplock.Targets: %v", err)
	}
	owner, err := ownershiplock.Acquire(targets)
	if err != nil {
		t.Fatalf("ownershiplock.Acquire (simulated live backend): %v", err)
	}
	defer func() { _ = owner.Close() }()

	_, err = Run(context.Background(), homeDir, "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
	if !errors.Is(err, ErrOwnershipConflict) {
		t.Fatalf("err = %v, want ErrOwnershipConflict", err)
	}

	// The refusal must be a pure no-op: no backup, no deletion attempt.
	backupDir := filepath.Join(filepath.Dir(f.dbPath), "backups")
	if _, statErr := os.Stat(backupDir); !os.IsNotExist(statErr) {
		t.Fatalf("ownership-conflict refusal must not create a backup dir; stat err = %v", statErr)
	}
}

// TestRunExecuteDeletesCandidatesBackupAndIsIdempotent is the end-to-end
// happy path: --execute takes a verified backup, deletes exactly the
// reported candidates, and a second run against the now-converged database
// deletes nothing further (idempotent retention).
func TestRunExecuteDeletesCandidatesBackupAndIsIdempotent(t *testing.T) {
	f := newTestFixture(t)
	base := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

	f.seedTask("task-exec")
	f.seedSession("task-exec", "session-exec")
	f.seedDuplicateGitSnapshots("session-exec", base)
	f.seedObsoletePlanRevisions("task-exec", 3, base)
	f.seedOrphanedMessagePayload("task-exec", "session-exec", "turn-exec")

	homeDir := t.TempDir()
	outcome, err := Run(context.Background(), homeDir, "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
	if err != nil {
		t.Fatalf("Run(execute): %v", err)
	}
	if !outcome.Executed {
		t.Fatal("Executed = false, want true")
	}
	if outcome.Execution.DeletedGitSnapshots != 1 {
		t.Fatalf("DeletedGitSnapshots = %d, want 1", outcome.Execution.DeletedGitSnapshots)
	}
	if outcome.Execution.DeletedPlanRevisions != 2 {
		t.Fatalf("DeletedPlanRevisions = %d, want 2", outcome.Execution.DeletedPlanRevisions)
	}
	if outcome.Execution.DeletedMessagePayloads != 1 {
		t.Fatalf("DeletedMessagePayloads = %d, want 1", outcome.Execution.DeletedMessagePayloads)
	}
	if outcome.BackupPath == "" {
		t.Fatal("BackupPath is empty, want a verified backup path")
	}
	if _, err := os.Stat(outcome.BackupPath); err != nil {
		t.Fatalf("backup file missing at %s: %v", outcome.BackupPath, err)
	}
	if err := verifySQLiteIntegrity(outcome.BackupPath); err != nil {
		t.Fatalf("backup failed integrity check: %v", err)
	}

	if got := f.countRows(`SELECT COUNT(*) FROM task_session_git_snapshots`); got != 1 {
		t.Fatalf("git snapshot rows after execute = %d, want 1 (the retained newest row)", got)
	}
	if got := f.countRows(`SELECT COUNT(*) FROM task_plan_revisions`); got != 1 {
		t.Fatalf("plan revision rows after execute = %d, want 1 (HEAD)", got)
	}
	if got := f.countRows(`SELECT COUNT(*) FROM task_message_payloads`); got != 0 {
		t.Fatalf("message payload rows after execute = %d, want 0", got)
	}

	// Second execute against the now-converged database must delete
	// nothing further - idempotent retention.
	second, err := Run(context.Background(), homeDir, "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
	if err != nil {
		t.Fatalf("Run(execute, second): %v", err)
	}
	if second.Execution.TotalDeleted() != 0 {
		t.Fatalf("second execute deleted %d rows, want 0 (idempotent)", second.Execution.TotalDeleted())
	}
}

func TestRunExecutePreservesAuthoritativeGitSnapshots(t *testing.T) {
	tests := []struct {
		name         string
		snapshotType models.SnapshotType
		triggeredBy  string
		archiveTask  bool
	}{
		{name: "archive", snapshotType: models.SnapshotTypeArchive, triggeredBy: "task_archived", archiveTask: true},
		{name: "agent completed", snapshotType: models.SnapshotTypeStatusUpdate, triggeredBy: "agent_completed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestFixture(t)
			base := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
			taskID := "task-protected-" + strings.ReplaceAll(tt.name, " ", "-")
			sessionID := "session-protected-" + strings.ReplaceAll(tt.name, " ", "-")
			f.seedTask(taskID)
			f.seedSession(taskID, sessionID)
			if tt.archiveTask {
				if _, err := f.repo.DB().Exec(`UPDATE tasks SET archived_at = ? WHERE id = ?`, base, taskID); err != nil {
					t.Fatalf("archive task: %v", err)
				}
			}

			seedSnapshot := func(id string, snapshotType models.SnapshotType, triggeredBy string, createdAt time.Time) {
				t.Helper()
				err := f.repo.CreateGitSnapshot(context.Background(), &models.GitSnapshot{
					ID: id, SessionID: sessionID, SnapshotType: snapshotType, TriggeredBy: triggeredBy,
					Branch: "feature/protected", RemoteBranch: "origin/feature/protected",
					HeadCommit: "protected-head", BaseCommit: "protected-base",
					Files:     map[string]interface{}{"protected.go": map[string]interface{}{"status": "modified"}},
					CreatedAt: createdAt,
				})
				if err != nil {
					t.Fatalf("CreateGitSnapshot(%s): %v", id, err)
				}
			}

			protectedID := sessionID + "-authoritative"
			seedSnapshot(protectedID, tt.snapshotType, tt.triggeredBy, base)
			seedSnapshot(sessionID+"-live-old", models.SnapshotTypeStatusUpdate, sqlite.TriggeredByLiveMonitor, base.Add(time.Minute))
			seedSnapshot(sessionID+"-live-new", models.SnapshotTypeStatusUpdate, sqlite.TriggeredByLiveMonitor, base.Add(2*time.Minute))

			before, err := f.repo.GetLatestGitSnapshot(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("GetLatestGitSnapshot(before): %v", err)
			}
			if before.ID != protectedID {
				t.Fatalf("authoritative snapshot before maintenance = %s, want %s", before.ID, protectedID)
			}

			outcome, err := Run(context.Background(), t.TempDir(), "sqlite", f.dbPath, RunOptions{Execute: true}, nil)
			if err != nil {
				t.Fatalf("Run(execute): %v", err)
			}
			if outcome.Execution.DeletedGitSnapshots != 1 {
				t.Fatalf("DeletedGitSnapshots = %d, want only the older live duplicate", outcome.Execution.DeletedGitSnapshots)
			}

			after, err := f.repo.GetLatestGitSnapshot(context.Background(), sessionID)
			if err != nil {
				t.Fatalf("GetLatestGitSnapshot(after): %v", err)
			}
			if after.ID != protectedID {
				t.Fatalf("authoritative snapshot after maintenance = %s, want %s", after.ID, protectedID)
			}
		})
	}
}

// TestRunExecuteWithCompactReplacesDatabaseAndPreservesRollback proves the
// staged-compaction path: after retention deletes commit, --compact stages
// a VACUUM INTO copy, verifies it, and atomically swaps it into place while
// preserving the pre-compaction file as a named rollback artifact that
// itself passes an integrity check.
func TestRunExecuteWithCompactReplacesDatabaseAndPreservesRollback(t *testing.T) {
	f := newTestFixture(t)
	base := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)

	f.seedTask("task-compact")
	f.seedSession("task-compact", "session-compact")
	f.seedDuplicateGitSnapshots("session-compact", base)

	homeDir := t.TempDir()
	outcome, err := Run(context.Background(), homeDir, "sqlite", f.dbPath, RunOptions{Execute: true, Compact: true}, nil)
	if err != nil {
		t.Fatalf("Run(execute+compact): %v", err)
	}
	if outcome.Compaction == nil {
		t.Fatal("Compaction result is nil, want a populated CompactionResult")
	}
	if outcome.Compaction.RollbackPath == "" {
		t.Fatal("RollbackPath is empty")
	}
	if _, err := os.Stat(outcome.Compaction.RollbackPath); err != nil {
		t.Fatalf("rollback artifact missing at %s: %v", outcome.Compaction.RollbackPath, err)
	}
	if err := verifySQLiteIntegrity(outcome.Compaction.RollbackPath); err != nil {
		t.Fatalf("rollback artifact failed integrity check: %v", err)
	}
	if err := verifySQLiteIntegrity(f.dbPath); err != nil {
		t.Fatalf("post-compaction live database failed integrity check: %v", err)
	}

	// A fresh connection against the replaced file must still see the
	// post-retention row counts (compaction must not lose data).
	verifyRepo := openRepoForVerification(t, f.dbPath)
	var count int
	if err := verifyRepo.DB().QueryRow(`SELECT COUNT(*) FROM task_session_git_snapshots`).Scan(&count); err != nil {
		t.Fatalf("count git snapshots after compaction: %v", err)
	}
	if count != 1 {
		t.Fatalf("git snapshot rows after compaction = %d, want 1", count)
	}
}

// TestCompactRefusesWithInsufficientDiskSpace injects a diskUsageFunc
// reporting almost no free space and confirms compact refuses before
// touching the live database file at all.
func TestCompactRefusesWithInsufficientDiskSpace(t *testing.T) {
	f := newTestFixture(t)
	f.seedTask("task-disk")
	sizeBefore := f.fileSize()

	starvedDiskUsage := func(_ context.Context, _ string) (metrics.DiskCapacity, error) {
		return metrics.DiskCapacity{TotalBytes: 1024, AvailableBytes: 1, UsedBytes: 1023, UsedPercent: 99.9}, nil
	}
	_, err := compact(context.Background(), f.sqlxDB, f.closeForSwap, f.dbPath, starvedDiskUsage, fakeSnapshotSuccess)
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Fatalf("err = %v, want ErrInsufficientDiskSpace", err)
	}
	if got := f.fileSize(); got != sizeBefore {
		t.Fatalf("live database size changed after a disk-space refusal: before=%d after=%d", sizeBefore, got)
	}
}

// TestCompactAbortsWhenSnapshotFails injects a failing snapshot function
// (simulating a VACUUM INTO failure) and confirms the live database is left
// completely untouched and no rollback file is created.
func TestCompactAbortsWhenSnapshotFails(t *testing.T) {
	f := newTestFixture(t)
	f.seedTask("task-snapfail")
	sizeBefore := f.fileSize()

	_, err := compact(context.Background(), f.sqlxDB, f.closeForSwap, f.dbPath, alwaysEnoughDiskUsage, fakeSnapshotFailure)
	if err == nil {
		t.Fatal("expected an error from a failing snapshot function")
	}
	if got := f.fileSize(); got != sizeBefore {
		t.Fatalf("live database size changed after a snapshot failure: before=%d after=%d", sizeBefore, got)
	}
	rollbackMatches, globErr := filepath.Glob(f.dbPath + ".pre-compact-*")
	if globErr != nil {
		t.Fatalf("glob for rollback artifacts: %v", globErr)
	}
	if len(rollbackMatches) != 0 {
		t.Fatalf("no rollback artifact should exist after a snapshot failure, found %v", rollbackMatches)
	}
}

// TestCompactAbortsOnIntegrityCheckFailure injects a snapshot function that
// "succeeds" but writes corrupt (non-SQLite) bytes to the staged path,
// proving PRAGMA integrity_check catches the corruption before the swap and
// the live database is left untouched.
func TestCompactAbortsOnIntegrityCheckFailure(t *testing.T) {
	f := newTestFixture(t)
	f.seedTask("task-corrupt")
	sizeBefore := f.fileSize()

	_, err := compact(context.Background(), f.sqlxDB, f.closeForSwap, f.dbPath, alwaysEnoughDiskUsage, fakeSnapshotCorrupt)
	if !errors.Is(err, ErrIntegrityCheckFailed) {
		t.Fatalf("err = %v, want ErrIntegrityCheckFailed", err)
	}
	if got := f.fileSize(); got != sizeBefore {
		t.Fatalf("live database size changed after an integrity-check failure: before=%d after=%d", sizeBefore, got)
	}
}

func alwaysEnoughDiskUsage(_ context.Context, _ string) (metrics.DiskCapacity, error) {
	return metrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: 1 << 40, UsedBytes: 0, UsedPercent: 0}, nil
}

func fakeSnapshotSuccess(_ *sqlx.DB, path string) (int64, error) {
	if err := os.WriteFile(path, []byte("not a real database"), 0o600); err != nil {
		return 0, err
	}
	return 20, nil
}

func fakeSnapshotFailure(_ *sqlx.DB, _ string) (int64, error) {
	return 0, errors.New("simulated VACUUM INTO failure")
}

func fakeSnapshotCorrupt(_ *sqlx.DB, path string) (int64, error) {
	if err := os.WriteFile(path, []byte("this is not a valid sqlite file"), 0o600); err != nil {
		return 0, err
	}
	return 31, nil
}
