package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type orderedResetAgentManager struct {
	*mockAgentManager
	events chan string
}

func (m *orderedResetAgentManager) CancelAgent(ctx context.Context, sessionID string) error {
	m.events <- "cancel"
	return m.mockAgentManager.CancelAgent(ctx, sessionID)
}

func (m *orderedResetAgentManager) ResetAgentContext(ctx context.Context, executionID string) error {
	m.events <- "reset"
	return m.mockAgentManager.ResetAgentContext(ctx, executionID)
}

func newActiveResetTestService(t *testing.T) (*Service, *sqliterepo.Repository, *orderedResetAgentManager, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	seedExecutorRunning(t, repo, "session1", "task1", "execution1")

	baseManager := &mockAgentManager{repoForExecutionLookup: repo}
	manager := &orderedResetAgentManager{
		mockAgentManager: baseManager,
		events:           make(chan string, 4),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.turnService = &repoTurnService{repo: repo}
	turn, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start active turn: %v", err)
	}
	svc.activeTurns.Store("session1", turn.ID)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	return svc, repo, manager, session
}

func requireResetEvents(t *testing.T, events <-chan string, want ...string) {
	t.Helper()
	for _, expected := range want {
		select {
		case got := <-events:
			if got != expected {
				t.Fatalf("event order = %q, want next event %q", got, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", expected)
		}
	}
}

func TestResetAgentContext_QuiescesActiveTurnBeforeProviderReset(t *testing.T) {
	svc, _, manager, session := newActiveResetTestService(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	if !svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned false")
	}

	requireResetEvents(t, manager.events, "cancel", "reset")
	if len(messages.sessionMessages) != 0 {
		t.Fatalf("internal reset cancellation created %d session messages", len(messages.sessionMessages))
	}
}

func TestResetAgentContext_ActiveTurnAllowsSuccessorPrompt(t *testing.T) {
	svc, repo, manager, session := newActiveResetTestService(t)
	manager.isAgentRunning = true
	manager.promptAgentFunc = func(
		_ context.Context,
		_ string,
		_ string,
		_ []v1.MessageAttachment,
		_ bool,
	) (*executor.PromptResult, error) {
		manager.events <- "prompt"
		return &executor.PromptResult{}, nil
	}
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	if !svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned false")
	}
	resetSession, err := svc.repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("reload reset session: %v", err)
	}
	step := &wfmodels.WorkflowStep{
		ID:         "step2",
		WorkflowID: "wf1",
		Name:       "Successor",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
		}},
	}
	if err := svc.autoStartStepPrompt(
		context.Background(), "task1", resetSession, step, "successor prompt", false, false,
	); err != nil {
		t.Fatalf("auto-start successor prompt: %v", err)
	}

	requireResetEvents(t, manager.events, "cancel", "reset", "prompt")
}

func TestResetAgentContext_CancelFailureStopsProviderReset(t *testing.T) {
	svc, _, manager, session := newActiveResetTestService(t)
	manager.cancelAgentErr = errors.New("cancel failed")

	if svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned true after cancellation failure")
	}

	requireResetEvents(t, manager.events, "cancel")
	if got := len(manager.restartProcessCalls); got != 0 {
		t.Fatalf("provider reset calls = %d, want 0", got)
	}
}
