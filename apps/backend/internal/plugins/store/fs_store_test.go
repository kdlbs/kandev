package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
)

// testRecord returns a minimal valid installed record for tests.
func testRecord(id string) *Record {
	return &Record{
		Manifest: manifest.Manifest{
			ID:          id,
			APIVersion:  1,
			Version:     "1.0.0",
			DisplayName: "Test Plugin",
			Runtime: manifest.Runtime{
				Type:        "binary",
				Executables: map[string]string{"linux-amd64": "server/plugin-linux-amd64"},
			},
		},
		Status:      StatusRegistered,
		InstallPath: "/home/user/.kandev/plugins/" + id + "/1.0.0",
		Signed:      false,
		InstalledAt: time.Now().UTC(),
	}
}

func TestFSStore_Save_WritesRecordFile(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	rec := testRecord("kandev-plugin-slack")
	if err := s.Save(rec); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "kandev-plugin-slack.yml")); err != nil {
		t.Fatalf("expected record file to exist: %v", err)
	}

	got, err := s.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.ID != rec.ID || got.Version != rec.Version || got.InstallPath != rec.InstallPath {
		t.Fatalf("Get() = %+v, want id/version/install_path to round-trip from %+v", got, rec)
	}
	if got.Status != StatusRegistered {
		t.Fatalf("Get().Status = %q, want %q", got.Status, StatusRegistered)
	}
}

func TestFSStore_Get_UnknownIDReturnsErrNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	_, err := s.Get("does-not-exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestFSStore_Delete_RemovesRecordAndConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	rec := testRecord("kandev-plugin-slack")
	if err := s.Save(rec); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	if err := s.SetConfig("kandev-plugin-slack", map[string]any{"a": 1}); err != nil {
		t.Fatalf("SetConfig() unexpected error: %v", err)
	}

	if err := s.Delete("kandev-plugin-slack"); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}

	if _, err := s.Get("kandev-plugin-slack"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "kandev-plugin-slack.config.yml")); !os.IsNotExist(err) {
		t.Fatalf("expected config file to be removed, stat err = %v", err)
	}
}

func TestFSStore_Delete_UnknownIDReturnsErrNotFound(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Delete("does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestFSStore_List_ReturnsAllInstalledPlugins(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save(slack) unexpected error: %v", err)
	}
	if err := s.Save(testRecord("kandev-plugin-jira")); err != nil {
		t.Fatalf("Save(jira) unexpected error: %v", err)
	}

	records, err := s.List()
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("List() returned %d records, want 2", len(records))
	}

	ids := map[string]bool{}
	for _, r := range records {
		ids[r.ID] = true
	}
	if !ids["kandev-plugin-slack"] || !ids["kandev-plugin-jira"] {
		t.Fatalf("List() ids = %v, want both plugins present", ids)
	}
}

func TestFSStore_List_EmptyDirReturnsNoRecords(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	records, err := s.List()
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("List() returned %d records, want 0", len(records))
	}
}

func TestFSStore_Save_PersistsUpdatedRecord(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	rec := testRecord("kandev-plugin-slack")
	if err := s.Save(rec); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	rec.Status = StatusDisabled
	rec.RestartCount = 2
	if err := s.Save(rec); err != nil {
		t.Fatalf("Save() (update) unexpected error: %v", err)
	}

	got, err := s.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if got.Status != StatusDisabled {
		t.Fatalf("Get().Status = %q, want %q", got.Status, StatusDisabled)
	}
	if got.RestartCount != 2 {
		t.Fatalf("Get().RestartCount = %d, want 2", got.RestartCount)
	}
}

func TestFSStore_SetConfigThenGetConfig_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	cfg := map[string]any{"default_channel": "#dev", "notify_on_task_created": true}
	if err := s.SetConfig("kandev-plugin-slack", cfg); err != nil {
		t.Fatalf("SetConfig() unexpected error: %v", err)
	}

	got, err := s.GetConfig("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("GetConfig() unexpected error: %v", err)
	}
	if got["default_channel"] != "#dev" {
		t.Fatalf("GetConfig()[\"default_channel\"] = %v, want %q", got["default_channel"], "#dev")
	}
	if got["notify_on_task_created"] != true {
		t.Fatalf("GetConfig()[\"notify_on_task_created\"] = %v, want true", got["notify_on_task_created"])
	}
}

func TestFSStore_GetConfig_NoConfigFileReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	got, err := s.GetConfig("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("GetConfig() unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("GetConfig() = %v, want empty map when no config was ever set", got)
	}
}
