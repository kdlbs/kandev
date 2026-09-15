package service_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// newObservedTestService is newTestServiceWithRepo's setup, but wired
// to an observed zap logger so a test can assert on structured log
// fields (newTestServiceWithRepo always builds a discard logger).
func newObservedTestService(t *testing.T) (
	*runsservice.Service, *runssqlite.Repository, *observer.ObservedLogs,
) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	seedAgentProfile(t, db, "agent-primary")

	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	eb := bus.NewMemoryEventBus(log)

	svc := runsservice.New(officeRepo.RunsRepository(), eb, log, nil)
	return svc, officeRepo.RunsRepository(), logs
}

// TestQueueRun_DepthRefusalLogsTheRefusingDepth pins
// AC-OFFICE-LAUNCH-SAFETY-003.5: a causation-depth refusal's durable
// record must name "the causation identifier, the refusing depth, and
// the agent that requested it" — not just the identifier and agent.
func TestQueueRun_DepthRefusalLogsTheRefusingDepth(t *testing.T) {
	svc, repo, logs := newObservedTestService(t)
	svc.SetLaunchSafetyLimits(1, runsservice.DefaultSelfTriggerAllowance)

	ctx := context.Background()
	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "root_reason",
		ActorKind:      models.ActorKindSystem,
	}); err != nil {
		t.Fatalf("queue root: %v", err)
	}
	root := getRun(t, repo, "agent-primary", "root_reason")

	if _, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "child1_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		CausingRunID:   root.ID,
	}); err != nil {
		t.Fatalf("queue child1: %v", err)
	}
	child1 := getRun(t, repo, "agent-primary", "child1_reason")

	_, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		AgentProfileID: "agent-primary",
		Reason:         "child2_reason",
		ActorKind:      models.ActorKindAgent,
		ActorID:        "agent-primary",
		CausingRunID:   child1.ID,
	})
	var refusal *runsservice.RefusalError
	if !errorsAsRefusal(err, &refusal) {
		t.Fatalf("queue child2 err = %v, want *RefusalError", err)
	}
	if refusal.Gate != runsservice.RefusalCausationDepth {
		t.Fatalf("gate = %q, want %q", refusal.Gate, runsservice.RefusalCausationDepth)
	}

	entries := logs.FilterMessage("run enqueue refused").All()
	if len(entries) == 0 {
		t.Fatal("no 'run enqueue refused' log entry recorded")
	}
	entry := entries[len(entries)-1]
	fields := entry.ContextMap()
	if _, ok := fields["causation_depth"]; !ok {
		t.Errorf("log entry missing causation_depth field: %+v", fields)
	}
	if _, ok := fields["max_causation_depth"]; !ok {
		t.Errorf("log entry missing max_causation_depth field: %+v", fields)
	}
}

func errorsAsRefusal(err error, target **runsservice.RefusalError) bool {
	re, ok := err.(*runsservice.RefusalError)
	if !ok {
		return false
	}
	*target = re
	return true
}
