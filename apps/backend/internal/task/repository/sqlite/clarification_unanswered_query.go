package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// An unanswered clarification bundle is a still-pending clarification that a
// newer turn of its own session has superseded. The agent is no longer
// waiting on it, so it can never become active again; it stays listable only
// while its workspace enables the unanswered inbox and nothing has closed or
// merged it. Every write in this file is a metadata patch on such a bundle:
// status, turn ownership, and the active-clarification claim are never
// touched, so the operational pending-interaction rules cannot observe them.

const clarificationRequestTypeLiteral = "'clarification_request'"

// unansweredBundlesExpr is the derived table every read and write in this
// file selects from. Columns: pending_id, session_id, task_id, workspace_id,
// created_at, last_created_at.
//
// Supersession is resolved on turn authority alone, as the History read does:
// a terminal session's current-turn bundle is expired by the lifecycle, not
// unanswered, while an older turn's bundle stays unanswered after the session
// ends because a late answer can still reach the task.
//
// The design's Selection states each rejection clause over the bundle's rows,
// so the parent-question exclusion is aggregated over the bundle like the
// resolvable-question-id and closed/merged clauses: a bundle carrying any
// autopilot parent-question row is excluded whole, not merely stripped of that
// row.
func unansweredBundlesExpr(drv string) string {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(drv, "m.metadata", "status")
	questionIDExpr := fmt.Sprintf(
		"COALESCE(NULLIF(%s, ''), NULLIF(%s, ''), '')",
		dialect.JSONExtract(drv, "m.metadata", "question_id"),
		dialect.JSONExtractPath(drv, "m.metadata", "question", "id"),
	)
	closedExpr := dialect.JSONExtract(drv, "m.metadata", models.ClarificationMetaClosedReason)
	mergedExpr := dialect.JSONExtract(drv, "m.metadata", models.ClarificationMetaMergedInto)
	notParentQuestion := dialect.ExcludeTruthyMetadataPredicate(drv, "m.metadata", models.MetaKeyParentQuestion)
	currentTurnIDExpr := turnAuthorityCurrentTurnIDExpr(drv, "g.session_id")
	return fmt.Sprintf(`(
		SELECT g.pending_id, g.session_id, g.task_id, t.workspace_id, g.created_at, g.last_created_at
		FROM (
			SELECT
				%[1]s AS pending_id,
				m.task_session_id AS session_id,
				COALESCE(NULLIF(MIN(m.task_id), ''), MIN(ts.task_id)) AS task_id,
				MIN(m.created_at) AS created_at,
				MAX(m.created_at) AS last_created_at,
				MIN(COALESCE(m.turn_id, '')) AS turn_id,
				MAX(CASE WHEN COALESCE(%[2]s, '') IN ('', 'pending') THEN 1 ELSE 0 END) AS has_pending,
				MAX(CASE WHEN %[3]s = '' THEN 1 ELSE 0 END) AS has_missing_question_id,
				MAX(CASE WHEN NOT (%[6]s) THEN 1 ELSE 0 END) AS has_parent_question,
				MAX(CASE WHEN COALESCE(%[4]s, '') <> '' OR COALESCE(%[5]s, '') <> '' THEN 1 ELSE 0 END) AS is_closed
			FROM task_session_messages m
			JOIN task_sessions ts ON ts.id = m.task_session_id
			WHERE m.type = %[8]s
			  AND COALESCE(%[1]s, '') <> ''
			GROUP BY %[1]s, m.task_session_id
		) g
		JOIN tasks t ON t.id = g.task_id
		JOIN workspaces w ON w.id = t.workspace_id
		WHERE g.has_pending = 1
		  AND g.has_missing_question_id = 0
		  AND g.has_parent_question = 0
		  AND g.is_closed = 0
		  AND t.archived_at IS NULL
		  AND w.clarification_unanswered_inbox_enabled = TRUE
		  AND g.turn_id <> COALESCE(%[7]s, '')
	)`, pendingIDExpr, statusExpr, questionIDExpr, closedExpr, mergedExpr,
		notParentQuestion, currentTurnIDExpr, clarificationRequestTypeLiteral)
}

// unansweredInboxWorkspace is one workspace with the unanswered inbox on.
type unansweredInboxWorkspace struct {
	id            string
	retentionDays int
}

// ListUnansweredClarificationBundles returns one page of unanswered bundles
// ordered by (created_at, pending_id). Retention is applied lazily first:
// bundles older than their workspace's retention are closed as
// expired_unanswered before the page is read, so a read can never return a
// bundle its retention has already expired.
func (r *Repository) ListUnansweredClarificationBundles(
	ctx context.Context,
	opts models.ListUnansweredClarificationsOptions,
) (*models.ClarificationBundlePage, error) {
	if opts.Limit < 1 {
		return nil, fmt.Errorf("ListUnansweredClarificationBundles: limit must be >= 1, got %d", opts.Limit)
	}
	workspaces, err := r.unansweredInboxWorkspaces(ctx, opts)
	if err != nil {
		return nil, err
	}
	if len(workspaces) == 0 {
		return &models.ClarificationBundlePage{}, nil
	}
	now := opts.Now
	if now.IsZero() {
		now = r.nowUTC()
	}
	if err := r.expireUnansweredClarifications(ctx, workspaces, now.UTC()); err != nil {
		return nil, err
	}

	drv := r.db.DriverName()
	ids := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		ids = append(ids, workspace.id)
	}
	joinExtra, joinArgs := clarificationSidecarJoin(opts.Sidecar)
	whereExtra, whereArgs := unansweredBundleWhereClause(ids, opts)
	query := "SELECT b.pending_id, b.session_id, b.task_id, b.created_at\nFROM " +
		unansweredBundlesExpr(drv) + " b\n" + joinExtra + "\nWHERE " + whereExtra +
		"\nORDER BY b.created_at ASC, b.pending_id ASC\nLIMIT ?"
	args := append(append(joinArgs, whereArgs...), opts.Limit+1)

	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	bundles, err := scanClarificationBundleRows(rows, dialect.IsPostgres(drv))
	if err != nil {
		return nil, err
	}
	page := &models.ClarificationBundlePage{Bundles: bundles}
	if len(bundles) > opts.Limit {
		page.Bundles = bundles[:opts.Limit]
		page.HasMore = true
	}
	return page, nil
}

func unansweredBundleWhereClause(workspaceIDs []string, opts models.ListUnansweredClarificationsOptions) (string, []interface{}) {
	placeholders, args := buildInPlaceholders(workspaceIDs)
	conditions := []string{fmt.Sprintf("b.workspace_id IN (%s)", placeholders)}
	if opts.TaskID != "" {
		conditions = append(conditions, "b.task_id = ?")
		args = append(args, opts.TaskID)
	}
	if opts.CreatedSince != nil {
		conditions = append(conditions, "b.created_at >= ?")
		args = append(args, *opts.CreatedSince)
	}
	if opts.CursorPendingID != "" {
		conditions = append(conditions, "(b.created_at > ? OR (b.created_at = ? AND b.pending_id > ?))")
		args = append(args, opts.CursorCreatedAt, opts.CursorCreatedAt, opts.CursorPendingID)
	}
	if predicate, predArgs := clarificationSidecarPredicate(opts.Sidecar); predicate != "" {
		conditions = append(conditions, predicate)
		args = append(args, predArgs...)
	}
	return strings.Join(conditions, " AND "), args
}

// unansweredInboxWorkspaces resolves the candidate workspaces that have the
// unanswered inbox enabled, with their retention.
func (r *Repository) unansweredInboxWorkspaces(
	ctx context.Context,
	opts models.ListUnansweredClarificationsOptions,
) ([]unansweredInboxWorkspace, error) {
	if !opts.AnyWorkspace && len(opts.WorkspaceIDs) == 0 {
		return nil, nil
	}
	query := "SELECT id, clarification_unanswered_retention_days FROM workspaces WHERE clarification_unanswered_inbox_enabled = TRUE"
	var args []interface{}
	if !opts.AnyWorkspace {
		placeholders, idArgs := buildInPlaceholders(opts.WorkspaceIDs)
		query += fmt.Sprintf(" AND id IN (%s)", placeholders)
		args = idArgs
	}
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query+" ORDER BY id"), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var workspaces []unansweredInboxWorkspace
	for rows.Next() {
		var workspace unansweredInboxWorkspace
		if err := rows.Scan(&workspace.id, &workspace.retentionDays); err != nil {
			return nil, err
		}
		workspace.retentionDays = models.ClampClarificationUnansweredRetentionDays(workspace.retentionDays)
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

// expireUnansweredClarifications closes every unanswered bundle created
// before its workspace's retention cutoff.
func (r *Repository) expireUnansweredClarifications(
	ctx context.Context,
	workspaces []unansweredInboxWorkspace,
	now time.Time,
) error {
	drv := r.db.DriverName()
	patch := []metadataPatchValue{
		{key: models.ClarificationMetaClosedReason, value: string(models.ClarificationClosedExpiredUnanswered)},
		{key: models.ClarificationMetaClosedAt, value: now.Format(time.RFC3339Nano)},
	}
	query := r.db.Rebind(unansweredPatchQuery(drv, patch,
		"b.workspace_id = ? AND b.created_at < ?", false))
	for _, workspace := range workspaces {
		cutoff := now.AddDate(0, 0, -workspace.retentionDays)
		args := append(metadataPatchArgs(patch), now, workspace.id, cutoff)
		if _, err := r.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("expire unanswered clarifications in workspace %s: %w", workspace.id, err)
		}
	}
	return nil
}

// CloseUnansweredClarificationBundle records a terminal outcome on exactly one
// unanswered bundle and returns the rows it changed. An empty result means the
// bundle is not unanswered (active, terminal, already closed or merged,
// archived, or its workspace has the inbox off); nothing is written then.
func (r *Repository) CloseUnansweredClarificationBundle(
	ctx context.Context,
	pendingID string,
	closeReq models.CloseUnansweredClarification,
) ([]*models.Message, error) {
	patch, err := unansweredClosePatch(closeReq)
	if err != nil {
		return nil, err
	}
	if closeReq.Reason == models.ClarificationClosedAnsweredLate {
		return r.closeUnansweredWithAdmission(ctx, pendingID, closeReq, patch)
	}
	return r.patchUnansweredBundle(ctx, pendingID, patch, closeReq.SourceSessionID)
}

func unansweredClosePatch(closeReq models.CloseUnansweredClarification) ([]metadataPatchValue, error) {
	at := closeReq.At.UTC().Format(time.RFC3339Nano)
	patch := []metadataPatchValue{
		{key: models.ClarificationMetaClosedReason, value: string(closeReq.Reason)},
		{key: models.ClarificationMetaClosedAt, value: at},
	}
	switch closeReq.Reason {
	case models.ClarificationClosedAnsweredLate:
		if closeReq.MessageID == "" || closeReq.SessionID == "" {
			return nil, fmt.Errorf("late answer close requires a message and session")
		}
		patch = append(patch,
			metadataPatchValue{key: models.ClarificationMetaLateAnsweredAt, value: at},
			metadataPatchValue{key: models.ClarificationMetaLateAnswerMessageID, value: closeReq.MessageID},
			metadataPatchValue{key: models.ClarificationMetaLateAnswerSessionID, value: closeReq.SessionID},
		)
	case models.ClarificationClosedByUser, models.ClarificationClosedExpiredUnanswered:
	default:
		return nil, fmt.Errorf("unsupported unanswered clarification close reason %q", closeReq.Reason)
	}
	return patch, nil
}

// MergeUnansweredClarificationDuplicates hides every unanswered bundle of
// taskID whose questions all reappear in the newly asked bundle pendingID,
// marking it merged_into=pendingID. Merging is a hide marker only: the older
// bundle keeps its history and is not resolved. Returns the merged pending
// IDs; a workspace with the unanswered inbox off has no candidates.
func (r *Repository) MergeUnansweredClarificationDuplicates(
	ctx context.Context,
	taskID, pendingID string,
	questions []models.ClarificationQuestionIdentity,
) ([]string, error) {
	if taskID == "" || pendingID == "" || len(questions) == 0 {
		return nil, nil
	}
	candidates, err := r.unansweredPendingIDsForTask(ctx, taskID, pendingID)
	if err != nil || len(candidates) == 0 {
		return nil, err
	}
	messagesByPendingID, err := r.FindMessagesByPendingIDs(ctx, candidates)
	if err != nil {
		return nil, err
	}
	patch := []metadataPatchValue{{key: models.ClarificationMetaMergedInto, value: pendingID}}
	var merged []string
	for _, candidate := range candidates {
		if !bundleRepeatedBy(messagesByPendingID[candidate], questions) {
			continue
		}
		updated, err := r.patchUnansweredBundle(ctx, candidate, patch)
		if err != nil {
			return merged, err
		}
		if len(updated) > 0 {
			merged = append(merged, candidate)
		}
	}
	return merged, nil
}

func (r *Repository) unansweredPendingIDsForTask(ctx context.Context, taskID, excludePendingID string) ([]string, error) {
	drv := r.db.DriverName()
	query := "SELECT b.pending_id FROM " + unansweredBundlesExpr(drv) +
		" b WHERE b.task_id = ? AND b.pending_id <> ? ORDER BY b.created_at ASC, b.pending_id ASC"
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query), taskID, excludePendingID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// bundleRepeatedBy reports whether every question of an older bundle matches
// some question of the newer one, so hiding the older bundle loses nothing.
func bundleRepeatedBy(older []*models.Message, newer []models.ClarificationQuestionIdentity) bool {
	if len(older) == 0 {
		return false
	}
	for _, message := range older {
		identity := clarificationQuestionIdentity(message)
		matched := false
		for _, question := range newer {
			if models.SameClarificationQuestion(identity, question) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func clarificationQuestionIdentity(message *models.Message) models.ClarificationQuestionIdentity {
	identity := models.ClarificationQuestionIdentity{Prompt: message.Content}
	if id, ok := message.Metadata["question_id"].(string); ok {
		identity.ID = id
	}
	if question, ok := message.Metadata["question"].(map[string]interface{}); ok {
		if identity.ID == "" {
			identity.ID, _ = question["id"].(string)
		}
		if prompt, ok := question["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
			identity.Prompt = prompt
		}
	}
	return identity
}

func (r *Repository) patchUnansweredBundle(
	ctx context.Context,
	pendingID string,
	patch []metadataPatchValue,
	sourceSession ...string,
) ([]*models.Message, error) {
	drv := r.db.DriverName()
	filter := "b.pending_id = ?"
	args := append(metadataPatchArgs(patch), r.nowUTC(), pendingID)
	if len(sourceSession) > 0 && sourceSession[0] != "" {
		filter += " AND b.session_id = ?"
		args = append(args, sourceSession[0])
	}
	query := unansweredPatchQuery(drv, patch, filter, false)
	if len(sourceSession) > 0 && sourceSession[0] != "" {
		query += " AND task_session_id = ?"
		args = append(args, sourceSession[0])
	}
	query += ` RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		content, requests_input, type, metadata, created_at, updated_at`
	rows, err := r.db.QueryContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("patch unanswered clarification %s: %w", pendingID, err)
	}
	messages, _, scanErr := scanMessageRows(rows, 0)
	closeErr := rows.Close()
	if scanErr != nil {
		return nil, scanErr
	}
	return messages, closeErr
}

// unansweredPatchQuery builds the UPDATE that patches every message row of
// the unanswered bundles matching bundleFilter (written against alias b of
// unansweredBundlesExpr). Placeholder order: patch values, updated_at, then
// the filter's own arguments.
func unansweredPatchQuery(drv string, patch []metadataPatchValue, bundleFilter string, returning bool) string {
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE type = %s
		  AND %s IN (SELECT b.pending_id FROM %s b WHERE %s)`,
		metadataPatchExpr(drv, "task_session_messages.metadata", patch),
		clarificationRequestTypeLiteral,
		dialect.JSONExtract(drv, "task_session_messages.metadata", "pending_id"),
		unansweredBundlesExpr(drv),
		bundleFilter,
	)
	if returning {
		query += `
		RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		          content, requests_input, type, metadata, created_at, updated_at`
	}
	return query
}

// metadataPatchValue is one string-valued top-level metadata key to set.
type metadataPatchValue struct {
	key   string
	value string
}

// metadataPatchExpr sets each patch key to its bound string value. Keys are
// package constants, never caller input; values are always bound.
func metadataPatchExpr(drv, column string, patch []metadataPatchValue) string {
	if dialect.IsPostgres(drv) {
		pairs := make([]string, 0, len(patch))
		for _, p := range patch {
			pairs = append(pairs, fmt.Sprintf("'%s', CAST(? AS TEXT)", p.key))
		}
		return fmt.Sprintf("(COALESCE(%s, '{}')::jsonb || jsonb_build_object(%s))::text",
			column, strings.Join(pairs, ", "))
	}
	pairs := make([]string, 0, len(patch))
	for _, p := range patch {
		pairs = append(pairs, fmt.Sprintf("'$.%s', ?", p.key))
	}
	return fmt.Sprintf("json_set(COALESCE(%s, '{}'), %s)", column, strings.Join(pairs, ", "))
}

func metadataPatchArgs(patch []metadataPatchValue) []interface{} {
	args := make([]interface{}, 0, len(patch)+3)
	for _, p := range patch {
		args = append(args, p.value)
	}
	return args
}
