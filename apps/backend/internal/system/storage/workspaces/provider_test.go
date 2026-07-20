package workspaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

type fakeInventorySource struct {
	inventory Inventory
	err       error
}

func (s fakeInventorySource) LoadWorkspaceInventory(context.Context) (Inventory, error) {
	return s.inventory, s.err
}

type fakeQuarantineStore struct {
	entries map[string]storage.QuarantineEntry
	order   []string
}

func newFakeQuarantineStore() *fakeQuarantineStore {
	return &fakeQuarantineStore{entries: make(map[string]storage.QuarantineEntry)}
}

func (s *fakeQuarantineStore) CreateQuarantineEntry(_ context.Context, entry *storage.QuarantineEntry) error {
	if _, exists := s.entries[entry.ID]; exists {
		return errors.New("duplicate quarantine entry")
	}
	for _, existing := range s.entries {
		if existing.OriginalPath == entry.OriginalPath &&
			(existing.State == storage.QuarantineStateQuarantined || existing.State == storage.QuarantineStateFailed) {
			return errors.New("duplicate active quarantine original path")
		}
	}
	s.entries[entry.ID] = *entry
	s.order = append(s.order, entry.ID)
	return nil
}

func (s *fakeQuarantineStore) GetQuarantineEntry(_ context.Context, id string) (storage.QuarantineEntry, error) {
	entry, ok := s.entries[id]
	if !ok {
		return storage.QuarantineEntry{}, storage.ErrNotFound
	}
	return entry, nil
}

func (s *fakeQuarantineStore) TransitionQuarantineEntry(
	_ context.Context,
	id string,
	next storage.QuarantineState,
	lastError string,
) (storage.QuarantineEntry, error) {
	entry, ok := s.entries[id]
	if !ok {
		return storage.QuarantineEntry{}, storage.ErrNotFound
	}
	entry.State = next
	entry.LastError = lastError
	s.entries[id] = entry
	return entry, nil
}

func (s *fakeQuarantineStore) ListQuarantineEntries(
	_ context.Context,
	includeTerminal bool,
) ([]storage.QuarantineEntry, error) {
	entries := make([]storage.QuarantineEntry, 0, len(s.entries))
	for _, id := range s.order {
		entry := s.entries[id]
		if !includeTerminal && entry.State != storage.QuarantineStateQuarantined &&
			entry.State != storage.QuarantineStateFailed {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func TestCleanupFailsClosedWhenInventoryIsIncompleteOrErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		inventory Inventory
		err       error
	}{
		{name: "incomplete", inventory: Inventory{Complete: false}},
		{name: "error", err: errors.New("inventory unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider, root, store := newProviderFixture(t, tt.inventory, tt.err)
			candidate := createOwnedCandidate(t, root, "old-task_abc", OwnershipMarker{
				TaskID: "task-1", WorkspaceID: "workspace-1", TaskDirName: "old-task_abc",
				LayoutVersion: LayoutVersionSemantic,
			})

			if _, err := provider.Cleanup(context.Background()); err == nil {
				t.Fatal("Cleanup succeeded without complete authoritative inventory")
			}
			if _, err := os.Stat(candidate); err != nil {
				t.Fatalf("candidate moved despite inventory failure: %v", err)
			}
			if len(store.entries) != 0 {
				t.Fatalf("quarantine entries = %d, want none", len(store.entries))
			}
		})
	}
}

func TestAnalyzeReportsAllWorkspaceBytesIncludingYoungOrphans(t *testing.T) {
	provider, root, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	oldOrphan := createOwnedCandidate(t, root, "old-orphan_abc", OwnershipMarker{
		TaskID: "old", TaskDirName: "old-orphan_abc", LayoutVersion: LayoutVersionSemantic,
	})
	youngOrphan := createOwnedCandidate(t, root, "young-orphan_def", OwnershipMarker{
		TaskID: "young", TaskDirName: "young-orphan_def", LayoutVersion: LayoutVersionSemantic,
	})
	active := createOwnedCandidate(t, root, "active-task_ghi", OwnershipMarker{
		TaskID: "active", TaskDirName: "active-task_ghi", LayoutVersion: LayoutVersionSemantic,
	})
	for path, contents := range map[string]string{
		oldOrphan: "old orphan bytes", youngOrphan: "young orphan bytes", active: "active bytes",
	} {
		if err := os.WriteFile(filepath.Join(path, "artifact"), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := provider.config.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldOrphan, old, old); err != nil {
		t.Fatal(err)
	}
	provider.config.Inventory = fakeInventorySource{inventory: Inventory{
		Complete: true, EnvironmentPaths: []string{active},
	}}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	wantTotal := int64(0)
	for _, path := range []string{oldOrphan, youngOrphan, active} {
		size, sizeErr := directorySizeNoFollow(path)
		if sizeErr != nil {
			t.Fatal(sizeErr)
		}
		wantTotal += size
	}
	if analysis.TotalBytes != wantTotal {
		t.Fatalf("TotalBytes = %d, want %d", analysis.TotalBytes, wantTotal)
	}
}

func TestCleanupQuarantinesOldOwnedOrphanAndRestoreIsConflictSafe(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, root, "orphan-task_abc", OwnershipMarker{
		TaskID: "task-orphan", WorkspaceID: "workspace-1", TaskDirName: "orphan-task_abc",
		LayoutVersion: LayoutVersionSemantic,
	})
	module := filepath.Join(candidate, "repo", "node_modules", "package", "index.js")
	if err := os.MkdirAll(filepath.Dir(module), 0o755); err != nil {
		t.Fatalf("MkdirAll node_modules: %v", err)
	}
	if err := os.WriteFile(module, []byte("large dependency"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Quarantined != 1 || result.ReclaimedBytes < int64(len("large dependency")) {
		t.Fatalf("cleanup result = %#v", result)
	}
	entry := store.entries["entry-1"]
	if entry.TaskID != "task-orphan" || entry.OriginalPath != candidate {
		t.Fatalf("persisted entry = %#v", entry)
	}
	if _, err := os.Stat(candidate); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original still exists after quarantine: %v", err)
	}
	if _, err := os.Stat(entry.QuarantinePath); err != nil {
		t.Fatalf("quarantine missing: %v", err)
	}

	if err := os.MkdirAll(candidate, 0o755); err != nil {
		t.Fatalf("create restore conflict: %v", err)
	}
	if _, err := provider.Restore(context.Background(), entry.ID); !errors.Is(err, ErrRestoreConflict) {
		t.Fatalf("Restore conflict error = %v, want ErrRestoreConflict", err)
	}
	if got := store.entries[entry.ID]; got.State != storage.QuarantineStateQuarantined {
		t.Fatalf("conflicted entry state = %q, want quarantined", got.State)
	}
	if err := os.Remove(candidate); err != nil {
		t.Fatalf("remove conflict: %v", err)
	}
	restored, err := provider.Restore(context.Background(), entry.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored.State != storage.QuarantineStateRestored {
		t.Fatalf("restored state = %q", restored.State)
	}
	if data, err := os.ReadFile(module); err != nil || string(data) != "large dependency" {
		t.Fatalf("restored node_modules data = %q, %v", data, err)
	}
}

func TestCleanupRetriesFailedWorkspaceIntentWhenMoveNeverHappened(t *testing.T) {
	provider, tasksRoot, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, tasksRoot, "retry-task_abc", OwnershipMarker{
		TaskID: "retry-task", TaskDirName: "retry-task_abc", LayoutVersion: LayoutVersionSemantic,
	})
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("age candidate: %v", err)
	}
	blockedQuarantine := filepath.Join(provider.config.TrashRoot, "tasks", "entry-1")
	if err := os.MkdirAll(blockedQuarantine, 0o700); err != nil {
		t.Fatalf("create blocked quarantine: %v", err)
	}
	if err := os.WriteFile(filepath.Join(blockedQuarantine, "blocker"), []byte("block"), 0o600); err != nil {
		t.Fatalf("write quarantine blocker: %v", err)
	}

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("first Cleanup succeeded despite blocked quarantine destination")
	}
	if got := store.entries["entry-1"].State; got != storage.QuarantineStateFailed {
		t.Fatalf("first intent state = %q, want failed", got)
	}
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("re-age candidate for ambiguous retry: %v", err)
	}
	if _, err := provider.Cleanup(context.Background()); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("ambiguous retry error = %v, want ErrConflict", err)
	}
	if got := store.entries["entry-1"].State; got != storage.QuarantineStateFailed {
		t.Fatalf("ambiguous intent state = %q, want failed", got)
	}
	if err := os.RemoveAll(blockedQuarantine); err != nil {
		t.Fatalf("remove quarantine blocker: %v", err)
	}
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("re-age candidate: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("retry Cleanup: %v", err)
	}
	if result.Quarantined != 1 {
		t.Fatalf("retry result = %#v, want one quarantined workspace", result)
	}
	if got := store.entries["entry-1"].State; got != storage.QuarantineStateRestored {
		t.Fatalf("released intent state = %q, want restored", got)
	}
	if got := store.entries["entry-2"].State; got != storage.QuarantineStateQuarantined {
		t.Fatalf("replacement intent state = %q, want quarantined", got)
	}
}

func TestCleanupProtectsEveryInventorySourceAndScratchSiblingsIndependently(t *testing.T) {
	home := t.TempDir()
	tasksRoot := filepath.Join(home, "tasks")
	trashRoot := filepath.Join(home, "trash")
	store := newFakeQuarantineStore()
	now := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	type rootSpec struct {
		rel    string
		marker OwnershipMarker
	}
	roots := []rootSpec{
		{rel: "worktree-task_abc", marker: OwnershipMarker{TaskID: "task-worktree", TaskDirName: "worktree-task_abc", LayoutVersion: LayoutVersionSemantic}},
		{rel: "environment-task_def", marker: OwnershipMarker{TaskID: "task-environment", TaskDirName: "environment-task_def", LayoutVersion: LayoutVersionSemantic}},
		{rel: "execution-task_ghi", marker: OwnershipMarker{TaskID: "task-execution", TaskDirName: "execution-task_ghi", LayoutVersion: LayoutVersionSemantic}},
		{rel: filepath.Join("workspace-1", "task-active"), marker: OwnershipMarker{TaskID: "task-active", WorkspaceID: "workspace-1", TaskDirName: "task-active", LayoutVersion: LayoutVersionScratch}},
		{rel: filepath.Join("workspace-1", "task-orphan"), marker: OwnershipMarker{TaskID: "task-orphan", WorkspaceID: "workspace-1", TaskDirName: "task-orphan", LayoutVersion: LayoutVersionScratch}},
	}
	paths := make(map[string]string)
	for _, spec := range roots {
		path := createOwnedCandidate(t, tasksRoot, spec.rel, spec.marker)
		old := now.Add(-8 * 24 * time.Hour)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("Chtimes(%s): %v", path, err)
		}
		paths[spec.marker.TaskID] = path
	}
	inventory := Inventory{
		Complete:         true,
		WorktreePaths:    []string{filepath.Join(paths["task-worktree"], "repo")},
		EnvironmentPaths: []string{paths["task-environment"]},
		ExecutionPaths:   []string{filepath.Join(paths["task-execution"], "repo")},
		ScratchRoots: []ScratchRoot{{
			TaskID: "task-active", WorkspaceID: "workspace-1", Path: paths["task-active"],
		}},
	}
	provider := New(Config{
		TasksRoot: tasksRoot, TrashRoot: trashRoot, Store: store,
		Inventory: fakeInventorySource{inventory: inventory}, GracePeriod: 7 * 24 * time.Hour,
		Now: func() time.Time { return now }, NewID: func() string { return "entry-orphan" },
	})
	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Quarantined != 1 || store.entries["entry-orphan"].TaskID != "task-orphan" {
		t.Fatalf("cleanup result=%#v entries=%#v", result, store.entries)
	}
	for _, taskID := range []string{"task-worktree", "task-environment", "task-execution", "task-active"} {
		if _, err := os.Stat(paths[taskID]); err != nil {
			t.Fatalf("protected %s root moved: %v", taskID, err)
		}
	}
}

func TestCleanupQuarantinesWorkspaceWithoutFollowingNestedSymlink(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, root, "linked-task_abc", OwnershipMarker{
		TaskID: "linked", TaskDirName: "linked-task_abc", LayoutVersion: LayoutVersionSemantic,
	})
	external := t.TempDir()
	externalArtifact := filepath.Join(external, "keep")
	if err := os.WriteFile(externalArtifact, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(candidate, "external-link")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes candidate: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Quarantined != 1 || len(store.entries) != 1 {
		t.Fatalf("cleanup result=%#v entries=%#v", result, store.entries)
	}
	if _, err := os.Stat(externalArtifact); err != nil {
		t.Fatalf("cleanup followed nested symlink target: %v", err)
	}
	entry := store.entries["entry-1"]
	if _, err := os.Lstat(filepath.Join(entry.QuarantinePath, "external-link")); err != nil {
		t.Fatalf("symlink entry was not quarantined: %v", err)
	}
	provider.config.Now = func() time.Time { return entry.DeleteAfter }
	if _, err := provider.PermanentDelete(context.Background(), entry.ID, "DELETE"); err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}
	if _, err := os.Stat(externalArtifact); err != nil {
		t.Fatalf("permanent deletion followed nested symlink target: %v", err)
	}
}

func TestCleanupRejectsSymlinkedQuarantineManifestWithoutFollowingTarget(t *testing.T) {
	provider, root, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, root, "linked-manifest_abc", OwnershipMarker{
		TaskID: "linked-manifest", TaskDirName: "linked-manifest_abc", LayoutVersion: LayoutVersionSemantic,
	})
	externalArtifact := filepath.Join(t.TempDir(), "keep")
	const original = "external"
	if err := os.WriteFile(externalArtifact, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalArtifact, filepath.Join(candidate, quarantineManifestName)); err != nil {
		t.Fatalf("Symlink quarantine manifest: %v", err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes candidate: %v", err)
	}

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("Cleanup succeeded with a symlinked quarantine manifest")
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate moved after manifest validation failed: %v", err)
	}
	contents, err := os.ReadFile(externalArtifact)
	if err != nil {
		t.Fatalf("read external target: %v", err)
	}
	if string(contents) != original {
		t.Fatalf("quarantine manifest write followed symlink target: %q", contents)
	}
}

func TestCleanupRejectsSymlinkedTrashPathsBeforeAnyMutation(t *testing.T) {
	for _, test := range []struct {
		name       string
		linkTarget func(t *testing.T, trashRoot, external string)
	}{
		{
			name: "trash root",
			linkTarget: func(t *testing.T, trashRoot, external string) {
				t.Helper()
				if err := os.Symlink(external, trashRoot); err != nil {
					t.Fatalf("symlink trash root: %v", err)
				}
			},
		},
		{
			name: "tasks beneath trash root",
			linkTarget: func(t *testing.T, trashRoot, external string) {
				t.Helper()
				if err := os.MkdirAll(trashRoot, 0o700); err != nil {
					t.Fatalf("create trash root: %v", err)
				}
				if err := os.Symlink(external, filepath.Join(trashRoot, "tasks")); err != nil {
					t.Fatalf("symlink task trash: %v", err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider, tasksRoot, store := newProviderFixture(t, Inventory{Complete: true}, nil)
			candidate := createOwnedCandidate(t, tasksRoot, "unsafe-trash_abc", OwnershipMarker{
				TaskID: "task-unsafe", TaskDirName: "unsafe-trash_abc", LayoutVersion: LayoutVersionSemantic,
			})
			old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
			if err := os.Chtimes(candidate, old, old); err != nil {
				t.Fatalf("age candidate: %v", err)
			}
			external := t.TempDir()
			test.linkTarget(t, provider.config.TrashRoot, external)

			if _, err := provider.Cleanup(context.Background()); err == nil {
				t.Fatal("Cleanup succeeded with a symlinked trash path")
			}
			if _, err := os.Stat(candidate); err != nil {
				t.Fatalf("candidate changed despite unsafe trash: %v", err)
			}
			if len(store.entries) != 0 {
				t.Fatalf("persisted %d entries before trash validation", len(store.entries))
			}
			externalEntries, err := os.ReadDir(external)
			if err != nil || len(externalEntries) != 0 {
				t.Fatalf("external trash target changed: entries=%v err=%v", externalEntries, err)
			}
		})
	}
}

func TestCleanupRejectsSymlinkedOwnershipMarker(t *testing.T) {
	provider, tasksRoot, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := filepath.Join(tasksRoot, "marker-link_abc")
	if err := os.MkdirAll(candidate, 0o755); err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	externalMarker := filepath.Join(t.TempDir(), "marker.json")
	marker := OwnershipMarker{
		TaskID: "task-link", TaskDirName: "marker-link_abc", LayoutVersion: LayoutVersionSemantic,
	}
	if err := writeJSONFile(externalMarker, marker); err != nil {
		t.Fatalf("write external marker: %v", err)
	}
	if err := os.Symlink(externalMarker, filepath.Join(candidate, OwnershipMarkerFilename)); err != nil {
		t.Fatalf("symlink marker: %v", err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("age candidate: %v", err)
	}

	if _, err := provider.Cleanup(context.Background()); err == nil {
		t.Fatal("Cleanup accepted a symlinked ownership marker")
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate changed despite symlinked marker: %v", err)
	}
	if len(store.entries) != 0 {
		t.Fatalf("persisted %d entries for symlinked marker", len(store.entries))
	}
}

func TestCleanupSupportsLegacySemanticAndScratchLayouts(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	semantic := filepath.Join(root, "legacy-task_abc")
	scratch := filepath.Join(root, "workspace-legacy", "task-legacy")
	for _, path := range []string{semantic, scratch} {
		if err := os.MkdirAll(filepath.Join(path, ".git"), 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", path, err)
		}
		old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("Chtimes(%s): %v", path, err)
		}
	}
	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Quarantined != 2 || len(store.entries) != 2 {
		t.Fatalf("legacy cleanup result=%#v entries=%#v", result, store.entries)
	}
}

type recordingPruner struct{ calls int }

func (p *recordingPruner) PruneQuarantinedWorkspace(context.Context, storage.QuarantineEntry) error {
	p.calls++
	return nil
}

func TestPermanentDeleteRequiresConfirmationAndPreservesRecoveryMetadata(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	pruner := &recordingPruner{}
	provider.config.Pruner = pruner
	candidate := createOwnedCandidate(t, root, "delete-task_abc", OwnershipMarker{TaskID: "delete-task", TaskDirName: "delete-task_abc", LayoutVersion: LayoutVersionSemantic})
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	entry := store.entries["entry-1"]
	provider.config.Now = func() time.Time { return entry.DeleteAfter.Add(time.Hour) }
	if _, err := provider.PermanentDelete(context.Background(), entry.ID, "wrong"); !errors.Is(err, ErrDeleteConfirmation) {
		t.Fatalf("PermanentDelete confirmation error = %v", err)
	}
	if _, err := os.Stat(entry.QuarantinePath); err != nil {
		t.Fatalf("quarantine changed without confirmation: %v", err)
	}
	deleted, err := provider.PermanentDelete(context.Background(), entry.ID, "DELETE")
	if err != nil {
		t.Fatalf("PermanentDelete: %v", err)
	}
	if deleted.State != storage.QuarantineStateDeleted || pruner.calls != 1 {
		t.Fatalf("deleted=%#v prune calls=%d", deleted, pruner.calls)
	}
	if entry.TaskID != "delete-task" || entry.Metadata == nil {
		t.Fatalf("historical quarantine identity lost: %#v", entry)
	}
}

func TestPermanentDeleteBeforeRetentionReturnsConflictAndKeepsQuarantine(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, root, "retained-task_abc", OwnershipMarker{
		TaskID: "retained-task", TaskDirName: "retained-task_abc", LayoutVersion: LayoutVersionSemantic,
	})
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	entry := store.entries["entry-1"]

	_, err := provider.PermanentDelete(context.Background(), entry.ID, "DELETE")
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("PermanentDelete error = %v, want storage.ErrConflict", err)
	}
	if _, err := os.Stat(entry.QuarantinePath); err != nil {
		t.Fatalf("quarantine path changed before deadline: %v", err)
	}
	if got := store.entries[entry.ID]; got.State != storage.QuarantineStateQuarantined {
		t.Fatalf("quarantine state = %q, want quarantined", got.State)
	}
}

func TestReconcileRecreatesMissingRecordFromQuarantineManifest(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, root, "reconcile-task_abc", OwnershipMarker{TaskID: "reconcile-task", TaskDirName: "reconcile-task_abc", LayoutVersion: LayoutVersionSemantic})
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	delete(store.entries, "entry-1")
	result, err := provider.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if result.Recovered != 1 || store.entries["entry-1"].TaskID != "reconcile-task" {
		t.Fatalf("reconcile result=%#v entries=%#v", result, store.entries)
	}
}

func TestRestoreTaskReportsRestoredNotFoundAndFailed(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	candidate := createOwnedCandidate(t, root, "restore-task_abc", OwnershipMarker{TaskID: "restore-task", TaskDirName: "restore-task_abc", LayoutVersion: LayoutVersionSemantic})
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(candidate, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	if _, err := provider.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if got := provider.RestoreTask(context.Background(), "missing-task"); got.Status != "not_found" {
		t.Fatalf("missing recovery = %#v", got)
	}
	if got := provider.RestoreTask(context.Background(), "restore-task"); got.Status != "restored" {
		t.Fatalf("restore recovery = %#v", got)
	}
	entry := store.entries["entry-1"]
	entry.State = storage.QuarantineStateQuarantined
	store.entries[entry.ID] = entry
	if got := provider.RestoreTask(context.Background(), "restore-task"); got.Status != "failed" {
		t.Fatalf("conflicted recovery = %#v", got)
	}
}

func TestRestoreTaskChoosesNewestQuarantinedEntry(t *testing.T) {
	provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	original := filepath.Join(root, "restore-task_abc")
	trashTasks := filepath.Join(filepath.Dir(root), "trash", "tasks")
	oldFailed := storage.QuarantineEntry{
		ID: "old-failed", ResourceType: storage.ResourceTypeTaskWorkspace, TaskID: "restore-task",
		OriginalPath: original, QuarantinePath: filepath.Join(trashTasks, "old-failed"),
		State: storage.QuarantineStateFailed, QuarantinedAt: time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
	}
	newest := storage.QuarantineEntry{
		ID: "newest", ResourceType: storage.ResourceTypeTaskWorkspace, TaskID: "restore-task",
		OriginalPath: original, QuarantinePath: filepath.Join(trashTasks, "newest"),
		State: storage.QuarantineStateQuarantined, QuarantinedAt: time.Date(2026, time.July, 2, 0, 0, 0, 0, time.UTC),
	}
	store.entries[oldFailed.ID] = oldFailed
	store.entries[newest.ID] = newest
	store.order = append(store.order, oldFailed.ID, newest.ID)
	if err := os.MkdirAll(newest.QuarantinePath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newest.QuarantinePath, "artifact"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := provider.RestoreTask(context.Background(), "restore-task"); got.Status != "restored" {
		t.Fatalf("RestoreTask = %#v, want restored", got)
	}
	if _, err := os.Stat(filepath.Join(original, "artifact")); err != nil {
		t.Fatalf("restored newest workspace: %v", err)
	}
}

func TestRestoreTaskReconcilesFailedEntryFilesystemState(t *testing.T) {
	tests := []struct {
		name             string
		createOriginal   bool
		createQuarantine bool
		wantStatus       string
		wantState        storage.QuarantineState
	}{
		{name: "original only", createOriginal: true, wantStatus: "restored", wantState: storage.QuarantineStateRestored},
		{name: "quarantine only", createQuarantine: true, wantStatus: "restored", wantState: storage.QuarantineStateRestored},
		{
			name: "both paths", createOriginal: true, createQuarantine: true,
			wantStatus: "failed", wantState: storage.QuarantineStateFailed,
		},
		{name: "neither path", wantStatus: "failed", wantState: storage.QuarantineStateFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, root, store := newProviderFixture(t, Inventory{Complete: true}, nil)
			original := filepath.Join(root, "restore-failed_abc")
			quarantined := filepath.Join(filepath.Dir(root), "trash", "tasks", "failed-entry")
			if test.createOriginal {
				writeWorkspaceArtifact(t, original, "original")
			}
			if test.createQuarantine {
				writeWorkspaceArtifact(t, quarantined, "quarantine")
			}
			entry := storage.QuarantineEntry{
				ID: "failed-entry", ResourceType: storage.ResourceTypeTaskWorkspace, TaskID: "restore-failed",
				OriginalPath: original, QuarantinePath: quarantined, State: storage.QuarantineStateFailed,
				QuarantinedAt: time.Now().UTC(),
			}
			store.entries[entry.ID] = entry
			store.order = append(store.order, entry.ID)

			if got := provider.RestoreTask(context.Background(), entry.TaskID); got.Status != test.wantStatus {
				t.Fatalf("RestoreTask = %#v, want status %q", got, test.wantStatus)
			}
			if got := store.entries[entry.ID].State; got != test.wantState {
				t.Fatalf("entry state = %q, want %q", got, test.wantState)
			}
			if test.wantStatus == "restored" {
				assertRestoredWorkspace(t, original, quarantined, test.createQuarantine)
				return
			}
			if test.createOriginal {
				if data, err := os.ReadFile(filepath.Join(original, "artifact")); err != nil || string(data) != "original" {
					t.Fatalf("original artifact changed: data=%q err=%v", data, err)
				}
			}
			if test.createQuarantine {
				if data, err := os.ReadFile(filepath.Join(quarantined, "artifact")); err != nil || string(data) != "quarantine" {
					t.Fatalf("quarantine artifact changed: data=%q err=%v", data, err)
				}
			}
		})
	}
}

func assertRestoredWorkspace(t *testing.T, original, quarantined string, restoredFromQuarantine bool) {
	t.Helper()
	wantContents := "original"
	if restoredFromQuarantine {
		wantContents = "quarantine"
	}
	if data, err := os.ReadFile(filepath.Join(original, "artifact")); err != nil || string(data) != wantContents {
		t.Fatalf("restored artifact: data=%q err=%v", data, err)
	}
	if !restoredFromQuarantine {
		return
	}
	if _, err := os.Lstat(quarantined); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantine path remains after restore: %v", err)
	}
}

func writeWorkspaceArtifact(t *testing.T, root, contents string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "artifact"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newProviderFixture(
	t *testing.T,
	inventory Inventory,
	inventoryErr error,
) (*Provider, string, *fakeQuarantineStore) {
	t.Helper()
	home := t.TempDir()
	tasksRoot := filepath.Join(home, "tasks")
	trashRoot := filepath.Join(home, "trash")
	if err := os.MkdirAll(tasksRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll tasks: %v", err)
	}
	store := newFakeQuarantineStore()
	now := time.Date(2026, time.July, 14, 0, 0, 0, 0, time.UTC)
	nextID := 0
	provider := New(Config{
		TasksRoot: tasksRoot, TrashRoot: trashRoot,
		Inventory: fakeInventorySource{inventory: inventory, err: inventoryErr}, Store: store,
		GracePeriod: 7 * 24 * time.Hour, Retention: 7 * 24 * time.Hour,
		Now: func() time.Time { return now }, NewID: func() string {
			nextID++
			return fmt.Sprintf("entry-%d", nextID)
		},
	})
	return provider, tasksRoot, store
}

func createOwnedCandidate(t *testing.T, tasksRoot, relative string, marker OwnershipMarker) string {
	t.Helper()
	root := filepath.Join(tasksRoot, relative)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll candidate: %v", err)
	}
	if err := WriteOwnershipMarker(root, marker); err != nil {
		t.Fatalf("WriteOwnershipMarker: %v", err)
	}
	return root
}
