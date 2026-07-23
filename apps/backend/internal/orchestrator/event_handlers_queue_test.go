package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func queuedReferenceFixture() v1.EntityReference {
	return v1.EntityReference{
		Version:  v1.EntityReferenceVersion,
		Ref:      "mention:v1:kandev:task:workspace-1:task-2",
		Provider: "kandev",
		Kind:     "task",
		ID:       "task-2",
		Key:      "TASK-2",
		Title:    "Referenced task",
		URL:      "/t/task-2",
		Scope:    "workspace-1",
	}
}

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

func TestExecuteQueuedMessage_RequeuesCancelReleaseFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{
		isAgentRunning: true,
		promptErr:      fmt.Errorf("failed to trigger prompt: prompt abandoned after cancel: %w", lifecycle.ErrCancelEscalated),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	queuedMsg := &messagequeue.QueuedMessage{
		ID:        "q-cancel",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "hello after cancel",
		QueuedBy:  "test",
		Metadata: map[string]interface{}{
			messagequeue.MetadataEntityReferences: []v1.EntityReference{queuedReferenceFixture()},
		},
	}

	svc.markQueuedDispatchInFlight("s1", queuedMsg.ID)
	svc.executeQueuedMessage("s1", queuedMsg)

	status := svc.messageQueue.GetStatus(ctx, "s1")
	if status.Count != 1 {
		t.Fatalf("expected queued message to be requeued after cancel-release failure, count=%d", status.Count)
	}
	if status.Entries[0].Content != "hello after cancel" {
		t.Fatalf("expected queued content to be preserved, got %q", status.Entries[0].Content)
	}
	if !reflect.DeepEqual(status.Entries[0].Metadata, queuedMsg.Metadata) {
		t.Fatalf("expected queued metadata to be preserved, got %#v", status.Entries[0].Metadata)
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

	svc.markQueuedDispatchInFlight("s1", queuedMsg.ID)
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
			"Fix the PR\n\n{{pr.feedback}}",
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

	svc.markQueuedDispatchInFlight("s1", queuedMsg.ID)
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

func TestExecuteQueuedMessage_NormalizesReferencesForMessageAndPrompt(t *testing.T) {
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

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	reference := queuedReferenceFixture()
	encoded, err := json.Marshal([]v1.EntityReference{reference})
	if err != nil {
		t.Fatalf("marshal reference: %v", err)
	}
	var persisted interface{}
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatalf("round-trip reference: %v", err)
	}
	queued := &messagequeue.QueuedMessage{
		ID:        "q-ref",
		SessionID: "s1",
		TaskID:    "t1",
		Content:   "inspect referenced work",
		QueuedBy:  messagequeue.QueuedByUser,
		Metadata: map[string]interface{}{
			"entity_references": persisted,
			"origin":            "quick-chat",
		},
	}

	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	if len(messages.userMessages) != 1 {
		t.Fatalf("expected one persisted user message, got %d", len(messages.userMessages))
	}
	stored := messages.userMessages[0]
	if !strings.Contains(stored.content, "inspect referenced work") || !strings.Contains(stored.content, "Validated work-item reference snapshots") {
		t.Fatalf("stored content missing validated reference context: %q", stored.content)
	}
	gotReferences, ok := stored.metadata["entity_references"].([]v1.EntityReference)
	if !ok || !reflect.DeepEqual(gotReferences, []v1.EntityReference{reference}) {
		t.Fatalf("stored references = %#v, want typed normalized reference", stored.metadata["entity_references"])
	}
	if stored.metadata["origin"] != "quick-chat" {
		t.Fatalf("unrelated queue metadata lost: %+v", stored.metadata)
	}
	if len(agentMgr.capturedPrompts) != 1 || strings.Count(agentMgr.capturedPrompts[0], "Validated work-item reference snapshots") != 1 {
		t.Fatalf("agent prompt missing one reference block: %+v", agentMgr.capturedPrompts)
	}
	if queued.Content != "inspect referenced work" {
		t.Fatalf("queue retry content mutated: %q", queued.Content)
	}
	if !reflect.DeepEqual(queued.Metadata[messagequeue.MetadataEntityReferences], persisted) {
		t.Fatalf("queue retry metadata mutated: %#v", queued.Metadata)
	}
}

func TestAutoStartStepPrompt_PreservesHandoffReferences(t *testing.T) {
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

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	reference := queuedReferenceFixture()
	handoff, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, "s1", "t1", "handoff details", "", messagequeue.QueuedByUser, false, nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference}},
	)
	if err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	step := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Review"}

	if err := svc.autoStartStepPrompt(ctx, "t1", session, step, "workflow start", false, false); err != nil {
		t.Fatalf("auto-start: %v", err)
	}

	if len(messages.userMessages) != 1 {
		t.Fatalf("expected one workflow user message, got %d", len(messages.userMessages))
	}
	stored := messages.userMessages[0]
	if strings.Count(stored.content, "Validated work-item reference snapshots") != 1 {
		t.Fatalf("stored workflow message missing one reference block: %q", stored.content)
	}
	if !strings.Contains(stored.content, "workflow start") || !strings.Contains(stored.content, "handoff details") {
		t.Fatalf("stored workflow message missing merged visible content: %q", stored.content)
	}
	gotReferences, ok := stored.metadata[messagequeue.MetadataEntityReferences].([]v1.EntityReference)
	if !ok || !reflect.DeepEqual(gotReferences, []v1.EntityReference{reference}) {
		t.Fatalf("workflow message references = %#v", stored.metadata[messagequeue.MetadataEntityReferences])
	}
	if len(agentMgr.capturedPrompts) != 1 || strings.Count(agentMgr.capturedPrompts[0], "Validated work-item reference snapshots") != 1 {
		t.Fatalf("workflow agent prompt missing one reference block: %+v", agentMgr.capturedPrompts)
	}
	if handoff.Content != "handoff details" {
		t.Fatalf("original queued payload mutated: %q", handoff.Content)
	}
}

func TestAutoStartStepPrompt_QueuesRawHandoffWithReferencesWhenBusy(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("failed to get session: %v", err)
	}
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to update session: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	reference := queuedReferenceFixture()
	_, err = svc.messageQueue.QueueMessageWithMetadata(
		ctx, "s1", "t1", "handoff details", "", messagequeue.QueuedByUser, false, nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference}},
	)
	if err != nil {
		t.Fatalf("queue handoff: %v", err)
	}

	step := &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Name: "Review"}
	if err := svc.autoStartStepPrompt(ctx, "t1", session, step, "workflow start", false, true); err != nil {
		t.Fatalf("auto-start: %v", err)
	}

	entries := svc.messageQueue.GetStatus(ctx, "s1").Entries
	if len(entries) != 1 {
		t.Fatalf("expected one merged workflow queue entry, got %d", len(entries))
	}
	entry := entries[0]
	if !strings.Contains(entry.Content, "workflow start") || !strings.Contains(entry.Content, "handoff details") {
		t.Fatalf("queued content missing merged prompt: %q", entry.Content)
	}
	if strings.Contains(entry.Content, "Validated work-item reference snapshots") {
		t.Fatalf("queue must retain raw retry content, got %q", entry.Content)
	}
	gotReferences, ok := entry.Metadata[messagequeue.MetadataEntityReferences].([]v1.EntityReference)
	if !ok || !reflect.DeepEqual(gotReferences, []v1.EntityReference{reference}) {
		t.Fatalf("queued workflow references = %#v", entry.Metadata[messagequeue.MetadataEntityReferences])
	}
}
