package plugins

import (
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

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
	reg.Add(&store.Record{Manifest: *testManifest("kandev-plugin-slack"), Status: store.StatusRegistered})

	rec, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected ok = true")
	}
	rec.Status = store.StatusActive // mutate the returned copy

	fresh, ok := reg.Get("kandev-plugin-slack")
	if !ok {
		t.Fatalf("Get() expected ok = true")
	}
	if fresh.Status != store.StatusRegistered {
		t.Fatalf("mutating a Get() result leaked into the registry: Status = %q, want %q", fresh.Status, store.StatusRegistered)
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
