package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/db"
)

// LatestSessionAttempt is an internal lookup. Public readers must authorize the
// session and workspace before projecting any captured execution details.
func (s *Service) LatestSessionAttempt(ctx context.Context, orgID, sessionID string) (*Attempt, error) {
	if sessionID == "" {
		return nil, ErrNotFound
	}
	var id string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT attempt_id FROM sandbox_attempts WHERE org_id=? AND session_id=? ORDER BY updated_at DESC,attempt_id DESC LIMIT 1`), orgID, sessionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.ReadAttempt(ctx, orgID, id)
}

func (s *Service) initAttemptSessions() error {
	migration := db.NewRequiredMigrateLogger(s.db, nil)
	if err := migration.Apply("sandbox_attempts.session_id", `ALTER TABLE sandbox_attempts ADD COLUMN session_id TEXT`); err != nil {
		return err
	}
	for {
		count, err := s.backfillAttemptSessions()
		if err != nil {
			return err
		}
		if count == 0 {
			break
		}
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS sandbox_attempts_session_history ON sandbox_attempts(org_id,session_id,updated_at);
 CREATE UNIQUE INDEX IF NOT EXISTS sandbox_attempts_active_session ON sandbox_attempts(org_id,session_id)
 WHERE session_id<>'' AND state IN ('prepared','starting','running','recovery_required');`)
	return err
}

func (s *Service) backfillAttemptSessions() (int, error) {
	rows, err := s.db.Query(`SELECT org_id,attempt_id,snapshot_json FROM sandbox_attempts WHERE session_id IS NULL LIMIT 256`)
	if err != nil {
		return 0, err
	}
	type item struct{ org, id, raw, session string }
	pending := make([]item, 0, 256)
	for rows.Next() {
		var value item
		if err = rows.Scan(&value.org, &value.id, &value.raw); err != nil {
			_ = rows.Close()
			return 0, err
		}
		var snapshot struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(value.raw), &snapshot) != nil {
			_ = rows.Close()
			return 0, ErrConflict
		}
		value.session = snapshot.SessionID
		pending = append(pending, value)
	}
	scanErr := rows.Err()
	if err = rows.Close(); err != nil {
		return 0, err
	}
	if scanErr != nil {
		return 0, scanErr
	}
	for _, value := range pending {
		_, err = s.db.ExecContext(context.Background(), s.db.Rebind(`UPDATE sandbox_attempts SET session_id=? WHERE org_id=? AND attempt_id=? AND session_id IS NULL`), value.session, value.org, value.id)
		if err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}
