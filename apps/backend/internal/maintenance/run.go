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
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

const maintenanceDefaultDeliveryCandidateLimit = 200

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
	CandidateLimit    int
	DeliveryRetention models.AgentDeliveryRetentionBatchOptions
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

	var repo *sqlite.Repository
	var writer *sqlx.DB
	var reader *sqlx.DB
	var closeConnections func() error
	var backupPath string
	if opts.Execute {
		writer, reader, closeConnections, err = openMaintenanceRawConnections(databasePath)
		if err != nil {
			return Outcome{}, err
		}
		backupDir := filepath.Join(filepath.Dir(databasePath), "backups")
		backupPath, err = createVerifiedBackup(writer, backupDir)
		if err != nil {
			_ = closeConnections()
			return Outcome{}, err
		}
		repo, err = openMaintenanceRepository(writer, reader, log)
		if err != nil {
			_ = closeConnections()
			return Outcome{}, err
		}
	} else {
		repo, closeConnections, err = openMaintenanceReadOnlyConnection(databasePath, log)
	}
	if err != nil {
		return Outcome{}, err
	}
	defer func() { _ = closeConnections() }()

	report, set, err := analyze(ctx, repo, AnalyzeOptions{
		KeepPlanRevisions: opts.KeepPlanRevisions,
		CandidateLimit:    opts.CandidateLimit,
		DeliveryRetention: applyDeliveryCandidateLimit(opts.DeliveryRetention, opts.CandidateLimit),
	})
	if err != nil {
		return Outcome{}, fmt.Errorf("analyze retention candidates: %w", err)
	}
	if info, statErr := os.Stat(databasePath); statErr == nil {
		report.DatabaseSizeBytes = info.Size()
	}

	if !opts.Execute {
		return Outcome{Executed: false, Report: report}, nil
	}

	compactRun := func() (CompactionResult, error) {
		return compact(ctx, writer, closeConnections, databasePath, metrics.DiskUsage, persistence.SnapshotSQLite)
	}
	return runExecuteAndCompact(ctx, writer, closeConnections, databasePath, backupPath,
		repo, report, set, opts.DeliveryRetention, opts.CandidateLimit, opts.Compact, compactRun)
}

// openMaintenanceReadOnlyConnection opens an existing SQLite database in
// read-only mode and deliberately skips repository schema initialization. A
// missing database or schema that predates the retention queries therefore
// fails closed without creating, migrating, or healing anything.
func openMaintenanceReadOnlyConnection(databasePath string, log *logger.Logger) (*sqlite.Repository, func() error, error) {
	readerConn, err := db.OpenSQLiteReader(databasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open database for reading: %w", err)
	}
	reader := sqlx.NewDb(readerConn, "sqlite3")
	return sqlite.NewReadOnlyWithDB(reader, log), reader.Close, nil
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
func openMaintenanceRawConnections(databasePath string) (*sqlx.DB, *sqlx.DB, func() error, error) {
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
	return writer, reader, closeConnections, nil
}

func openMaintenanceRepository(writer, reader *sqlx.DB, log *logger.Logger) (*sqlite.Repository, error) {
	repo, err := sqlite.NewWithDB(writer, reader, log)
	if err != nil {
		return nil, fmt.Errorf("initialize task repository: %w", err)
	}
	return repo, nil
}

// runExecuteAndCompact performs the --execute path: a fresh verified backup,
// the retention deletes, and (only when compactRequested) the staged
// VACUUM INTO compaction and atomic replacement. A compaction failure is
// reported as a partial-success Outcome (deletes already committed and are
// backed by backupPath) rather than a total abort.
func runExecuteAndCompact(
	ctx context.Context,
	writer *sqlx.DB,
	closeConnections func() error,
	databasePath, backupPath string,
	repo *sqlite.Repository,
	report Report,
	set candidateSet,
	retentionOptions models.AgentDeliveryRetentionBatchOptions,
	candidateLimit int,
	compactRequested bool,
	compactRun func() (CompactionResult, error),
) (Outcome, error) {
	execResult, err := execute(ctx, writer.DB, set)
	if err != nil {
		return Outcome{}, fmt.Errorf("execute retention deletes: %w", err)
	}

	outcome := Outcome{Executed: true, Report: report, BackupPath: backupPath, Execution: execResult}
	retentionOptions = applyDeliveryCandidateLimit(retentionOptions, candidateLimit)
	retentionResult, err := executeDeliveryRetention(ctx, repo, retentionOptions)
	outcome.Execution.DeletedDeliveryEventPayloads += retentionResult.DeletedDeliveryEventPayloads
	outcome.Execution.DeletedSettledDeliveryPrompts += retentionResult.DeletedSettledDeliveryPrompts
	outcome.Execution.RetiredDeliveryEventReceipts += retentionResult.RetiredDeliveryEventReceipts
	outcome.Execution.PurgedRetiredDeliveryRows += retentionResult.PurgedRetiredDeliveryRows
	outcome.Execution.DeliveryOrphanJobsScheduled += retentionResult.DeliveryOrphanJobsScheduled
	outcome.Execution.DeliveryRetirementJobsCompleted += retentionResult.DeliveryRetirementJobsCompleted
	outcome.Execution.DeliveryBatches += retentionResult.DeliveryBatches
	outcome.Execution.Partial = retentionResult.Partial || deliveryReportWasLimited(report)
	if err != nil {
		outcome.Execution.Partial = true
		return outcome, fmt.Errorf("execute bounded delivery retention: %w", err)
	}
	if !compactRequested {
		return outcome, nil
	}

	compactionResult, err := compactRun()
	if err != nil {
		// Retention deletes already committed and are backed by backupPath;
		// only the optional compaction step failed, so the run is reported
		// as a partial failure rather than a total abort.
		return outcome, fmt.Errorf("compact database: %w", err)
	}
	outcome.Compaction = &compactionResult
	return outcome, nil
}

func applyDeliveryCandidateLimit(options models.AgentDeliveryRetentionBatchOptions, candidateLimit int) models.AgentDeliveryRetentionBatchOptions {
	if candidateLimit > 0 && (options.MaxCandidates <= 0 || candidateLimit < options.MaxCandidates) {
		options.MaxCandidates = candidateLimit
	}
	return options
}

func executeDeliveryRetention(
	ctx context.Context,
	repo *sqlite.Repository,
	options models.AgentDeliveryRetentionBatchOptions,
) (ExecutionResult, error) {
	var execution ExecutionResult
	result, err := repo.RunAgentDeliveryRetentionBatch(ctx, options)
	if err != nil {
		return execution, err
	}
	execution.DeliveryBatches++
	execution.DeletedDeliveryEventPayloads = result.EventPayloadsPruned
	execution.DeletedSettledDeliveryPrompts = result.PromptPayloadsPruned
	execution.RetiredDeliveryEventReceipts = result.EventRowsRetired
	execution.Partial = result.Candidates >= deliveryCandidateLimit(options.MaxCandidates)
	orphans, err := repo.RunAgentDeliveryOrphanBatch(ctx, models.AgentDeliveryOrphanBatchOptions{MaxCandidates: options.MaxCandidates})
	if err != nil {
		execution.Partial = execution.DeliveryBatches > 0
		return execution, err
	}
	execution.DeliveryBatches++
	execution.DeliveryOrphanJobsScheduled = orphans.JobsScheduled
	execution.Partial = execution.Partial || orphans.Candidates >= deliveryCandidateLimit(options.MaxCandidates)
	retirement, err := repo.RunAgentDeliveryRetirementBatch(ctx, models.AgentDeliveryRetirementBatchOptions{MaxCandidates: options.MaxCandidates})
	if err != nil {
		execution.Partial = execution.DeliveryBatches > 0
		return execution, err
	}
	execution.DeliveryBatches++
	execution.PurgedRetiredDeliveryRows = retirement.RowsPurged
	execution.DeliveryRetirementJobsCompleted = retirement.Completed
	execution.Partial = execution.Partial || retirement.Pending || retirement.Suspended > 0 ||
		retirement.RowsPurged >= deliveryCandidateLimit(options.MaxCandidates)
	return execution, nil
}

func deliveryCandidateLimit(limit int) int {
	if limit <= 0 || limit > maintenanceDefaultDeliveryCandidateLimit {
		return maintenanceDefaultDeliveryCandidateLimit
	}
	return limit
}

func deliveryReportWasLimited(report Report) bool {
	for _, group := range []CandidateGroup{
		report.DuplicateGitSnapshots, report.ObsoletePlanRevisions, report.OrphanedMessagePayloads,
		report.ProjectedDeliveryPayloads, report.SettledDeliveryPrompts,
		report.RetiredDeliveryEventReceipts, report.RetiredDeliverySessionEvidence,
		report.OrphanedDeliveryIdentities,
	} {
		if group.SampleLimitReached {
			return true
		}
	}
	return false
}
