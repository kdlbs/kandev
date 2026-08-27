package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func intPtr(v int) *int { return &v }

// fullAgentInstance builds an office agent with every column the office
// INSERT writes set to a distinct non-default value.
func fullAgentInstance(id, workspaceID, name string) *models.AgentInstance {
	lastRun := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	return &models.AgentInstance{
		ID:                         id,
		AgentID:                    "cli-claude",
		WorkspaceID:                workspaceID,
		Name:                       name,
		AgentDisplayName:           name + " (display)",
		Model:                      "claude-opus-5",
		Mode:                       "architect",
		AutoApprove:                true,
		DangerouslySkipPermissions: true,
		AllowIndexing:              true,
		CLIPassthrough:             true,
		UserModified:               true,
		Role:                       settingsmodels.AgentRole("cto"),
		Icon:                       "🤖",
		Status:                     settingsmodels.AgentStatus("paused"),
		ReportsTo:                  "agent-ceo",
		Permissions:                `{"hire_agents":true}`,
		BudgetMonthlyCents:         4200,
		MaxConcurrentSessions:      3,
		CooldownSec:                90,
		SkipIdleRuns:               true,
		LastRunFinishedAt:          &lastRun,
		SkillIDs:                   `["skill-a","skill-b"]`,
		DesiredSkills:              `["go","sql"]`,
		CustomPrompt:               "be terse",
		ExecutorPreference:         "local_docker",
		PauseReason:                "cooling off",
		ConsecutiveFailures:        2,
		FailureThreshold:           intPtr(7),
		Settings:                   `{"routing":{"tier_source":"explicit"}}`,
	}
}

// assertAgentProjection compares every field the office SELECT projection
// surfaces. A column dropped from either the INSERT or the projection
// fails here rather than silently reading back a COALESCE default.
func assertAgentProjection(t *testing.T, got, want *models.AgentInstance) {
	t.Helper()
	checks := []struct {
		field    string
		got, exp interface{}
	}{
		{"ID", got.ID, want.ID},
		{"AgentID", got.AgentID, want.AgentID},
		{"WorkspaceID", got.WorkspaceID, want.WorkspaceID},
		{"Name", got.Name, want.Name},
		{"AgentDisplayName", got.AgentDisplayName, want.AgentDisplayName},
		{"Model", got.Model, want.Model},
		{"AutoApprove", got.AutoApprove, want.AutoApprove},
		{"AllowIndexing", got.AllowIndexing, want.AllowIndexing},
		{"CLIPassthrough", got.CLIPassthrough, want.CLIPassthrough},
		{"Role", got.Role, want.Role},
		{"Icon", got.Icon, want.Icon},
		{"Status", got.Status, want.Status},
		{"ReportsTo", got.ReportsTo, want.ReportsTo},
		{"Permissions", got.Permissions, want.Permissions},
		{"BudgetMonthlyCents", got.BudgetMonthlyCents, want.BudgetMonthlyCents},
		{"MaxConcurrentSessions", got.MaxConcurrentSessions, want.MaxConcurrentSessions},
		{"CooldownSec", got.CooldownSec, want.CooldownSec},
		{"SkipIdleRuns", got.SkipIdleRuns, want.SkipIdleRuns},
		{"SkillIDs", got.SkillIDs, want.SkillIDs},
		{"DesiredSkills", got.DesiredSkills, want.DesiredSkills},
		{"ExecutorPreference", got.ExecutorPreference, want.ExecutorPreference},
		{"PauseReason", got.PauseReason, want.PauseReason},
		{"ConsecutiveFailures", got.ConsecutiveFailures, want.ConsecutiveFailures},
	}
	for _, c := range checks {
		if c.got != c.exp {
			t.Errorf("%s = %v, want %v", c.field, c.got, c.exp)
		}
	}
	switch {
	case want.FailureThreshold == nil && got.FailureThreshold != nil:
		t.Errorf("FailureThreshold = %d, want nil", *got.FailureThreshold)
	case want.FailureThreshold != nil && got.FailureThreshold == nil:
		t.Errorf("FailureThreshold = nil, want %d", *want.FailureThreshold)
	case want.FailureThreshold != nil && *got.FailureThreshold != *want.FailureThreshold:
		t.Errorf("FailureThreshold = %d, want %d", *got.FailureThreshold, *want.FailureThreshold)
	}
	switch {
	case want.LastRunFinishedAt == nil && got.LastRunFinishedAt != nil:
		t.Errorf("LastRunFinishedAt = %v, want nil", *got.LastRunFinishedAt)
	case want.LastRunFinishedAt != nil && got.LastRunFinishedAt == nil:
		t.Error("LastRunFinishedAt = nil, want a timestamp")
	case want.LastRunFinishedAt != nil && !got.LastRunFinishedAt.Equal(*want.LastRunFinishedAt):
		t.Errorf("LastRunFinishedAt = %v, want %v", *got.LastRunFinishedAt, *want.LastRunFinishedAt)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

func TestCreateAgentInstance_RoundTripsEveryProjectedField(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullAgentInstance("agent-full", "ws-1", "Ada")
	if err := repo.CreateAgentInstance(ctx, want); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "agent-full")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	assertAgentProjection(t, got, want)

	// CreateAgentInstance writes settings as the literal '{}' — the
	// struct value is persisted by UpdateAgentSettings / UpdateAgentInstance.
	if got.Settings != "{}" {
		t.Errorf("Settings = %q, want {} on create", got.Settings)
	}

	// Columns written by the INSERT but absent from the office projection
	// still have to land, so read them straight from the row.
	var row struct {
		Mode                       string `db:"mode"`
		CustomPrompt               string `db:"custom_prompt"`
		DangerouslySkipPermissions int    `db:"dangerously_skip_permissions"`
		UserModified               int    `db:"user_modified"`
	}
	if err := repo.ReaderDB().Get(&row, `SELECT COALESCE(mode,'') AS mode,
		COALESCE(custom_prompt,'') AS custom_prompt,
		dangerously_skip_permissions, user_modified
		FROM agent_profiles WHERE id = 'agent-full'`); err != nil {
		t.Fatalf("read unprojected columns: %v", err)
	}
	if row.Mode != "architect" {
		t.Errorf("mode = %q, want architect", row.Mode)
	}
	if row.CustomPrompt != "be terse" {
		t.Errorf("custom_prompt = %q, want 'be terse'", row.CustomPrompt)
	}
	if row.DangerouslySkipPermissions != 1 {
		t.Errorf("dangerously_skip_permissions = %d, want 1", row.DangerouslySkipPermissions)
	}
	if row.UserModified != 1 {
		t.Errorf("user_modified = %d, want 1", row.UserModified)
	}
}

func TestCreateAgentInstance_NormalisesDefaults(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Minimal"}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}
	if agent.ID == "" {
		t.Fatal("CreateAgentInstance left ID empty")
	}

	got, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Status != settingsmodels.AgentStatus("idle") {
		t.Errorf("Status = %q, want the idle default", got.Status)
	}
	if got.AgentDisplayName != "Minimal" {
		t.Errorf("AgentDisplayName = %q, want it defaulted to Name", got.AgentDisplayName)
	}
	if got.SkillIDs != "[]" {
		t.Errorf("SkillIDs = %q, want []", got.SkillIDs)
	}
	if got.DesiredSkills != "[]" {
		t.Errorf("DesiredSkills = %q, want []", got.DesiredSkills)
	}
	if got.Permissions != "{}" {
		t.Errorf("Permissions = %q, want {}", got.Permissions)
	}
	if got.FailureThreshold != nil {
		t.Errorf("FailureThreshold = %d, want nil (0 in the column means 'inherit')", *got.FailureThreshold)
	}
	if got.LastRunFinishedAt != nil {
		t.Errorf("LastRunFinishedAt = %v, want nil", *got.LastRunFinishedAt)
	}
	if got.MaxConcurrentSessions != 0 {
		t.Errorf("MaxConcurrentSessions = %d, want the inserted 0", got.MaxConcurrentSessions)
	}
}

func TestCreateAgentInstance_RequiresWorkspace(t *testing.T) {
	repo := newTestRepo(t)

	err := repo.CreateAgentInstance(context.Background(), &models.AgentInstance{Name: "Orphan"})
	if err == nil {
		t.Fatal("CreateAgentInstance without a workspace = nil error, want a rejection")
	}
	if !strings.Contains(err.Error(), "workspace_id is required") {
		t.Errorf("error = %q, want it to name the missing workspace_id", err)
	}
}

// The agent_id column is a FK onto the CLI-tool registrations, so the
// inheritance fallbacks in CreateAgentInstance are load-bearing under FK
// enforcement. Reuses the FK-enabled repo helper for exactly that reason.
func TestCreateAgentInstance_InheritsAgentIDUnderForeignKeys(t *testing.T) {
	repo, db := newRouteAttemptsRepoWithFK(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('cli-first', 'First', ?, ?)`,
		now, now); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES ('cli-second', 'Second', ?, ?)`,
		now.Add(time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatalf("seed second agents row: %v", err)
	}

	// No workspace agent exists yet → falls back to the oldest CLI tool.
	seeded := &models.AgentInstance{WorkspaceID: "ws-fk", Name: "Seeder"}
	if err := repo.CreateAgentInstance(ctx, seeded); err != nil {
		t.Fatalf("CreateAgentInstance (CLI fallback): %v", err)
	}
	if seeded.AgentID != "cli-first" {
		t.Errorf("AgentID = %q, want cli-first (oldest registered CLI tool)", seeded.AgentID)
	}

	// A later agent in the same workspace inherits from the existing profile.
	if _, err := db.Exec(
		`UPDATE agent_profiles SET agent_id = 'cli-second' WHERE id = ?`, seeded.ID); err != nil {
		t.Fatalf("repoint seeded profile: %v", err)
	}
	inheritor := &models.AgentInstance{WorkspaceID: "ws-fk", Name: "Inheritor"}
	if err := repo.CreateAgentInstance(ctx, inheritor); err != nil {
		t.Fatalf("CreateAgentInstance (workspace inherit): %v", err)
	}
	if inheritor.AgentID != "cli-second" {
		t.Errorf("AgentID = %q, want cli-second inherited from the workspace peer", inheritor.AgentID)
	}

	// Deleting the CLI registration cascades the profiles away.
	if _, err := db.Exec(`DELETE FROM agents WHERE id = 'cli-second'`); err != nil {
		t.Fatalf("delete agents row: %v", err)
	}
	remaining, err := repo.ListAgentInstances(ctx, "ws-fk")
	if err != nil {
		t.Fatalf("ListAgentInstances: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("agents after cascade = %d, want 0: %+v", len(remaining), remaining)
	}
}

func TestGetAgentInstance_NotFoundAndOfficeScoping(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("agent-1", "ws-1", "Ada")); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}
	// A shallow kanban profile (workspace_id = '') is invisible to office reads.
	seedAgentProfile(t, db, "kanban-profile", "")

	got, err := repo.GetAgentInstance(ctx, "missing")
	if err == nil {
		t.Fatal("GetAgentInstance(missing) = nil error, want not-found")
	}
	if !strings.Contains(err.Error(), "agent instance not found: missing") {
		t.Errorf("error = %q, want it to name the missing agent", err)
	}
	if got != nil && got.ID != "" {
		t.Errorf("GetAgentInstance(missing) returned %+v", got)
	}

	if _, err := repo.GetAgentInstance(ctx, "kanban-profile"); err == nil {
		t.Error("GetAgentInstance returned a workspace-less profile, want it hidden from office reads")
	}

	// A soft-deleted row is also invisible.
	if err := repo.DeleteAgentInstance(ctx, "agent-1"); err != nil {
		t.Fatalf("DeleteAgentInstance: %v", err)
	}
	if _, err := repo.GetAgentInstance(ctx, "agent-1"); err == nil {
		t.Error("GetAgentInstance returned a soft-deleted agent")
	}
	var deletedRows int
	if err := db.Get(&deletedRows,
		`SELECT COUNT(*) FROM agent_profiles WHERE id = 'agent-1' AND deleted_at IS NOT NULL`); err != nil {
		t.Fatalf("count soft-deleted rows: %v", err)
	}
	if deletedRows != 1 {
		t.Errorf("soft-deleted rows = %d, want the audit row preserved", deletedRows)
	}
}

func TestGetAgentInstanceByName(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullAgentInstance("a-1", "ws-a", "Shared")
	if err := repo.CreateAgentInstance(ctx, want); err != nil {
		t.Fatalf("create ws-a agent: %v", err)
	}
	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("a-2", "ws-b", "Shared")); err != nil {
		t.Fatalf("create ws-b agent: %v", err)
	}

	got, err := repo.GetAgentInstanceByName(ctx, "ws-a", "Shared")
	if err != nil {
		t.Fatalf("GetAgentInstanceByName: %v", err)
	}
	assertAgentProjection(t, got, want)

	if _, err := repo.GetAgentInstanceByName(ctx, "ws-a", "Nobody"); err == nil {
		t.Error("GetAgentInstanceByName(missing) = nil error, want not-found")
	} else if !strings.Contains(err.Error(), "agent instance not found: Nobody") {
		t.Errorf("error = %q, want it to name the agent", err)
	}
}

func TestGetAgentInstanceByNameAny(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("a-1", "ws-b", "Unique")); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	got, err := repo.GetAgentInstanceByNameAny(ctx, "Unique")
	if err != nil {
		t.Fatalf("GetAgentInstanceByNameAny: %v", err)
	}
	if got.ID != "a-1" || got.WorkspaceID != "ws-b" {
		t.Errorf("got %q in %q, want a-1 in ws-b", got.ID, got.WorkspaceID)
	}

	if _, err := repo.GetAgentInstanceByNameAny(ctx, "Nobody"); err == nil {
		t.Error("GetAgentInstanceByNameAny(missing) = nil error, want not-found")
	}
}

func TestListAgentInstances_ScopeAndOrdering(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	for i, spec := range []struct{ id, ws, name string }{
		{"l-1", "ws-1", "First"},
		{"l-2", "ws-1", "Second"},
		{"l-3", "ws-2", "Third"},
	} {
		agent := fullAgentInstance(spec.id, spec.ws, spec.name)
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create %s: %v", spec.id, err)
		}
		// Force a deterministic created_at ordering (reverse of insert order).
		stamp := time.Date(2026, 1, 10-i, 0, 0, 0, 0, time.UTC)
		if _, err := db.Exec(`UPDATE agent_profiles SET created_at = ? WHERE id = ?`, stamp, spec.id); err != nil {
			t.Fatalf("stamp %s: %v", spec.id, err)
		}
	}
	seedAgentProfile(t, db, "kanban", "")

	scoped, err := repo.ListAgentInstances(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListAgentInstances(ws-1): %v", err)
	}
	if len(scoped) != 2 {
		t.Fatalf("ws-1 agents = %d, want 2", len(scoped))
	}
	if scoped[0].ID != "l-2" || scoped[1].ID != "l-1" {
		t.Errorf("order = %q,%q, want l-2,l-1 (created_at ascending)", scoped[0].ID, scoped[1].ID)
	}

	all, err := repo.ListAgentInstances(ctx, "")
	if err != nil {
		t.Fatalf("ListAgentInstances(all): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all agents = %d, want 3 (workspace-less profile excluded): %+v", len(all), all)
	}
	for _, a := range all {
		if a.ID == "kanban" {
			t.Fatal("workspace-less profile leaked into the office list")
		}
	}

	empty, err := repo.ListAgentInstances(ctx, "ws-nothing")
	if err != nil {
		t.Fatalf("ListAgentInstances(empty): %v", err)
	}
	if empty == nil {
		t.Fatal("returned nil slice, want empty non-nil")
	}
	if len(empty) != 0 {
		t.Errorf("len = %d, want 0", len(empty))
	}
}

func TestUpdateAgentInstance_PersistsEveryMutableField(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := &models.AgentInstance{ID: "u-1", WorkspaceID: "ws-1", Name: "Before"}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}
	createdAt := agent.CreatedAt

	lastRun := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	agent.Name = "After"
	agent.Role = settingsmodels.AgentRole("engineer")
	agent.Icon = "🛠"
	agent.Status = settingsmodels.AgentStatus("working")
	agent.ReportsTo = "agent-boss"
	agent.Permissions = `{"approve_budget":true}`
	agent.BudgetMonthlyCents = 5150
	agent.MaxConcurrentSessions = 4
	agent.CooldownSec = 30
	agent.SkipIdleRuns = true
	agent.LastRunFinishedAt = &lastRun
	agent.SkillIDs = `["skill-x"]`
	agent.DesiredSkills = `["rust"]`
	agent.ExecutorPreference = "sprites"
	agent.PauseReason = "waiting on review"
	agent.FailureThreshold = intPtr(11)
	agent.Settings = `{"routing":{"tier_source":"inherit"}}`
	agent.AutoApprove = true
	agent.AllowIndexing = true
	agent.CLIPassthrough = true
	if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("UpdateAgentInstance: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "u-1")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	// agent_display_name is not in the office UPDATE — it stays at the value
	// CreateAgentInstance defaulted from Name.
	agent.AgentDisplayName = "Before"
	assertAgentProjection(t, got, agent)
	if got.Settings != `{"routing":{"tier_source":"inherit"}}` {
		t.Errorf("Settings = %q, want the updated JSON", got.Settings)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt moved to %v, want %v", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.After(createdAt) && !got.UpdatedAt.Equal(createdAt) {
		t.Errorf("UpdatedAt = %v, want >= CreatedAt", got.UpdatedAt)
	}
}

func TestUpdateAgentInstance_NormalisesEmptyJSONAndStatus(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	agent := fullAgentInstance("u-2", "ws-1", "Normalise")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	agent.Status = ""
	agent.Settings = ""
	agent.Permissions = "  "
	agent.SkillIDs = ""
	agent.DesiredSkills = ""
	agent.FailureThreshold = nil
	if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("UpdateAgentInstance: %v", err)
	}

	got, err := repo.GetAgentInstance(ctx, "u-2")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Status != settingsmodels.AgentStatus("idle") {
		t.Errorf("Status = %q, want idle", got.Status)
	}
	if got.Settings != "{}" {
		t.Errorf("Settings = %q, want {}", got.Settings)
	}
	if got.Permissions != "{}" {
		t.Errorf("Permissions = %q, want {}", got.Permissions)
	}
	if got.SkillIDs != "[]" || got.DesiredSkills != "[]" {
		t.Errorf("skill arrays = %q / %q, want [] / []", got.SkillIDs, got.DesiredSkills)
	}
	if got.FailureThreshold != nil {
		t.Errorf("FailureThreshold = %d, want nil after clearing the override", *got.FailureThreshold)
	}
}

func TestUpdateAgentInstance_SkipsDeletedAndForeignRows(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	agent := fullAgentInstance("u-3", "ws-1", "Doomed")
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}
	if err := repo.DeleteAgentInstance(ctx, "u-3"); err != nil {
		t.Fatalf("DeleteAgentInstance: %v", err)
	}

	agent.Name = "Renamed after delete"
	if err := repo.UpdateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("UpdateAgentInstance: %v", err)
	}
	var name string
	if err := db.Get(&name, `SELECT name FROM agent_profiles WHERE id = 'u-3'`); err != nil {
		t.Fatalf("read name: %v", err)
	}
	if name != "Doomed" {
		t.Errorf("name = %q, want the soft-deleted row left untouched", name)
	}
}

func TestUpdateAgentSettings(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("s-1", "ws-1", "Settings")); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	if err := repo.UpdateAgentSettings(ctx, "s-1", `{"routing":{"provider_order_source":"explicit"}}`); err != nil {
		t.Fatalf("UpdateAgentSettings: %v", err)
	}
	got, err := repo.GetAgentInstance(ctx, "s-1")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Settings != `{"routing":{"provider_order_source":"explicit"}}` {
		t.Errorf("Settings = %q, want the written JSON", got.Settings)
	}
	// Other columns must be untouched.
	if got.Name != "Settings" || got.Role != settingsmodels.AgentRole("cto") {
		t.Errorf("UpdateAgentSettings changed unrelated fields: name=%q role=%q", got.Name, got.Role)
	}

	if err := repo.UpdateAgentSettings(ctx, "s-1", ""); err != nil {
		t.Fatalf("UpdateAgentSettings(empty): %v", err)
	}
	if got, err = repo.GetAgentInstance(ctx, "s-1"); err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Settings != "{}" {
		t.Errorf("Settings = %q, want the empty string normalised to {}", got.Settings)
	}
}

func TestUpdateAgentStatusFields(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("st-1", "ws-1", "Status")); err != nil {
		t.Fatalf("CreateAgentInstance: %v", err)
	}

	if err := repo.UpdateAgentStatusFields(ctx, "st-1", "paused", "Auto-paused: 3 failures"); err != nil {
		t.Fatalf("UpdateAgentStatusFields: %v", err)
	}
	got, err := repo.GetAgentInstance(ctx, "st-1")
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Status != settingsmodels.AgentStatus("paused") {
		t.Errorf("Status = %q, want paused", got.Status)
	}
	if got.PauseReason != "Auto-paused: 3 failures" {
		t.Errorf("PauseReason = %q, want the written reason", got.PauseReason)
	}

	if err := repo.UpdateAgentStatusFields(ctx, "st-1", "idle", ""); err != nil {
		t.Fatalf("UpdateAgentStatusFields(clear): %v", err)
	}
	if got, err = repo.GetAgentInstance(ctx, "st-1"); err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if got.Status != settingsmodels.AgentStatus("idle") || got.PauseReason != "" {
		t.Errorf("status/reason = %q/%q, want idle and empty", got.Status, got.PauseReason)
	}
	// Unrelated columns survive.
	if got.Icon != "🤖" {
		t.Errorf("Icon = %q, want it untouched", got.Icon)
	}
}

func TestCountAgentInstancesByRole(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	specs := []struct{ id, ws, name, role string }{
		{"c-1", "ws-1", "Eng One", "engineer"},
		{"c-2", "ws-1", "Eng Two", "engineer"},
		{"c-3", "ws-1", "The CTO", "cto"},
		{"c-4", "ws-2", "Eng Three", "engineer"},
	}
	for _, s := range specs {
		agent := fullAgentInstance(s.id, s.ws, s.name)
		agent.Role = settingsmodels.AgentRole(s.role)
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create %s: %v", s.id, err)
		}
	}
	if err := repo.CreateAgentInstance(ctx, func() *models.AgentInstance {
		a := fullAgentInstance("c-del", "ws-1", "Deleted Eng")
		a.Role = settingsmodels.AgentRole("engineer")
		return a
	}()); err != nil {
		t.Fatalf("create c-del: %v", err)
	}
	if err := repo.DeleteAgentInstance(ctx, "c-del"); err != nil {
		t.Fatalf("delete c-del: %v", err)
	}

	cases := []struct {
		name      string
		workspace string
		role      string
		exclude   string
		want      int
	}{
		{"workspace scoped", "ws-1", "engineer", "", 2},
		{"other role", "ws-1", "cto", "", 1},
		{"with exclusion", "ws-1", "engineer", "c-1", 1},
		{"all workspaces", "", "engineer", "", 3},
		{"unknown role", "ws-1", "designer", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.CountAgentInstancesByRole(ctx, tc.workspace, tc.role, tc.exclude)
			if err != nil {
				t.Fatalf("CountAgentInstancesByRole: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAgentInstanceExistsByName(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("e-1", "ws-1", "Taken")); err != nil {
		t.Fatalf("create e-1: %v", err)
	}
	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("e-2", "ws-2", "Elsewhere")); err != nil {
		t.Fatalf("create e-2: %v", err)
	}

	cases := []struct {
		name      string
		workspace string
		agentName string
		exclude   string
		want      bool
	}{
		{"name taken in workspace", "ws-1", "Taken", "", true},
		{"excluding the holder frees the name", "ws-1", "Taken", "e-1", false},
		{"other workspace does not clash", "ws-1", "Elsewhere", "", false},
		{"any workspace when unscoped", "", "Elsewhere", "", true},
		{"unknown name", "ws-1", "Free", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.AgentInstanceExistsByName(ctx, tc.workspace, tc.agentName, tc.exclude)
			if err != nil {
				t.Fatalf("AgentInstanceExistsByName: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	// A soft-deleted agent frees its name.
	if err := repo.DeleteAgentInstance(ctx, "e-1"); err != nil {
		t.Fatalf("DeleteAgentInstance: %v", err)
	}
	exists, err := repo.AgentInstanceExistsByName(ctx, "ws-1", "Taken", "")
	if err != nil {
		t.Fatalf("AgentInstanceExistsByName after delete: %v", err)
	}
	if exists {
		t.Error("soft-deleted agent still reserves its name")
	}
}

// TestAgentProfileExists_ResolvesShallowKanbanProfile guards
// REQ-OFFICE-REVIEW-SEATS-004.3's quorum-guard resolution: a seat's
// agent_profile_id can come from the casting resolution's runner fallback,
// and a task's runner is not guaranteed to be an Office agent
// (workspace_id != ”). AgentProfileExists must resolve a shallow Kanban
// profile (workspace_id = ”) rather than treat it as deleted, unlike
// agentInstanceFilter-scoped reads elsewhere in this file.
func TestAgentProfileExists_ResolvesShallowKanbanProfile(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"cli-kanban", "cli-kanban", now, now); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, '', '', ?, ?)`,
		"kanban-runner", "cli-kanban", "Kanban Runner", "Kanban Runner", now, now); err != nil {
		t.Fatalf("seed shallow kanban profile: %v", err)
	}

	exists, err := repo.AgentProfileExists(ctx, "kanban-runner")
	if err != nil {
		t.Fatalf("AgentProfileExists: %v", err)
	}
	if !exists {
		t.Error("AgentProfileExists = false for a live shallow Kanban profile, want true")
	}

	unknown, err := repo.AgentProfileExists(ctx, "does-not-exist")
	if err != nil {
		t.Fatalf("AgentProfileExists (unknown id): %v", err)
	}
	if unknown {
		t.Error("AgentProfileExists = true for an unknown id, want false")
	}
}

// TestAgentProfileExists_FalseForSoftDeletedOfficeAgent proves the migrated
// AgentProfileExists still honors the soft-delete boundary Office agents
// rely on, matching the previous AgentInstanceExists behavior.
func TestAgentProfileExists_FalseForSoftDeletedOfficeAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("office-agent", "ws-1", "Office Agent")); err != nil {
		t.Fatalf("create office-agent: %v", err)
	}
	if err := repo.DeleteAgentInstance(ctx, "office-agent"); err != nil {
		t.Fatalf("DeleteAgentInstance: %v", err)
	}

	exists, err := repo.AgentProfileExists(ctx, "office-agent")
	if err != nil {
		t.Fatalf("AgentProfileExists: %v", err)
	}
	if exists {
		t.Error("AgentProfileExists = true for a soft-deleted Office agent, want false")
	}
}

// TestAgentProfileExists_TrueForActiveOfficeAgent covers the third case
// requested alongside the shallow-Kanban and soft-deleted cases above: a
// live, non-deleted Office agent (workspace_id != ”) must resolve true,
// same as it did under the pre-migration AgentInstanceExists name.
func TestAgentProfileExists_TrueForActiveOfficeAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("office-agent-active", "ws-1", "Office Agent")); err != nil {
		t.Fatalf("create office-agent-active: %v", err)
	}

	exists, err := repo.AgentProfileExists(ctx, "office-agent-active")
	if err != nil {
		t.Fatalf("AgentProfileExists: %v", err)
	}
	if !exists {
		t.Error("AgentProfileExists = false for a live Office agent, want true")
	}
}

func TestDeleteAgentInstance_MissingRowIsNotAnError(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.DeleteAgentInstance(context.Background(), "never-existed"); err != nil {
		t.Fatalf("DeleteAgentInstance(missing) = %v, want nil", err)
	}
}

func TestListAgentInstancesFiltered(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()

	specs := []struct{ id, ws, name, role, status, reportsTo string }{
		{"f-1", "ws-1", "Eng One", "engineer", "idle", "f-boss"},
		{"f-2", "ws-1", "Eng Two", "engineer", "working", "f-boss"},
		{"f-3", "ws-1", "Designer", "designer", "idle", "f-other"},
		{"f-4", "ws-2", "Eng Three", "engineer", "idle", "f-boss"},
	}
	for i, s := range specs {
		agent := fullAgentInstance(s.id, s.ws, s.name)
		agent.Role = settingsmodels.AgentRole(s.role)
		agent.Status = settingsmodels.AgentStatus(s.status)
		agent.ReportsTo = s.reportsTo
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create %s: %v", s.id, err)
		}
		stamp := time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		if _, err := db.Exec(`UPDATE agent_profiles SET created_at = ? WHERE id = ?`, stamp, s.id); err != nil {
			t.Fatalf("stamp %s: %v", s.id, err)
		}
	}

	cases := []struct {
		name      string
		workspace string
		filter    sqlite.AgentListFilter
		wantIDs   []string
	}{
		{"no filters, workspace scoped", "ws-1", sqlite.AgentListFilter{}, []string{"f-1", "f-2", "f-3"}},
		{"role", "ws-1", sqlite.AgentListFilter{Role: "engineer"}, []string{"f-1", "f-2"}},
		{"status", "ws-1", sqlite.AgentListFilter{Status: "idle"}, []string{"f-1", "f-3"}},
		{"reports_to", "ws-1", sqlite.AgentListFilter{ReportsTo: "f-boss"}, []string{"f-1", "f-2"}},
		{
			"combined", "ws-1",
			sqlite.AgentListFilter{Role: "engineer", Status: "working", ReportsTo: "f-boss"},
			[]string{"f-2"},
		},
		{"all workspaces", "", sqlite.AgentListFilter{Role: "engineer"}, []string{"f-1", "f-2", "f-4"}},
		{"no match", "ws-1", sqlite.AgentListFilter{Role: "nope"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.ListAgentInstancesFiltered(ctx, tc.workspace, tc.filter)
			if err != nil {
				t.Fatalf("ListAgentInstancesFiltered: %v", err)
			}
			if got == nil {
				t.Fatal("returned nil slice, want empty non-nil")
			}
			ids := make([]string, 0, len(got))
			for _, a := range got {
				ids = append(ids, a.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("ids = %v, want %v (created_at ascending)", ids, tc.wantIDs)
			}
		})
	}
}

func TestListAgentInstancesFiltered_ExcludesDeleted(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("d-1", "ws-1", "Alive")); err != nil {
		t.Fatalf("create d-1: %v", err)
	}
	if err := repo.CreateAgentInstance(ctx, fullAgentInstance("d-2", "ws-1", "Gone")); err != nil {
		t.Fatalf("create d-2: %v", err)
	}
	if err := repo.DeleteAgentInstance(ctx, "d-2"); err != nil {
		t.Fatalf("DeleteAgentInstance: %v", err)
	}

	got, err := repo.ListAgentInstancesFiltered(ctx, "ws-1", sqlite.AgentListFilter{})
	if err != nil {
		t.Fatalf("ListAgentInstancesFiltered: %v", err)
	}
	if len(got) != 1 || got[0].ID != "d-1" {
		t.Errorf("got %+v, want only d-1", got)
	}
}
