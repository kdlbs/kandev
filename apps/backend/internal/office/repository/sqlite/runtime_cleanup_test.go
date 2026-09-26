package sqlite_test

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedRoutine inserts an office_routines row plus a trigger and a routine
// run for it, so the per-routine cleanup deletes have something to remove.
func seedRoutine(t *testing.T, repo *sqlite.Repository, routineID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at)
		VALUES (?, 'ws-1', ?, datetime('now'), datetime('now'))
	`, routineID, routineID); err != nil {
		t.Fatalf("seed routine %s: %v", routineID, err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_routine_triggers (id, routine_id, kind, created_at, updated_at)
		VALUES (?, ?, 'cron', datetime('now'), datetime('now'))
	`, "trg-"+routineID, routineID); err != nil {
		t.Fatalf("seed trigger for %s: %v", routineID, err)
	}
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_routine_runs (id, routine_id, source, created_at)
		VALUES (?, ?, 'cron', datetime('now'))
	`, "run-"+routineID, routineID); err != nil {
		t.Fatalf("seed routine run for %s: %v", routineID, err)
	}
}

func TestListDistinctTriggerRoutineIDs(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	ids, err := repo.ListDistinctTriggerRoutineIDs(ctx)
	if err != nil {
		t.Fatalf("ListDistinctTriggerRoutineIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want none on a clean database", ids)
	}

	seedRoutine(t, repo, "rt-a")
	seedRoutine(t, repo, "rt-b")
	// A second trigger for rt-a must not produce a duplicate id.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_routine_triggers (id, routine_id, kind, created_at, updated_at)
		VALUES ('trg-rt-a-2', 'rt-a', 'webhook', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed second trigger: %v", err)
	}

	ids, err = repo.ListDistinctTriggerRoutineIDs(ctx)
	if err != nil {
		t.Fatalf("ListDistinctTriggerRoutineIDs: %v", err)
	}
	sort.Strings(ids)
	if strings.Join(ids, ",") != "rt-a,rt-b" {
		t.Errorf("ids = %v, want rt-a,rt-b exactly once each", ids)
	}
}

func TestDeleteTriggersAndRunsByRoutineID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedRoutine(t, repo, "rt-target")
	seedRoutine(t, repo, "rt-keep")

	if err := repo.DeleteTriggersByRoutineID(ctx, "rt-target"); err != nil {
		t.Fatalf("DeleteTriggersByRoutineID: %v", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_routine_triggers WHERE routine_id = 'rt-target'`); n != 0 {
		t.Errorf("triggers for rt-target = %d, want 0", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_routine_triggers WHERE routine_id = 'rt-keep'`); n != 1 {
		t.Errorf("triggers for rt-keep = %d, want 1", n)
	}
	// Deleting triggers must not touch the routine's runs.
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = 'rt-target'`); n != 1 {
		t.Errorf("routine runs for rt-target = %d, want 1", n)
	}

	if err := repo.DeleteRunsByRoutineID(ctx, "rt-target"); err != nil {
		t.Fatalf("DeleteRunsByRoutineID: %v", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = 'rt-target'`); n != 0 {
		t.Errorf("routine runs for rt-target = %d, want 0", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = 'rt-keep'`); n != 1 {
		t.Errorf("routine runs for rt-keep = %d, want 1", n)
	}
	// The routine row itself is not this method's business.
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_routines WHERE id = 'rt-target'`); n != 1 {
		t.Errorf("routine rows = %d, want the routine itself preserved", n)
	}

	// Unknown routine ids are no-ops.
	if err := repo.DeleteTriggersByRoutineID(ctx, "rt-ghost"); err != nil {
		t.Errorf("DeleteTriggersByRoutineID(ghost) = %v, want nil", err)
	}
	if err := repo.DeleteRunsByRoutineID(ctx, "rt-ghost"); err != nil {
		t.Errorf("DeleteRunsByRoutineID(ghost) = %v, want nil", err)
	}
}

func seedBudgetPolicy(t *testing.T, repo *sqlite.Repository, id, scopeType, scopeID string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO office_budget_policies
			(id, workspace_id, scope_type, scope_id, limit_subcents, period, created_at, updated_at)
		VALUES (?, 'ws-1', ?, ?, 1000, 'monthly', datetime('now'), datetime('now'))
	`, id, scopeType, scopeID); err != nil {
		t.Fatalf("seed budget policy %s: %v", id, err)
	}
}

func TestDeleteBudgetPoliciesForRemovedScopes(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedBudgetPolicy(t, repo, "bp-agent-live", "agent", "agent-live")
	seedBudgetPolicy(t, repo, "bp-agent-gone", "agent", "agent-gone")
	seedBudgetPolicy(t, repo, "bp-agent-gone2", "agent", "agent-gone-2")
	seedBudgetPolicy(t, repo, "bp-project", "project", "proj-1")

	removed, err := repo.DeleteBudgetPoliciesForRemovedScopes(ctx, "agent", []string{"agent-live"})
	if err != nil {
		t.Fatalf("DeleteBudgetPoliciesForRemovedScopes: %v", err)
	}
	if removed != 2 {
		t.Errorf("rows affected = %d, want 2", removed)
	}
	// Assert identity, not just the count: deleting the wrong two rows
	// leaves the same total behind.
	if got := scopeIDs(t, repo, "agent"); strings.Join(got, ",") != "agent-live" {
		t.Errorf("surviving agent scopes = %v, want only agent-live", got)
	}
	// A different scope_type is out of scope for this call.
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_budget_policies WHERE scope_type = 'project'`); n != 1 {
		t.Errorf("project policies = %d, want 1 (untouched)", n)
	}

	// Every id still valid → nothing to delete.
	removed, err = repo.DeleteBudgetPoliciesForRemovedScopes(ctx, "agent", []string{"agent-live"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if removed != 0 {
		t.Errorf("rows affected = %d, want 0", removed)
	}

	// An empty valid-id list wipes the whole scope type.
	removed, err = repo.DeleteBudgetPoliciesForRemovedScopes(ctx, "agent", nil)
	if err != nil {
		t.Fatalf("empty valid ids: %v", err)
	}
	if removed != 1 {
		t.Errorf("rows affected = %d, want 1", removed)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_budget_policies WHERE scope_type = 'agent'`); n != 0 {
		t.Errorf("agent policies = %d, want 0", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_budget_policies WHERE scope_type = 'project'`); n != 1 {
		t.Errorf("project policies = %d, want the other scope type preserved", n)
	}
}

func seedChannel(t *testing.T, repo *sqlite.Repository, id, agentID string) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `
		INSERT INTO office_channels
			(id, workspace_id, agent_profile_id, platform, created_at, updated_at)
		VALUES (?, 'ws-1', ?, 'slack', datetime('now'), datetime('now'))
	`, id, agentID); err != nil {
		t.Fatalf("seed channel %s: %v", id, err)
	}
}

func TestDeleteChannelsForRemovedAgents(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	seedChannel(t, repo, "ch-live", "agent-live")
	seedChannel(t, repo, "ch-live-2", "agent-live")
	seedChannel(t, repo, "ch-gone", "agent-gone")

	removed, err := repo.DeleteChannelsForRemovedAgents(ctx, []string{"agent-live"})
	if err != nil {
		t.Fatalf("DeleteChannelsForRemovedAgents: %v", err)
	}
	if removed != 1 {
		t.Errorf("rows affected = %d, want 1", removed)
	}
	// Both of the live agent's channels must survive and the removed
	// agent's must be gone — a count of 2 alone does not say which.
	if got := channelIDs(t, repo); strings.Join(got, ",") != "ch-live,ch-live-2" {
		t.Errorf("surviving channels = %v, want ch-live,ch-live-2", got)
	}

	// Nothing left to remove.
	removed, err = repo.DeleteChannelsForRemovedAgents(ctx, []string{"agent-live"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if removed != 0 {
		t.Errorf("rows affected = %d, want 0", removed)
	}

	// An empty valid-agent list wipes every channel.
	removed, err = repo.DeleteChannelsForRemovedAgents(ctx, nil)
	if err != nil {
		t.Fatalf("empty valid ids: %v", err)
	}
	if removed != 2 {
		t.Errorf("rows affected = %d, want 2", removed)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM office_channels`); n != 0 {
		t.Errorf("channels = %d, want 0", n)
	}
}

// scopeIDs returns the surviving budget-policy scope ids for a scope type,
// sorted, so tests can assert which rows a cleanup preserved.
func scopeIDs(t *testing.T, repo *sqlite.Repository, scopeType string) []string {
	t.Helper()
	var ids []string
	if err := repo.ReaderDB().Select(&ids,
		`SELECT scope_id FROM office_budget_policies WHERE scope_type = ? ORDER BY scope_id`,
		scopeType); err != nil {
		t.Fatalf("select scope ids: %v", err)
	}
	return ids
}

// channelIDs returns the surviving channel ids, sorted.
func channelIDs(t *testing.T, repo *sqlite.Repository) []string {
	t.Helper()
	var ids []string
	if err := repo.ReaderDB().Select(&ids,
		`SELECT id FROM office_channels ORDER BY id`); err != nil {
		t.Fatalf("select channel ids: %v", err)
	}
	return ids
}

func countRows(t *testing.T, repo *sqlite.Repository, query string) int {
	t.Helper()
	var n int
	if err := repo.ReaderDB().Get(&n, query); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}
