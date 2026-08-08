package workspaces

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestDependencyDirectoryAllowlistIsExactAndReturnedByCopy(t *testing.T) {
	want := []string{
		"node_modules", "bower_components", ".pnpm-store", ".yarn/cache", ".yarn/unplugged",
		".venv", "venv", ".tox", ".nox", "__pypackages__", "Pods", ".gradle",
	}
	got := DependencyDirectoryAllowlist()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allowlist = %#v, want %#v", got, want)
	}
	got[0] = "mutated"
	if DependencyDirectoryAllowlist()[0] != want[0] {
		t.Fatal("allowlist exposed its internal storage")
	}
}

func TestCleanupDependenciesOnlyRemovesAllowlistedDirectoriesFromArchivedAndDeletedWorkspaces(t *testing.T) {
	provider, tasksRoot, inventory, _ := newDependencyProviderFixture(t)
	archived := createDependencyWorkspace(t, tasksRoot, "archived-task_abc", "archived-task")
	deleted := createDependencyWorkspace(t, tasksRoot, "deleted-task_def", "deleted-task")
	active := createDependencyWorkspace(t, tasksRoot, "active-task_ghi", "active-task")
	inventory.inventory.WorktreePaths = []string{filepath.Join(active, "repo")}

	for _, root := range []string{archived, deleted, active} {
		writeDependencyFixture(t, root)
	}

	result, err := provider.CleanupDependencies(context.Background())
	if err != nil {
		t.Fatalf("CleanupDependencies: %v", err)
	}
	wantDirectories := (len(DependencyDirectoryAllowlist()) + 1) * 2
	if result.Directories != wantDirectories {
		t.Fatalf("directories = %d, want %d; result=%#v", result.Directories, wantDirectories, result)
	}
	if result.Workspaces != 2 {
		t.Fatalf("workspaces = %d, want 2; result=%#v", result.Workspaces, result)
	}
	if result.ReclaimedBytes != int64(wantDirectories*4) {
		t.Fatalf("reclaimed bytes = %d, want %d", result.ReclaimedBytes, wantDirectories*4)
	}

	for _, root := range []string{archived, deleted} {
		for _, relative := range DependencyDirectoryAllowlist() {
			assertDependencyRemoved(t, root, filepath.FromSlash(relative))
		}
		assertDependencyRemoved(t, root, filepath.Join("src", "node_modules"))
		assertDependencyKept(t, root, filepath.Join("vendor", "node_modules"))
		assertDependencyKept(t, root, filepath.Join("target", "node_modules"))
		assertDependencyKept(t, root, filepath.Join("build", "node_modules"))
		assertDependencyKept(t, root, filepath.Join(".cache", "node_modules"))
		assertDependencyKept(t, root, filepath.Join(".git", "node_modules"))
		assertDependencyKept(t, root, "source.txt")
	}
	for _, root := range []string{active} {
		for _, relative := range DependencyDirectoryAllowlist() {
			assertDependencyKept(t, root, filepath.FromSlash(relative))
		}
	}
}

func TestCleanupDependenciesSkipsSymlinkTargetsAndExternalPayloads(t *testing.T) {
	provider, tasksRoot, _, _ := newDependencyProviderFixture(t)
	root := createDependencyWorkspace(t, tasksRoot, "symlink-task_abc", "archived-task")
	external := t.TempDir()
	externalFile := filepath.Join(external, "keep.txt")
	if err := os.WriteFile(externalFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "node_modules")); err != nil {
		t.Fatalf("symlink dependency: %v", err)
	}

	result, err := provider.CleanupDependencies(context.Background())
	if err != nil {
		t.Fatalf("CleanupDependencies: %v", err)
	}
	if result.Directories != 0 {
		t.Fatalf("directories = %d, want 0", result.Directories)
	}
	assertDependencyKept(t, root, "node_modules")
	if _, err := os.Stat(externalFile); err != nil {
		t.Fatalf("external symlink payload changed: %v", err)
	}
}

func TestCleanupDependenciesFailsClosedWhenEligibilityInventoryIsMissing(t *testing.T) {
	provider, tasksRoot, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	root := createDependencyWorkspace(t, tasksRoot, "missing-state-task_abc", "missing-state-task")
	writeDependencyDirectory(t, filepath.Join(root, "node_modules"), "keep")

	result, err := provider.CleanupDependencies(context.Background())
	if err == nil {
		t.Fatal("CleanupDependencies succeeded without task eligibility inventory")
	}
	if !errors.Is(err, ErrInventoryIncomplete) {
		t.Fatalf("error = %v, want ErrInventoryIncomplete", err)
	}
	if result.Directories != 0 || len(store.entries) != 0 {
		t.Fatalf("cleanup mutated storage: result=%#v entries=%d", result, len(store.entries))
	}
	assertDependencyKept(t, root, "node_modules")
}

func TestCleanupDependenciesHonorsCancellationBeforeMutation(t *testing.T) {
	provider, tasksRoot, _, _ := newDependencyProviderFixture(t)
	root := createDependencyWorkspace(t, tasksRoot, "cancelled-task_abc", "deleted-task")
	writeDependencyDirectory(t, filepath.Join(root, "node_modules"), "keep")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := provider.CleanupDependencies(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if result.Directories != 0 {
		t.Fatalf("directories = %d, want 0", result.Directories)
	}
	assertDependencyKept(t, root, "node_modules")
}

func TestCleanupDependenciesPreservesWorkspaceForRestore(t *testing.T) {
	provider, tasksRoot, _, store := newDependencyProviderFixture(t)
	root := createDependencyWorkspace(t, tasksRoot, "restore-task_abc", "archived-task")
	writeDependencyDirectory(t, filepath.Join(root, "node_modules"), "remove")
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(root, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if result, err := provider.CleanupDependencies(context.Background()); err != nil || result.Directories != 1 {
		t.Fatalf("CleanupDependencies result=%#v err=%v", result, err)
	}
	if err := os.Chtimes(root, old, old); err != nil {
		t.Fatalf("Chtimes after dependency cleanup: %v", err)
	}
	cleanupResult, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if cleanupResult.Candidates != 1 || cleanupResult.Quarantined != 1 {
		t.Fatalf("Cleanup result=%#v, want one quarantined workspace", cleanupResult)
	}
	if len(store.order) != 1 {
		t.Fatalf("quarantine entries = %d, want 1", len(store.order))
	}
	entry := store.entries[store.order[0]]
	if _, err := provider.Restore(context.Background(), entry.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "source.txt")); err != nil {
		t.Fatalf("restored source: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("pruned dependency restored unexpectedly: %v", err)
	}
	owner, marked, err := readOwnershipMarker(root)
	if err != nil || !marked || owner.TaskID != "archived-task" {
		t.Fatalf("restored ownership marker=%#v marked=%v err=%v", owner, marked, err)
	}
	if store.entries[entry.ID].State != storage.QuarantineStateRestored {
		t.Fatalf("restore state = %q, want %q", store.entries[entry.ID].State, storage.QuarantineStateRestored)
	}
}

func newDependencyProviderFixture(t *testing.T) (*Provider, string, *dependencyInventorySource, *fakeQuarantineStore) {
	t.Helper()
	home := t.TempDir()
	tasksRoot := filepath.Join(home, "tasks")
	if err := os.MkdirAll(tasksRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	store := newFakeQuarantineStore()
	known := map[string]struct{}{"archived-task": {}}
	archived := map[string]struct{}{"archived-task": {}}
	inventory := &Inventory{
		Complete: true, KnownTaskIDs: known, ArchivedTaskIDs: archived,
	}
	source := &dependencyInventorySource{inventory: *inventory}
	provider := New(Config{
		TasksRoot: tasksRoot, TrashRoot: filepath.Join(home, "trash"), Store: store,
		Inventory: source, GracePeriod: 7 * 24 * time.Hour,
		Now: func() time.Time { return time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC) },
	})
	return provider, tasksRoot, source, store
}

type dependencyInventorySource struct{ inventory Inventory }

func (s *dependencyInventorySource) LoadWorkspaceInventory(context.Context) (Inventory, error) {
	return s.inventory, nil
}

func createDependencyWorkspace(t *testing.T, tasksRoot, relative, taskID string) string {
	t.Helper()
	root := filepath.Join(tasksRoot, relative)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteOwnershipMarker(root, OwnershipMarker{
		TaskID: taskID, TaskDirName: relative, LayoutVersion: LayoutVersionSemantic,
	}); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(root, old, old); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeDependencyFixture(t *testing.T, root string) {
	t.Helper()
	for _, relative := range DependencyDirectoryAllowlist() {
		writeDependencyDirectory(t, filepath.Join(root, filepath.FromSlash(relative)), "keep")
	}
	for _, relative := range []string{
		filepath.Join("src", "node_modules"), filepath.Join("vendor", "node_modules"),
		filepath.Join("target", "node_modules"), filepath.Join("build", "node_modules"),
		filepath.Join("dist", "node_modules"), filepath.Join(".cache", "node_modules"),
		filepath.Join(".git", "node_modules"),
	} {
		writeDependencyDirectory(t, filepath.Join(root, relative), "keep")
	}
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeDependencyDirectory(t *testing.T, root, contents string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "payload.txt"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertDependencyRemoved(t *testing.T, root, relative string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, relative)); !os.IsNotExist(err) {
		t.Fatalf("dependency %s still exists or returned unexpected error: %v", relative, err)
	}
}

func assertDependencyKept(t *testing.T, root, relative string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, relative)); err != nil {
		t.Fatalf("dependency %s was removed: %v", relative, err)
	}
}
