package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	runtimeapi "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestClassifyStartupCleanupDecision(t *testing.T) {
	tests := []struct {
		name            string
		err             error
		liveness        models.ProcessLiveness
		wantDisposition startupCleanupDisposition
		wantProceed     bool
		wantExpected    bool
	}{
		{
			name:            "successful stop",
			liveness:        models.ProcessLivenessAlive,
			wantDisposition: startupCleanupDispositionStopped,
			wantProceed:     true,
		},
		{
			name:            "not found dead local",
			err:             runtimeapi.ErrNotFound,
			liveness:        models.ProcessLivenessDead,
			wantDisposition: startupCleanupDispositionAlreadyAbsent,
			wantProceed:     true,
		},
		{
			name:            "not found alive local",
			err:             runtimeapi.ErrNotFound,
			liveness:        models.ProcessLivenessAlive,
			wantDisposition: startupCleanupDispositionPreserved,
			wantExpected:    true,
		},
		{
			name:            "not found unknown",
			err:             runtimeapi.ErrNotFound,
			liveness:        models.ProcessLivenessUnknown,
			wantDisposition: startupCleanupDispositionPreserved,
			wantExpected:    true,
		},
		{
			name:            "unexpected stop failure",
			err:             errors.New("database unavailable"),
			liveness:        models.ProcessLivenessDead,
			wantDisposition: startupCleanupDispositionPreserved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := classifyStartupCleanup(tt.err, tt.liveness)
			if decision.disposition != tt.wantDisposition {
				t.Fatalf("disposition = %q, want %q", decision.disposition, tt.wantDisposition)
			}
			if decision.proceed != tt.wantProceed {
				t.Fatalf("proceed = %v, want %v", decision.proceed, tt.wantProceed)
			}
			if decision.expected != tt.wantExpected {
				t.Fatalf("expected = %v, want %v", decision.expected, tt.wantExpected)
			}
		})
	}
}

func TestStartupCleanupReportAggregatesExpectedPreservationWithoutSecrets(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	report := newStartupCleanupReport()
	row := &models.ExecutorRunning{
		SessionID:        "session-safe",
		TaskID:           "task-safe",
		AgentExecutionID: "exec-safe",
		ResumeToken:      "resume-token-must-not-log",
	}
	decision := classifyStartupCleanup(runtimeapi.ErrNotFound, models.ProcessLivenessUnknown)
	report.recordExpected(row, decision)
	report.recordExpected(row, decision)
	report.flush(log)

	if logs.Len() != 1 {
		t.Fatalf("summary log count = %d, want 1", logs.Len())
	}
	entry := logs.All()[0]
	if entry.Message != "startup runtime cleanup summary" {
		t.Fatalf("summary message = %q", entry.Message)
	}
	got := fmt.Sprint(entry.ContextMap())
	if !containsAll(got, "expected_preserved_count", "runtime_not_found", "unknown") {
		t.Fatalf("summary fields = %s", got)
	}
	if containsAll(got, "resume-token-must-not-log") {
		t.Fatal("summary leaked a resume token")
	}
}

func TestStopRuntimeForStartupCleanupPreservesUnknownRowsWithoutHandle(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seedTaskAndSession(t, repo, "taskUnknown", "sessionUnknown", models.TaskSessionStateCancelled)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "sessionUnknown", SessionID: "sessionUnknown", TaskID: "taskUnknown",
		Runtime: agentruntime.RuntimeSSH, Status: models.ExecutorRunningStatusRunning,
		PID: 9191, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert unknown row: %v", err)
	}

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessUnknown
		},
	})
	service.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "sessionUnknown"); err != nil {
		t.Fatalf("unknown row without a stop handle must be preserved: %v", err)
	}
}

func TestRepairDeadRowLivenessLogSeverity(t *testing.T) {
	t.Run("rotated execution is expected", func(t *testing.T) {
		repo := setupTestRepo(t)
		ctx := context.Background()
		seedSession(t, repo, "task-rotated", "session-rotated", "")
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID: "session-rotated", SessionID: "session-rotated", TaskID: "task-rotated",
			AgentExecutionID: "new-execution", Status: models.ExecutorRunningStatusRunning,
		}); err != nil {
			t.Fatalf("UpsertExecutorRunning: %v", err)
		}
		current, err := repo.GetExecutorRunningBySessionID(ctx, "session-rotated")
		if err != nil {
			t.Fatalf("GetExecutorRunningBySessionID: %v", err)
		}
		stale := *current
		stale.AgentExecutionID = "old-execution"

		service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
		core, logs := observer.New(zapcore.DebugLevel)
		log, err := logger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("NewFromZap: %v", err)
		}
		service.logger = log

		if err := service.repairDeadRowLiveness(ctx, &stale); err != nil {
			t.Fatalf("repairDeadRowLiveness: %v", err)
		}
		if entries := logs.FilterLevelExact(zapcore.WarnLevel).All(); len(entries) != 0 {
			t.Fatalf("rotated repair warning count = %d, want 0", len(entries))
		}
		current, err = repo.GetExecutorRunningBySessionID(ctx, "session-rotated")
		if err != nil {
			t.Fatalf("GetExecutorRunningBySessionID after repair: %v", err)
		}
		if current.AgentExecutionID != "new-execution" || current.Status != models.ExecutorRunningStatusRunning {
			t.Fatalf("rotated row = %+v, want newer running execution unchanged", current)
		}
	})

	t.Run("unrelated repair error remains warning", func(t *testing.T) {
		repo := setupTestRepo(t)
		service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
		core, logs := observer.New(zapcore.DebugLevel)
		log, err := logger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("NewFromZap: %v", err)
		}
		service.logger = log

		err = service.repairDeadRowLiveness(context.Background(), &models.ExecutorRunning{})
		if err == nil {
			t.Fatal("repairDeadRowLiveness returned nil for an invalid repair request")
		}
		entries := logs.FilterMessage("failed to repair executor row liveness during reconciliation").All()
		if len(entries) != 1 {
			t.Fatalf("unrelated repair warning count = %d, want 1", len(entries))
		}
		if entries[0].Level != zapcore.WarnLevel {
			t.Fatalf("unrelated repair log level = %s, want warn", entries[0].Level)
		}
	})
}

func containsAll(value string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(value, needle) {
			return false
		}
	}
	return true
}
