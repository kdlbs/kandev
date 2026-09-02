package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/kandev/kandev/internal/lsp"
)

const taskLSPColumns = `
	task_id, language, policy, detected, detection_state,
	detection_scanned_at, detection_truncated, phase, generation, revision,
	runtime_incarnation, runtime_started_at, runtime_revision, process_absent_generation,
	process_started_at, initialize_started_at, ready_at, last_transition_at,
	last_action, last_action_at, last_stop_reason, last_restart_reason,
	last_initiator, restart_required, restart_required_reason, error_code,
	error_message, created_at, updated_at`

type taskLSPScanner interface {
	Scan(dest ...any) error
}

func (r *Repository) GetTaskLSPLanguage(
	ctx context.Context,
	taskID, language string,
) (*lsp.TaskLanguageState, bool, error) {
	if taskID == "" || language == "" {
		return nil, false, fmt.Errorf("task_id and language are required")
	}
	state, err := scanTaskLSPLanguage(r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT `+taskLSPColumns+`
		FROM task_lsp_languages
		WHERE task_id = ? AND language = ?
	`), taskID, language))
	if errors.Is(err, sql.ErrNoRows) {
		state := lsp.DefaultTaskLanguageState(taskID, language)
		return &state, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get task LSP language: %w", err)
	}
	return state, true, nil
}

func (r *Repository) ListTaskLSPLanguages(
	ctx context.Context,
	taskID string,
) ([]lsp.TaskLanguageState, error) {
	if taskID == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT `+taskLSPColumns+`
		FROM task_lsp_languages
		WHERE task_id = ?
		ORDER BY language
	`), taskID)
	if err != nil {
		return nil, fmt.Errorf("list task LSP languages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	states := make([]lsp.TaskLanguageState, 0)
	for rows.Next() {
		state, scanErr := scanTaskLSPLanguage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan task LSP language: %w", scanErr)
		}
		states = append(states, *state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read task LSP languages: %w", err)
	}
	return states, nil
}

func (r *Repository) ListAllTaskLSPLanguages(ctx context.Context) ([]lsp.TaskLanguageState, error) {
	rows, err := r.ro.QueryxContext(ctx, `
		SELECT `+taskLSPColumns+`
		FROM task_lsp_languages
		ORDER BY task_id, language
	`)
	if err != nil {
		return nil, fmt.Errorf("list all task LSP languages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	states := make([]lsp.TaskLanguageState, 0)
	for rows.Next() {
		state, scanErr := scanTaskLSPLanguage(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan task LSP language: %w", scanErr)
		}
		states = append(states, *state)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read all task LSP languages: %w", err)
	}
	return states, nil
}

func (r *Repository) CompareAndUpdateTaskLSPLanguage(
	ctx context.Context,
	next lsp.TaskLanguageState,
	expectedRevision uint64,
) (*lsp.TaskLanguageState, error) {
	if err := next.Validate(); err != nil {
		return nil, err
	}
	if expectedRevision > math.MaxInt64 {
		return nil, fmt.Errorf("expected revision exceeds database range")
	}

	now := time.Now().UTC()
	if next.LastTransitionAt.IsZero() {
		next.LastTransitionAt = now
	}
	if expectedRevision == 0 {
		return r.insertTaskLSPLanguage(ctx, next, now)
	}
	return r.updateTaskLSPLanguage(ctx, next, expectedRevision, now)
}

func (r *Repository) insertTaskLSPLanguage(
	ctx context.Context,
	next lsp.TaskLanguageState,
	now time.Time,
) (*lsp.TaskLanguageState, error) {
	query := `
		INSERT INTO task_lsp_languages (
			task_id, language, policy, detected, detection_state,
			detection_scanned_at, detection_truncated, phase, generation, revision,
			runtime_incarnation, runtime_started_at, runtime_revision, process_absent_generation,
			process_started_at, initialize_started_at, ready_at, last_transition_at,
			last_action, last_action_at, last_stop_reason, last_restart_reason,
			last_initiator, restart_required, restart_required_reason, error_code,
			error_message, created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
		ON CONFLICT (task_id, language) DO NOTHING
		RETURNING ` + taskLSPColumns
	state, err := scanTaskLSPLanguage(r.db.QueryRowxContext(
		ctx, r.db.Rebind(query), taskLSPWriteArgs(next, now, true)...,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, lsp.ErrRevisionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("insert task LSP language: %w", err)
	}
	return state, nil
}

func (r *Repository) updateTaskLSPLanguage(
	ctx context.Context,
	next lsp.TaskLanguageState,
	expectedRevision uint64,
	now time.Time,
) (*lsp.TaskLanguageState, error) {
	query := `
		UPDATE task_lsp_languages SET
			policy = ?, detected = ?, detection_state = ?, detection_scanned_at = ?,
			detection_truncated = ?, phase = ?, generation = ?, revision = revision + 1,
			runtime_incarnation = ?, runtime_started_at = ?, runtime_revision = ?,
			process_absent_generation = ?,
			process_started_at = ?, initialize_started_at = ?, ready_at = ?,
			last_transition_at = ?, last_action = ?, last_action_at = ?,
			last_stop_reason = ?, last_restart_reason = ?, last_initiator = ?,
			restart_required = ?, restart_required_reason = ?, error_code = ?,
			error_message = ?, updated_at = ?
		WHERE task_id = ? AND language = ? AND revision = ?
		RETURNING ` + taskLSPColumns
	state, err := scanTaskLSPLanguage(r.db.QueryRowxContext(
		ctx, r.db.Rebind(query), taskLSPUpdateArgs(next, expectedRevision, now)...,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, lsp.ErrRevisionConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update task LSP language: %w", err)
	}
	return state, nil
}

func (r *Repository) AllocateTaskLSPGeneration(
	ctx context.Context,
	taskID, language string,
	action lsp.Action,
	initiator lsp.Initiator,
	reason string,
	acceptedAt time.Time,
) (*lsp.TaskLanguageState, error) {
	if action != lsp.ActionStart && action != lsp.ActionRestart && action != lsp.ActionReconcile {
		return nil, fmt.Errorf("generation allocation requires start, restart, or reconcile action")
	}
	probe := lsp.DefaultTaskLanguageState(taskID, language)
	probe.LastAction = action
	probe.LastInitiator = initiator
	if err := probe.Validate(); err != nil {
		return nil, err
	}
	if acceptedAt.IsZero() {
		acceptedAt = time.Now().UTC()
	}
	restartReason := ""
	if action == lsp.ActionRestart {
		restartReason = reason
	}
	query := `
		INSERT INTO task_lsp_languages (
			task_id, language, policy, phase, generation, revision,
			last_transition_at, last_action, last_action_at, last_restart_reason,
			last_initiator, created_at, updated_at
		) VALUES (?, ?, CASE WHEN ? = 'start' THEN 'keep_warm' ELSE 'inherit' END,
			'starting', 1, 1, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (task_id, language) DO UPDATE SET
			policy = CASE
				WHEN excluded.last_action = 'start' THEN 'keep_warm'
				ELSE task_lsp_languages.policy
			END,
			phase = 'starting',
			generation = task_lsp_languages.generation + 1,
			revision = task_lsp_languages.revision + 1,
			runtime_incarnation = '',
			runtime_started_at = NULL,
			runtime_revision = 0,
			process_absent_generation = 0,
			process_started_at = NULL,
			initialize_started_at = NULL,
			ready_at = NULL,
			last_transition_at = excluded.last_transition_at,
			last_action = excluded.last_action,
			last_action_at = excluded.last_action_at,
			last_restart_reason = CASE
				WHEN excluded.last_action = 'restart' THEN excluded.last_restart_reason
				ELSE task_lsp_languages.last_restart_reason
			END,
			last_initiator = excluded.last_initiator,
			restart_required = FALSE,
			restart_required_reason = '',
			error_code = '',
			error_message = '',
			updated_at = excluded.updated_at
		RETURNING ` + taskLSPColumns
	state, err := scanTaskLSPLanguage(r.db.QueryRowxContext(
		ctx,
		r.db.Rebind(query),
		taskID,
		language,
		action,
		acceptedAt,
		action,
		acceptedAt,
		restartReason,
		initiator,
		acceptedAt,
		acceptedAt,
	))
	if err != nil {
		return nil, fmt.Errorf("allocate task LSP generation: %w", err)
	}
	return state, nil
}

func taskLSPWriteArgs(next lsp.TaskLanguageState, now time.Time, includeKey bool) []any {
	args := make([]any, 0, 29)
	if includeKey {
		args = append(args, next.TaskID, next.Language)
	}
	return append(args,
		next.Policy,
		next.Detected,
		next.DetectionState,
		next.DetectionScannedAt,
		next.DetectionTruncated,
		next.Phase,
		next.Generation,
		next.RuntimeIncarnation,
		next.RuntimeStartedAt,
		next.RuntimeRevision,
		next.ProcessAbsentGeneration,
		next.ProcessStartedAt,
		next.InitializeStartedAt,
		next.ReadyAt,
		next.LastTransitionAt,
		next.LastAction,
		next.LastActionAt,
		next.LastStopReason,
		next.LastRestartReason,
		next.LastInitiator,
		next.RestartRequired,
		next.RestartRequiredReason,
		next.ErrorCode,
		next.ErrorMessage,
		now,
		now,
	)
}

func taskLSPUpdateArgs(next lsp.TaskLanguageState, expectedRevision uint64, now time.Time) []any {
	args := taskLSPWriteArgs(next, now, false)
	// UPDATE preserves created_at, so remove its insert-only duplicate timestamp.
	args = args[:len(args)-1]
	return append(args, next.TaskID, next.Language, expectedRevision)
}

func scanTaskLSPLanguage(scanner taskLSPScanner) (*lsp.TaskLanguageState, error) {
	var (
		state                   lsp.TaskLanguageState
		generation              int64
		revision                int64
		runtimeRevision         int64
		processAbsentGeneration int64
		detectionScannedAt      sql.NullTime
		runtimeStartedAt        sql.NullTime
		processStartedAt        sql.NullTime
		initializeStartedAt     sql.NullTime
		readyAt                 sql.NullTime
		lastActionAt            sql.NullTime
	)
	if err := scanner.Scan(
		&state.TaskID,
		&state.Language,
		&state.Policy,
		&state.Detected,
		&state.DetectionState,
		&detectionScannedAt,
		&state.DetectionTruncated,
		&state.Phase,
		&generation,
		&revision,
		&state.RuntimeIncarnation,
		&runtimeStartedAt,
		&runtimeRevision,
		&processAbsentGeneration,
		&processStartedAt,
		&initializeStartedAt,
		&readyAt,
		&state.LastTransitionAt,
		&state.LastAction,
		&lastActionAt,
		&state.LastStopReason,
		&state.LastRestartReason,
		&state.LastInitiator,
		&state.RestartRequired,
		&state.RestartRequiredReason,
		&state.ErrorCode,
		&state.ErrorMessage,
		&state.CreatedAt,
		&state.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if generation < 0 || revision < 0 || runtimeRevision < 0 || processAbsentGeneration < 0 {
		return nil, fmt.Errorf("negative task LSP generation/revision")
	}
	state.Generation = uint64(generation)
	state.Revision = uint64(revision)
	state.RuntimeRevision = uint64(runtimeRevision)
	state.ProcessAbsentGeneration = uint64(processAbsentGeneration)
	state.DetectionScannedAt = nullTimePointer(detectionScannedAt)
	state.RuntimeStartedAt = nullTimePointer(runtimeStartedAt)
	state.ProcessStartedAt = nullTimePointer(processStartedAt)
	state.InitializeStartedAt = nullTimePointer(initializeStartedAt)
	state.ReadyAt = nullTimePointer(readyAt)
	state.LastActionAt = nullTimePointer(lastActionAt)
	if err := state.Validate(); err != nil {
		return nil, err
	}
	return &state, nil
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

var _ lsp.Store = (*Repository)(nil)
