package github

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
)

func (s *Service) auditCIRun(ctx context.Context, request *CIRunRequest, event string, class CIRunFailureClass) error {
	return s.store.AppendCIRunAuditEvent(ctx, s.newCIRunAuditEvent(request, event, class))
}

func (s *Service) newCIRunAuditEvent(
	request *CIRunRequest, event string, class CIRunFailureClass,
) *CIRunAuditEvent {
	details, _ := json.Marshal(map[string]any{"workspace_id": request.WorkspaceID, "grant_id": request.GrantID, "grant_generation": request.GrantGeneration, "actor_task_id": request.ActorTaskID, "actor_session_id": request.ActorSessionID, "target_task_id": request.TargetTaskID, "workflow_id": request.WorkflowID, "workflow_step_id": request.WorkflowStepID, "repository_id": request.RepositoryID, "canonical_repository": request.CanonicalRepository, "pr_number": request.PRNumber, "expected_head_sha": request.ExpectedHeadSHA, "observed_pr_head_sha": request.ObservedPRHeadSHA, "source_run_id": request.SourceRunID, "expected_source_attempt": request.ExpectedSourceAttempt, "operation": request.Operation, "provider_run_id": request.ProviderRunID, "provider_attempt": request.ProviderAttempt, "provider_workflow_id": request.ProviderWorkflowID, "provider_event": request.ProviderEvent, "provider_head_repo": request.ProviderHeadRepo, "provider_head_ref": request.ProviderHeadRef, "provider_head_sha": request.ProviderHeadSHA, "provider_principal": ciRunProviderPrincipalAudit(request.ProviderPrincipalJSON), "provider_request_id": request.ProviderRequestID, "provider_url": request.ProviderURL, "provider_retry_after": request.ProviderRetryAfter, "evidence_kind": request.EvidenceKind, "evidence_verdict": evidenceVerdict(request), "request_created_at": request.CreatedAt, "request_updated_at": request.UpdatedAt})
	return &CIRunAuditEvent{ID: uuid.NewString(), RequestID: request.ID, EventType: event,
		FailureClass: string(class), DetailsJSON: string(details), CreatedAt: s.ciRunClock()().UTC()}
}
