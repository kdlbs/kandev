package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kandev/kandev/internal/task/models"
)

// reviewRunColumns is the canonical column list for task_review_runs. Kept as a
// constant so every read path scans in the same order as writes.
const reviewRunColumns = `id, task_id, session_id, trigger, workflow_step_id, agent_id, model,
	status, error_code, error_message, summary, finding_count, file_count, repository_count,
	prompt_tokens, response_tokens, duration_ms, entry_id, created_at, completed_at`

const reviewFindingColumns = `id, run_id, task_id, repository_id, repository_name, file_path,
	start_line, end_line, side, severity, category, title, body, suggestion, anchor_text,
	file_diff_hash, status, resolved_at, created_at, updated_at`

// restartCancelReason is recorded on runs that were still in flight when the
// backend stopped. In-flight review passes are never resumed (see the spec's
// Persistence guarantees), so the boot sweep closes them explicitly instead of
// leaving a run that looks alive forever.
const (
	restartCancelReason = "interrupted by restart"
	restartCancelCode   = "review_interrupted"
)

// CreateTaskReviewRun inserts a review run, filling in ID, timestamp, status,
// and trigger defaults when the caller left them blank.
func (r *Repository) CreateTaskReviewRun(ctx context.Context, run *models.TaskReviewRun) error {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if run.Status == "" {
		run.Status = models.ReviewRunPending
	}
	if run.Trigger == "" {
		run.Trigger = models.ReviewTriggerManual
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_review_runs (`+reviewRunColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), run.ID, run.TaskID, run.SessionID, string(run.Trigger), run.WorkflowStepID, run.AgentID,
		run.Model, string(run.Status), run.ErrorCode, run.ErrorMessage, run.Summary,
		run.FindingCount, run.FileCount, run.RepositoryCount, run.PromptTokens,
		run.ResponseTokens, run.DurationMs, run.EntryID, run.CreatedAt, run.CompletedAt)
	if err != nil {
		if isTaskReviewRunEntryViolation(err) {
			return fmt.Errorf("%w: entry %s", ErrTaskReviewRunEntryConflict, run.EntryID)
		}
		return fmt.Errorf("failed to create task review run: %w", err)
	}
	return nil
}

// ErrTaskReviewRunEntryConflict is returned by CreateTaskReviewRun when a run
// with the same non-empty EntryID already exists (idx_task_review_runs_entry_id).
// The narrow race this guards is two concurrent redeliveries of the same
// step-entry both passing FindTaskReviewRunByEntryID's pre-check before
// either has inserted. Neither caller re-fetches the winner's row on this
// error today: review.Runner.launch (internal/review/runner.go) returns it
// as-is, and orchestrator's runCodeReviewCallback logs and swallows it by
// design, matching every other review-launch failure ("a review failure
// never blocks the transition") — see AC-OFFICE-STEP-ENTRY-001.10. The loser
// of the race simply does not get a run for that entry; a later redelivery
// of the same entry (restart, retry) will find the winner's row through the
// FindTaskReviewRunByEntryID pre-check instead of hitting this conflict
// again.
var ErrTaskReviewRunEntryConflict = errors.New("task review run entry conflict")

// sqliteTaskReviewRunEntryViolationMessage is the substring go-sqlite3 puts in
// a UNIQUE-constraint error for idx_task_review_runs_entry_id.
const sqliteTaskReviewRunEntryViolationMessage = "UNIQUE constraint failed: task_review_runs.entry_id"

// isTaskReviewRunEntryViolation reports whether err is a violation of
// idx_task_review_runs_entry_id specifically, not any unique violation. On
// PostgreSQL it inspects the typed pgconn.PgError's constraint name; on
// SQLite (no typed access to the constraint name) it matches the message
// documented above. Mirrors isParticipantsNaturalKeyViolation in
// internal/workflow/repository/phase2_sqlite.go.
func isTaskReviewRunEntryViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == "idx_task_review_runs_entry_id"
	}
	return strings.Contains(err.Error(), sqliteTaskReviewRunEntryViolationMessage)
}

// FindTaskReviewRunByEntryID returns the run created for the given step-entry
// ledger identifier, or nil when no run carries it. entryID must be
// non-empty; callers should not call this for manual/MCP-triggered runs,
// which never carry an entry ID.
func (r *Repository) FindTaskReviewRunByEntryID(ctx context.Context, entryID string) (*models.TaskReviewRun, error) {
	if entryID == "" {
		return nil, nil
	}
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+reviewRunColumns+` FROM task_review_runs WHERE entry_id = ?`), entryID)
	run, err := scanReviewRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

// UpdateTaskReviewRun persists every mutable field of a run.
func (r *Repository) UpdateTaskReviewRun(ctx context.Context, run *models.TaskReviewRun) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_review_runs SET
			session_id = ?, agent_id = ?, model = ?, status = ?, error_code = ?,
			error_message = ?, summary = ?, finding_count = ?, file_count = ?,
			repository_count = ?, prompt_tokens = ?, response_tokens = ?,
			duration_ms = ?, completed_at = ?
		WHERE id = ?
	`), run.SessionID, run.AgentID, run.Model, string(run.Status), run.ErrorCode,
		run.ErrorMessage, run.Summary, run.FindingCount, run.FileCount, run.RepositoryCount,
		run.PromptTokens, run.ResponseTokens, run.DurationMs, run.CompletedAt, run.ID)
	if err != nil {
		return fmt.Errorf("failed to update task review run: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", models.ErrTaskReviewRunNotFound, run.ID)
	}
	return nil
}

// GetTaskReviewRun retrieves a run by ID.
func (r *Repository) GetTaskReviewRun(ctx context.Context, runID string) (*models.TaskReviewRun, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+reviewRunColumns+` FROM task_review_runs WHERE id = ?`), runID)
	run, err := scanReviewRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", models.ErrTaskReviewRunNotFound, runID)
	}
	return run, err
}

// ListTaskReviewRuns returns a task's runs, newest first, capped at limit.
func (r *Repository) ListTaskReviewRuns(ctx context.Context, taskID string, limit int) ([]*models.TaskReviewRun, error) {
	if limit <= 0 {
		limit = defaultReviewRunHistory
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+reviewRunColumns+` FROM task_review_runs
		 WHERE task_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`), taskID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list task review runs: %w", err)
	}
	return collectReviewRuns(rows)
}

// defaultReviewRunHistory bounds how much run history a task read returns.
const defaultReviewRunHistory = 20

// ListActiveTaskReviewRuns returns the task's pending/running runs, newest
// first. The in-flight guard uses this so a second review request rejoins the
// existing pass instead of starting a duplicate.
func (r *Repository) ListActiveTaskReviewRuns(ctx context.Context, taskID string) ([]*models.TaskReviewRun, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+reviewRunColumns+` FROM task_review_runs
		 WHERE task_id = ? AND status IN ('pending', 'running')
		 ORDER BY created_at DESC, id DESC`), taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list active task review runs: %w", err)
	}
	return collectReviewRuns(rows)
}

func collectReviewRuns(rows *sql.Rows) ([]*models.TaskReviewRun, error) {
	defer func() { _ = rows.Close() }()
	var runs []*models.TaskReviewRun
	for rows.Next() {
		run, err := scanReviewRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate task review runs: %w", err)
	}
	return runs, nil
}

// CancelInFlightTaskReviewRuns closes runs left pending/running by a previous
// process. Returns the number of runs cancelled.
func (r *Repository) CancelInFlightTaskReviewRuns(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_review_runs
		SET status = ?, error_code = ?, error_message = ?, completed_at = ?
		WHERE status IN ('pending', 'running')
	`), string(models.ReviewRunCancelled), restartCancelCode, restartCancelReason, now)
	if err != nil {
		return 0, fmt.Errorf("failed to cancel in-flight task review runs: %w", err)
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

// CreateTaskReviewFindings inserts findings in a single transaction so a
// partially-anchored review is never visible to a reader.
func (r *Repository) CreateTaskReviewFindings(ctx context.Context, findings []*models.TaskReviewFinding) error {
	if len(findings) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin task review findings tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	stmt := tx.Rebind(`INSERT INTO task_review_findings (` + reviewFindingColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	for _, f := range findings {
		applyFindingDefaults(f, now)
		if _, execErr := tx.ExecContext(ctx, stmt,
			f.ID, f.RunID, f.TaskID, f.RepositoryID, f.RepositoryName, f.FilePath,
			f.StartLine, f.EndLine, f.Side, string(f.Severity), f.Category, f.Title,
			f.Body, f.Suggestion, f.AnchorText, f.FileDiffHash, string(f.Status),
			f.ResolvedAt, f.CreatedAt, f.UpdatedAt,
		); execErr != nil {
			return fmt.Errorf("failed to insert task review finding: %w", execErr)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit task review findings: %w", err)
	}
	return nil
}

func applyFindingDefaults(f *models.TaskReviewFinding, now time.Time) {
	if f.ID == "" {
		f.ID = uuid.New().String()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = now
	}
	f.UpdatedAt = now
	if f.Status == "" {
		f.Status = models.ReviewFindingOpen
	}
	if f.Side == "" {
		f.Side = models.ReviewSideAdditions
	}
}

// ListTaskReviewFindings returns every finding for a task, ordered for stable
// rendering: repository, then file, then line.
func (r *Repository) ListTaskReviewFindings(ctx context.Context, taskID string) ([]*models.TaskReviewFinding, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT `+reviewFindingColumns+` FROM task_review_findings
		 WHERE task_id = ?
		 ORDER BY repository_name, file_path, start_line, id`), taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to list task review findings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var findings []*models.TaskReviewFinding
	for rows.Next() {
		f, scanErr := scanReviewFinding(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		findings = append(findings, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate task review findings: %w", err)
	}
	return findings, nil
}

// GetTaskReviewFinding retrieves a single finding by ID.
func (r *Repository) GetTaskReviewFinding(ctx context.Context, findingID string) (*models.TaskReviewFinding, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+reviewFindingColumns+` FROM task_review_findings WHERE id = ?`), findingID)
	f, err := scanReviewFinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", models.ErrTaskReviewFindingNotFound, findingID)
	}
	return f, err
}

// UpdateTaskReviewFindingStatus sets a finding's status and resolved_at.
func (r *Repository) UpdateTaskReviewFindingStatus(ctx context.Context, findingID string, status models.ReviewFindingStatus, resolvedAt *time.Time) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_review_findings SET status = ?, resolved_at = ?, updated_at = ? WHERE id = ?
	`), string(status), resolvedAt, time.Now().UTC(), findingID)
	if err != nil {
		return fmt.Errorf("failed to update task review finding status: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", models.ErrTaskReviewFindingNotFound, findingID)
	}
	return nil
}

// DeleteSupersededTaskReviewFindings removes still-open findings from earlier
// runs that anchor to the same place with the same title as one of keys. A
// re-review therefore refreshes an issue instead of listing it twice, while
// findings the human already resolved or dismissed stay untouched.
func (r *Repository) DeleteSupersededTaskReviewFindings(ctx context.Context, taskID, runID string, keys []models.ReviewFindingKey) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin supersede tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// RETURNING id rather than a bare row count: a connected client holds the
	// old findings in memory and needs the exact ids to drop, otherwise a
	// re-review shows the superseded finding alongside its replacement.
	// One statement per key keeps the SQL simple and stays well inside SQLite's
	// bound-parameter limit even for a large review.
	stmt := tx.Rebind(`DELETE FROM task_review_findings
		WHERE task_id = ? AND run_id != ? AND status = ?
			AND repository_name = ? AND file_path = ?
			AND start_line = ? AND end_line = ? AND title = ?
		RETURNING id`)
	var deleted []string
	for _, k := range keys {
		rows, execErr := tx.QueryContext(ctx, stmt, taskID, runID,
			string(models.ReviewFindingOpen), k.RepositoryName, k.FilePath,
			k.StartLine, k.EndLine, k.Title)
		if execErr != nil {
			return nil, fmt.Errorf("failed to delete superseded task review finding: %w", execErr)
		}
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("failed to scan superseded finding id: %w", scanErr)
			}
			deleted = append(deleted, id)
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("failed to iterate superseded finding ids: %w", rowsErr)
		}
		_ = rows.Close()
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit supersede tx: %w", err)
	}
	return deleted, nil
}

// DeleteTaskReviewByTask removes every run and finding for a task. Findings go
// first so the run FK never dangles on a connection without enforced FKs.
func (r *Repository) DeleteTaskReviewByTask(ctx context.Context, taskID string) error {
	statements := []string{
		`DELETE FROM task_review_findings WHERE task_id = ?`,
		`DELETE FROM task_review_runs WHERE task_id = ?`,
	}
	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(stmt), taskID); err != nil {
			return fmt.Errorf("failed to delete task review state: %w", err)
		}
	}
	return nil
}

// DeleteTaskReviewByWorkspace removes review state for every task in a
// workspace. The worker-scoped E2E reset needs this: task-scoped entities must
// be cleared before the tasks themselves are deleted.
func (r *Repository) DeleteTaskReviewByWorkspace(ctx context.Context, workspaceID string) error {
	statements := []string{
		`DELETE FROM task_review_findings WHERE task_id IN (SELECT id FROM tasks WHERE workspace_id = ?)`,
		`DELETE FROM task_review_runs WHERE task_id IN (SELECT id FROM tasks WHERE workspace_id = ?)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(stmt), workspaceID); err != nil {
			return fmt.Errorf("failed to delete workspace task review state: %w", err)
		}
	}
	return nil
}

// reviewRowScanner is satisfied by both *sql.Row and *sql.Rows.
type reviewRowScanner interface {
	Scan(dest ...any) error
}

func scanReviewRun(s reviewRowScanner) (*models.TaskReviewRun, error) {
	run := &models.TaskReviewRun{}
	var trigger, status string
	var completedAt sql.NullTime
	err := s.Scan(&run.ID, &run.TaskID, &run.SessionID, &trigger, &run.WorkflowStepID,
		&run.AgentID, &run.Model, &status, &run.ErrorCode, &run.ErrorMessage, &run.Summary,
		&run.FindingCount, &run.FileCount, &run.RepositoryCount, &run.PromptTokens,
		&run.ResponseTokens, &run.DurationMs, &run.EntryID, &run.CreatedAt, &completedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan task review run: %w", err)
	}
	run.Trigger = models.ReviewRunTrigger(trigger)
	run.Status = models.ReviewRunStatus(status)
	if completedAt.Valid {
		t := completedAt.Time.UTC()
		run.CompletedAt = &t
	}
	return run, nil
}

func scanReviewFinding(s reviewRowScanner) (*models.TaskReviewFinding, error) {
	f := &models.TaskReviewFinding{}
	var severity, status string
	var resolvedAt sql.NullTime
	err := s.Scan(&f.ID, &f.RunID, &f.TaskID, &f.RepositoryID, &f.RepositoryName, &f.FilePath,
		&f.StartLine, &f.EndLine, &f.Side, &severity, &f.Category, &f.Title, &f.Body,
		&f.Suggestion, &f.AnchorText, &f.FileDiffHash, &status, &resolvedAt,
		&f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan task review finding: %w", err)
	}
	f.Severity = models.ReviewSeverity(severity)
	f.Status = models.ReviewFindingStatus(status)
	if resolvedAt.Valid {
		t := resolvedAt.Time.UTC()
		f.ResolvedAt = &t
	}
	return f, nil
}

// SupersedeKeysFor builds the deduplicated supersede key set for a batch of
// findings, so the delete pass runs one statement per distinct anchor.
func SupersedeKeysFor(findings []*models.TaskReviewFinding) []models.ReviewFindingKey {
	seen := make(map[string]struct{}, len(findings))
	keys := make([]models.ReviewFindingKey, 0, len(findings))
	for _, f := range findings {
		k := f.SupersedeKey()
		id := strings.Join([]string{
			k.RepositoryName, k.FilePath,
			fmt.Sprint(k.StartLine), fmt.Sprint(k.EndLine), k.Title,
		}, "\x00")
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		keys = append(keys, k)
	}
	return keys
}
