package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// migrationSaveErrorStore exposes both legacy and already-migrated records,
// then fails the migration write. Registry.Load must still publish the full
// List result so boot scans cannot treat valid records as sideloads.
type migrationSaveErrorStore struct {
	store.Store
	records []*store.Record
	saves   int
}

func (s *migrationSaveErrorStore) List() ([]*store.Record, error) {
	return s.records, nil
}

func (s *migrationSaveErrorStore) Save(*store.Record) error {
	s.saves++
	return errors.New("simulated migration save failure")
}

// testManifest returns a minimal valid runtime-managed manifest for tests
// that only need a store.Record, not a full installed package (see
// service_test.go's testPackage for that).
func testManifest(id string) *manifest.Manifest {
	return &manifest.Manifest{
		ID:          id,
		APIVersion:  1,
		Version:     "1.0.0",
		DisplayName: "Test Plugin",
		Runtime: manifest.Runtime{
			Type:        "binary",
			Executables: map[string]string{"linux-amd64": "server/plugin-linux-amd64"},
		},
	}
}

func TestRegistryLoadPopulatesFromStore(t *testing.T) {
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	if err := fsStore.Save(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusRegistered}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reg := NewRegistry()
	if err := reg.Load(fsStore); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	rec, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected record to be present after Load()")
	}
	if rec.ID != "kandev-plugin-slack" {
		t.Fatalf("Get() ID = %q, want %q", rec.ID, "kandev-plugin-slack")
	}
}

func TestRegistryLoadMigratesMissingInstallationIDOnce(t *testing.T) {
	dir := t.TempDir()
	fsStore := store.NewFSStore(dir)
	if err := fsStore.Save(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusRegistered}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reg := NewRegistry()
	if err := reg.Load(fsStore); err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}

	first, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatal("Get() expected record to be present after Load()")
	}
	if first.InstallationID == "" {
		t.Fatal("Load() did not mint an installation id for a legacy record")
	}

	reloaded, err := fsStore.Get("kandev-plugin-slack")
	if err != nil {
		t.Fatalf("reloaded record: %v", err)
	}
	if reloaded.InstallationID != first.InstallationID {
		t.Fatalf("installation id changed across migration/save: first=%q reloaded=%q", first.InstallationID, reloaded.InstallationID)
	}

	reg2 := NewRegistry()
	if err := reg2.Load(fsStore); err != nil {
		t.Fatalf("second Load() unexpected error: %v", err)
	}
	second, ok := reg2.Get("kandev-plugin-slack")
	if !ok {
		t.Fatal("Get() expected record to be present after second Load()")
	}
	if second.InstallationID != first.InstallationID {
		t.Fatalf("installation id was regenerated on replay: first=%q second=%q", first.InstallationID, second.InstallationID)
	}
}

func TestRegistryLoadRetainsRecordsWhenMigrationSaveFails(t *testing.T) {
	legacy := &store.Record{Manifest: *testManifest("kandev-plugin-legacy"), Status: store.StatusDisabled}
	migrated := &store.Record{
		Manifest:       *testManifest("kandev-plugin-migrated"),
		Status:         store.StatusActive,
		InstallationID: "stable-installation",
	}
	registry := NewRegistry()
	backend := &migrationSaveErrorStore{records: []*store.Record{legacy, migrated}}
	err := registry.Load(backend)
	if err == nil {
		t.Fatal("Load() expected migration error")
	}

	for _, want := range []*store.Record{legacy, migrated} {
		got, ok := registry.Get(want.ID)
		if !ok {
			t.Fatalf("Load() dropped %q after migration failure", want.ID)
		}
		if got.Status != want.Status {
			t.Fatalf("Load() status for %q = %q, want %q", want.ID, got.Status, want.Status)
		}
	}
	if got, _ := registry.Get(legacy.ID); got.InstallationID != "" {
		t.Fatalf("Load() exposed an unpersisted installation id %q", got.InstallationID)
	}

	pluginsDir := t.TempDir()
	manifestPath := filepath.Join(pluginsDir, legacy.ID, "1.0.0", manifestFileName)
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("id: kandev-plugin-legacy\napi_version: 1\nversion: 1.0.0\ndisplay_name: Legacy\nruntime:\n  type: binary\n  executables:\n    linux-amd64: server/plugin\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	svc := &Service{store: backend, registry: registry, log: testLogger(t)}
	svc.SetPluginsDir(pluginsDir)
	if _, err := svc.approvalLedger().grant("stable-installation", "workspace-1", 1, "digest", nil, "human", "grant", "audit-1", time.Now().UTC()); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	result := svc.bootScan(context.Background())
	if len(result.Added) != 0 {
		t.Fatalf("boot scan registered an existing record as a sideload: %#v", result.Added)
	}
	if backend.saves != 1 {
		t.Fatalf("boot scan performed %d store saves, want only the failed migration save", backend.saves)
	}
	if _, ok, err := svc.approvalCurrent("stable-installation", "workspace-1"); err != nil || !ok {
		t.Fatalf("boot scan lost an unrelated approval: ok=%v err=%v", ok, err)
	}
}

func TestServiceUninstallLegacyRecordAfterInstallationIDMigrationFails(t *testing.T) {
	pluginsDir := t.TempDir()
	fsStore := store.NewFSStore(pluginsDir)
	legacy := &store.Record{
		Manifest:    *testManifest("kandev-plugin-legacy"),
		Status:      store.StatusDisabled,
		InstallPath: filepath.Join(pluginsDir, "kandev-plugin-legacy", "1.0.0"),
	}
	if err := os.MkdirAll(legacy.InstallPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() = %v", err)
	}
	if err := fsStore.Save(legacy); err != nil {
		t.Fatalf("Save() = %v", err)
	}

	registry := NewRegistry()
	backend := &migrationSaveErrorStore{Store: fsStore, records: []*store.Record{legacy}}
	if err := registry.Load(backend); err == nil {
		t.Fatal("Load() expected migration error")
	}
	svc := NewService(backend, registry, nil, testLogger(t))
	svc.SetPluginsDir(pluginsDir)

	if err := svc.Uninstall(t.Context(), legacy.ID); err != nil {
		t.Fatalf("Uninstall() legacy record: %v", err)
	}
	if _, err := fsStore.Get(legacy.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("store.Get() after Uninstall() = %v, want store.ErrNotFound", err)
	}
	if _, ok := registry.Get(legacy.ID); ok {
		t.Fatal("registry retained legacy record after Uninstall()")
	}
}

func TestRegistryGetMissingReturnsNotOK(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.Get("missing"); ok {
		t.Fatalf("Get() expected ok = false for missing id")
	}
}

func TestRegistryAddThenListReturnsRecord(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusRegistered})

	list := reg.List()
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	if list[0].ID != "kandev-plugin-slack" {
		t.Fatalf("List()[0].ID = %q, want %q", list[0].ID, "kandev-plugin-slack")
	}
}

func TestRegistryRemoveDeletesRecord(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusRegistered})

	reg.Remove("kandev-plugin-slack")

	if _, ok := reg.Get("kandev-plugin-slack"); ok {
		t.Fatalf("Get() expected record to be removed")
	}
}

func TestRegistrySetStatusUpdatesRecord(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusRegistered})

	updated, ok := reg.SetStatus("kandev-plugin-slack", store.StatusActive)
	if !ok {
		t.Fatalf("SetStatus() expected ok = true")
	}
	if updated.Status != store.StatusActive {
		t.Fatalf("SetStatus() returned Status = %q, want %q", updated.Status, store.StatusActive)
	}

	rec, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected record present")
	}
	if rec.Status != store.StatusActive {
		t.Fatalf("Get() Status = %q, want %q", rec.Status, store.StatusActive)
	}
}

func TestRegistrySetStatusMissingReturnsNotOK(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.SetStatus("missing", store.StatusActive); ok {
		t.Fatalf("SetStatus() expected ok = false for missing id")
	}
}

func TestRegistryGetReturnsIndependentCopy(t *testing.T) {
	reg := NewRegistry()
	autoUpdate := true
	reg.Add(&store.Record{
		Manifest:   *testManifest("kandev-plugin-slack"),
		Status:     store.StatusRegistered,
		AutoUpdate: &autoUpdate,
	})

	rec, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected ok = true")
	}
	rec.Status = store.StatusActive // mutate the returned copy
	*rec.AutoUpdate = false         // mutate the returned pointer copy

	fresh, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected ok = true")
	}
	if fresh.Status != store.StatusRegistered {
		t.Fatalf("mutating a Get() result leaked into the registry: Status = %q, want %q", fresh.Status, store.StatusRegistered)
	}
	if fresh.AutoUpdate == nil || !*fresh.AutoUpdate {
		t.Fatalf("mutating a Get() result leaked into the registry: AutoUpdate = %v, want true", fresh.AutoUpdate)
	}
}

func TestRegistrySetRestartCountUpdatesRecord(t *testing.T) {
	reg := NewRegistry()
	reg.Add(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusActive})

	updated, ok := reg.SetRestartCount("kandev-plugin-slack", 2)
	if !ok {
		t.Fatalf("SetRestartCount() expected ok = true")
	}
	if updated.RestartCount != 2 {
		t.Fatalf("SetRestartCount() RestartCount = %d, want 2", updated.RestartCount)
	}

	rec, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected record present")
	}
	if rec.RestartCount != 2 {
		t.Fatalf("Get() RestartCount = %d, want 2", rec.RestartCount)
	}
}

func TestRegistrySetRestartCountMissingReturnsNotOK(t *testing.T) {
	reg := NewRegistry()
	if _, ok := reg.SetRestartCount("missing", 1); ok {
		t.Fatalf("SetRestartCount() expected ok = false for missing id")
	}
}
