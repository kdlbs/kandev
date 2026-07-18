package store

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// The config file can carry cleartext secret values (config_schema fields
// marked secret, e.g. a PAT), so it must be owner-only like the record file
// — never world-readable.
func TestFSStore_SetConfig_WritesOwnerOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits not meaningful on windows")
	}
	dir := t.TempDir()
	s := NewFSStore(dir)

	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}
	if err := s.SetConfig("kandev-plugin-slack", map[string]any{"api_token": "ghp_secret"}); err != nil {
		t.Fatalf("SetConfig() unexpected error: %v", err)
	}

	info, err := os.Stat(s.configPath("kandev-plugin-slack"))
	if err != nil {
		t.Fatalf("Stat() unexpected error: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file mode = %o, want 0600", perm)
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

// --- concurrency: SetConfig/GetConfig must hold s.mu, matching Save/Delete,
// so concurrent UpdateConfig PATCH requests cannot interleave writes to the
// same config path. ---

// TestFSStore_SetConfig_SerializedByMu proves SetConfig takes s.mu (the same
// mutex Save/Delete already hold during disk I/O) rather than writing to
// disk unsynchronized. This test is in-package so it can grab s.mu directly
// to simulate a concurrent Save/Delete/SetConfig already in flight.
func TestFSStore_SetConfig_SerializedByMu(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)
	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	s.mu.Lock()

	done := make(chan error, 1)
	go func() {
		done <- s.SetConfig("kandev-plugin-slack", map[string]any{"a": 1})
	}()

	select {
	case err := <-done:
		t.Fatalf("SetConfig() returned (err=%v) while s.mu was held externally — SetConfig is not guarded by s.mu", err)
	case <-time.After(200 * time.Millisecond):
	}

	s.mu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SetConfig() unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SetConfig() never returned after s.mu was released")
	}
}

// TestFSStore_GetConfig_SerializedByMu proves GetConfig takes s.mu (RLock)
// before reading the config file, so a read can never observe a write that
// is only partially applied.
func TestFSStore_GetConfig_SerializedByMu(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)
	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	s.mu.Lock()

	done := make(chan error, 1)
	go func() {
		_, err := s.GetConfig("kandev-plugin-slack")
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("GetConfig() returned (err=%v) while s.mu was held externally — GetConfig is not guarded by s.mu", err)
	case <-time.After(200 * time.Millisecond):
	}

	s.mu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("GetConfig() unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetConfig() never returned after s.mu was released")
	}
}

// TestFSStore_SetConfig_ConcurrentCallsLeaveConsistentFile stress-tests many
// concurrent SetConfig calls for the same id: the config file must always
// end up fully valid (never truncated/interleaved content from two
// writers), and no tmp artifact from the atomic-write helper may leak into
// the store dir. Run with -race.
func TestFSStore_SetConfig_ConcurrentCallsLeaveConsistentFile(t *testing.T) {
	dir := t.TempDir()
	s := NewFSStore(dir)
	if err := s.Save(testRecord("kandev-plugin-slack")); err != nil {
		t.Fatalf("Save() unexpected error: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			cfg := map[string]any{"writer": i, "padding": strings.Repeat("x", 4096)}
			if err := s.SetConfig("kandev-plugin-slack", cfg); err != nil {
				t.Errorf("SetConfig(%d) unexpected error: %v", i, err)
			}
		}()
	}
	wg.Wait()

	got, err := s.GetConfig("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("GetConfig() unexpected error after concurrent SetConfig calls (corrupt file): %v", err)
	}
	writer, ok := got["writer"].(int)
	if !ok || writer < 0 || writer >= n {
		t.Fatalf("GetConfig() = %v, want a single writer's complete config (not merged/corrupted)", got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() unexpected error: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("store dir leaked a temp file after concurrent SetConfig calls: %v", entries)
		}
	}
}
