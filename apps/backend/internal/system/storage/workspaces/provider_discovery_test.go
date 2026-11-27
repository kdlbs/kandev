package workspaces

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.7
// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
func TestAnalyzeKeepsValidWorkspacesWithUnclassifiedCheckout(t *testing.T) {
	provider, tasksRoot, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	workspace := createOwnedCandidate(t, tasksRoot, "recognized-task_abc", OwnershipMarker{
		TaskID: "recognized-task", WorkspaceID: "recognized-workspace",
		TaskDirName: "recognized-task_abc", LayoutVersion: LayoutVersionSemantic,
		CreatedAt: time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
	})
	const workspacePayload = "recognized workspace payload"
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte(workspacePayload), 0o600); err != nil {
		t.Fatalf("write workspace payload: %v", err)
	}
	markerInfo, err := os.Stat(filepath.Join(workspace, OwnershipMarkerFilename))
	if err != nil {
		t.Fatalf("stat ownership marker: %v", err)
	}

	checkout := filepath.Join(tasksRoot, "kandev-host-v0.96.0")
	for _, name := range []string{".git", "apps", "node_modules"} {
		if err := os.MkdirAll(filepath.Join(checkout, name), 0o755); err != nil {
			t.Fatalf("create checkout directory %s: %v", name, err)
		}
	}
	external := t.TempDir()
	externalAgentGuide := filepath.Join(external, "AGENTS.md")
	const externalPayload = "external guide must not be measured or changed"
	if err := os.WriteFile(externalAgentGuide, []byte(externalPayload), 0o600); err != nil {
		t.Fatalf("write external guide: %v", err)
	}
	if err := os.Symlink(externalAgentGuide, filepath.Join(checkout, "CLAUDE.md")); err != nil {
		t.Fatalf("symlink checkout guide: %v", err)
	}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	wantBytes := int64(len(workspacePayload)) + markerInfo.Size()
	if analysis.TotalBytes != wantBytes {
		t.Fatalf("TotalBytes = %d, want recognized workspace bytes %d", analysis.TotalBytes, wantBytes)
	}
	if !containsWarning(analysis.Warnings, "unclassified task directory kept: "+checkout) {
		t.Fatalf("Warnings = %#v, want omission warning for %s", analysis.Warnings, checkout)
	}
	if got, err := os.ReadFile(externalAgentGuide); err != nil || string(got) != externalPayload {
		t.Fatalf("external guide changed: contents=%q err=%v", got, err)
	}
}

func containsWarning(warnings []string, want string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, want) {
			return true
		}
	}
	return false
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestUnclassifiedDirectoriesAreNotMeasuredOrCleaned(t *testing.T) {
	provider, tasksRoot, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	workspace := createOwnedCandidate(t, tasksRoot, "active-task_abc", OwnershipMarker{
		TaskID: "active-task", TaskDirName: "active-task_abc", LayoutVersion: LayoutVersionSemantic,
	})
	const workspacePayload = "recognized bytes"
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte(workspacePayload), 0o600); err != nil {
		t.Fatalf("write workspace payload: %v", err)
	}
	markerInfo, err := os.Stat(filepath.Join(workspace, OwnershipMarkerFilename))
	if err != nil {
		t.Fatalf("stat ownership marker: %v", err)
	}
	provider.config.Inventory = fakeInventorySource{inventory: Inventory{
		Complete: true, WorktreePaths: []string{workspace},
	}}

	checkout := filepath.Join(tasksRoot, "unrelated-checkout")
	old := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range []string{".git", "apps", "node_modules"} {
		directory := filepath.Join(checkout, name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create checkout directory %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(directory, "keep.txt"), []byte("unclassified payload"), 0o600); err != nil {
			t.Fatalf("write checkout payload %s: %v", name, err)
		}
		if err := os.Chtimes(directory, old, old); err != nil {
			t.Fatalf("age checkout directory %s: %v", name, err)
		}
	}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	wantBytes := int64(len(workspacePayload)) + markerInfo.Size()
	if analysis.TotalBytes != wantBytes {
		t.Fatalf("TotalBytes = %d, want recognized workspace bytes %d", analysis.TotalBytes, wantBytes)
	}
	if !containsWarning(analysis.Warnings, "unclassified task directory kept: "+checkout) {
		t.Fatalf("Warnings = %#v, want omission warning for %s", analysis.Warnings, checkout)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Candidates != 0 || result.Quarantined != 0 {
		t.Fatalf("Cleanup result = %#v, want no unclassified candidates", result)
	}
	for _, name := range []string{".git", "apps", "node_modules"} {
		payload := filepath.Join(checkout, name, "keep.txt")
		if contents, err := os.ReadFile(payload); err != nil || string(contents) != "unclassified payload" {
			t.Fatalf("checkout payload %s changed: contents=%q err=%v", name, contents, err)
		}
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestWorkspaceDiscoveryPreservesSupportedLayoutsAndWarnsUnmarkedChildren(t *testing.T) {
	tasksRoot := t.TempDir()
	markedSemantic := createOwnedCandidate(t, tasksRoot, "marked-task_abc", OwnershipMarker{
		TaskID: "marked-task", TaskDirName: "marked-task_abc", LayoutVersion: LayoutVersionSemantic,
	})
	legacySemantic := filepath.Join(tasksRoot, "legacy-task_def")
	if err := os.Mkdir(legacySemantic, 0o755); err != nil {
		t.Fatalf("create legacy semantic root: %v", err)
	}
	markedScratch := createOwnedCandidate(t, tasksRoot, filepath.Join("named-workspace", "marked-child"), OwnershipMarker{
		TaskID: "marked-scratch", WorkspaceID: "named-workspace", TaskDirName: "marked-child",
		LayoutVersion: LayoutVersionScratch,
	})
	if err := os.Mkdir(filepath.Join(tasksRoot, "named-workspace", "node_modules"), 0o755); err != nil {
		t.Fatalf("create unmarked scratch sibling: %v", err)
	}
	workspaceID := "33333333-3333-4333-8333-333333333333"
	taskID := "44444444-4444-4444-8444-444444444444"
	legacyScratch := filepath.Join(tasksRoot, workspaceID, taskID)
	if err := os.MkdirAll(legacyScratch, 0o755); err != nil {
		t.Fatalf("create legacy scratch root: %v", err)
	}
	unknownChild := filepath.Join(tasksRoot, workspaceID, "apps")
	if err := os.Mkdir(unknownChild, 0o755); err != nil {
		t.Fatalf("create unmarked uuid-container sibling: %v", err)
	}
	customScratch := filepath.Join(tasksRoot, "workspace-legacy", "task-legacy")
	if err := os.MkdirAll(customScratch, 0o755); err != nil {
		t.Fatalf("create unsupported non-UUID scratch root: %v", err)
	}

	roots, warnings, err := discoverTaskRoots(tasksRoot)
	if err != nil {
		t.Fatalf("discoverTaskRoots: %v", err)
	}
	gotPaths := make([]string, 0, len(roots))
	for _, root := range roots {
		gotPaths = append(gotPaths, root.path)
	}
	wantPaths := []string{markedSemantic, legacySemantic, markedScratch, legacyScratch}
	sort.Strings(gotPaths)
	sort.Strings(wantPaths)
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("recognized roots = %#v, want %#v", gotPaths, wantPaths)
	}
	if !containsWarning(warnings, "unclassified task directory kept: "+filepath.Join(tasksRoot, "named-workspace", "node_modules")) {
		t.Fatalf("Warnings = %#v, missing marked-container sibling warning", warnings)
	}
	if !containsWarning(warnings, "unclassified task directory kept: "+unknownChild) {
		t.Fatalf("Warnings = %#v, missing UUID-container sibling warning", warnings)
	}
	if !containsWarning(warnings, "unclassified task directory kept: "+filepath.Dir(customScratch)) {
		t.Fatalf("Warnings = %#v, missing non-UUID scratch omission warning", warnings)
	}
	for _, root := range roots {
		if root.path == legacyScratch && (root.owner.TaskID != taskID || root.owner.WorkspaceID != workspaceID) {
			t.Fatalf("legacy scratch owner = %#v, want UUID task/workspace IDs", root.owner)
		}
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestIsCanonicalUUIDAcceptsOnlyDashedUUIDSyntax(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: "33333333-3333-4333-8333-333333333333", want: true},
		{value: "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF", want: true},
		{value: "33333333333343338333333333333333"},
		{value: "urn:uuid:33333333-3333-4333-8333-333333333333"},
		{value: "workspace-33333333-3333-4333-8333-333333333333"},
		{value: ""},
	} {
		if got := isCanonicalUUID(test.value); got != test.want {
			t.Errorf("isCanonicalUUID(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestWorkspaceDiscoveryRejectsSymlinksInRecognizedScratchContainers(t *testing.T) {
	for _, test := range []struct {
		name        string
		workspaceID string
		markedChild bool
	}{
		{name: "uuid parent", workspaceID: "33333333-3333-4333-8333-333333333333"},
		{name: "marker-backed parent", workspaceID: "named-workspace", markedChild: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tasksRoot := t.TempDir()
			workspace := filepath.Join(tasksRoot, test.workspaceID)
			if err := os.MkdirAll(workspace, 0o755); err != nil {
				t.Fatalf("create workspace container: %v", err)
			}
			external := t.TempDir()
			externalFile := filepath.Join(external, "outside.txt")
			if err := os.WriteFile(externalFile, []byte("outside"), 0o600); err != nil {
				t.Fatalf("write external sentinel: %v", err)
			}
			if err := os.Symlink(external, filepath.Join(workspace, "a-link")); err != nil {
				t.Fatalf("create child symlink: %v", err)
			}
			if test.markedChild {
				createOwnedCandidate(t, tasksRoot, filepath.Join(test.workspaceID, "z-task"), OwnershipMarker{
					TaskID: "task-marked", WorkspaceID: test.workspaceID, TaskDirName: "z-task",
					LayoutVersion: LayoutVersionScratch,
				})
			} else if err := os.Mkdir(filepath.Join(workspace, "z-task"), 0o755); err != nil {
				t.Fatalf("create UUID-container child: %v", err)
			}

			if _, _, err := discoverTaskRoots(tasksRoot); err == nil || !strings.Contains(err.Error(), "symlink beneath tasks root") {
				t.Fatalf("discoverTaskRoots error = %v, want recognized-container symlink rejection", err)
			}
			if contents, err := os.ReadFile(externalFile); err != nil || string(contents) != "outside" {
				t.Fatalf("symlink target changed: contents=%q err=%v", contents, err)
			}
		})
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestWorkspaceDiscoveryRejectsSemanticMarkerInsideScratchContainer(t *testing.T) {
	tasksRoot := t.TempDir()
	workspaceID := "33333333-3333-4333-8333-333333333333"
	childPath := createOwnedCandidate(t, tasksRoot, filepath.Join(workspaceID, "semantic-task_abc"), OwnershipMarker{
		TaskID: "semantic-task", WorkspaceID: workspaceID,
		TaskDirName: "semantic-task_abc", LayoutVersion: LayoutVersionSemantic,
	})

	if _, _, err := discoverTaskRoots(tasksRoot); err == nil || !strings.Contains(err.Error(), "unexpected nested semantic workspace marker: "+childPath) {
		t.Fatalf("discoverTaskRoots error = %v, want nested semantic marker rejection for %s", err, childPath)
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
func TestWorkspaceDiscoveryOmitsUnreadableUnclassifiedCheckout(t *testing.T) {
	provider, tasksRoot, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	workspace := createOwnedCandidate(t, tasksRoot, "recognized-task_abc", OwnershipMarker{
		TaskID: "recognized-task", WorkspaceID: "recognized-workspace",
		TaskDirName: "recognized-task_abc", LayoutVersion: LayoutVersionSemantic,
	})
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("recognized workspace"), 0o600); err != nil {
		t.Fatalf("write workspace payload: %v", err)
	}
	markerInfo, err := os.Stat(filepath.Join(workspace, OwnershipMarkerFilename))
	if err != nil {
		t.Fatalf("stat ownership marker: %v", err)
	}

	checkout := filepath.Join(tasksRoot, "unrelated-checkout")
	if err := os.Mkdir(checkout, 0o700); err != nil {
		t.Fatalf("create unrelated checkout: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(checkout, 0o700); err != nil {
			t.Errorf("restore checkout permissions: %v", err)
		}
	})
	if err := os.Chmod(checkout, 0o300); err != nil {
		t.Fatalf("remove checkout read permission: %v", err)
	}
	if _, err := os.ReadDir(checkout); !errors.Is(err, os.ErrPermission) {
		if err == nil {
			t.Skip("filesystem permission bits are not enforced by this executor")
		}
		t.Fatalf("ReadDir unreadable checkout error = %v, want permission denied", err)
	}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze with unreadable unclassified checkout: %v", err)
	}
	wantBytes := int64(len("recognized workspace")) + markerInfo.Size()
	if analysis.TotalBytes != wantBytes {
		t.Fatalf("TotalBytes = %d, want recognized workspace bytes %d", analysis.TotalBytes, wantBytes)
	}
	if !containsWarning(analysis.Warnings, "unclassified task directory kept: "+checkout) {
		t.Fatalf("Warnings = %#v, want omission warning for %s", analysis.Warnings, checkout)
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
func TestWorkspaceDiscoveryOmitsUnreadableUnmarkedScratchSibling(t *testing.T) {
	provider, tasksRoot, _ := newProviderFixture(t, Inventory{Complete: true}, nil)
	workspaceID := "33333333-3333-4333-8333-333333333333"
	taskID := "44444444-4444-4444-8444-444444444444"
	workspace := filepath.Join(tasksRoot, workspaceID, taskID)
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("create recognized workspace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "source.txt"), []byte("recognized workspace"), 0o600); err != nil {
		t.Fatalf("write workspace payload: %v", err)
	}
	unreadable := filepath.Join(tasksRoot, workspaceID, "apps")
	if err := os.Mkdir(unreadable, 0o700); err != nil {
		t.Fatalf("create unmarked scratch sibling: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(unreadable, 0o700); err != nil {
			t.Errorf("restore sibling permissions: %v", err)
		}
	})
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("remove sibling access: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(unreadable, OwnershipMarkerFilename)); !errors.Is(err, os.ErrPermission) {
		if err == nil || errors.Is(err, os.ErrNotExist) {
			t.Skip("filesystem permission bits are not enforced by this executor")
		}
		t.Fatalf("Lstat unreadable sibling marker error = %v, want permission denied", err)
	}

	analysis, err := provider.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze with unreadable scratch sibling: %v", err)
	}
	wantBytes := int64(len("recognized workspace"))
	if analysis.TotalBytes != wantBytes {
		t.Fatalf("TotalBytes = %d, want recognized workspace bytes %d", analysis.TotalBytes, wantBytes)
	}
	if !containsWarning(analysis.Warnings, "unclassified task directory kept: "+unreadable) {
		t.Fatalf("Warnings = %#v, want omission warning for %s", analysis.Warnings, unreadable)
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.9
func TestWorkspaceDiscoveryRetainsPermissionErrorsForRecognizedRoots(t *testing.T) {
	tasksRoot := t.TempDir()
	workspace := filepath.Join(tasksRoot, "recognized-task_abc")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatalf("create recognized workspace: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(workspace, 0o700); err != nil {
			t.Errorf("restore workspace permissions: %v", err)
		}
	})
	if err := os.Chmod(workspace, 0o000); err != nil {
		t.Fatalf("remove workspace access: %v", err)
	}
	if _, err := os.ReadDir(workspace); !errors.Is(err, os.ErrPermission) {
		if err == nil {
			t.Skip("filesystem permission bits are not enforced by this executor")
		}
		t.Fatalf("ReadDir inaccessible workspace error = %v, want permission denied", err)
	}

	if _, _, err := discoverTaskRoots(tasksRoot); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("discoverTaskRoots error = %v, want recognized semantic-root permission error", err)
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
func TestWorkspaceDiscoveryRejectsDirectTaskRootSymlink(t *testing.T) {
	tasksRoot := t.TempDir()
	external := t.TempDir()
	externalFile := filepath.Join(external, "outside.txt")
	if err := os.WriteFile(externalFile, []byte("outside"), 0o600); err != nil {
		t.Fatalf("write external sentinel: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(tasksRoot, "linked-task_abc")); err != nil {
		t.Fatalf("create direct task-root symlink: %v", err)
	}

	if _, _, err := discoverTaskRoots(tasksRoot); err == nil || !strings.Contains(err.Error(), "symlink beneath tasks root") {
		t.Fatalf("discoverTaskRoots error = %v, want direct task-root symlink rejection", err)
	}
	if contents, err := os.ReadFile(externalFile); err != nil || string(contents) != "outside" {
		t.Fatalf("symlink target changed: contents=%q err=%v", contents, err)
	}
}

// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.10
// @covers AC-SYSTEM-PAGE-STORAGE-MAINTENANCE-001.11
func TestCleanupKeepsUnclassifiedCheckoutBesideEligibleOrphan(t *testing.T) {
	provider, tasksRoot, store := newProviderFixture(t, Inventory{Complete: true}, nil)
	orphan := createOwnedCandidate(t, tasksRoot, "eligible-orphan_abc", OwnershipMarker{
		TaskID: "eligible-orphan", TaskDirName: "eligible-orphan_abc", LayoutVersion: LayoutVersionSemantic,
	})
	if err := os.WriteFile(filepath.Join(orphan, "source.txt"), []byte("owned workspace"), 0o600); err != nil {
		t.Fatalf("write orphan payload: %v", err)
	}
	old := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatalf("age orphan: %v", err)
	}

	checkout := filepath.Join(tasksRoot, "unrelated-checkout")
	for _, name := range []string{".git", "apps", "node_modules"} {
		directory := filepath.Join(checkout, name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatalf("create checkout directory %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(directory, "keep.txt"), []byte("preserve"), 0o600); err != nil {
			t.Fatalf("write checkout payload %s: %v", name, err)
		}
	}
	externalGuide := filepath.Join(t.TempDir(), "AGENTS.md")
	const externalPayload = "external target remains unchanged"
	if err := os.WriteFile(externalGuide, []byte(externalPayload), 0o600); err != nil {
		t.Fatalf("write external guide: %v", err)
	}
	link := filepath.Join(checkout, "CLAUDE.md")
	if err := os.Symlink(externalGuide, link); err != nil {
		t.Fatalf("symlink checkout guide: %v", err)
	}

	result, err := provider.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if result.Candidates != 1 || result.Quarantined != 1 || len(store.entries) != 1 {
		t.Fatalf("Cleanup result = %#v entries=%d, want only the eligible orphan", result, len(store.entries))
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Fatalf("eligible orphan remains after cleanup: %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatalf("unclassified checkout symlink changed: %v", err)
	}
	for _, name := range []string{".git", "apps", "node_modules"} {
		payload := filepath.Join(checkout, name, "keep.txt")
		if contents, err := os.ReadFile(payload); err != nil || string(contents) != "preserve" {
			t.Fatalf("unclassified checkout payload %s changed: contents=%q err=%v", name, contents, err)
		}
	}
	if contents, err := os.ReadFile(externalGuide); err != nil || string(contents) != externalPayload {
		t.Fatalf("external guide changed: contents=%q err=%v", contents, err)
	}
	if !containsWarning(result.Warnings, "unclassified task directory kept: "+checkout) {
		t.Fatalf("Cleanup warnings = %#v, want omission warning for %s", result.Warnings, checkout)
	}
}
