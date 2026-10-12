package automation

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

var ErrRetryLaunchUncertain = errors.New("retry launch ownership or acceptance is unresolved; automatic relaunch is blocked")

const (
	RetryLaunchPrepared    = "prepared"
	RetryLaunchPreparing   = "preparing"
	RetryLaunchQueued      = "queued"
	RetryLaunchDispatching = "dispatching"
	RetryLaunchAccepted    = "accepted"
	RetryLaunchUncertain   = "uncertain"
	RetryLaunchStopped     = "stopped"
)

const retryLaunchTablesSQL = `
 CREATE TABLE IF NOT EXISTS automation_retry_launch_protocols (
 run_id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL DEFAULT '',
  version INTEGER NOT NULL
 );
 CREATE TABLE IF NOT EXISTS automation_retry_launches (
  run_id TEXT PRIMARY KEY,
  automation_id TEXT NOT NULL,
  group_generation BIGINT NOT NULL,
  task_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  submission_id TEXT NOT NULL,
  state TEXT NOT NULL,
  token TEXT NOT NULL DEFAULT '',
  execution_id TEXT NOT NULL DEFAULT '',
  cancel_requested BOOLEAN NOT NULL DEFAULT FALSE,
  created_at {{timestamp}} NOT NULL,
  updated_at {{timestamp}} NOT NULL
 );
 CREATE INDEX IF NOT EXISTS idx_automation_retry_launches_owner ON automation_retry_launches(automation_id, state);
`

// RetryLaunch owns one immutable launch tuple independently of task creation.
// It has no expiry and survives automation deletion; uncertainty is not replay permission.
type RetryLaunch struct {
	RunID           string    `db:"run_id" json:"run_id"`
	AutomationID    string    `db:"automation_id" json:"automation_id"`
	Generation      int64     `db:"group_generation" json:"group_generation"`
	TaskID          string    `db:"task_id" json:"task_id"`
	SessionID       string    `db:"session_id" json:"session_id"`
	TurnID          string    `db:"turn_id" json:"turn_id"`
	SubmissionID    string    `db:"submission_id" json:"submission_id"`
	State           string    `db:"state" json:"state"`
	Token           string    `db:"token" json:"token"`
	ExecutionID     string    `db:"execution_id" json:"execution_id"`
	CancelRequested bool      `db:"cancel_requested" json:"cancel_requested"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

const retryLaunchColumns = `run_id, automation_id, group_generation, task_id, session_id, turn_id,
 submission_id, state, token, execution_id, cancel_requested, created_at, updated_at`

func (s *Store) GetRetryLaunch(ctx context.Context, runID string) (*RetryLaunch, error) {
	var launch RetryLaunch
	if err := s.ro.GetContext(ctx, &launch, s.ro.Rebind(`SELECT `+retryLaunchColumns+` FROM automation_retry_launches WHERE run_id = ?`), runID); err != nil {
		return nil, err
	}
	return &launch, nil
}

func lockRetryAutomation(ctx context.Context, tx *sqlx.Tx, automationID string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET id = id WHERE id = ?`), automationID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

// ReserveRetryLaunch projects the reserved tuple before any external launch.
// The task_created status also prevents legacy dispatchers from starting this run.
func (s *Store) ReserveRetryLaunch(ctx context.Context, runID, taskID, sessionID string, action ThreadAction, reason string) (*RetryLaunch, error) {
	if !s.launchIdentityGuards {
		return nil, ErrRetryLaunchUncertain
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockRetryRunOwner(ctx, tx, runID); err != nil {
		return nil, err
	}
	var run AutomationRun
	if err := tx.GetContext(ctx, &run, tx.Rebind(`SELECT * FROM automation_runs WHERE id = ?`), runID); err != nil {
		return nil, err
	}
	if err := requireRetryLaunchProtocol(ctx, tx, runID); err != nil {
		if errors.Is(err, ErrRetryLaunchUncertain) {
			return nil, errors.Join(err, tx.Commit())
		}
		return nil, err
	}
	existing, err := loadExistingRetryLaunch(ctx, tx, &run, taskID, sessionID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, tx.Commit()
	}
	launch, err := reserveRetryLaunchTuple(ctx, tx, &run, taskID, sessionID, action, reason)
	if err != nil {
		if errors.Is(err, ErrRetryLaunchUncertain) {
			if markErr := markRetryReservationUncertain(ctx, tx, runID); markErr != nil {
				return nil, errors.Join(err, markErr)
			}
			return nil, errors.Join(err, tx.Commit())
		}
		return nil, err
	}
	return launch, tx.Commit()
}

func lockRetryRunOwner(ctx context.Context, tx *sqlx.Tx, runID string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET id = id WHERE id = (SELECT automation_id FROM automation_runs WHERE id = ?)`), runID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

func requireRetryLaunchProtocol(ctx context.Context, tx *sqlx.Tx, runID string) error {
	var protocol int
	if err := tx.GetContext(ctx, &protocol, tx.Rebind(`SELECT COUNT(*) FROM automation_retry_launch_protocols WHERE run_id = ? AND version = 1`), runID); err != nil {
		return err
	}
	if protocol == 1 {
		return nil
	}
	if err := markRetryReservationUncertain(ctx, tx, runID); err != nil {
		return err
	}
	return ErrRetryLaunchUncertain
}

func markRetryReservationUncertain(ctx context.Context, tx *sqlx.Tx, runID string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_launch_protocols SET version = 0 WHERE run_id = ? AND version = 1`), runID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, error_message = ? WHERE id = ? AND status IN (?, ?)`), RunStatusTaskCreated, ErrRetryLaunchUncertain.Error(), runID, RunStatusTriggered, RunStatusTaskCreated)
	return err
}

func loadExistingRetryLaunch(ctx context.Context, tx *sqlx.Tx, run *AutomationRun, taskID, sessionID string) (*RetryLaunch, error) {
	var existing RetryLaunch
	err := tx.GetContext(ctx, &existing, tx.Rebind(`SELECT `+retryLaunchColumns+` FROM automation_retry_launches WHERE run_id = ?`), run.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if existing.Generation != run.RetryGroupGeneration || existing.TaskID != taskID || (sessionID != "" && existing.SessionID != sessionID) {
		return nil, ErrRetryGenerationMismatch
	}
	return &existing, nil
}

func reserveRetryLaunchTuple(ctx context.Context, tx *sqlx.Tx, run *AutomationRun, taskID, sessionID string, action ThreadAction, reason string) (*RetryLaunch, error) {
	runID := run.ID
	if taskID == "" || run.TaskID != taskID || run.RetryGroupID == "" || run.Status != RunStatusTriggered {
		return nil, ErrRetryGenerationMismatch
	}
	if err := retryLaunchLive(ctx, tx, runID, run.RetryGroupGeneration); err != nil {
		return nil, err
	}
	if err := validateRetryLaunchSession(ctx, tx, taskID, sessionID); err != nil {
		return nil, err
	}

	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	now := time.Now().UTC()
	launch := &RetryLaunch{RunID: runID, AutomationID: run.AutomationID, Generation: run.RetryGroupGeneration, TaskID: taskID, SessionID: sessionID, TurnID: uuid.NewString(), SubmissionID: "prompt:retry:" + uuid.NewString(), State: RetryLaunchPrepared, CreatedAt: now, UpdatedAt: now}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automation_retry_launches (`+retryLaunchColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), launch.RunID, launch.AutomationID, launch.Generation, launch.TaskID, launch.SessionID, launch.TurnID, launch.SubmissionID, launch.State, "", "", false, now, now)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET status = ?, session_id = ?, turn_id = ?, thread_action = ?, thread_reason = ?, error_message = ? WHERE id = ? AND status = ?`), RunStatusTaskCreated, launch.SessionID, launch.TurnID, action, reason, "Retry launch is pending provider acceptance", runID, RunStatusTriggered)
	if err != nil {
		return nil, err
	}
	return launch, nil
}

func retryLaunchLive(ctx context.Context, tx *sqlx.Tx, runID string, generation int64) error {
	var live int
	err := tx.GetContext(ctx, &live, tx.Rebind(`SELECT COUNT(*) FROM automation_runs ar JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id JOIN automations a ON a.id = ar.automation_id WHERE ar.id = ? AND ar.retry_group_generation = ? AND rg.generation = ? AND rg.state = ? AND a.enabled = TRUE AND ar.status IN (?, ?)`), runID, generation, generation, RetryGroupLive, RunStatusTriggered, RunStatusTaskCreated)
	if err != nil {
		return err
	}
	if live != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

func (s *Store) ClaimRetryLaunch(ctx context.Context, runID string) (*RetryLaunch, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE automations SET id = id WHERE id = (SELECT automation_id FROM automation_retry_launches WHERE run_id = ?)`), runID)
	if err != nil {
		return nil, err
	}
	var launch RetryLaunch
	if err := tx.GetContext(ctx, &launch, tx.Rebind(`SELECT `+retryLaunchColumns+` FROM automation_retry_launches WHERE run_id = ?`), runID); err != nil {
		return nil, err
	}
	if launch.State != RetryLaunchPrepared && launch.State != RetryLaunchQueued {
		return nil, ErrRetryLaunchUncertain
	}
	if launch.CancelRequested {
		return nil, ErrRetryGenerationMismatch
	}
	if err := retryLaunchLive(ctx, tx, runID, launch.Generation); err != nil {
		return nil, err
	}
	token := uuid.NewString()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_launches SET state = ?, token = ?, updated_at = ? WHERE run_id = ? AND state = ? AND cancel_requested = FALSE`), RetryLaunchPreparing, token, time.Now().UTC(), runID, launch.State)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return nil, ErrRetryLaunchUncertain
	}
	launch.State = RetryLaunchPreparing
	launch.Token = token
	return &launch, tx.Commit()
}

func retryLaunchTuple(launch *RetryLaunch) []any {
	return []any{launch.RunID, launch.Generation, launch.TaskID, launch.SessionID, launch.TurnID, launch.SubmissionID, launch.Token}
}

func (s *Store) AdmitRetryLaunch(ctx context.Context, launch *RetryLaunch, executionID string) error {
	if launch == nil || executionID == "" {
		return ErrRetryGenerationMismatch
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockRetryAutomation(ctx, tx, launch.AutomationID); err != nil {
		return err
	}
	if err := retryLaunchLive(ctx, tx, launch.RunID, launch.Generation); err != nil {
		return err
	}
	args := append([]any{RetryLaunchDispatching, executionID, time.Now().UTC()}, retryLaunchTuple(launch)...)
	args = append(args, RetryLaunchPreparing)
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_launches SET state = ?, execution_id = ?, updated_at = ? WHERE run_id = ? AND group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND submission_id = ? AND token = ? AND state = ? AND cancel_requested = FALSE`), args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryGenerationMismatch
	}
	return tx.Commit()
}

func (s *Store) VerifyRetryLaunch(ctx context.Context, launch *RetryLaunch, executionID string) error {
	if launch == nil {
		return ErrRetryGenerationMismatch
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockRetryAutomation(ctx, tx, launch.AutomationID); err != nil {
		return err
	}
	if err := retryLaunchLive(ctx, tx, launch.RunID, launch.Generation); err != nil {
		return err
	}
	args := retryLaunchTuple(launch)
	args = append(args, RetryLaunchDispatching, executionID)
	var live int
	err = tx.GetContext(ctx, &live, tx.Rebind(`SELECT COUNT(*) FROM automation_retry_launches WHERE run_id = ? AND group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND submission_id = ? AND token = ? AND state = ? AND execution_id = ? AND cancel_requested = FALSE`), args...)
	if err != nil {
		return err
	}
	if live != 1 {
		return ErrRetryGenerationMismatch
	}
	return tx.Commit()
}

// AcceptRetryLaunch records acceptance even when cancellation won after admission.
// The same immutable tuple may settle uncertainty; it never authorizes another send.
func (s *Store) AcceptRetryLaunch(ctx context.Context, launch *RetryLaunch, executionID string) error {
	if launch == nil || executionID == "" {
		return ErrRetryGenerationMismatch
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	args := append([]any{RetryLaunchAccepted, time.Now().UTC()}, retryLaunchTuple(launch)...)
	args = append(args, executionID, RetryLaunchDispatching, RetryLaunchUncertain, RetryLaunchAccepted)
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_retry_launches SET state = ?, updated_at = ? WHERE run_id = ? AND group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND submission_id = ? AND token = ? AND execution_id = ? AND state IN (?, ?, ?)`), args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryGenerationMismatch
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE automation_runs SET error_message = '' WHERE id = ? AND retry_group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id=automation_runs.id AND l.cancel_requested=FALSE)`), launch.RunID, launch.Generation, launch.TaskID, launch.SessionID, launch.TurnID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeferRetryLaunch(ctx context.Context, launch *RetryLaunch) error {
	if launch == nil {
		return ErrRetryGenerationMismatch
	}
	args := append([]any{RetryLaunchStopped, RetryLaunchQueued, time.Now().UTC()}, retryLaunchTuple(launch)...)
	args = append(args, RetryLaunchPreparing)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_retry_launches SET state = CASE WHEN cancel_requested THEN ? ELSE ? END, updated_at = ? WHERE run_id = ? AND group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND submission_id = ? AND token = ? AND state = ? AND execution_id = ''`), args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryLaunchUncertain
	}
	return nil
}

func (s *Store) MarkRetryLaunchUncertain(ctx context.Context, launch *RetryLaunch) error {
	if launch == nil {
		return ErrRetryGenerationMismatch
	}
	args := append([]any{RetryLaunchUncertain, time.Now().UTC()}, retryLaunchTuple(launch)...)
	args = append(args, RetryLaunchPreparing, RetryLaunchDispatching)
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_retry_launches SET state = ?, updated_at = ? WHERE run_id = ? AND group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND submission_id = ? AND token = ? AND state IN (?, ?)`), args...)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET error_message = ? WHERE id = ? AND EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id = automation_runs.id AND l.state = ?)`), ErrRetryLaunchUncertain.Error(), launch.RunID, RetryLaunchUncertain)
	return err
}

func (s *Store) RequestRetryLaunchStop(ctx context.Context, runID string) (*RetryLaunch, error) {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_retry_launches SET cancel_requested = TRUE, state = CASE WHEN state IN (?, ?) THEN ? ELSE state END, updated_at = ? WHERE run_id = ?`), RetryLaunchPrepared, RetryLaunchQueued, RetryLaunchStopped, time.Now().UTC(), runID)
	if err != nil {
		return nil, err
	}
	return s.GetRetryLaunch(ctx, runID)
}

// CancelQueuedRetryLaunch settles only a launch that never crossed admission.
func (s *Service) CancelQueuedRetryLaunch(ctx context.Context, runID string) (bool, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil || run.RetryGroupID == "" {
		return false, err
	}
	_, err = s.store.db.ExecContext(ctx, s.store.db.Rebind(`UPDATE automation_retry_launches
	 SET state = ?, cancel_requested = TRUE, updated_at = ?
	 WHERE run_id = ? AND execution_id = '' AND state IN (?, ?)`),
		RetryLaunchStopped, time.Now().UTC(), runID, RetryLaunchPrepared, RetryLaunchQueued)
	if err != nil {
		return true, err
	}
	launch, err := s.store.GetRetryLaunch(ctx, runID)
	if err != nil || launch.State != RetryLaunchStopped {
		return true, errors.Join(ErrRetryLaunchUncertain, err)
	}
	// A concurrent claim either lost the CAS above or retains uncertain ownership.
	if run.Status != RunStatusTriggered && run.Status != RunStatusTaskCreated {
		return true, nil
	}
	return true, s.store.CancelRetryRun(ctx, runID, run.RetryGroupGeneration)
}

func (s *Store) CompleteRetryLaunchStop(ctx context.Context, launch *RetryLaunch) error {
	if launch == nil {
		return ErrRetryGenerationMismatch
	}
	args := append([]any{RetryLaunchStopped, time.Now().UTC()}, retryLaunchTuple(launch)...)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_retry_launches SET state = ?, cancel_requested = TRUE, updated_at = ? WHERE run_id = ? AND group_generation = ? AND task_id = ? AND session_id = ? AND turn_id = ? AND submission_id = ? AND token = ?`), args...)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

// ObserveRetryLaunchTerminal records the exact authenticated turn-complete boundary.
// Session state changes and failed network calls are not terminal evidence.
func (s *Store) ObserveRetryLaunchTerminal(ctx context.Context, taskID, sessionID, turnID, executionID string) error {
	if taskID == "" || sessionID == "" || turnID == "" || executionID == "" {
		return ErrRetryGenerationMismatch
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_retry_launches SET state = ?, updated_at = ? WHERE task_id = ? AND session_id = ? AND turn_id = ? AND execution_id = ? AND state IN (?, ?, ?)`), RetryLaunchStopped, time.Now().UTC(), taskID, sessionID, turnID, executionID, RetryLaunchDispatching, RetryLaunchAccepted, RetryLaunchUncertain)
	return err
}

func (s *Service) ObserveRetryLaunchTerminal(ctx context.Context, taskID, sessionID, turnID, executionID string) error {
	return s.store.ObserveRetryLaunchTerminal(ctx, taskID, sessionID, turnID, executionID)
}

func requireResolvedRetryLaunches(ctx context.Context, tx *sqlx.Tx, column, id string) error {
	// Callers pass only these fixed internal columns.
	if column != "automation_id" && column != "run_id" {
		return ErrRetryGenerationMismatch
	}
	var pending int
	if err := tx.GetContext(ctx, &pending, tx.Rebind(`SELECT COUNT(*) FROM automation_retry_launches WHERE ((? = 'automation_id' AND automation_id = ?) OR (? = 'run_id' AND run_id = ?)) AND state <> ?`), column, id, column, id, RetryLaunchStopped); err != nil {
		return err
	}
	if pending > 0 {
		return ErrRetryLaunchUncertain
	}
	var legacy int
	err := tx.GetContext(ctx, &legacy, tx.Rebind(`SELECT COUNT(*) FROM automation_runs ar WHERE ((? = 'automation_id' AND ar.automation_id = ?) OR (? = 'run_id' AND ar.id = ?)) AND ar.retry_group_id <> '' AND ar.status IN (?, ?) AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id = ar.id AND p.version IN (1, -1))`), column, id, column, id, RunStatusTriggered, RunStatusTaskCreated)
	if err != nil {
		return err
	}
	if legacy > 0 {
		return ErrRetryLaunchUncertain
	}
	return nil
}

func (s *Service) GetRetryLaunch(ctx context.Context, runID string) (*RetryLaunch, error) {
	return s.store.GetRetryLaunch(ctx, runID)
}

func (s *Service) RetryAdmissionUnresolved(ctx context.Context, runID string) (bool, error) {
	var evidence struct {
		Owners  int `db:"owners"`
		Pending int `db:"pending"`
	}
	err := s.store.ro.GetContext(ctx, &evidence, s.store.ro.Rebind(`SELECT
	 (SELECT COUNT(*) FROM automation_retry_launch_protocols WHERE run_id=?) AS owners,
	 (SELECT COUNT(*) FROM automation_retry_launches WHERE run_id=? AND state<>'stopped') AS pending`), runID, runID)
	return evidence.Owners != 1 || evidence.Pending != 0, err
}
func (s *Service) ReserveRetryLaunch(ctx context.Context, runID, taskID, sessionID string, action ThreadAction, reason string) (*RetryLaunch, error) {
	return s.store.ReserveRetryLaunch(ctx, runID, taskID, sessionID, action, reason)
}
func (s *Service) AdmitRetryLaunch(ctx context.Context, launch *RetryLaunch, executionID string) error {
	return s.store.AdmitRetryLaunch(ctx, launch, executionID)
}
func (s *Service) VerifyRetryLaunch(ctx context.Context, launch *RetryLaunch, executionID string) error {
	return s.store.VerifyRetryLaunch(ctx, launch, executionID)
}
func (s *Service) AcceptRetryLaunch(ctx context.Context, launch *RetryLaunch, executionID string) error {
	return s.store.AcceptRetryLaunch(ctx, launch, executionID)
}

var ErrRetryCapacityReached = errors.New("automation retry admission capacity reached")

func checkRetryAdmissionCapacity(ctx context.Context, tx *sqlx.Tx, automationID string) error {
	var admission struct {
		Limit   int  `db:"max_concurrent_runs"`
		Enabled bool `db:"enabled"`
	}
	if err := tx.GetContext(ctx, &admission, tx.Rebind(`SELECT max_concurrent_runs, enabled FROM automations WHERE id = ?`), automationID); err != nil {
		return err
	}
	if !admission.Enabled {
		return ErrRetryGenerationMismatch
	}
	return checkRetryCapacityLimit(ctx, tx, automationID, admission.Limit)
}

func checkRetryCapacityLimit(ctx context.Context, tx *sqlx.Tx, automationID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	tables, err := retryCapacityTaskTablesPresent(ctx, tx)
	if err != nil {
		return err
	}
	var active int
	if tables {
		err = tx.GetContext(ctx, &active, tx.Rebind(`SELECT COUNT(*) FROM automation_runs ar LEFT JOIN tasks t ON t.id = ar.task_id WHERE ar.automation_id = ? AND `+openRunPredicateSQL), append([]any{automationID}, openRunArgs()...)...)
	} else {
		err = tx.GetContext(ctx, &active, tx.Rebind(`SELECT COUNT(*) FROM automation_runs WHERE automation_id = ? AND status IN (?, ?)`), automationID, RunStatusTriggered, RunStatusTaskCreated)
	}
	if err != nil {
		return err
	}
	if active >= limit {
		return ErrRetryCapacityReached
	}
	return nil
}

func retryCapacityTaskTablesPresent(ctx context.Context, tx *sqlx.Tx) (bool, error) {
	return retryRequiredTablesPresent(ctx, tx, "tasks", "task_sessions")
}

func retryRequiredTablesPresent(ctx context.Context, conn internaldb.ContextSchemaQuerier, tables ...string) (bool, error) {
	for _, table := range tables {
		present, err := internaldb.TableExistsContext(ctx, conn, table)
		if err != nil || !present {
			return false, err
		}
	}
	return true, nil
}

func retryLaunchLiveForClaim(ctx context.Context, tx *sqlx.Tx, runID string, generation int64) error {
	var live int
	err := tx.GetContext(ctx, &live, tx.Rebind(`SELECT COUNT(*) FROM automation_runs ar JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id JOIN automations a ON a.id = ar.automation_id WHERE ar.id = ? AND ar.retry_group_generation = ? AND rg.generation = ? AND rg.state = ? AND a.enabled = TRUE`), runID, generation, generation, RetryGroupLive)
	if err != nil {
		return err
	}
	if live != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}

func (s *Service) dispatchRetryLaunch(ctx context.Context, run *AutomationRun, dispatch func() (RunDispatch, error)) error {
	launch, err := s.store.ClaimRetryLaunch(ctx, run.ID)
	if err != nil {
		return errors.Join(ErrRetryLaunchUncertain, err)
	}
	result, dispatchErr := dispatch()
	receiptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if errors.Is(dispatchErr, ErrRunDeferred) {
		if err := s.store.DeferRetryLaunch(receiptCtx, launch); err != nil {
			return errors.Join(ErrRetryLaunchUncertain, err)
		}
		return ErrRunDeferred
	}
	if dispatchErr == nil && result.TaskID == launch.TaskID && result.SessionID == launch.SessionID && result.TurnID == launch.TurnID {
		accepted, err := s.store.GetRetryLaunch(receiptCtx, run.ID)
		if err == nil && accepted.State == RetryLaunchAccepted {
			return nil
		}
		if err == nil && accepted.State == RetryLaunchDispatching {
			return ErrRetryLaunchUncertain
		}
	}
	if err := s.store.MarkRetryLaunchUncertain(receiptCtx, launch); err != nil {
		return errors.Join(ErrRetryLaunchUncertain, dispatchErr, err)
	}
	return errors.Join(ErrRetryLaunchUncertain, dispatchErr)
}

// A legacy stop receipt records termination, without inventing acceptance or launch IDs.
func (s *Store) recordLegacyRetryStop(ctx context.Context, run *AutomationRun) error {
	if run == nil || run.ID == "" || run.TaskID == "" || run.SessionID == "" || run.TurnID == "" || run.RetryGroupGeneration < 1 {
		return ErrRetryGenerationMismatch
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockRetryAutomation(ctx, tx, run.AutomationID); err != nil {
		return err
	}
	var exact int
	if err := tx.GetContext(ctx, &exact, tx.Rebind(`SELECT COUNT(*) FROM automation_runs ar WHERE ar.id=? AND ar.retry_group_generation=? AND
 ((ar.task_id=? AND ar.session_id=? AND ar.turn_id=?) OR
  (COALESCE(ar.task_id,'') IN ('',?) AND COALESCE(ar.session_id,'') IN ('',?) AND COALESCE(ar.turn_id,'') IN ('',?)
   AND EXISTS (SELECT 1 FROM automation_run_operations o WHERE o.run_id=ar.id AND o.group_generation=ar.retry_group_generation
    AND o.operation_kind=? AND o.state IN (?,?,?) AND o.external_task_id=? AND o.external_session_id=? AND o.external_turn_id=?)))`),
		run.ID, run.RetryGroupGeneration, run.TaskID, run.SessionID, run.TurnID, run.TaskID, run.SessionID, run.TurnID, retryTaskOperationKind, retryOperationCommitted, retryOperationAmbiguous, retryOperationAbandoned, run.TaskID, run.SessionID, run.TurnID); err != nil {
		return err
	}
	if exact != 1 {
		return ErrRetryGenerationMismatch
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO automation_retry_launch_protocols(run_id,task_id,version) VALUES (?,?,-1) ON CONFLICT(run_id) DO UPDATE SET version = -1, task_id = excluded.task_id WHERE automation_retry_launch_protocols.task_id IN ('', excluded.task_id)`), run.ID, run.TaskID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrRetryGenerationMismatch
	}
	return tx.Commit()
}

func (s *Store) RecordRetryLaunchUncertainty(ctx context.Context, runID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET error_message=? WHERE id=? AND retry_group_id<>'' AND status IN ('triggered','task_created')`), ErrRetryLaunchUncertain.Error(), runID)
	return err
}

// RecordLegacyRetryLaunchUncertainty preserves unbound identity while surfacing
// missing launch provenance. An outbox or task ID cannot establish acceptance.
func (s *Store) RecordLegacyRetryLaunchUncertainty(ctx context.Context, runID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE automation_runs SET error_message=?
 WHERE id=? AND retry_group_id<>'' AND status IN (?,?)
 AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id=automation_runs.id AND p.version IN (1,-1))`),
		ErrRetryLaunchUncertain.Error(), runID, RunStatusTriggered, RunStatusTaskCreated)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected != 0, err
}

func validateRetryLaunchSession(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) error {
	query := `SELECT COUNT(*) FROM task_sessions WHERE task_id = ?`
	args := []any{taskID}
	if sessionID != "" {
		query += ` AND id = ?`
		args = append(args, sessionID)
	}
	var sessions int
	if err := tx.GetContext(ctx, &sessions, tx.Rebind(query), args...); err != nil {
		return err
	}
	if sessionID == "" && sessions != 0 {
		return ErrRetryLaunchUncertain
	}
	if sessionID != "" && sessions != 1 {
		return ErrRetryGenerationMismatch
	}
	return nil
}
