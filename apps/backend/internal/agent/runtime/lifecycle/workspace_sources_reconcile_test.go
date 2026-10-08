package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/worktree"
)

func TestReconcileWorkspaceSources_RejectsMissingFolderTarget(t *testing.T) {
	err := reconcileWorkspaceSources(context.Background(), t.TempDir(), []WorkspaceFolderSpec{{Name: "missing", LocalPath: "/definitely/not/a/kandev-folder"}}, testWorkspaceLinkOwner())
	if err == nil {
		t.Fatal("missing durable folder target was accepted")
	}
}

// canonicalTempDir resolves symlinks in t.TempDir() so tests hand production
// code a canonical owned control root (macOS /var -> /private/var); no-op on Linux.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	return d
}

func testWorkspaceLinkOwner() worktree.OwnedDirectoryLinkOwner {
	return worktree.OwnedDirectoryLinkOwner{TaskID: "task-1", TaskDirName: "task-1"}
}

func TestProjectWritableRootsReachAgentExecution(t *testing.T) {
	root := canonicalTempDir(t)
	paths := []string{
		filepath.Join(root, "context"), filepath.Join(root, "repo-api"), filepath.Join(root, "repo-web"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	writableRoots, err := projectWritableRoots(&ProjectWorkspaceAccess{
		ContextPath: paths[0], RepositoryWorktreePaths: paths[1:],
	})
	if err != nil {
		t.Fatalf("projectWritableRoots: %v", err)
	}
	if !reflect.DeepEqual(writableRoots, paths) {
		t.Fatalf("project writable roots = %v, want context and every selected repository %v", writableRoots, paths)
	}
	execution := (&ExecutorInstance{}).ToAgentExecution(&ExecutorCreateRequest{
		Env: map[string]string{}, ProjectWritableRoots: writableRoots,
	})
	if !reflect.DeepEqual(execution.ProjectWritableRoots, paths) {
		t.Fatalf("agent execution writable roots = %v, want %v", execution.ProjectWritableRoots, paths)
	}
}

func TestProjectWritableRootsFailClosedForMissingSelectedRepository(t *testing.T) {
	contextPath := filepath.Join(canonicalTempDir(t), "context")
	if err := os.MkdirAll(contextPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if roots, err := projectWritableRoots(&ProjectWorkspaceAccess{
		ContextPath:             contextPath,
		RepositoryWorktreePaths: []string{filepath.Join(filepath.Dir(contextPath), "missing-repository")},
	}); err == nil || roots != nil {
		t.Fatalf("projectWritableRoots = %v, %v; want failure without a partial root list", roots, err)
	}
}

func TestReconcileProjectWorkspaceLinksCanonicalContextAtTaskRoot(t *testing.T) {
	base := canonicalTempDir(t)
	taskDirName := "task-project_abc"
	taskID := "task-project"
	taskRoot := filepath.Join(base, "tasks", taskDirName)
	primary := filepath.Join(taskRoot, "api")
	contextPath := filepath.Join(base, "agent-projects", uuid.NewString(), "context")
	if err := os.MkdirAll(primary, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contextPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "init", "-q", primary).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskRoot, storageworkspaces.OwnershipMarker{
		TaskID: taskID, WorkspaceID: "workspace-1", TaskDirName: taskDirName,
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatal(err)
	}

	gotRoot, err := reconcileProjectWorkspace(primary, contextPath, taskID, "workspace-1", taskDirName)
	if err != nil {
		t.Fatalf("reconcileProjectWorkspace: %v", err)
	}
	if gotRoot != taskRoot {
		t.Fatalf("task root = %q, want %q", gotRoot, taskRoot)
	}
	linkPath := filepath.Join(taskRoot, "context")
	if !worktree.IsDirectoryLink(linkPath) {
		t.Fatalf("task context entry is not a directory link: %s", linkPath)
	}
	resolved, err := filepath.EvalSymlinks(linkPath)
	if err != nil || resolved != contextPath {
		t.Fatalf("task context link resolves to %q, %v; want %q", resolved, err, contextPath)
	}
	if _, err := os.Lstat(filepath.Join(primary, "context")); !os.IsNotExist(err) {
		t.Fatalf("context must remain outside repository Git scope, lstat err=%v", err)
	}
	if err := os.WriteFile(filepath.Join(taskRoot, "context", "notes.md"), []byte("shared update\n"), 0o600); err != nil {
		t.Fatalf("write shared context through task link: %v", err)
	}
	status, err := exec.Command("git", "-C", primary, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v: %s", err, status)
	}
	if len(status) != 0 {
		t.Fatalf("shared context edit entered repository Git status: %s", status)
	}
}

func TestReconcileProjectWorkspaceRejectsWrongContextLink(t *testing.T) {
	base := canonicalTempDir(t)
	taskDirName := "task-project_abc"
	taskID := "task-project"
	taskRoot := filepath.Join(base, "tasks", taskDirName)
	primary := filepath.Join(taskRoot, "api")
	canonical := filepath.Join(base, "agent-projects", uuid.NewString(), "context")
	wrong := filepath.Join(base, "agent-projects", uuid.NewString(), "context")
	for _, path := range []string{primary, canonical, wrong} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := storageworkspaces.WriteOwnershipMarker(taskRoot, storageworkspaces.OwnershipMarker{
		TaskID: taskID, WorkspaceID: "workspace-1", TaskDirName: taskDirName,
		LayoutVersion: storageworkspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.EnsureOwnedDirectoryLink(taskRoot, "context", wrong, worktree.OwnedDirectoryLinkOwner{
		TaskID: taskID, TaskDirName: taskDirName,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := reconcileProjectWorkspace(primary, canonical, taskID, "workspace-1", taskDirName); err == nil {
		t.Fatal("wrong project context link was silently repointed")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(taskRoot, "context"))
	if err != nil || resolved != wrong {
		t.Fatalf("wrong link target changed to %q, %v; want %q", resolved, err, wrong)
	}
}

func TestWorkspaceSourceRootsUseProjectWorktreesAndContext(t *testing.T) {
	contextPath := t.TempDir()
	primaryWorktree := t.TempDir()
	otherWorktree := t.TempDir()
	sourceRepository := t.TempDir()
	roots := workspaceSourceRoots(nil, []WorkspaceRepositorySpec{{RepositoryPath: sourceRepository}}, &ProjectWorkspaceAccess{
		ContextPath:             contextPath,
		RepositoryWorktreePaths: []string{primaryWorktree, otherWorktree},
	})
	want := []string{contextPath, primaryWorktree, otherWorktree}
	if len(roots) != len(want) {
		t.Fatalf("source roots = %v, want %v", roots, want)
	}
	for i := range roots {
		if roots[i] != want[i] {
			t.Fatalf("source roots = %v, want %v", roots, want)
		}
	}
}

func TestReconcileWorkspaceRepositories_RecreatesMissingOwnedLink(t *testing.T) {
	root, source := canonicalTempDir(t), t.TempDir()
	writeMarker(t, source)
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}, nil, testWorkspaceLinkOwner()); err != nil {
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
	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: source}}, nil, testWorkspaceLinkOwner()); err != nil {
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

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: root}}, nil, testWorkspaceLinkOwner()); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "api")); !os.IsNotExist(err) {
		t.Fatalf("self-referential entry created inside the repository: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "one" {
		t.Fatalf("workspace root content = %q, %v", got, err)
	}
}

// A self-link planted by an earlier release is reported, never deleted: it
// cannot be shown to be Kandev-owned, so removing it could destroy a link the
// user or the repository keeps on purpose. Preserving it is only half the
// contract though — the user has to learn the entry is there — so the warning
// and its structured entry path are asserted too.
func TestReconcileWorkspaceRepositories_PreservesAndReportsPreExistingSelfLink(t *testing.T) {
	root := canonicalTempDir(t)
	marker := writeMarker(t, root)
	if _, err := worktree.CreateOwnedDirectoryLink(root, "api", root); err != nil {
		t.Fatalf("seed self link: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: root}}, log, testWorkspaceLinkOwner()); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "api")); err != nil {
		t.Fatalf("pre-existing self link was removed: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "one" {
		t.Fatalf("workspace root content = %q, %v", got, err)
	}

	entries := logs.FilterMessage(selfReferentialEntryWarning).All()
	if len(entries) != 1 {
		t.Fatalf("warnings = %d, want 1", len(entries))
	}
	if got, want := entries[0].ContextMap()["entry"], filepath.Join(root, "api"); got != want {
		t.Fatalf("entry = %v, want %v", got, want)
	}
}

// A repository that is the workspace root but carries no stale entry must stay
// silent: there is nothing for the user to clean up.
func TestReconcileWorkspaceRepositories_DoesNotWarnWithoutSelfLink(t *testing.T) {
	root := t.TempDir()
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: root}}, log, testWorkspaceLinkOwner()); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if got := logs.FilterMessage(selfReferentialEntryWarning).Len(); got != 0 {
		t.Fatalf("warnings = %d, want 0", got)
	}
}

// The guard is per spec, not a blanket skip: siblings still need their links.
func TestReconcileWorkspaceRepositories_LinksSiblingWhenPrimaryIsWorkspaceRoot(t *testing.T) {
	root, sibling := canonicalTempDir(t), t.TempDir()
	writeMarker(t, sibling)

	err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{
		{RepoName: "api", RepositoryPath: root},
		{RepoName: "libs", RepositoryPath: sibling},
	}, nil, testWorkspaceLinkOwner())
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
	if err := reconcileWorkspaceRepositories(repo, workspaceRepositorySpecsFromLaunch(req), nil, testWorkspaceLinkOwner()); err != nil {
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

	err := reconcileWorkspaceRepositories(file, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: file}}, nil, testWorkspaceLinkOwner())
	if err == nil {
		t.Fatal("a regular file was accepted as both workspace root and repository")
	}
}

func TestIsWorkspaceEntryName(t *testing.T) {
	accepted := []string{"api", "libs", "my-service", "repo.git"}
	rejected := []string{"", ".", "..", "/", "a/b", string(filepath.Separator)}

	// A backslash separates path elements only on Windows, and only there does a
	// drive letter prefix a path. On Unix all three are ordinary characters in a
	// legal single-component name, so the expectation flips with the platform.
	if runtime.GOOS == "windows" {
		rejected = append(rejected, `\`, "C:", `a\b`)
	} else {
		accepted = append(accepted, `\`, "C:", `a\b`)
	}

	for _, name := range accepted {
		if !isWorkspaceEntryName(name) {
			t.Errorf("isWorkspaceEntryName(%q) = false, want true", name)
		}
	}
	for _, name := range rejected {
		if isWorkspaceEntryName(name) {
			t.Errorf("isWorkspaceEntryName(%q) = true, want false", name)
		}
	}
}

// ".", ".." and a bare filesystem root all survive a filepath.Base round-trip,
// so they need rejecting here rather than deeper in the owned-link helpers:
// joining any of them resolves back to the root itself instead of to an entry
// below it. Only names invalid on every platform belong here — the
// Windows-specific ones are covered by TestIsWorkspaceEntryName.
func TestReconcileWorkspaceRepositories_RejectsTraversalRepoName(t *testing.T) {
	root, source := t.TempDir(), t.TempDir()
	for _, name := range []string{".", "..", "/", string(filepath.Separator)} {
		if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: name, RepositoryPath: source}}, nil, testWorkspaceLinkOwner()); err == nil {
			t.Fatalf("RepoName %q was accepted", name)
		}
	}
}

// A host-materialized task roots the workspace at ~/.kandev/tasks/<taskDir>,
// where the first repository is a real sibling — so the guard must not degrade
// into skipping index 0.
func TestReconcileWorkspaceRepositories_LinksPrimaryWhenRootIsTaskDirectory(t *testing.T) {
	root := filepath.Join(canonicalTempDir(t), "tasks", "task-1")
	primary := t.TempDir()
	writeMarker(t, primary)

	if err := reconcileWorkspaceRepositories(root, []WorkspaceRepositorySpec{{RepoName: "api", RepositoryPath: primary}}, nil, testWorkspaceLinkOwner()); err != nil {
		t.Fatalf("reconcileWorkspaceRepositories: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "api", "live.txt")); err != nil || string(got) != "one" {
		t.Fatalf("primary link = %q, %v; the primary must be linked into a Kandev task root", got, err)
	}
}
