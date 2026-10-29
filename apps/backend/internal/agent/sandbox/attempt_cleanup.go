package sandbox

import "context"

type CleanupCursor struct{ OrgID, AttemptID string }

// HasSessionAttempts checks retained ownership without opening a runtime.
func (s *Service) HasSessionAttempts(ctx context.Context, sessionID string) (bool, error) {
	if sessionID == "" {
		return false, ErrConflict
	}
	var found bool
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT EXISTS(SELECT 1 FROM sandbox_attempts WHERE session_id=?)`), sessionID).Scan(&found)
	return found, err
}

// SessionCleanupAttempts is a trusted lifecycle lookup. The caller must
// serialize deletion and stop admission for the exact task/session first.
func (s *Service) SessionCleanupAttempts(ctx context.Context, taskID, sessionID string, after CleanupCursor) ([]*Attempt, error) {
	if sessionID == "" {
		return nil, ErrConflict
	}
	keys := []struct {
		OrgID string `db:"org_id"`
		ID    string `db:"attempt_id"`
	}{}
	err := s.db.SelectContext(ctx, &keys, s.db.Rebind(`SELECT org_id,attempt_id FROM sandbox_attempts
 WHERE session_id=? AND (org_id>? OR (org_id=? AND attempt_id>?)) ORDER BY org_id,attempt_id LIMIT 64`), sessionID, after.OrgID, after.OrgID, after.AttemptID)
	if err != nil {
		return nil, err
	}
	result := make([]*Attempt, 0, len(keys))
	for _, key := range keys {
		attempt, err := s.ReadAttempt(ctx, key.OrgID, key.ID)
		if err != nil {
			return nil, err
		}
		if attempt.Snapshot.TaskID != taskID || attempt.Snapshot.SessionID != sessionID {
			return nil, ErrConflict
		}
		result = append(result, attempt)
	}
	return result, nil
}
