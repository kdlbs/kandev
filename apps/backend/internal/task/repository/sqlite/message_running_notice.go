package sqlite

import (
	"context"
	"fmt"
	"time"
)

// GetLastAgentActivityForTurn reads resolution evidence across the whole turn
// without loading tool payloads or depending on the transcript's current page.
func (r *Repository) GetLastAgentActivityForTurn(ctx context.Context, sessionID, turnID string) (time.Time, error) {
	query := `SELECT MAX(CASE WHEN updated_at > created_at THEN updated_at ELSE created_at END)
		FROM task_session_messages
		WHERE task_session_id = ? AND turn_id = ? AND author_type = 'agent'
		AND type IN ('message', 'content', 'thinking', 'tool_call', 'tool_read',
			'tool_edit', 'tool_execute', 'tool_search', 'agent_plan', 'todo', 'permission_request')`
	var raw interface{}
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), sessionID, turnID).Scan(&raw); err != nil {
		return time.Time{}, fmt.Errorf("read agent activity for turn: %w", err)
	}
	if raw == nil {
		return time.Time{}, nil
	}
	return parseTaskActivityTime(raw)
}
