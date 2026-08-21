package sqlite

import (
	"context"
	"database/sql"
	"errors"
)

// maxOutputSummaryChars caps the run output summary at the same length as
// the per-entity summaries this subsystem already truncates (see
// maxCommentChars in blockers.go) rather than introducing a new limit.
const maxOutputSummaryChars = 500

// GetFinalAgentMessage returns the most recent agent-authored message body
// for a session, truncated to maxOutputSummaryChars. Returns "" (not an
// error) when the session has no agent message, so callers can treat the
// result as best-effort.
func (r *Repository) GetFinalAgentMessage(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		return "", nil
	}
	var content string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT SUBSTR(content, 1, ?)
		FROM task_session_messages
		WHERE task_session_id = ? AND type = 'message' AND author_type = 'agent'
		ORDER BY created_at DESC
		LIMIT 1
	`), maxOutputSummaryChars, sessionID).Scan(&content)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return content, nil
}
