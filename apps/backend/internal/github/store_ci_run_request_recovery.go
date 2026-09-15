package github

import (
	"context"
	"errors"
)

func allCIRunStringsPresent(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return false
		}
	}
	return true
}

func (s *Store) ReleaseCIRunExecutionForRetryablePreflight(ctx context.Context, request *CIRunRequest, event *CIRunAuditEvent) error {
	if request == nil || request.ID == "" || request.ExecutionOwner == "" || event == nil || event.RequestID != request.ID {
		return errors.New("complete retryable preflight release identity is required")
	}
	if err := validateCIRunAuditDetails(event.DetailsJSON); err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE github_ci_run_requests SET failure_class = ?, execution_owner = '', execution_lease_expires_at = NULL, updated_at = ? WHERE id = ? AND status = ? AND execution_owner = ? AND provider_call_started_at IS NULL`), request.FailureClass, request.UpdatedAt, request.ID, CIRunRequestPending, request.ExecutionOwner)
	if err != nil {
		return err
	}
	if err := requireOneAffectedRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO github_ci_run_audit_events (id, request_id, event_type, failure_class, details_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`), event.ID, event.RequestID, event.EventType, event.FailureClass, event.DetailsJSON, event.CreatedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	request.ExecutionOwner, request.ExecutionLeaseExpires = "", nil
	return nil
}

func (s *Store) RecordCIRunReconciliationReadFailure(ctx context.Context, request *CIRunRequest, event *CIRunAuditEvent) error {
	if request == nil || request.ID == "" || request.ProviderCallStartedAt == nil || request.Operation == "" || event == nil || event.RequestID != request.ID {
		return errors.New("complete reconciliation read failure identity is required")
	}
	if err := validateCIRunAuditDetails(event.DetailsJSON); err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE github_ci_run_requests SET failure_class = ?, provider_request_id = ?, provider_url = ?, updated_at = ? WHERE id = ? AND status = ? AND operation = ? AND provider_call_started_at IS NOT NULL`), request.FailureClass, request.ProviderRequestID, request.ProviderURL, request.UpdatedAt, request.ID, CIRunRequestReconciling, request.Operation)
	if err != nil {
		return err
	}
	if err := requireOneAffectedRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO github_ci_run_audit_events (id, request_id, event_type, failure_class, details_json, created_at) VALUES (?, ?, ?, ?, ?, ?)`), event.ID, event.RequestID, event.EventType, event.FailureClass, event.DetailsJSON, event.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}
