package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// newScopeTestRepo adds the `tasks` table (owned by the task repository, not
// office) so WorkspaceIDForTask has something to read, then seeds one
// row per Office resource kind under workspaceID.
func newScopeTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	repo := newTestRepo(t)
	if _, err := repo.ExecRaw(context.Background(), `
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	return repo
}

// seedScopedResources inserts one row of every Office resource kind owned by
// workspaceID, suffixing every id with suffix so two workspaces can coexist.
// Raw INSERTs (not the typed Create* methods) keep the fixture to the columns
// the resolvers actually read.
func seedScopedResources(t *testing.T, repo *sqlite.Repository, workspaceID, suffix string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := repo.ExecRaw(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO agents (id, name, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"agent-"+suffix, "agent-"+suffix, workspaceID, now, now)
	exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		"agent-"+suffix, "agent-"+suffix, "n", "d", workspaceID, now, now)
	exec(`INSERT INTO tasks (id, workspace_id, title) VALUES (?, ?, ?)`, "task-"+suffix, workspaceID, "t")
	exec(`INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"routine-"+suffix, workspaceID, "r", now, now)
	exec(`INSERT INTO office_routine_triggers (id, routine_id, kind, public_id, created_at, updated_at)
		VALUES (?, ?, 'webhook', ?, ?, ?)`,
		"trigger-"+suffix, "routine-"+suffix, "public-"+suffix, now, now)
	exec(`INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"project-"+suffix, workspaceID, "p", now, now)
	exec(`INSERT INTO office_skills (id, workspace_id, name, slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"skill-"+suffix, workspaceID, "s", "s-"+suffix, now, now)
	exec(`INSERT INTO office_budget_policies
		(id, workspace_id, scope_type, scope_id, limit_subcents, period, created_at, updated_at)
		VALUES (?, ?, 'workspace', '', 100, 'monthly', ?, ?)`,
		"budget-"+suffix, workspaceID, now, now)
	exec(`INSERT INTO office_approvals (id, workspace_id, type, created_at, updated_at) VALUES (?, ?, 'hire_agent', ?, ?)`,
		"approval-"+suffix, workspaceID, now, now)
	exec(`INSERT INTO office_channels (id, workspace_id, agent_profile_id, platform, created_at, updated_at)
		VALUES (?, ?, ?, 'telegram', ?, ?)`,
		"channel-"+suffix, workspaceID, "agent-"+suffix, now, now)
	exec(`INSERT INTO office_labels (id, workspace_id, name) VALUES (?, ?, ?)`,
		"label-"+suffix, workspaceID, "l-"+suffix)
}

// scopeResolvers is every by-ID workspace resolver the Office HTTP scope
// middleware depends on, keyed by the id prefix seedScopedResources uses.
func scopeResolvers(repo *sqlite.Repository) map[string]func(context.Context, string) (string, error) {
	return map[string]func(context.Context, string) (string, error){
		"agent":    repo.WorkspaceIDForAgent,
		"task":     repo.WorkspaceIDForTask,
		"routine":  repo.WorkspaceIDForRoutine,
		"trigger":  repo.WorkspaceIDForRoutineTrigger,
		"public":   repo.WorkspaceIDForRoutineTriggerPublicID,
		"project":  repo.WorkspaceIDForProject,
		"skill":    repo.WorkspaceIDForSkill,
		"budget":   repo.WorkspaceIDForBudget,
		"approval": repo.WorkspaceIDForApproval,
		"channel":  repo.WorkspaceIDForChannel,
		"label":    repo.WorkspaceIDForLabel,
	}
}

// TestWorkspaceScopeResolvers_ResolveOwningWorkspace pins that every by-ID
// resolver returns the workspace that actually owns the row, and that a row
// owned by a *different* workspace never resolves to the first one — the
// property the HTTP guard's cross-user denial rests on.
func TestWorkspaceScopeResolvers_ResolveOwningWorkspace(t *testing.T) {
	repo := newScopeTestRepo(t)
	seedScopedResources(t, repo, "ws-a", "a")
	seedScopedResources(t, repo, "ws-b", "b")

	for kind, resolve := range scopeResolvers(repo) {
		for suffix, want := range map[string]string{"a": "ws-a", "b": "ws-b"} {
			got, err := resolve(context.Background(), kind+"-"+suffix)
			if err != nil {
				t.Errorf("%s-%s: %v", kind, suffix, err)
				continue
			}
			if got != want {
				t.Errorf("%s-%s resolved to %q, want %q", kind, suffix, got, want)
			}
		}
	}
}

// TestWorkspaceScopeResolvers_UnknownIDReturnsNoRows pins the sentinel the
// middleware fails closed on: an id that names nothing must be an error, not
// an empty string that AuthorizeWorkspaceAccess would read as "unscoped".
func TestWorkspaceScopeResolvers_UnknownIDReturnsNoRows(t *testing.T) {
	repo := newScopeTestRepo(t)

	for kind, resolve := range scopeResolvers(repo) {
		got, err := resolve(context.Background(), kind+"-missing")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("%s unknown id: err = %v (workspace %q), want sql.ErrNoRows", kind, err, got)
		}
	}
}

// TestWorkspaceIDForAgent_ResolvesSoftDeletedAgent mirrors GetRunWorkspaceID's
// documented reason for a raw read: filtering deleted_at would make a
// soft-deleted agent permanently unresolvable, and so permanently denied to
// its own owner.
func TestWorkspaceIDForAgent_ResolvesSoftDeletedAgent(t *testing.T) {
	repo := newScopeTestRepo(t)
	seedScopedResources(t, repo, "ws-a", "a")
	if _, err := repo.ExecRaw(context.Background(),
		`UPDATE agent_profiles SET deleted_at = ? WHERE id = ?`, time.Now().UTC(), "agent-a"); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	got, err := repo.WorkspaceIDForAgent(context.Background(), "agent-a")
	if err != nil {
		t.Fatalf("WorkspaceIDForAgent(soft-deleted) = %v, want nil", err)
	}
	if got != "ws-a" {
		t.Errorf("workspace = %q, want %q", got, "ws-a")
	}
}
