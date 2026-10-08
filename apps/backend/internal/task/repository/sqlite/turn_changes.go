package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var (
	ErrTurnChangeSetNotFound         = repoerrors.ErrTurnChangeSetNotFound
	ErrTurnChangeSetIdentityConflict = repoerrors.ErrTurnChangeSetIdentityConflict
	ErrTurnChangeRelationship        = repoerrors.ErrTurnChangeRelationship
)

func (r *Repository) CreateTurnChangeSet(ctx context.Context, changeSet *models.TurnChangeSet) error {
	if err := prepareTurnChangeSet(changeSet); err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := validateTurnChangeSetRelationship(ctx, tx, r.db, changeSet); err != nil {
		return err
	}
	if err := assignTurnChangeOrdinal(ctx, tx, r.db, changeSet); err != nil {
		return err
	}
	if err := insertTurnChangeSet(ctx, tx, r.db, changeSet); err != nil {
		return err
	}
	return tx.Commit()
}

func assignTurnChangeOrdinal(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, changeSet *models.TurnChangeSet) error {
	return tx.GetContext(ctx, &changeSet.TurnOrdinal, db.Rebind(`
		SELECT COUNT(*)
		FROM task_session_turns current_turn
		JOIN task_session_turns prior_turn ON prior_turn.task_session_id = current_turn.task_session_id
		WHERE current_turn.id = ?
		  AND (prior_turn.started_at < current_turn.started_at
		    OR (prior_turn.started_at = current_turn.started_at AND prior_turn.created_at < current_turn.created_at)
		    OR (prior_turn.started_at = current_turn.started_at AND prior_turn.created_at = current_turn.created_at AND prior_turn.id <= current_turn.id))
	`), changeSet.TurnID)
}

func prepareTurnChangeSet(changeSet *models.TurnChangeSet) error {
	if changeSet == nil {
		return fmt.Errorf("create turn change set: nil change set")
	}
	if err := validateTurnChangeSetInput(changeSet); err != nil {
		return err
	}
	if changeSet.FallbackAnchor == "" {
		changeSet.FallbackAnchor = "turn-changes:" + changeSet.TurnID
	}
	now := time.Now().UTC()
	if changeSet.CreatedAt.IsZero() {
		changeSet.CreatedAt = now
	}
	if changeSet.UpdatedAt.IsZero() {
		changeSet.UpdatedAt = changeSet.CreatedAt
	}
	overlapJSON, err := encodeTurnChangeOverlaps(changeSet.OverlapIntervals)
	if err != nil {
		return fmt.Errorf("encode turn change overlaps: %w", err)
	}
	changeSet.OverlapIntervalsJSON = overlapJSON
	return nil
}

func validateTurnChangeSetInput(changeSet *models.TurnChangeSet) error {
	if changeSet.ID == "" {
		changeSet.ID = uuid.NewString()
	}
	if changeSet.TaskID == "" || changeSet.TaskSessionID == "" || changeSet.TurnID == "" || changeSet.TaskEnvironmentID == "" {
		return fmt.Errorf("create turn change set: task, session, turn, and environment identities are required")
	}
	if !changeSet.Availability.Valid() {
		return fmt.Errorf("create turn change set: invalid availability %q", changeSet.Availability)
	}
	if !changeSet.Reason.Valid() {
		return fmt.Errorf("create turn change set: invalid reason code %q", changeSet.Reason)
	}
	if changeSet.Revision <= 0 {
		changeSet.Revision = 1
	}
	if changeSet.CaptureEnabled && (changeSet.SettingsUserID == "" || !changeSet.ResolutionKind.Valid()) {
		return fmt.Errorf("create turn change set: enabled policy requires settings identity and resolution kind")
	}
	return nil
}

func validateTurnChangeSetRelationship(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, changeSet *models.TurnChangeSet) error {
	var valid int
	err := tx.GetContext(ctx, &valid, db.Rebind(`
		SELECT 1
		FROM task_sessions s
		JOIN task_session_turns turn_row
		  ON turn_row.id = ? AND turn_row.task_session_id = s.id AND turn_row.task_id = s.task_id
		JOIN task_environments environment
		  ON environment.id = ? AND environment.task_id = s.task_id
		WHERE s.id = ? AND s.task_id = ? AND s.task_environment_id = environment.id
	`), changeSet.TurnID, changeSet.TaskEnvironmentID, changeSet.TaskSessionID, changeSet.TaskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrTurnChangeRelationship
		}
		return err
	}
	return nil
}

func insertTurnChangeSet(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, changeSet *models.TurnChangeSet) error {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO turn_change_sets (
			id, task_id, session_id, turn_id, task_environment_id, revision,
			runtime_execution_id, startup_attempt_id, prompt_generation, execution_profile_id, route_generation,
			capture_enabled, settings_user_id, actor_id, settings_revision, resolution_kind,
			availability, reason, start_accepted, complete, summary_complete, content_complete,
			turn_ordinal, terminal_at, terminal_outcome, final_assistant_message_id,
			terminal_capture_started_at, terminal_capture_execution_id, terminal_capture_startup_attempt_id,
			terminal_capture_prompt_generation, terminal_capture_environment_id, terminal_capture_outcome, terminal_capture_final_message_id,
			fallback_anchor,
			file_count, added_lines, deleted_lines, binary_file_count, unknown_count_file_count, repository_count,
			retain_until, expiry_reason, content_bytes, overlap_intervals_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`),
		changeSet.ID, changeSet.TaskID, changeSet.TaskSessionID, changeSet.TurnID, changeSet.TaskEnvironmentID, changeSet.Revision,
		changeSet.RuntimeExecutionID, changeSet.StartupAttemptID, changeSet.PromptGeneration, changeSet.ExecutionProfileID, changeSet.RouteGeneration,
		changeSet.CaptureEnabled, changeSet.SettingsUserID, changeSet.ActorID, changeSet.SettingsRevision, changeSet.ResolutionKind,
		changeSet.Availability, changeSet.Reason, changeSet.StartAccepted, changeSet.Complete, changeSet.SummaryComplete, changeSet.ContentComplete,
		changeSet.TurnOrdinal, changeSet.TerminalAt, changeSet.TerminalOutcome, changeSet.FinalAssistantMessageID,
		changeSet.TerminalCaptureStartedAt, changeSet.TerminalCaptureExecutionID, changeSet.TerminalCaptureStartupAttemptID,
		changeSet.TerminalCapturePromptGeneration, changeSet.TerminalCaptureEnvironmentID, changeSet.TerminalCaptureOutcome,
		changeSet.TerminalCaptureFinalMessageID, changeSet.FallbackAnchor,
		changeSet.FileCount, changeSet.AddedLines, changeSet.DeletedLines, changeSet.BinaryFileCount, changeSet.UnknownCountFileCount, changeSet.RepositoryCount,
		changeSet.RetainUntil, changeSet.ExpiryReason, changeSet.ContentBytes, changeSet.OverlapIntervalsJSON, changeSet.CreatedAt, changeSet.UpdatedAt,
	)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted != 1 {
		return ErrTurnChangeSetIdentityConflict
	}
	return nil
}

func (r *Repository) GetTurnChangeSet(ctx context.Context, taskID, sessionID, changeSetID string) (*models.TurnChangeSet, error) {
	changeSet := &models.TurnChangeSet{}
	err := r.ro.GetContext(ctx, changeSet, r.ro.Rebind(`
		SELECT id, task_id, session_id, turn_id, task_environment_id, revision,
			runtime_execution_id, startup_attempt_id, prompt_generation, execution_profile_id, route_generation,
			capture_enabled, settings_user_id, actor_id, settings_revision, resolution_kind,
			availability, reason, start_accepted, complete, summary_complete, content_complete,
			turn_ordinal, terminal_at, terminal_outcome, final_assistant_message_id,
			terminal_capture_started_at, terminal_capture_execution_id, terminal_capture_startup_attempt_id,
			terminal_capture_prompt_generation, terminal_capture_environment_id, terminal_capture_outcome, terminal_capture_final_message_id,
			fallback_anchor,
			file_count, added_lines, deleted_lines, binary_file_count, unknown_count_file_count, repository_count,
			retain_until, expiry_reason, content_bytes, overlap_intervals_json, created_at, updated_at
		FROM turn_change_sets
		WHERE task_id = ? AND session_id = ? AND id = ?
	`), taskID, sessionID, changeSetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrTurnChangeSetNotFound, changeSetID)
	}
	if err != nil {
		return nil, err
	}
	if err := decodeTurnChangeOverlaps(changeSet.OverlapIntervalsJSON, &changeSet.OverlapIntervals); err != nil {
		return nil, fmt.Errorf("decode turn change overlaps: %w", err)
	}
	return changeSet, nil
}

func (r *Repository) ListTurnChangeSets(ctx context.Context, taskID, sessionID string, offset, limit int) ([]*models.TurnChangeSet, int, error) {
	if taskID == "" || sessionID == "" {
		return nil, 0, nil
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var total int
	if err := r.ro.GetContext(ctx, &total, r.ro.Rebind(`SELECT COUNT(*) FROM turn_change_sets WHERE task_id = ? AND session_id = ?`), taskID, sessionID); err != nil {
		return nil, 0, err
	}
	var changeSets []*models.TurnChangeSet
	err := r.ro.SelectContext(ctx, &changeSets, r.ro.Rebind(`
		SELECT id, task_id, session_id, turn_id, task_environment_id, revision,
			runtime_execution_id, startup_attempt_id, prompt_generation, execution_profile_id, route_generation,
			capture_enabled, settings_user_id, actor_id, settings_revision, resolution_kind,
			availability, reason, start_accepted, complete, summary_complete, content_complete,
			turn_ordinal, terminal_at, terminal_outcome, final_assistant_message_id, fallback_anchor,
			file_count, added_lines, deleted_lines, binary_file_count, unknown_count_file_count, repository_count,
			retain_until, expiry_reason, content_bytes, overlap_intervals_json, created_at, updated_at
		FROM turn_change_sets
		WHERE task_id = ? AND session_id = ?
		ORDER BY turn_ordinal DESC, turn_id DESC, id DESC
		LIMIT ? OFFSET ?
	`), taskID, sessionID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	for _, changeSet := range changeSets {
		if err := decodeTurnChangeOverlaps(changeSet.OverlapIntervalsJSON, &changeSet.OverlapIntervals); err != nil {
			return nil, 0, fmt.Errorf("decode turn change overlaps: %w", err)
		}
	}
	return changeSets, total, nil
}

func (r *Repository) ListUnfinishedTurnChangeSets(ctx context.Context, limit int) ([]*models.TurnChangeSet, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var changeSets []*models.TurnChangeSet
	err := r.ro.SelectContext(ctx, &changeSets, r.ro.Rebind(`
		SELECT id, task_id, session_id, turn_id, task_environment_id, revision,
			runtime_execution_id, startup_attempt_id, prompt_generation, execution_profile_id, route_generation,
			capture_enabled, settings_user_id, actor_id, settings_revision, resolution_kind,
			availability, reason, start_accepted, complete, summary_complete, content_complete,
			turn_ordinal, terminal_at, terminal_outcome, final_assistant_message_id,
			terminal_capture_started_at, terminal_capture_execution_id, terminal_capture_startup_attempt_id,
			terminal_capture_prompt_generation, terminal_capture_environment_id, terminal_capture_outcome, terminal_capture_final_message_id,
			fallback_anchor, file_count, added_lines, deleted_lines, binary_file_count, unknown_count_file_count, repository_count,
			retain_until, expiry_reason, content_bytes, overlap_intervals_json, created_at, updated_at
		FROM turn_change_sets
		WHERE terminal_at IS NULL
		ORDER BY created_at, id
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	for _, changeSet := range changeSets {
		if err := decodeTurnChangeOverlaps(changeSet.OverlapIntervalsJSON, &changeSet.OverlapIntervals); err != nil {
			return nil, fmt.Errorf("decode unfinished turn change overlaps: %w", err)
		}
	}
	return changeSets, nil
}

func (r *Repository) ListTurnRepositoryChanges(ctx context.Context, changeSetID string) ([]*models.TurnRepositoryChangeSet, error) {
	var changes []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &changes, r.ro.Rebind(`
		SELECT id, change_set_id, checkout_id, environment_repo_id, task_repository_id, repository_id,
			worktree_id, display_name, repository_subpath, start_commit_oid, start_tree_oid, end_commit_oid,
			end_tree_oid, hash_algorithm, start_captured_at, end_captured_at, start_ref, end_ref,
			availability, reason, cleanup_pending, enumeration_complete, comparison_complete, content_complete,
			overlap_intervals_json, created_at, updated_at
		FROM turn_repository_changes
		WHERE change_set_id = ?
		ORDER BY checkout_id, id
	`), changeSetID)
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		if err := decodeTurnChangeOverlaps(change.OverlapIntervalsJSON, &change.OverlapIntervals); err != nil {
			return nil, fmt.Errorf("decode repository change overlaps: %w", err)
		}
	}
	return changes, nil
}

func (r *Repository) ListTurnRepositoryChangesForSets(ctx context.Context, changeSetIDs []string) (map[string][]*models.TurnRepositoryChangeSet, error) {
	rowsBySet := make(map[string][]*models.TurnRepositoryChangeSet, len(changeSetIDs))
	ids := make([]string, 0, len(changeSetIDs))
	seen := make(map[string]struct{}, len(changeSetIDs))
	for _, id := range changeSetIDs {
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		rowsBySet[id] = nil
	}
	if len(ids) == 0 {
		return rowsBySet, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	var changes []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &changes, r.ro.Rebind(`
		SELECT id, change_set_id, checkout_id, environment_repo_id, task_repository_id, repository_id,
			worktree_id, display_name, repository_subpath, start_commit_oid, start_tree_oid, end_commit_oid,
			end_tree_oid, hash_algorithm, start_captured_at, end_captured_at, start_ref, end_ref,
			availability, reason, cleanup_pending, enumeration_complete, comparison_complete, content_complete,
			overlap_intervals_json, created_at, updated_at
		FROM turn_repository_changes
		WHERE change_set_id IN (`+placeholders+`)
		ORDER BY change_set_id, checkout_id, id
	`), args...)
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		if err := decodeTurnChangeOverlaps(change.OverlapIntervalsJSON, &change.OverlapIntervals); err != nil {
			return nil, fmt.Errorf("decode repository change overlaps: %w", err)
		}
		rowsBySet[change.TurnChangeSetID] = append(rowsBySet[change.TurnChangeSetID], change)
	}
	return rowsBySet, nil
}

func (r *Repository) GetTurnRepositoryChange(ctx context.Context, changeSetID, repositoryChangeID string) (*models.TurnRepositoryChangeSet, error) {
	change := &models.TurnRepositoryChangeSet{}
	err := r.ro.GetContext(ctx, change, r.ro.Rebind(`
		SELECT id, change_set_id, checkout_id, environment_repo_id, task_repository_id, repository_id,
			worktree_id, display_name, repository_subpath, start_commit_oid, start_tree_oid, end_commit_oid,
			end_tree_oid, hash_algorithm, start_captured_at, end_captured_at, start_ref, end_ref,
			availability, reason, cleanup_pending, enumeration_complete, comparison_complete, content_complete,
			overlap_intervals_json, created_at, updated_at
		FROM turn_repository_changes WHERE change_set_id = ? AND id = ?
	`), changeSetID, repositoryChangeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTurnChangeRelationship
	}
	if err != nil {
		return nil, err
	}
	if err := decodeTurnChangeOverlaps(change.OverlapIntervalsJSON, &change.OverlapIntervals); err != nil {
		return nil, fmt.Errorf("decode repository change overlaps: %w", err)
	}
	return change, nil
}

func (r *Repository) ListTurnFileChanges(ctx context.Context, changeSetID, repositoryChangeID string, offset, limit int) ([]*models.TurnFileChange, int, error) {
	if changeSetID == "" || repositoryChangeID == "" {
		return nil, 0, ErrTurnChangeRelationship
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var total int
	if err := r.ro.GetContext(ctx, &total, r.ro.Rebind(`
		SELECT COUNT(*) FROM turn_file_changes f
		JOIN turn_repository_changes rc ON rc.id = f.repository_change_id
		WHERE rc.change_set_id = ? AND rc.id = ?
	`), changeSetID, repositoryChangeID); err != nil {
		return nil, 0, err
	}
	var files []*models.TurnFileChange
	err := r.ro.SelectContext(ctx, &files, r.ro.Rebind(`
		SELECT f.id, f.repository_change_id, f.checkout_id, f.path, f.path_bytes, f.old_path, f.old_path_bytes,
			f.kind, f.old_blob_oid, f.new_blob_oid, f.old_mode, f.new_mode, f.submodule, f.is_binary,
			f.added_lines, f.deleted_lines, f.canonical_content_id, f.filtered_content_id, f.old_content_id, f.new_content_id,
			f.content_availability, f.content_reason, f.content_truncated, f.canonical_content_bytes, f.created_at
		FROM turn_file_changes f
		JOIN turn_repository_changes rc ON rc.id = f.repository_change_id
		WHERE rc.change_set_id = ? AND rc.id = ?
		ORDER BY f.path_bytes, f.id
		LIMIT ? OFFSET ?
	`), changeSetID, repositoryChangeID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return files, total, nil
}

func (r *Repository) AcceptTurnChangeSetStart(
	ctx context.Context,
	changeSetID string,
	expectedRevision int64,
	repositories []models.TurnRepositoryChangeSet,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	changeSet, err := lockTurnChangeSet(ctx, tx, r.db, changeSetID)
	if err != nil {
		return false, turnChangeSetLockError(err)
	}
	if !canAcceptTurnChangeSetStart(changeSet, expectedRevision) {
		return false, nil
	}
	if err := acceptTurnChangeRepositoryStarts(ctx, tx, r.db, changeSet, changeSetID, repositories); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets SET start_accepted = ?, revision = revision + 1, updated_at = ?
		WHERE id = ? AND revision = ? AND start_accepted = ? AND terminal_at IS NULL
	`), true, time.Now().UTC(), changeSetID, expectedRevision, false)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) UpdateTurnChangeSetOverlaps(ctx context.Context, changeSetID string, overlaps []models.TurnChangeOverlap) error {
	encoded, err := encodeTurnChangeOverlaps(overlaps)
	if err != nil {
		return fmt.Errorf("encode turn change overlaps: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets SET overlap_intervals_json = ?, updated_at = ? WHERE id = ?
	`), encoded, time.Now().UTC(), changeSetID)
	return err
}

func (r *Repository) MarkTurnRepositoryCheckpointRefsCleaned(ctx context.Context, repositoryChangeID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes SET cleanup_pending = ?, updated_at = ? WHERE id = ?
	`), false, time.Now().UTC(), repositoryChangeID)
	return err
}

func turnChangeSetLockError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTurnChangeSetNotFound
	}
	return err
}

func canAcceptTurnChangeSetStart(changeSet *models.TurnChangeSet, expectedRevision int64) bool {
	return changeSet.Revision == expectedRevision && !changeSet.StartAccepted && changeSet.TerminalAt == nil &&
		changeSet.CaptureEnabled && changeSet.Availability == models.TurnChangeAvailabilityPending
}

func acceptTurnChangeRepositoryStarts(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSet *models.TurnChangeSet,
	changeSetID string,
	repositories []models.TurnRepositoryChangeSet,
) error {
	seenCheckouts := make(map[string]struct{}, len(repositories))
	for _, repositoryChange := range repositories {
		if _, exists := seenCheckouts[repositoryChange.CheckoutID]; exists {
			return ErrTurnChangeRelationship
		}
		seenCheckouts[repositoryChange.CheckoutID] = struct{}{}
		if err := persistTurnChangeRepositoryStart(ctx, tx, db, changeSet, changeSetID, repositoryChange); err != nil {
			return err
		}
	}
	return nil
}

func persistTurnChangeRepositoryStart(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSet *models.TurnChangeSet,
	changeSetID string,
	repositoryChange models.TurnRepositoryChangeSet,
) error {
	if err := validateTurnRepositoryChangeStart(changeSetID, repositoryChange); err != nil {
		return err
	}
	var valid int
	err := tx.GetContext(ctx, &valid, db.Rebind(`
		SELECT 1 FROM task_environment_repos
		WHERE id = ? AND task_environment_id = ? AND repository_id = ?
	`), repositoryChange.TaskEnvironmentRepoID, changeSet.TaskEnvironmentID, repositoryChange.RepositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTurnChangeRelationship
	}
	if err != nil {
		return err
	}
	if err := prepareTurnRepositoryChangeStart(&repositoryChange); err != nil {
		return err
	}
	return insertTurnRepositoryChangeStart(ctx, tx, db, changeSetID, repositoryChange)
}

func validateTurnRepositoryChangeStart(changeSetID string, repositoryChange models.TurnRepositoryChangeSet) error {
	if repositoryChange.TurnChangeSetID != "" && repositoryChange.TurnChangeSetID != changeSetID {
		return ErrTurnChangeRelationship
	}
	missingIdentity := make([]string, 0, 4)
	if repositoryChange.ID == "" {
		missingIdentity = append(missingIdentity, "id")
	}
	if repositoryChange.CheckoutID == "" {
		missingIdentity = append(missingIdentity, "checkout_id")
	}
	if repositoryChange.TaskEnvironmentRepoID == "" {
		missingIdentity = append(missingIdentity, "task_environment_repo_id")
	}
	if repositoryChange.RepositoryID == "" {
		missingIdentity = append(missingIdentity, "repository_id")
	}
	if len(missingIdentity) > 0 {
		return fmt.Errorf("accept turn change start: missing checkout identity fields: %s", strings.Join(missingIdentity, ", "))
	}
	if repositoryChange.Availability != "" && !repositoryChange.Availability.Valid() {
		return fmt.Errorf("accept turn change start: invalid availability %q", repositoryChange.Availability)
	}
	if !repositoryChange.Reason.Valid() {
		return fmt.Errorf("accept turn change start: invalid reason code %q", repositoryChange.Reason)
	}
	if repositoryChange.Availability == models.TurnChangeAvailabilityUnavailable || repositoryChange.Availability == models.TurnChangeAvailabilityFailed {
		if repositoryChange.Reason == "" {
			return fmt.Errorf("accept turn change start: unavailable capture requires a reason")
		}
		return nil
	}
	if hasEmptyValue(repositoryChange.StartCommitOID, repositoryChange.StartTreeOID,
		repositoryChange.HashAlgorithm, repositoryChange.StartReachabilityRef) {
		return fmt.Errorf("accept turn change start: immutable start endpoint fields are required")
	}
	return nil
}

func hasEmptyValue(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func prepareTurnRepositoryChangeStart(repositoryChange *models.TurnRepositoryChangeSet) error {
	now := time.Now().UTC()
	repositoryChange.CleanupPending = repositoryChange.StartReachabilityRef != ""
	if repositoryChange.Availability != models.TurnChangeAvailabilityUnavailable &&
		repositoryChange.Availability != models.TurnChangeAvailabilityFailed && repositoryChange.StartCapturedAt == nil {
		repositoryChange.StartCapturedAt = &now
	}
	if repositoryChange.CreatedAt.IsZero() {
		repositoryChange.CreatedAt = now
	}
	if repositoryChange.UpdatedAt.IsZero() {
		repositoryChange.UpdatedAt = now
	}
	if repositoryChange.Availability == "" {
		repositoryChange.Availability = models.TurnChangeAvailabilityPending
	}
	overlapJSON, err := encodeTurnChangeOverlaps(repositoryChange.OverlapIntervals)
	if err != nil {
		return fmt.Errorf("encode repository change overlaps: %w", err)
	}
	repositoryChange.OverlapIntervalsJSON = overlapJSON
	return nil
}

func insertTurnRepositoryChangeStart(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSetID string,
	repositoryChange models.TurnRepositoryChangeSet,
) error {
	_, err := tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO turn_repository_changes (
			id, change_set_id, checkout_id, environment_repo_id, task_repository_id, repository_id,
			worktree_id, display_name, repository_subpath, start_commit_oid, start_tree_oid, hash_algorithm,
			start_captured_at, start_ref, availability, reason, cleanup_pending, overlap_intervals_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`), repositoryChange.ID, changeSetID, repositoryChange.CheckoutID, repositoryChange.TaskEnvironmentRepoID,
		repositoryChange.TaskRepositoryID, repositoryChange.RepositoryID, repositoryChange.WorktreeID,
		repositoryChange.DisplayName, repositoryChange.RepositorySubpath, repositoryChange.StartCommitOID,
		repositoryChange.StartTreeOID, repositoryChange.HashAlgorithm, repositoryChange.StartCapturedAt,
		repositoryChange.StartReachabilityRef, repositoryChange.Availability, repositoryChange.Reason, repositoryChange.CleanupPending,
		repositoryChange.OverlapIntervalsJSON, repositoryChange.CreatedAt, repositoryChange.UpdatedAt)
	if err != nil {
		return err
	}
	var inserted int
	err = tx.GetContext(ctx, &inserted, db.Rebind(`
		SELECT COUNT(*) FROM turn_repository_changes
		WHERE change_set_id = ? AND checkout_id = ? AND id = ? AND start_commit_oid = ? AND start_tree_oid = ?
	`), changeSetID, repositoryChange.CheckoutID, repositoryChange.ID, repositoryChange.StartCommitOID, repositoryChange.StartTreeOID)
	if err != nil {
		return err
	}
	if inserted != 1 {
		return ErrTurnChangeRelationship
	}
	return nil
}

func (r *Repository) FinalizeTurnChangeSet(
	ctx context.Context,
	changeSetID string,
	expectedRevision int64,
	finalization models.TurnChangeSetFinalization,
) (bool, error) {
	if err := validateTurnChangeSetFinalization(finalization); err != nil {
		return false, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changeSet, err := lockTurnChangeSet(ctx, tx, r.db, changeSetID)
	if err != nil {
		return false, turnChangeSetLockError(err)
	}
	if !canFinalizeTurnChangeSet(changeSet, expectedRevision) {
		return false, nil
	}
	overlapJSON, err := encodeTurnChangeOverlaps(finalization.OverlapIntervals)
	if err != nil {
		return false, fmt.Errorf("encode turn change overlaps: %w", err)
	}
	if err := finalizeTurnChangeRepositories(ctx, tx, r.db, changeSetID, finalization); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets SET revision = revision + 1, availability = ?, reason = ?, complete = ?,
			summary_complete = ?, content_complete = ?, terminal_at = ?, terminal_outcome = ?,
			final_assistant_message_id = ?, file_count = ?, added_lines = ?, deleted_lines = ?,
			binary_file_count = ?, unknown_count_file_count = ?, repository_count = ?, retain_until = ?,
			expiry_reason = ?, content_bytes = ?, overlap_intervals_json = ?, updated_at = ?
		WHERE id = ? AND revision = ? AND (start_accepted = ? OR capture_enabled = ?) AND terminal_at IS NULL
	`), finalization.Availability, finalization.Reason, finalization.Complete, finalization.SummaryComplete,
		finalization.ContentComplete, finalization.TerminalAt, finalization.TerminalOutcome,
		finalization.FinalAssistantMessageID, finalization.FileCount, finalization.AddedLines, finalization.DeletedLines,
		finalization.BinaryFileCount, finalization.UnknownCountFileCount, finalization.RepositoryCount, finalization.RetainUntil,
		finalization.ExpiryReason, finalization.ContentBytes, overlapJSON, finalization.TerminalAt, changeSetID, expectedRevision, true, false)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func validateTurnChangeSetFinalization(finalization models.TurnChangeSetFinalization) error {
	if !finalization.Availability.Valid() || finalization.Availability == models.TurnChangeAvailabilityPending || finalization.TerminalAt.IsZero() {
		return fmt.Errorf("finalize turn change set: terminal availability and timestamp are required")
	}
	if !finalization.Reason.Valid() {
		return fmt.Errorf("finalize turn change set: invalid reason code %q", finalization.Reason)
	}
	if hasNegativeValue(finalization.FileCount, finalization.BinaryFileCount, finalization.UnknownCountFileCount,
		finalization.RepositoryCount, finalization.ContentBytes) {
		return fmt.Errorf("finalize turn change set: summary counts cannot be negative")
	}
	return nil
}

func hasNegativeValue(values ...int64) bool {
	for _, value := range values {
		if value < 0 {
			return true
		}
	}
	return false
}

func canFinalizeTurnChangeSet(changeSet *models.TurnChangeSet, expectedRevision int64) bool {
	return changeSet.Revision == expectedRevision && (changeSet.StartAccepted || !changeSet.CaptureEnabled) && changeSet.TerminalAt == nil
}

func finalizeTurnChangeRepositories(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSetID string,
	finalization models.TurnChangeSetFinalization,
) error {
	for _, repositoryChange := range finalization.Repositories {
		if repositoryChange.TurnChangeSetID != "" && repositoryChange.TurnChangeSetID != changeSetID {
			return ErrTurnChangeRelationship
		}
		overlapJSON, err := prepareTurnRepositoryFinalization(&repositoryChange, finalization)
		if err != nil {
			return err
		}
		repositoryChange.OverlapIntervalsJSON = overlapJSON
		result, err := tx.ExecContext(ctx, db.Rebind(`
			UPDATE turn_repository_changes SET end_commit_oid = ?, end_tree_oid = ?, end_captured_at = ?, end_ref = ?,
				availability = ?, reason = ?, enumeration_complete = ?, comparison_complete = ?, content_complete = ?,
				overlap_intervals_json = ?, updated_at = ?
			WHERE id = ? AND change_set_id = ? AND (
				(end_commit_oid = '' AND end_tree_oid = '' AND ? = '' AND ? = '' AND ? = '') OR
				(end_commit_oid = ? AND end_tree_oid = ? AND end_ref = ?)
			)
		`), repositoryChange.EndCommitOID, repositoryChange.EndTreeOID, repositoryChange.EndCapturedAt,
			repositoryChange.EndReachabilityRef, repositoryChange.Availability, repositoryChange.Reason,
			repositoryChange.EnumerationComplete, repositoryChange.ComparisonComplete, repositoryChange.ContentComplete,
			repositoryChange.OverlapIntervalsJSON, finalization.TerminalAt, repositoryChange.ID, changeSetID,
			repositoryChange.EndCommitOID, repositoryChange.EndTreeOID, repositoryChange.EndReachabilityRef,
			repositoryChange.EndCommitOID, repositoryChange.EndTreeOID, repositoryChange.EndReachabilityRef)
		if err != nil {
			return err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if updated != 1 {
			return ErrTurnChangeRelationship
		}
	}
	return nil
}

func prepareTurnRepositoryFinalization(
	repositoryChange *models.TurnRepositoryChangeSet,
	finalization models.TurnChangeSetFinalization,
) (string, error) {
	if !repositoryChange.Reason.Valid() {
		return "", fmt.Errorf("finalize turn change set: invalid repository reason code %q", repositoryChange.Reason)
	}
	if finalization.Availability == models.TurnChangeAvailabilityReady &&
		hasEmptyValue(repositoryChange.EndCommitOID, repositoryChange.EndTreeOID, repositoryChange.EndReachabilityRef) {
		return "", fmt.Errorf("finalize turn change set: immutable end endpoint fields are required")
	}
	if repositoryChange.EndCommitOID != "" && repositoryChange.EndCapturedAt == nil {
		terminalAt := finalization.TerminalAt
		repositoryChange.EndCapturedAt = &terminalAt
	}
	if repositoryChange.Availability == "" {
		repositoryChange.Availability = finalization.Availability
	}
	overlapJSON, err := encodeTurnChangeOverlaps(repositoryChange.OverlapIntervals)
	if err != nil {
		return "", fmt.Errorf("encode repository change overlaps: %w", err)
	}
	repositoryChange.OverlapIntervalsJSON = overlapJSON
	return overlapJSON, nil
}

func lockTurnChangeSet(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, id string) (*models.TurnChangeSet, error) {
	query := `SELECT id, task_id, session_id, turn_id, task_environment_id, revision,
		runtime_execution_id, startup_attempt_id, prompt_generation, execution_profile_id, route_generation,
		capture_enabled, availability, start_accepted, terminal_at, terminal_capture_started_at,
		terminal_capture_execution_id, terminal_capture_startup_attempt_id, terminal_capture_prompt_generation,
		terminal_capture_environment_id, terminal_capture_outcome, terminal_capture_final_message_id
		FROM turn_change_sets WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		query += ` FOR UPDATE`
	}
	changeSet := &models.TurnChangeSet{}
	if err := tx.GetContext(ctx, changeSet, db.Rebind(query), id); err != nil {
		return nil, err
	}
	return changeSet, nil
}

func encodeTurnChangeOverlaps(overlaps []models.TurnChangeOverlap) (string, error) {
	if overlaps == nil {
		overlaps = []models.TurnChangeOverlap{}
	}
	encoded, err := json.Marshal(overlaps)
	return string(encoded), err
}

func decodeTurnChangeOverlaps(raw string, overlaps *[]models.TurnChangeOverlap) error {
	if raw == "" {
		*overlaps = []models.TurnChangeOverlap{}
		return nil
	}
	return json.Unmarshal([]byte(raw), overlaps)
}
