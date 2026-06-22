package orchestrator

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestExecuteQueuedMessage_RequeuesWhenResetInProgress(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, promptErr: ErrSessionResetInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "hello",
		QueuedBy:  "test",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count != 1 {
		t.Fatalf("expected queued message to be requeued when reset is in progress, count=%d", status.Count)
	}
	if status.Entries[0].Content != "hello" {
		t.Fatalf("expected queued content to be preserved, got %q", status.Entries[0].Content)
	}
}

// TestExecuteQueuedMessage_SkipsUserMessageWhenAlreadyRecorded pins the
// duplicate-prompt fix: when a queued workflow auto-start carries
// metadata[user_message_recorded]=true (set by autoStartStepPrompt's
// post-recordAutoStartMessage retry branches), executeQueuedMessage must NOT
// call CreateUserMessage. Without this guard, the boot_ready drain produces
// the second identical "Merge"-step user row observed on the ACP-removal task.
func TestExecuteQueuedMessage_SkipsUserMessageWhenAlreadyRecorded(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	mc := &mockMessageCreator{}
	svc.messageCreator = mc

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "merge it",
		QueuedBy:  messagequeue.QueuedByWorkflow,
		Metadata: map[string]interface{}{
			"workflow_step_name":       "Merge",
			metaKeyUserMessageRecorded: true,
		},
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	if len(mc.userMessages) != 0 {
		t.Fatalf("expected 0 user messages (already recorded before queueing), got %d", len(mc.userMessages))
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected the prompt to still reach PromptAgent, captured=%d", len(agentMgr.capturedPrompts))
	}
}

func TestExecuteQueuedMessage_RecordsCIAutomationPromptOnDrain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	mc := &mockMessageCreator{}
	svc.messageCreator = mc

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q1",
		SessionID: "s1",
		TaskID:    "t1",
		Content: ciAutomationChatPrompt(ciAutomationRenderPrompt(
			"Fix the PR",
			&github.TaskPR{Owner: "acme", Repo: "widget", PRNumber: 42},
			ciAutomationCheckpoint{
				FailedChecks: []ciAutomationCheckSnapshot{{Name: "unit", Conclusion: "failure"}},
			},
		)),
		QueuedBy: messagequeue.QueuedByWorkflow,
		Metadata: map[string]interface{}{
			"origin":     ciAutomationOrigin,
			"auto_start": true,
		},
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	if len(mc.userMessages) != 1 {
		t.Fatalf("expected CI automation user message to be recorded on drain, got %d", len(mc.userMessages))
	}
	chatMessage := mc.userMessages[0]
	visible := sysprompt.StripSystemContent(chatMessage.content)
	if !strings.Contains(visible, "@ci-auto-fix") || !strings.Contains(visible, "PR: acme/widget#42") || !strings.Contains(visible, "unit: failure") {
		t.Fatalf("expected visible chat prompt to include @ci-auto-fix and PR snapshot, got %q", visible)
	}
	if strings.Contains(visible, "Fix the PR") {
		t.Fatalf("expected shared CI prompt to stay hidden, got %q", visible)
	}
	if !strings.Contains(chatMessage.content, "<kandev-system>") || !strings.Contains(chatMessage.content, "Fix the PR") || !strings.Contains(chatMessage.content, "unit") {
		t.Fatalf("expected raw chat message to preserve hidden CI prompt, got %q", chatMessage.content)
	}
	if chatMessage.metadata["origin"] != ciAutomationOrigin || chatMessage.metadata["auto_start"] != true {
		t.Fatalf("expected CI automation metadata, got %+v", chatMessage.metadata)
	}
	if len(agentMgr.capturedPrompts) != 1 {
		t.Fatalf("expected the prompt to reach PromptAgent, captured=%d", len(agentMgr.capturedPrompts))
	}
}

func TestExecuteQueuedMessage_StoresAttachmentsInUserMessageMetadata(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{isAgentRunning: true}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	mc := &mockMessageCreator{}
	svc.messageCreator = mc

	queuedAtts := []messagequeue.MessageAttachment{
		{Type: "image", Data: "base64payload", MimeType: "image/png"},
	}
	queuedMsg := &messagequeue.QueuedMessage{
		ID:          "q1",
		SessionID:   "s1",
		TaskID:      "t1",
		Content:     "look at this screenshot",
		Attachments: queuedAtts,
		QueuedBy:    "test",
	}

	svc.executeQueuedMessage("s1", queuedMsg)

	if len(mc.userMessages) != 1 {
		t.Fatalf("expected 1 user message recorded, got %d", len(mc.userMessages))
	}
	meta := mc.userMessages[0].metadata
	if meta == nil {
		t.Fatalf("expected metadata on user message, got nil")
	}
	raw, ok := meta["attachments"]
	if !ok {
		t.Fatalf("expected metadata to contain 'attachments' key, got %v", meta)
	}
	got, ok := raw.([]v1.MessageAttachment)
	if !ok {
		t.Fatalf("expected attachments to be []v1.MessageAttachment, got %T", raw)
	}
	want := []v1.MessageAttachment{
		{Type: "image", Data: "base64payload", MimeType: "image/png"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attachments mismatch\n got: %+v\nwant: %+v", got, want)
	}
}
