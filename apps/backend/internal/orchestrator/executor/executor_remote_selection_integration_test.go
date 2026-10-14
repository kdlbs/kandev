package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	tasksvc "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var remoteSelectionSQLiteTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	_, err := sqliterepo.NewWithDB(database, database, nil)
	return err
})

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.1
// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.3
// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.6
func TestRemoteSelection_DeletedLocalCheckoutPreparesManagedWorkspace(t *testing.T) {
	fixture := newRemoteSelectionLaunchFixture(t)
	result, err := fixture.executor.LaunchPreparedSession(context.Background(), fixture.task.ToAPI(), fixture.sessionID,
		LaunchOptions{AgentProfileID: "profile", ExecutorID: "worktree", StartAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.started:
	case <-time.After(5 * time.Second):
		t.Fatal("agent did not reach startup after workspace preparation")
	}
	if result.WorktreePath == "" || !isLocalGitRepo(result.WorktreePath) {
		t.Fatalf("launch did not prepare a Git worktree: %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(result.WorktreePath, "README.md"))
	if err != nil || string(content) != "hi\n" {
		t.Fatalf("managed worktree did not contain remote content: %q, %v", content, err)
	}
	if fixture.cloner.request.WorkspaceID != "ws-remote" || fixture.cloner.request.CloneURL != "https://github.com/acme/widgets.git" ||
		fixture.cloner.request.TaskID != fixture.task.ID || fixture.cloner.request.SessionID != fixture.sessionID {
		t.Fatalf("clone request lost remote identity: %+v", fixture.cloner.request)
	}
	head, err := exec.Command("git", "-C", result.WorktreePath, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	base, err := exec.Command("git", "-C", fixture.cloner.origin, "rev-parse", "main").CombinedOutput()
	if err != nil || string(head) != string(base) {
		t.Fatalf("worktree did not use the requested main branch: head=%q base=%q error=%v", head, base, err)
	}
	stored, err := fixture.store.GetRepository(context.Background(), fixture.cloner.request.RepositoryID)
	if err != nil || stored.LocalPath != fixture.cloner.target {
		t.Fatalf("managed path not persisted: %+v, %v", stored, err)
	}
	fixture.assertLocalPreserved(t)
}

// @covers AC-WORKSPACES-REMOTE-RESOLUTION-001.6
func TestRemoteSelection_CloneFailureDoesNotStartAgent(t *testing.T) {
	for _, failure := range []string{"authentication", "cancellation"} {
		t.Run(failure, func(t *testing.T) {
			fixture := newRemoteSelectionLaunchFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			cloneError := errors.New("clone authentication failed")
			fixture.cloner.failure = cloneError
			if failure == "cancellation" {
				cloneError = context.Canceled
				fixture.cloner.failure = nil
				fixture.cloner.beforeClone = cancel
			}
			_, err := fixture.executor.LaunchPreparedSession(ctx, fixture.task.ToAPI(), fixture.sessionID,
				LaunchOptions{AgentProfileID: "profile", ExecutorID: "worktree", StartAgent: true})
			if !errors.Is(err, cloneError) {
				t.Fatalf("launch failure = %v, want clone failure", err)
			}
			if fixture.agent.launchAgentCallCount != 0 {
				t.Fatal("agent execution was launched after clone failure")
			}
			select {
			case <-fixture.started:
				t.Fatal("agent started after clone failure")
			default:
			}
			fixture.assertLocalPreserved(t)
		})
	}
}

type remoteSelectionLaunchFixture struct {
	executor  *Executor
	store     *sqliterepo.Repository
	task      *models.Task
	local     *models.Repository
	sessionID string
	cloner    *remoteSelectionLocalTransport
	agent     *mockAgentManager
	started   chan struct{}
}

func newRemoteSelectionLaunchFixture(t *testing.T) *remoteSelectionLaunchFixture {
	t.Helper()
	ctx := context.Background()
	database, _ := remoteSelectionSQLiteTemplate.Open(t)
	store := sqliterepo.NewWithInitializedDB(database, database, nil)
	if err := store.CreateWorkspace(ctx, &models.Workspace{ID: "ws-remote", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	log := logger.Default()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := tasksvc.NewService(tasksvc.Repos{Workspaces: store, RepoEntities: store}, eventBus, log, tasksvc.RepositoryDiscoveryConfig{})
	origin := initBareOriginWithMain(t)
	localPath := filepath.Join(t.TempDir(), "user-checkout")
	runGitInTest(t, "", "clone", origin, localPath)
	local, err := svc.CreateRepository(ctx, &tasksvc.CreateRepositoryRequest{
		WorkspaceID: "ws-remote", Name: "acme/widgets", SourceType: sourceTypeLocal, LocalPath: localPath,
		Provider: "github", ProviderOwner: "acme", ProviderName: "widgets", DefaultBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(localPath); err != nil {
		t.Fatal(err)
	}
	id, branch, _, err := svc.ResolveRepositoryRef(ctx, "ws-remote", tasksvc.TaskRepositoryInput{
		RemoteURL: "https://github.com/acme/widgets.git", BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	task := &models.Task{ID: "task-remote", WorkspaceID: "ws-remote", Title: "Remote work"}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	attachment := &models.TaskRepository{ID: "attachment", TaskID: task.ID, RepositoryID: id, BaseBranch: branch}
	if err := store.CreateTaskRepository(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	fixture := &remoteSelectionLaunchFixture{store: store, task: task, local: local, sessionID: "session", started: make(chan struct{}, 1)}
	fixture.cloner = &remoteSelectionLocalTransport{origin: origin, target: filepath.Join(t.TempDir(), "managed-clone")}
	fixture.agent = remoteSelectionWorktreeAgent(t, fixture)
	mock := newMockRepository()
	mock.tasks[task.ID] = task
	mock.taskRepositories[attachment.ID] = attachment
	mock.executors["worktree"] = &models.Executor{ID: "worktree", Type: models.ExecutorTypeWorktree}
	mock.sessions[fixture.sessionID] = &models.TaskSession{ID: fixture.sessionID, TaskID: task.ID, AgentProfileID: "profile", State: models.TaskSessionStateCreated}
	runtimeStore := &remoteSelectionRuntimeStore{mockRepository: mock, database: store}
	fixture.executor = newTestExecutor(t, fixture.agent, runtimeStore)
	fixture.executor.SetTaskGitCredentialPolicyResolver(fakeTaskGitCredentialPolicyResolver{policy: TaskGitCredentialPolicy{Mode: taskGitCredentialsModeExecutor}})
	fixture.executor.SetRepoCloner(fixture.cloner, runtimeStore)
	return fixture
}

func remoteSelectionWorktreeAgent(t *testing.T, fixture *remoteSelectionLaunchFixture) *mockAgentManager {
	t.Helper()
	manager, err := worktree.NewManager(worktree.Config{Enabled: true, TasksBasePath: t.TempDir(), BranchPrefix: "kandev/"}, nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	return &mockAgentManager{
		launchAgentFunc: func(ctx context.Context, request *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			checkout, err := manager.Create(ctx, worktree.CreateRequest{
				TaskID: request.TaskID, SessionID: request.SessionID, RepositoryID: request.RepositoryID,
				RepositoryPath: request.RepositoryPath, BaseBranch: request.BaseBranch,
				TaskDirName: "remote-task", RepoName: "widgets",
			})
			if err != nil {
				return nil, err
			}
			return &LaunchAgentResponse{AgentExecutionID: "execution", Status: v1.AgentStatusStarting,
				WorktreeID: checkout.ID, WorktreePath: checkout.Path, WorktreeBranch: checkout.Branch,
				WorkspacePath: checkout.Path, BaseBranch: request.BaseBranch}, nil
		},
		startAgentProcessFunc: func(context.Context, string) error { fixture.started <- struct{}{}; return nil },
	}
}

func (f *remoteSelectionLaunchFixture) assertLocalPreserved(t *testing.T) {
	t.Helper()
	local, err := f.store.GetRepository(context.Background(), f.local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if local.SourceType != sourceTypeLocal || local.LocalPath != f.local.LocalPath {
		t.Fatalf("local registration changed: %+v", local)
	}
	if _, err := os.Stat(local.LocalPath); !os.IsNotExist(err) {
		t.Fatalf("original checkout recreated: %v", err)
	}
	links, err := f.store.ListTaskRepositories(context.Background(), f.task.ID)
	if err != nil || len(links) != 1 || links[0].RepositoryID == local.ID {
		t.Fatalf("new task attachment uses deleted checkout: %+v, %v", links, err)
	}
}

type remoteSelectionRuntimeStore struct {
	*mockRepository
	database *sqliterepo.Repository
}

func (s *remoteSelectionRuntimeStore) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	return s.database.GetRepository(ctx, id)
}

func (s *remoteSelectionRuntimeStore) ListTaskRepositories(ctx context.Context, id string) ([]*models.TaskRepository, error) {
	return s.database.ListTaskRepositories(ctx, id)
}

func (s *remoteSelectionRuntimeStore) UpdateRepositoryLocalPath(ctx context.Context, id, path string) error {
	r, err := s.database.GetRepository(ctx, id)
	if err != nil {
		return err
	}
	r.LocalPath = path
	return s.database.UpdateRepository(ctx, r)
}

func (s *remoteSelectionRuntimeStore) UpdateRepositoryDefaultBranch(ctx context.Context, id, branch string) error {
	r, err := s.database.GetRepository(ctx, id)
	if err != nil {
		return err
	}
	return s.database.UpdateRepositoryDefaultBranch(ctx, id, r.DefaultBranch, branch)
}

type remoteSelectionLocalTransport struct {
	fakeRepoCloner
	origin, target string
	request        repoclone.GitCredentialRequest
	failure        error
	beforeClone    func()
}

func (c *remoteSelectionLocalTransport) EnsureWorkspaceClonedWithCredentialRequest(ctx context.Context, request repoclone.GitCredentialRequest, _, _ string) (string, error) {
	c.request = request
	if c.failure != nil {
		return "", c.failure
	}
	if c.beforeClone != nil {
		c.beforeClone()
	}
	output, err := exec.CommandContext(ctx, "git", "clone", c.origin, c.target).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("clone fixture: %w: %s", err, output)
	}
	return c.target, nil
}
