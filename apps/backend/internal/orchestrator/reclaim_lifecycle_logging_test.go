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

func TestReclaimIdleSessionRoutineRefusalDoesNotLogPerSession(t *testing.T) {
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
	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("routine refusal emitted logs: %+v", entries)
	}
}

func TestReclaimIdleSessionActiveLSPRefusalDoesNotLogPerSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "task-reclaim-lsp-log", "session-reclaim-lsp-log", models.TaskSessionStateWaitingForInput)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-reclaim-lsp-log", SessionID: "session-reclaim-lsp-log",
		TaskID: "task-reclaim-lsp-log", AgentExecutionID: "exec-reclaim-lsp-log",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		ResumeToken: "resume-token", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}
	core, observed := observer.New(zap.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	manager := newReclaimTrackingAgentManager(&mockAgentManager{isAgentRunning: false})
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.logger = log
	svc.turnService = &inactiveTurnService{}
	svc.SetLSPLeaseLifecycle(&activeLSPLeaseForTest{sessionID: "session-reclaim-lsp-log"})

	if err := svc.reclaimIdleSession(ctx, "session-reclaim-lsp-log"); err != nil {
		t.Fatalf("reclaimIdleSession: %v", err)
	}
	if entries := observed.All(); len(entries) != 0 {
		t.Fatalf("active-lease refusal emitted logs: %+v", entries)
	}
	if manager.callCount() != 0 {
		t.Fatalf("cleanup calls = %v, want none with active LSP lease", manager.callsSnapshot())
	}
}

func TestReclaimIdleSessionKeepsProbeWarningAndSuccessInfo(t *testing.T) {
	t.Run("probe failure warning remains", func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		seedTaskAndSession(t, repo, "task-reclaim-probe-log", "session-reclaim-probe-log", models.TaskSessionStateWaitingForInput)
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "session-reclaim-probe-log", SessionID: "session-reclaim-probe-log",
			TaskID: "task-reclaim-probe-log", AgentExecutionID: "exec-reclaim-probe-log",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
			ResumeToken: "resume-token", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("upsert executor row: %v", err)
		}
		core, observed := observer.New(zap.DebugLevel)
		log, err := commonlogger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("create observer logger: %v", err)
		}
		base := &mockAgentManager{isAgentRunning: false}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &failingAgentLivenessProbe{mockAgentManager: base})
		svc.logger = log
		svc.turnService = &inactiveTurnService{}

		if err := svc.reclaimIdleSession(ctx, "session-reclaim-probe-log"); err != nil {
			t.Fatalf("reclaimIdleSession: %v", err)
		}
		entries := observed.FilterMessage("idle reclaim skipped; agent liveness probe failed").All()
		if len(entries) != 1 || entries[0].Level != zap.WarnLevel {
			t.Fatalf("probe failure logs = %+v, want one warning", entries)
		}
	})

	t.Run("successful reclaim info remains", func(t *testing.T) {
		ctx := context.Background()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		seedTaskAndSession(t, repo, "task-reclaim-success-log", "session-reclaim-success-log", models.TaskSessionStateWaitingForInput)
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "session-reclaim-success-log", SessionID: "session-reclaim-success-log",
			TaskID: "task-reclaim-success-log", AgentExecutionID: "exec-reclaim-success-log",
			Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
			ResumeToken: "resume-token", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("upsert executor row: %v", err)
		}
		core, observed := observer.New(zap.DebugLevel)
		log, err := commonlogger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("create observer logger: %v", err)
		}
		manager := &mockAgentManager{
			isAgentRunning: false,
			rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
				return models.ProcessLivenessDead
			},
		}
		svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
		svc.logger = log
		svc.turnService = &inactiveTurnService{}

		if err := svc.reclaimIdleSession(ctx, "session-reclaim-success-log"); err != nil {
			t.Fatalf("reclaimIdleSession: %v", err)
		}
		entries := observed.FilterMessage("idle reclaim: provider runtime released; row preserved for resume").All()
		if len(entries) != 1 || entries[0].Level != zap.InfoLevel {
			t.Fatalf("successful reclaim logs = %+v, want one info entry", entries)
		}
	})
}
