package config

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// These tests are the regression coverage for the full-row read-modify-write
// bug: applyAgents/applySkills/applyRoutines/applyProjects used to read a
// full row via List*, mutate only the fields the import owns on that struct,
// and hand the *whole* struct back to a full-row Update*. Any column that a
// genuinely concurrent writer (UpdateAgentStatusFields, the agent runtime,
// direct API edits) changed between the List read and the Update write was
// silently reverted to its stale snapshot value.
//
// Each apply* function calls List* exactly once, then loops over the
// incoming bundle entries and issues one Update* per matched row, in bundle
// order. That gives a real window between the single List call and the
// Update call for any row after the first: a write landing in that window is
// exactly what production concurrency looks like (a budget-cap pause, a
// direct API edit, another import) racing an in-flight import.
//
// Each test below drives that window through ConfigService.ApplyImport
// itself (not the repository methods directly - those already have their
// own coverage in internal/office/repository/sqlite). A bundle with two
// entities puts the first entity's Update* first in the loop; a SQL trigger
// fires when that first Update* statement runs and, in response, writes an
// unowned column on the *second* entity - landing strictly between the
// single List call (which read the second entity's pre-write state) and the
// second entity's own Update* call later in the same loop. That is the
// production race window, reproduced deterministically instead of with
// goroutines and a timing hope.
//
// Before the fix, the second entity's Update* call used the row List*
// captured before the trigger fired - the mutated-in-place stale struct -
// and a full-row Update* silently reverted the trigger's write. After the
// fix, Update*ConfigFields never references the unowned column at the SQL
// level, so staleness of the rest of the struct cannot matter.

// installRaceTrigger creates a same-table trigger that fires when the row
// identified by triggerID is updated, and in response writes value into
// column on the row identified by targetID. Used to land a "concurrent"
// write strictly inside an apply* function's List-once-then-loop window: the
// trigger fires off the *first* entity's Update* statement and mutates the
// *second* entity's unowned column before the loop reaches that row's own
// Update* call.
func installRaceTrigger(t *testing.T, e *testEnv, table, triggerID, targetID, column, value string) {
	t.Helper()
	stmt := fmt.Sprintf(`
		CREATE TRIGGER race_%[1]s
		AFTER UPDATE ON %[1]s
		WHEN NEW.id = %[2]s
		BEGIN
			UPDATE %[1]s SET %[3]s = %[4]s WHERE id = %[5]s;
		END;
	`, table, sqlLit(triggerID), column, sqlLit(value), sqlLit(targetID))
	if _, err := e.db.Exec(stmt); err != nil {
		t.Fatalf("install race trigger on %s: %v", table, err)
	}
}

// installReportsToRaceTrigger lands a status update after the reports_to
// resolver reads all agents but before it updates a later agent. The trigger
// does not fire during the first config-field pass because that pass does not
// include reports_to in its UPDATE statement.
func installReportsToRaceTrigger(t *testing.T, e *testEnv, triggerID, targetID string) {
	t.Helper()
	stmt := fmt.Sprintf(`
		CREATE TRIGGER race_agent_reports_to
		AFTER UPDATE OF reports_to ON agent_profiles
		WHEN NEW.id = %[1]s
		BEGIN
			UPDATE agent_profiles SET status = 'paused' WHERE id = %[2]s;
		END;
	`, sqlLit(triggerID), sqlLit(targetID))
	if _, err := e.db.Exec(stmt); err != nil {
		t.Fatalf("install reports_to race trigger: %v", err)
	}
}

// sqlLit renders s as a single-quoted SQL string literal. Trigger bodies
// cannot take bind parameters, so the id/value operands have to be spliced
// into the statement text; every value here comes from uuid.New().String()
// or a test-chosen literal, but the escaping is done properly anyway rather
// than assumed safe.
func sqlLit(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// TestApplyImport_Agents_PreservesConcurrentWriteBetweenListAndUpdate
// reproduces the triage receipt: a budget-cap pause (UpdateAgentStatusFields,
// the real concurrent writer used by internal/office/costs/budgets.go)
// landing between applyAgents' single List call and its Update call for a
// later row must survive, instead of being reverted to the stale "idle"
// status that List captured before the write happened.
func TestApplyImport_Agents_PreservesConcurrentWriteBetweenListAndUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	first := seedAgent(t, e, testWorkspaceID, "alpha")
	second := seedAgent(t, e, testWorkspaceID, "beta")

	installRaceTrigger(t, e, "agent_profiles", first.ID, second.ID, "status", "paused")

	bundle := &ConfigBundle{
		Agents: []AgentConfig{
			{Name: "alpha", Role: "cto", BudgetMonthlyCents: 100, MaxConcurrentSessions: 1},
			{Name: "beta", Role: "cfo", Icon: "💰", BudgetMonthlyCents: 200, MaxConcurrentSessions: 2},
		},
	}
	if _, err := e.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	got := agentByName(t, e, testWorkspaceID, "beta")
	assertEqual(t, "status survives concurrent write during import", string(got.Status), "paused")

	// The owned fields must still have landed - a scoped update that writes
	// nothing would also make the assertion above pass for the wrong reason.
	assertEqual(t, "role updated", string(got.Role), "cfo")
	assertEqual(t, "icon updated", got.Icon, "💰")
	assertEqual(t, "budget updated", got.BudgetMonthlyCents, 200)
	assertEqual(t, "max sessions updated", got.MaxConcurrentSessions, 2)
}

func TestApplyImport_Agents_PreservesConcurrentWriteDuringReportsToUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	first := seedAgent(t, e, testWorkspaceID, "alpha")
	second := seedAgent(t, e, testWorkspaceID, "beta")
	manager := seedAgent(t, e, testWorkspaceID, "lead")

	installReportsToRaceTrigger(t, e, first.ID, second.ID)

	bundle := &ConfigBundle{Agents: []AgentConfig{
		{Name: "alpha", Role: "engineer", ReportsTo: "lead", MaxConcurrentSessions: 1},
		{Name: "beta", Role: "engineer", ReportsTo: "lead", MaxConcurrentSessions: 1},
		{Name: "lead", Role: "cto", MaxConcurrentSessions: 1},
	}}
	if _, err := e.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	gotFirst := agentByName(t, e, testWorkspaceID, "alpha")
	gotSecond := agentByName(t, e, testWorkspaceID, "beta")
	assertEqual(t, "first reports_to updated", gotFirst.ReportsTo, manager.ID)
	assertEqual(t, "second reports_to updated", gotSecond.ReportsTo, manager.ID)
	assertEqual(t, "status survives concurrent write during reports_to update", string(gotSecond.Status), "paused")
}

// TestApplyImport_Skills_PreservesConcurrentWriteBetweenListAndUpdate mirrors
// the agent case for skills: approval_state is not a column applySkills
// owns, so a write landing between applyImport's List call and a later
// row's Update call must survive.
func TestApplyImport_Skills_PreservesConcurrentWriteBetweenListAndUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	first := seedSkill(t, e, testWorkspaceID, "alpha")
	second := seedSkill(t, e, testWorkspaceID, "beta")

	installRaceTrigger(t, e, "office_skills", first.ID, second.ID, "approval_state", "approved")

	bundle := &ConfigBundle{
		Skills: []SkillConfig{
			{Name: "Alpha", Slug: "alpha", Description: "first", SourceType: "inline", Content: "# Alpha\n"},
			{Name: "Beta", Slug: "beta", Description: "second", SourceType: "inline", Content: "# Beta\n"},
		},
	}
	if _, err := e.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	got := skillBySlug(t, e, testWorkspaceID, "beta")
	assertEqual(t, "approval_state survives concurrent write during import", string(got.ApprovalState), "approved")

	assertEqual(t, "name updated", got.Name, "Beta")
	assertEqual(t, "description updated", got.Description, "second")
	assertEqual(t, "content updated", got.Content, "# Beta\n")
}

// TestApplyImport_Routines_PreservesConcurrentWriteBetweenListAndUpdate
// mirrors the agent case for routines: status is not a column applyRoutines
// owns.
func TestApplyImport_Routines_PreservesConcurrentWriteBetweenListAndUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	first := seedRoutine(t, e, testWorkspaceID, "alpha")
	second := seedRoutine(t, e, testWorkspaceID, "beta")

	installRaceTrigger(t, e, "office_routines", first.ID, second.ID, "status", "paused")

	bundle := &ConfigBundle{
		Routines: []RoutineConfig{
			{Name: "alpha", Description: "first", TaskTemplate: "do the first thing", ConcurrencyPolicy: "skip"},
			{Name: "beta", Description: "second", TaskTemplate: "do the second thing", ConcurrencyPolicy: "always_create"},
		},
	}
	if _, err := e.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	got := routineByName(t, e, testWorkspaceID, "beta")
	assertEqual(t, "status survives concurrent write during import", got.Status, "paused")

	assertEqual(t, "description updated", got.Description, "second")
	assertEqual(t, "task template updated", got.TaskTemplate, "do the second thing")
	assertEqual(t, "concurrency policy updated", string(got.ConcurrencyPolicy), "always_create")
}

// TestApplyImport_Projects_PreservesConcurrentWriteBetweenListAndUpdate
// mirrors the agent case for projects: status is not a column applyProjects
// owns.
func TestApplyImport_Projects_PreservesConcurrentWriteBetweenListAndUpdate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	first := seedProject(t, e, testWorkspaceID, "alpha")
	second := seedProject(t, e, testWorkspaceID, "beta")

	installRaceTrigger(t, e, "office_projects", first.ID, second.ID, "status", "on_hold")

	bundle := &ConfigBundle{
		Projects: []ProjectConfig{
			{Name: "alpha", Description: "first", Color: "#ff0000", BudgetCents: 100},
			{Name: "beta", Description: "second", Color: "#00ff00", BudgetCents: 200, Repositories: `["kdlbs/other"]`},
		},
	}
	if _, err := e.svc.ApplyImport(ctx, testWorkspaceID, bundle); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	got := projectByName(t, e, testWorkspaceID, "beta")
	assertEqual(t, "status survives concurrent write during import", string(got.Status), "on_hold")

	assertEqual(t, "description updated", got.Description, "second")
	assertEqual(t, "color updated", got.Color, "#00ff00")
	assertEqual(t, "budget updated", got.BudgetCents, 200)
	assertEqual(t, "repositories updated", got.Repositories, `["kdlbs/other"]`)
}
