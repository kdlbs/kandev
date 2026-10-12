package service_test

// Covers AC-OFFICE-RUN-CAUSATION-001.3/.4: QueueRunWithActor previously had
// no way to name a causing run at all, so every caller — including
// office/runtime's SpawnAgentRun, which always has a live invoking run
// (runCtx.RunID) — was forced to root a new causation chain instead of
// chaining off the run that caused it. QueueRunWithActor gained a trailing
// causingRunID parameter; this file proves it reaches the persisted run's
// causation-chain fields end to end through the real runs/service seam,
// the same real-service pattern office/scheduler/run_delegation_test.go
// uses for the equivalent delegation proof.

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// newTestServiceWithRunsService builds a Service backed by a real
// runs/service.Service over the same repo, so QueueRunWithActor's
// causingRunID delegates into real causation resolution instead of the
// legacy queueRunInline fallback, which never resolves causation at all.
func newTestServiceWithRunsService(t *testing.T) (*service.Service, *officesqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	log := logger.Default()
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log})
	svc.SetRunsService(runsservice.New(repo.RunsRepository(), nil, log, nil))
	return svc, repo
}

func TestQueueRunWithActor_ChainsCausationFromCausingRunID(t *testing.T) {
	svc, repo := newTestServiceWithRunsService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Root cause: no causingRunID, so this roots a new causation chain.
	if _, err := svc.QueueRunWithActor(ctx, agent.ID, "heartbeat", `{}`, "",
		models.ActorKindAgent, agent.ID, ""); err != nil {
		t.Fatalf("queue root run: %v", err)
	}
	roots, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("want 1 root run, got %d", len(roots))
	}
	rootRun := roots[0]
	if rootRun.ChainCausationID != rootRun.ID {
		t.Fatalf("root causation_id = %q, want self-rooted %q", rootRun.ChainCausationID, rootRun.ID)
	}
	if rootRun.CausationDepth != 0 {
		t.Fatalf("root causation_depth = %d, want 0", rootRun.CausationDepth)
	}

	// Caused run: names the root as its causing run, exactly what
	// SpawnAgentRun does with runCtx.RunID.
	if _, err := svc.QueueRunWithActor(ctx, agent.ID, "spawn_agent_run", `{}`, "",
		models.ActorKindAgent, agent.ID, rootRun.ID); err != nil {
		t.Fatalf("queue caused run: %v", err)
	}

	all, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2 runs, got %d", len(all))
	}
	var causedRun *models.Run
	for _, r := range all {
		if r.ID != rootRun.ID {
			causedRun = r
		}
	}
	if causedRun == nil {
		t.Fatal("caused run not found")
	}
	if causedRun.ParentRunID != rootRun.ID {
		t.Errorf("parent_run_id = %q, want %q (the causing run)", causedRun.ParentRunID, rootRun.ID)
	}
	if causedRun.ChainCausationID != rootRun.ChainCausationID {
		t.Errorf("causation_id = %q, want %q (inherited from the causing run)",
			causedRun.ChainCausationID, rootRun.ChainCausationID)
	}
	if causedRun.CausationDepth != rootRun.CausationDepth+1 {
		t.Errorf("causation_depth = %d, want %d (causing run's depth + 1)",
			causedRun.CausationDepth, rootRun.CausationDepth+1)
	}
}
