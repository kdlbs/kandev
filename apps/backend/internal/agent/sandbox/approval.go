package sandbox

import (
	"context"
	"time"
)

const (
	filesystemRead      = "read"
	filesystemWrite     = "write"
	ApprovalPending     = "pending"
	ApprovalApproved    = "approved"
	ApprovalDenied      = "denied"
	ApprovalExpired     = "expired"
	ApprovalResponding  = "responding"
	ApprovalSent        = "sent"
	ApprovalUndelivered = "undelivered"
)

type ApprovalOperation struct {
	RequestID         string `json:"request_id"`
	ProviderSessionID string `json:"provider_session_id"`
	ChildPID          int    `json:"child_pid"`
	Access            string `json:"access"`
	Target            string `json:"target"`
}

type ApprovalSnapshot struct {
	RevisionID        string            `json:"revision_id"`
	CeilingRevisionID string            `json:"ceiling_revision_id"`
	OrgID             string            `json:"org_id"`
	WorkspaceID       string            `json:"workspace_id"`
	SessionID         string            `json:"session_id"`
	AttemptID         string            `json:"attempt_id"`
	Generation        uint64            `json:"generation"`
	PolicyDigest      string            `json:"policy_digest"`
	Owner             AttemptOwner      `json:"owner"`
	Operation         ApprovalOperation `json:"operation"`
	SourceDigest      string            `json:"source_digest"`
	CreatedAt         time.Time         `json:"created_at"`
	ExpiresAt         time.Time         `json:"expires_at"`
}

// Approval is private persistence data, not the operator-facing projection.
type Approval struct {
	Version    int64            `json:"version"`
	State      string           `json:"state"`
	OperatorID string           `json:"operator_id"`
	Snapshot   ApprovalSnapshot `json:"snapshot"`
}

// SetApprovalGuard binds the live callback and provider ownership check at startup.
func (s *Service) SetApprovalGuard(guard func(context.Context, *Attempt, *Approval) error) {
	s.approvalGuard = guard
}
