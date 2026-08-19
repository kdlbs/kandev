package config

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// These tests are the regression coverage for the full-row read-modify-write
// bug: applyAgents/applySkills/applyRoutines/applyProjects used to read a
// full row via List*, mutate only the fields the import owns on that struct,
// and hand the *whole* struct back to a full-row Update*. Any column that a
// genuinely concurrent writer (UpdateAgentStatusFields, the agent runtime,
// direct API edits) changed between the List read and the Update write was
// silently reverted to its stale snapshot value.
//
// Each test reproduces the exact production sequence: capture the row the
// way applyAgents' List call would, perform the concurrent write, then drive
// the import side of the sequence through the repository. Before the fix
// that last step was `Update<Entity>(ctx, stale)` — the mutated-in-place
// stale struct — which silently reverted the concurrent write. After the
// fix it is `Update<Entity>ConfigFields`, which never references the
// unowned column at the SQL level, so staleness of the rest of the struct
// cannot matter.

// TestUpdateAgentInstanceConfigFields_PreservesConcurrentStatusWrite
// reproduces the triage receipt: a budget-cap pause (UpdateAgentStatusFields,
// the real concurrent writer used by internal/office/costs/budgets.go)
// landing after the import's list read but before its write must survive,
// instead of being reverted to the stale "idle" status the import read.
func TestUpdateAgentInstanceConfigFields_PreservesConcurrentStatusWrite(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	seedAgent(t, e, testWorkspaceID, "ada")

	// What applyAgents' List call would have captured.
	snapshot, err := e.repo.ListAgentInstances(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ListAgentInstances: %v", err)
	}
	stale := snapshot[0]

	// A concurrent writer pauses the agent for a budget cap between the
	// import's read and its write.
	if err := e.repo.UpdateAgentStatusFields(ctx, stale.ID, "paused", "budget cap exceeded"); err != nil {
		t.Fatalf("UpdateAgentStatusFields: %v", err)
	}

	// The import proceeds with only the fields it owns (import.go's
	// applyAgents builds these from the incoming AgentConfig, not from the
	// stale struct, and only stale.ID identifies the row).
	fields := sqlite.AgentInstanceConfigFields{
		Role:                  "cto",
		Icon:                  "🤖",
		BudgetMonthlyCents:    4200,
		MaxConcurrentSessions: 3,
		DesiredSkills:         `["go"]`,
		ExecutorPreference:    "local_docker",
	}
	if err := e.repo.UpdateAgentInstanceConfigFields(ctx, stale.ID, fields); err != nil {
		t.Fatalf("UpdateAgentInstanceConfigFields: %v", err)
	}

	got := agentByName(t, e, testWorkspaceID, "ada")
	assertEqual(t, "status survives import", string(got.Status), "paused")
	assertEqual(t, "pause reason survives import", got.PauseReason, "budget cap exceeded")

	// The owned fields must still have landed - a scoped update that writes
	// nothing would also make the clobber assertions above pass for the
	// wrong reason.
	assertEqual(t, "role updated", string(got.Role), "cto")
	assertEqual(t, "icon updated", got.Icon, "🤖")
	assertEqual(t, "budget updated", got.BudgetMonthlyCents, 4200)
	assertEqual(t, "max sessions updated", got.MaxConcurrentSessions, 3)
	assertEqual(t, "desired skills updated", got.DesiredSkills, `["go"]`)
	assertEqual(t, "executor preference updated", got.ExecutorPreference, "local_docker")
}

// TestUpdateSkillConfigFields_PreservesConcurrentApprovalStateWrite mirrors
// the agent case for skills: approval_state is not a column applySkills
// owns, so a write landing between the import's read and its write must
// survive.
func TestUpdateSkillConfigFields_PreservesConcurrentApprovalStateWrite(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	seedSkill(t, e, testWorkspaceID, "code-review")

	snapshot, err := e.repo.ListSkills(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	stale := snapshot[0]

	if _, err := e.db.ExecContext(ctx,
		`UPDATE office_skills SET approval_state = ? WHERE id = ?`, "approved", stale.ID,
	); err != nil {
		t.Fatalf("concurrent approval_state write: %v", err)
	}

	fields := sqlite.SkillConfigFields{
		Name:        "Deep Review",
		Description: "reads everything",
		SourceType:  models.SkillSourceType("git"),
		Content:     "# Deep Review\n",
	}
	if err := e.repo.UpdateSkillConfigFields(ctx, stale.ID, fields); err != nil {
		t.Fatalf("UpdateSkillConfigFields: %v", err)
	}

	got := skillBySlug(t, e, testWorkspaceID, "code-review")
	assertEqual(t, "approval_state survives import", string(got.ApprovalState), "approved")

	assertEqual(t, "name updated", got.Name, "Deep Review")
	assertEqual(t, "description updated", got.Description, "reads everything")
	assertEqual(t, "source type updated", string(got.SourceType), "git")
	assertEqual(t, "content updated", got.Content, "# Deep Review\n")
}

// TestUpdateRoutineConfigFields_PreservesConcurrentStatusWrite mirrors the
// agent case for routines: status is not a column applyRoutines owns.
func TestUpdateRoutineConfigFields_PreservesConcurrentStatusWrite(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	seedRoutine(t, e, testWorkspaceID, "standup")

	snapshot, err := e.repo.ListRoutines(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ListRoutines: %v", err)
	}
	stale := snapshot[0]

	if _, err := e.db.ExecContext(ctx,
		`UPDATE office_routines SET status = ? WHERE id = ?`, "paused", stale.ID,
	); err != nil {
		t.Fatalf("concurrent status write: %v", err)
	}

	fields := sqlite.RoutineConfigFields{
		Description:       "weekly",
		TaskTemplate:      "summarise the week",
		ConcurrencyPolicy: models.RoutineConcurrencyPolicy("always_create"),
	}
	if err := e.repo.UpdateRoutineConfigFields(ctx, stale.ID, fields); err != nil {
		t.Fatalf("UpdateRoutineConfigFields: %v", err)
	}

	got := routineByName(t, e, testWorkspaceID, "standup")
	assertEqual(t, "status survives import", got.Status, "paused")

	assertEqual(t, "description updated", got.Description, "weekly")
	assertEqual(t, "task template updated", got.TaskTemplate, "summarise the week")
	assertEqual(t, "concurrency policy updated", string(got.ConcurrencyPolicy), "always_create")
}

// TestUpdateProjectConfigFields_PreservesConcurrentStatusWrite mirrors the
// agent case for projects: status is not a column applyProjects owns.
func TestUpdateProjectConfigFields_PreservesConcurrentStatusWrite(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	seedProject(t, e, testWorkspaceID, "apollo")

	snapshot, err := e.repo.ListProjects(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	stale := snapshot[0]

	if _, err := e.db.ExecContext(ctx,
		`UPDATE office_projects SET status = ? WHERE id = ?`, "on_hold", stale.ID,
	); err != nil {
		t.Fatalf("concurrent status write: %v", err)
	}

	fields := sqlite.ProjectConfigFields{
		Description:    "mars shot",
		Color:          "#00ff00",
		BudgetCents:    1234,
		Repositories:   `["kdlbs/other"]`,
		ExecutorConfig: `{"type":"local_pc"}`,
	}
	if err := e.repo.UpdateProjectConfigFields(ctx, stale.ID, fields); err != nil {
		t.Fatalf("UpdateProjectConfigFields: %v", err)
	}

	got := projectByName(t, e, testWorkspaceID, "apollo")
	assertEqual(t, "status survives import", string(got.Status), "on_hold")

	assertEqual(t, "description updated", got.Description, "mars shot")
	assertEqual(t, "color updated", got.Color, "#00ff00")
	assertEqual(t, "budget updated", got.BudgetCents, 1234)
	assertEqual(t, "repositories updated", got.Repositories, `["kdlbs/other"]`)
	assertEqual(t, "executor config updated", got.ExecutorConfig, `{"type":"local_pc"}`)
}
