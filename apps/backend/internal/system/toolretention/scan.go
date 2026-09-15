package toolretention

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"time"
)

const batchRows = 100
const batchBytes = 8 * 1024 * 1024
const statementBudget = 200 * time.Millisecond

var errDatabaseChanged = errors.New("database_changed")

func (s *Service) stepScan(ctx context.Context) error {
	r, err := readRecord(ctx, s.pool.Reader())
	if err != nil {
		return err
	}
	if r.Operation == nil || r.Operation.State != stateRunning || r.Operation.Kind == choiceBackup {
		return nil
	}
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

func (s *Service) stepAnalysis(ctx context.Context, r record) error {
	_, err := s.scanBatch(ctx, s.pool.Reader(), &r, false)
	if err != nil {
		return err
	}
	return s.storeScanProgress(ctx, r)
}

func (s *Service) storeScanProgress(ctx context.Context, r record) error {
	_, err := s.change(ctx, func(current *record, tx *sqlx.Tx) error {
		if !operationMatches(current, r.Operation.ID) {
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
			if r.Operation.Kind == kindCleanup {
				next := s.opts.Now().Add(24 * time.Hour)
				current.NextDueAt = &next
			}
		}
		return nil
	})
	return err
}

func scanGet(ctx context.Context, q sqlx.QueryerContext, dest any, query string, args ...any) error {
	stmt, cancel := context.WithTimeout(ctx, statementBudget)
	defer cancel()
	return sqlx.GetContext(stmt, q, dest, query, args...)
}

func (s *Service) scanBatch(ctx context.Context, q sqlx.QueryerContext, r *record, mutate bool) ([]string, error) {
	if err := checkScanSchema(ctx, q, r); err != nil {
		return nil, err
	}
	var changed []string
	used := int64(0)
	validatedTask := ""
	started := time.Now()
	for i := 0; i < batchRows && used < batchBytes; i++ {
		if i > 0 && time.Since(started) >= statementBudget {
			break
		}
		ready, done, err := s.advanceCursor(ctx, q, r, &validatedTask)
		if err != nil {
			return nil, err
		}
		if done {
			state, code := stateSucceeded, ""
			if r.Operation.Skipped["eligibility_budget"] > 0 {
				state, code = statePartial, "eligibility_budget"
			}
			finishOperation(r, state, code, s.opts.Now())
			break
		}
		if !ready {
			continue
		}
		id, full, err := s.consumeRow(ctx, q, r, &used, mutate)
		if err != nil {
			return nil, err
		}
		if full {
			break
		}
		if id != "" {
			changed = append(changed, id)
		}
	}
	return changed, checkScanSchema(ctx, q, r)
}

func (s *Service) consumeRow(ctx context.Context, q sqlx.QueryerContext, r *record, used *int64, mutate bool) (string, bool, error) {
	var row payloadRow
	err := scanGet(ctx, q, &row, `SELECT rowid AS row_id,id,type,COALESCE(length(CAST(metadata AS BLOB)),0) AS bytes,
	 COALESCE(julianday(created_at) IS NOT NULL AND julianday(updated_at) IS NOT NULL AND task_id=?,0) AS valid
	 FROM task_session_messages WHERE task_session_id=? AND rowid>? AND rowid<=? ORDER BY rowid LIMIT 1`, r.Progress.Task, r.Progress.Session, r.Progress.Message, r.Progress.UpperMessage)
	if errors.Is(err, sql.ErrNoRows) {
		finishSession(&r.Progress)
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if rowNeedsNextBatch(row, *used) {
		return "", true, nil
	}
	r.Progress.Message = row.RowID
	r.Operation.Scanned++
	if !row.Valid {
		r.Operation.Skipped["invalid_message"]++
		return "", false, nil
	}
	if row.Bytes > models.ToolPayloadMaxBytes {
		r.Operation.Skipped["oversize"]++
		return "", false, nil
	}
	*used += row.Bytes
	id, err := s.reduceRow(ctx, q, r, row, mutate)
	return id, false, err
}

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
		if !operationMatches(r, id) {
			return nil
		}
		finishOperation(r, statePartial, errDatabaseChanged.Error(), s.opts.Now())
		if r.Policy.Enabled && r.Operation.Kind == kindCleanup {
			next := s.opts.Now().Add(24 * time.Hour)
			r.NextDueAt = &next
		}
		return nil
	})
	return err
}

func rowNeedsNextBatch(row payloadRow, used int64) bool {
	return row.Valid && row.Bytes <= models.ToolPayloadMaxBytes && !rowFitsBatch(used, row.Bytes)
}

type payloadRow struct {
	RowID int64  `db:"row_id"`
	ID    string `db:"id"`
	Type  string `db:"type"`
	Bytes int64  `db:"bytes"`
	Valid bool   `db:"valid"`
}

func (s *Service) reduceRow(ctx context.Context, q sqlx.QueryerContext, r *record, row payloadRow, mutate bool) (string, error) {
	var raw string
	if err := scanGet(ctx, q, &raw, `SELECT COALESCE(metadata,'') FROM task_session_messages WHERE rowid=? AND id=?`, row.RowID, row.ID); err != nil {
		return "", err
	}
	reduced, err := models.ReduceToolPayload(row.Type, []byte(raw), r.Operation.StartedAt)
	if err != nil {
		return "", err
	}
	if reduced.Reason != "" {
		r.Operation.Skipped[reduced.Reason]++
		return "", nil
	}
	if mutate {
		tx, ok := q.(*sqlx.Tx)
		if !ok {
			return "", errors.New("writer_required")
		}
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_messages SET metadata=? WHERE id=? AND metadata=?`), string(reduced.Metadata), row.ID, raw)
		if err != nil {
			return "", err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return "", err
		}
		if n != 1 {
			return "", errors.New("message_conflict")
		}
		r.Operation.RemovedMessages++
		r.FirstMutation = true
	}
	r.Operation.EligibleMessages++
	r.Operation.PayloadBytes += reduced.RemovedBytes
	if !r.Progress.TaskCounted {
		r.Operation.EligibleTasks++
		r.Progress.TaskCounted = true
	}
	return row.ID, nil
}

func (s *Service) advanceCursor(ctx context.Context, q sqlx.QueryerContext, r *record, validatedTask *string) (bool, bool, error) {
	p := &r.Progress
	if p.Task == "" {
		err := scanGet(ctx, q, &p.Task, `SELECT id FROM tasks WHERE id>? AND id<=? ORDER BY id LIMIT 1`, p.TaskAfter, p.UpperTask)
		if errors.Is(err, sql.ErrNoRows) {
			return false, true, nil
		}
		if err != nil {
			return false, false, err
		}
	}
	if *validatedTask != p.Task {
		check, cancel := context.WithTimeout(ctx, statementBudget)
		eligible, err := tasksqlite.PayloadTaskEligible(check, q, p.Task, r.Operation.Cutoff)
		cancel()
		if errors.Is(err, tasksqlite.ErrPayloadEligibilityBudget) {
			r.Operation.Skipped["eligibility_budget"]++
			finishTask(p)
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		if !eligible {
			r.Operation.Skipped["protected_tasks"]++
			finishTask(p)
			return false, false, nil
		}
		*validatedTask = p.Task
	}
	var err error
	if p.Session == "" {
		err = scanGet(ctx, q, &p.Session, `SELECT id FROM task_sessions WHERE task_id=? AND id>? ORDER BY id LIMIT 1`, p.Task, p.SessionAfter)
		if errors.Is(err, sql.ErrNoRows) {
			finishTask(p)
			return false, false, nil
		}
		if err != nil {
			return false, false, err
		}
		err = scanGet(ctx, q, &p.UpperMessage, `SELECT COALESCE(MAX(rowid),0) FROM task_session_messages WHERE task_session_id=?`, p.Session)
	}
	return true, false, err
}

func finishSession(p *progress) {
	p.SessionAfter = p.Session
	p.Session = ""
	p.Message = 0
	p.UpperMessage = 0
}
func finishTask(p *progress) {
	p.TaskAfter = p.Task
	p.Task = ""
	p.Session = ""
	p.SessionAfter = ""
	p.Message = 0
	p.UpperMessage = 0
	p.TaskCounted = false
}
