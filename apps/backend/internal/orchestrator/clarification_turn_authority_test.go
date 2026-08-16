package orchestrator

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type unusedClarificationTurnService struct{ TurnService }

func TestClarificationTurnAuthorityDistinguishesMissingDependencyFromMissingIdentity(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	observedLogger, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	svc := &Service{logger: observedLogger}
	data := clarificationAnsweredData{SessionID: "session-1", PendingID: "pending-1"}

	data.ClarificationTurnID = "turn-1"
	if svc.clarificationTurnStillCurrent(context.Background(), data) {
		t.Fatal("missing turn service accepted clarification fallback")
	}
	warnings := logs.FilterMessage("skipping clarification fallback: turn service unavailable").All()
	if len(warnings) != 1 || warnings[0].Level != zapcore.WarnLevel {
		t.Fatalf("missing turn service logs = %#v, want one warning", warnings)
	}
	logs.TakeAll()

	svc.turnService = unusedClarificationTurnService{}
	data.ClarificationTurnID = ""
	if svc.clarificationTurnStillCurrent(context.Background(), data) {
		t.Fatal("missing clarification turn ID accepted fallback")
	}
	debugEntries := logs.FilterMessage("skipping clarification fallback: event carries no turn ID").All()
	if len(debugEntries) != 1 || debugEntries[0].Level != zapcore.DebugLevel {
		t.Fatalf("missing turn ID logs = %#v, want one debug entry", debugEntries)
	}
}

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
