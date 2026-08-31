package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const ciRunRequestColumns = `
	id, grant_id, workspace_id, actor_task_id, actor_session_id, target_task_id,
	workflow_id, workflow_step_id, repository_id, pr_number, expected_head_sha,
	source_run_id, expected_source_attempt, evidence_kind, idempotency_hash,
	status, execution_owner, execution_lease_expires_at, provider_retry_after,
	operation, provider_call_started_at, provider_call_revision, provider_run_watermark, provider_run_id,
	provider_workflow_id, provider_workflow_name, provider_workflow_path,
	provider_attempt, provider_head_repo, provider_head_ref,
	provider_head_sha, failure_class, created_at, updated_at`

func (s *Store) UpsertCIRunGrant(ctx context.Context, grant *CIRunGrant) error {
	if grant == nil || grant.ID == "" || grant.WorkspaceID == "" || grant.ActorTaskID == "" ||
		grant.TargetTaskID == "" || grant.WorkflowID == "" || grant.WorkflowStepID == "" ||
		grant.RepositoryID == "" || grant.CreatedByUserID == "" {
		return errors.New("complete CI run grant identity is required")
	}
	now := time.Now().UTC()
	if grant.CreatedAt.IsZero() {
		grant.CreatedAt = now
	}
	if grant.UpdatedAt.IsZero() {
		grant.UpdatedAt = now
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_ci_run_grants (
			id, workspace_id, actor_task_id, target_task_id, workflow_id,
			workflow_step_id, repository_id, created_by_user_id, revoked_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		grant.ID, grant.WorkspaceID, grant.ActorTaskID, grant.TargetTaskID, grant.WorkflowID,
		grant.WorkflowStepID, grant.RepositoryID, grant.CreatedByUserID, grant.RevokedAt,
		grant.CreatedAt, grant.UpdatedAt)
	return err
}

func (s *Store) GetActiveCIRunGrant(
	ctx context.Context,
	workspaceID, actorTaskID, targetTaskID, workflowID, workflowStepID, repositoryID string,
) (*CIRunGrant, error) {
	var grant CIRunGrant
	err := s.ro.GetContext(ctx, &grant, s.ro.Rebind(`
		SELECT id, workspace_id, actor_task_id, target_task_id, workflow_id,
			workflow_step_id, repository_id, created_by_user_id, revoked_at, created_at, updated_at
		FROM github_ci_run_grants
		WHERE workspace_id = ? AND actor_task_id = ? AND target_task_id = ?
			AND workflow_id = ? AND workflow_step_id = ? AND repository_id = ?
			AND revoked_at IS NULL
		ORDER BY updated_at DESC LIMIT 1`),
		workspaceID, actorTaskID, targetTaskID, workflowID, workflowStepID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &grant, err
}

func (s *Store) GetAuthorizedCIRunGrant(
	ctx context.Context,
	actorTaskID, actorSessionID, targetTaskID, repositoryID string,
) (*CIRunGrant, error) {
	var grant CIRunGrant
	err := s.ro.GetContext(ctx, &grant, s.ro.Rebind(`
		SELECT grant.id, grant.workspace_id, grant.actor_task_id, grant.target_task_id,
			grant.workflow_id, grant.workflow_step_id, grant.repository_id,
			grant.created_by_user_id, grant.revoked_at, grant.created_at, grant.updated_at
		FROM github_ci_run_grants grant
		JOIN tasks actor ON actor.id = grant.actor_task_id
			AND actor.workspace_id = grant.workspace_id
		JOIN task_sessions session ON session.task_id = actor.id
		WHERE grant.actor_task_id = ? AND session.id = ?
			AND grant.target_task_id = ? AND grant.repository_id = ?
			AND grant.revoked_at IS NULL
		ORDER BY grant.updated_at DESC LIMIT 1`),
		actorTaskID, actorSessionID, targetTaskID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &grant, err
}

func (s *Store) RevokeCIRunGrant(ctx context.Context, workspaceID, id string, at time.Time) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE github_ci_run_grants
		SET revoked_at = ?, updated_at = ? WHERE id = ? AND workspace_id = ? AND revoked_at IS NULL`),
		at.UTC(), at.UTC(), id, workspaceID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ClaimCIRunRequest(ctx context.Context, request *CIRunRequest) (*CIRunRequest, bool, error) {
	if err := validateCIRunRequest(request); err != nil {
		return nil, false, err
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO github_ci_run_requests (`+ciRunRequestColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING`), ciRunRequestArgs(request)...)
	if err != nil {
		return nil, false, err
	}
	created := false
	if affected, rowsErr := result.RowsAffected(); rowsErr == nil {
		created = affected == 1
	}
	loaded, err := s.getCIRunRequestByCallerKey(ctx, request)
	if err == nil {
		if !sameCIRunSemanticIdentity(loaded, request) {
			return loaded, false, ErrCIRunIdempotencyConflict
		}
		return loaded, created, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	semantic, semanticErr := s.getCIRunRequestBySemanticKey(ctx, request)
	if semanticErr == nil && semantic != nil {
		return semantic, false, ErrCIRunSemanticConflict
	}
	if semanticErr != nil && !errors.Is(semanticErr, sql.ErrNoRows) {
		return nil, false, semanticErr
	}
	return nil, false, errors.New("CI run request claim was not persisted")
}

func sameCIRunSemanticIdentity(left, right *CIRunRequest) bool {
	return left != nil && right != nil && left.WorkspaceID == right.WorkspaceID &&
		left.ActorTaskID == right.ActorTaskID && left.TargetTaskID == right.TargetTaskID &&
		left.WorkflowID == right.WorkflowID && left.WorkflowStepID == right.WorkflowStepID &&
		left.RepositoryID == right.RepositoryID && left.PRNumber == right.PRNumber &&
		strings.EqualFold(left.ExpectedHeadSHA, right.ExpectedHeadSHA) &&
		left.SourceRunID == right.SourceRunID &&
		left.ExpectedSourceAttempt == right.ExpectedSourceAttempt &&
		left.EvidenceKind == right.EvidenceKind
}

func validateCIRunRequest(request *CIRunRequest) error {
	if request == nil {
		return errors.New("complete CI run request identity is required")
	}
	if !allCIRunStringsPresent(request.ID, request.GrantID, request.WorkspaceID,
		request.ActorTaskID, request.ActorSessionID, request.TargetTaskID, request.WorkflowID,
		request.WorkflowStepID, request.RepositoryID) {
		return errors.New("complete CI run request identity is required")
	}
	if request.PRNumber <= 0 || len(request.ExpectedHeadSHA) != 40 ||
		request.SourceRunID <= 0 || request.ExpectedSourceAttempt <= 0 ||
		len(request.IdempotencyHash) != 64 {
		return errors.New("complete CI run request identity is required")
	}
	if request.EvidenceKind != CIRunEvidencePRHead && request.EvidenceKind != CIRunEvidenceCurrentMerge {
		return errors.New("invalid CI run evidence kind")
	}
	return nil
}

func allCIRunStringsPresent(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return false
		}
	}
	return true
}

func ciRunRequestArgs(r *CIRunRequest) []any {
	return []any{
		r.ID, r.GrantID, r.WorkspaceID, r.ActorTaskID, r.ActorSessionID, r.TargetTaskID,
		r.WorkflowID, r.WorkflowStepID, r.RepositoryID, r.PRNumber, r.ExpectedHeadSHA,
		r.SourceRunID, r.ExpectedSourceAttempt, r.EvidenceKind, r.IdempotencyHash,
		r.Status, r.ExecutionOwner, r.ExecutionLeaseExpires, r.ProviderRetryAfter,
		r.Operation, r.ProviderCallStartedAt, r.ProviderCallRevision, r.ProviderRunWatermark,
		r.ProviderRunID, r.ProviderWorkflowID,
		r.ProviderWorkflowName, r.ProviderWorkflowPath, r.ProviderAttempt,
		r.ProviderHeadRepo, r.ProviderHeadRef, r.ProviderHeadSHA,
		r.FailureClass, r.CreatedAt, r.UpdatedAt,
	}
}

func (s *Store) AcquireCIRunExecution(
	ctx context.Context,
	request *CIRunRequest,
	owner string,
	at time.Time,
	leaseDuration time.Duration,
) (bool, error) {
	if request == nil || request.ID == "" || owner == "" || leaseDuration <= 0 {
		return false, errors.New("complete CI run execution lease is required")
	}
	at = at.UTC()
	expiresAt := at.Add(leaseDuration)
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE github_ci_run_requests
		SET execution_owner = ?, execution_lease_expires_at = ?, updated_at = ?
		WHERE id = ? AND status = ? AND provider_call_started_at IS NULL
			AND (execution_owner = '' OR execution_lease_expires_at IS NULL
				OR execution_lease_expires_at <= ?)`),
		owner, expiresAt, at, request.ID, CIRunRequestPending, at)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected != 1 {
		return false, nil
	}
	request.ExecutionOwner = owner
	request.ExecutionLeaseExpires = &expiresAt
	request.UpdatedAt = at
	return true, nil
}

func (s *Store) getCIRunRequestByCallerKey(ctx context.Context, r *CIRunRequest) (*CIRunRequest, error) {
	var loaded CIRunRequest
	err := s.ro.GetContext(ctx, &loaded, s.ro.Rebind(`SELECT `+ciRunRequestColumns+`
		FROM github_ci_run_requests WHERE grant_id = ? AND actor_task_id = ? AND idempotency_hash = ?`),
		r.GrantID, r.ActorTaskID, r.IdempotencyHash)
	return &loaded, err
}

func (s *Store) getCIRunRequestBySemanticKey(ctx context.Context, r *CIRunRequest) (*CIRunRequest, error) {
	var loaded CIRunRequest
	err := s.ro.GetContext(ctx, &loaded, s.ro.Rebind(`SELECT `+ciRunRequestColumns+`
		FROM github_ci_run_requests
		WHERE target_task_id = ? AND repository_id = ? AND pr_number = ?
			AND source_run_id = ? AND expected_source_attempt = ? AND evidence_kind = ?`),
		r.TargetTaskID, r.RepositoryID, r.PRNumber, r.SourceRunID,
		r.ExpectedSourceAttempt, r.EvidenceKind)
	return &loaded, err
}

func (s *Store) GetCIRunRequest(ctx context.Context, id string) (*CIRunRequest, error) {
	var request CIRunRequest
	err := s.ro.GetContext(ctx, &request, s.ro.Rebind(`SELECT `+ciRunRequestColumns+`
		FROM github_ci_run_requests WHERE id = ?`), id)
	return &request, err
}

func (s *Store) MarkCIRunProviderCallStarted(
	ctx context.Context, request *CIRunRequest, at time.Time,
) error {
	if request == nil || request.ID == "" || request.ExecutionOwner == "" ||
		request.Operation == "" || request.ProviderWorkflowID <= 0 {
		return errors.New("complete provider call identity is required")
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE github_ci_run_requests
		SET provider_call_started_at = ?,
			provider_call_revision = provider_call_revision + 1,
			status = ?, operation = ?, failure_class = '', provider_retry_after = NULL,
			provider_workflow_id = ?,
			provider_workflow_name = ?, provider_workflow_path = ?, provider_head_repo = ?,
			provider_head_ref = ?, provider_head_sha = ?, provider_run_watermark = ?, updated_at = ?
		WHERE id = ? AND execution_owner = ? AND status = ?
			AND provider_call_started_at IS NULL AND provider_call_revision = ?`),
		at.UTC(), CIRunRequestReconciling, request.Operation, request.ProviderWorkflowID,
		request.ProviderWorkflowName, request.ProviderWorkflowPath, request.ProviderHeadRepo,
		request.ProviderHeadRef, request.ProviderHeadSHA, request.ProviderRunWatermark,
		at.UTC(), request.ID, request.ExecutionOwner, CIRunRequestPending,
		request.ProviderCallRevision)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	startedAt := at.UTC()
	request.ProviderCallStartedAt = &startedAt
	request.ProviderCallRevision++
	request.Status = CIRunRequestReconciling
	request.UpdatedAt = startedAt
	return nil
}

func (s *Store) PrepareCIRunDispatchFallback(
	ctx context.Context, request *CIRunRequest, at time.Time,
) error {
	if request == nil || request.ID == "" || request.ExecutionOwner == "" ||
		request.Operation != CIRunOperationRerunFailedJobs || request.ProviderCallStartedAt == nil {
		return errors.New("complete rerun fallback identity is required")
	}
	at = at.UTC()
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE github_ci_run_requests
		SET status = ?, operation = ?, provider_call_started_at = NULL,
			provider_run_watermark = 0, failure_class = '', execution_owner = '',
			execution_lease_expires_at = NULL, provider_retry_after = NULL, updated_at = ?
		WHERE id = ? AND execution_owner = ? AND status = ? AND operation = ?
			AND provider_call_started_at IS NOT NULL AND provider_call_revision = ?`),
		CIRunRequestPending, CIRunOperationWorkflowDispatch, at, request.ID,
		request.ExecutionOwner, CIRunRequestReconciling, CIRunOperationRerunFailedJobs,
		request.ProviderCallRevision)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	request.Status = CIRunRequestPending
	request.Operation = CIRunOperationWorkflowDispatch
	request.ProviderCallStartedAt = nil
	request.ProviderRunWatermark = 0
	request.FailureClass = ""
	request.ExecutionOwner = ""
	request.ExecutionLeaseExpires = nil
	request.ProviderRetryAfter = nil
	request.UpdatedAt = at
	return nil
}

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
			provider_retry_after = ?, updated_at = ?
		WHERE id = ? AND execution_owner = ?`),
		CIRunRequestPending, CIRunFailureProviderRateLimited, retryAfter, at,
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

func (s *Store) CompleteCIRunRequest(ctx context.Context, request *CIRunRequest) error {
	if request == nil || request.ID == "" {
		return errors.New("CI run request is required")
	}
	query := `UPDATE github_ci_run_requests SET
		status = ?, operation = ?, provider_run_id = ?, provider_workflow_id = ?,
		provider_workflow_name = ?, provider_workflow_path = ?,
		provider_attempt = ?, provider_head_repo = ?, provider_head_ref = ?,
		provider_head_sha = ?, failure_class = ?, execution_owner = '',
		execution_lease_expires_at = NULL, provider_retry_after = NULL,
		updated_at = ? WHERE id = ?`
	args := []any{
		request.Status, request.Operation, request.ProviderRunID, request.ProviderWorkflowID,
		request.ProviderWorkflowName, request.ProviderWorkflowPath, request.ProviderAttempt,
		request.ProviderHeadRepo, request.ProviderHeadRef,
		request.ProviderHeadSHA, request.FailureClass, request.UpdatedAt, request.ID,
	}
	if request.ProviderCallStartedAt == nil {
		if request.ExecutionOwner == "" {
			return errors.New("pre-provider completion owner is required")
		}
		query += ` AND status = ? AND execution_owner = ? AND provider_call_started_at IS NULL`
		args = append(args, CIRunRequestPending, request.ExecutionOwner)
	} else {
		query += ` AND status = ? AND operation = ? AND provider_call_revision = ?`
		args = append(args, CIRunRequestReconciling, request.Operation, request.ProviderCallRevision)
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AppendCIRunAuditEvent(ctx context.Context, event *CIRunAuditEvent) error {
	if event == nil || event.ID == "" || event.RequestID == "" || event.EventType == "" {
		return errors.New("complete CI run audit identity is required")
	}
	if err := validateCIRunAuditDetails(event.DetailsJSON); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO github_ci_run_audit_events
		(id, request_id, event_type, failure_class, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`), event.ID, event.RequestID, event.EventType,
		event.FailureClass, event.DetailsJSON, event.CreatedAt)
	return err
}

func validateCIRunAuditDetails(raw string) error {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return fmt.Errorf("invalid CI run audit details: %w", err)
	}
	if hasSecretAuditKey(value) {
		return errors.New("CI run audit details contain a forbidden secret field")
	}
	return nil
}

func hasSecretAuditKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "token") || strings.Contains(lower, "authorization") ||
				strings.Contains(lower, "private_key") || strings.Contains(lower, "credential") {
				return true
			}
			if hasSecretAuditKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasSecretAuditKey(child) {
				return true
			}
		}
	}
	return false
}
