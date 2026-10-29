package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// CaptureAttempt persists a core-owned launch after rechecking its selected revision.
func (s *Service) CaptureAttempt(ctx context.Context, request CaptureRequest) (*Attempt, error) {
	snapshot, err := s.captureSnapshot(ctx, request)
	if err != nil {
		return nil, err
	}
	if err = s.insertCapturedAttempt(ctx, request, snapshot); err != nil {
		return nil, err
	}
	s.notifyAttemptChange()
	return s.ReadAttempt(ctx, snapshot.OrgID, request.Launch.AttemptID)
}

func (s *Service) insertCapturedAttempt(ctx context.Context, request CaptureRequest, snapshot AttemptSnapshot) error {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > 1<<20 {
		return contract.ErrInvalidPolicy
	}
	query := `INSERT INTO sandbox_attempts (org_id,attempt_id,execution_id,session_id,generation,version,state,snapshot_json,owner_json,updated_at)
 SELECT ?,?,?,?,?,1,?,?,?,? WHERE 1=1`
	args := []any{snapshot.OrgID, request.Launch.AttemptID, snapshot.ExecutionID, snapshot.SessionID, request.Launch.Generation, AttemptPrepared, string(raw), "null", time.Now().UTC().Format(time.RFC3339Nano)}
	query, args = appendCaptureAuthority(query, args, snapshot)
	query += ` ON CONFLICT DO NOTHING`
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

func appendCaptureAuthority(query string, args []any, snapshot AttemptSnapshot) (string, []any) {
	query, args = appendRevisionAuthority(query, args, snapshot)
	if snapshot.RestartOf != "" {
		query += ` AND EXISTS (SELECT 1 FROM sandbox_attempts a WHERE a.org_id=? AND a.attempt_id=? AND a.version=? AND a.state=?)`
		args = append(args, snapshot.OrgID, snapshot.RestartOf, snapshot.RestartVersion, AttemptStopped)
		if snapshot.RestartReview == nil {
			return query, args
		}
	}
	for _, binding := range snapshot.Resolved.Bindings {
		query += ` AND COALESCE((SELECT b.version FROM sandbox_policy_bindings b WHERE b.org_id=? AND b.scope=? AND b.target_id=?),0)=?`
		args = append(args, snapshot.OrgID, binding.Scope, binding.TargetID, binding.Version)
	}
	return query, args
}

func appendRevisionAuthority(query string, args []any, snapshot AttemptSnapshot) (string, []any) {
	for _, revision := range []*Revision{snapshot.Resolved.Revision, snapshot.Resolved.Ceiling} {
		query += ` AND EXISTS (SELECT 1 FROM sandbox_policy_revisions r WHERE r.org_id=? AND r.id=? AND r.revoked=0)`
		args = append(args, snapshot.OrgID, revision.ID)
	}
	return query, args
}

// ReadAttempt is an internal owner lookup. Public handlers must authorize the workspace first.
func (s *Service) ReadAttempt(ctx context.Context, orgID, attemptID string) (*Attempt, error) {
	var snapshot, owner, updated, executionID, sessionID string
	var generation uint64
	result := &Attempt{ID: attemptID}
	err := s.db.QueryRowxContext(ctx, s.db.Rebind(`SELECT version,state,snapshot_json,owner_json,updated_at,execution_id,generation,session_id FROM sandbox_attempts WHERE org_id=? AND attempt_id=?`), orgID, attemptID).Scan(&result.Version, &result.State, &snapshot, &owner, &updated, &executionID, &generation, &sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(snapshot), &result.Snapshot); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(owner), &result.Owner); err != nil {
		return nil, err
	}
	result.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return nil, err
	}
	if !validStoredAttempt(result, orgID, executionID, generation) || result.Snapshot.SessionID != sessionID {
		return nil, ErrConflict
	}
	return result, nil
}

// TransitionAttempt changes only lifecycle evidence. The captured policy cannot change.
func (s *Service) TransitionAttempt(ctx context.Context, orgID, attemptID string, expected int64, state string, owner *AttemptOwner) (*Attempt, error) {
	current, err := s.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return nil, err
	}
	if current.Version != expected {
		return nil, ErrConflict
	}
	if !validAttemptTransition(current.State, state) {
		return nil, ErrConflict
	}
	if owner == nil {
		owner = current.Owner
	}
	if !validAttemptOwner(current.Owner, owner, state) {
		return nil, contract.ErrInvalidPolicy
	}
	raw, err := json.Marshal(owner)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	query := `UPDATE sandbox_attempts SET version=version+1,state=?,owner_json=?,updated_at=? WHERE org_id=? AND attempt_id=? AND version=?`
	arguments := []any{state, string(raw), now.Format(time.RFC3339Nano), orgID, attemptID, expected}
	if state == AttemptStarting {
		query, arguments = appendRevisionAuthority(query, arguments, current.Snapshot)
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), arguments...)
	if err != nil {
		return nil, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, ErrConflict
	}
	s.notifyAttemptChange()
	return s.ReadAttempt(ctx, orgID, attemptID)
}
