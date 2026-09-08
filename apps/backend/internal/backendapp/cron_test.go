package backendapp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newTestOfficeRepoForCron builds a real office SQLite repository (plus the
// shared agent_profiles schema owned by internal/agent/settings/store) so
// the cooldown gate regression test exercises the actual SELECT/UPDATE
// statements rather than a fake — this is a repository-column bug
// (reading the wrong table's last_run_finished_at), and a fake would
// hide exactly that class of defect.
func newTestOfficeRepoForCron(t *testing.T) (*officesqlite.Repository, *sqlx.DB) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "cron-cooldown.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	_, cleanup, err := store.Provide(database, database, nil)
	if err != nil {
		t.Fatalf("agent settings store migrations: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	repo, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("office migrations: %v", err)
	}
	// agent_profiles.agent_id is a FK into agents (CLI tool registrations);
	// seed one so CreateAgentInstance's insert satisfies it.
	now := time.Now().UTC()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"agent-cli-tool", "cli-tool", now, now,
	); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}
	return repo, database
}

// TestHeartbeatAgentRuntime_AllowFire_ReadsOfficeRuntimeCooldown is the
// regression test for the cooldown-gate defect: AllowFire must gate on
// office_agent_runtime.last_run_finished_at (the column
// Service.stampRunFinished actually maintains), not
// agent_profiles.last_run_finished_at (a same-named column inherited
// from the shared kanban agent-settings table that no Office write path
// ever stamps). Before the fix, AllowFire read the agent_profiles
// column, which stays NULL for Office agents, so the cooldown branch
// never triggered and every case below returned true regardless of the
// office_agent_runtime state.
func TestHeartbeatAgentRuntime_AllowFire_ReadsOfficeRuntimeCooldown(t *testing.T) {
	repo, _ := newTestOfficeRepoForCron(t)
	ctx := context.Background()
	gate := &heartbeatAgentRuntime{office: repo}

	agent := &models.AgentInstance{
		ID:          "agent-cooldown-1",
		WorkspaceID: "ws-1",
		Name:        "cooldown-worker",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
		CooldownSec: 60,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	now := time.Now().UTC()

	// Poison agent_profiles.last_run_finished_at with a value inside the
	// 60s cooldown window (now - 5s), not outside it. If AllowFire were
	// still reading this column instead of office_agent_runtime, this
	// poisoned value alone would gate Case 1 below (no office_agent_runtime
	// row yet, wants true) since it never changes across cases — proving
	// the column is not consulted. Also confirms this column round-trips
	// harmlessly and is not itself relied on by the fixed gate.
	staleAgentProfilesValue := now.Add(-5 * time.Second)
	agent.LastRunFinishedAt = &staleAgentProfilesValue
	if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("poison agent_profiles.last_run_finished_at: %v", err)
	}

	// Case 1: no office_agent_runtime row yet -> an agent that has never
	// run is not gated by cooldown.
	allowed, err := gate.AllowFire(ctx, agent.ID, now)
	if err != nil {
		t.Fatalf("AllowFire (no runtime row): %v", err)
	}
	if !allowed {
		t.Error("AllowFire (no runtime row) = false, want true (never-run agent must not be gated)")
	}

	// Stamp office_agent_runtime.last_run_finished_at to "just now".
	if err := repo.UpdateRuntimeLastRunFinished(ctx, agent.ID, now); err != nil {
		t.Fatalf("stamp runtime: %v", err)
	}

	// Case 2: 30s into a 60s cooldown -> blocked.
	allowed, err = gate.AllowFire(ctx, agent.ID, now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("AllowFire (within cooldown): %v", err)
	}
	if allowed {
		t.Error("AllowFire (within cooldown) = true, want false")
	}

	// Case 3: 61s after the stamp -> cooldown elapsed, allowed.
	allowed, err = gate.AllowFire(ctx, agent.ID, now.Add(61*time.Second))
	if err != nil {
		t.Fatalf("AllowFire (after cooldown): %v", err)
	}
	if !allowed {
		t.Error("AllowFire (after cooldown) = false, want true")
	}
}

// TestHeartbeatAgentRuntime_AllowFire_ZeroCooldownSkipsRuntimeLookup
// pins the existing short-circuit: cooldown_sec <= 0 always allows
// firing without querying office_agent_runtime at all. The runtime
// table is dropped before the call, so an implementation that queries
// it anyway (instead of skipping) would surface as an error here
// rather than an unproven true.
func TestHeartbeatAgentRuntime_AllowFire_ZeroCooldownSkipsRuntimeLookup(t *testing.T) {
	repo, database := newTestOfficeRepoForCron(t)
	ctx := context.Background()
	gate := &heartbeatAgentRuntime{office: repo}

	agent := &models.AgentInstance{
		ID:          "agent-cooldown-2",
		WorkspaceID: "ws-1",
		Name:        "no-cooldown-worker",
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
		CooldownSec: 0,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DROP TABLE office_agent_runtime`); err != nil {
		t.Fatalf("drop office_agent_runtime: %v", err)
	}

	allowed, err := gate.AllowFire(ctx, agent.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("AllowFire queried office_agent_runtime despite cooldown_sec=0: %v", err)
	}
	if !allowed {
		t.Error("AllowFire (cooldown_sec=0) = false, want true")
	}
}
