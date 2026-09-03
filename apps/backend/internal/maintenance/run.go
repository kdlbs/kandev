package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/backendapp/ownershiplock"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/persistence"
	"github.com/kandev/kandev/internal/system/metrics"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

// RunOptions configures one `kandev maintenance database` invocation.
type RunOptions struct {
	// Execute, when false (the default), performs a read-only Analyze and
	// returns without acquiring any lock, taking any backup, or deleting or
	// compacting anything.
	Execute bool
	// Compact additionally stages a VACUUM INTO compaction and atomic
	// replacement after retention deletes commit. Ignored when Execute is
	// false.
	Compact bool
	// KeepPlanRevisions is forwarded to AnalyzeOptions.KeepPlanRevisions.
	KeepPlanRevisions int
	// CandidateLimit is forwarded to AnalyzeOptions.CandidateLimit.
	CandidateLimit int
}

// Outcome is the full result of one Run call, covering both dry-run and
// execute modes; fields not applicable to the mode that ran are left at
// their zero value.
type Outcome struct {
	Executed   bool
	Report     Report
	BackupPath string
	Execution  ExecutionResult
	Compaction *CompactionResult
}

// Run performs one `kandev maintenance database` invocation against the
// SQLite database at databasePath. homeDir/databaseDriver/databasePath are
// the same triple internal/backendapp/ownershiplock.Targets uses at backend
// boot, so a live backend and a --execute maintenance run can never
// overlap. log receives only structural events (counts, paths, durations);
// no row content or identifier is ever logged.
func Run(ctx context.Context, homeDir, databaseDriver, databasePath string, opts RunOptions, log *logger.Logger) (Outcome, error) {
	if !strings.EqualFold(databaseDriver, "sqlite") {
		return Outcome{}, ErrUnsupportedDriver
	}

	owner, err := acquireExecuteLock(homeDir, databaseDriver, databasePath, opts.Execute)
	if err != nil {
		return Outcome{}, err
	}
	if owner != nil {
		defer func() { _ = owner.Close() }()
	}

	repo, writer, closeConnections, err := openMaintenanceConnections(databasePath, log)
	if err != nil {
		return Outcome{}, err
	}
	defer func() { _ = closeConnections() }()

	report, set, err := analyze(ctx, repo, AnalyzeOptions{KeepPlanRevisions: opts.KeepPlanRevisions, CandidateLimit: opts.CandidateLimit})
	if err != nil {
		return Outcome{}, fmt.Errorf("analyze retention candidates: %w", err)
	}
	if info, statErr := os.Stat(databasePath); statErr == nil {
		report.DatabaseSizeBytes = info.Size()
	}

	if !opts.Execute {
		return Outcome{Executed: false, Report: report}, nil
	}

	return runExecuteAndCompact(ctx, writer, closeConnections, databasePath, report, set, opts.Compact)
}

// acquireExecuteLock takes the same exclusive-access ownership lock a live
// backend takes at boot, but only when execute is true: a dry run must
// remain safely runnable concurrently with a live backend.
func acquireExecuteLock(homeDir, databaseDriver, databasePath string, execute bool) (*ownershiplock.Owner, error) {
	if !execute {
		return nil, nil
	}
	targets, err := ownershiplock.Targets(homeDir, databaseDriver, databasePath)
	if err != nil {
		return nil, fmt.Errorf("compute ownership lock targets: %w", err)
	}
	owner, err := ownershiplock.Acquire(targets)
	if err != nil {
		var conflict *ownershiplock.ConflictError
		if errors.As(err, &conflict) {
			return nil, fmt.Errorf("%w: %s", ErrOwnershipConflict, conflict.Error())
		}
		return nil, fmt.Errorf("acquire exclusive database access: %w", err)
	}
	return owner, nil
}

// openMaintenanceConnections opens the CLI's own raw writer/reader SQLite
// connections (deliberately bypassing persistence.Provide's boot-specific
// version-migration side effects) and wraps them in a task repository.
// The returned close func is idempotent and safe to call from a defer even
// after compact() has already closed the connections itself.
func openMaintenanceConnections(databasePath string, log *logger.Logger) (*sqlite.Repository, *sqlx.DB, func() error, error) {
	writerConn, err := db.OpenSQLite(databasePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open database for writing: %w", err)
	}
	readerConn, err := db.OpenSQLiteReader(databasePath)
	if err != nil {
		_ = writerConn.Close()
		return nil, nil, nil, fmt.Errorf("open database for reading: %w", err)
	}
	writer := sqlx.NewDb(writerConn, "sqlite3")
	reader := sqlx.NewDb(readerConn, "sqlite3")
	closed := false
	closeConnections := func() error {
		if closed {
			return nil
		}
		closed = true
		writerErr := writer.Close()
		readerErr := reader.Close()
		if writerErr != nil {
			return writerErr
		}
		return readerErr
	}

	repo, err := sqlite.NewWithDB(writer, reader, log)
	if err != nil {
		_ = closeConnections()
		return nil, nil, nil, fmt.Errorf("initialize task repository: %w", err)
	}
	return repo, writer, closeConnections, nil
}

// runExecuteAndCompact performs the --execute path: a fresh verified backup,
// the retention deletes, and (only when compactRequested) the staged
// VACUUM INTO compaction and atomic replacement. A compaction failure is
// reported as a partial-success Outcome (deletes already committed and are
// backed by backupPath) rather than a total abort.
func runExecuteAndCompact(ctx context.Context, writer *sqlx.DB, closeConnections func() error, databasePath string, report Report, set candidateSet, compactRequested bool) (Outcome, error) {
	backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
	backupPath, err := createVerifiedBackup(writer, backupDir)
	if err != nil {
		return Outcome{}, err
	}

	execResult, err := execute(ctx, writer.DB, set)
	if err != nil {
		return Outcome{}, fmt.Errorf("execute retention deletes: %w", err)
	}

	outcome := Outcome{Executed: true, Report: report, BackupPath: backupPath, Execution: execResult}
	if !compactRequested {
		return outcome, nil
	}

	compactionResult, err := compact(ctx, writer, closeConnections, databasePath, metrics.DiskUsage, persistence.SnapshotSQLite)
	if err != nil {
		// Retention deletes already committed and are backed by backupPath;
		// only the optional compaction step failed, so the run is reported
		// as a partial failure rather than a total abort.
		return outcome, fmt.Errorf("compact database: %w", err)
	}
	outcome.Compaction = &compactionResult
	return outcome, nil
}
