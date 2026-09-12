package service_test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerOutcome_IdleSkipped_FinishRunErrorIsLogged is the PR fixup
// round 2 regression test (CodeRabbit). The idle-skip FinishRun call site
// used to discard FinishRun's error (`wrote, _ := ...`), treating a genuine
// persistence failure exactly like a benign lost terminal-write race
// (wrote=false, err=nil) — silently, with no log line either way. A run
// stuck 'claimed' because the terminal write itself failed needs to be
// distinguishable in logs from one that lost a race to another writer, so
// operators (and ReapStaleCheckouts/RecoverStale, the actual recovery path)
// aren't flying blind on which case occurred.
func TestSchedulerOutcome_IdleSkipped_FinishRunErrorIsLogged(t *testing.T) {
	core, logs := observer.New(zapcore.ErrorLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	svc := newTestService(t, service.ServiceOptions{Logger: log})
	ctx := context.Background()

	// Reject the UPDATE FinishRun issues (status -> 'finished') so the
	// idle-skip path's FinishRun call returns a genuine persistence error
	// instead of a lost-race no-op.
	svc.ExecSQL(t, `
		CREATE TRIGGER reject_idle_skip_finish
		BEFORE UPDATE OF status ON runs
		WHEN NEW.status = 'finished'
		BEGIN
			SELECT RAISE(ABORT, 'finish run write rejected for test');
		END;
	`)

	agent := &models.AgentInstance{
		WorkspaceID:        "ws-1",
		Name:               "idle-outcome-worker-err",
		Role:               models.AgentRoleWorker,
		Status:             models.AgentStatusIdle,
		ExecutorPreference: `{"type":"worktree"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonHeartbeat, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	service.RunSchedulerTick(svc, ctx)

	if logs.FilterMessage("failed to finish idle-skipped run").Len() == 0 {
		t.Fatal("expected the FinishRun persistence error to be logged, not silently swallowed")
	}

	run := findRunForAgent(t, svc, ctx, "ws-1", agent.ID, service.RunReasonHeartbeat)
	if run.Status != "claimed" {
		t.Fatalf("status = %q, want claimed — the write was rejected, so the run must stay "+
			"claimed for the stale-claim reaper to recover, not silently read as finished", run.Status)
	}
}
