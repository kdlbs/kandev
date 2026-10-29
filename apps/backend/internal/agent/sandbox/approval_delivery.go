package sandbox

import (
	"context"
)

// FinishApproval records the outcome of one response write. It cannot authorize a retry.
func (s *Service) FinishApproval(ctx context.Context, orgID, attemptID, requestID string, expected int64, sent bool) (*Approval, error) {
	state := ApprovalUndelivered
	if sent {
		state = ApprovalSent
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_approval_requests SET state=?,version=version+1 WHERE org_id=? AND attempt_id=? AND request_id=? AND version=? AND state=?`), state, orgID, attemptID, requestID, expected, ApprovalResponding)
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
	s.notifyApprovalChange(ctx, orgID, attemptID)
	return s.ReadApproval(ctx, orgID, attemptID, requestID)
}

// CloseApprovals invalidates every response that was not already sent or refused.
func (s *Service) CloseApprovals(ctx context.Context, orgID, attemptID string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_approval_requests SET state=?,version=version+1 WHERE org_id=? AND attempt_id=? AND state IN (?,?,?)`), ApprovalUndelivered, orgID, attemptID, ApprovalPending, ApprovalApproved, ApprovalResponding)
	if err != nil {
		return err
	}
	return s.notifyApprovalResult(ctx, orgID, attemptID, result)
}

func (s *Service) abandonApproval(ctx context.Context, orgID, attemptID, requestID string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_approval_requests SET state=?,version=version+1 WHERE org_id=? AND attempt_id=? AND request_id=? AND state IN (?,?,?)`), ApprovalUndelivered, orgID, attemptID, requestID, ApprovalPending, ApprovalApproved, ApprovalResponding)
	if err != nil {
		return err
	}
	return s.notifyApprovalResult(ctx, orgID, attemptID, result)
}
