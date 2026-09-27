package backendapp

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	officecosts "github.com/kandev/kandev/internal/office/costs"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
)

// TestTasklessRunResolvesBoundDynamicProfile reproduces the Office defect where
// the first heartbeat of a CEO row bound to a dynamic profile failed with
// "agent profile belongs to a virtual execution family" because the taskless
// launch passed the virtual family ID as the execution profile. With the
// Office binding the launcher resolves a concrete candidate while the run
// session keeps the Office ID as its owner.
func TestTasklessRunResolvesBoundDynamicProfile(t *testing.T) {
	ctx := context.Background()
	officeRepo, settingsRepo := openTasklessDynamicRepo(t)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	svc := officeservice.NewService(officeservice.ServiceOptions{
		Repo: officeRepo, Logger: log, EventBus: eventBus,
	})
	svc.SetSyncHandlers(true)
	if err := svc.RegisterEventSubscribers(eventBus); err != nil {
		t.Fatalf("register event subscribers: %v", err)
	}
	activity := shared.NewActivityLogger(officeRepo, log)
	svc.SetBudgetChecker(officecosts.NewCostService(officeRepo, log, activity, svc, svc))

	resolver := agentruntime.NewProfileExecutionResolver(settingsRepo, dynamicruntime.NewEngine(), true)
	launcher := newOfficeRunSessionLauncher(officeRepo, newFakeReconcileBackend(), resolver, log)
	svc.SetRunSessionLauncher(launcher)

	agent := &officemodels.AgentInstance{
		ID: "ceo-dynamic", WorkspaceID: "ws-1", Name: "CEO", AgentID: "dynamic",
		ExecutionAgentProfileID: "profile-dynamic",
		Role:                    officemodels.AgentRoleCEO,
		Status:                  officemodels.AgentStatusIdle,
		ExecutorPreference:      `{"type":"local_pc"}`,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := svc.QueueRun(ctx, agent.ID, officeservice.RunReasonRoutineTrigger, `{}`, "taskless-dynamic"); err != nil {
		t.Fatalf("queue run: %v", err)
	}
	officeservice.RunSchedulerTick(svc, ctx)

	runs, err := svc.ListRuns(ctx, agent.WorkspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: runs=%#v err=%v", runs, err)
	}
	if runs[0].SessionID == "" {
		t.Fatalf("run was not launched: %#v", runs[0])
	}
	session, err := officeRepo.GetRunSession(ctx, runs[0].SessionID)
	if err != nil || session == nil {
		t.Fatalf("get run session %q: %v", runs[0].SessionID, err)
	}
	if session.ExecutionProfileID != "concrete-profile" {
		t.Fatalf("execution profile = %q, want the bound concrete candidate", session.ExecutionProfileID)
	}
	if session.Adapter != "concrete-agent" {
		t.Fatalf("adapter = %q, want concrete-agent", session.Adapter)
	}
	if session.AgentProfileID != agent.ID {
		t.Fatalf("run session owner = %q, want the Office ID %q", session.AgentProfileID, agent.ID)
	}
}

func openTasklessDynamicRepo(
	t *testing.T,
) (*officesqlite.Repository, settingsstore.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "taskless-dynamic.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	settingsRepo, cleanup, err := settingsstore.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	officeRepo, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("office repository: %v", err)
	}
	ctx := context.Background()
	for _, id := range []string{"dynamic", "concrete-agent"} {
		if _, err := sqlxDB.Exec(
			`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id, id,
		); err != nil {
			t.Fatalf("seed agents row %q: %v", id, err)
		}
	}
	for _, profile := range []*agentsettingsmodels.AgentProfile{
		{ID: "profile-dynamic", AgentID: "dynamic", Name: "Cascade", Enabled: true},
		{ID: "concrete-profile", AgentID: "concrete-agent", Name: "Concrete", Model: "opus", Enabled: true},
	} {
		if err := settingsRepo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatalf("create profile %q: %v", profile.ID, err)
		}
	}
	if err := settingsRepo.CreateDynamicAgentProfile(ctx,
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: "profile-dynamic", Version: 1},
		[]agentsettingsmodels.DynamicAgentRoute{{
			DynamicProfileID: "profile-dynamic", ExecutionProfileID: "concrete-profile", Enabled: true,
		}},
	); err != nil {
		t.Fatalf("create dynamic profile: %v", err)
	}
	return officeRepo, settingsRepo
}
