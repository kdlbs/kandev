package orchestrator

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentruntime"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

func TestReclaimIdleSessionSkipLogIncludesRowDiagnostics(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "task-reclaim-log", "session-reclaim-log", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-reclaim-log", SessionID: "session-reclaim-log",
		TaskID: "task-reclaim-log", AgentExecutionID: "exec-reclaim-log",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusPrepared,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}

	core, observed := observer.New(zap.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		isAgentRunning: false,
	})
	svc.logger = log
	svc.turnService = &inactiveTurnService{}

	if err := svc.reclaimIdleSession(ctx, "session-reclaim-log"); err != nil {
		t.Fatalf("reclaimIdleSession: %v", err)
	}
	entries := observed.FilterMessage("idle reclaim skipped").All()
	if len(entries) != 1 {
		t.Fatalf("skip log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if got, ok := fields["has_resume_token"].(bool); !ok || got {
		t.Errorf("has_resume_token = %v, want false", fields["has_resume_token"])
	}
	if got := fields["row_status"]; got != models.ExecutorRunningStatusPrepared {
		t.Errorf("row_status = %v, want %q", got, models.ExecutorRunningStatusPrepared)
	}
	if _, ok := fields["resume_token"]; ok {
		t.Error("skip log must not include the resume token")
	}
}
