package dataretention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

const (
	batchRows       = 100
	batchBytes      = 8 * 1024 * 1024
	statementBudget = 200 * time.Millisecond
)

// Skip reasons. The reducer's own reasons are recorded verbatim so a pass
// reports the same vocabulary as the existing tool payload policy.
const (
	skipProtectedTasks    = "protected_tasks"
	skipEligibilityBudget = "eligibility_budget"
)

var (
	errDatabaseChanged = errors.New("database_changed")
	errWriterRequired  = errors.New("writer_required")
)

func scanGet(ctx context.Context, q sqlx.QueryerContext, dest any, query string, args ...any) error {
	stmt, cancel := context.WithTimeout(ctx, statementBudget)
	defer cancel()
	return sqlx.GetContext(stmt, q, dest, query, args...)
}

// scannedRows is the rows both targets have already charged to this batch. The
// row budget is shared, so the message and job phases read it together.
func scannedRows(r *record) int64 {
	if r.Operation == nil {
		return 0
	}
	return r.Operation.Messages.Rows + r.Operation.CleanupJobs.Rows
}

func (s *Service) stepScan(ctx context.Context, r record) error {
	if r.Operation == nil || r.Operation.State != stateRunning || r.Operation.Kind == choiceBackup {
		return nil
	}
	var err error
	if r.Operation.Kind == kindCleanup {
		err = s.stepCleanup(ctx, r.Operation.ID)
	} else {
		err = s.stepAnalysis(ctx, r)
	}
	if errors.Is(err, errDatabaseChanged) {
		return s.stopChangedScan(ctx, r.Operation.ID)
	}
	return err
}

// stepAnalysis measures the same candidates a cleanup pass would touch without
// writing anything, so the settings surface can show the effect before it runs.
func (s *Service) stepAnalysis(ctx context.Context, r record) error {
	current, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	if !activeOperation(&current, r.Operation.ID) {
		// A cancel committed after the tick read; do not start a new batch.
		return nil
	}
	if _, err := s.scanBatch(ctx, s.pool.Reader(), &r, false); err != nil {
		return err
	}
	return s.storeScanProgress(ctx, r)
}

func (s *Service) storeScanProgress(ctx context.Context, r record) error {
	_, err := s.change(ctx, func(current *record, tx *sqlx.Tx) error {
		if !operationMatches(current, r.Operation.ID) {
			return nil
		}
		if current.Operation.State != stateRunning {
			// A cancel or another writer settled this row after the tick read.
			return nil
		}
		if r.Operation.Kind == kindCleanup && (!approved(current) || current.Policy.Revision != r.Progress.Revision) {
			return errors.New("conflict")
		}
		if err := checkScanSchema(ctx, tx, current); err != nil {
			return err
		}
		current.Operation = r.Operation
		current.Progress = r.Progress
		if r.Operation.State != stateRunning {
			finishOperation(current, r.Operation.State, r.Operation.Error, s.opts.Now())
			scheduleNextPass(current, s.opts.Now())
		}
		return nil
	})
	return err
}

// scanBatch runs one bounded unit of work. Both targets share the row and byte
// budget so a pass cannot spend its whole budget on transcripts and skip the
// cleanup snapshots, or the reverse.
func (s *Service) scanBatch(ctx context.Context, q sqlx.QueryerContext, r *record, mutate bool) ([]string, error) {
	if err := checkScanSchema(ctx, q, r); err != nil {
		return nil, err
	}
	used := int64(0)
	// rows is shared by both phases, so the batch honours one 100-row budget.
	rows := int64(0)
	changed, err := s.scanMessageBatch(ctx, q, r, mutate, &used, &rows)
	if err != nil {
		return nil, err
	}
	if used < batchBytes && r.Progress.Phase == phaseJobs {
		if _, err := s.scanJobBatch(ctx, q, r, mutate, &used, &rows); err != nil {
			return nil, err
		}
	}
	return changed, checkScanSchema(ctx, q, r)
}

func (s *Service) scanMessageBatch(ctx context.Context, q sqlx.QueryerContext, r *record, mutate bool, used, rows *int64) ([]string, error) {
	changed := []string{}
	// Re-validate the open task at the start of every batch; a task can become
	// ineligible between batches, and the batch must not keep crediting it.
	validated := ""
	started := time.Now()
	for i := 0; i < batchRows && *used < batchBytes; i++ {
		if i > 0 && time.Since(started) >= statementBudget {
			break
		}
		remaining := int64(batchRows) - *rows
		if remaining <= 0 {
			break
		}
		ready, err := s.advanceMessageCursor(ctx, q, r, &validated)
		if err != nil {
			return nil, err
		}
		if r.Progress.Phase != phaseMessages {
			return changed, nil
		}
		if !ready {
			continue
		}
		before := scannedRows(r)
		full, err := s.consumeMessagePage(ctx, q, r, mutate, used, &changed, int(remaining))
		if err != nil {
			return nil, err
		}
		*rows += scannedRows(r) - before
		if full {
			break
		}
	}
	return changed, nil
}

// advanceMessageCursor resolves the next candidate task, session and message
// page to read. A candidate is a finished or archived task. It reports false
// when it skipped a task or session instead of producing a page, and moves the
// pass to the cleanup phase once no candidate remains below the captured upper
// cursor.
func (s *Service) advanceMessageCursor(ctx context.Context, q sqlx.QueryerContext, r *record, validated *string) (bool, error) {
	p := &r.Progress
	if p.Task == "" {
		id, err := s.nextHistoryTask(ctx, q, p.TaskAfter, p.UpperTask)
		if err != nil {
			return false, err
		}
		if id == "" {
			finishMessageTask(p)
			p.Phase = phaseJobs
			return false, nil
		}
		p.Task = id
	}
	if *validated != p.Task {
		eligible, err := s.historyTaskEligible(ctx, q, r, p.Task)
		if err != nil {
			return false, err
		}
		if !eligible {
			finishMessageTask(p)
			return false, nil
		}
		*validated = p.Task
	}
	if p.Session == "" {
		return false, s.openMessageSession(ctx, q, r)
	}
	return true, nil
}

func (s *Service) nextHistoryTask(ctx context.Context, q sqlx.QueryerContext, after, upper string) (string, error) {
	id, err := tasksqlite.NextHistoryTaskID(ctx, q, after, upper)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Service) historyTaskEligible(ctx context.Context, q sqlx.QueryerContext, r *record, taskID string) (bool, error) {
	eligible, err := tasksqlite.HistoryTaskEligible(ctx, q, s.driverName(), taskID, r.Operation.ArchivedCutoff)
	if errors.Is(err, tasksqlite.ErrArchivedTaskEligibilityBudget) || errors.Is(err, tasksqlite.ErrPayloadEligibilityBudget) {
		r.Operation.Messages.Skipped[skipEligibilityBudget]++
		finishMessageTask(&r.Progress)
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !eligible {
		r.Operation.Messages.Skipped[skipProtectedTasks]++
		finishMessageTask(&r.Progress)
	}
	return eligible, nil
}

func (s *Service) openMessageSession(ctx context.Context, q sqlx.QueryerContext, r *record) error {
	p := &r.Progress
	id, err := tasksqlite.NextSessionID(ctx, q, p.Task, p.SessionAfter)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil || id == "" {
		finishMessageTask(p)
		return nil
	}
	upper, err := tasksqlite.SessionUpperMessageRowID(ctx, q, id)
	if err != nil {
		return err
	}
	p.Session = id
	p.UpperMessage = upper
	return nil
}

// consumeMessagePage reduces one bounded page of an archived session. The page
// is stopped before a row that would exceed the batch byte budget, so the row is
// left for the next batch with its cursor intact.
func (s *Service) consumeMessagePage(ctx context.Context, q sqlx.QueryerContext, r *record, mutate bool, used *int64, changed *[]string, limit int) (bool, error) {
	p := &r.Progress
	rows, err := tasksqlite.NextArchivedMessageRows(ctx, q, p.Session, p.Message, p.UpperMessage, limit)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		finishSession(p)
		return false, nil
	}
	for _, row := range rows {
		if row.Bytes <= models.ToolPayloadMaxBytes && *used > 0 && row.Bytes > batchBytes-*used {
			return true, nil
		}
		p.Message = row.RowID
		target := &r.Operation.Messages
		target.Rows++
		target.TotalBytes += row.Bytes
		// Eligibility was decided for the task being walked; a row stored under
		// another task, or one the agent did not author, is never reduced.
		if row.TaskID != p.Task || row.AuthorType != "agent" {
			r.Operation.Messages.Skipped["invalid_message"]++
			continue
		}
		if row.Bytes > models.ToolPayloadMaxBytes {
			target.Oversized++
			r.Operation.Messages.Skipped["oversize"]++
			continue
		}
		*used += row.Bytes
		id, err := s.reduceMessage(ctx, q, r, row, mutate)
		if err != nil {
			return false, err
		}
		if id != "" {
			*changed = append(*changed, id)
		}
	}
	return false, nil
}

// reduceMessage shares the tool payload reducer with the existing policy, so
// both paths write the identical removal marker and the second is a no-op on a
// row the first already handled. Eligibility is re-read inside the writer
// transaction so a task that becomes active mid-pass keeps its payload.
func (s *Service) reduceMessage(ctx context.Context, q sqlx.QueryerContext, r *record, row tasksqlite.ArchivedMessageRow, mutate bool) (string, error) {
	raw, err := tasksqlite.ArchivedMessageMetadata(ctx, q, row.RowID, row.ID)
	if err != nil {
		return "", err
	}
	reduced, err := models.ReduceToolPayload(row.Type, []byte(raw), r.Operation.StartedAt)
	if err != nil {
		return "", err
	}
	if reduced.Reason != "" {
		r.Operation.Messages.Skipped[reduced.Reason]++
		return "", nil
	}
	target := &r.Operation.Messages
	target.Eligible++
	target.Bytes += reduced.RemovedBytes
	if !mutate {
		// A dry run still reports the candidate so the caller can tell "there is
		// work to gate on" from "nothing matched in this batch".
		return row.ID, nil
	}
	tx, ok := q.(*sqlx.Tx)
	if !ok {
		return "", errWriterRequired
	}
	eligible, err := tasksqlite.HistoryTaskEligible(ctx, tx, s.driverName(), r.Progress.Task, r.Operation.ArchivedCutoff)
	if err != nil {
		if errors.Is(err, tasksqlite.ErrArchivedTaskEligibilityBudget) || errors.Is(err, tasksqlite.ErrPayloadEligibilityBudget) {
			// A budget timeout is a skip, not a failed batch: the row stays and
			// the pass ends partial.
			r.Operation.Messages.Skipped[skipEligibilityBudget]++
			return "", nil
		}
		return "", err
	}
	if !eligible {
		return "", nil
	}
	if err := tasksqlite.WriteReducedMessageMetadata(ctx, tx, row.ID, raw, string(reduced.Metadata)); err != nil {
		return "", err
	}
	// Incremental shell output lives in its own chunk rows; a reduced payload
	// must drop them in the same transaction, as the tool payload policy does.
	var metadata map[string]any
	if json.Unmarshal([]byte(raw), &metadata) == nil && models.HasIncrementalShellOutput(metadata) {
		if err := tasksqlite.RemoveMessageShellOutputTx(ctx, tx, row.ID); err != nil {
			return "", err
		}
	}
	target.Reduced++
	r.FirstMutation = true
	return row.ID, nil
}

// scanJobBatch reduces finished cleanup-job snapshots. It reports no changed
// message ids: the Changed hook only refreshes message payloads in the UI, and a
// reduced job snapshot has no reader that needs a notification.
func (s *Service) scanJobBatch(ctx context.Context, q sqlx.QueryerContext, r *record, mutate bool, used, rows *int64) (bool, error) {
	started := time.Now()
	for i := 0; i < batchRows && *used < batchBytes; i++ {
		if i > 0 && time.Since(started) >= statementBudget {
			break
		}
		if *rows >= int64(batchRows) {
			break
		}
		p := &r.Progress
		if p.Job == "" {
			id, err := tasksqlite.NextSucceededCleanupJobID(ctx, q, p.JobAfter, p.UpperJob, r.Operation.CleanupCutoff)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return false, err
			}
			if err != nil || id == "" {
				completeOperation(r, s.opts.Now())
				return false, nil
			}
			p.Job = id
		}
		before := scannedRows(r)
		full, err := s.reduceCleanupJob(ctx, q, r, mutate, used)
		if err != nil {
			return false, err
		}
		*rows += scannedRows(r) - before
		if full {
			break
		}
	}
	return false, nil
}

// reduceCleanupJob keeps only the archive source manifest, the one field a
// succeeded job still serves through GetTaskSourceManifest after task deletion.
func (s *Service) reduceCleanupJob(ctx context.Context, q sqlx.QueryerContext, r *record, mutate bool, used *int64) (bool, error) {
	jobID := r.Progress.Job
	bytes, err := tasksqlite.CleanupJobSnapshotBytes(ctx, q, jobID)
	if err != nil {
		return false, err
	}
	target := &r.Operation.CleanupJobs
	// Only the size is read yet; a row deferred by the byte budget keeps its
	// cursor and must not be counted until it is actually processed.
	if bytes > models.ToolPayloadMaxBytes {
		target.Rows++
		target.TotalBytes += bytes
		target.Oversized++
		r.Operation.CleanupJobs.Skipped["oversize"]++
		finishCleanupJob(&r.Progress)
		return false, nil
	}
	if bytes > 0 && *used > 0 && bytes > batchBytes-*used {
		return true, nil
	}
	// The row counts as scanned and is charged to the shared byte budget before
	// the body is fetched, so a timeout or ineligible row cannot grow the batch.
	target.Rows++
	target.TotalBytes += bytes
	*used += bytes
	snapshot, err := tasksqlite.CleanupJobSnapshotBody(ctx, q, jobID)
	if err != nil {
		return false, err
	}
	eligible, err := tasksqlite.SucceededCleanupJobEligible(ctx, q, s.driverName(), jobID, r.Operation.CleanupCutoff)
	if err != nil {
		if errors.Is(err, tasksqlite.ErrCleanupSnapshotEligibilityBudget) {
			r.Operation.CleanupJobs.Skipped[skipEligibilityBudget]++
			finishCleanupJob(&r.Progress)
			return false, nil
		}
		return false, err
	}
	if !eligible {
		r.Operation.CleanupJobs.Skipped[skipProtectedTasks]++
		finishCleanupJob(&r.Progress)
		return false, nil
	}
	reduced, err := models.ReduceCleanupSnapshot(snapshot, r.Operation.StartedAt)
	if err != nil {
		return false, err
	}
	finishCleanupJob(&r.Progress)
	if reduced.Reason != "" {
		r.Operation.CleanupJobs.Skipped[reduced.Reason]++
		return false, nil
	}
	target.Eligible++
	target.Bytes += reduced.RemovedBytes
	if !mutate {
		return false, nil
	}
	tx, ok := q.(*sqlx.Tx)
	if !ok {
		return false, errWriterRequired
	}
	if err := tasksqlite.WriteReducedCleanupSnapshot(ctx, tx, jobID, snapshot, reduced.Snapshot); err != nil {
		return false, err
	}
	target.Reduced++
	r.FirstMutation = true
	return false, nil
}

// checkScanSchema ends the pass when the schema changed under it: the captured
// upper cursors and row identities no longer describe the same database.
func checkScanSchema(ctx context.Context, q sqlx.QueryerContext, r *record) error {
	var version int
	if err := scanGet(ctx, q, &version, `PRAGMA schema_version`); err != nil {
		return err
	}
	if version != r.Progress.SchemaVersion {
		return errDatabaseChanged
	}
	return nil
}

func (s *Service) stopChangedScan(ctx context.Context, id string) error {
	_, err := s.change(ctx, func(r *record, tx *sqlx.Tx) error {
		if !activeOperation(r, id) {
			return nil
		}
		finishOperation(r, statePartial, errDatabaseChanged.Error(), s.opts.Now())
		scheduleNextPass(r, s.opts.Now())
		return nil
	})
	return err
}

// completeOperation closes the pass. A skipped eligibility budget is reported as
// partial, so an unbounded decision is never presented as a complete removal.
func completeOperation(r *record, now time.Time) {
	state, code := stateSucceeded, ""
	if r.Operation.Messages.Skipped[skipEligibilityBudget] > 0 || r.Operation.CleanupJobs.Skipped[skipEligibilityBudget] > 0 {
		state, code = statePartial, skipEligibilityBudget
	}
	finishOperation(r, state, code, now)
	r.Operation.Complete = true
	r.Operation.CompletedTarget = r.Progress.Phase
}

func scheduleNextPass(r *record, now time.Time) {
	if r.Operation == nil || r.Operation.Kind != kindCleanup || !r.Policy.Enabled {
		return
	}
	next := now.Add(24 * time.Hour)
	r.NextDueAt = &next
}

func finishSession(p *progress) {
	p.SessionAfter = p.Session
	p.Session = ""
	p.Message = 0
	p.UpperMessage = 0
}

func finishCleanupJob(p *progress) {
	p.JobAfter = p.Job
	p.Job = ""
}

// finishMessageTask retires the current task. The task cursor only advances
// when a task was actually open: assigning an empty Task back onto TaskAfter
// would rewind the cursor onto an already-skipped task, so a scan would re-select
// it forever and never reach the cleanup-job phase.
func finishMessageTask(p *progress) {
	if p.Task != "" {
		p.TaskAfter = p.Task
	}
	p.Task = ""
	p.Session = ""
	p.SessionAfter = ""
	p.Message = 0
	p.UpperMessage = 0
}
