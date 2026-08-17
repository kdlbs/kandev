package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// clarificationTerminalStatuses mirrors clarification.Status's four terminal
// values (clarification/types.go:69-75, spec D3). Duplicated as literals
// rather than imported so the repository layer does not take on a
// dependency edge on the clarification feature package.
const clarificationTerminalStatusesSQL = `'answered', 'rejected', 'cancelled', 'expired'`

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

// clarificationBundleQuery assembles the bundle-grouping subquery (D4a both
// conjuncts via has_pending and the NOT EXISTS resolution check) joined to
// its visibility-checked task row.
func clarificationBundleQuery(drv, whereExtra string) string {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(drv, "m.metadata", "status")
	return fmt.Sprintf(`
		SELECT b.pending_id, b.session_id, b.task_id, b.created_at
		FROM (
			SELECT
				%[1]s AS pending_id,
				m.task_session_id AS session_id,
				COALESCE(NULLIF(MIN(m.task_id), ''), MIN(ts.task_id)) AS task_id,
				MIN(m.created_at) AS created_at,
				MAX(CASE WHEN %[2]s IS NULL OR %[2]s NOT IN (%[3]s) THEN 1 ELSE 0 END) AS has_pending
			FROM task_session_messages m
			JOIN task_sessions ts ON ts.id = m.task_session_id
			WHERE m.type = 'clarification_request'
			GROUP BY %[1]s, m.task_session_id
		) b
		JOIN tasks t ON t.id = b.task_id
		WHERE b.has_pending = 1
		  AND NOT EXISTS (SELECT 1 FROM clarification_resolutions cr WHERE cr.pending_id = b.pending_id)
		  %[4]s
		ORDER BY b.created_at ASC, b.pending_id ASC
		LIMIT ?
	`, pendingIDExpr, statusExpr, clarificationTerminalStatusesSQL, whereExtra)
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

// visibilityPredicate builds the L1c/L1d disjunction (per the plan's
// corrected numbering, not the spec's defective L1c prose — see
// docs/plans/external-question-answering/plan.md, "BUILD MUST READ FIRST").
// For an unscoped caller the predicate is satisfied unconditionally, so no
// clause is added at all.
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
