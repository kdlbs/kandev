package sandbox

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/mcp/origin"
	"github.com/kandev/kandev/internal/mcp/scope"
)

// ApprovalView excludes callback capabilities, runtime paths, and process identities.
type ApprovalView struct {
	RequestID    string    `json:"request_id"`
	AttemptID    string    `json:"attempt_id"`
	SessionID    string    `json:"session_id"`
	Version      int64     `json:"version"`
	State        string    `json:"state"`
	Access       string    `json:"access"`
	Target       string    `json:"target"`
	PolicyDigest string    `json:"policy_digest"`
	ExpiresAt    time.Time `json:"expires_at"`
	CanDecide    bool      `json:"can_decide"`
}

func (s *Service) approvalView(ctx context.Context, approval *Approval) ApprovalView {
	snapshot := approval.Snapshot
	canDecide := false
	if _, err := s.operator(ctx); err == nil && approval.State == ApprovalPending && time.Now().Before(snapshot.ExpiresAt) {
		canDecide = s.admitApprovalGrant(ctx, approval) == nil
	}
	state := approval.State
	if state == ApprovalPending && !time.Now().Before(snapshot.ExpiresAt) {
		state = ApprovalExpired
	}
	return ApprovalView{RequestID: snapshot.Operation.RequestID, AttemptID: snapshot.AttemptID, SessionID: snapshot.SessionID, Version: approval.Version, State: state, Access: snapshot.Operation.Access, Target: snapshot.Operation.Target, PolicyDigest: snapshot.PolicyDigest, ExpiresAt: snapshot.ExpiresAt, CanDecide: canDecide}
}

func (s *Service) ListApprovals(ctx context.Context, sessionID string) ([]ApprovalView, error) {
	identity, err := s.approvalReader(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.sessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(`SELECT attempt_id,request_id FROM sandbox_approval_requests WHERE org_id=? AND session_id=? ORDER BY expires_at_ms DESC,request_id LIMIT 100`), identity.OrgID, sessionID)
	if err != nil {
		return nil, err
	}
	type key struct{ attempt, request string }
	keys := make([]key, 0)
	for rows.Next() {
		var item key
		if err = rows.Scan(&item.attempt, &item.request); err != nil {
			_ = rows.Close()
			return nil, err
		}
		keys = append(keys, item)
	}
	scanErr := rows.Err()
	if err = rows.Close(); err != nil {
		return nil, err
	}
	if scanErr != nil {
		return nil, scanErr
	}
	result := make([]ApprovalView, 0, len(keys))
	for _, item := range keys {
		approval, readErr := s.ReadApproval(ctx, identity.OrgID, item.attempt, item.request)
		if readErr != nil {
			return nil, readErr
		}
		if s.workspaceAccess(ctx, approval.Snapshot.WorkspaceID) != nil {
			continue
		}
		result = append(result, s.approvalView(ctx, approval))
	}
	return result, nil
}

func (s *Service) approvalReader(ctx context.Context) (authn.Identity, error) {
	if !s.enabled {
		return authn.Identity{}, ErrDisabled
	}
	identity, ok := authn.IdentityFromContext(ctx)
	_, agent := scope.PrincipalFromContext(ctx)
	if !ok || identity.UserID == "" || identity.SessionID == "" || identity.Synthetic || identity.Instance || identity.TokenID != "" || agent || origin.IsTrustedExternalTransport(ctx) || origin.IsTrustedInternalCall(ctx) {
		return authn.Identity{}, ErrAuthority
	}
	return identity, nil
}
