package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

// RecordApproval accepts only a request already bound by core to an owned attempt.
func (s *Service) RecordApproval(ctx context.Context, orgID, attemptID string, operation ApprovalOperation, sourceDigest string, deadline time.Time) (*Approval, error) {
	attempt, err := s.ReadAttempt(ctx, orgID, attemptID)
	if err != nil {
		return nil, err
	}
	if err = s.validateApprovalCapture(attempt, operation, sourceDigest, deadline); err != nil {
		return nil, err
	}
	snapshot := ApprovalSnapshot{RevisionID: attempt.Snapshot.Resolved.Revision.ID, CeilingRevisionID: attempt.Snapshot.Resolved.Ceiling.ID, OrgID: orgID, WorkspaceID: attempt.Snapshot.WorkspaceID, SessionID: attempt.Snapshot.SessionID, AttemptID: attemptID,
		Generation: attempt.Snapshot.Launch.Generation, PolicyDigest: attempt.Snapshot.Launch.PolicyDigest, Owner: *attempt.Owner,
		Operation: operation, SourceDigest: sourceDigest, CreatedAt: time.Now().UTC(), ExpiresAt: deadline.UTC()}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO sandbox_approval_requests (org_id,attempt_id,request_id,version,state,operator_id,expires_at_ms,session_id,snapshot_json)
 VALUES (?,?,?,1,?,'',?,?,?) ON CONFLICT DO NOTHING`), orgID, attemptID, operation.RequestID, ApprovalPending, deadline.UnixMilli(), snapshot.SessionID, string(raw))
	if err != nil {
		return nil, err
	}
	recorded, err := s.ReadApproval(ctx, orgID, attemptID, operation.RequestID)
	if err != nil {
		return nil, err
	}
	if recorded.Snapshot.SourceDigest != sourceDigest || recorded.Snapshot.Operation != operation || recorded.Snapshot.Owner != snapshot.Owner {
		return nil, ErrConflict
	}
	return recorded, s.notifyApprovalResult(ctx, orgID, attemptID, result)
}

func (s *Service) ReadApproval(ctx context.Context, orgID, attemptID, requestID string) (*Approval, error) {
	var raw string
	approval := &Approval{}
	err := s.db.QueryRowxContext(ctx, s.db.Rebind(`SELECT version,state,operator_id,snapshot_json FROM sandbox_approval_requests WHERE org_id=? AND attempt_id=? AND request_id=?`), orgID, attemptID, requestID).Scan(&approval.Version, &approval.State, &approval.OperatorID, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &approval.Snapshot); err != nil {
		return nil, err
	}
	return approval, nil
}

func (s *Service) DecideApproval(ctx context.Context, attemptID, requestID string, expected int64, decision string) (*Approval, error) {
	identity, err := s.operator(ctx)
	if err != nil {
		return nil, err
	}
	if decision != "allow_once" && decision != "deny" {
		return nil, contract.ErrInvalidPolicy
	}
	approval, err := s.ReadApproval(ctx, identity.OrgID, attemptID, requestID)
	if err != nil {
		return nil, err
	}
	if err = s.sessionAccess(ctx, approval.Snapshot.SessionID); err != nil {
		return nil, err
	}
	if err = s.workspaceAccess(ctx, approval.Snapshot.WorkspaceID); err != nil {
		return nil, ErrAuthority
	}
	if approval.Version != expected || approval.State != ApprovalPending || !time.Now().Before(approval.Snapshot.ExpiresAt) {
		return nil, ErrConflict
	}
	state := ApprovalDenied
	if decision == "allow_once" {
		if err = s.admitApprovalGrant(ctx, approval); err != nil {
			return nil, err
		}
		state = ApprovalApproved
	}
	return s.changeApprovalState(ctx, approval, state, identity.UserID)
}

// ClaimApproval consumes an approved response before its single delivery attempt.
func (s *Service) ClaimApproval(ctx context.Context, orgID, attemptID, requestID string, expected int64) (*Approval, error) {
	approval, err := s.ReadApproval(ctx, orgID, attemptID, requestID)
	if err != nil {
		return nil, err
	}
	if approval.Version != expected || approval.State != ApprovalApproved || !time.Now().Before(approval.Snapshot.ExpiresAt) {
		return nil, ErrConflict
	}
	if err = s.admitApprovalGrant(ctx, approval); err != nil {
		return nil, err
	}
	return s.changeApprovalState(ctx, approval, ApprovalResponding, approval.OperatorID)
}

func (s *Service) changeApprovalState(ctx context.Context, current *Approval, state, operatorID string) (*Approval, error) {
	snapshot := current.Snapshot
	query := `UPDATE sandbox_approval_requests SET state=?,operator_id=?,version=version+1 WHERE org_id=? AND attempt_id=? AND request_id=? AND version=? AND state=? AND expires_at_ms>?`
	arguments := []any{state, operatorID, snapshot.OrgID, snapshot.AttemptID, snapshot.Operation.RequestID, current.Version, current.State, time.Now().UnixMilli()}
	if state == ApprovalApproved || state == ApprovalResponding {
		owner, err := json.Marshal(snapshot.Owner)
		if err != nil {
			return nil, err
		}
		query += ` AND EXISTS (SELECT 1 FROM sandbox_attempts a WHERE a.org_id=? AND a.attempt_id=? AND a.state=? AND a.owner_json=?)`
		arguments = append(arguments, snapshot.OrgID, snapshot.AttemptID, AttemptRunning, string(owner))
		for _, revisionID := range []string{snapshot.RevisionID, snapshot.CeilingRevisionID} {
			query += ` AND EXISTS (SELECT 1 FROM sandbox_policy_revisions r WHERE r.org_id=? AND r.id=? AND r.revoked=0)`
			arguments = append(arguments, snapshot.OrgID, revisionID)
		}

	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), arguments...)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, ErrConflict
	}
	s.notifyApprovalChange(ctx, snapshot.OrgID, snapshot.AttemptID)
	return s.ReadApproval(ctx, snapshot.OrgID, snapshot.AttemptID, snapshot.Operation.RequestID)
}
