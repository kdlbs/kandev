package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/user/models"
)

type settingsScanner struct {
	raw string
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
	if err := repo.UpsertUserSettings(ctx, settings); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	if !got.SystemMetricsDisplay.ShowInTopbar {
		t.Fatal("expected system metrics display preference to round-trip")
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
	if err := repo.UpsertUserSettings(ctx, settings); err != nil {
		t.Fatalf("upsert settings: %v", err)
	}
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
