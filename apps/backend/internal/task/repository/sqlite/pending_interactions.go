package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/tracing"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// pendingInteractionColumns is the message projection both pending-interaction
// CTEs select. Kept identical to scanMessageRows' expected column order so the
// UNION ALL below stays scannable by the shared helper.
const pendingInteractionColumns = `m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type,
	m.author_id, m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at`

// ListPendingInteractions returns the durable request rows for every agent
// interaction still owed a user response, using the SAME authority the compact
// pending-action projection uses (GetPendingActionsBySessionIDs): only the
// session's current durable turn counts, terminal sessions quarantine pending
// history, and a permission row only counts when it is the newest permission of
// that turn. Anything weaker re-reports interactions kandev's own list surfaces
// already consider settled — the false-positive class ADR 0052 exists to close.
//
// Clarification bundles come back as one row per question (all sharing
// metadata.pending_id); the service layer groups them into a single
// interaction. A bundle qualifies when ANY of its questions is still pending,
// and then EVERY question of that bundle is returned: the agent stays blocked
// until the whole bundle is answered, so a caller handed only a partially
// answered bundle's pending rows would build a response the answer path
// rejects for having too few answers. It also keeps this read and
// GetInteraction (which resolves by pending_id) agreeing on a question set.
// Rows are ordered oldest-first by (created_at, id).
func (r *Repository) ListPendingInteractions(
	ctx context.Context,
	filter models.PendingInteractionFilter,
) ([]*models.Message, error) {
	ctx, span := tracing.Tracer("kandev-db").Start(ctx, "db.ListPendingInteractions")
	defer span.End()

	query, args := buildPendingInteractionQuery(r.ro.DriverName(), filter)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	messages, _, err := scanMessageRows(rows, 0)
	return messages, err
}

// buildPendingInteractionQuery assembles the pending-interaction query. The
// session-scoping subquery is shared by the current-turn CTE and both message
// CTEs, so a session/task filter narrows the authority window itself rather
// than filtering rows the CTEs already resolved against a wider set.
func buildPendingInteractionQuery(
	driverName string,
	filter models.PendingInteractionFilter,
) (string, []interface{}) {
	statusExpr := dialect.JSONExtract(driverName, "m.metadata", "status")
	pendingIDExpr := dialect.JSONExtract(driverName, "m.metadata", "pending_id")
	scopedSessions, args := pendingInteractionSessionScope(filter)
	kindConditions, kindArgs := pendingInteractionKindClauses(filter.Kinds)

	query := fmt.Sprintf(
		pendingInteractionQueryTemplate,
		scopedSessions,
		turnAuthorityPredicate(driverName, "turn_row"),
		nonTerminalSessionPredicate("turn_row"),
		pendingIDExpr,
		models.MessageTypeClarificationRequest,
		statusExpr,
		models.InteractionStatusPending,
		pendingInteractionColumns,
		pendingIDExpr,
		models.MessageTypeClarificationRequest,
		pendingInteractionColumns,
		statusExpr,
		pendingActionMessageOrder(driverName, "m"),
		models.MessageTypePermissionRequest,
		models.InteractionStatusPending,
	)
	if kindConditions != "" {
		query = "SELECT * FROM (" + query + ") interactions WHERE " + kindConditions
		args = append(args, kindArgs...)
	}
	query += " ORDER BY created_at ASC, id ASC"
	return query, args
}

// pendingInteractionQueryTemplate is the format string behind
// buildPendingInteractionQuery. current_turn duplicates the ranking in
// pendingActionsBySessionQuery on purpose: both must agree, and a shared
// builder would have to thread the scoping subquery's bind arguments through
// the projection's placeholder list. The pinning test
// (TestListPendingInteractionsAgreesWithPendingActionProjection) fails if the
// two ever disagree on a session.
const pendingInteractionQueryTemplate = `
		WITH scoped_sessions AS (%s),
		current_turn AS (
			SELECT task_session_id, turn_id
			FROM (
				SELECT task_session_id,
				       id AS turn_id,
				       ROW_NUMBER() OVER (
				         PARTITION BY task_session_id
				         ORDER BY started_at DESC, created_at DESC, id DESC
				       ) AS rn
				FROM task_session_turns turn_row
				WHERE turn_row.task_session_id IN (SELECT id FROM scoped_sessions)
				  AND %s
				  AND %s
			) ranked
			WHERE rn = 1
		),
		pending_bundles AS (
			SELECT DISTINCT m.task_session_id, %s AS pending_id
			FROM task_session_messages m
			JOIN current_turn current
			  ON current.task_session_id = m.task_session_id
			 AND current.turn_id = m.turn_id
			WHERE m.type = '%s'
			  AND COALESCE(%s, '') IN ('', '%s')
		),
		pending_clarifications AS (
			SELECT %s
			FROM task_session_messages m
			JOIN current_turn current
			  ON current.task_session_id = m.task_session_id
			 AND current.turn_id = m.turn_id
			JOIN pending_bundles bundle
			  ON bundle.task_session_id = m.task_session_id
			 AND bundle.pending_id = %s
			WHERE m.type = '%s'
		),
		ranked_permissions AS (
			SELECT %s,
			       COALESCE(%s, '') AS permission_status,
			       ROW_NUMBER() OVER (
			         PARTITION BY m.task_session_id
			         ORDER BY m.created_at DESC, %s DESC
			       ) AS rn
			FROM task_session_messages m
			JOIN current_turn current
			  ON current.task_session_id = m.task_session_id
			 AND current.turn_id = m.turn_id
			WHERE m.type = '%s'
		)
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id,
		       content, requests_input, type, metadata, created_at, updated_at
		FROM pending_clarifications
		UNION ALL
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id,
		       content, requests_input, type, metadata, created_at, updated_at
		FROM ranked_permissions
		WHERE rn = 1 AND permission_status IN ('', '%s')
	`

// pendingInteractionSessionScope builds the scoped_sessions subquery. Session
// and task filters are ANDed (a session must match both axes when both are
// set), matching PluginMessageFilter's documented semantics.
func pendingInteractionSessionScope(filter models.PendingInteractionFilter) (string, []interface{}) {
	query := "SELECT id FROM task_sessions"
	var conditions []string
	var args []interface{}
	if ph, inArgs := buildInPlaceholders(filter.SessionIDs); ph != "" {
		conditions = append(conditions, "id IN ("+ph+")")
		args = append(args, inArgs...)
	}
	if ph, inArgs := buildInPlaceholders(filter.TaskIDs); ph != "" {
		conditions = append(conditions, "task_id IN ("+ph+")")
		args = append(args, inArgs...)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	return query, args
}

// pendingInteractionKindClauses narrows the UNION result to the requested
// interaction kinds by mapping each kind back to its message type. An unknown
// kind matches nothing rather than everything, so a typo fails closed.
func pendingInteractionKindClauses(kinds []string) (string, []interface{}) {
	if len(kinds) == 0 {
		return "", nil
	}
	types := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		switch models.InteractionKind(kind) {
		case models.InteractionKindClarification:
			types = append(types, string(models.MessageTypeClarificationRequest))
		case models.InteractionKindPermission:
			types = append(types, string(models.MessageTypePermissionRequest))
		}
	}
	if len(types) == 0 {
		return "1 = 0", nil
	}
	ph, args := buildInPlaceholders(types)
	return "type IN (" + ph + ")", args
}
