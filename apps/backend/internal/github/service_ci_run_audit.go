package github

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
)

func (s *Service) auditCIRun(ctx context.Context, request *CIRunRequest, event string, class CIRunFailureClass) error {
	details, _ := json.Marshal(map[string]any{"workspace_id": request.WorkspaceID, "grant_generation": request.GrantGeneration, "actor_task_id": request.ActorTaskID, "actor_session_id": request.ActorSessionID, "target_task_id": request.TargetTaskID, "workflow_id": request.WorkflowID, "workflow_step_id": request.WorkflowStepID, "repository_id": request.RepositoryID, "pr_number": request.PRNumber, "expected_head_sha": request.ExpectedHeadSHA, "source_run_id": request.SourceRunID, "expected_source_attempt": request.ExpectedSourceAttempt, "operation": request.Operation, "provider_run_id": request.ProviderRunID, "provider_workflow_id": request.ProviderWorkflowID, "provider_head_repo": request.ProviderHeadRepo, "provider_head_ref": request.ProviderHeadRef, "provider_head_sha": request.ProviderHeadSHA})
	return s.store.AppendCIRunAuditEvent(ctx, &CIRunAuditEvent{ID: uuid.NewString(), RequestID: request.ID, EventType: event, FailureClass: string(class), DetailsJSON: string(details), CreatedAt: s.ciRunClock()().UTC()})
}
