package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/worktree"
)

func TestReconcileWorkspaceSources_RejectsMissingFolderTarget(t *testing.T) {
	err := reconcileWorkspaceSources(context.Background(), t.TempDir(), []WorkspaceFolderSpec{{Name: "missing", LocalPath: "/definitely/not/a/kandev-folder"}})
	if err == nil {
		t.Fatal("missing durable folder target was accepted")
	}
}

func TestReconcileWorkspaceRepositories_RecreatesMissingOwnedLink(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	writeMarker(t, source)
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	// Read through the link instead of comparing os.Readlink output: Go
	// normalizes a junction target, so it never string-equals a t.TempDir()
	// carrying an 8.3 component such as C:\Users\JOHNDO~1.
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("read through repository link = %q, %v", got, err)
	}
	if err := os.Remove(filepath.Join(root, "api")); err != nil {
		t.Fatal(err)
	}
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}); err != nil {
		t.Fatalf("reconcile after reset: %v", err)
	}
}

// writeMarker seeds a file used to prove a directory survived reconciliation,
// or that a link resolves to it.
func writeMarker(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "live.txt")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The local executor roots the workspace at the primary repository itself, so
// linking that repository would plant a self-referential junction inside the
// user's own checkout.
func TestReconcileWorkspaceRepositories_SkipsRepositoryThatIsWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	marker := writeMarker(t, root)

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: root}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "api")); !os.IsNotExist(err) {
		t.Fatalf("self-referential entry created inside the repository: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "one" {
		t.Fatalf("workspace root content = %q, %v", got, err)
	}
}

// EnsureOwnedDirectoryLink accepts a matching existing link forever, so a
// self-link planted by an earlier release is only cleared by the guard.
func TestReconcileWorkspaceRepositories_RemovesPreExistingSelfLink(t *testing.T) {
	root := t.TempDir()
	marker := writeMarker(t, root)
	if _, err := worktree.CreateOwnedDirectoryLink(root, "api", root); err != nil {
		t.Fatalf("seed self link: %v", err)
	}

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: root}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "api")); !os.IsNotExist(err) {
		t.Fatalf("pre-existing self link survived: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "one" {
		t.Fatalf("workspace root content = %q, %v; removal must not touch the target", got, err)
	}
}

// The guard is per spec, not a blanket skip: siblings still need their links.
func TestReconcileWorkspaceRepositories_LinksSiblingWhenPrimaryIsWorkspaceRoot(t *testing.T) {
	root, sibling := t.TempDir(), t.TempDir()
	writeMarker(t, sibling)

	err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{
		{RepoName: "api", RepositoryPath: root},
		{RepoName: "libs", RepositoryPath: sibling},
	})
	if err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "api")); !os.IsNotExist(err) {
		t.Fatalf("self-referential entry created for the primary: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "libs", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("sibling link = %q, %v; siblings must still be linked", got, err)
	}
}

// Drives the exact production wiring of manager_launch.go: a single-repo local
// LaunchRequest, whose specs are synthesized by RepoSpecs() rather than built
// by hand, reconciled against the workspace path that launchResolveWorkspacePath
// derives for a non-worktree executor — the repository itself.
func TestReconcileWorkspaceRepositories_LocalLaunchRequestPlantsNoSelfLink(t *testing.T) {
	repo := t.TempDir()
	marker := writeMarker(t, repo)
	req := &LaunchRequest{
		ExecutorType:   "local",
		RepositoryID:   "repo-1",
		RepositoryPath: repo,
		RepoName:       filepath.Base(repo),
	}

	// launchResolveWorkspacePath returns req.RepositoryPath when WorkspacePath
	// is empty and the executor is not worktree-backed.
	if err := reconcileWorkspaceRepositories(repo, workspaceRepositorySpecsFromLaunch(req)); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(repo, req.RepoName)); !os.IsNotExist(err) {
		t.Fatalf("local launch planted a self-referential entry: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "one" {
		t.Fatalf("repository content = %q, %v", got, err)
	}
}

// A repository path replaced by a regular file must surface as a missing
// target. Comparing by identity alone would report the file as "already the
// workspace root" and skip the IsDir validation, letting the launch continue
// with a workspace path that is not a directory.
func TestReconcileWorkspaceRepositories_RejectsFileAsRepositoryPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := reconcileWorkspaceRepositories(file, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: file}})
	if err == nil {
		t.Fatal("a regular file was accepted as both workspace root and repository")
	}
}

// "." and ".." survive a filepath.Base round-trip, so they need rejecting here
// rather than deeper in the owned-link helpers.
func TestReconcileWorkspaceRepositories_RejectsTraversalRepoName(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	for _, name := range []string{".", ".."} {
		if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: name, RepositoryPath: source}}); err == nil {
			t.Fatalf("RepoName %q was accepted", name)
		}
	}
}

// A host-materialized task roots the workspace at ~/.kandev/tasks/<taskDir>,
// where the first repository is a real sibling — so the guard must not degrade
// into skipping index 0.
func TestReconcileWorkspaceRepositories_LinksPrimaryWhenRootIsTaskDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "tasks", "task-1")
	primary := t.TempDir()
	writeMarker(t, primary)

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: primary}}); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("primary link = %q, %v; the primary must be linked into a Kandev task root", got, err)
	}
}
