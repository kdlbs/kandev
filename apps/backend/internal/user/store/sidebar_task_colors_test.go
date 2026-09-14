package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/user/models"
	_ "github.com/mattn/go-sqlite3"
)

func TestScanUserSettingsSidebarTaskColorsFiltersInvalidValues(t *testing.T) {
	raw := `{"workspace_id":"workspace-1","sidebar_task_colors":{"task-red":"red","task-cleared":null,"task-invalid":"gray","":"blue","task-object":{},"task-number":7}}`
	settings, err := scanUserSettings(settingsScanner{raw: raw}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan settings: %v", err)
	}
	if settings.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace = %q, want workspace-1", settings.WorkspaceID)
	}
	if value := settings.SidebarTaskColors["task-red"]; value == nil || *value != "red" {
		t.Fatalf("red color = %#v, want red", value)
	}
	if value, present := settings.SidebarTaskColors["task-cleared"]; !present || value != nil {
		t.Fatalf("tombstone = (%#v, %t), want (nil, true)", value, present)
	}
	if len(settings.SidebarTaskColors) != 2 {
		t.Fatalf("normalized colors = %#v, want two valid entries", settings.SidebarTaskColors)
	}
}

func TestSQLiteRepositorySidebarTaskColorsRoundTrip(t *testing.T) {
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
	red := "red"
	settings.SidebarTaskColors = map[string]*string{"task-red": &red, "task-cleared": nil}
	upsertUserSettingsForTest(t, repo, ctx, settings)

	got, err := repo.GetUserSettings(ctx, DefaultUserID)
	if err != nil {
		t.Fatalf("get saved settings: %v", err)
	}
	if value := got.SidebarTaskColors["task-red"]; value == nil || *value != "red" {
		t.Fatalf("round-trip red color = %#v, want red", value)
	}
	if value, present := got.SidebarTaskColors["task-cleared"]; !present || value != nil {
		t.Fatalf("round-trip tombstone = (%#v, %t), want (nil, true)", value, present)
	}
}

func TestDecodeSidebarTaskColorsCapsStoredMap(t *testing.T) {
	red := "red"
	colors := make(map[string]*string, models.MaxSidebarTaskColors+1)
	for index := 0; index < models.MaxSidebarTaskColors+1; index++ {
		colors["task-"+strings.Repeat("x", 4)+string(rune(index))] = &red
	}
	rawColors, err := json.Marshal(colors)
	if err != nil {
		t.Fatalf("marshal colors: %v", err)
	}
	settings, err := scanUserSettings(settingsScanner{raw: `{"sidebar_task_colors":` + string(rawColors) + `}`}, DefaultUserID)
	if err != nil {
		t.Fatalf("scan oversized colors: %v", err)
	}
	if len(settings.SidebarTaskColors) > models.MaxSidebarTaskColors {
		t.Fatalf("stored color count = %d, want at most %d", len(settings.SidebarTaskColors), models.MaxSidebarTaskColors)
	}
}
