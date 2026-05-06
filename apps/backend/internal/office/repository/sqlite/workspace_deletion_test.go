package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestDeleteWorkspaceDataDeletesOwnedOfficeRows(t *testing.T) {
	repo, db := newWorkspaceDeletionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedWorkspaceDeletionRows(t, repo, "ws-delete", now)
	seedWorkspaceDeletionRows(t, repo, "ws-keep", now)

	if err := repo.DeleteWorkspaceData(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspaceData: %v", err)
	}

	assertWorkspaceOfficeRows(t, db, "ws-delete", 0)
	assertWorkspaceOfficeRows(t, db, "ws-keep", 1)

	for _, table := range []string{
		"office_agent_memory",
		"office_agent_instructions",
		"office_agent_runtime",
		"runs",
		"office_routine_triggers",
		"office_routine_runs",
		"office_task_labels",
	} {
		assertTableCount(t, db, table, "ws-delete", 0)
		assertTableCount(t, db, table, "ws-keep", 1)
	}
}

func TestGetWorkspaceDeletionCounts(t *testing.T) {
	repo, _ := newWorkspaceDeletionRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	seedWorkspaceDeletionRows(t, repo, "ws-delete", now)
	seedWorkspaceDeletionRows(t, repo, "ws-keep", now)

	counts, err := repo.GetWorkspaceDeletionCounts(ctx, "ws-delete")
	if err != nil {
		t.Fatalf("GetWorkspaceDeletionCounts: %v", err)
	}

	if counts.Agents != 1 {
		t.Fatalf("agents = %d, want 1", counts.Agents)
	}
	if counts.Skills != 1 {
		t.Fatalf("skills = %d, want 1", counts.Skills)
	}
}

func newWorkspaceDeletionRepo(t *testing.T) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo, db
}

func seedWorkspaceDeletionRows(t *testing.T, repo *sqlite.Repository, workspaceID string, now time.Time) {
	t.Helper()
	agentID := workspaceID + "-agent"
	projectID := workspaceID + "-project"
	routineID := workspaceID + "-routine"
	labelID := workspaceID + "-label"

	// Office agents now live in agent_profiles (workspace_id != ''). The
	// seed inserts the minimum columns the merged schema requires.
	execRaw(t, repo, `INSERT INTO agent_profiles (
		id, agent_id, name, agent_display_name, model,
		created_at, updated_at,
		workspace_id, role
	) VALUES (?, '', ?, ?, '', ?, ?, ?, 'worker')`, agentID, agentID, agentID, now, now, workspaceID)
	execRaw(t, repo, `INSERT INTO office_skills (id, workspace_id, name, slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, workspaceID+"-skill", workspaceID, "Skill "+workspaceID, "skill-"+workspaceID, now, now)
	execRaw(t, repo, `INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, projectID, workspaceID, "Project "+workspaceID, now, now)
	execRaw(t, repo, `INSERT INTO office_agent_runtime (agent_id, status, updated_at) VALUES (?, 'running', ?)`, agentID, now)
	execRaw(t, repo, `INSERT INTO office_agent_memory (id, agent_profile_id, layer, key, created_at, updated_at) VALUES (?, ?, 'project', 'note', ?, ?)`, workspaceID+"-memory", agentID, now, now)
	execRaw(t, repo, `INSERT INTO office_agent_instructions (id, agent_profile_id, filename, content, created_at, updated_at) VALUES (?, ?, 'guide.md', 'content', ?, ?)`, workspaceID+"-instruction", agentID, now, now)
	execRaw(t, repo, `INSERT INTO runs (id, agent_profile_id, reason, requested_at) VALUES (?, ?, 'test', ?)`, workspaceID+"-run", agentID, now)
	execRaw(t, repo, `INSERT INTO office_cost_events (id, agent_profile_id, project_id, occurred_at, created_at) VALUES (?, ?, ?, ?, ?)`, workspaceID+"-cost", agentID, projectID, now, now)
	execRaw(t, repo, `INSERT INTO office_budget_policies (id, workspace_id, scope_type, scope_id, limit_subcents, period, created_at, updated_at) VALUES (?, ?, 'workspace', ?, 100, 'month', ?, ?)`, workspaceID+"-budget", workspaceID, workspaceID, now, now)
	execRaw(t, repo, `INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, routineID, workspaceID, "Routine "+workspaceID, now, now)
	execRaw(t, repo, `INSERT INTO office_routine_triggers (id, routine_id, kind, created_at, updated_at) VALUES (?, ?, 'manual', ?, ?)`, workspaceID+"-trigger", routineID, now, now)
	execRaw(t, repo, `INSERT INTO office_routine_runs (id, routine_id, source, created_at) VALUES (?, ?, 'manual', ?)`, workspaceID+"-run", routineID, now)
	execRaw(t, repo, `INSERT INTO office_labels (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, labelID, workspaceID, "Label "+workspaceID, now, now)
	execRaw(t, repo, `INSERT INTO office_task_labels (task_id, label_id, created_at) VALUES (?, ?, ?)`, workspaceID+"-task", labelID, now)
	execRaw(t, repo, `INSERT INTO office_channels (id, workspace_id, agent_profile_id, platform, created_at, updated_at) VALUES (?, ?, ?, 'slack', ?, ?)`, workspaceID+"-channel", workspaceID, agentID, now, now)
	execRaw(t, repo, `INSERT INTO office_approvals (id, workspace_id, type, created_at, updated_at) VALUES (?, ?, 'test', ?, ?)`, workspaceID+"-approval", workspaceID, now, now)
	execRaw(t, repo, `INSERT INTO office_activity_log (id, workspace_id, actor_type, actor_id, action, created_at) VALUES (?, ?, 'agent', ?, 'created', ?)`, workspaceID+"-activity", workspaceID, agentID, now)
	execRaw(t, repo, `INSERT INTO office_onboarding (workspace_id, completed, ceo_agent_id, created_at) VALUES (?, 1, ?, ?)`, workspaceID, agentID, now)
	execRaw(t, repo, `INSERT INTO office_workspace_governance (workspace_id, key, value) VALUES (?, 'require_approval_for_new_agents', 1)`, workspaceID)
}

func execRaw(t *testing.T, repo *sqlite.Repository, query string, args ...interface{}) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func assertWorkspaceOfficeRows(t *testing.T, db *sqlx.DB, workspaceID string, want int) {
	t.Helper()
	for _, table := range []string{
		"agent_profiles",
		"office_skills",
		"office_projects",
		"office_cost_events",
		"office_budget_policies",
		"office_routines",
		"office_labels",
		"office_channels",
		"office_approvals",
		"office_activity_log",
		"office_onboarding",
		"office_workspace_governance",
	} {
		assertTableCount(t, db, table, workspaceID, want)
	}
}

func assertTableCount(t *testing.T, db *sqlx.DB, table, workspaceID string, want int) {
	t.Helper()
	spec := workspaceDeletionTableSpec(table)
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE "+spec.column+" "+spec.operator+" ?", spec.value(workspaceID)).Scan(&count)
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count for %s = %d, want %d", table, workspaceID, count, want)
	}
}

type workspaceDeletionCountSpec struct {
	column   string
	operator string
	value    func(string) interface{}
}

func workspaceDeletionTableSpec(table string) workspaceDeletionCountSpec {
	likeID := func(workspaceID string) interface{} { return workspaceID + "-%" }
	eqWorkspace := func(workspaceID string) interface{} { return workspaceID }
	switch table {
	case "agent_profiles", "office_skills", "office_projects", "office_cost_events",
		"office_budget_policies", "office_routines", "office_labels", "office_channels",
		"office_approvals", "office_activity_log":
		return workspaceDeletionCountSpec{column: "id", operator: "LIKE", value: likeID}
	case "office_agent_memory", "office_agent_instructions", "runs":
		return workspaceDeletionCountSpec{column: "agent_profile_id", operator: "LIKE", value: likeID}
	case "office_agent_runtime":
		return workspaceDeletionCountSpec{column: "agent_id", operator: "LIKE", value: likeID}
	case "office_routine_triggers", "office_routine_runs":
		return workspaceDeletionCountSpec{column: "routine_id", operator: "LIKE", value: likeID}
	case "office_task_labels":
		return workspaceDeletionCountSpec{column: "label_id", operator: "LIKE", value: likeID}
	default:
		return workspaceDeletionCountSpec{column: "workspace_id", operator: "=", value: eqWorkspace}
	}
}
