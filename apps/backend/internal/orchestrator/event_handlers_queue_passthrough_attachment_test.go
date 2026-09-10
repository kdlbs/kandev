package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestHandleAgentReady_PassthroughAttachmentOnlyOrdinaryMessageUsesAttachmentAwarePrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-attachment-only", "session-attachment-only", "step1")
	session, err := repo.GetTaskSession(ctx, "session-attachment-only")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.WorkspacePath = t.TempDir()
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session workspace: %v", err)
	}
	seedExecutorRunning(t, repo, "session-attachment-only", "task-attachment-only", "execution-attachment-only")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-attachment-only", v1.TaskStateReview)
	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "workflow1"}
	svc := createTestServiceWithAgent(repo, steps, taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.messageQueue.QueueMessage(
		ctx,
		"session-attachment-only",
		"task-attachment-only",
		"",
		"",
		messagequeue.QueuedByUser,
		false,
		[]messagequeue.MessageAttachment{{Type: "image", Data: "AQ==", MimeType: "image/png"}},
	)
	if err != nil {
		t.Fatalf("queue attachment-only message: %v", err)
	}

	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID:    "task-attachment-only",
		SessionID: "session-attachment-only",
	})

	if len(agentMgr.passthroughStdinCalls) != 1 {
		t.Fatalf("passthrough writes = %+v, want one attachment-aware write", agentMgr.passthroughStdinCalls)
	}
	if !strings.Contains(agentMgr.passthroughStdinCalls[0].Data, ".kandev/attachments") {
		t.Fatalf("passthrough write omitted attachment path: %q", agentMgr.passthroughStdinCalls[0].Data)
	}
}

func TestHandleAgentReady_PassthroughQueuedAttachmentUsesAttachmentAwarePrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-attachment", "session-attachment", "step1")
	session, err := repo.GetTaskSession(ctx, "session-attachment")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.WorkspacePath = t.TempDir()
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session workspace: %v", err)
	}
	seedExecutorRunning(t, repo, "session-attachment", "task-attachment", "execution-attachment")
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-attachment", v1.TaskStateReview)
	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "workflow1"}
	svc := createTestServiceWithAgent(repo, steps, taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	_, err = svc.messageQueue.QueueMessage(
		ctx,
		"session-attachment",
		"task-attachment",
		"read attached file",
		"",
		messagequeue.QueuedByUser,
		false,
		[]messagequeue.MessageAttachment{{Type: "resource", Data: "AQ==", MimeType: "text/plain", Name: "note.txt"}},
	)
	if err != nil {
		t.Fatalf("queue message with attachment: %v", err)
	}

	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID:    "task-attachment",
		SessionID: "session-attachment",
	})

	if len(agentMgr.passthroughStdinCalls) != 1 {
		t.Fatalf("passthrough writes = %+v, want one attachment-aware write", agentMgr.passthroughStdinCalls)
	}
	if !strings.Contains(agentMgr.passthroughStdinCalls[0].Data, ".kandev/attachments") {
		t.Fatalf("passthrough write omitted attachment path: %q", agentMgr.passthroughStdinCalls[0].Data)
	}
}
