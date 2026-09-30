package provideraccess

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"
	"time"
)

// AuditOutcome is a closed vocabulary; provider response text never enters the ledger.
type AuditOutcome string

const (
	AuditGrantCreated       AuditOutcome = "grant_created"
	AuditGrantRevoked       AuditOutcome = "grant_revoked"
	AuditLeaseIssued        AuditOutcome = "lease_issued"
	AuditLeaseReplayed      AuditOutcome = "lease_replayed"
	AuditLeaseDenied        AuditOutcome = "lease_denied"
	AuditTokenIssued        AuditOutcome = "token_issued"
	AuditRevocationPending  AuditOutcome = "revocation_pending"
	AuditRevokedAtProvider  AuditOutcome = "revoked_at_provider"
	AuditExpiryBoundedToken AuditOutcome = "expiry_bounded_residual"
)

// AuditEvent contains only reviewed identifiers, generations and a closed outcome.
type AuditEvent struct {
	ID                   string
	GrantID              string
	LeaseID              string
	PluginInstallationID string
	WorkspaceID          string
	ManagedTaskID        string
	SessionID            string
	TargetDigest         string
	GrantGeneration      int64
	ApprovalRevision     uint64
	ConnectionGeneration string
	Provider             string
	Purpose              string
	Outcome              AuditOutcome
	At                   time.Time
}

type auditRow struct {
	ID                   string `db:"id"`
	GrantID              string `db:"grant_id"`
	LeaseID              string `db:"lease_id"`
	PluginInstallationID string `db:"plugin_installation_id"`
	WorkspaceID          string `db:"workspace_id"`
	ManagedTaskID        string `db:"managed_task_id"`
	SessionID            string `db:"session_id"`
	TargetDigest         string `db:"target_digest"`
	GrantGeneration      int64  `db:"grant_generation"`
	ApprovalRevision     int64  `db:"approval_revision"`
	ConnectionGeneration string `db:"connection_generation"`
	Provider             string `db:"provider"`
	Purpose              string `db:"purpose"`
	Outcome              string `db:"outcome"`
	At                   int64  `db:"at"`
}

// RecordAudit appends a non-secret receipt; callers cannot provide freeform errors.
func (s *Store) RecordAudit(ctx context.Context, event AuditEvent) error {
	if err := validateAudit(event); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO provider_access_audit (
  id, grant_id, lease_id, plugin_installation_id, workspace_id,
  managed_task_id, session_id, target_digest, grant_generation,
  approval_revision, connection_generation, provider, purpose, outcome, at)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		event.ID, event.GrantID, event.LeaseID, event.PluginInstallationID,
		event.WorkspaceID, event.ManagedTaskID, event.SessionID, event.TargetDigest,
		event.GrantGeneration, int64(event.ApprovalRevision), event.ConnectionGeneration,
		event.Provider, event.Purpose, event.Outcome, event.At.Unix())
	return err
}

func validateAudit(event AuditEvent) error {
	for _, value := range []string{event.ID, event.GrantID, event.PluginInstallationID,
		event.WorkspaceID, event.Provider, event.Purpose} {
		if value == "" || value != strings.TrimSpace(value) {
			return errors.New("complete provider access audit identity is required")
		}
	}
	if event.GrantGeneration <= 0 || event.ApprovalRevision > math.MaxInt64 || event.At.IsZero() {
		return errors.New("valid provider access audit generation and time are required")
	}
	switch event.Outcome {
	case AuditGrantCreated, AuditGrantRevoked, AuditLeaseIssued, AuditLeaseReplayed,
		AuditLeaseDenied, AuditTokenIssued, AuditRevocationPending,
		AuditRevokedAtProvider, AuditExpiryBoundedToken:
		return nil
	default:
		return errors.New("unsupported provider access audit outcome")
	}
}

// GetAudit reads a receipt by immutable ID.
func (s *Store) GetAudit(ctx context.Context, id string) (*AuditEvent, error) {
	var row auditRow
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT * FROM provider_access_audit WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &AuditEvent{
		ID: row.ID, GrantID: row.GrantID, LeaseID: row.LeaseID,
		PluginInstallationID: row.PluginInstallationID, WorkspaceID: row.WorkspaceID,
		ManagedTaskID: row.ManagedTaskID, SessionID: row.SessionID,
		TargetDigest: row.TargetDigest, GrantGeneration: row.GrantGeneration,
		ApprovalRevision:     uint64(row.ApprovalRevision),
		ConnectionGeneration: row.ConnectionGeneration, Provider: row.Provider,
		Purpose: row.Purpose, Outcome: AuditOutcome(row.Outcome),
		At: time.Unix(row.At, 0).UTC(),
	}, nil
}
