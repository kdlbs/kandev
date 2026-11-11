package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

type recordingWorkspaceRescanner struct {
	roots    [][]string
	sessions []string
}

func (r *recordingWorkspaceRescanner) RescanWorkspaceForSession(_ context.Context, sessionID, _ string, sourceRoots ...[]string) error {
	r.sessions = append(r.sessions, sessionID)
	if len(sourceRoots) > 0 {
		r.roots = append(r.roots, append([]string(nil), sourceRoots[0]...))
	}
	return nil
}

func TestAdoptWorktree_Reported(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	taskRoot, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")

	beforeBranch, beforeHead := gitValue(t, linked, "rev-parse", "--abbrev-ref", "HEAD"), gitValue(t, linked, "rev-parse", "HEAD")
	rescanner := &recordingWorkspaceRescanner{}
	svc.SetWorkspaceSourceRescanner(rescanner)

	result, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{
		TaskID: task.ID, Path: linked, Source: WorktreeAdoptionSourceReported,
	})
	if err != nil {
		t.Fatalf("AdoptWorktree: %v", err)
	}
	if !result.Adopted || result.AlreadyAttached {
		t.Fatalf("unexpected adoption flags: %+v", result)
	}
	if result.WorktreePath != linked || result.TaskWorkspacePath != taskRoot || result.WorktreeBranch != "fix-b" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := gitValue(t, linked, "rev-parse", "--abbrev-ref", "HEAD"); got != beforeBranch {
		t.Fatalf("branch changed to %q", got)
	}
	if got := gitValue(t, linked, "rev-parse", "HEAD"); got != beforeHead {
		t.Fatalf("HEAD changed to %q", got)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	if err != nil {
		t.Fatalf("ListTaskEnvironmentRepos: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 environment rows, got %d", len(rows))
	}
	var adopted *models.TaskEnvironmentRepo
	for _, row := range rows {
		if row.WorktreeOrigin == models.WorktreeOriginAdopted {
			adopted = row
		}
	}
	if adopted == nil || adopted.WorktreePath != linked || adopted.WorktreeBranch != "fix-b" || adopted.WorktreeBranchOwner != "external" {
		t.Fatalf("unexpected adopted inventory row: %+v", adopted)
	}
	if len(rescanner.sessions) == 0 {
		t.Fatal("expected a live workspace rescan after adoption")
	}

	again, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil {
		t.Fatalf("second AdoptWorktree: %v", err)
	}
	if !again.AlreadyAttached {
		t.Fatal("expected the second report to reuse the existing root")
	}
	taskRows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(taskRows) != 2 {
		t.Fatalf("expected exactly 2 task repositories, got %d", len(taskRows))
	}
}

func TestAdoptWorktree_RejectsInvalidCandidates(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")
	subdirectory := filepath.Join(linked, "src")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatalf("mkdir subdirectory: %v", err)
	}

	outside := filepath.Join(t.TempDir(), "outside")
	cmd := exec.Command("git", "worktree", "add", "-b", "outside", outside, "main")
	cmd.Dir = primary
	cmd.Env = isolatedGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add outside: %v: %s", err, out)
	}

	cases := []struct {
		name           string
		path           string
		checkoutBranch string
	}{
		{name: "outside task root", path: outside},
		{name: "not a worktree", path: primary},
		{name: "missing path", path: filepath.Join(taskWorkspaceRootForTest(primary), "missing")},
		{name: "branch mismatch", path: linked, checkoutBranch: "other"},
		{name: "worktree subdirectory", path: subdirectory},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{
				TaskID: task.ID, Path: tc.path, CheckoutBranch: tc.checkoutBranch,
			})
			if err == nil {
				t.Fatal("expected adoption to be rejected")
			}
			if !errors.Is(err, ErrWorktreeNotAdoptable) {
				t.Fatalf("expected ErrWorktreeNotAdoptable, got %v", err)
			}
		})
	}
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(rows) != 1 {
		t.Fatalf("rejected reports must not write rows, got %d", len(rows))
	}
}

func TestDetachTaskRepository_RemovesRootAndSuppressesDetection(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")
	adopted, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil {
		t.Fatalf("AdoptWorktree: %v", err)
	}

	if err := svc.DetachTaskRepository(ctx, task.ID, adopted.ID); err != nil {
		t.Fatalf("DetachTaskRepository: %v", err)
	}
	taskRows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(taskRows) != 1 || taskRows[0].CheckoutBranch == "fix-b" {
		t.Fatalf("detached row still present: %+v", taskRows)
	}
	envRows, _ := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	if len(envRows) != 2 {
		t.Fatalf("expected primary plus retained detached inventory, got %d", len(envRows))
	}
	var retained *models.TaskEnvironmentRepo
	for _, row := range envRows {
		if row.BranchSlug == "fix-b" {
			retained = row
		}
	}
	if retained == nil || retained.Status != models.TaskEnvironmentRepoStatusDetached {
		t.Fatalf("expected retained detached inventory row, got %+v", retained)
	}
	store, ok := svc.taskEnvironments.(taskrepository.TaskDetachedWorktreeRepository)
	if !ok {
		t.Fatal("test repository does not expose detachment storage")
	}
	detached, err := store.IsTaskWorktreeDetached(ctx, task.ID, linked)
	if err != nil || !detached {
		t.Fatalf("expected detachment suppression, got %v / %v", detached, err)
	}
	if _, err := os.Stat(linked); err != nil {
		t.Fatalf("detach must not delete the worktree: %v", err)
	}

	// Detection of the suppressed path is a no-op.
	svc.HandleWorkspaceWorktreesObserved(ctx, observedInventory(task, filepath.Join(primary, ".git"), linked, "fix-b"))
	taskRows, _ = repo.ListTaskRepositories(ctx, task.ID)
	if len(taskRows) != 1 {
		t.Fatalf("detection re-adopted a suppressed worktree: %d rows", len(taskRows))
	}

	// An explicit report clears the suppression and attaches it again.
	reported, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil {
		t.Fatalf("reported adoption after detach: %v", err)
	}
	if reported.AlreadyAttached {
		t.Fatal("expected a fresh attachment after detach")
	}
	detached, _ = store.IsTaskWorktreeDetached(ctx, task.ID, linked)
	if detached {
		t.Fatal("explicit report must clear the detachment")
	}
}

func TestDetachTaskRepository_RejectsPrimaryRoot(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, _ := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	rows, _ := repo.ListTaskRepositories(ctx, task.ID)
	if len(rows) != 1 {
		t.Fatalf("expected one primary row, got %d", len(rows))
	}
	if err := svc.DetachTaskRepository(ctx, task.ID, rows[0].ID); !errors.Is(err, ErrPrimaryTaskRepositoryRoot) {
		t.Fatalf("expected primary-root rejection, got %v", err)
	}
	rows, _ = repo.ListTaskRepositories(ctx, task.ID)
	if len(rows) != 1 {
		t.Fatal("primary rejection must not delete the row")
	}
}

func setupAdoptionRepository(t *testing.T) (taskRoot, primary, linked string) {
	t.Helper()
	taskRoot = t.TempDir()
	primary = filepath.Join(taskRoot, "primary")
	initRealGitRepo(t, primary)
	linked = filepath.Join(taskRoot, ".claude", "worktrees", "fix-b")
	return taskRoot, primary, linked
}

func createAdoptionTask(t *testing.T, ctx context.Context, svc *Service, repo *sqliterepo.Repository, primary string) *models.Task {
	t.Helper()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-1", WorkspaceID: "ws-1", Name: "primary", DefaultBranch: "main",
		LocalPath: canonicalRepoTestPath(t, primary), SourceType: sourceTypeLocal,
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Adoption",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return result.Task
}

func createAdoptionEnvironment(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, taskID, primary string) {
	t.Helper()
	commonDir := filepath.Join(primary, ".git")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: taskID, ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: primary, TaskDirName: filepath.Base(filepath.Dir(primary)),
		Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-primary", RepositoryID: "repo-1", WorktreeID: "wt-primary",
			WorktreePath: primary, WorktreeBranch: "main", BranchSlug: "main",
			WorktreeSourceClonePath: primary, WorktreeSourceCommonDir: commonDir,
			WorktreeOrigin: models.WorktreeOriginKandev, Status: "active",
		}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: taskID, TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateRunning, AgentProfileID: "agent-1", IsPrimary: true,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func addLinkedWorktree(t *testing.T, primary, linked, branch string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(linked), 0o755); err != nil {
		t.Fatalf("mkdir linked parent: %v", err)
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, linked, "main")
	cmd.Dir = primary
	cmd.Env = isolatedGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
}

func gitValue(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = isolatedGitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return string(out)
}

func taskWorkspaceRootForTest(primary string) string {
	return filepath.Dir(primary)
}

func observedInventory(task *models.Task, commonDir, path, branch string) *events.WorktreeInventoryData {
	entries := []events.WorktreeInventoryEntryData{}
	if path != "" {
		entries = append(entries, events.WorktreeInventoryEntryData{Path: path, Branch: branch})
	}
	return &events.WorktreeInventoryData{
		TaskID: task.ID, SessionID: "session-1", TaskEnvironmentID: "env-1",
		ExecutorType: string(models.ExecutorTypeWorktree), CommonDir: commonDir,
		Entries: entries,
	}
}

func TestWorktreeObserved_DetectsThenRetiresMissingRoot(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")
	commonDir := filepath.Join(primary, ".git")

	svc.HandleWorkspaceWorktreesObserved(ctx, observedInventory(task, commonDir, linked, "fix-b"))
	taskRows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil || len(taskRows) != 2 {
		t.Fatalf("detection did not adopt the worktree: rows=%d err=%v", len(taskRows), err)
	}

	reported, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil || !reported.AlreadyAttached {
		t.Fatalf("report after detection should reuse the root: %+v / %v", reported, err)
	}

	cmd := exec.Command("git", "worktree", "remove", "--force", linked)
	cmd.Dir = primary
	cmd.Env = isolatedGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree remove: %v: %s", err, out)
	}
	svc.HandleWorkspaceWorktreesObserved(ctx, observedInventory(task, commonDir, "", ""))
	taskRows, _ = repo.ListTaskRepositories(ctx, task.ID)
	if len(taskRows) != 1 {
		t.Fatalf("removed worktree should lose its root, got %d rows", len(taskRows))
	}
	store, _ := svc.taskEnvironments.(taskrepository.TaskDetachedWorktreeRepository)
	detached, _ := store.IsTaskWorktreeDetached(ctx, task.ID, linked)
	if detached {
		t.Fatal("removal from disk must not create a user-detachment suppression row")
	}
}

type recordingNestedExclusioner struct {
	added   [][2]string
	removed [][2]string
}

func (r *recordingNestedExclusioner) AddNestedWorkspaceExclusion(_ context.Context, outerRoot, nestedPath string) (bool, error) {
	r.added = append(r.added, [2]string{outerRoot, nestedPath})
	return true, nil
}

func (r *recordingNestedExclusioner) RemoveNestedWorkspaceExclusion(_ context.Context, outerRoot, nestedPath string) error {
	r.removed = append(r.removed, [2]string{outerRoot, nestedPath})
	return nil
}

func TestAdoptWorktree_ExcludesNestedCheckoutFromOuterStatus(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, _ := setupAdoptionRepository(t)
	linked := filepath.Join(primary, ".claude", "worktrees", "fix-b")
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")
	exclusions := &recordingNestedExclusioner{}
	svc.SetNestedWorkspaceExclusioner(exclusions)

	adopted, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil {
		t.Fatalf("AdoptWorktree: %v", err)
	}
	if len(exclusions.added) != 1 || exclusions.added[0][0] != primary || exclusions.added[0][1] != linked {
		t.Fatalf("nested exclusion add = %+v", exclusions.added)
	}
	if err := svc.DetachTaskRepository(ctx, task.ID, adopted.ID); err != nil {
		t.Fatalf("DetachTaskRepository: %v", err)
	}
	if len(exclusions.removed) != 0 {
		t.Fatalf("user detach must keep the nested exclusion, removed = %+v", exclusions.removed)
	}

	reported, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil {
		t.Fatalf("re-report nested worktree: %v", err)
	}
	cmd := exec.Command("git", "worktree", "remove", "--force", linked)
	cmd.Dir = primary
	cmd.Env = isolatedGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree remove: %v: %s", err, out)
	}
	if err := svc.DetachTaskRepositoryFromDisk(ctx, task.ID, reported.ID); err != nil {
		t.Fatalf("DetachTaskRepositoryFromDisk: %v", err)
	}
	if len(exclusions.removed) != 1 || exclusions.removed[0][0] != primary || exclusions.removed[0][1] != linked {
		t.Fatalf("removed-from-disk exclusion remove = %+v", exclusions.removed)
	}
}

func TestAdoptWorktree_ReportedPathRequiresTaskScope(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS", OwnerID: "owner-a"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-1", WorkspaceID: "ws-1", Name: "primary", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Adoption",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main", CheckoutBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	foreign := authn.WithIdentity(ctx, authn.Identity{UserID: "owner-b", Role: authn.RoleMember})
	if _, err := svc.AdoptWorktree(foreign, AdoptWorktreeRequest{
		TaskID: result.Task.ID, Path: "/missing/worktree", Source: WorktreeAdoptionSourceReported,
	}); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign reported adoption error = %v, want ErrTaskNotFound", err)
	}
	owner := authn.WithIdentity(ctx, authn.Identity{UserID: "owner-a", Role: authn.RoleMember})
	if _, err := svc.AdoptWorktree(owner, AdoptWorktreeRequest{
		TaskID: result.Task.ID, Path: "/missing/worktree", Source: WorktreeAdoptionSourceReported,
	}); !errors.Is(err, ErrWorktreeNotAdoptable) {
		t.Fatalf("owner reported adoption error = %v, want ErrWorktreeNotAdoptable", err)
	}
}

func TestSourceClonePathHandlesWorktreesAndBareRepositories(t *testing.T) {
	cases := map[string]string{
		"/repos/project/.git":  "/repos/project",
		"/repos/project/.git/": "/repos/project",
		"/repos/project.git":   "/repos/project.git",
		"/repos/project":       "/repos",
	}
	for commonDir, want := range cases {
		if got := sourceClonePath(commonDir); got != want {
			t.Errorf("sourceClonePath(%q) = %q, want %q", commonDir, got, want)
		}
	}
}

func TestAdoptionRowMatchingUsesExactIdentity(t *testing.T) {
	rows := []*models.TaskEnvironmentRepo{
		{ID: "env-a", RepositoryID: "repo-1", WorktreeBranch: "fix-a", BranchSlug: "fix-a", WorkspaceRelativePath: "a", Status: models.TaskEnvironmentRepoStatusActive},
		{ID: "env-b", RepositoryID: "repo-1", WorktreeBranch: "fix-b", BranchSlug: "fix-b", WorkspaceRelativePath: "b", Status: models.TaskEnvironmentRepoStatusActive},
	}
	taskRepo := &models.TaskRepository{ID: "tr-b", RepositoryID: "repo-1", CheckoutBranch: "fix-b", WorkspaceRelativePath: "b"}
	if got := environmentRepoForTaskRepository(rows, taskRepo); got == nil || got.ID != "env-b" {
		t.Fatalf("environmentRepoForTaskRepository = %+v, want env-b", got)
	}
	if got := environmentRepoForTaskRepository([]*models.TaskEnvironmentRepo{{
		ID: "env-adopted", RepositoryID: "repo-1", WorktreeBranch: "actual",
		BranchSlug: "actual", WorktreeOrigin: models.WorktreeOriginAdopted,
		Status: models.TaskEnvironmentRepoStatusActive,
	}}, &models.TaskRepository{ID: "tr-adopted", RepositoryID: "repo-1", CheckoutBranch: "expected"}); got != nil {
		t.Fatalf("adopted root guessed a mismatched environment row: %+v", got)
	}
	ambiguous := []*models.TaskRepository{
		{ID: "tr-a", RepositoryID: "repo-1", CheckoutBranch: "fix-a", WorkspaceRelativePath: "a"},
		{ID: "tr-b", RepositoryID: "repo-1", CheckoutBranch: "fix-b", WorkspaceRelativePath: "b"},
	}
	svc := &Service{taskRepos: &stubTaskRepositoryRows{rows: ambiguous}}
	if got := svc.taskRepositoryForEnvironmentRow(context.Background(), "task", &models.TaskEnvironmentRepo{
		RepositoryID: "repo-1", WorktreeBranch: "fix-b", WorktreeOrigin: models.WorktreeOriginAdopted,
	}); got == nil || got.ID != "tr-b" {
		t.Fatalf("taskRepositoryForEnvironmentRow = %+v, want tr-b", got)
	}
	if got := svc.taskRepositoryForEnvironmentRow(context.Background(), "task", &models.TaskEnvironmentRepo{
		RepositoryID: "repo-1", WorktreeBranch: "unknown", WorktreeOrigin: models.WorktreeOriginAdopted,
	}); got != nil {
		t.Fatalf("ambiguous adopted match guessed %+v", got)
	}
}

type stubTaskRepositoryRows struct {
	taskrepository.TaskRepoRepository
	rows []*models.TaskRepository
}

func (s *stubTaskRepositoryRows) ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error) {
	return s.rows, nil
}

func TestDetachKandevCreatedRootRetainsCleanupInventory(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, _ := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	secondaryPath := filepath.Join(filepath.Dir(primary), "primary-feature-b")
	secondaryTaskRepo := &models.TaskRepository{
		ID: "tr-secondary", TaskID: task.ID, RepositoryID: "repo-1",
		BaseBranch: "main", CheckoutBranch: "feature/b", Position: 1,
	}
	if err := repo.CreateTaskRepository(ctx, secondaryTaskRepo); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	secondaryEnvRepo := &models.TaskEnvironmentRepo{
		ID: "env-repo-secondary", TaskEnvironmentID: "env-1", RepositoryID: "repo-1",
		WorktreeID: "wt-secondary", WorktreePath: secondaryPath, WorktreeBranch: "feature/b",
		BranchSlug: "feature-b", WorktreeBranchOwner: worktree.BranchOwnerManaged,
		WorktreeOrigin: models.WorktreeOriginKandev, Status: models.TaskEnvironmentRepoStatusActive,
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, secondaryEnvRepo); err != nil {
		t.Fatalf("CreateTaskEnvironmentRepo: %v", err)
	}
	if err := svc.DetachTaskRepository(ctx, task.ID, secondaryTaskRepo.ID); err != nil {
		t.Fatalf("DetachTaskRepository: %v", err)
	}
	rows, err := repo.ListTaskEnvironmentRepos(ctx, "env-1")
	if err != nil {
		t.Fatal(err)
	}
	var retained *models.TaskEnvironmentRepo
	for _, row := range rows {
		if row.ID == secondaryEnvRepo.ID {
			retained = row
		}
	}
	if retained == nil || retained.Status != models.TaskEnvironmentRepoStatusDetached {
		t.Fatalf("Kandev-created cleanup inventory = %+v", retained)
	}
	if retained.WorktreeBranchOwner != worktree.BranchOwnerManaged {
		t.Fatalf("branch owner lost on detach: %+v", retained)
	}
}

func TestWorktreeObservedRetriesRescanForAlreadyAttachedRoot(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")
	commonDir := filepath.Join(primary, ".git")
	svc.processWorkspaceWorktreesObserved(ctx, observedInventory(task, commonDir, linked, "fix-b"))

	rescanner := &recordingWorkspaceRescanner{}
	svc.SetWorkspaceSourceRescanner(rescanner)
	svc.processWorkspaceWorktreesObserved(ctx, observedInventory(task, commonDir, linked, "fix-b"))
	if len(rescanner.sessions) == 0 {
		t.Fatal("already-attached adopted root did not retry its workspace rescan")
	}
}

func TestAdoptWorktree_ReportedBaseUpdatesExistingAttachment(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	gitWorktreeAddBranch(t, primary, linked, "fix-b", "main")
	if _, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked}); err != nil {
		t.Fatalf("initial AdoptWorktree: %v", err)
	}
	reported, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked, BaseBranch: "release"})
	if err != nil {
		t.Fatalf("reported AdoptWorktree: %v", err)
	}
	if !reported.AlreadyAttached || reported.BaseBranch != "release" {
		t.Fatalf("reported adoption = %+v", reported)
	}
}

func gitWorktreeAddBranch(t *testing.T, primary, path, branch, base string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir linked parent: %v", err)
	}
	cmd := exec.Command("git", "worktree", "add", "-b", branch, path, base)
	cmd.Dir = primary
	cmd.Env = isolatedGitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}
}

func TestAdoptWorktree_RepairsMissingAttachmentRow(t *testing.T) {
	isolateGitEnvForTest(t)
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_, primary, linked := setupAdoptionRepository(t)
	task := createAdoptionTask(t, ctx, svc, repo, primary)
	createAdoptionEnvironment(t, ctx, repo, task.ID, primary)
	addLinkedWorktree(t, primary, linked, "fix-b")
	adopted, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{TaskID: task.ID, Path: linked})
	if err != nil {
		t.Fatalf("initial AdoptWorktree: %v", err)
	}
	if err := repo.DeleteTaskRepository(ctx, adopted.ID); err != nil {
		t.Fatalf("delete attachment row: %v", err)
	}

	repaired, err := svc.AdoptWorktree(ctx, AdoptWorktreeRequest{
		TaskID: task.ID, Path: linked, BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("repair AdoptWorktree: %v", err)
	}
	if repaired.TaskRepository == nil || repaired.ID == "" {
		t.Fatalf("repair result = %+v", repaired)
	}
	rows, err := repo.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		t.Fatalf("list task repositories: %v", err)
	}
	var found bool
	for _, row := range rows {
		if row.CheckoutBranch == "fix-b" && row.BaseBranch == "main" {
			found = true
		}
	}
	if !found {
		t.Fatalf("repair did not recreate the attachment row: %+v", rows)
	}
}

func TestTaskWorkspaceRootLegacyFlatKeepsPrimaryContainment(t *testing.T) {
	isolateGitEnvForTest(t)
	_, primary, _ := setupAdoptionRepository(t)
	svc := &Service{}
	root, err := svc.taskWorkspaceRoot(
		&models.TaskEnvironment{ExecutorType: string(models.ExecutorTypeWorktree), WorkspacePath: primary},
		[]*models.TaskEnvironmentRepo{{
			RepositoryID: "repo-1", WorktreePath: primary, Status: models.TaskEnvironmentRepoStatusActive,
		}},
	)
	if err != nil {
		t.Fatalf("taskWorkspaceRoot: %v", err)
	}
	if !sameResolvedPath(root, primary) {
		t.Fatalf("legacy workspace root = %q, want primary checkout %q", root, primary)
	}
}
