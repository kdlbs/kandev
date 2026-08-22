package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ListUnresolvedClarificationBundles returns the page of pending clarification
// bundles matching opts, per spec D4a (both conjuncts), L1a-L7a's visibility
// predicate, L8's created_since filter, and L9/D6's cursor pagination. Every
// predicate is applied inside this single query — never as a post-query
// filter over an already-limited page (L1a) — so opts.Limit counts bundles
// actually returned.
func (r *Repository) ListUnresolvedClarificationBundles(ctx context.Context, opts models.ListClarificationBundlesOptions) (*models.ClarificationBundlePage, error) {
	if opts.Limit < 1 {
		return nil, fmt.Errorf("ListUnresolvedClarificationBundles: limit must be >= 1, got %d", opts.Limit)
	}

	drv := r.ro.DriverName()
	whereExtra, args := clarificationBundleWhereClause(opts)
	query := clarificationBundleQuery(drv, whereExtra)
	args = append(args, opts.Limit+1)

	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
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

// clarificationBundleWhereClause builds the extra AND-ed WHERE conditions
// (visibility, L7 workspace filter, L8 created_since, L9/D6 cursor) and their
// bound args, in the same order the conditions appear.
func clarificationBundleWhereClause(opts models.ListClarificationBundlesOptions) (string, []interface{}) {
	var args []interface{}
	var conditions []string

	conditions = append(conditions, visibilityPredicate(opts, &args)...)
	if opts.WorkspaceID != "" {
		conditions = append(conditions, "t.workspace_id = ?")
		args = append(args, opts.WorkspaceID)
	}
	if opts.CreatedSince != nil {
		conditions = append(conditions, "b.created_at >= ?")
		args = append(args, *opts.CreatedSince)
	}
	if opts.CursorPendingID != "" {
		conditions = append(conditions, "(b.created_at > ? OR (b.created_at = ? AND b.pending_id > ?))")
		args = append(args, opts.CursorCreatedAt, opts.CursorCreatedAt, opts.CursorPendingID)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return "AND " + strings.Join(conditions, " AND "), args
}

// clarificationBundleQuery assembles the bundle-grouping subquery joined to
// its visibility-checked task row. Per spec D4/L2a, a bundle is pending only
// when all three conjuncts hold: at least one message has status in (”,
// 'pending') (has_pending), the message's turn is the session's current turn,
// and the owning session is not terminal. All three conjuncts mirror
// claimActiveClarificationBundle's own claim predicate exactly — has_pending
// is the same COALESCE(status, ”) IN (”, 'pending') test, not an
// independently-derived "not a known terminal value" negation, and the
// latter two reuse claimActiveClarificationBundle's own helpers
// (turnAuthorityPredicate, nonTerminalSessionPredicate) rather than
// hand-written — per L2a, so a future change to activeness cannot leave this
// tool behind, and so a status value outside both sets (a corrupted or
// future-version row) cannot be listed as claimable when it can never
// actually be claimed.
//
// It also excludes any message carrying the autopilot parent-question
// marker (models.MetaKeyParentQuestion, "parent_question.go"). That
// protocol's durable records share this table's message type and its
// status/pending_id metadata keys with an ordinary clarification bundle,
// but they are resolved exclusively through message_task_kandev's
// reply_to_question_id path, gated on the direct-parent ownership check in
// validateParentQuestionOwnership. Admitting them here would let any
// workspace-authorized external caller list and answer a running autopilot
// agent's question to its parent, bypassing that ownership check and the
// parent-only interaction boundary the spec's S2 relies on (Review round 2
// finding).
//
// Per spec L16, it also excludes any bundle where a message's question_id
// cannot be resolved: the flat metadata.question_id key, falling back to the
// nested metadata.question.id key, mirroring questionIDFromMetadata
// (bundle_question.go) exactly so this list stays consistent with what
// answer_question_kandev's pre-claim validation already rejects. A bundle
// admitted here with no resolvable question_id could be listed but could
// never be answered.
func clarificationBundleQuery(drv, whereExtra string) string {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(drv, "m.metadata", "status")
	questionIDExpr := fmt.Sprintf(
		"COALESCE(NULLIF(%s, ''), NULLIF(%s, ''), '')",
		dialect.JSONExtract(drv, "m.metadata", "question_id"),
		dialect.JSONExtractPath(drv, "m.metadata", "question", "id"),
	)
	notParentQuestion := dialect.ExcludeTruthyMetadataPredicate(drv, "m.metadata", "parent_question")
	nonTerminalSession := nonTerminalSessionPredicate("m")
	currentTurn := fmt.Sprintf(`m.turn_id = (
			SELECT turn_row.id
			FROM task_session_turns turn_row
			WHERE turn_row.task_session_id = m.task_session_id
			  AND %s
			ORDER BY turn_row.started_at DESC, turn_row.created_at DESC, turn_row.id DESC
			LIMIT 1
		  )`, turnAuthorityPredicate(drv, "turn_row"))
	return fmt.Sprintf(`
		SELECT b.pending_id, b.session_id, b.task_id, b.created_at
		FROM (
			SELECT
				%[1]s AS pending_id,
				m.task_session_id AS session_id,
				COALESCE(NULLIF(MIN(m.task_id), ''), MIN(ts.task_id)) AS task_id,
				MIN(m.created_at) AS created_at,
				MAX(CASE WHEN COALESCE(%[2]s, '') IN ('', 'pending') THEN 1 ELSE 0 END) AS has_pending,
				MAX(CASE WHEN %[7]s = '' THEN 1 ELSE 0 END) AS has_missing_question_id
			FROM task_session_messages m
			JOIN task_sessions ts ON ts.id = m.task_session_id
			WHERE m.type = 'clarification_request'
			  AND %[4]s
			  AND %[5]s
			  AND %[6]s
			GROUP BY %[1]s, m.task_session_id
		) b
		JOIN tasks t ON t.id = b.task_id
		WHERE b.has_pending = 1
		  AND b.has_missing_question_id = 0
		  %[3]s
		ORDER BY b.created_at ASC, b.pending_id ASC
		LIMIT ?
	`, pendingIDExpr, statusExpr, whereExtra, notParentQuestion, nonTerminalSession, currentTurn, questionIDExpr)
}

// scanClarificationBundleRows scans the bundle rows. The created_at column
// is a MIN() aggregate, not a direct column reference: SQLite's driver only
// auto-converts to time.Time when it can see the declared column type, which
// MIN() loses (see subagentContextBackfillHighWaterMark); PostgreSQL's
// driver has no such gap.
func scanClarificationBundleRows(rows *sql.Rows, isPostgres bool) ([]models.ClarificationBundleSummary, error) {
	var bundles []models.ClarificationBundleSummary
	for rows.Next() {
		var b models.ClarificationBundleSummary
		if isPostgres {
			if err := rows.Scan(&b.PendingID, &b.SessionID, &b.TaskID, &b.CreatedAt); err != nil {
				return nil, err
			}
		} else {
			var createdAtRaw string
			if err := rows.Scan(&b.PendingID, &b.SessionID, &b.TaskID, &createdAtRaw); err != nil {
				return nil, err
			}
			b.CreatedAt = parseLegacyTimestamp(createdAtRaw)
		}
		bundles = append(bundles, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bundles, nil
}

// visibilityPredicate builds the L1c/L1d three-way disjunction (spec
// docs/specs/external-question-answering/spec.md, "L1c"/"L1d"): the task's
// workspace_id is empty, names no existing workspace row, or is in the L1a
// visible set. For an unscoped caller the predicate is satisfied
// unconditionally, so no clause is added at all.
func visibilityPredicate(opts models.ListClarificationBundlesOptions, args *[]interface{}) []string {
	if opts.Unscoped {
		return nil
	}
	disjuncts := []string{
		"t.workspace_id = ''",
		"NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.id = t.workspace_id)",
	}
	if len(opts.VisibleWorkspaceIDs) > 0 {
		placeholders := make([]string, len(opts.VisibleWorkspaceIDs))
		for i, id := range opts.VisibleWorkspaceIDs {
			placeholders[i] = "?"
			*args = append(*args, id)
		}
		disjuncts = append(disjuncts, fmt.Sprintf("t.workspace_id IN (%s)", strings.Join(placeholders, ", ")))
	}
	return []string{"(" + strings.Join(disjuncts, " OR ") + ")"}
}
