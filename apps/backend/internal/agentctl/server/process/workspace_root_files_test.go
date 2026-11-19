package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestWorkspaceRootFilesMonitorTickPublishesChanges(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout")
	if err := os.MkdirAll(filepath.Join(checkout, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(checkout, "tracked.txt"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "git-invoked")
	if runtime.GOOS != "windows" {
		binDir := t.TempDir()
		fakeGit := filepath.Join(binDir, "git")
		if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nprintf called >> \"$GIT_MARKER\"\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", binDir)
		t.Setenv("GIT_MARKER", marker)
	}

	tracker := NewWorkspaceTrackerForRepo(root, "", newTestLogger(t))
	tracker.suppressGitPublication = true
	sub := tracker.SubscribeWorkspaceStream()
	defer tracker.UnsubscribeWorkspaceStream(sub)
	t.Cleanup(tracker.Stop)

	last := &workspaceState{}
	failures := 0
	tracker.monitorTick(context.Background(), last, &failures)
	assertNoRootFileNotification(t, sub)

	path := filepath.Join(root, "notes.md")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracker.monitorTick(context.Background(), last, &failures)
	assertRootFileNotification(t, sub, "notes.md", types.FileOpCreate)

	if err := os.WriteFile(path, []byte("modified content"), 0o644); err != nil {
		t.Fatal(err)
	}
	tracker.monitorTick(context.Background(), last, &failures)
	assertRootFileNotification(t, sub, "notes.md", types.FileOpWrite)

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	tracker.monitorTick(context.Background(), last, &failures)
	assertRootFileNotification(t, sub, "notes.md", types.FileOpRemove)
	assertNoRootFileNotification(t, sub)

	if runtime.GOOS != "windows" {
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("monitor tick invoked git; marker stat error = %v", err)
		}
	}
}

func TestWorkspaceRootFilesListingExcludesCheckoutsAndUnsafeEntries(t *testing.T) {
	root := t.TempDir()
	firstRepo := filepath.Join(root, "frontend")
	secondRepo := filepath.Join(root, "backend")
	initGitRepoAt(t, firstRepo)
	initGitRepoAt(t, secondRepo)
	writeWorkspaceRootFile(t, firstRepo, "checkout-only.txt", "hidden")
	writeWorkspaceRootFile(t, secondRepo, "checkout-only.txt", "hidden")

	for name, body := range map[string]string{
		".env":                          "visible dot-file",
		"notes.md":                      "notes",
		"dir/report.json":               "{}",
		".hidden/secret.txt":            "hidden",
		"node_modules/package/index.js": "hidden",
		"other/.git/config":             "nested repository",
		storageworkspaces.OwnershipMarkerFilename: "workspace marker",
		"recovery-artifact.txt":                   "recovery",
	} {
		writeWorkspaceRootFile(t, root, name, body)
	}

	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "external.txt")
	if err := os.WriteFile(externalFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalFile, filepath.Join(root, "file-link")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}
	if err := os.Symlink(externalDir, filepath.Join(root, "directory-link")); err != nil && runtime.GOOS != "windows" {
		t.Fatal(err)
	}

	tracker := NewWorkspaceTrackerForRepo(root, "", newTestLogger(t))
	tracker.SetCheckoutRoots([]string{firstRepo, secondRepo})
	tracker.SetRecoveryArtifactExclusions([]string{filepath.Join(root, "recovery-artifact.txt")})
	listing, err := tracker.ListWorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceRootFiles: %v", err)
	}
	got := make([]string, 0, len(listing.Files))
	for _, file := range listing.Files {
		got = append(got, file.Path)
		if file.ModifiedAt.IsZero() {
			t.Errorf("%q has zero modified_at", file.Path)
		}
	}
	want := []string{".env", "dir/report.json", "notes.md"}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("root file paths = %v, want %v", got, want)
	}
}

func TestWorkspaceRootFilesListingCapsAt500(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < maxWorkspaceRootFiles+1; index++ {
		name := filepath.Join(root, fmt.Sprintf("file-%03d.txt", index))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tracker := NewWorkspaceTrackerForRepo(root, "", newTestLogger(t))
	listing, err := tracker.ListWorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceRootFiles: %v", err)
	}
	if len(listing.Files) != maxWorkspaceRootFiles || !listing.Truncated {
		t.Fatalf("listing has %d files, truncated=%v, want 500 and true", len(listing.Files), listing.Truncated)
	}
}

func TestWorkspaceRootFilesListingSkipsUnreadableDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this test user")
	}
	root := t.TempDir()
	unreadable := filepath.Join(root, "a-unreadable")
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceRootFile(t, unreadable, "hidden.txt", "hidden")
	writeWorkspaceRootFile(t, root, "sibling.txt", "visible")
	if err := os.Chmod(unreadable, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o700) })

	tracker := NewWorkspaceTrackerForRepo(root, "", newTestLogger(t))
	listing, err := tracker.ListWorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceRootFiles: %v", err)
	}
	if got := rootFilePaths(listing.Files); len(got) != 1 || got[0] != "sibling.txt" {
		t.Fatalf("root file paths = %v, want [sibling.txt]", got)
	}
}

func TestWorkspaceRootFilesListingSkipsReadDirPermissionAndKeepsSiblings(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for this test user")
	}
	root := t.TempDir()
	noRead := filepath.Join(root, "a-no-read")
	if err := os.Mkdir(noRead, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceRootFile(t, noRead, "hidden.txt", "hidden")
	writeWorkspaceRootFile(t, root, "sibling.txt", "visible")
	if err := os.Chmod(noRead, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(noRead, 0o700) })

	tracker := NewWorkspaceTrackerForRepo(root, "", newTestLogger(t))
	listing, err := tracker.ListWorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceRootFiles: %v", err)
	}
	if got := rootFilePaths(listing.Files); len(got) != 1 || got[0] != "sibling.txt" {
		t.Fatalf("root file paths = %v, want [sibling.txt]", got)
	}
}

func TestWorkspaceRootFilesSkipsVanishedNestedDirectoryAndKeepsSiblings(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceRootFile(t, root, "sibling/sibling.txt", "visible")
	if err := os.Mkdir(filepath.Join(root, "vanished"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := make([]types.WorkspaceRootFile, 0)
	truncated := false
	if err := walkWorkspaceRootFilesDirectory(context.Background(), root, "sibling", nil, nil, &files, &truncated); err != nil {
		t.Fatalf("walk sibling directory: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "vanished")); err != nil {
		t.Fatal(err)
	}
	if err := walkWorkspaceRootFilesDirectory(context.Background(), root, "vanished", nil, nil, &files, &truncated); err != nil {
		t.Fatalf("walk vanished directory: %v", err)
	}
	if got := rootFilePaths(files); len(got) != 1 || got[0] != "sibling/sibling.txt" {
		t.Fatalf("root file paths = %v, want [sibling/sibling.txt]", got)
	}
}

func TestWorkspaceRootFilesListingExcludesCheckoutUnderSymlinkedRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink behavior differs on Windows")
	}
	physicalParent := t.TempDir()
	linkedParent := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(physicalParent, linkedParent); err != nil {
		t.Skipf("create symlinked parent: %v", err)
	}
	root := filepath.Join(linkedParent, "workspace")
	checkout := filepath.Join(root, "checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	writeWorkspaceRootFile(t, checkout, "checkout-only.txt", "hidden")
	writeWorkspaceRootFile(t, root, "sibling.txt", "visible")

	tracker := NewWorkspaceTrackerForRepo(root, "", newTestLogger(t))
	tracker.SetCheckoutRoots([]string{checkout})
	listing, err := tracker.ListWorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceRootFiles: %v", err)
	}
	if got := rootFilePaths(listing.Files); len(got) != 1 || got[0] != "sibling.txt" {
		t.Fatalf("root file paths = %v, want [sibling.txt]", got)
	}
}

func TestManagerWorkspaceRootFilesOnlySupportsBareMultiRepoRoot(t *testing.T) {
	logger := newTestLogger(t)
	gitRoot := filepath.Join(t.TempDir(), "repository")
	initGitRepoAt(t, gitRoot)
	writeWorkspaceRootFile(t, gitRoot, "visible.txt", "visible")
	unsupported := &Manager{workspaceTracker: NewWorkspaceTrackerForRepo(gitRoot, "", logger)}
	listing, err := unsupported.WorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("WorkspaceRootFiles: %v", err)
	}
	if listing.Supported || len(listing.Files) != 0 || len(listing.Destinations) != 0 {
		t.Fatalf("single repository listing = %+v, want unsupported and empty", listing)
	}

	taskRoot := t.TempDir()
	writeWorkspaceRootFile(t, taskRoot, "notes.md", "notes")
	for _, path := range []string{"frontend", filepath.Join(".claude", "worktrees", "fix"), filepath.Join("frontend", "vendor")} {
		if err := os.MkdirAll(filepath.Join(taskRoot, path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := NewWorkspaceTrackerForRepo(taskRoot, "", logger)
	root.suppressGitPublication = true
	manager := &Manager{
		workspaceTracker: root,
		repoTrackers: []*WorkspaceTracker{
			NewWorkspaceTrackerForRepo(filepath.Join(taskRoot, "frontend"), "frontend", logger),
			NewWorkspaceTrackerForRepo(filepath.Join(taskRoot, ".claude", "worktrees", "fix"), ".claude/worktrees/fix", logger),
			{workDir: filepath.Join(taskRoot, "frontend", "vendor"), repositoryName: "frontend/vendor", comparisonAnchorSet: true},
		},
	}
	listing, err = manager.WorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("WorkspaceRootFiles: %v", err)
	}
	if !listing.Supported || len(listing.Files) != 1 || listing.Files[0].Path != "notes.md" {
		t.Fatalf("bare-root listing = %+v, want notes.md supported", listing)
	}
	wantDestinations := []types.WorkspaceRootDestination{
		{RepositoryName: "frontend", Path: "frontend"},
		{RepositoryName: ".claude/worktrees/fix", Path: ".claude/worktrees/fix"},
	}
	if len(listing.Destinations) != len(wantDestinations) {
		t.Fatalf("destinations = %+v, want %+v", listing.Destinations, wantDestinations)
	}
	for index, want := range wantDestinations {
		if listing.Destinations[index] != want {
			t.Errorf("destination[%d] = %+v, want %+v", index, listing.Destinations[index], want)
		}
	}
}

func writeWorkspaceRootFile(t *testing.T, root, rel, body string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir for %q: %v", rel, err)
	}
	if err := os.WriteFile(fullPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q: %v", rel, err)
	}
}

func rootFilePaths(files []types.WorkspaceRootFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func assertRootFileNotification(t *testing.T, sub types.WorkspaceStreamSubscriber, path, operation string) {
	t.Helper()
	select {
	case message := <-sub:
		if message.FileChange == nil {
			t.Fatalf("workspace message = %+v, want file change", message)
		}
		if got := message.FileChange.Path; got != path {
			t.Errorf("file path = %q, want %q", got, path)
		}
		if got := message.FileChange.Operation; got != operation {
			t.Errorf("operation = %q, want %q", got, operation)
		}
		if got := message.FileChange.RepositoryName; got != "" {
			t.Errorf("repository name = %q, want empty for a root file", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s notification for %q", operation, path)
	}
}

func assertNoRootFileNotification(t *testing.T, sub types.WorkspaceStreamSubscriber) {
	t.Helper()
	select {
	case message := <-sub:
		t.Fatalf("unexpected root file notification: %+v", message)
	default:
	}
}
