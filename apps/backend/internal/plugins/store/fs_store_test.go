package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// TestFSStore_List_SkipsCorruptRecordAndReturnsRest pins the fix that one
// unparseable ".yml" (a stray/corrupt file, or one written by a future
// incompatible version) never aborts List() wholesale — the whole plugin
// subsystem depends on List() succeeding at boot (Registry.Load), so a
// single bad record must be skipped (and logged), not fatal.
func TestFSStore_List_SkipsCorruptRecordAndReturnsRest(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save(slack) unexpected error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kandev-plugin-broken.yml"), []byte("not: [valid yaml"), 0o600); err != nil {
		t.Fatalf("write corrupt record: %v", err)
	}

	records, err := s.List()
	if err != nil {
		t.Fatalf("List() unexpected error (a corrupt record must be skipped, not fatal): %v", err)
	}
	if len(records) != 1 || records[0].ID != "kandev-plugin-slack" {
		t.Fatalf("List() = %v, want only the valid kandev-plugin-slack record", records)
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

// TestFSStore_Save_LeavesNoTempFilesBehind pins the fix that writeRecord
// writes via a tmp-file + rename instead of a plain os.WriteFile, so a
// process crash mid-write can never leave a half-written "<id>.yml" for
// List()/Get() to trip over. A successful Save must not leave any stray
// tmp artifact in the store directory.
func TestFSStore_Save_LeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "kandev-plugin-slack.yml" {
		t.Fatalf("store dir entries = %v, want exactly [kandev-plugin-slack.yml] (no leaked tmp file)", entries)
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

// --- id validation: every FSStore method that builds a path from an id
// must reject a traversal/unsafe id before touching the filesystem. ---

func TestFSStore_Get_RejectsTraversalID(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if _, err := s.Get("../escape"); err == nil {
		t.Fatal("Get() expected error for traversal id, got nil")
	}
}

func TestFSStore_Save_RejectsTraversalID(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	err := s.Save(testRecord("../escape"))
	if err == nil {
		t.Fatal("Save() expected error for traversal id, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escape.yml")); !os.IsNotExist(statErr) {
		t.Fatalf("Save() wrote a record file outside the store dir: stat err = %v", statErr)
	}
}

func TestFSStore_Delete_RejectsTraversalID(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Delete("../escape"); err == nil {
		t.Fatal("Delete() expected error for traversal id, got nil")
	}
}

func TestFSStore_GetConfig_RejectsTraversalID(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if _, err := s.GetConfig("../escape"); err == nil {
		t.Fatal("GetConfig() expected error for traversal id, got nil")
	}
}

func TestFSStore_SetConfig_RejectsTraversalID(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	err := s.SetConfig("../escape", map[string]any{"a": 1})
	if err == nil {
		t.Fatal("SetConfig() expected error for traversal id, got nil")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(dir), "escape.config.yml")); !os.IsNotExist(statErr) {
		t.Fatalf("SetConfig() wrote a config file outside the store dir: stat err = %v", statErr)
	}
}

func TestFSStore_Get_RejectsIDWithPathSeparator(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if _, err := s.Get("kandev/plugin"); err == nil {
		t.Fatal("Get() expected error for id containing a path separator, got nil")
	}
}

func TestFSStore_Get_RejectsEmptyID(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if _, err := s.Get(""); err == nil {
		t.Fatal("Get() expected error for empty id, got nil")
	}
}

// TestFSStore_Save_RejectsIDEndingInDotConfig pins the fix for an id whose
// record filename ("<id>.yml") would alias another plugin's config
// filename convention: an id like "foo.config" writes "foo.config.yml",
// which isRecordFile classifies as plugin "foo"'s config file, not a
// record — so the "foo.config" record silently vanishes from List() and
// collides with "foo"'s config storage. safePluginID must reject this
// shape before any FSStore method reaches disk.
func TestFSStore_Save_RejectsIDEndingInDotConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	err := s.Save(testRecord("foo.config"))
	if err == nil {
		t.Fatal("Save() expected error for an id ending in \".config\", got nil")
	}
	if !strings.Contains(err.Error(), "invalid plugin id") {
		t.Fatalf("Save() error = %q, want it to come from the id guard (\"invalid plugin id\")", err.Error())
	}
	if _, statErr := os.Stat(filepath.Join(dir, "foo.config.yml")); !os.IsNotExist(statErr) {
		t.Fatalf("Save() wrote foo.config.yml despite rejecting the id: stat err = %v", statErr)
	}
}

func TestFSStore_Get_RejectsIDEndingInDotConfig(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)

	if _, err := s.Get("foo.config"); err == nil {
		t.Fatal("Get() expected error for an id ending in \".config\", got nil")
	}
}
