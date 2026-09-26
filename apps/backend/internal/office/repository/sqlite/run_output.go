package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// maxOutputSummaryChars caps the run output summary at the same length as
// the per-entity summaries this subsystem already truncates (see
// maxCommentChars in blockers.go) rather than introducing a new limit.
const maxOutputSummaryChars = 500

// GetFinalAgentMessage returns the most recent agent-authored message body
// created at or after since, for a session, truncated to
// maxOutputSummaryChars. Returns "" (not an error) when no matching message
// exists, so callers can treat the result as best-effort.
//
// The since bound matters because office task-bound sessions are reused
// across runs (same DB session_id across turns — see
// handleAgentTurnMessageSaved's doc comment). Without it, a run that
// completes without producing a new agent message would report a prior
// run's message as its own output instead of staying empty. Callers pass
// the run's claimed_at as since.
//
// id DESC is a secondary sort key only for determinism when two messages
// share a created_at value (matches the tie-breaker convention documented
// in task/repository/sqlite/session.go and used by message.go); message
// ids are random, so it is not a true arrival-order signal.
func (r *Repository) GetFinalAgentMessage(ctx context.Context, sessionID string, since time.Time) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	var content string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT SUBSTR(content, 1, ?)
		FROM task_session_messages
		WHERE task_session_id = ? AND type = 'message' AND author_type = 'agent' AND created_at >= ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`), maxOutputSummaryChars, sessionID, since).Scan(&content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return content, nil
}
