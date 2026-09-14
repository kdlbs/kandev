package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ListInboxHistoryBundles is the History tab's exported entry point
// (AC-UI-INBOX-HISTORY-001.5): a separate, additive read from the
// operational pending-interaction path. It reuses this package's unexported
// currentTurnAuthority and nonTerminalSessionPredicate machinery only
// through turnAuthorityCurrentTurnIDExpr, and is never called from any sink
// that gates a workflow transition, emits a pending-action event, resolves
// an answer, or contributes to the pending-action projection --
// ARCH-INBOX-HISTORY-ISOLATION enforces that statically.
func (r *Repository) ListInboxHistoryBundles(ctx context.Context, opts models.ListClarificationHistoryOptions) (*models.ClarificationHistoryPage, error) {
	if opts.Limit < 1 {
		return nil, fmt.Errorf("ListInboxHistoryBundles: limit must be >= 1, got %d", opts.Limit)
	}
	if opts.WorkspaceID == "" {
		return nil, fmt.Errorf("ListInboxHistoryBundles: workspace_id is required")
	}

	drv := r.ro.DriverName()
	args := []interface{}{opts.WorkspaceID}
	query := inboxHistoryQueryBase(drv) + "\nSELECT " + inboxHistorySelectColumns + "\nFROM reasoned r\nJOIN tasks t ON t.id = r.task_id\n" + inboxHistoryWhereClause
	if opts.CursorPendingID != "" {
		query += "\n  AND (r.created_at > ? OR (r.created_at = ? AND r.pending_id > ?))"
		args = append(args, opts.CursorCreatedAt, opts.CursorCreatedAt, opts.CursorPendingID)
	}
	query += "\nORDER BY r.created_at ASC, r.pending_id ASC\nLIMIT ?"
	args = append(args, opts.Limit+1)

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	bundles, err := scanInboxHistoryBundleRows(rows, dialect.IsPostgres(drv))
	if err != nil {
		return nil, err
	}

	page := &models.ClarificationHistoryPage{Bundles: bundles}
	if len(bundles) > opts.Limit {
		page.Bundles = bundles[:opts.Limit]
		page.HasMore = true
	}
	return page, nil
}

// CountInboxHistoryBundles is the History tab's second exported entry point
// (AC .5/.20): the unbounded, workspace-wide bundle total, independent of
// page size and of any cursor. Subject to the same ARCH-INBOX-HISTORY-ISOLATION
// isolation as ListInboxHistoryBundles.
func (r *Repository) CountInboxHistoryBundles(ctx context.Context, opts models.ListClarificationHistoryOptions) (int, error) {
	if opts.WorkspaceID == "" {
		return 0, fmt.Errorf("CountInboxHistoryBundles: workspace_id is required")
	}
	drv := r.ro.DriverName()
	query := inboxHistoryQueryBase(drv) + "\nSELECT COUNT(*)\nFROM reasoned r\nJOIN tasks t ON t.id = r.task_id\n" + inboxHistoryWhereClause

	var total int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), opts.WorkspaceID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// inboxHistoryWhereClause is shared by the page and count queries: the AC .2
// eligibility gate (non-terminal, unarchived, workspace-scoped) AND
// (superseded OR session_ended OR unreadable) -- a bundle matching none of
// the three is live and excluded from both queries.
const inboxHistoryWhereClause = `WHERE r.has_pending = 1
  AND t.archived_at IS NULL
  AND t.workspace_id = ?
  AND (r.turn_mismatch = 1
       OR (r.bundle_type = 'permission_request' AND r.permission_not_newest = 1)
       OR r.is_session_ended = 1
       OR r.is_unreadable = 1)`

// inboxHistorySelectColumns is the page query's column list; the count
// query only needs the WHERE clause's inputs, which inboxHistoryWhereClause
// already references by CTE column name.
const inboxHistorySelectColumns = `r.pending_id, r.session_id, r.task_id, r.created_at, r.bundle_type,
       r.turn_id, r.current_turn_id, r.turn_mismatch, r.permission_not_newest,
       r.is_session_ended, r.is_unreadable`

// inboxHistoryQueryBase builds the two CTEs shared by the page and count
// queries: `bundles` groups messages into per-pending_id aggregates exactly
// like clarificationBundleTableExpr's inner subquery, but across BOTH
// clarification_request and permission_request (AC .2 is evaluated per
// bundle regardless of kind), additionally split by request_id for a
// permission_request row so a reused pending_id never merges two distinct
// requests; `reasoned` resolves the three raw exclusion signals once per
// bundle so the outer WHERE and SELECT never recompute the current-turn
// correlated subquery more than once.
func inboxHistoryQueryBase(drv string) string {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(drv, "m.metadata", "status")
	questionIDExpr := fmt.Sprintf(
		"COALESCE(NULLIF(%s, ''), NULLIF(%s, ''), '')",
		dialect.JSONExtract(drv, "m.metadata", "question_id"),
		dialect.JSONExtractPath(drv, "m.metadata", "question", "id"),
	)
	notParentQuestion := dialect.ExcludeTruthyMetadataPredicate(drv, "m.metadata", "parent_question")
	permissionOrder := pendingActionMessageOrder(drv, "m2")
	currentTurnIDExpr := turnAuthorityCurrentTurnIDExpr(drv, "bundles.session_id")
	terminalStates := terminalSessionStatesExpr("s.state")
	// A provider may reuse a permission's pending_id for a later, unrelated
	// request once the original resolves (message.go's
	// GetPermissionMessageByIdentity, service_messages.go's
	// UpdatePermissionMessage, event_handlers_streaming.go's
	// handlePermissionCancelledEvent). Grouping permission rows by pending_id
	// alone would merge that later request with the resolved original into
	// one bundle. request_id disambiguates them the same way those three
	// sites do; message id is the fallback for a row with no request_id.
	requestIDExpr := dialect.JSONExtract(drv, "m.metadata", "request_id")
	permissionGroupKeyExpr := fmt.Sprintf(
		"CASE WHEN m.type = 'permission_request' THEN COALESCE(NULLIF(%s, ''), m.id) ELSE '' END",
		requestIDExpr,
	)

	return fmt.Sprintf(`WITH bundles AS (
    SELECT
        %[1]s AS pending_id,
        m.task_session_id AS session_id,
        COALESCE(NULLIF(MIN(m.task_id), ''), MIN(ts.task_id)) AS task_id,
        MIN(m.created_at) AS created_at,
        MIN(m.type) AS bundle_type,
        MIN(m.turn_id) AS turn_id,
        MAX(CASE WHEN COALESCE(%[2]s, '') IN ('', 'pending') THEN 1 ELSE 0 END) AS has_pending,
        MAX(CASE WHEN %[3]s = '' THEN 1 ELSE 0 END) AS has_missing_question_id,
        MAX(CASE WHEN pr.rn IS NOT NULL AND pr.rn > 1 THEN 1 ELSE 0 END) AS permission_not_newest
    FROM task_session_messages m
    JOIN task_sessions ts ON ts.id = m.task_session_id
    LEFT JOIN (
        SELECT m2.id AS message_id,
               ROW_NUMBER() OVER (
                 PARTITION BY m2.task_session_id, m2.turn_id
                 ORDER BY m2.created_at DESC, %[6]s DESC
               ) AS rn
        FROM task_session_messages m2
        WHERE m2.type = 'permission_request'
    ) pr ON pr.message_id = m.id
    WHERE m.type IN ('clarification_request', 'permission_request')
      AND %[4]s
    GROUP BY %[1]s, m.task_session_id, %[8]s
),
reasoned AS (
    SELECT
        bundles.*,
        %[5]s AS current_turn_id,
        CASE WHEN COALESCE(bundles.turn_id, '') <> COALESCE(%[5]s, '') THEN 1 ELSE 0 END AS turn_mismatch,
        CASE WHEN %[7]s THEN 1 ELSE 0 END AS is_session_ended,
        CASE WHEN bundles.bundle_type = 'clarification_request' AND bundles.has_missing_question_id = 1 THEN 1 ELSE 0 END AS is_unreadable
    FROM bundles
    JOIN task_sessions s ON s.id = bundles.session_id
)
`, pendingIDExpr, statusExpr, questionIDExpr, notParentQuestion, currentTurnIDExpr, permissionOrder, terminalStates, permissionGroupKeyExpr)
}

// turnAuthorityCurrentTurnIDExpr resolves the session's current turn by
// currentTurnAuthority ALONE (AC .2's "superseded" clause), never composed
// with nonTerminalSessionPredicate: composing the two would give a
// terminated session no current turn at all, making every one of its rows
// read superseded and making session_ended unreachable (system design, "Why
// superseded resolves on turn authority alone").
func turnAuthorityCurrentTurnIDExpr(drv, sessionExpr string) string {
	predicate, orderBy := currentTurnAuthority(drv, "turn_row")
	return fmt.Sprintf(`(
        SELECT turn_row.id FROM task_session_turns turn_row
        WHERE turn_row.task_session_id = %s AND %s
        ORDER BY %s LIMIT 1
    )`, sessionExpr, predicate, orderBy)
}

// terminalSessionStatesExpr builds the session_ended raw signal: the same
// three terminal states nonTerminalSessionPredicate excludes, applied
// directly to a session-state column rather than as an EXISTS negation,
// since reasoned already joins task_sessions once per bundle.
func terminalSessionStatesExpr(stateColumn string) string {
	return fmt.Sprintf("%s IN ('%s', '%s', '%s')",
		stateColumn,
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	)
}

// scanInboxHistoryBundleRows scans inboxHistorySelectColumns. created_at is
// a MIN() aggregate, which loses SQLite driver type affinity exactly like
// scanClarificationBundleRows's identical case.
func scanInboxHistoryBundleRows(rows *sql.Rows, isPostgres bool) ([]models.ClarificationHistoryBundleSummary, error) {
	var bundles []models.ClarificationHistoryBundleSummary
	for rows.Next() {
		var (
			pendingID, sessionID, taskID, bundleType, turnID string
			currentTurnID                                    sql.NullString
			turnMismatch, permissionNotNewest                int
			isSessionEnded, isUnreadable                     int
			createdAt                                        time.Time
		)
		if isPostgres {
			if err := rows.Scan(&pendingID, &sessionID, &taskID, &createdAt, &bundleType,
				&turnID, &currentTurnID, &turnMismatch, &permissionNotNewest,
				&isSessionEnded, &isUnreadable); err != nil {
				return nil, err
			}
		} else {
			var createdAtRaw string
			if err := rows.Scan(&pendingID, &sessionID, &taskID, &createdAtRaw, &bundleType,
				&turnID, &currentTurnID, &turnMismatch, &permissionNotNewest,
				&isSessionEnded, &isUnreadable); err != nil {
				return nil, err
			}
			createdAt = parseLegacyTimestamp(createdAtRaw)
		}

		isSuperseded := turnMismatch == 1 || (bundleType == "permission_request" && permissionNotNewest == 1)
		summary := models.ClarificationHistoryBundleSummary{
			PendingID:    pendingID,
			SessionID:    sessionID,
			TaskID:       taskID,
			CreatedAt:    createdAt,
			AskingTurnID: turnID,
		}
		switch {
		case isSuperseded:
			summary.Reason = models.ClarificationHistoryReasonSuperseded
			if turnMismatch == 1 && currentTurnID.Valid {
				summary.SupersedingTurnID = currentTurnID.String
			}
		case isSessionEnded == 1:
			summary.Reason = models.ClarificationHistoryReasonSessionEnded
		case isUnreadable == 1:
			summary.Reason = models.ClarificationHistoryReasonUnreadable
		}
		bundles = append(bundles, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bundles, nil
}
