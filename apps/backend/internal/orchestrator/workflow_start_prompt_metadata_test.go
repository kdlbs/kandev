package orchestrator

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestWorkflowAsyncStartFailure_PreservesInputMetadata(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "workflow-metadata-task"
		sessionID   = "workflow-metadata-session"
		stepID      = "workflow-metadata-step"
		executionID = "workflow-metadata-execution"
	)

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateStarting)
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = stepID
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("update task workflow step: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.agentManager = agentMgr
	svc.turnService = &repoTurnService{repo: repo}
	turnID := svc.startTurnForSession(ctx, sessionID)
	if turnID == "" {
		t.Fatal("expected active turn for metadata preservation")
	}

	origin := workflowMessageOrigin{StepID: stepID, StepName: "Async metadata", StepColor: "#abc123"}
	attachments := []v1.MessageAttachment{{
		Type:         "image",
		AttachmentID: "attachment-1",
		MimeType:     "image/png",
		Name:         "diagram.png",
		SizeBytes:    42,
		DeliveryMode: "path",
	}}
	references := []v1.EntityReference{{
		Version:  1,
		Ref:      "JIRA-123",
		Provider: "jira",
		Kind:     "issue",
		ID:       "123",
		Title:    "Preserve launch input",
		URL:      "https://example.invalid/JIRA-123",
		Scope:    "workspace",
	}}
	attempt := newWorkflowStartPromptAttempt(
		taskID, sessionID, origin, "raw workflow prompt", true,
		attachments, references, "handoff once", true,
	)
	serviceCtx := bindWorkflowStartPromptAttemptTurn(
		withWorkflowStartPromptAttempt(ctx, attempt), turnID,
	)
	svc.preserveWorkflowStartPromptAfterFailure(serviceCtx, taskID, sessionID, executionID)

	status := svc.messageQueue.GetStatus(ctx, sessionID)
	if status.Count != 1 {
		t.Fatalf("queue count = %d, want 1", status.Count)
	}
	queued := status.Entries[0]
	if queued.Content != "raw workflow prompt" {
		t.Fatalf("queued content = %q, want raw workflow prompt", queued.Content)
	}
	if !queued.PlanMode {
		t.Fatal("queued plan mode = false, want true")
	}
	if !reflect.DeepEqual(queued.Attachments, toQueuedAttachments(attachments)) {
		t.Fatalf("queued attachments = %#v, want %#v", queued.Attachments, toQueuedAttachments(attachments))
	}
	if got, _ := queued.Metadata["workflow_step_id"].(string); got != stepID {
		t.Fatalf("workflow_step_id = %q, want %q", got, stepID)
	}
	if got, _ := queued.Metadata["workflow_step_name"].(string); got != "Async metadata" {
		t.Fatalf("workflow_step_name = %q, want Async metadata", got)
	}
	if got, _ := queued.Metadata[messagequeue.MetadataStepHandoff].(string); got != "handoff once" {
		t.Fatalf("step handoff = %q, want handoff once", got)
	}
	if recorded, _ := queued.Metadata[metaKeyUserMessageRecorded].(bool); !recorded {
		t.Fatal("user_message_recorded = false, want true")
	}
	if got, ok := queued.Metadata["entity_references"].([]v1.EntityReference); !ok || !reflect.DeepEqual(got, references) {
		t.Fatalf("entity references = %#v, want %#v", queued.Metadata["entity_references"], references)
	}
}
