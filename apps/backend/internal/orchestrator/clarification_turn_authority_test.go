package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestClarificationWatchdogDoesNotDispatchAfterTurnIsSuperseded(t *testing.T) {
	svc, agentMgr := setupSupersededClarificationTurn(t)
	svc.clarificationWatchdogTimeout = time.Millisecond
	t.Cleanup(func() { svc.cancelAllClarificationWatchdogs() })
	svc.scheduleClarificationWatchdog(clarificationAnsweredData{
		TaskID: "task-watchdog-authority", SessionID: "session-watchdog-authority",
		PendingID: "pending-watchdog-authority", ClarificationTurnID: "turn-clarification",
		AnswerText: "Continue",
	})
	deadline := time.Now().Add(time.Second)
	for countClarificationWatchdogs(svc) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := countClarificationWatchdogs(svc); got != 0 {
		t.Fatalf("stale watchdog entries = %d, want 0", got)
	}

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	if got := len(agentMgr.capturedPromptCalls); got != 0 {
		t.Fatalf("stale watchdog prompt calls = %d, want 0", got)
	}
}

func TestPromptAdmissionRejectsSupersededClarificationTurn(t *testing.T) {
	svc, agentMgr := setupSupersededClarificationTurn(t)
	_, err := svc.promptTask(
		context.Background(),
		"task-watchdog-authority",
		"session-watchdog-authority",
		"Continue",
		"",
		false,
		nil,
		false,
		promptTaskOptions{expectedCurrentTurnID: "turn-clarification"},
	)
	if err == nil {
		t.Fatal("superseded clarification prompt admission succeeded")
	}
	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	if got := len(agentMgr.capturedPromptCalls); got != 0 {
		t.Fatalf("superseded admission prompt calls = %d, want 0", got)
	}
}

func setupSupersededClarificationTurn(t *testing.T) (*Service, *mockAgentManager) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-watchdog-authority", "session-watchdog-authority", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-watchdog-authority", "task-watchdog-authority", "exec-watchdog-authority")
	base := time.Date(2026, time.August, 16, 16, 40, 0, 0, time.UTC)
	completedAt := base.Add(time.Second)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-clarification", TaskID: "task-watchdog-authority",
		TaskSessionID: "session-watchdog-authority", StartedAt: base, CreatedAt: base,
		CompletedAt: &completedAt,
	}); err != nil {
		t.Fatalf("create clarification turn: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-successor", TaskID: "task-watchdog-authority",
		TaskSessionID: "session-watchdog-authority", StartedAt: base.Add(time.Minute),
		CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("create successor turn: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	return svc, agentMgr
}
