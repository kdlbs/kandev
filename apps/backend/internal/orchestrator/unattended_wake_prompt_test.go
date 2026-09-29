package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

type wakePromptFixture struct {
	svc      *Service
	agent    *mockAgentManager
	messages *mockMessageCreator
}

func newWakePromptFixture(t *testing.T, state models.TaskSessionState) *wakePromptFixture {
	t.Helper()
	repo := setupTestRepo(t)
	agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	svc.executor = executor.NewExecutor(agent, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoBackedTurnService{repo: repo}
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	seedTaskAndSession(t, repo, "task1", "session1", state)
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatal(err)
	}
	session.AgentExecutionID = "exec-1"
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-1")
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	return &wakePromptFixture{svc: svc, agent: agent, messages: messages}
}

func TestPromptUnattendedWake_StoresMessageOnReservedTurnAndBindsAcceptance(t *testing.T) {
	f := newWakePromptFixture(t, models.TaskSessionStateWaitingForInput)
	var events []string
	var reserved, accepted string
	msgID, err := f.svc.PromptUnattendedWake(context.Background(), UnattendedWakePrompt{
		TaskID: "task1", SessionID: "session1", Content: "Unattended turn.", WakeTurnID: "wake-turn-1",
		OnReserved: func(id string) { events = append(events, "reserved"); reserved = id },
		OnAccepted: func(id string) { events = append(events, "accepted"); accepted = id },
	})
	if err != nil {
		t.Fatal(err)
	}
	if msgID == "" {
		t.Fatal("message id must be returned on success")
	}
	if len(f.messages.userMessages) != 1 {
		t.Fatalf("stored messages = %d, want 1", len(f.messages.userMessages))
	}
	msg := f.messages.userMessages[0]
	if msg.content != "Unattended turn." || msg.sessionID != "session1" || msg.taskID != "task1" {
		t.Fatalf("message = %+v", msg)
	}
	if msg.metadata["coordinator_wake_turn_id"] != "wake-turn-1" {
		t.Fatalf("metadata = %v", msg.metadata)
	}
	if reserved == "" || msg.turnID != reserved || accepted != reserved {
		t.Fatalf("turn ids: message=%q reserved=%q accepted=%q, want one shared id", msg.turnID, reserved, accepted)
	}
	if len(events) != 2 || events[0] != "reserved" || events[1] != "accepted" {
		t.Fatalf("event order = %v, want reserved then accepted", events)
	}
	if len(f.agent.capturedPrompts) != 1 {
		t.Fatalf("prompts sent = %d, want 1", len(f.agent.capturedPrompts))
	}
}

func TestPromptUnattendedWake_StoreFailureIsNotDispatchedAndSendsNothing(t *testing.T) {
	f := newWakePromptFixture(t, models.TaskSessionStateWaitingForInput)
	f.messages.userMessageErr = errors.New("store down")
	var accepted bool
	_, err := f.svc.PromptUnattendedWake(context.Background(), UnattendedWakePrompt{
		TaskID: "task1", SessionID: "session1", Content: "x", WakeTurnID: "wake-turn-1",
		OnAccepted: func(string) { accepted = true },
	})
	if !errors.Is(err, ErrWakePromptNotDispatched) {
		t.Fatalf("err = %v, want ErrWakePromptNotDispatched", err)
	}
	if accepted || len(f.agent.capturedPrompts) != 0 {
		t.Fatalf("a failed seam must send nothing: accepted=%v prompts=%d", accepted, len(f.agent.capturedPrompts))
	}
}

func TestPromptUnattendedWake_UnpromptableSessionIsNotDispatched(t *testing.T) {
	f := newWakePromptFixture(t, models.TaskSessionStateFailed)
	_, err := f.svc.PromptUnattendedWake(context.Background(), UnattendedWakePrompt{
		TaskID: "task1", SessionID: "session1", Content: "x", WakeTurnID: "wake-turn-1",
	})
	if !errors.Is(err, ErrWakePromptNotDispatched) {
		t.Fatalf("err = %v, want ErrWakePromptNotDispatched", err)
	}
	if len(f.messages.userMessages) != 0 || len(f.agent.capturedPrompts) != 0 {
		t.Fatal("nothing may be stored or sent for an unpromptable session")
	}
}

func TestPromptUnattendedWake_NeverUsesTheQueue(t *testing.T) {
	f := newWakePromptFixture(t, models.TaskSessionStateRunning)
	_, err := f.svc.PromptUnattendedWake(context.Background(), UnattendedWakePrompt{
		TaskID: "task1", SessionID: "session1", Content: "x", WakeTurnID: "wake-turn-1",
	})
	if !errors.Is(err, ErrWakePromptNotDispatched) {
		t.Fatalf("a busy session must fail as not dispatched, got %v", err)
	}
	if queued, _ := f.svc.messageQueue.HasPendingForSession(context.Background(), "session1"); queued {
		t.Fatal("a wake send must never be queued")
	}
}

func TestPromptUnattendedWake_DispatchFailureAfterStoreIsNotMarkedNotDispatched(t *testing.T) {
	f := newWakePromptFixture(t, models.TaskSessionStateWaitingForInput)
	f.agent.promptErr = errors.New("transport broke")
	var accepted bool
	_, err := f.svc.PromptUnattendedWake(context.Background(), UnattendedWakePrompt{
		TaskID: "task1", SessionID: "session1", Content: "x", WakeTurnID: "wake-turn-1",
		OnAccepted: func(string) { accepted = true },
	})
	if err == nil {
		t.Fatal("a failed dispatch must return an error")
	}
	if errors.Is(err, ErrWakePromptNotDispatched) {
		t.Fatalf("a failure after the message was stored may not be reported as not dispatched: %v", err)
	}
	if accepted {
		t.Fatal("a failed dispatch must not report acceptance")
	}
}
