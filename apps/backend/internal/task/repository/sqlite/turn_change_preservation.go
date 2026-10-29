package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// UpdateTurnRepositoryPreservation records durable preservation evidence for
// one immutable interval. It never changes the accepted endpoint identities.
func (r *Repository) UpdateTurnRepositoryPreservation(
	ctx context.Context,
	repositoryChangeID string,
	source models.TurnChangeContentSource,
	state models.TurnChangePreservation,
	reason models.TurnChangeReason,
	attempts, bytes int64,
	receipt string,
	contentComplete bool,
) error {
	if repositoryChangeID == "" || !source.Valid() || !state.Valid() || !reason.Valid() || attempts < 0 || bytes < 0 {
		return fmt.Errorf("update turn repository preservation: invalid input")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes SET content_source = ?, preservation_state = ?, preservation_reason = ?,
			preservation_attempts = ?, preservation_bytes = ?, preservation_receipt = ?,
			content_complete = ?, updated_at = ?
		WHERE id = ?
	`), source, state, reason, attempts, bytes, receipt, contentComplete, time.Now().UTC(), repositoryChangeID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return repoerrors.ErrTurnChangeRelationship
	}
	return nil
}

// MarkTurnChangeFilesGitBacked makes file entries readable through their
// preserved archive without creating SQL patch payloads.
func (r *Repository) MarkTurnChangeFilesGitBacked(ctx context.Context, repositoryChangeID string) (bool, error) {
	if repositoryChangeID == "" {
		return false, repoerrors.ErrTurnChangeRelationship
	}
	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_file_changes SET content_availability = 'ready', content_reason = '',
			content_truncated = FALSE, canonical_content_id = '', filtered_content_id = '',
			old_content_id = '', new_content_id = ''
		WHERE repository_change_id = ? AND content_availability <> 'expired'
			AND content_reason <> 'invalid_path_encoding'
	`), repositoryChangeID); err != nil {
		return false, err
	}
	var unavailable int
	if err := r.db.GetContext(ctx, &unavailable, r.db.Rebind(`
		SELECT COUNT(*) FROM turn_file_changes
		WHERE repository_change_id = ? AND (content_availability <> 'ready' OR content_truncated = TRUE)
	`), repositoryChangeID); err != nil {
		return false, err
	}
	return unavailable == 0, nil
}

// GetTurnFileContentSource resolves one server-owned historical read context.
// The change set, repository, and file must all belong to the same change set.
func (r *Repository) GetTurnFileContentSource(
	ctx context.Context,
	changeSetID, fileChangeID string,
) (*models.TurnFileContentSource, error) {
	if changeSetID == "" || fileChangeID == "" {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	changeSet := &models.TurnChangeSet{}
	err := r.ro.GetContext(ctx, changeSet, r.ro.Rebind(`
		SELECT id, task_id, session_id, revision, availability, expiry_reason, retain_until, terminal_at
		FROM turn_change_sets WHERE id = ?
	`), changeSetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	if err != nil {
		return nil, err
	}
	repositoryChange := &models.TurnRepositoryChangeSet{}
	err = r.ro.GetContext(ctx, repositoryChange, r.ro.Rebind(`
		SELECT rc.id, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.hash_algorithm,
			rc.start_commit_oid, rc.start_tree_oid, rc.end_commit_oid, rc.end_tree_oid,
			rc.source_git_dir, rc.content_source, rc.preservation_state, rc.preservation_reason,
			rc.preservation_attempts, rc.preservation_bytes, rc.preservation_receipt,
			rc.availability, rc.reason, rc.content_complete
		FROM turn_repository_changes rc
		JOIN turn_file_changes f ON f.repository_change_id = rc.id
		WHERE rc.change_set_id = ? AND f.id = ?
	`), changeSetID, fileChangeID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if err != nil {
		return nil, err
	}
	file := &models.TurnFileChange{}
	err = r.ro.GetContext(ctx, file, r.ro.Rebind(`
		SELECT id, repository_change_id, checkout_id, path, path_bytes, old_path, old_path_bytes,
			kind, old_blob_oid, new_blob_oid, old_mode, new_mode, submodule, is_binary,
			added_lines, deleted_lines, content_availability, content_reason, content_truncated, canonical_content_bytes
		FROM turn_file_changes WHERE id = ? AND repository_change_id = ?
	`), fileChangeID, repositoryChange.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, repoerrors.ErrTurnChangeContentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &models.TurnFileContentSource{ChangeSet: changeSet, Repository: repositoryChange, File: file}, nil
}

// ListTurnRepositoryPreservationRows finds intervals whose preservation has
// not settled, oldest first, for bounded restart recovery.
func (r *Repository) ListTurnRepositoryPreservationRows(
	ctx context.Context,
	states []models.TurnChangePreservation,
	limit int,
) ([]*models.TurnRepositoryChangeSet, error) {
	if len(states) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(states)), ",")
	args := make([]any, 0, len(states)+1)
	for _, state := range states {
		args = append(args, string(state))
	}
	args = append(args, limit)
	var rows []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT id, change_set_id, checkout_id, repository_id, hash_algorithm,
			start_commit_oid, start_tree_oid, end_commit_oid, end_tree_oid,
			source_git_dir, content_source, preservation_state, preservation_reason, preservation_attempts,
			preservation_bytes, preservation_receipt, preservation_deadline, availability, reason, content_complete
		FROM turn_repository_changes
		WHERE preservation_state IN (`+placeholders+`)
		ORDER BY created_at, id
		LIMIT ?
	`), args...)
	return rows, err
}

// ListTurnHistoryRetentionCandidates returns the first page of terminal change
// sets whose owning task passes the shared conservative activity predicate.
// Eligibility is evaluated per task cohort, not per individual turn timestamp.
func (r *Repository) ListTurnHistoryRetentionCandidates(
	ctx context.Context,
	cutoff, now time.Time,
	limit int,
) ([]*models.TurnHistoryRetentionCandidate, error) {
	candidates, _, err := r.ListTurnHistoryRetentionCandidatesPage(ctx, cutoff, now, "", limit)
	return candidates, err
}

// ListTurnHistoryRetentionCandidatesPage paginates by task identity. It returns
// the next task cursor when more eligible tasks may remain.
func (r *Repository) ListTurnHistoryRetentionCandidatesPage(
	ctx context.Context,
	cutoff, now time.Time,
	afterTaskID string,
	limit int,
) ([]*models.TurnHistoryRetentionCandidate, string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	taskIDs, err := r.turnHistoryCandidateTaskIDs(ctx, afterTaskID, 200)
	if err != nil {
		return nil, "", err
	}
	candidates := make([]*models.TurnHistoryRetentionCandidate, 0, limit)
	next := ""
	for _, taskID := range taskIDs {
		eligible, eligibilityErr := PayloadTaskEligible(ctx, r.ro, taskID, cutoff)
		if eligibilityErr != nil || !eligible {
			next = taskID
			continue
		}
		sets, setsErr := r.turnHistoryTaskCandidates(ctx, taskID, now, 5000)
		if setsErr != nil {
			return nil, "", setsErr
		}
		for _, setID := range sets {
			candidate, loadErr := r.loadTurnHistoryRetentionCandidate(ctx, setID)
			if loadErr != nil {
				return nil, "", loadErr
			}
			candidates = append(candidates, candidate)
		}
		next = taskID
		if len(candidates) >= limit {
			return candidates, next, nil
		}
	}
	return candidates, next, nil
}

func (r *Repository) turnHistoryCandidateTaskIDs(ctx context.Context, afterTaskID string, limit int) ([]string, error) {
	var taskIDs []string
	err := r.ro.SelectContext(ctx, &taskIDs, r.ro.Rebind(`
		SELECT DISTINCT task_id FROM turn_change_sets
		WHERE terminal_at IS NOT NULL AND availability <> 'expired' AND task_id > ?
		ORDER BY task_id
		LIMIT ?
	`), afterTaskID, limit)
	return taskIDs, err
}

func (r *Repository) turnHistoryTaskCandidates(ctx context.Context, taskID string, now time.Time, limit int) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(`
		SELECT cs.id FROM turn_change_sets cs
		WHERE cs.task_id = ? AND cs.terminal_at IS NOT NULL AND cs.availability <> 'expired'
			AND NOT EXISTS (
				SELECT 1 FROM turn_change_content_leases l
				WHERE l.change_set_id = cs.id AND l.expires_at > ?
			)
			AND NOT EXISTS (
				SELECT 1 FROM turn_repository_changes pending
				WHERE pending.change_set_id = cs.id AND pending.preservation_state = 'pending'
			)
		ORDER BY cs.terminal_at, cs.id
		LIMIT ?
	`), taskID, now, limit)
	return ids, err
}

func (r *Repository) loadTurnHistoryRetentionCandidate(ctx context.Context, id string) (*models.TurnHistoryRetentionCandidate, error) {
	candidate := &models.TurnHistoryRetentionCandidate{ChangeSetID: id}
	if err := r.ro.GetContext(ctx, candidate, r.ro.Rebind(`
		SELECT id AS change_set_id, task_id, terminal_at
		FROM turn_change_sets WHERE id = ?
	`), id); err != nil {
		return nil, err
	}
	var repositories []*models.TurnRepositoryChangeSet
	if err := r.ro.SelectContext(ctx, &repositories, r.ro.Rebind(`
		SELECT id, change_set_id, checkout_id, repository_id, hash_algorithm,
			start_commit_oid, start_tree_oid, end_commit_oid, end_tree_oid,
			source_git_dir, content_source, preservation_state, preservation_reason, preservation_attempts,
			preservation_bytes, preservation_receipt, preservation_deadline, availability, reason, content_complete
		FROM turn_repository_changes WHERE change_set_id = ? ORDER BY id
	`), id); err != nil {
		return nil, err
	}
	candidate.Repositories = repositories
	var legacyBytes sql.NullInt64
	if err := r.ro.GetContext(ctx, &legacyBytes, r.ro.Rebind(`
		SELECT COALESCE(SUM(length(c.payload_bytes)), 0)
		FROM turn_change_contents c
		WHERE EXISTS (
			SELECT 1 FROM turn_change_content_links l
			JOIN turn_file_changes f ON f.id = l.file_change_id
			JOIN turn_repository_changes rc ON rc.id = f.repository_change_id
			WHERE l.content_id = c.id AND rc.change_set_id = ?
		)
	`), id); err != nil {
		return nil, err
	}
	if legacyBytes.Valid {
		candidate.LegacySQLBytes = legacyBytes.Int64
	}
	for _, repository := range repositories {
		if repository.PreservationBytes > 0 {
			candidate.PreservationBytes += repository.PreservationBytes
		}
	}
	return candidate, nil
}

// ExpireTurnHistory marks eligible change sets expired and records the exact
// owned refs that must be removed from backend archives. SQL evidence is
// removed first; the receipt makes external cleanup resumable.
func (r *Repository) ExpireTurnHistory(
	ctx context.Context,
	changeSetIDs []string,
	cutoff time.Time,
	reason models.TurnChangeReason,
	now time.Time,
) (models.TurnHistoryExpiryResult, error) {
	result := models.TurnHistoryExpiryResult{}
	if len(changeSetIDs) == 0 {
		return result, nil
	}
	if !reason.Valid() {
		return result, fmt.Errorf("expire turn history: invalid reason")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockTurnChangeRetention(ctx, tx, r.db); err != nil {
		return result, err
	}
	for _, changeSetID := range changeSetIDs {
		expired, err := expireTurnHistorySet(ctx, tx, r.db, changeSetID, cutoff, reason, now)
		if err != nil {
			return result, err
		}
		if expired {
			result.ExpiredChangeSets++
		}
	}
	deleted, freed, err := deleteOrphanedTurnChangeContent(ctx, tx, r.db)
	if err != nil {
		return result, err
	}
	result.DeletedContents, result.FreedPayloadBytes = deleted, freed
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func expireTurnHistorySet(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	changeSetID string,
	cutoff time.Time,
	reason models.TurnChangeReason,
	now time.Time,
) (bool, error) {
	var taskID string
	if err := tx.GetContext(ctx, &taskID, db.Rebind(`SELECT task_id FROM turn_change_sets WHERE id = ?`), changeSetID); err != nil {
		return false, err
	}
	eligible, err := PayloadTaskEligible(ctx, tx, taskID, cutoff)
	if err != nil || !eligible {
		return false, nil
	}
	var pending bool
	if err := tx.GetContext(ctx, &pending, db.Rebind(`
		SELECT EXISTS(SELECT 1 FROM turn_repository_changes WHERE change_set_id = ? AND preservation_state = 'pending')
	`), changeSetID); err != nil {
		return false, err
	}
	if pending {
		return false, nil
	}
	var leased bool
	if err := tx.GetContext(ctx, &leased, db.Rebind(`
		SELECT EXISTS(SELECT 1 FROM turn_change_content_leases WHERE change_set_id = ? AND expires_at > ?)
	`), changeSetID, now); err != nil {
		return false, err
	}
	if leased {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE turn_file_changes SET canonical_content_id = '', filtered_content_id = '', old_content_id = '', new_content_id = '',
			content_availability = 'expired', content_reason = ?, content_truncated = TRUE
		WHERE repository_change_id IN (SELECT id FROM turn_repository_changes WHERE change_set_id = ?)
			AND content_availability <> 'expired'
	`), reason, changeSetID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`DELETE FROM turn_change_content_links WHERE file_change_id IN (
		SELECT f.id FROM turn_file_changes f JOIN turn_repository_changes r ON r.id = f.repository_change_id WHERE r.change_set_id = ?
	)`), changeSetID); err != nil {
		return false, err
	}
	updated, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE turn_change_sets SET availability = 'expired', reason = ?, content_complete = FALSE,
			expiry_reason = ?, content_bytes = 0, revision = revision + 1, updated_at = ?
		WHERE id = ? AND terminal_at IS NOT NULL AND availability <> 'expired'
	`), reason, reason, now, changeSetID)
	if err != nil {
		return false, err
	}
	affected, err := updated.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// ListExpiredTurnHistoryRepositories returns archive receipts that still need
// owned-ref cleanup after SQL expiry. A non-empty receipt is the durable
// cleanup intent; a cleared receipt means the external step is complete.
func (r *Repository) ListExpiredTurnHistoryRepositories(
	ctx context.Context,
	limit int,
) ([]*models.TurnRepositoryChangeSet, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT rc.id, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.hash_algorithm,
			rc.start_commit_oid, rc.start_tree_oid, rc.end_commit_oid, rc.end_tree_oid,
			rc.source_git_dir, rc.content_source, rc.preservation_state, rc.preservation_reason, rc.preservation_attempts,
			rc.preservation_bytes, rc.preservation_receipt, rc.preservation_deadline, rc.availability, rc.reason, rc.content_complete
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE cs.availability = 'expired' AND rc.preservation_receipt <> '' AND rc.content_source = 'git_archive'
		ORDER BY cs.updated_at, rc.id
		LIMIT ?
	`), limit)
	return rows, err
}

// ClearTurnRepositoryPreservationReceipt acknowledges completed owned-ref
// cleanup without altering turn or file summaries.
func (r *Repository) ClearTurnRepositoryPreservationReceipt(ctx context.Context, repositoryChangeID string) error {
	if repositoryChangeID == "" {
		return repoerrors.ErrTurnChangeRelationship
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes SET preservation_receipt = '', preservation_bytes = 0, updated_at = ? WHERE id = ?
	`), time.Now().UTC(), repositoryChangeID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return repoerrors.ErrTurnChangeRelationship
	}
	return nil
}

// ListTurnRepositoryPreservationReady returns every live, verified archive
// binding for backup publication.
func (r *Repository) ListTurnRepositoryPreservationReady(
	ctx context.Context,
	limit int,
) ([]*models.TurnRepositoryChangeSet, error) {
	rows, _, err := r.ListTurnRepositoryPreservationReadyPage(ctx, "", limit)
	return rows, err
}

// ListTurnRepositoryPreservationReadyPage pages by repository-interval ID.
func (r *Repository) ListTurnRepositoryPreservationReadyPage(
	ctx context.Context,
	afterID string,
	limit int,
) ([]*models.TurnRepositoryChangeSet, string, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	var rows []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT rc.id, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.hash_algorithm,
			rc.start_commit_oid, rc.start_tree_oid, rc.end_commit_oid, rc.end_tree_oid,
			rc.source_git_dir, rc.content_source, rc.preservation_state, rc.preservation_reason, rc.preservation_attempts,
			rc.preservation_bytes, rc.preservation_receipt, rc.preservation_deadline, rc.availability, rc.reason, rc.content_complete
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE cs.availability <> 'expired' AND rc.content_source IN ('git_archive','git_common')
			AND rc.preservation_state = 'ready'
			AND rc.id > ?
		ORDER BY rc.id
		LIMIT ?
	`), afterID, limit)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(rows) == limit {
		next = rows[len(rows)-1].ID
	}
	return rows, next, nil
}

// SetTurnRepositoryPreservationDeadline records the bounded recovery window
// for pending preservation work.
func (r *Repository) SetTurnRepositoryPreservationDeadline(ctx context.Context, repositoryChangeID string, deadline time.Time) error {
	if repositoryChangeID == "" || deadline.IsZero() {
		return repoerrors.ErrTurnChangeRelationship
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes SET preservation_deadline = ?, updated_at = ? WHERE id = ?
	`), deadline.UTC(), time.Now().UTC(), repositoryChangeID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return repoerrors.ErrTurnChangeRelationship
	}
	return nil
}

// ListTurnPreservationJobs returns pending terminal intervals for the durable
// retry owner, oldest deadline first.
func (r *Repository) ListTurnPreservationJobs(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT rc.id, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.repository_subpath, rc.hash_algorithm,
			rc.start_commit_oid, rc.start_tree_oid, rc.end_commit_oid, rc.end_tree_oid,
			rc.source_git_dir, rc.content_source, rc.preservation_state, rc.preservation_reason, rc.preservation_attempts,
			rc.preservation_bytes, rc.preservation_receipt, rc.preservation_deadline,
			rc.availability, rc.reason, rc.content_complete, cs.task_id, cs.session_id
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE rc.preservation_state = 'pending' AND cs.terminal_at IS NOT NULL
		ORDER BY COALESCE(rc.preservation_deadline, rc.updated_at), rc.id
		LIMIT ?
	`), limit)
	return rows, err
}

// UpdateTurnChangeSetContentStatus publishes recovered preservation to the
// original immutable turn summary without replacing terminal identity.
func (r *Repository) UpdateTurnChangeSetContentStatus(
	ctx context.Context,
	changeSetID string,
	contentComplete bool,
	reason models.TurnChangeReason,
) error {
	if changeSetID == "" || !reason.Valid() {
		return repoerrors.ErrTurnChangeRelationship
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_change_sets SET content_complete = ?, complete = summary_complete AND ?,
			reason = CASE WHEN ? THEN '' ELSE ? END,
			revision = revision + 1, updated_at = ?
		WHERE id = ? AND terminal_at IS NOT NULL AND availability <> 'expired'
	`), contentComplete, contentComplete, contentComplete, reason, time.Now().UTC(), changeSetID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return repoerrors.ErrTurnChangeRelationship
	}
	return nil
}

// StageTurnHistoryCleanupForTask persists owned-ref cleanup intent before the
// task cascade removes the only row that names those refs.
func (r *Repository) StageTurnHistoryCleanupForTask(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO turn_history_cleanup_intents(id, source_id, start_ref, end_ref, start_commit_oid, end_commit_oid, change_set_id, checkout_id, repository_id, source_git_dir, created_at)
		SELECT `+r.cleanupIntentIDExpression()+`, CASE WHEN rc.repository_id <> '' THEN rc.repository_id ELSE rc.checkout_id END,
			'refs/kandev/history/' || rc.change_set_id || '/' || rc.checkout_id || '/start',
			'refs/kandev/history/' || rc.change_set_id || '/' || rc.checkout_id || '/end',
			rc.start_commit_oid, rc.end_commit_oid, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.source_git_dir, ?
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE cs.task_id = ? AND rc.content_source IN ('git_archive','git_common')
	`), time.Now().UTC(), taskID)
	return err
}

// StageTurnHistoryCleanupForSession persists intent for one deleted session.
func (r *Repository) StageTurnHistoryCleanupForSession(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO turn_history_cleanup_intents(id, source_id, start_ref, end_ref, start_commit_oid, end_commit_oid, change_set_id, checkout_id, repository_id, source_git_dir, created_at)
		SELECT `+r.cleanupIntentIDExpression()+`, CASE WHEN rc.repository_id <> '' THEN rc.repository_id ELSE rc.checkout_id END,
			'refs/kandev/history/' || rc.change_set_id || '/' || rc.checkout_id || '/start',
			'refs/kandev/history/' || rc.change_set_id || '/' || rc.checkout_id || '/end',
			rc.start_commit_oid, rc.end_commit_oid, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.source_git_dir, ?
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE cs.session_id = ? AND rc.content_source IN ('git_archive','git_common')
	`), time.Now().UTC(), sessionID)
	return err
}

// ListTurnHistoryCleanupIntents pages independent cleanup work.
func (r *Repository) ListTurnHistoryCleanupIntents(ctx context.Context, limit int) ([]*models.TurnHistoryCleanupIntent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var intents []*models.TurnHistoryCleanupIntent
	err := r.ro.SelectContext(ctx, &intents, r.ro.Rebind(`
		SELECT id, source_id, start_ref, end_ref, start_commit_oid, end_commit_oid,
			change_set_id, checkout_id, repository_id, source_git_dir, created_at
		FROM turn_history_cleanup_intents ORDER BY created_at, id LIMIT ?
	`), limit)
	return intents, err
}

func (r *Repository) DeleteTurnHistoryCleanupIntent(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM turn_history_cleanup_intents WHERE id = ?`), id)
	return err
}

func (r *Repository) cleanupIntentIDExpression() string {
	if dialect.IsPostgres(r.db.DriverName()) {
		return "md5(random()::text || clock_timestamp()::text)"
	}
	return "lower(hex(randomblob(16)))"
}

// TaskPreservationBytes reports backend-owned archive bytes charged to one
// task for capacity admission.
func (r *Repository) TaskPreservationBytes(ctx context.Context, changeSetID string) (int64, error) {
	var bytes int64
	err := r.ro.GetContext(ctx, &bytes, r.ro.Rebind(`
		SELECT COALESCE(SUM(rc.preservation_bytes), 0)
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE cs.availability <> 'expired'
			AND cs.task_id = (SELECT task_id FROM turn_change_sets WHERE id = ?)
	`), changeSetID)
	return bytes, err
}

// ListExpiredCommonHistory returns expired local common-source intervals whose
// executor refs still need removal.
func (r *Repository) ListExpiredCommonHistory(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var rows []*models.TurnRepositoryChangeSet
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT rc.id, rc.change_set_id, rc.checkout_id, rc.repository_id, rc.hash_algorithm,
			rc.start_commit_oid, rc.start_tree_oid, rc.end_commit_oid, rc.end_tree_oid,
			rc.start_ref, rc.end_ref, rc.source_git_dir, rc.content_source, rc.preservation_state,
			rc.preservation_reason, rc.preservation_attempts, rc.preservation_bytes, rc.preservation_receipt,
			rc.preservation_deadline, rc.availability, rc.reason, rc.content_complete
		FROM turn_repository_changes rc
		JOIN turn_change_sets cs ON cs.id = rc.change_set_id
		WHERE cs.availability = 'expired' AND rc.content_source = 'git_common' AND rc.source_git_dir <> ''
		ORDER BY cs.updated_at, rc.id LIMIT ?
	`), limit)
	return rows, err
}

func (r *Repository) ClearTurnRepositoryCommonCleanup(ctx context.Context, repositoryChangeID string) error {
	if repositoryChangeID == "" {
		return repoerrors.ErrTurnChangeRelationship
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE turn_repository_changes SET source_git_dir = '', cleanup_pending = FALSE, updated_at = ? WHERE id = ?
	`), time.Now().UTC(), repositoryChangeID)
	return err
}
