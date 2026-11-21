package state

import (
	"context"
	"fmt"
)

// ListKeys reads only keys in one plugin namespace. after is exclusive, including
// when it points to an empty key. The extra row determines continuation.
func (s *Store) ListKeys(ctx context.Context, pluginID, scope, scopeID string, after *string, limit int) ([]string, bool, error) {
	if limit < 1 || limit > 200 {
		return nil, false, fmt.Errorf("state key page limit must be between 1 and 200")
	}
	query := `SELECT state_key FROM plugin_state WHERE plugin_id=? AND scope=? AND scope_id=?`
	args := []any{pluginID, scope, scopeID}
	if after != nil {
		query += ` AND state_key>?`
		args = append(args, *after)
	}
	query += ` ORDER BY state_key ASC LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	keys := make([]string, 0, limit+1)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, false, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(keys) > limit
	if more {
		keys = keys[:limit]
	}
	return keys, more, nil
}
