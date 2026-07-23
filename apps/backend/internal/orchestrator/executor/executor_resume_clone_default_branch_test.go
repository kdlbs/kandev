package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestResolveTaskRepoInfo_BackfillsDefaultBranchAfterClone is the regression
// guard for the production bug where MCP create_task with a bare github URL
// produced a Repository row with empty default_branch and a TaskRepository
// row with empty base_branch. Result: worktree.Manager.Create returned
// "base branch does not exist" because BaseBranch was empty by the time it
// was called.
//
// After the fix, ensureRepoCloned reads origin/HEAD from the freshly cloned
// repo via gitref.DefaultBranch and persists it onto the Repository row,
// which lets resolveTaskRepoInfo's existing fallback lift the value into
// repoInfo.BaseBranch.
func TestResolveTaskRepoInfo_BackfillsDefaultBranchAfterClone(t *testing.T) {
	originPath := initBareOriginWithMain(t)
	clonePath := filepath.Join(t.TempDir(), "clone")
	runGitInTest(t, "", "clone", originPath, clonePath)

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     "", // forces the on-demand clone path
		DefaultBranch: "", // mirrors the buggy MCP-created row
	}
	taskRepo := &models.TaskRepository{
		ID:           "tr-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		BaseBranch:   "", // mirrors task_repositories row left empty by MCP path
	}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(&fakeRepoCloner{returnPath: clonePath}, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.BaseBranch != "main" {
		t.Errorf("BaseBranch: got %q, want %q", info.BaseBranch, "main")
	}
	if info.Repository == nil || info.Repository.DefaultBranch != "main" {
		t.Errorf("Repository.DefaultBranch: got %q, want %q", info.Repository.DefaultBranch, "main")
	}
	if got := updater.getDefaultBranch("repo-1"); got != "main" {
		t.Errorf("UpdateRepositoryDefaultBranch: got %q, want %q", got, "main")
	}
}

func TestEnsureRepositoryCloned_ReturnsOnlyLocalPath(t *testing.T) {
	clonePath := filepath.Join(t.TempDir(), "clone")
	exec := newTestExecutor(t, &mockAgentManager{}, newMockRepository())
	exec.SetRepoCloner(&fakeRepoCloner{returnPath: clonePath}, &recordingRepoUpdater{})
	repository := &models.Repository{ID: "repo-1", Provider: "github", ProviderOwner: "acme", ProviderName: "thing"}

	path, err := exec.EnsureRepositoryCloned(context.Background(), repository)
	if err != nil {
		t.Fatalf("EnsureRepositoryCloned: %v", err)
	}
	if path != clonePath || repository.LocalPath != clonePath {
		t.Fatalf("path = %q, repository.LocalPath = %q; want %q", path, repository.LocalPath, clonePath)
	}
}

// TestResolveTaskRepoInfo_BackfillsDefaultBranchForAlreadyClonedRepo guards
// against the second-launch regression: a previous failed attempt populated
// repositories.local_path but left default_branch empty (because the backfill
// code didn't exist yet), so on the next launch resolveTaskRepoInfo skipped
// the clone path entirely and never looked at origin/HEAD. Same surface
// error: "base branch does not exist".
func TestResolveTaskRepoInfo_BackfillsDefaultBranchForAlreadyClonedRepo(t *testing.T) {
	originPath := initBareOriginWithMain(t)
	clonePath := filepath.Join(t.TempDir(), "clone")
	runGitInTest(t, "", "clone", originPath, clonePath)

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     clonePath, // already cloned by a prior (failed) launch
		DefaultBranch: "",        // never persisted
	}
	taskRepo := &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1"}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	// No cloner needed: LocalPath is already set so ensureRepoCloned never runs.
	exc.SetRepoCloner(nil, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.BaseBranch != "main" {
		t.Errorf("BaseBranch: got %q, want %q", info.BaseBranch, "main")
	}
	if got := updater.getDefaultBranch("repo-1"); got != "main" {
		t.Errorf("UpdateRepositoryDefaultBranch: got %q, want %q", got, "main")
	}
}

// TestResolveTaskRepoInfo_LeavesNonEmptyDefaultBranchAlone guards against the
// backfill stomping on an already-populated default_branch.
func TestResolveTaskRepoInfo_LeavesNonEmptyDefaultBranchAlone(t *testing.T) {
	originPath := initBareOriginWithMain(t)
	clonePath := filepath.Join(t.TempDir(), "clone")
	runGitInTest(t, "", "clone", originPath, clonePath)

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     "",
		DefaultBranch: "develop", // already set; backfill must not overwrite
	}
	taskRepo := &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1"}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(&fakeRepoCloner{returnPath: clonePath}, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.BaseBranch != "develop" {
		t.Errorf("BaseBranch should follow the existing default_branch: got %q, want %q", info.BaseBranch, "develop")
	}
	if got := updater.getDefaultBranch("repo-1"); got != "" {
		t.Errorf("UpdateRepositoryDefaultBranch should not be called when DefaultBranch is set, got %q", got)
	}
}

// TestResolveTaskRepoInfo_ExplicitTaskBaseBranchWinsOverBackfill pins the
// precedence: a TaskRepository.BaseBranch set by the caller (e.g. agent
// targeting a non-default branch on purpose) must NOT be overwritten by the
// backfilled default_branch. The backfill should still persist the detected
// branch so future tasks on the same repo get the fallback for free.
func TestResolveTaskRepoInfo_ExplicitTaskBaseBranchWinsOverBackfill(t *testing.T) {
	originPath := initBareOriginWithMain(t)
	clonePath := filepath.Join(t.TempDir(), "clone")
	runGitInTest(t, "", "clone", originPath, clonePath)

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     clonePath,
		DefaultBranch: "",
	}
	taskRepo := &models.TaskRepository{
		ID:           "tr-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		BaseBranch:   "feature-x", // explicit, must survive
	}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(nil, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.BaseBranch != "feature-x" {
		t.Errorf("BaseBranch should follow explicit task value, got %q want %q", info.BaseBranch, "feature-x")
	}
	if got := updater.getDefaultBranch("repo-1"); got != "main" {
		t.Errorf("backfill should still persist detected default for future tasks: got %q want %q", got, "main")
	}
}

// TestResolveTaskRepoInfo_BackfillIsBestEffortWhenLocalPathBroken guards the
// "log and continue" contract for gitref.DefaultBranch failures. If the local
// path doesn't point at a git repo (e.g. a stub directory created for a
// quick-chat task), resolveTaskRepoInfo must NOT return an error: the backfill
// silently no-ops and BaseBranch stays empty (the worktree manager owns the
// downstream "no branch" handling).
func TestResolveTaskRepoInfo_BackfillIsBestEffortWhenLocalPathBroken(t *testing.T) {
	notAGitRepo := t.TempDir() // empty dir, no .git inside

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     notAGitRepo,
		DefaultBranch: "",
	}
	taskRepo := &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1"}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(nil, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo should not fail when backfill cannot detect: %v", err)
	}
	if info.BaseBranch != "" {
		t.Errorf("BaseBranch should stay empty when detection fails, got %q", info.BaseBranch)
	}
	if got := updater.getDefaultBranch("repo-1"); got != "" {
		t.Errorf("UpdateRepositoryDefaultBranch should not be called on detection failure, got %q", got)
	}
}

// TestResolveTaskRepoInfo_ReClonesWhenLocalPathIsNotAGitRepo is the regression
// guard for the production bug where a provider-backed repository row ends up
// with a non-empty LocalPath pointing at a directory with no ".git" (e.g. a
// stale path left after a moved/deleted clone). Previously the clone guard
// only fired when LocalPath == "", so this stale path sailed straight through
// to the worktree preparer, which failed with "repository is not a git
// repository". resolveTaskRepoInfo must detect the invalid path and re-clone.
func TestResolveTaskRepoInfo_ReClonesWhenLocalPathIsNotAGitRepo(t *testing.T) {
	staleLocalPath := t.TempDir() // exists on disk, but has no .git inside

	originPath := initBareOriginWithMain(t)
	freshClonePath := filepath.Join(t.TempDir(), "fresh-clone")
	runGitInTest(t, "", "clone", originPath, freshClonePath)

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     staleLocalPath, // set, but not a valid git checkout
		DefaultBranch: "",
	}
	taskRepo := &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1"}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(&fakeRepoCloner{returnPath: freshClonePath}, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.RepositoryPath != freshClonePath {
		t.Errorf("RepositoryPath: got %q, want re-cloned path %q", info.RepositoryPath, freshClonePath)
	}
	if info.BaseBranch != "main" {
		t.Errorf("BaseBranch: got %q, want %q (detected from the fresh clone)", info.BaseBranch, "main")
	}
}

// TestResolveTaskRepoInfo_KeepsStaleLocalPathWhenNoClonerConfigured guards the
// "never blank a set path" contract: when the stored LocalPath is invalid but
// no cloner is configured to fix it, resolveTaskRepoInfo must not wipe out the
// existing (bad) path — it should be left as-is so the failure surfaces with
// full context rather than silently becoming empty.
func TestResolveTaskRepoInfo_KeepsStaleLocalPathWhenNoClonerConfigured(t *testing.T) {
	staleLocalPath := t.TempDir()

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     staleLocalPath,
		DefaultBranch: "",
	}
	taskRepo := &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1"}

	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(nil, &recordingRepoUpdater{})

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.RepositoryPath != staleLocalPath {
		t.Errorf("RepositoryPath: got %q, want unchanged stale path %q", info.RepositoryPath, staleLocalPath)
	}
}

// TestResolveTaskRepoInfo_DoesNotReCloneLocalSourceTypeRepo is the regression
// guard for the Codex review finding on the stale-path re-clone fix: a repo
// can be SourceType "local" (the user picked a checkout on their own machine)
// while still carrying ProviderOwner/ProviderName (the origin it was imported
// from). If that local checkout is temporarily missing/unmounted, the guard
// must NOT clone the remote into a managed path and overwrite the user's
// saved LocalPath — only genuinely provider-backed repos (SourceType !=
// "local") are eligible for the self-heal re-clone.
func TestResolveTaskRepoInfo_DoesNotReCloneLocalSourceTypeRepo(t *testing.T) {
	missingLocalPath := filepath.Join(t.TempDir(), "unmounted-checkout") // does not exist on disk

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		SourceType:    "local",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     missingLocalPath,
		DefaultBranch: "",
	}
	taskRepo := &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1"}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	// A cloner IS configured; if the guard incorrectly fired, this would clone
	// and overwrite LocalPath. The test asserts it does not fire.
	exc.SetRepoCloner(&fakeRepoCloner{returnPath: filepath.Join(t.TempDir(), "should-not-be-used")}, updater)

	info, err := exc.resolveTaskRepoInfo(context.Background(), taskRepo)
	if err != nil {
		t.Fatalf("resolveTaskRepoInfo: %v", err)
	}
	if info.RepositoryPath != missingLocalPath {
		t.Errorf("RepositoryPath: got %q, want unchanged local path %q (must not be overwritten by a re-clone)", info.RepositoryPath, missingLocalPath)
	}
	if got := updater.getDefaultBranch("repo-1"); got != "" {
		t.Errorf("UpdateRepositoryDefaultBranch should not be called for a local-sourced repo, got %q", got)
	}
}

// TestApplyResumeRepoConfig_SelfHealsStaleProviderPath is the regression guard
// for the Greptile review finding that the single-repo resume path
// (applyResumeRepoConfig) read repository.LocalPath directly without running
// it through the same stale-path re-clone guard as resolveTaskRepoInfo. A
// RESUME of a session whose repo has a stale/missing provider-backed local
// path must self-heal by re-cloning, just like a fresh launch does.
func TestApplyResumeRepoConfig_SelfHealsStaleProviderPath(t *testing.T) {
	staleLocalPath := t.TempDir() // exists on disk, but has no .git inside

	originPath := initBareOriginWithMain(t)
	freshClonePath := filepath.Join(t.TempDir(), "fresh-clone")
	runGitInTest(t, "", "clone", originPath, freshClonePath)

	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID:            "repo-1",
		Provider:      "github",
		ProviderOwner: "acme",
		ProviderName:  "thing",
		LocalPath:     staleLocalPath,
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	task := (&models.Task{ID: "task-1"}).ToAPI()
	session := &models.TaskSession{
		ID:           "sess-1",
		TaskID:       "task-1",
		RepositoryID: "repo-1",
		BaseBranch:   "main",
	}

	updater := &recordingRepoUpdater{}
	exc := newTestExecutor(t, &mockAgentManager{}, repo)
	exc.SetRepoCloner(&fakeRepoCloner{returnPath: freshClonePath}, updater)

	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "sess-1", ExecutorType: "worktree"}
	if _, err := exc.applyResumeRepoConfig(context.Background(), task, session, req, nil); err != nil {
		t.Fatalf("applyResumeRepoConfig: %v", err)
	}

	if repo.repositories["repo-1"].LocalPath != freshClonePath {
		t.Errorf("Repository.LocalPath: got %q, want re-cloned path %q", repo.repositories["repo-1"].LocalPath, freshClonePath)
	}
	if req.RepositoryPath != freshClonePath {
		t.Errorf("req.RepositoryPath: got %q, want re-cloned path %q", req.RepositoryPath, freshClonePath)
	}
}

// fakeRepoCloner returns a fixed local path for any clone request.
type fakeRepoCloner struct{ returnPath string }

func (f *fakeRepoCloner) EnsureClonedForProvider(
	_ context.Context, _, _, _, _, _, _, _ string,
) (string, error) {
	return f.returnPath, nil
}

func (f *fakeRepoCloner) BuildCloneURLWithHost(_, _, owner, name string) (string, error) {
	return "https://github.com/" + owner + "/" + name + ".git", nil
}

// recordingRepoUpdater captures calls so tests can assert on what was persisted.
type recordingRepoUpdater struct {
	mu            sync.Mutex
	defaultBranch map[string]string
}

func (r *recordingRepoUpdater) UpdateRepositoryLocalPath(_ context.Context, _, _ string) error {
	return nil
}

func (r *recordingRepoUpdater) UpdateRepositoryDefaultBranch(_ context.Context, repositoryID, defaultBranch string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defaultBranch == nil {
		r.defaultBranch = make(map[string]string)
	}
	r.defaultBranch[repositoryID] = defaultBranch
	return nil
}

func (r *recordingRepoUpdater) getDefaultBranch(repositoryID string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.defaultBranch[repositoryID]
}

// initBareOriginWithMain creates a bare git repo with one commit on `main`.
// Cloning from this path produces a working tree whose origin/HEAD points at
// origin/main, which is what gitref.DefaultBranch reads.
func initBareOriginWithMain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	originPath := filepath.Join(root, "origin.git")
	workPath := filepath.Join(root, "work")
	runGitInTest(t, "", "init", "--bare", "-b", "main", originPath)
	runGitInTest(t, "", "clone", originPath, workPath)
	runGitInTest(t, workPath, "config", "user.email", "test@example.com")
	runGitInTest(t, workPath, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(workPath, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitInTest(t, workPath, "add", "README.md")
	runGitInTest(t, workPath, "commit", "-m", "initial")
	runGitInTest(t, workPath, "push", "origin", "main")
	return originPath
}

func runGitInTest(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
