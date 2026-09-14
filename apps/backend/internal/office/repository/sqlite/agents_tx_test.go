package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestCreateAgentInstanceTx_CommitPersists verifies config sync can write a
// new agent inside a caller-owned transaction and have it visible once the
// transaction commits (AC-OFFICE-CONFIG-SYNC-003.14's per-entity atomicity).
func TestCreateAgentInstanceTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullAgentInstance("agent-tx-1", "ws-1", "Ada")
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.CreateAgentInstanceTx(ctx, tx, want); err != nil {
		t.Fatalf("CreateAgentInstanceTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "agent-tx-1")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	assertAgentProjection(t, got, want)
}

// TestCreateAgentInstanceTx_RollbackDiscards verifies a rolled-back
// transaction leaves no row behind — the "New" apply case must not partially
// land if the manifest write in the same transaction fails.
func TestCreateAgentInstanceTx_RollbackDiscards(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-tx-2", "ws-1", "Grace")
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.CreateAgentInstanceTx(ctx, tx, agent); err != nil {
		t.Fatalf("CreateAgentInstanceTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetAgentInstance(ctx, "agent-tx-2"); err == nil {
		t.Fatal("GetAgentInstance() error = nil, want not-found after rollback")
	}
}

// TestUpdateAgentInstanceConfigFieldsTx_CommitPersists verifies the
// transaction-scoped config-field writer used by reconcile's "Existing"
// apply case.
func TestUpdateAgentInstanceConfigFieldsTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-tx-3", "ws-1", "Marie")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	fields := sqlite.AgentInstanceConfigFields{
		Role:                  "reviewer",
		Icon:                  "🔍",
		BudgetMonthlyCents:    999,
		MaxConcurrentSessions: 5,
		DesiredSkills:         `["python"]`,
		ExecutorPreference:    "local_pc",
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.UpdateAgentInstanceConfigFieldsTx(ctx, tx, "agent-tx-3", fields); err != nil {
		t.Fatalf("UpdateAgentInstanceConfigFieldsTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "agent-tx-3")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if string(got.Role) != fields.Role || got.Icon != fields.Icon ||
		got.BudgetMonthlyCents != fields.BudgetMonthlyCents ||
		got.MaxConcurrentSessions != fields.MaxConcurrentSessions ||
		got.DesiredSkills != fields.DesiredSkills ||
		got.ExecutorPreference != fields.ExecutorPreference {
		t.Errorf("got %+v, want owned fields %+v", got, fields)
	}
}

// TestUpdateAgentInstanceConfigFieldsTx_RollbackDiscards verifies a
// rolled-back config-field update leaves the prior row untouched.
func TestUpdateAgentInstanceConfigFieldsTx_RollbackDiscards(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-tx-4", "ws-1", "Alan")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	fields := sqlite.AgentInstanceConfigFields{Role: "changed"}
	if err := repo.UpdateAgentInstanceConfigFieldsTx(ctx, tx, "agent-tx-4", fields); err != nil {
		t.Fatalf("UpdateAgentInstanceConfigFieldsTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "agent-tx-4")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if string(got.Role) != string(agent.Role) {
		t.Errorf("Role = %q, want unchanged %q after rollback", got.Role, agent.Role)
	}
}

// TestUpdateAgentReportsToTx_CommitPersists verifies the second-pass
// reports_to writer used after all agents in a sync run are written.
func TestUpdateAgentReportsToTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-tx-5", "ws-1", "Turing")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.UpdateAgentReportsToTx(ctx, tx, "agent-tx-5", "agent-lead"); err != nil {
		t.Fatalf("UpdateAgentReportsToTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "agent-tx-5")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.ReportsTo != "agent-lead" {
		t.Errorf("ReportsTo = %q, want agent-lead", got.ReportsTo)
	}
}

// TestDeleteAgentInstanceTx_CommitRemoves verifies the transaction-scoped
// delete used by reconcile's "Removed upstream" apply case.
func TestDeleteAgentInstanceTx_CommitRemoves(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-tx-6", "ws-1", "Lovelace")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteAgentInstanceTx(ctx, tx, "agent-tx-6"); err != nil {
		t.Fatalf("DeleteAgentInstanceTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := repo.GetAgentInstance(ctx, "agent-tx-6"); err == nil {
		t.Fatal("GetAgentInstance() error = nil, want not-found after delete commit")
	}
}

// TestDeleteAgentInstanceTx_RollbackKeepsRow verifies a rolled-back delete
// leaves the row intact.
func TestDeleteAgentInstanceTx_RollbackKeepsRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("agent-tx-7", "ws-1", "Hopper")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteAgentInstanceTx(ctx, tx, "agent-tx-7"); err != nil {
		t.Fatalf("DeleteAgentInstanceTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetAgentInstance(ctx, "agent-tx-7"); err != nil {
		t.Fatalf("GetAgentInstance() error = %v, want row to survive rollback", err)
	}
}

// TestDefaultAgentID_NoAgentsReturnsEmpty verifies a workspace with no
// registered CLI tools and no existing Office agents yields "", the signal
// config sync's agent-create path uses to leave AgentID unset rather than
// invent a value.
func TestDefaultAgentID_NoAgentsReturnsEmpty(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if got := repo.DefaultAgentID(ctx, "ws-empty"); got != "" {
		t.Errorf("DefaultAgentID() = %q, want empty", got)
	}
}

// TestDefaultAgentID_InheritsFromExistingWorkspaceAgent verifies config
// sync's agent-create path (which cannot repeat CreateAgentInstance's
// FK-inheritance lookup inside its own uncommitted transaction) can resolve
// the same default by calling DefaultAgentID beforehand.
func TestDefaultAgentID_InheritsFromExistingWorkspaceAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	existing := fullAgentInstance("agent-tx-8", "ws-1", "Grace")
	existing.AgentID = "cli-claude"
	if err := repo.CreateAgentInstance(ctx, existing); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	if got := repo.DefaultAgentID(ctx, "ws-1"); got != "cli-claude" {
		t.Errorf("DefaultAgentID() = %q, want %q", got, "cli-claude")
	}
	if got := repo.DefaultAgentID(ctx, "ws-other"); got != "" {
		t.Errorf("DefaultAgentID() for unrelated workspace = %q, want empty", got)
	}
}
