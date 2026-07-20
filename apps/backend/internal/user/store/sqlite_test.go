package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/user/models"
)

type settingsScanner struct {
	raw string
}

func upsertUserSettingsForTest(t *testing.T, repo *sqliteRepository, ctx context.Context, settings *models.UserSettings) {
	t.Helper()
	var patch *models.TaskCreateLastUsed
	if settings.TaskCreateLastUsed != (models.TaskCreateLastUsed{}) {
		patch = &settings.TaskCreateLastUsed
	}
	if _, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, settings, patch); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
}

func (s settingsScanner) Scan(dest ...any) error {
	*(dest[0].(*string)) = s.raw
	*(dest[1].(*time.Time)) = time.Time{}
	return nil
}

func TestScanUserSettingsChangesPanelLayoutDefault(t *testing.T) {
	t.Run("empty settings default to tree", func(t *testing.T) {
		settings, err := scanUserSettings(settingsScanner{raw: "{}"}, DefaultUserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %q", settings.ChangesPanelLayout)
		}
	})

	t.Run("missing layout defaults to tree", func(t *testing.T) {
		settings, err := scanUserSettings(settingsScanner{raw: `{"chat_submit_key":"cmd_enter"}`}, DefaultUserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "tree" {
			t.Fatalf("expected ChangesPanelLayout=tree, got %q", settings.ChangesPanelLayout)
		}
	})

	t.Run("explicit flat is preserved", func(t *testing.T) {
		settings, err := scanUserSettings(settingsScanner{raw: `{"changes_panel_layout":"flat"}`}, DefaultUserID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if settings.ChangesPanelLayout != "flat" {
			t.Fatalf("expected ChangesPanelLayout=flat, got %q", settings.ChangesPanelLayout)
		}
	})
}

func TestScanUserSettingsConfirmTaskArchiveDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "empty settings require confirmation", raw: `{}`, want: true},
		{name: "missing setting requires confirmation", raw: `{"chat_submit_key":"enter"}`, want: true},
		{name: "explicit false skips confirmation", raw: `{"confirm_task_archive":false}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			if settings.ConfirmTaskArchive != tt.want {
				t.Fatalf("ConfirmTaskArchive = %v, want %v", settings.ConfirmTaskArchive, tt.want)
			}
		})
	}
}

func TestScanUserSettingsMCPTaskAgentProfileDefault(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty settings use current task", raw: `{}`, want: "current_task"},
		{name: "missing setting uses current task", raw: `{"chat_submit_key":"enter"}`, want: "current_task"},
		{name: "unknown setting uses current task", raw: `{"mcp_task_agent_profile_default":"future_value"}`, want: "current_task"},
		{name: "workspace default is preserved", raw: `{"mcp_task_agent_profile_default":"workspace_default"}`, want: "workspace_default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := scanUserSettings(settingsScanner{raw: tt.raw}, DefaultUserID)
			if err != nil {
				t.Fatalf("scan settings: %v", err)
			}
			raw, err := json.Marshal(settings)
			if err != nil {
				t.Fatalf("marshal normalized settings: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode normalized settings: %v", err)
			}
			if got := payload["mcp_task_agent_profile_default"]; got != tt.want {
				t.Fatalf("mcp_task_agent_profile_default = %#v, want %q", got, tt.want)
			}
		})
	}
}

func TestMarshalUserSettingsPersistsDisabledArchiveConfirmation(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{ConfirmTaskArchive: false})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got, ok := payload["confirm_task_archive"].(bool); !ok || got {
		t.Fatalf("confirm_task_archive = %#v, want false", payload["confirm_task_archive"])
	}
}

func TestMarshalUserSettingsPersistsMCPTaskAgentProfileDefault(t *testing.T) {
	raw, err := marshalUserSettingsPayload(&models.UserSettings{
		MCPTaskAgentProfileDefault: models.MCPTaskAgentProfileDefaultWorkspaceDefault,
	})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if got := payload["mcp_task_agent_profile_default"]; got != models.MCPTaskAgentProfileDefaultWorkspaceDefault {
		t.Fatalf("mcp_task_agent_profile_default = %#v, want workspace_default", got)
	}
}

func TestSQLiteRepositoryMCPTaskAgentProfileDefaultRoundTrip(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	settings.MCPTaskAgentProfileDefault = models.MCPTaskAgentProfileDefaultWorkspaceDefault
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get saved settings: %v", err)
	}
	if got.MCPTaskAgentProfileDefault != models.MCPTaskAgentProfileDefaultWorkspaceDefault {
		t.Fatalf("MCPTaskAgentProfileDefault = %q, want workspace_default", got.MCPTaskAgentProfileDefault)
	}
}

func TestScanUserSettingsSystemMetricsDisplayDefault(t *testing.T) {
	settings, err := scanUserSettings(settingsScanner{raw: "{}"}, DefaultUserID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if settings.SystemMetricsDisplay.ShowInTopbar {
		t.Fatal("system metrics display should default to disabled")
	}

	settings, err = scanUserSettings(settingsScanner{raw: `{"system_metrics_display":{"show_in_topbar":true}}`}, DefaultUserID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !settings.SystemMetricsDisplay.ShowInTopbar {
		t.Fatal("expected stored system metrics display preference")
	}
}

func TestSQLiteRepositorySystemMetricsDisplayRoundTrip(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	settings.SystemMetricsDisplay = models.SystemMetricsDisplaySettings{ShowInTopbar: true}
	upsertUserSettingsForTest(t, repo, ctx, settings)
	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if !got.SystemMetricsDisplay.ShowInTopbar {
		t.Fatal("expected system metrics display preference to round-trip")
	}
}

func TestSQLiteRepositoryUpdateTaskCreateLastUsedPatchesNonEmptyFields(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	settings.SidebarActiveViewID = "view-1"
	settings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-1",
		Branch:            "main",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	}
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		Branch:         "feature",
		AgentProfileID: "agent-2",
	})
	if err != nil {
		t.Fatalf("update task-create last-used: %v", err)
	}

	if got.TaskCreateLastUsed.RepositoryID != "repo-1" {
		t.Fatalf("repository id should be preserved, got %q", got.TaskCreateLastUsed.RepositoryID)
	}
	if got.TaskCreateLastUsed.Branch != "feature" {
		t.Fatalf("branch should update, got %q", got.TaskCreateLastUsed.Branch)
	}
	if got.TaskCreateLastUsed.AgentProfileID != "agent-2" {
		t.Fatalf("agent profile should update, got %q", got.TaskCreateLastUsed.AgentProfileID)
	}
	if got.TaskCreateLastUsed.ExecutorProfileID != "exec-1" {
		t.Fatalf("executor profile should be preserved, got %q", got.TaskCreateLastUsed.ExecutorProfileID)
	}
	if got.SidebarActiveViewID != "view-1" {
		t.Fatalf("unrelated settings should be preserved, got active view %q", got.SidebarActiveViewID)
	}
}

func TestSQLiteRepositoryUpdateTaskCreateLastUsedClearsBranchOnRepositoryChange(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	settings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-before",
		Branch:            "main",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	}
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID: "repo-after",
	})
	if err != nil {
		t.Fatalf("update task-create last-used: %v", err)
	}

	if got.TaskCreateLastUsed.RepositoryID != "repo-after" {
		t.Fatalf("repository id should update, got %q", got.TaskCreateLastUsed.RepositoryID)
	}
	if got.TaskCreateLastUsed.Branch != "" {
		t.Fatalf("branch should clear on repository change, got %q", got.TaskCreateLastUsed.Branch)
	}
	if got.TaskCreateLastUsed.AgentProfileID != "agent-1" {
		t.Fatalf("agent profile should be preserved, got %q", got.TaskCreateLastUsed.AgentProfileID)
	}
	if got.TaskCreateLastUsed.ExecutorProfileID != "exec-1" {
		t.Fatalf("executor profile should be preserved, got %q", got.TaskCreateLastUsed.ExecutorProfileID)
	}
}

func TestBuildPostgresTaskCreateLastUsedUpdatePatchesNonEmptyFields(t *testing.T) {
	query, args := buildPostgresTaskCreateLastUsedUpdate(models.TaskCreateLastUsed{
		RepositoryID:      "repo-1",
		Branch:            "feature",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})

	if strings.Contains(query, "json(") || strings.Contains(query, "json_extract") {
		t.Fatalf("postgres update must not use sqlite JSON functions: %s", query)
	}
	if !strings.Contains(query, "jsonb_set") {
		t.Fatalf("postgres update should use jsonb_set: %s", query)
	}
	if !strings.Contains(query, "{task_create_last_used,repository_id}") ||
		!strings.Contains(query, "{task_create_last_used,branch}") ||
		!strings.Contains(query, "{task_create_last_used,agent_profile_id}") ||
		!strings.Contains(query, "{task_create_last_used,executor_profile_id}") {
		t.Fatalf("postgres update should patch task-create fields: %s", query)
	}
	if len(args) != 4 {
		t.Fatalf("expected one arg per task-create field, got %d", len(args))
	}
}

func TestBuildPostgresTaskCreateLastUsedUpdateClearsBranchOnRepositoryChange(t *testing.T) {
	query, args := buildPostgresTaskCreateLastUsedUpdate(models.TaskCreateLastUsed{
		RepositoryID: "repo-after",
	})

	if !strings.Contains(query, "{task_create_last_used,repository_id}") ||
		!strings.Contains(query, "{task_create_last_used,branch}") {
		t.Fatalf("postgres update should patch repository and clear branch: %s", query)
	}
	if strings.Contains(query, "{task_create_last_used,agent_profile_id}") ||
		strings.Contains(query, "{task_create_last_used,executor_profile_id}") {
		t.Fatalf("postgres update should not patch profile fields: %s", query)
	}
	if len(args) != 2 {
		t.Fatalf("expected repository and empty branch args, got %d", len(args))
	}
	if args[0] != "repo-after" || args[1] != "" {
		t.Fatalf("expected repo-after and empty branch args, got %#v", args)
	}
}

func TestBuildPostgresUserSettingsPreservingTaskCreateLastUsedUpdateUsesJSONB(t *testing.T) {
	patch := models.TaskCreateLastUsed{
		RepositoryID:      "repo-1",
		Branch:            "feature",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	}
	query, args := buildPostgresUserSettingsPreservingTaskCreateLastUsedUpdate(&patch)

	if strings.Contains(query, "json(") || strings.Contains(query, "json_extract") {
		t.Fatalf("postgres update must not use sqlite JSON functions: %s", query)
	}
	if !strings.Contains(query, "?::jsonb") || !strings.Contains(query, "jsonb_set") {
		t.Fatalf("postgres update should merge payload with jsonb_set: %s", query)
	}
	if !strings.Contains(query, "{task_create_last_used,repository_id}") ||
		!strings.Contains(query, "{task_create_last_used,branch}") ||
		!strings.Contains(query, "{task_create_last_used,agent_profile_id}") ||
		!strings.Contains(query, "{task_create_last_used,executor_profile_id}") {
		t.Fatalf("postgres update missing task-create paths: %s", query)
	}
	if len(args) != 4 {
		t.Fatalf("expected one arg per task-create field, got %d", len(args))
	}
}

func TestPostgresRepositoryTaskCreateLastUsedRoundTrip(t *testing.T) {
	conn := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new postgres repo: %v", err)
	}

	ctx := context.Background()
	staleSettings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleSettings.SidebarActiveViewID = "view-before"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-before",
		Branch:            "main",
		AgentProfileID:    "agent-before",
		ExecutorProfileID: "exec-before",
	}
	upsertUserSettingsForTest(t, repo, ctx, staleSettings)

	got, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID:   "repo-after",
		Branch:         "feature",
		AgentProfileID: "agent-after",
	})
	if err != nil {
		t.Fatalf("update postgres task-create last-used: %v", err)
	}
	if got.SidebarActiveViewID != "view-before" {
		t.Fatalf("unrelated postgres setting should be preserved, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-after" ||
		got.TaskCreateLastUsed.Branch != "feature" ||
		got.TaskCreateLastUsed.AgentProfileID != "agent-after" ||
		got.TaskCreateLastUsed.ExecutorProfileID != "exec-before" {
		t.Fatalf("postgres task-create update mismatch: %+v", got.TaskCreateLastUsed)
	}

	staleSettings.SidebarActiveViewID = "view-after"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID: "repo-stale",
		Branch:       "stale",
	}
	got, err = repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, staleSettings, nil)
	if err != nil {
		t.Fatalf("upsert preserving postgres task-create last-used: %v", err)
	}
	if got.SidebarActiveViewID != "view-after" {
		t.Fatalf("expected unrelated postgres setting to update, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-after" ||
		got.TaskCreateLastUsed.Branch != "feature" ||
		got.TaskCreateLastUsed.AgentProfileID != "agent-after" ||
		got.TaskCreateLastUsed.ExecutorProfileID != "exec-before" {
		t.Fatalf("postgres preserving upsert should keep current task-create values: %+v", got.TaskCreateLastUsed)
	}
}

func TestSQLiteRepositoryUpsertSettingsPreservesCurrentTaskCreateLastUsed(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	staleSettings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleSettings.SidebarActiveViewID = "view-before"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{RepositoryID: "repo-before"}
	upsertUserSettingsForTest(t, repo, ctx, staleSettings)

	// Simulate a task-create write that lands after another settings caller
	// read the old blob but before that caller writes its unrelated change.
	if _, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID: "repo-after",
		Branch:       "feature",
	}); err != nil {
		t.Fatalf("update task-create last-used: %v", err)
	}

	staleSettings.SidebarActiveViewID = "view-after"
	got, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, staleSettings, nil)
	if err != nil {
		t.Fatalf("upsert preserving task-create last-used: %v", err)
	}

	if got.SidebarActiveViewID != "view-after" {
		t.Fatalf("expected unrelated setting to update, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-after" {
		t.Fatalf("expected current task-create repository to survive stale write, got %q", got.TaskCreateLastUsed.RepositoryID)
	}
	if got.TaskCreateLastUsed.Branch != "feature" {
		t.Fatalf("expected current task-create branch to survive stale write, got %q", got.TaskCreateLastUsed.Branch)
	}
}

func TestSQLiteRepositoryUpsertSettingsPreservingTaskCreateLastUsedAppliesPatch(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	staleSettings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	staleSettings.SidebarActiveViewID = "view-before"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-before",
		Branch:            "main",
		AgentProfileID:    "agent-before",
		ExecutorProfileID: "exec-before",
	}
	upsertUserSettingsForTest(t, repo, ctx, staleSettings)
	if _, err := repo.UpdateTaskCreateLastUsed(ctx, DefaultUserID, models.TaskCreateLastUsed{
		RepositoryID:      "repo-current",
		Branch:            "current",
		ExecutorProfileID: "exec-current",
	}); err != nil {
		t.Fatalf("update current task-create last-used: %v", err)
	}

	staleSettings.SidebarActiveViewID = "view-after"
	staleSettings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:   "repo-stale",
		Branch:         "stale",
		AgentProfileID: "agent-stale",
	}
	patch := models.TaskCreateLastUsed{AgentProfileID: "agent-after"}
	got, err := repo.UpsertUserSettingsPreservingTaskCreateLastUsed(ctx, staleSettings, &patch)
	if err != nil {
		t.Fatalf("upsert preserving task-create last-used with patch: %v", err)
	}

	if got.SidebarActiveViewID != "view-after" {
		t.Fatalf("expected unrelated setting to update, got %q", got.SidebarActiveViewID)
	}
	if got.TaskCreateLastUsed.RepositoryID != "repo-current" {
		t.Fatalf("expected current repository to survive stale write, got %q", got.TaskCreateLastUsed.RepositoryID)
	}
	if got.TaskCreateLastUsed.Branch != "current" {
		t.Fatalf("expected current branch to survive stale write, got %q", got.TaskCreateLastUsed.Branch)
	}
	if got.TaskCreateLastUsed.AgentProfileID != "agent-after" {
		t.Fatalf("expected patch agent profile to apply, got %q", got.TaskCreateLastUsed.AgentProfileID)
	}
	if got.TaskCreateLastUsed.ExecutorProfileID != "exec-current" {
		t.Fatalf("expected current executor profile to survive stale write, got %q", got.TaskCreateLastUsed.ExecutorProfileID)
	}
}

func TestSQLiteRepositorySidebarViewStateRoundTrip(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	repo, err := newSQLiteRepositoryWithDB(conn, conn)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	ctx := context.Background()
	settings, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get defaults: %v", err)
	}
	settings.SidebarActiveViewID = "view-1"
	settings.SidebarTaskPrefs = models.SidebarTaskPrefs{
		PinnedTaskIDs:          []string{"task-1"},
		OrderedTaskIDs:         []string{"task-2", "task-1"},
		SubtaskOrderByParentID: map[string][]string{"task-1": {"sub-1"}},
	}
	settings.TaskCreateLastUsed = models.TaskCreateLastUsed{
		RepositoryID:      "repo-1",
		Branch:            "main",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	}
	settings.JiraSavedViews = json.RawMessage(`[{"id":"view-1"}]`)
	settings.GitLabSavedPresets = json.RawMessage(`[{"id":"preset-1"}]`)
	settings.SidebarDraft = &models.SidebarViewDraft{
		BaseViewID: "view-1",
		Filters: []models.SidebarViewClause{{
			ID:        "clause-1",
			Dimension: "titleMatch",
			Op:        "matches",
			Value:     json.RawMessage(`"bug"`),
		}},
		Sort:  models.SidebarViewSort{Key: "updatedAt", Direction: "desc"},
		Group: "workflow",
	}
	upsertUserSettingsForTest(t, repo, ctx, settings)
	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if got.SidebarActiveViewID != "view-1" {
		t.Fatalf("expected active view to round-trip, got %q", got.SidebarActiveViewID)
	}
	if got.SidebarDraft == nil || got.SidebarDraft.Group != "workflow" {
		t.Fatalf("expected sidebar draft to round-trip, got %+v", got.SidebarDraft)
	}
	if got.SidebarTaskPrefs.PinnedTaskIDs[0] != "task-1" {
		t.Fatalf("expected sidebar task prefs to round-trip, got %+v", got.SidebarTaskPrefs)
	}
	if got.TaskCreateLastUsed.Branch != "main" {
		t.Fatalf("expected task-create prefs to round-trip, got %+v", got.TaskCreateLastUsed)
	}
	if string(got.JiraSavedViews) != `[{"id":"view-1"}]` {
		t.Fatalf("expected Jira saved views to round-trip, got %s", string(got.JiraSavedViews))
	}
	if string(got.GitLabSavedPresets) != `[{"id":"preset-1"}]` {
		t.Fatalf("expected GitLab presets to round-trip, got %s", string(got.GitLabSavedPresets))
	}
}
