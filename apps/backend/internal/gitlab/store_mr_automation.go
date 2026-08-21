package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const createMRAutomationTablesSQL = `
	CREATE TABLE IF NOT EXISTS gitlab_task_mr_options (
		task_id TEXT PRIMARY KEY,
		auto_fix_enabled BOOLEAN NOT NULL DEFAULT 0,
		auto_merge_enabled BOOLEAN NOT NULL DEFAULT 0,
		auto_fix_prompt_override TEXT,
		prompt_on_review_requested BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_merged BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_closed BOOLEAN NOT NULL DEFAULT 0,
		review_reviewer_username TEXT NOT NULL DEFAULT '',
		automation_revision INTEGER NOT NULL DEFAULT 0,
		mr_scope_migrated_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	-- Per-MR automation switches. Source of truth for the five switches
	-- (auto-fix, auto-merge, and the three lifecycle notifications) that
	-- gitlab_task_mr_options used to hold task-wide. Keyed by the same
	-- four-part MR identity as gitlab_task_mr_state, so a task with two
	-- linked MRs configures each independently. See
	-- migrateTaskMROptionsToMRScope.
	CREATE TABLE IF NOT EXISTS gitlab_task_mr_automation_options (
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL,
		mr_iid INTEGER NOT NULL,
		auto_fix_enabled BOOLEAN NOT NULL DEFAULT 0,
		auto_merge_enabled BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_review_requested BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_merged BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_closed BOOLEAN NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (task_id, repository_id, project_path, mr_iid),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS gitlab_task_mr_state (
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL,
		mr_iid INTEGER NOT NULL,
		review_request_initialized BOOLEAN NOT NULL DEFAULT 0,
		last_review_requested BOOLEAN NOT NULL DEFAULT 0,
		last_observed_state TEXT NOT NULL DEFAULT '',
		last_lifecycle_event TEXT NOT NULL DEFAULT '',
		last_lifecycle_prompt_at DATETIME,
		last_lifecycle_session_id TEXT,
		last_error TEXT,
		last_sync_error TEXT,
		last_fix_signature TEXT NOT NULL DEFAULT '',
		last_fix_checkpoint_json TEXT NOT NULL DEFAULT '',
		last_fix_enqueued_at DATETIME,
		last_fix_session_id TEXT,
		auto_fix_round_count INTEGER NOT NULL DEFAULT 0,
		auto_fix_exhausted_at DATETIME,
		last_merge_signature TEXT NOT NULL DEFAULT '',
		last_merge_attempt_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (task_id, repository_id, project_path, mr_iid),
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
`

// createMRAutomationTables is called from Store.createTables. On a fresh DB
// the CREATE TABLE IF NOT EXISTS above already includes every column,
// including the auto-fix/auto-merge ones this task added. On a DB created
// between #2125 merging and this task landing, both tables already exist
// without those columns — CREATE TABLE IF NOT EXISTS is a no-op there, so
// migrateMRAutomationAutomationColumns (called from Store.createTables
// after this) adds them via idempotent ALTER TABLE.
func (s *Store) createMRAutomationTables() error {
	_, err := s.db.Exec(createMRAutomationTablesSQL)
	return err
}

// migrateMRAutomationAutomationColumns adds the auto-fix/auto-merge columns
// (C9) to gitlab_task_mr_options and gitlab_task_mr_state for databases
// that already have those tables from before this task.
func (s *Store) migrateMRAutomationAutomationColumns() error {
	optionsColumns := []struct{ name, ddl string }{
		{"auto_fix_enabled", sqlBooleanDefaultFalse},
		{"auto_merge_enabled", sqlBooleanDefaultFalse},
		{"auto_fix_prompt_override", "TEXT"},
		{"automation_revision", sqlIntegerDefaultZero},
		// Guards the one-time fan-out of the legacy task-wide switches onto
		// per-MR rows — see migrateTaskMROptionsToMRScope.
		{"mr_scope_migrated_at", "DATETIME"},
	}
	if err := addMissingColumns(s, "gitlab_task_mr_options", optionsColumns); err != nil {
		return err
	}
	stateColumns := []struct{ name, ddl string }{
		{"last_fix_signature", sqlTextDefaultEmpty},
		{"last_fix_checkpoint_json", sqlTextDefaultEmpty},
		{"last_fix_enqueued_at", sqlDateTime},
		{"last_fix_session_id", "TEXT"},
		{"auto_fix_round_count", sqlIntegerDefaultZero},
		{"auto_fix_exhausted_at", sqlDateTime},
		{"last_merge_signature", sqlTextDefaultEmpty},
		{"last_merge_attempt_at", sqlDateTime},
	}
	return addMissingColumns(s, "gitlab_task_mr_state", stateColumns)
}

// legacyMROptionsRow is one pre-scope-migration gitlab_task_mr_options row,
// read only so its task-wide switch values can be fanned out onto the task's
// linked MRs exactly once.
type legacyMROptionsRow struct {
	taskID                                                       string
	autoFix, autoMerge, promptReview, promptMerged, promptClosed bool
}

// migrateTaskMROptionsToMRScope seeds gitlab_task_mr_automation_options rows
// from each pre-upgrade gitlab_task_mr_options row's legacy switches, fanning
// each task's values out onto every gitlab_task_mrs row currently linked to
// it. Guarded by mr_scope_migrated_at, stamped in the same transaction as the
// fan-out insert: without the marker, replaying this on every boot would
// re-enable a switch the user has since turned off for one MR, and an MR
// linked to the task after migration would inherit the legacy value instead
// of starting all-off (which ON CONFLICT DO NOTHING alone cannot prevent).
// Mirrors github.Store.migrateTaskCIOptionsToPRScope.
func (s *Store) migrateTaskMROptionsToMRScope() error {
	legacy, err := s.unmigratedMROptionRows()
	if err != nil {
		return err
	}
	for _, row := range legacy {
		if err := s.fanOutMROptionsToMRScope(row); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) unmigratedMROptionRows() ([]legacyMROptionsRow, error) {
	rows, err := s.ro.Query(`
		SELECT task_id, auto_fix_enabled, auto_merge_enabled, prompt_on_review_requested,
			prompt_on_merged, prompt_on_closed
		FROM gitlab_task_mr_options
		WHERE mr_scope_migrated_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list unmigrated task MR options: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var legacy []legacyMROptionsRow
	for rows.Next() {
		var row legacyMROptionsRow
		if err := rows.Scan(&row.taskID, &row.autoFix, &row.autoMerge,
			&row.promptReview, &row.promptMerged, &row.promptClosed); err != nil {
			return nil, fmt.Errorf("scan unmigrated task MR options: %w", err)
		}
		legacy = append(legacy, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unmigrated task MR options: %w", err)
	}
	return legacy, nil
}

func (s *Store) fanOutMROptionsToMRScope(row legacyMROptionsRow) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	if _, err := tx.Exec(`
		INSERT INTO gitlab_task_mr_automation_options (
			task_id, repository_id, project_path, mr_iid, auto_fix_enabled, auto_merge_enabled,
			prompt_on_review_requested, prompt_on_merged, prompt_on_closed, created_at, updated_at
		)
		SELECT task_id, repository_id, project_path, mr_iid, ?, ?, ?, ?, ?, ?, ?
		FROM gitlab_task_mrs
		WHERE task_id = ?
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO NOTHING`,
		row.autoFix, row.autoMerge, row.promptReview, row.promptMerged, row.promptClosed,
		now, now, row.taskID); err != nil {
		return fmt.Errorf("fan out task MR options for %s: %w", row.taskID, err)
	}
	if _, err := tx.Exec(
		`UPDATE gitlab_task_mr_options SET mr_scope_migrated_at = ? WHERE task_id = ?`, now, row.taskID,
	); err != nil {
		return fmt.Errorf("stamp mr_scope_migrated_at for %s: %w", row.taskID, err)
	}
	return tx.Commit()
}

func addMissingColumns(s *Store, table string, columns []struct{ name, ddl string }) error {
	existing, err := s.tableColumns(table)
	if err != nil {
		return err
	}
	for _, column := range columns {
		if _, ok := existing[column.name]; ok {
			continue
		}
		if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column.name, column.ddl)); err != nil {
			return fmt.Errorf("migrate %s.%s: %w", table, column.name, err)
		}
	}
	return nil
}

// execContext is satisfied by *sqlx.Tx (and *sqlx.DB), narrowed to the one
// method deleteMRAutomationForWorkspace needs so it can run inside the
// existing ResetWorkspaceE2E transaction.
type execContext interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

const mrAutomationOptionsSelectCols = `task_id, automation_revision, auto_fix_enabled, auto_merge_enabled, auto_fix_prompt_override,
	prompt_on_review_requested, prompt_on_merged, prompt_on_closed,
	review_reviewer_username, created_at, updated_at`

// GetTaskMRAutomationOptions returns a task's MR automation options, or an
// implicit all-false default when no row has been persisted yet (AC1).
func (s *Store) GetTaskMRAutomationOptions(ctx context.Context, taskID string) (*TaskMRAutomationOptions, error) {
	var row TaskMRAutomationOptions
	err := s.ro.GetContext(ctx, &row, `
		SELECT `+mrAutomationOptionsSelectCols+`
		FROM gitlab_task_mr_options WHERE task_id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return &TaskMRAutomationOptions{TaskID: taskID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// boolPatchValue splits an optional patch field into a "was this field
// present" flag and its value, matching the shape a CASE WHEN ? THEN ? END
// clause needs. Mirrors internal/github's store.go helper of the same name.
func boolPatchValue(value *bool) (bool, bool) {
	if value == nil {
		return false, false
	}
	return true, *value
}

// UpdateTaskMRAutomationOptions applies a partial update to the genuinely
// task-level MR automation fields: the auto-fix prompt override and the
// server-resolved reviewer username. The five automation switches are
// per-MR — see UpdateTaskMRAutomationOptionsForMR.
//
// Every column write is a single atomic UPDATE ... CASE WHEN <field was
// patched> THEN <new value> ELSE <column> END statement — there is no
// read-modify-write step for the values themselves, so two concurrent PATCHes
// touching different fields cannot lose one side's change to the other's
// stale snapshot (this does NOT hold under a bare transaction + full-row
// read-then-upsert on PostgreSQL, which only guarantees isolation between
// statements a transaction actually executes, not implicit serialization of
// concurrent read-then-write pairs).
//
// The one read this still performs (of the pre-patch row) exists solely to
// detect a reviewer-identity change, which invalidates every linked MR's
// review-request baseline at once.
func (s *Store) UpdateTaskMRAutomationOptions(
	ctx context.Context, taskID string, patch TaskMRAutomationPatch, reviewerUsername *string,
) (*TaskMRAutomationOptions, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := updateTaskMRAutomationOptionsTx(ctx, tx, taskID, patch, reviewerUsername); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTaskMRAutomationOptions(ctx, taskID)
}

// UpdateTaskMRAutomationOptionsWithMRs applies task-level fields and one
// per-MR switch patch in the same transaction. A mixed PATCH must either
// commit both kinds of settings or leave neither behind.
func (s *Store) UpdateTaskMRAutomationOptionsWithMRs(
	ctx context.Context, taskID string, patch TaskMRAutomationPatch, reviewerUsername *string,
	ids []MRIdentity, switches TaskMRAutomationSwitchPatch,
) (*TaskMRAutomationOptions, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := updateTaskMRAutomationOptionsTx(ctx, tx, taskID, patch, reviewerUsername); err != nil {
		return nil, err
	}
	if err := applyMRSwitchPatchBatchTx(ctx, tx, taskID, ids, switches); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTaskMRAutomationOptions(ctx, taskID)
}

// updateTaskMRAutomationOptionsTx applies the task-level half of an
// automation patch in its caller's transaction.
func updateTaskMRAutomationOptionsTx(
	ctx context.Context, tx *sqlx.Tx, taskID string, patch TaskMRAutomationPatch, reviewerUsername *string,
) error {
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_options (task_id, created_at, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(task_id) DO NOTHING`, taskID, now, now); err != nil {
		return err
	}
	var previous TaskMRAutomationOptions
	if err := tx.GetContext(ctx, &previous,
		`SELECT `+mrAutomationOptionsSelectCols+`
		 FROM gitlab_task_mr_options WHERE task_id = ?`, taskID); err != nil {
		return err
	}

	promptSet := patch.AutoFixPromptOverride != nil
	promptValue := normalizedMRPromptOverride(patch.AutoFixPromptOverride)
	reviewerSet := reviewerUsername != nil
	reviewerValue := ""
	if reviewerSet {
		reviewerValue = *reviewerUsername
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gitlab_task_mr_options SET
			auto_fix_prompt_override = CASE WHEN ? THEN ? ELSE auto_fix_prompt_override END,
			review_reviewer_username = CASE WHEN ? THEN ? ELSE review_reviewer_username END,
			automation_revision = automation_revision + 1,
			updated_at = ?
		WHERE task_id = ?`,
		promptSet, promptValue, reviewerSet, reviewerValue, now, taskID); err != nil {
		return err
	}
	// A changed connected GitLab account invalidates every linked MR's
	// review-request baseline, not just one MR's: a baseline recorded against
	// the old identity would otherwise survive and could suppress or misfire
	// the next prompt evaluated against the new one.
	if reviewerSet && previous.ReviewReviewerUsername != reviewerValue {
		if err := resetReviewBaselinesForTask(ctx, tx, taskID); err != nil {
			return err
		}
	}
	return nil
}

const mrAutomationSwitchSelectCols = `task_id, repository_id, project_path, mr_iid,
	auto_fix_enabled, auto_merge_enabled, prompt_on_review_requested,
	prompt_on_merged, prompt_on_closed, created_at, updated_at`

// GetTaskMRAutomationOptionsForMR returns one linked MR's automation
// switches, or all-off defaults when nothing has been persisted for it yet.
func (s *Store) GetTaskMRAutomationOptionsForMR(
	ctx context.Context, taskID string, id MRIdentity,
) (*TaskMRAutomationOptionsForMR, error) {
	var row TaskMRAutomationOptionsForMR
	err := s.ro.GetContext(ctx, &row, `
		SELECT `+mrAutomationSwitchSelectCols+`
		FROM gitlab_task_mr_automation_options
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		taskID, id.RepositoryID, id.ProjectPath, id.MRIID)
	if errors.Is(err, sql.ErrNoRows) {
		return &TaskMRAutomationOptionsForMR{
			TaskID: taskID, RepositoryID: id.RepositoryID,
			ProjectPath: id.ProjectPath, MRIID: id.MRIID,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListTaskMRAutomationOptions returns every stored per-MR switch row for a task.
func (s *Store) ListTaskMRAutomationOptions(ctx context.Context, taskID string) ([]*TaskMRAutomationOptionsForMR, error) {
	var rows []TaskMRAutomationOptionsForMR
	if err := s.ro.SelectContext(ctx, &rows, `
		SELECT `+mrAutomationSwitchSelectCols+`
		FROM gitlab_task_mr_automation_options
		WHERE task_id = ? ORDER BY project_path ASC, mr_iid ASC`, taskID); err != nil {
		return nil, err
	}
	out := make([]*TaskMRAutomationOptionsForMR, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// UpdateTaskMRAutomationOptionsForMR applies a partial update to one linked
// MR's five automation switches, upserting the row when absent. It carries
// the same atomic CASE-WHEN write and single pre-patch read as
// UpdateTaskMRAutomationOptions; here the read decides which of that MR's
// checkpoints need resetting.
func (s *Store) UpdateTaskMRAutomationOptionsForMR(
	ctx context.Context, taskID string, id MRIdentity, patch TaskMRAutomationSwitchPatch,
) (*TaskMRAutomationOptionsForMR, error) {
	if err := s.UpdateTaskMRAutomationOptionsForMRs(ctx, taskID, []MRIdentity{id}, patch); err != nil {
		return nil, err
	}
	return s.GetTaskMRAutomationOptionsForMR(ctx, taskID, id)
}

// UpdateTaskMRAutomationOptionsForMRs applies one switch patch to every
// identity in ids inside a SINGLE transaction. The fan-out case (a PATCH or
// MCP call that names no MR, so the change applies to every linked MR) must
// not be a loop of independent transactions: a failure partway through would
// leave the switch armed on the MRs already committed while returning an
// error to the caller, and auto-merge would then act on those MRs even though
// the operation reported failure. All-or-nothing instead.
func (s *Store) UpdateTaskMRAutomationOptionsForMRs(
	ctx context.Context, taskID string, ids []MRIdentity, patch TaskMRAutomationSwitchPatch,
) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := applyMRSwitchPatchBatchTx(ctx, tx, taskID, ids, patch); err != nil {
		return err
	}
	return tx.Commit()
}

// applyMRSwitchPatchBatchTx applies a switch patch to every identity inside
// an existing transaction so task-level and per-MR changes can commit together.
func applyMRSwitchPatchBatchTx(
	ctx context.Context, tx *sqlx.Tx, taskID string, ids []MRIdentity, patch TaskMRAutomationSwitchPatch,
) error {
	if !patch.HasAny() {
		return nil
	}
	now := time.Now().UTC()
	fields := mrAutomationSwitchFields(patch)
	for _, id := range ids {
		if err := applyMRSwitchPatchTx(ctx, tx, taskID, id, now, fields); err != nil {
			return err
		}
	}
	return nil
}

// applyMRSwitchPatchTx upserts one MR's switch row, applies the patch, and
// runs that MR's checkpoint resets — all against the caller's transaction, so
// several MRs can be updated atomically. Split out of
// UpdateTaskMRAutomationOptionsForMRs to keep it under the function-length
// limit.
func applyMRSwitchPatchTx(
	ctx context.Context, tx *sqlx.Tx, taskID string, id MRIdentity,
	now time.Time, fields mrAutomationSwitchPatchFields,
) error {
	if err := ensureTaskMRLinkTx(ctx, tx, taskID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_automation_options (
			task_id, repository_id, project_path, mr_iid, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO NOTHING`,
		taskID, id.RepositoryID, id.ProjectPath, id.MRIID, now, now); err != nil {
		return err
	}
	var previous TaskMRAutomationOptionsForMR
	if err := tx.GetContext(ctx, &previous, `
		SELECT `+mrAutomationSwitchSelectCols+`
		FROM gitlab_task_mr_automation_options
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		taskID, id.RepositoryID, id.ProjectPath, id.MRIID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gitlab_task_mr_automation_options SET
			auto_fix_enabled = CASE WHEN ? THEN ? ELSE auto_fix_enabled END,
			auto_merge_enabled = CASE WHEN ? THEN ? ELSE auto_merge_enabled END,
			prompt_on_review_requested = CASE WHEN ? THEN ? ELSE prompt_on_review_requested END,
			prompt_on_merged = CASE WHEN ? THEN ? ELSE prompt_on_merged END,
			prompt_on_closed = CASE WHEN ? THEN ? ELSE prompt_on_closed END,
			updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		fields.autoFixSet, fields.autoFixValue, fields.autoMergeSet, fields.autoMergeValue,
		fields.reviewSet, fields.reviewValue, fields.mergedSet, fields.mergedValue,
		fields.closedSet, fields.closedValue,
		now, taskID, id.RepositoryID, id.ProjectPath, id.MRIID); err != nil {
		return err
	}
	return applyMRAutomationOptionResets(ctx, tx, taskID, id, now, previous, fields)
}

func ensureTaskMRLinkTx(ctx context.Context, tx *sqlx.Tx, taskID string, id MRIdentity) error {
	// A plain SELECT is not enough here. PostgreSQL's READ COMMITTED isolation
	// allows an unlink to delete the association after the read and before the
	// options INSERT, which leaves an orphan row that can re-arm on relink.
	// The no-op UPDATE takes a write lock on the association row and is portable
	// across PostgreSQL and SQLite. The unlink transaction must wait until this
	// transaction commits, then removes the association and its options.
	result, err := tx.ExecContext(ctx, `
		UPDATE gitlab_task_mrs SET updated_at = updated_at
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		taskID, id.RepositoryID, id.ProjectPath, id.MRIID)
	if err != nil {
		return err
	}
	linked, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if linked == 0 {
		return fmt.Errorf("%w: project_path=%s mr_iid=%d", ErrTaskMRNotLinked, id.ProjectPath, id.MRIID)
	}
	return nil
}

// mrAutomationSwitchPatchFields flattens a switch patch into the "was this
// field present, what value" pairs both the atomic UPDATE and the
// reset-decision logic need.
type mrAutomationSwitchPatchFields struct {
	autoFixSet, autoFixValue     bool
	autoMergeSet, autoMergeValue bool
	reviewSet, reviewValue       bool
	mergedSet, mergedValue       bool
	closedSet, closedValue       bool
}

func mrAutomationSwitchFields(patch TaskMRAutomationSwitchPatch) mrAutomationSwitchPatchFields {
	autoFixSet, autoFixValue := boolPatchValue(patch.AutoFixEnabled)
	autoMergeSet, autoMergeValue := boolPatchValue(patch.AutoMergeEnabled)
	reviewSet, reviewValue := boolPatchValue(patch.PromptOnReviewRequested)
	mergedSet, mergedValue := boolPatchValue(patch.PromptOnMerged)
	closedSet, closedValue := boolPatchValue(patch.PromptOnClosed)
	return mrAutomationSwitchPatchFields{
		autoFixSet: autoFixSet, autoFixValue: autoFixValue,
		autoMergeSet: autoMergeSet, autoMergeValue: autoMergeValue,
		reviewSet: reviewSet, reviewValue: reviewValue,
		mergedSet: mergedSet, mergedValue: mergedValue,
		closedSet: closedSet, closedValue: closedValue,
	}
}

// normalizedMRPromptOverride turns an explicit empty-string override into a
// NULL column value (restoring the default prompt), matching GitHub's
// normalizedPromptOverride.
func normalizedMRPromptOverride(override *string) *string {
	if override == nil || *override == "" {
		return nil
	}
	return override
}

// applyMRAutomationOptionResets resets the targeted MR's review-request
// baseline, a terminal event's checkpoint, or the auto-fix round-cap state
// when the corresponding switch actually changed value against the pre-patch
// row. Every reset is scoped to the one MR being patched, so reconfiguring
// one linked MR never re-arms another's checkpoints. Mirrors GitHub's
// applyTaskPRAutomationOptionResets.
func applyMRAutomationOptionResets(
	ctx context.Context, tx execContext, taskID string, id MRIdentity, now time.Time,
	previous TaskMRAutomationOptionsForMR, fields mrAutomationSwitchPatchFields,
) error {
	if fields.reviewSet && previous.PromptOnReviewRequested != fields.reviewValue {
		if err := resetReviewBaselineForMR(ctx, tx, taskID, id); err != nil {
			return err
		}
	}
	if err := resetMRTerminalCheckpointsOnReenable(ctx, tx, taskID, id, now, previous, fields); err != nil {
		return err
	}
	// Re-enabling auto-fix after it was off must clear the round cap and
	// exhaustion so a fresh evaluation pass can dispatch again (AC11).
	if fields.autoFixSet && fields.autoFixValue && !previous.AutoFixEnabled {
		if err := resetMRAutoFixState(ctx, tx, taskID, id, now); err != nil {
			return err
		}
	}
	return nil
}

// resetMRTerminalCheckpointsOnReenable resets a terminal event's (merged or
// closed) checkpoint when its switch flips from off to on, so an MR already
// checkpointed in that state can fire again — mirrors GitHub's
// shouldResetTerminalPrompt/resetTaskCITerminalCheckpoint. Split out of
// applyMRAutomationOptionResets to keep that function under the cyclomatic
// complexity limit.
func resetMRTerminalCheckpointsOnReenable(
	ctx context.Context, tx execContext, taskID string, id MRIdentity, now time.Time,
	previous TaskMRAutomationOptionsForMR, fields mrAutomationSwitchPatchFields,
) error {
	if fields.mergedSet && fields.mergedValue && !previous.PromptOnMerged {
		if err := resetMRTerminalCheckpoint(ctx, tx, taskID, id, gitlabStateMerged, now); err != nil {
			return err
		}
	}
	if fields.closedSet && fields.closedValue && !previous.PromptOnClosed {
		if err := resetMRTerminalCheckpoint(ctx, tx, taskID, id, gitlabStateClosed, now); err != nil {
			return err
		}
	}
	return nil
}

// resetMRAutoFixState clears one MR's auto-fix round-cap/checkpoint columns,
// mirroring GitHub's resetTaskCIAutoFixState. A previously recorded
// exhaustion error is cleared along with it; any other kind of error (sync,
// merge) is left untouched.
func resetMRAutoFixState(ctx context.Context, exec execContext, taskID string, id MRIdentity, now time.Time) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state
		SET auto_fix_round_count = 0,
		    last_fix_signature = '',
		    last_fix_checkpoint_json = '',
		    last_fix_enqueued_at = NULL,
		    last_fix_session_id = NULL,
		    last_error = CASE WHEN auto_fix_exhausted_at IS NOT NULL THEN NULL ELSE last_error END,
		    auto_fix_exhausted_at = NULL,
		    updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		now, taskID, id.RepositoryID, id.ProjectPath, id.MRIID)
	return err
}

const mrLifecycleStateSelectCols = `task_id, repository_id, project_path, mr_iid,
	review_request_initialized, last_review_requested, last_observed_state,
	last_lifecycle_event, last_lifecycle_prompt_at, last_lifecycle_session_id,
	last_error, last_sync_error,
	last_fix_signature, last_fix_checkpoint_json, last_fix_enqueued_at, last_fix_session_id,
	auto_fix_round_count, auto_fix_exhausted_at, last_merge_signature, last_merge_attempt_at,
	created_at, updated_at`

// GetTaskMRLifecycleState returns the checkpoint row for one linked MR, or
// nil when no evaluation has run yet (silent-baseline case, AC10).
func (s *Store) GetTaskMRLifecycleState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) (*TaskMRLifecycleState, error) {
	var row TaskMRLifecycleState
	err := s.ro.GetContext(ctx, &row, `
		SELECT `+mrLifecycleStateSelectCols+` FROM gitlab_task_mr_state
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		taskID, repositoryID, projectPath, mrIID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListTaskMRLifecycleStates returns every checkpoint row for a task.
func (s *Store) ListTaskMRLifecycleStates(ctx context.Context, taskID string) ([]*TaskMRLifecycleState, error) {
	var rows []TaskMRLifecycleState
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+mrLifecycleStateSelectCols+` FROM gitlab_task_mr_state
		 WHERE task_id = ? ORDER BY project_path ASC, mr_iid ASC`, taskID); err != nil {
		return nil, err
	}
	out := make([]*TaskMRLifecycleState, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// SetTaskMRReviewRequestState stamps the review-request baseline/observation
// used to detect a false→true edge (AC10-AC12). A single upsert that only
// ever writes its own two columns (plus updated_at) — no read, so it cannot
// clobber a concurrent SetTaskMRObservedState/RecordTaskMRAutomationError
// write to this same row.
func (s *Store) SetTaskMRReviewRequestState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, requested bool) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid,
			review_request_initialized, last_review_requested, created_at, updated_at
		) VALUES (?, ?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			review_request_initialized = 1,
			last_review_requested = excluded.last_review_requested,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, projectPath, mrIID, requested, now, now)
	return err
}

// SetTaskMRObservedState stamps the last-observed MR state, used to detect a
// terminal-state transition (AC13-AC15). Entering a non-terminal state clears
// last_lifecycle_event so a later re-entry into the same terminal state can
// fire its prompt again (matches GitHub's SetTaskPRObservedState).
func (s *Store) SetTaskMRObservedState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, state string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, last_observed_state, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			last_observed_state = excluded.last_observed_state,
			last_lifecycle_event = CASE
				WHEN excluded.last_observed_state IN (?, ?)
				THEN gitlab_task_mr_state.last_lifecycle_event
				ELSE '' END,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, projectPath, mrIID, state, now, now, gitlabStateMerged, gitlabStateClosed)
	return err
}

// RecordTaskMRLifecyclePrompt persists an accepted lifecycle prompt
// checkpoint, clearing any prior error for this MR. Each field is
// individually gated so this upsert cannot overwrite a concurrent mutator's
// unrelated column (matches GitHub's RecordTaskPRLifecyclePrompt).
func (s *Store) RecordTaskMRLifecyclePrompt(ctx context.Context, prompt TaskMRLifecyclePrompt) error {
	promptedAt := prompt.PromptedAt
	if promptedAt.IsZero() {
		promptedAt = time.Now().UTC()
	}
	now := time.Now().UTC()
	var sessionID *string
	if prompt.SessionID != "" {
		sessionID = &prompt.SessionID
	}
	isReviewRequested := prompt.Event == mrLifecycleEventReviewRequested
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, review_request_initialized,
			last_review_requested, last_observed_state, last_lifecycle_event,
			last_lifecycle_prompt_at, last_lifecycle_session_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			review_request_initialized = CASE
				WHEN excluded.last_lifecycle_event = ? THEN 1
				ELSE gitlab_task_mr_state.review_request_initialized END,
			last_review_requested = CASE
				WHEN excluded.last_lifecycle_event = ? THEN excluded.last_review_requested
				ELSE gitlab_task_mr_state.last_review_requested END,
			last_observed_state = CASE
				WHEN excluded.last_observed_state <> '' THEN excluded.last_observed_state
				ELSE gitlab_task_mr_state.last_observed_state END,
			last_lifecycle_event = excluded.last_lifecycle_event,
			last_lifecycle_prompt_at = excluded.last_lifecycle_prompt_at,
			last_lifecycle_session_id = excluded.last_lifecycle_session_id,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		prompt.TaskID, prompt.RepositoryID, prompt.ProjectPath, prompt.MRIID, isReviewRequested,
		prompt.ReviewRequested, prompt.ObservedState, prompt.Event, promptedAt, sessionID, now, now,
		mrLifecycleEventReviewRequested, mrLifecycleEventReviewRequested)
	return err
}

// RecordTaskMRAutomationError persists a lifecycle *delivery* evaluation
// error (evaluate/dispatch failure) against the per-MR checkpoint row
// without aborting the caller's poll loop (AC25). Writes only last_error —
// see TaskMRLifecycleState's field comment for why this is kept separate
// from the poller's sync-error column.
func (s *Store) RecordTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, projectPath, mrIID, message, now, now)
	return err
}

// ClearTaskMRAutomationError clears a previously recorded lifecycle delivery
// error. A blind conditional UPDATE — idempotent, and does not require the
// row to already exist (matches GitHub's ClearTaskCIError). Only touches
// last_error; a sync success does not imply delivery has recovered, so it
// must not clear this column (that call goes through
// ClearTaskMRSyncError instead).
func (s *Store) ClearTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state SET last_error = NULL, updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		time.Now().UTC(), taskID, repositoryID, projectPath, mrIID)
	return err
}

// RecordTaskMRSyncError persists a poller sync failure (SyncTaskMRStrict)
// against the per-MR checkpoint row, in its own column so it cannot clobber
// (or be clobbered by) a concurrently recorded/cleared lifecycle-delivery
// error in last_error.
func (s *Store) RecordTaskMRSyncError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, last_sync_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			last_sync_error = excluded.last_sync_error,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, projectPath, mrIID, message, now, now)
	return err
}

// ClearTaskMRSyncError clears a previously recorded sync error after a sync
// succeeds. A blind conditional UPDATE — idempotent, does not require the
// row to already exist, and only touches last_sync_error.
func (s *Store) ClearTaskMRSyncError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state SET last_sync_error = NULL, updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		time.Now().UTC(), taskID, repositoryID, projectPath, mrIID)
	return err
}

// RebindTaskMRReviewer replaces the stored reviewer username and, when it
// changed, atomically clears every review-request baseline for the task
// (risk 3 in the plan) — in the same transaction as the username write, so a
// failure between the two writes cannot leave the new identity paired with
// the old identity's baseline. The identity comparison itself inherently
// needs a read (matches GitHub's RebindTaskPRReviewer).
func (s *Store) RebindTaskMRReviewer(ctx context.Context, taskID, username string) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var current string
	err = tx.GetContext(ctx, &current, `SELECT review_reviewer_username FROM gitlab_task_mr_options WHERE task_id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if current == username {
		return false, tx.Commit()
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx,
		`UPDATE gitlab_task_mr_options SET review_reviewer_username = ?, updated_at = ? WHERE task_id = ?`,
		username, now, taskID); err != nil {
		return false, err
	}
	if err := resetReviewBaselinesForTask(ctx, tx, taskID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// resetReviewBaselinesForTask clears every linked MR's review-request
// baseline. Reserved for a change of the task-level reviewer identity, which
// invalidates all of them at once; a single MR's switch flip goes through
// resetReviewBaselineForMR instead.
func resetReviewBaselinesForTask(ctx context.Context, exec execContext, taskID string) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state
		SET review_request_initialized = 0, last_review_requested = 0, updated_at = ?
		WHERE task_id = ?`, time.Now().UTC(), taskID)
	return err
}

// resetReviewBaselineForMR clears one linked MR's review-request baseline.
func resetReviewBaselineForMR(ctx context.Context, exec execContext, taskID string, id MRIdentity) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state
		SET review_request_initialized = 0, last_review_requested = 0, updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		time.Now().UTC(), taskID, id.RepositoryID, id.ProjectPath, id.MRIID)
	return err
}

// resetMRTerminalCheckpoint clears the terminal checkpoint for one MR when it
// is currently observed in (or was last recorded as) the given terminal
// state, so a switch re-enabled after being off can re-evaluate and re-fire
// for an MR that reached that state while the switch was disabled. Matches
// GitHub's resetTaskCITerminalCheckpoint.
func resetMRTerminalCheckpoint(
	ctx context.Context, exec execContext, taskID string, id MRIdentity, state string, now time.Time,
) error {
	_, err := exec.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state
		SET last_observed_state = '',
		    last_lifecycle_event = '',
		    last_lifecycle_prompt_at = NULL,
		    last_lifecycle_session_id = NULL,
		    updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?
		  AND (last_observed_state = ? OR last_lifecycle_event = ?)`,
		now, taskID, id.RepositoryID, id.ProjectPath, id.MRIID, state, state)
	return err
}

// ListAutomationSubscribedTaskMRs returns every linked MR (gitlab_task_mrs
// row) that has at least one lifecycle switch OR auto-fix OR auto-merge
// enabled. Drives the poller's sync pass (AC22); widened from the #2125
// lifecycle-only ListLifecycleSubscribedTaskMRs so auto-fix and auto-merge
// get evaluated on the same poll without a second query. The join is on the
// full MR identity, not just task_id, so a task's unconfigured MRs are not
// polled just because a sibling MR has a switch on.
func (s *Store) ListAutomationSubscribedTaskMRs(ctx context.Context) ([]*TaskMR, error) {
	var mrs []TaskMR
	if err := s.ro.SelectContext(ctx, &mrs, `
		SELECT `+taskMRSelectColsQualified+` FROM gitlab_task_mrs gtm
		INNER JOIN gitlab_task_mr_automation_options o
			ON o.task_id = gtm.task_id
			AND o.repository_id = gtm.repository_id
			AND o.project_path = gtm.project_path
			AND o.mr_iid = gtm.mr_iid
		WHERE o.prompt_on_review_requested = 1 OR o.prompt_on_merged = 1 OR o.prompt_on_closed = 1
			OR o.auto_fix_enabled = 1 OR o.auto_merge_enabled = 1
		ORDER BY gtm.created_at ASC`); err != nil {
		return nil, err
	}
	out := make([]*TaskMR, 0, len(mrs))
	for i := range mrs {
		out = append(out, &mrs[i])
	}
	return out, nil
}

// RecordTaskMRFixAttempt persists an auto-fix prompt dispatch: the new
// feedback checkpoint/signature, the session it was sent to, and (when
// IncrementRound) one added round toward the per-MR cap. Mirrors GitHub's
// RecordTaskCIFixAttempt.
func (s *Store) RecordTaskMRFixAttempt(ctx context.Context, attempt TaskMRFixAttempt) error {
	ctx = context.WithoutCancel(ctx)
	when := attempt.EnqueuedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	now := time.Now().UTC()
	roundCount := 0
	if attempt.IncrementRound {
		roundCount = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, last_fix_signature, last_fix_checkpoint_json,
			last_fix_enqueued_at, last_fix_session_id, auto_fix_round_count, auto_fix_exhausted_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			last_fix_signature = excluded.last_fix_signature,
			last_fix_checkpoint_json = excluded.last_fix_checkpoint_json,
			last_fix_enqueued_at = excluded.last_fix_enqueued_at,
			last_fix_session_id = excluded.last_fix_session_id,
			auto_fix_round_count = gitlab_task_mr_state.auto_fix_round_count + excluded.auto_fix_round_count,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		attempt.TaskID, attempt.RepositoryID, attempt.ProjectPath, attempt.MRIID, attempt.Signature,
		attempt.CheckpointJSON, when, nullableString(attempt.SessionID), roundCount, now, now)
	return err
}

// RefreshTaskMRFixCheckpoint updates the current feedback checkpoint
// without recording a new prompt dispatch (the empty-delta path — AC7). It
// is called only at the start of a fresh auto-fix cycle, so replacing a
// conflicting row intentionally clears its stale enqueue/session markers.
// Mirrors GitHub's RefreshTaskCIFixCheckpoint.
func (s *Store) RefreshTaskMRFixCheckpoint(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, signature, checkpointJSON string) error {
	ctx = context.WithoutCancel(ctx)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, last_fix_signature, last_fix_checkpoint_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			last_fix_signature = excluded.last_fix_signature,
			last_fix_checkpoint_json = excluded.last_fix_checkpoint_json,
			last_fix_enqueued_at = NULL,
			last_fix_session_id = NULL,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, projectPath, mrIID, signature, checkpointJSON, now, now)
	return err
}

// MarkTaskMRAutoFixExhausted records that auto-fix reached its per-MR round
// cap (AC10). Mirrors GitHub's MarkTaskCIAutoFixExhausted.
func (s *Store) MarkTaskMRAutoFixExhausted(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	ctx = context.WithoutCancel(ctx)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, auto_fix_exhausted_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			auto_fix_exhausted_at = excluded.auto_fix_exhausted_at,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, projectPath, mrIID, now, strings.TrimSpace(message), now, now)
	return err
}

// RecordTaskMRMergeAttempt records an auto-merge attempt signature (AC13),
// clearing any previous automation error. Mirrors GitHub's
// RecordTaskCIMergeAttempt.
func (s *Store) RecordTaskMRMergeAttempt(ctx context.Context, attempt TaskMRMergeAttempt) error {
	ctx = context.WithoutCancel(ctx)
	when := attempt.AttemptedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid, last_merge_signature, last_merge_attempt_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			last_merge_signature = excluded.last_merge_signature,
			last_merge_attempt_at = excluded.last_merge_attempt_at,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		attempt.TaskID, attempt.RepositoryID, attempt.ProjectPath, attempt.MRIID, attempt.Signature, when, now, now)
	return err
}

// nullableString converts an empty string to a nil driver value so the
// column stores SQL NULL rather than "" when no session ID is known.
func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// deleteMRAutomationForWorkspace removes MR automation rows scoped to a
// workspace's tasks, ahead of task deletion in the shared E2E reset (see
// apps/backend/AGENTS.md's E2E reset invariant). The FOREIGN KEY ... ON
// DELETE CASCADE on both tables would also catch this on a real task
// deletion; E2E reset deletes tasks directly via SQL without going through
// that path in every deployment configuration, so this stays explicit.
func (s *Store) deleteMRAutomationForWorkspace(ctx context.Context, tx execContext, workspaceID string) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_state WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`, workspaceID); err != nil {
		return fmt.Errorf("delete gitlab_task_mr_state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_automation_options WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`, workspaceID); err != nil {
		return fmt.Errorf("delete gitlab_task_mr_automation_options: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_options WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`, workspaceID); err != nil {
		return fmt.Errorf("delete gitlab_task_mr_options: %w", err)
	}
	return nil
}
