package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

const terminalRunRetention = 20

var (
	ErrNotFound          = errors.New("storage record not found")
	ErrConflict          = errors.New("storage operation conflicts with current state")
	ErrInvalidTransition = errors.New("invalid storage state transition")
)

type RunTrigger string

const (
	RunTriggerScheduled RunTrigger = "scheduled"
	RunTriggerManual    RunTrigger = "manual"
	RunTriggerAnalysis  RunTrigger = "analysis"
)

type RunState string

const (
	RunStateQueued      RunState = "queued"
	RunStateRunning     RunState = "running"
	RunStateSucceeded   RunState = "succeeded"
	RunStateFailed      RunState = "failed"
	RunStateCancelled   RunState = "cancelled"
	RunStateSkippedBusy RunState = "skipped_busy"
)

type MaintenanceRun struct {
	ID               string          `json:"id"`
	Trigger          RunTrigger      `json:"trigger"`
	State            RunState        `json:"state"`
	SettingsSnapshot json.RawMessage `json:"settings_snapshot"`
	Result           json.RawMessage `json:"result"`
	Message          string          `json:"message"`
	StartedAt        time.Time       `json:"started_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

type ResourceType string

const (
	ResourceTypeTaskWorkspace ResourceType = "task_workspace"
	ResourceTypeGoCache       ResourceType = "go_cache"
)

type QuarantineState string

const (
	QuarantineStateQuarantined QuarantineState = "quarantined"
	QuarantineStateRestored    QuarantineState = "restored"
	QuarantineStateDeleted     QuarantineState = "deleted"
	QuarantineStateFailed      QuarantineState = "failed"
)

type QuarantineEntry struct {
	ID             string          `json:"id"`
	ResourceType   ResourceType    `json:"resource_type"`
	TaskID         string          `json:"task_id,omitempty"`
	WorkspaceID    string          `json:"workspace_id,omitempty"`
	OriginalPath   string          `json:"original_path"`
	QuarantinePath string          `json:"quarantine_path"`
	SizeBytes      int64           `json:"size_bytes"`
	State          QuarantineState `json:"state"`
	QuarantinedAt  time.Time       `json:"quarantined_at"`
	DeleteAfter    time.Time       `json:"delete_after"`
	RestoredAt     *time.Time      `json:"restored_at,omitempty"`
	DeletedAt      *time.Time      `json:"deleted_at,omitempty"`
	LastError      string          `json:"last_error"`
	Metadata       json.RawMessage `json:"metadata"`
}

type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

func NewStore(pool *db.Pool) (*Store, error) {
	store := &Store{db: pool.Writer(), ro: pool.Reader()}
	if err := initStorageSchema(store.db); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) CreateRun(ctx context.Context, run *MaintenanceRun) error {
	if err := normalizeNewRun(run); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO storage_maintenance_runs
			(id, trigger, state, settings_snapshot, result, message, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), run.ID, run.Trigger, run.State, string(run.SettingsSnapshot), string(run.Result), run.Message,
		run.StartedAt, run.CompletedAt)
	if err != nil {
		return fmt.Errorf("create storage maintenance run: %w", err)
	}
	return nil
}

func (s *Store) GetRun(ctx context.Context, id string) (MaintenanceRun, error) {
	row, err := getRunRow(ctx, s.ro, id)
	if err != nil {
		return MaintenanceRun{}, err
	}
	return row.run(), nil
}

func (s *Store) TransitionRun(
	ctx context.Context,
	id string,
	next RunState,
	result json.RawMessage,
	message string,
) (MaintenanceRun, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return MaintenanceRun{}, fmt.Errorf("begin run transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getRunRow(ctx, tx, id)
	if err != nil {
		return MaintenanceRun{}, err
	}
	if !validRunTransition(RunState(current.State), next) {
		return MaintenanceRun{}, transitionError("run", current.State, string(next))
	}
	if len(result) == 0 {
		result = json.RawMessage(current.Result)
	}
	completedAt := runCompletionTime(next)
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE storage_maintenance_runs
		SET state = ?, result = ?, message = ?, completed_at = ?
		WHERE id = ? AND state = ?
	`), next, string(normalizeJSON(result)), message, completedAt, id, current.State)
	if err != nil {
		return MaintenanceRun{}, fmt.Errorf("transition storage maintenance run: %w", err)
	}
	if err := requireOneRow(res); err != nil {
		return MaintenanceRun{}, err
	}
	if isTerminalRunState(next) {
		if err := pruneTerminalRuns(ctx, tx); err != nil {
			return MaintenanceRun{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return MaintenanceRun{}, fmt.Errorf("commit run transition: %w", err)
	}
	return s.GetRun(ctx, id)
}

func (s *Store) ListRuns(ctx context.Context, terminalLimit int) ([]MaintenanceRun, error) {
	if terminalLimit <= 0 || terminalLimit > terminalRunRetention {
		terminalLimit = terminalRunRetention
	}
	rows := make([]runRow, 0)
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(`
		SELECT id, trigger, state, settings_snapshot, result, message, started_at, completed_at
		FROM storage_maintenance_runs
		WHERE state NOT IN ('succeeded', 'failed', 'cancelled', 'skipped_busy')
		   OR id IN (
			SELECT id FROM storage_maintenance_runs
			WHERE state IN ('succeeded', 'failed', 'cancelled', 'skipped_busy')
			ORDER BY completed_at DESC, started_at DESC, id DESC
			LIMIT ?
		   )
		ORDER BY started_at DESC, id DESC
	`), terminalLimit)
	if err != nil {
		return nil, fmt.Errorf("list storage maintenance runs: %w", err)
	}
	runs := make([]MaintenanceRun, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, row.run())
	}
	return runs, nil
}

func (s *Store) CreateQuarantineEntry(ctx context.Context, entry *QuarantineEntry) error {
	if err := normalizeNewQuarantineEntry(entry); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO storage_quarantine_entries
			(id, resource_type, task_id, workspace_id, original_path, quarantine_path, size_bytes,
			 state, quarantined_at, delete_after, restored_at, deleted_at, last_error, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), entry.ID, entry.ResourceType, nullableString(entry.TaskID), nullableString(entry.WorkspaceID),
		entry.OriginalPath, entry.QuarantinePath, entry.SizeBytes, entry.State, entry.QuarantinedAt,
		entry.DeleteAfter, entry.RestoredAt, entry.DeletedAt, entry.LastError, string(entry.Metadata))
	if err != nil {
		return fmt.Errorf("create storage quarantine entry: %w", err)
	}
	return nil
}

func (s *Store) GetQuarantineEntry(ctx context.Context, id string) (QuarantineEntry, error) {
	row, err := getQuarantineRow(ctx, s.ro, id)
	if err != nil {
		return QuarantineEntry{}, err
	}
	return row.entry(), nil
}

func (s *Store) TransitionQuarantineEntry(
	ctx context.Context,
	id string,
	next QuarantineState,
	lastError string,
) (QuarantineEntry, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return QuarantineEntry{}, fmt.Errorf("begin quarantine transition: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := getQuarantineRow(ctx, tx, id)
	if err != nil {
		return QuarantineEntry{}, err
	}
	if !validQuarantineTransition(QuarantineState(current.State), next) {
		return QuarantineEntry{}, transitionError("quarantine", current.State, string(next))
	}
	restoredAt, deletedAt := quarantineCompletionTimes(next)
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE storage_quarantine_entries
		SET state = ?, restored_at = ?, deleted_at = ?, last_error = ?
		WHERE id = ? AND state = ?
	`), next, restoredAt, deletedAt, lastError, id, current.State)
	if err != nil {
		return QuarantineEntry{}, fmt.Errorf("transition storage quarantine entry: %w", err)
	}
	if err := requireOneRow(res); err != nil {
		return QuarantineEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return QuarantineEntry{}, fmt.Errorf("commit quarantine transition: %w", err)
	}
	return s.GetQuarantineEntry(ctx, id)
}

func (s *Store) ListQuarantineEntries(ctx context.Context, includeTerminal bool) ([]QuarantineEntry, error) {
	query := `SELECT id, resource_type, task_id, workspace_id, original_path, quarantine_path,
		size_bytes, state, quarantined_at, delete_after, restored_at, deleted_at, last_error, metadata
		FROM storage_quarantine_entries`
	if !includeTerminal {
		query += ` WHERE state IN ('quarantined', 'failed')`
	}
	query += ` ORDER BY quarantined_at DESC, id DESC`
	rows := make([]quarantineRow, 0)
	if err := s.ro.SelectContext(ctx, &rows, query); err != nil {
		return nil, fmt.Errorf("list storage quarantine entries: %w", err)
	}
	entries := make([]QuarantineEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, row.entry())
	}
	return entries, nil
}

func (s *Store) SummarizeQuarantine(ctx context.Context) (QuarantineSummary, error) {
	var summary QuarantineSummary
	err := s.ro.GetContext(ctx, &summary, `
		SELECT COUNT(*) AS count, COALESCE(SUM(size_bytes), 0) AS size_bytes
		FROM storage_quarantine_entries
		WHERE state IN ('quarantined', 'failed')
	`)
	if err != nil {
		return QuarantineSummary{}, fmt.Errorf("summarize storage quarantine: %w", err)
	}
	return summary, nil
}

type queryer interface {
	GetContext(context.Context, any, string, ...any) error
	Rebind(string) string
}

type runRow struct {
	ID               string     `db:"id"`
	Trigger          string     `db:"trigger"`
	State            string     `db:"state"`
	SettingsSnapshot string     `db:"settings_snapshot"`
	Result           string     `db:"result"`
	Message          string     `db:"message"`
	StartedAt        time.Time  `db:"started_at"`
	CompletedAt      *time.Time `db:"completed_at"`
}

func getRunRow(ctx context.Context, conn queryer, id string) (runRow, error) {
	var row runRow
	err := conn.GetContext(ctx, &row, conn.Rebind(`
		SELECT id, trigger, state, settings_snapshot, result, message, started_at, completed_at
		FROM storage_maintenance_runs WHERE id = ?
	`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return runRow{}, ErrNotFound
	}
	if err != nil {
		return runRow{}, fmt.Errorf("get storage maintenance run: %w", err)
	}
	return row, nil
}

func (r runRow) run() MaintenanceRun {
	return MaintenanceRun{
		ID: r.ID, Trigger: RunTrigger(r.Trigger), State: RunState(r.State),
		SettingsSnapshot: json.RawMessage(r.SettingsSnapshot), Result: json.RawMessage(r.Result),
		Message: r.Message, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt,
	}
}

type quarantineRow struct {
	ID             string         `db:"id"`
	ResourceType   string         `db:"resource_type"`
	TaskID         sql.NullString `db:"task_id"`
	WorkspaceID    sql.NullString `db:"workspace_id"`
	OriginalPath   string         `db:"original_path"`
	QuarantinePath string         `db:"quarantine_path"`
	SizeBytes      int64          `db:"size_bytes"`
	State          string         `db:"state"`
	QuarantinedAt  time.Time      `db:"quarantined_at"`
	DeleteAfter    time.Time      `db:"delete_after"`
	RestoredAt     *time.Time     `db:"restored_at"`
	DeletedAt      *time.Time     `db:"deleted_at"`
	LastError      string         `db:"last_error"`
	Metadata       string         `db:"metadata"`
}

func getQuarantineRow(ctx context.Context, conn queryer, id string) (quarantineRow, error) {
	var row quarantineRow
	err := conn.GetContext(ctx, &row, conn.Rebind(`
		SELECT id, resource_type, task_id, workspace_id, original_path, quarantine_path,
			size_bytes, state, quarantined_at, delete_after, restored_at, deleted_at, last_error, metadata
		FROM storage_quarantine_entries WHERE id = ?
	`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return quarantineRow{}, ErrNotFound
	}
	if err != nil {
		return quarantineRow{}, fmt.Errorf("get storage quarantine entry: %w", err)
	}
	return row, nil
}

func (r quarantineRow) entry() QuarantineEntry {
	return QuarantineEntry{
		ID: r.ID, ResourceType: ResourceType(r.ResourceType), TaskID: r.TaskID.String,
		WorkspaceID: r.WorkspaceID.String, OriginalPath: r.OriginalPath, QuarantinePath: r.QuarantinePath,
		SizeBytes: r.SizeBytes, State: QuarantineState(r.State), QuarantinedAt: r.QuarantinedAt,
		DeleteAfter: r.DeleteAfter, RestoredAt: r.RestoredAt, DeletedAt: r.DeletedAt,
		LastError: r.LastError, Metadata: json.RawMessage(r.Metadata),
	}
}

func normalizeNewRun(run *MaintenanceRun) error {
	if run == nil || run.ID == "" {
		return validationError("run id is required")
	}
	if run.State != RunStateQueued {
		return validationError("new run state must be queued")
	}
	if run.Trigger != RunTriggerScheduled && run.Trigger != RunTriggerManual && run.Trigger != RunTriggerAnalysis {
		return validationError("unknown run trigger %q", run.Trigger)
	}
	run.SettingsSnapshot = normalizeJSON(run.SettingsSnapshot)
	run.Result = normalizeJSON(run.Result)
	if run.StartedAt.IsZero() {
		run.StartedAt = time.Now().UTC()
	}
	run.StartedAt = run.StartedAt.UTC()
	return nil
}

func normalizeNewQuarantineEntry(entry *QuarantineEntry) error {
	if entry == nil || entry.ID == "" {
		return validationError("quarantine entry id is required")
	}
	if entry.ResourceType != ResourceTypeTaskWorkspace && entry.ResourceType != ResourceTypeGoCache {
		return validationError("unknown quarantine resource type %q", entry.ResourceType)
	}
	if entry.State != QuarantineStateQuarantined {
		return validationError("new quarantine state must be quarantined")
	}
	if entry.SizeBytes < 0 {
		return validationError("quarantine size_bytes cannot be negative")
	}
	var err error
	entry.OriginalPath, err = normalizeAbsolutePath("original_path", entry.OriginalPath)
	if err != nil {
		return err
	}
	entry.QuarantinePath, err = normalizeAbsolutePath("quarantine_path", entry.QuarantinePath)
	if err != nil {
		return err
	}
	if entry.QuarantinedAt.IsZero() {
		entry.QuarantinedAt = time.Now().UTC()
	}
	if entry.DeleteAfter.IsZero() || entry.DeleteAfter.Before(entry.QuarantinedAt) {
		return validationError("delete_after must be at or after quarantined_at")
	}
	entry.QuarantinedAt = entry.QuarantinedAt.UTC()
	entry.DeleteAfter = entry.DeleteAfter.UTC()
	entry.Metadata = normalizeJSON(entry.Metadata)
	return nil
}

func normalizeAbsolutePath(field, path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", validationError("%s must be an absolute path", field)
	}
	return filepath.Clean(path), nil
}

func validRunTransition(current, next RunState) bool {
	if current == RunStateQueued {
		return next == RunStateRunning || next == RunStateSkippedBusy ||
			next == RunStateCancelled || next == RunStateFailed
	}
	if current == RunStateRunning {
		return next == RunStateSucceeded || next == RunStateFailed || next == RunStateCancelled
	}
	return false
}

func validQuarantineTransition(current, next QuarantineState) bool {
	if current == QuarantineStateQuarantined {
		return next == QuarantineStateRestored || next == QuarantineStateDeleted || next == QuarantineStateFailed
	}
	if current == QuarantineStateFailed {
		return next == QuarantineStateRestored || next == QuarantineStateDeleted
	}
	return false
}

func isTerminalRunState(state RunState) bool {
	return state == RunStateSucceeded || state == RunStateFailed || state == RunStateCancelled || state == RunStateSkippedBusy
}

func runCompletionTime(state RunState) *time.Time {
	if !isTerminalRunState(state) {
		return nil
	}
	now := time.Now().UTC()
	return &now
}

func quarantineCompletionTimes(state QuarantineState) (*time.Time, *time.Time) {
	now := time.Now().UTC()
	switch state {
	case QuarantineStateRestored:
		return &now, nil
	case QuarantineStateDeleted:
		return nil, &now
	default:
		return nil, nil
	}
}

func normalizeJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func transitionError(kind, current, next string) error {
	return fmt.Errorf("%w: %s %s -> %s", ErrInvalidTransition, kind, current, next)
}

func requireOneRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read transition result: %w", err)
	}
	if rows != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func pruneTerminalRuns(ctx context.Context, tx *sqlx.Tx) error {
	query := tx.Rebind(`
		DELETE FROM storage_maintenance_runs
		WHERE state IN ('succeeded', 'failed', 'cancelled', 'skipped_busy')
		  AND id NOT IN (
			SELECT id FROM storage_maintenance_runs
			WHERE state IN ('succeeded', 'failed', 'cancelled', 'skipped_busy')
			ORDER BY completed_at DESC, started_at DESC, id DESC
			LIMIT ?
		  )
	`)
	_, err := tx.ExecContext(ctx, query, terminalRunRetention)
	if err != nil {
		return fmt.Errorf("prune storage maintenance runs: %w", err)
	}
	return nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
