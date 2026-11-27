//go:build unix

package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/kandev/kandev/pkg/api/v1"
)

const permissionRecoveryOperationID = "123e4567-e89b-12d3-a456-426614174000"

func TestPermissionRecoveryResumeIntegration(t *testing.T) {
	fixture := newExecutorPermissionRecoveryFixture(t)
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, fixture.taskID, fixture.sessionID, models.TaskSessionStateCancelled)
	configureExecutorPermissionRecoveryRepository(repo, fixture)
	var launched *LaunchAgentRequest
	manager := &mockAgentManager{launchAgentFunc: func(ctx context.Context, req *LaunchAgentRequest) (*LaunchAgentResponse, error) {
		launched = req
		if req.ACPSessionID != "provider-conversation-kept" {
			t.Errorf("provider conversation = %q, want original resume token", req.ACPSessionID)
		}
		claim, err := fixture.store.GetTaskEnvironmentRecoveryClaim(ctx, fixture.environmentID)
		if err != nil || claim == nil || claim.SessionID != fixture.sessionID {
			t.Errorf("runtime did not retain SQLite recovery claim: %+v, %v", claim, err)
		}
		published, err := fixture.store.GetWorktreeByID(ctx, fixture.replacementWorktreeID)
		if err != nil || published == nil || published.Path != fixture.replacement {
			t.Errorf("runtime worktree = %+v, %v, want published replacement %q", published, err, fixture.replacement)
		}
		return &LaunchAgentResponse{AgentExecutionID: "execution-permission-retry", Status: v1.AgentStatusStarting}, nil
	}}
	executor := newTestExecutor(t, manager, repo)
	executor.SetRepoCloner(fixture.cloner, nil)
	executor.SetSelectedWorktreeRecoveryAdmission(fixture.manager.AdmitRecovery)
	session := repo.sessions[fixture.sessionID]

	if _, err := executor.ResumeSession(context.Background(), session, false); err == nil {
		t.Fatal("ordinary resume accepted dirty managed-clone relocation")
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("ordinary resume launched runtime %d times", manager.launchAgentCallCount)
	}
	if _, err := os.Lstat(fixture.newSnapshot); !os.IsNotExist(err) {
		t.Fatalf("ordinary resume created retry snapshot: %v", err)
	}

	ctx := worktree.WithDirtyCloneRelocation(context.Background())
	ctx = worktree.WithManagedCloneRelocationAuthorization(ctx, func(context.Context) error { return nil })
	_, err := executor.ResumeSessionWithOptions(ctx, session, true, ResumeOptions{})
	if err != nil {
		t.Fatalf("explicit authorized permission retry resume: %v", err)
	}
	if manager.launchAgentCallCount != 1 || launched == nil {
		t.Fatalf("runtime launches = %d, request = %+v, want one provider resume", manager.launchAgentCallCount, launched)
	}
	if launched.SessionID != fixture.sessionID || launched.TaskEnvironmentID != fixture.environmentID ||
		launched.ACPSessionID != "provider-conversation-kept" {
		t.Fatalf("runtime resume identity = session %q, environment %q, provider token %q", launched.SessionID,
			launched.TaskEnvironmentID, launched.ACPSessionID)
	}
	claim, err := fixture.store.GetTaskEnvironmentRecoveryClaim(context.Background(), fixture.environmentID)
	if err != nil || claim != nil {
		t.Fatalf("recovery claim after resume = %+v, %v, want released", claim, err)
	}
	published, err := fixture.store.GetWorktreeByID(context.Background(), fixture.replacementWorktreeID)
	if err != nil || published == nil || published.Path != fixture.replacement {
		t.Fatalf("canonical SQLite worktree = %+v, %v, want %q", published, err, fixture.replacement)
	}
	environment, err := fixture.taskRepo.GetTaskEnvironment(context.Background(), fixture.environmentID)
	if err != nil || environment == nil || len(environment.Repos) != 1 || environment.Repos[0].WorktreePath != fixture.replacement {
		t.Fatalf("canonical SQLite environment = %+v, %v, want replacement path", environment, err)
	}
	if mode := permissionRetryMode(t, filepath.Join(fixture.replacement, "group-writable.txt")); mode != 0o775 {
		t.Fatalf("published dirty file mode = %04o, want 0775", mode)
	}
	if content, err := os.ReadFile(filepath.Join(fixture.replacement, "group-writable.txt")); err != nil || string(content) != "preserve provider work\n" {
		t.Fatalf("published dirty content = %q, %v", content, err)
	}
	recovery := readExecutorPermissionRecoveryRecord(t, fixture.worktreePath)
	if recovery.State != "complete" || recovery.OperationID != permissionRecoveryOperationID || recovery.Snapshot != fixture.newSnapshot {
		t.Fatalf("completed recovery record = %+v", recovery)
	}
	if _, err := os.Stat(fixture.oldSnapshot); err != nil {
		t.Fatalf("historical failed snapshot was not retained: %v", err)
	}
	if recovery.Original == fixture.worktreePath {
		t.Fatal("completed recovery record did not retain the archived original path")
	}
	if content, err := os.ReadFile(filepath.Join(recovery.Original, "group-writable.txt")); err != nil || string(content) != "preserve provider work\n" {
		t.Fatalf("archived original content = %q, %v", content, err)
	}
}

func TestPermissionRecoveryIdentityRefusalPreventsProviderStartup(t *testing.T) {
	fixture := newExecutorPermissionRecoveryFixture(t)
	setuidOriginal := filepath.Join(fixture.worktreePath, "setuid-file")
	setuidSnapshot := filepath.Join(fixture.oldSnapshot, "setuid-file")
	if err := os.WriteFile(setuidOriginal, []byte("setuid identity evidence\n"), 0o755); err != nil {
		t.Fatalf("write source setuid file: %v", err)
	}
	if err := chmodPermissionRecoveryUnixMode(setuidOriginal, 0o4755); err != nil {
		t.Fatalf("set source setuid mode: %v", err)
	}
	if info, err := os.Stat(setuidOriginal); err != nil {
		t.Fatalf("stat source setuid file: %v", err)
	} else if info.Mode()&os.ModeSetuid == 0 {
		t.Skipf("host filesystem does not retain setuid mode: %s", info.Mode())
	}
	if err := os.WriteFile(setuidSnapshot, []byte("setuid identity evidence\n"), 0o755); err != nil {
		t.Fatalf("write historical setuid snapshot: %v", err)
	}
	otherUID := 65534
	if otherUID == os.Geteuid() {
		otherUID = 65533
	}
	if err := os.Chown(setuidSnapshot, otherUID, -1); err != nil {
		t.Skipf("host cannot seed a copied setuid owner mismatch: %v", err)
	}
	if err := chmodPermissionRecoveryUnixMode(setuidSnapshot, 0o4755); err != nil {
		t.Fatalf("restore historical setuid mode: %v", err)
	}
	if info, err := os.Stat(setuidSnapshot); err != nil || info.Mode()&os.ModeSetuid == 0 {
		t.Skip("host filesystem does not retain setuid mode after ownership changes")
	}

	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, fixture.taskID, fixture.sessionID, models.TaskSessionStateCancelled)
	configureExecutorPermissionRecoveryRepository(repo, fixture)
	manager := &mockAgentManager{}
	executor := newTestExecutor(t, manager, repo)
	executor.SetRepoCloner(fixture.cloner, nil)
	executor.SetSelectedWorktreeRecoveryAdmission(fixture.manager.AdmitRecovery)
	ctx := worktree.WithDirtyCloneRelocation(context.Background())
	ctx = worktree.WithManagedCloneRelocationAuthorization(ctx, func(context.Context) error { return nil })
	if _, err := executor.ResumeSessionWithOptions(ctx, repo.sessions[fixture.sessionID], true, ResumeOptions{}); err == nil {
		t.Fatal("explicit retry accepted a retained setuid identity mismatch")
	}
	if manager.launchAgentCallCount != 0 {
		t.Fatalf("identity refusal started the provider %d times", manager.launchAgentCallCount)
	}
	published, err := fixture.store.GetWorktreeByID(context.Background(), fixture.worktreeID)
	if err != nil || published == nil || published.Path != fixture.worktreePath {
		t.Fatalf("identity refusal changed canonical worktree = %+v, %v", published, err)
	}
	if info, err := os.Stat(setuidOriginal); err != nil || info.Mode()&os.ModeSetuid == 0 {
		t.Fatalf("original setuid checkout changed after refusal: info=%v err=%v", info, err)
	}
}

type executorPermissionRecoveryFixture struct {
	db                    *sqlx.DB
	taskRepo              *tasksqlite.Repository
	store                 *worktree.SQLiteStore
	manager               *worktree.Manager
	cloner                *repoclone.Cloner
	config                worktree.Config
	taskID                string
	sessionID             string
	workspaceID           string
	environmentID         string
	worktreeID            string
	replacementWorktreeID string
	environmentRepo       string
	taskDirName           string
	repositoryID          string
	branch                string
	branchSlug            string
	repositoryPath        string
	worktreePath          string
	sourceClone           string
	newSnapshot           string
	oldSnapshot           string
	replacement           string
}

func newExecutorPermissionRecoveryFixture(t *testing.T) *executorPermissionRecoveryFixture {
	t.Helper()
	fixture := &executorPermissionRecoveryFixture{
		taskID: "task-permission-recovery", sessionID: "session-permission-recovery",
		workspaceID: "workspace-permission-recovery", environmentID: "environment-permission-recovery",
		worktreeID: "worktree-permission-recovery", replacementWorktreeID: "worktree-permission-recovery-replacement",
		environmentRepo: "environment-repo-permission-recovery",
		taskDirName:     "task-permission-recovery-root", repositoryID: "repo-permission-recovery",
		branch: "feature/permission-recovery", branchSlug: "main",
	}
	dbConn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "permission-recovery.db"))
	if err != nil {
		t.Fatalf("open recovery SQLite database: %v", err)
	}
	fixture.db = sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = fixture.db.Close() })
	fixture.taskRepo, err = tasksqlite.NewWithDB(fixture.db, fixture.db, nil)
	if err != nil {
		t.Fatalf("initialize task repository: %v", err)
	}
	fixture.createSQLiteInventory(t)
	fixture.createManagedClones(t)
	fixture.manager, err = worktree.NewManager(fixture.config, fixture.store, logger.Default())
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	return fixture
}

func (f *executorPermissionRecoveryFixture) createSQLiteInventory(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, create := range []func() error{
		func() error {
			return f.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: f.workspaceID, Name: f.workspaceID})
		},
		func() error {
			return f.taskRepo.CreateTask(ctx, &models.Task{ID: f.taskID, WorkspaceID: f.workspaceID, Title: "Permission recovery"})
		},
	} {
		if err := create(); err != nil {
			t.Fatalf("create SQLite task identity: %v", err)
		}
	}
	fixtureRepo := &models.Repository{
		ID: f.repositoryID, WorkspaceID: f.workspaceID, Name: "widget", SourceType: "provider",
		Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widget",
		DefaultBranch: "main", CreatedAt: now, UpdatedAt: now,
	}
	fixtureRepo.LocalPath = ""
	managedRoot := filepath.Join(t.TempDir(), "managed-clones")
	f.cloner = repoclone.NewCloner(repoclone.Config{BasePath: managedRoot}, repoclone.ProtocolHTTPS, "", nil)
	_, source, _, destination, ok, err := f.cloner.ManagedCloneRelocationPaths(fixtureRepo)
	if err != nil || !ok {
		t.Fatalf("resolve managed clone paths: ok=%t err=%v", ok, err)
	}
	f.sourceClone, f.repositoryPath = source, destination
	fixtureRepo.LocalPath = destination
	if err := f.taskRepo.CreateRepository(ctx, fixtureRepo); err != nil {
		t.Fatalf("create provider repository: %v", err)
	}
	if err := f.taskRepo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-permission-recovery", TaskID: f.taskID, RepositoryID: f.repositoryID,
		BaseBranch: "main", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task repository: %v", err)
	}
	f.config = worktree.Config{TasksBasePath: filepath.Join(t.TempDir(), "tasks")}
	f.worktreePath, err = f.config.TaskWorktreePath(f.taskDirName, "widget", f.branchSlug)
	if err != nil {
		t.Fatalf("build canonical worktree path: %v", err)
	}
	taskRoot := filepath.Dir(f.worktreePath)
	if err := os.MkdirAll(taskRoot, 0o755); err != nil {
		t.Fatalf("create task root: %v", err)
	}
	if err := workspaces.WriteOwnershipMarker(taskRoot, workspaces.OwnershipMarker{
		TaskID: f.taskID, TaskDirName: f.taskDirName, LayoutVersion: workspaces.LayoutVersionSemantic,
	}); err != nil {
		t.Fatalf("write task ownership marker: %v", err)
	}
	common := filepath.Join(source, ".git")
	if err := f.taskRepo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: f.environmentID, TaskID: f.taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: taskRoot, TaskDirName: f.taskDirName,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: f.environmentRepo, TaskEnvironmentID: f.environmentID, RepositoryID: f.repositoryID,
			BranchSlug: f.branchSlug, WorktreeID: f.worktreeID, WorktreePath: f.worktreePath,
			WorktreeBranch: f.branch, WorktreeSourceClonePath: source, WorktreeSourceCommonDir: common,
			Status: "active", Position: 0, CreatedAt: now, UpdatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := f.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: f.sessionID, TaskID: f.taskID, TaskEnvironmentID: f.environmentID,
		State: models.TaskSessionStateCancelled, ExecutorID: models.ExecutorIDWorktree,
		AgentProfileID: "profile-permission-recovery", RepositoryID: f.repositoryID, BaseBranch: "main",
	}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	f.store, err = worktree.NewSQLiteStore(f.db, f.db)
	if err != nil {
		t.Fatalf("create SQLite worktree store: %v", err)
	}
}

func (f *executorPermissionRecoveryFixture) createManagedClones(t *testing.T) {
	t.Helper()
	seed := initExecutorRecoveryGitRepository(t)
	for _, clone := range []string{f.sourceClone, f.repositoryPath} {
		if err := os.MkdirAll(filepath.Dir(clone), 0o755); err != nil {
			t.Fatalf("create managed clone parent: %v", err)
		}
		runExecutorRecoveryGit(t, filepath.Dir(clone), "clone", "--no-hardlinks", seed, clone)
		runExecutorRecoveryGit(t, clone, "remote", "set-url", "origin", "https://github.com/acme/widget.git")
		runExecutorRecoveryGit(t, clone, "config", "user.email", "recovery@example.test")
		runExecutorRecoveryGit(t, clone, "config", "user.name", "Recovery Test")
	}
	runExecutorRecoveryGit(t, f.sourceClone, "checkout", "-b", f.branch)
	if err := os.WriteFile(filepath.Join(f.sourceClone, "branch.txt"), []byte("unpublished commit\n"), 0o644); err != nil {
		t.Fatalf("write managed branch content: %v", err)
	}
	runExecutorRecoveryGit(t, f.sourceClone, "add", "branch.txt")
	runExecutorRecoveryGit(t, f.sourceClone, "commit", "-m", "permission recovery commit")
	head := strings.TrimSpace(runExecutorRecoveryGit(t, f.sourceClone, "rev-parse", "HEAD"))
	runExecutorRecoveryGit(t, f.sourceClone, "checkout", "main")
	runExecutorRecoveryGit(t, f.repositoryPath, "fetch", "--no-tags", f.sourceClone, head)
	runExecutorRecoveryGit(t, f.repositoryPath, "update-ref", "refs/heads/"+f.branch, head)
	f.replacement = f.worktreePath + ".relocated-" + permissionRecoveryOperationID[:8]
	runExecutorRecoveryGit(t, f.repositoryPath, "worktree", "add", f.replacement, f.branch)
	if err := os.MkdirAll(filepath.Dir(f.worktreePath), 0o755); err != nil {
		t.Fatalf("create original worktree parent: %v", err)
	}
	runExecutorRecoveryGit(t, f.sourceClone, "worktree", "add", f.worktreePath, f.branch)
	if err := os.WriteFile(filepath.Join(f.worktreePath, "group-writable.txt"), []byte("preserve provider work\n"), 0o775); err != nil {
		t.Fatalf("write dirty worktree content: %v", err)
	}
	if err := os.Chmod(filepath.Join(f.worktreePath, "group-writable.txt"), 0o775); err != nil {
		t.Fatalf("set dirty worktree mode: %v", err)
	}
	if err := os.Symlink("group-writable.txt", filepath.Join(f.worktreePath, "dirty-link")); err != nil {
		t.Fatalf("create dirty worktree link: %v", err)
	}
	f.oldSnapshot = f.worktreePath + ".kandev-recovery-" + permissionRecoveryOperationID
	if err := copyPermissionRetryFixture(f.worktreePath, f.oldSnapshot); err != nil {
		t.Fatalf("copy historical recovery snapshot: %v", err)
	}
	if err := os.Chmod(filepath.Join(f.oldSnapshot, "group-writable.txt"), 0o755); err != nil {
		t.Fatalf("simulate historical permission loss: %v", err)
	}
	f.newSnapshot = f.worktreePath + ".kandev-recovery-" + permissionRecoveryOperationID + "-modes-v1"
	if err := f.store.CreateWorktree(context.Background(), &worktree.Worktree{
		ID: f.worktreeID, SessionID: f.sessionID, TaskID: f.taskID, TaskEnvironmentID: f.environmentID,
		RepositoryID: f.repositoryID, TaskDirName: f.taskDirName, BranchSlug: f.branchSlug,
		RepositoryPath: f.repositoryPath, Path: f.worktreePath, Branch: f.branch, BaseBranch: "main",
		SourceClonePath: f.sourceClone, SourceCommonDir: filepath.Join(f.sourceClone, ".git"), Status: worktree.StatusActive,
	}); err != nil {
		t.Fatalf("create SQLite worktree row: %v", err)
	}
	f.seedManagedCloneJournal(t, head)
}

func (f *executorPermissionRecoveryFixture) seedManagedCloneJournal(t *testing.T, head string) {
	t.Helper()
	updated := time.Date(2026, 10, 4, 15, 0, 0, 0, time.UTC)
	recovery := map[string]interface{}{
		"operation_id": permissionRecoveryOperationID, "task_id": f.taskID, "worktree_id": f.worktreeID,
		"original": f.worktreePath, "snapshot": f.oldSnapshot, "manifest": "", "state": "blocked",
		"updated_at": updated, "error": "recovery snapshot does not match original checkout",
	}
	writePermissionRetryJSON(t, f.worktreePath+".kandev-recovery.json", recovery)
	relocation := map[string]interface{}{
		"operation_id": permissionRecoveryOperationID, "task_id": f.taskID, "environment_id": f.environmentID,
		"worktree_id": f.worktreeID, "original": f.worktreePath, "original_workspace_path": f.worktreePath,
		"replacement": f.replacement, "replacement_id": "worktree-permission-recovery-replacement",
		"source_path": f.sourceClone, "source_common_dir": filepath.Join(f.sourceClone, ".git"),
		"destination_path": f.repositoryPath, "destination_common_dir": filepath.Join(f.repositoryPath, ".git"),
		"branch": f.branch, "head": head, "state": "materialized",
	}
	writePermissionRetryJSON(t, f.worktreePath+".kandev-clone-relocation.json", relocation)
}

func configureExecutorPermissionRecoveryRepository(repo *mockRepository, fixture *executorPermissionRecoveryFixture) {
	env := repo.taskEnvironments["environment-recovery"]
	delete(repo.taskEnvironments, "environment-recovery")
	delete(repo.taskEnvironmentRepos, "environment-recovery")
	env.ID = fixture.environmentID
	env.TaskID, env.TaskDirName, env.WorkspacePath = fixture.taskID, fixture.taskDirName, filepath.Dir(fixture.worktreePath)
	env.OwnershipGeneration, env.ExecutorType, env.ExecutorID = 1, string(models.ExecutorTypeWorktree), models.ExecutorIDWorktree
	row := env.Repos[0]
	row.ID, row.TaskEnvironmentID = fixture.environmentRepo, fixture.environmentID
	row.WorktreeID, row.WorktreePath, row.WorktreeBranch = fixture.worktreeID, fixture.worktreePath, fixture.branch
	row.RepositoryID, row.BranchSlug = fixture.repositoryID, fixture.branchSlug
	row.WorktreeSourceClonePath, row.WorktreeSourceCommonDir = fixture.sourceClone, filepath.Join(fixture.sourceClone, ".git")
	repo.taskEnvironments[fixture.environmentID] = env
	repo.taskEnvironmentRepos[fixture.environmentID] = env.Repos
	repository := repo.repositories["repo-recovery"]
	delete(repo.repositories, "repo-recovery")
	repository.ID, repository.WorkspaceID, repository.Name = fixture.repositoryID, fixture.workspaceID, "widget"
	repository.LocalPath, repository.SourceType = fixture.repositoryPath, "provider"
	repository.Provider, repository.ProviderHost = "github", "https://github.com"
	repository.ProviderOwner, repository.ProviderName = "acme", "widget"
	repo.repositories[fixture.repositoryID] = repository
	taskRepository := repo.taskRepositories["task-repo-recovery"]
	taskRepository.TaskID, taskRepository.RepositoryID = fixture.taskID, fixture.repositoryID
	session := repo.sessions[fixture.sessionID]
	if session != nil {
		session.ID = fixture.sessionID
		session.TaskID, session.TaskEnvironmentID, session.State = fixture.taskID, fixture.environmentID, models.TaskSessionStateCancelled
		session.ExecutorID, session.AgentProfileID = models.ExecutorIDWorktree, "profile-permission-recovery"
		session.RepositoryID, session.BaseBranch = fixture.repositoryID, "main"
		repo.sessions[fixture.sessionID] = session
	}
	repo.tasks[fixture.taskID] = &models.Task{ID: fixture.taskID, WorkspaceID: fixture.workspaceID, Title: "Permission recovery"}
	repo.executorsRunning[fixture.sessionID] = &models.ExecutorRunning{
		ID: "execution-permission-recovery-old", SessionID: fixture.sessionID, TaskID: fixture.taskID,
		AgentExecutionID: "execution-permission-recovery-old", ResumeToken: "provider-conversation-kept", Resumable: true,
	}
}

func copyPermissionRetryFixture(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.Mkdir(destination, 0o700)
		}
		if relative == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if info.IsDir() {
			if err := os.Mkdir(target, 0o700); err != nil {
				return err
			}
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, contents, 0o600); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode().Perm())
	})
}

func writePermissionRetryJSON(t *testing.T, path string, value interface{}) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal recovery fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write recovery fixture %q: %v", path, err)
	}
}

type executorPermissionRecoveryRecord struct {
	OperationID string `json:"operation_id"`
	Original    string `json:"original"`
	Snapshot    string `json:"snapshot"`
	State       string `json:"state"`
}

func readExecutorPermissionRecoveryRecord(t *testing.T, original string) executorPermissionRecoveryRecord {
	t.Helper()
	data, err := os.ReadFile(original + ".kandev-recovery.json")
	if err != nil {
		t.Fatalf("read completed permission recovery record: %v", err)
	}
	var record executorPermissionRecoveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("decode completed permission recovery record: %v", err)
	}
	return record
}

func permissionRetryMode(t *testing.T, path string) uint32 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat recovered permission fixture: %v", err)
	}
	return uint32(info.Mode().Perm())
}

func chmodPermissionRecoveryUnixMode(path string, mode uint32) error {
	fileMode := os.FileMode(mode & 0o777)
	if mode&0o4000 != 0 {
		fileMode |= os.ModeSetuid
	}
	if mode&0o2000 != 0 {
		fileMode |= os.ModeSetgid
	}
	if mode&0o1000 != 0 {
		fileMode |= os.ModeSticky
	}
	return os.Chmod(path, fileMode)
}
