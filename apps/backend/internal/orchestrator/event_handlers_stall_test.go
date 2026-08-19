package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestHandleAgentStalled_PersistsNeutralRunningNotice(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	before, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session before handling stall: %v", err)
	}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		currentPromptExecutionID: "execution-1",
	}
	agentMgr.currentPromptGeneration.Store(7)
	agentMgr.currentPromptActivityEpoch.Store(1)
	svc := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		newMockTaskRepo(),
		agentMgr,
	)
	svc.turnService = &repoTurnService{repo: repo}
	activeTurn, err := svc.turnService.StartTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("start active turn: %v", err)
	}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.handleAgentStalled(ctx, lifecycle.AgentStalledPayload{
		AgentExecutionID: "execution-1",
		TaskID:           "task-1",
		SessionID:        "session-1",
		PromptGeneration: 7,
		ActivityEpoch:    1,
		ToolName:         "shell",
		ToolTitle:        "Start dev server",
		ToolStatus:       "in_progress",
	})

	if len(messages.sessionMessages) != 1 {
		t.Fatalf("session messages = %d, want 1", len(messages.sessionMessages))
	}
	message := messages.sessionMessages[0]
	if message.messageType != string(v1.MessageTypeStatus) {
		t.Fatalf("message type = %q, want status", message.messageType)
	}
	if !strings.Contains(message.content, "Still waiting on Start dev server") {
		t.Fatalf("notice content = %q, want sanitized tool title", message.content)
	}
	if message.metadata["action_visibility"] != actionVisibilityRunning {
		t.Fatalf("action visibility = %v, want running", message.metadata["action_visibility"])
	}
	if message.turnID != activeTurn.ID {
		t.Fatalf("notice turn ID = %q, want active turn %q", message.turnID, activeTurn.ID)
	}
	if _, hasVariant := message.metadata["variant"]; hasVariant {
		t.Fatalf("notice metadata unexpectedly set a warning/error variant: %#v", message.metadata)
	}
	actions, ok := message.metadata["actions"].([]map[string]interface{})
	if !ok || len(actions) != 1 {
		t.Fatalf("actions = %#v, want one cancel action", message.metadata["actions"])
	}
	action := actions[0]
	if action["label"] != "Cancel turn" || action["test_id"] != "stall-cancel-turn-button" {
		t.Fatalf("cancel action = %#v", action)
	}
	params, ok := action["params"].(map[string]interface{})
	if !ok || params["method"] != "agent.cancel" {
		t.Fatalf("cancel params = %#v, want agent.cancel", action["params"])
	}

	after, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session after handling stall: %v", err)
	}
	if after.State != before.State {
		t.Fatalf("session state changed from %q to %q", before.State, after.State)
	}
}

func TestHandleAgentStalled_NeverStartedFailsSessionAndTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	agentMgr := &mockAgentManager{
		repoForExecutionLookup:   repo,
		currentPromptExecutionID: "execution-1",
	}
	agentMgr.currentPromptGeneration.Store(7)
	agentMgr.currentPromptActivityEpoch.Store(1)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-1", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		taskRepo,
		agentMgr,
	)
	svc.turnService = &repoTurnService{repo: repo}
	activeTurn, err := svc.turnService.StartTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("start active turn: %v", err)
	}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	svc.handleAgentStalled(ctx, lifecycle.AgentStalledPayload{
		AgentExecutionID: "execution-1",
		TaskID:           "task-1",
		SessionID:        "session-1",
		PromptGeneration: 7,
		ActivityEpoch:    1,
		NeverStarted:     true,
	})

	if len(messages.sessionMessages) != 1 {
		t.Fatalf("session messages = %d, want 1", len(messages.sessionMessages))
	}
	message := messages.sessionMessages[0]
	if message.messageType != string(v1.MessageTypeError) {
		t.Fatalf("message type = %q, want error", message.messageType)
	}
	if message.content != neverStartedNoticeContent {
		t.Fatalf("notice content = %q, want the never-started condition named", message.content)
	}
	if message.turnID != activeTurn.ID {
		t.Fatalf("notice turn ID = %q, want active turn %q", message.turnID, activeTurn.ID)
	}
	if _, hasVisibility := message.metadata["action_visibility"]; hasVisibility {
		t.Fatalf("terminal notice has running visibility metadata: %#v", message.metadata)
	}
	if _, hasActions := message.metadata["actions"]; hasActions {
		t.Fatalf("terminal notice has a running cancel action: %#v", message.metadata)
	}

	after, err := repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("get session after handling stall: %v", err)
	}
	if after.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %q, want FAILED", after.State)
	}

	taskRepo.mu.Lock()
	taskState := taskRepo.updatedStates["task-1"]
	taskRepo.mu.Unlock()
	if taskState != v1.TaskStateFailed {
		t.Fatalf("task state = %q, want FAILED", taskState)
	}
}

func TestHandleAgentStalled_RejectsActivityEpochChangedAfterSnapshot(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	agentMgr := &mockAgentManager{currentPromptExecutionID: "execution-1"}
	agentMgr.currentPromptGeneration.Store(7)
	agentMgr.currentPromptActivityEpoch.Store(2)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageCreator = &mockMessageCreator{}

	svc.handleAgentStalled(ctx, lifecycle.AgentStalledPayload{
		AgentExecutionID: "execution-1",
		TaskID:           "task-1",
		SessionID:        "session-1",
		PromptGeneration: 7,
		ActivityEpoch:    1,
		NeverStarted:     true,
	})

	if got := len(svc.messageCreator.(*mockMessageCreator).sessionMessages); got != 0 {
		t.Fatalf("stale activity snapshot created %d messages, want 0", got)
	}
}

func TestHandleAgentStalled_RejectsSettledOrStalePrompt(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	agentMgr := &mockAgentManager{currentPromptExecutionID: "execution-1"}
	agentMgr.currentPromptGeneration.Store(7)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	payload := lifecycle.AgentStalledPayload{
		AgentExecutionID: "execution-1",
		TaskID:           "task-1",
		SessionID:        "session-1",
		PromptGeneration: 7,
	}

	if err := repo.UpdateTaskSessionState(ctx, "session-1", models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("settle session: %v", err)
	}
	svc.handleAgentStalled(ctx, payload)
	if len(messages.sessionMessages) != 0 {
		t.Fatalf("settled session messages = %d, want 0", len(messages.sessionMessages))
	}
	if err := repo.UpdateTaskSessionState(ctx, "session-1", models.TaskSessionStateRunning, ""); err != nil {
		t.Fatalf("resume session: %v", err)
	}
	agentMgr.currentPromptGeneration.Store(8)
	svc.handleAgentStalled(ctx, payload)
	if len(messages.sessionMessages) != 0 {
		t.Fatalf("stale generation messages = %d, want 0", len(messages.sessionMessages))
	}
}

func TestStallNoticeContentFallsBackWithoutTool(t *testing.T) {
	if got := stallNoticeContent(lifecycle.AgentStalledPayload{}); got != "Still waiting for the agent." {
		t.Fatalf("stallNoticeContent() = %q, want generic fallback", got)
	}
}

func TestStallNoticeContentNamesNeverStartedCondition(t *testing.T) {
	payload := lifecycle.AgentStalledPayload{NeverStarted: true, ToolName: "shell", ToolTitle: "Start dev server"}
	if got := stallNoticeContent(payload); got != neverStartedNoticeContent {
		t.Fatalf("stallNoticeContent() = %q, want the never-started message regardless of tool info", got)
	}
}
