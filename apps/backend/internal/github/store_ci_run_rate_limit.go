package github

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) DeferCIRunForRateLimit(
	ctx context.Context,
	request *CIRunRequest,
	retryAfter time.Time,
	at time.Time,
) error {
	if request == nil || request.ID == "" || request.ExecutionOwner == "" || retryAfter.IsZero() {
		return errors.New("complete rate-limited CI run request is required")
	}
	retryAfter = retryAfter.UTC()
	at = at.UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE github_ci_run_requests
		SET status = ?, provider_call_started_at = NULL, failure_class = ?,
			execution_owner = '', execution_lease_expires_at = NULL,
			provider_retry_after = ?, provider_request_id = ?, provider_url = ?, updated_at = ?
		WHERE id = ? AND execution_owner = ?`),
		CIRunRequestPending, CIRunFailureProviderRateLimited, retryAfter,
		request.ProviderRequestID, request.ProviderURL, at,
		request.ID, request.ExecutionOwner)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	request.Status = CIRunRequestPending
	request.ProviderCallStartedAt = nil
	request.FailureClass = string(CIRunFailureProviderRateLimited)
	request.ExecutionOwner = ""
	request.ExecutionLeaseExpires = nil
	request.ProviderRetryAfter = &retryAfter
	request.UpdatedAt = at
	return nil
}

// DeferCIRunForRateLimitWithAudit persists a pre-provider retry deferral and
// its audit event in one transaction.
func (s *Store) DeferCIRunForRateLimitWithAudit(
	ctx context.Context, request *CIRunRequest, retryAfter, at time.Time, event *CIRunAuditEvent,
) error {
	if event == nil || request == nil || event.RequestID != request.ID {
		return errors.New("rate-limit audit identity is required")
	}
	if err := validateCIRunAuditDetails(event.DetailsJSON); err != nil {
		return err
	}
	if request.ID == "" || request.ExecutionOwner == "" || retryAfter.IsZero() {
		return errors.New("complete rate-limited CI run request is required")
	}
	retryAfter, at = retryAfter.UTC(), at.UTC()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE github_ci_run_requests
		SET status = ?, provider_call_started_at = NULL, failure_class = ?,
			execution_owner = '', execution_lease_expires_at = NULL, provider_retry_after = ?,
			provider_request_id = ?, provider_url = ?, updated_at = ?
		WHERE id = ? AND execution_owner = ?`), CIRunRequestPending,
		CIRunFailureProviderRateLimited, retryAfter, request.ProviderRequestID,
		request.ProviderURL, at, request.ID, request.ExecutionOwner)
	if err != nil {
		return err
	}
	if err := requireOneAffectedRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO github_ci_run_audit_events
		(id, request_id, event_type, failure_class, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`), event.ID, event.RequestID, event.EventType,
		event.FailureClass, event.DetailsJSON, event.CreatedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	request.Status = CIRunRequestPending
	request.ProviderCallStartedAt = nil
	request.FailureClass = string(CIRunFailureProviderRateLimited)
	request.ExecutionOwner = ""
	request.ExecutionLeaseExpires = nil
	request.ProviderRetryAfter = &retryAfter
	request.UpdatedAt = at
	return nil
}
