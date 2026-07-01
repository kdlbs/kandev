package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// MockEventBus implements bus.EventBus for testing
type MockEventBus struct {
	mu              sync.Mutex
	publishedEvents []*bus.Event
	closed          bool
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		publishedEvents: make([]*bus.Event, 0),
	}
}

func (m *MockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = append(m.publishedEvents, event)
	return nil
}

func (m *MockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *MockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *MockEventBus) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *MockEventBus) IsConnected() bool {
	return !m.closed
}

func (m *MockEventBus) GetPublishedEvents() []*bus.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.publishedEvents
}

func (m *MockEventBus) ClearEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishedEvents = make([]*bus.Event, 0)
}

func createTestService(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	tmpDir := t.TempDir()
	dbConn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("failed to create test repository: %v", err)
	}
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("failed to init worktree store: %v", err)
	}
	// Apply office migrations on top of the task schema. The office
	// package adds CHECK constraints (notably tasks.priority) and
	// enables foreign_keys=ON. Running both migrations here mirrors
	// production startup so service-layer tests catch cross-package
	// constraint regressions automatically.
	if _, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("failed to apply office migrations: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlxDB.Close(); err != nil {
			t.Errorf("failed to close sqlite db: %v", err)
		}
		if err := cleanup(); err != nil {
			t.Errorf("failed to close repo: %v", err)
		}
	})
	eventBus := NewMockEventBus()
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewService(Repos{
		Workspaces:       repo,
		Tasks:            repo,
		TaskRepos:        repo,
		Workflows:        repo,
		Messages:         repo,
		Turns:            repo,
		Sessions:         repo,
		GitSnapshots:     repo,
		RepoEntities:     repo,
		Executors:        repo,
		Environments:     repo,
		TaskEnvironments: repo,
		Reviews:          repo,
	}, eventBus, log, RepositoryDiscoveryConfig{})
	return svc, eventBus, repo
}

// Task tests

func TestService_CreateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create workflow first
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	repository := &models.Repository{ID: "repo-123", WorkspaceID: "ws-1", Name: "Test Repo"}
	_ = repo.CreateRepository(ctx, repository)

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-123",
		WorkflowStepID: "step-123",
		Title:          "Test Task",
		Description:    "A test task",
		Priority:       "high",
		Repositories: []TaskRepositoryInput{
			{
				RepositoryID: "repo-123",
				BaseBranch:   "main",
			},
		},
	}

	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if task.ID == "" {
		t.Error("expected task ID to be set")
	}
	if task.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", task.Title)
	}
	if task.State != v1.TaskStateCreated {
		t.Errorf("expected state CREATED, got %s", task.State)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.created" {
		t.Errorf("expected event type 'task.created', got %s", events[0].Type)
	}
	// workspace_id MUST be in the event payload so the office WS handler
	// can workspace-scope the dashboard refetch.
	data, ok := events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data is not map[string]interface{}, got %T", events[0].Data)
	}
	if got := data["workspace_id"]; got != "ws-1" {
		t.Errorf("expected workspace_id 'ws-1' in event payload, got %v", got)
	}
}

// TestService_CreateTask_DefaultsPriorityWhenEmpty pins the regression
// from the office priority migration. The migration added a CHECK
// constraint that the priority column must be one of {critical, high,
// medium, low}; callers that don't set Priority on the request (e.g.
// the onboarding adapter creating the CEO's first task) must still
// produce a row that satisfies the constraint. buildTask defaults the
// priority to "medium" exactly for this case.
func TestService_CreateTask_DefaultsPriorityWhenEmpty(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "Onboarding-style task with no priority",
		// Priority intentionally omitted — caller didn't set it.
	}
	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask with empty priority should succeed, got %v", err)
	}
	if task.Priority != "medium" {
		t.Errorf("expected default priority 'medium', got %q", task.Priority)
	}
}

func TestService_CreateTask_RejectsDuplicateRepositories(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-dup", WorkspaceID: "ws-1", Name: "Dup"})

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "dup repos",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-dup", BaseBranch: "main"},
			{RepositoryID: "repo-dup", BaseBranch: "main"},
		},
	}
	_, err := svc.CreateTask(ctx, req)
	if err == nil {
		t.Fatal("expected duplicate-repository error, got nil")
	}
	// The error must name the repository (its display name) rather than leak
	// the raw repository UUID, which the create-task dialog shows verbatim.
	if !strings.Contains(err.Error(), "Dup") {
		t.Errorf("expected error to name the repo (%q), got: %v", "Dup", err)
	}
	if strings.Contains(err.Error(), "repo-dup") {
		t.Errorf("expected error not to leak repository UUID, got: %v", err)
	}
}

// Regression: two PR URLs of the same GitHub repo (e.g. /pull/1116 and
// /pull/1117) parse to the same owner/repo and collapse to one repositoryID,
// tripping the duplicate guard. The error must name owner/repo, not the UUID.
func TestService_CreateTask_RejectsDuplicateGitHubURLs(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "dup PR urls",
		Repositories: []TaskRepositoryInput{
			{GitHubURL: "https://github.com/kdlbs/kandev/pull/1116"},
			{GitHubURL: "https://github.com/kdlbs/kandev/pull/1117"},
		},
	}
	_, err := svc.CreateTask(ctx, req)
	if err == nil {
		t.Fatal("expected duplicate-repository error, got nil")
	}
	if !strings.Contains(err.Error(), "kdlbs/kandev") {
		t.Errorf("expected error to name owner/repo (kdlbs/kandev), got: %v", err)
	}
}

func TestService_CreateTask_AcceptsMultipleDistinctRepositories(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-front", WorkspaceID: "ws-1", Name: "front"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-back", WorkspaceID: "ws-1", Name: "back"})

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Title:          "multi",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-front", BaseBranch: "main"},
			{RepositoryID: "repo-back", BaseBranch: "main"},
		},
	}
	task, err := svc.CreateTask(ctx, req)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(task.Repositories) != 2 {
		t.Errorf("expected 2 task repositories, got %d", len(task.Repositories))
	}
}

func TestService_CreateTask_RejectsRepositoryFromOtherWorkspace(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-a", Name: "A"})
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-b", Name: "B"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-a", WorkspaceID: "ws-a", Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{ID: "repo-b", WorkspaceID: "ws-b", Name: "B repo"})

	req := &CreateTaskRequest{
		WorkspaceID:    "ws-a",
		WorkflowID:     "wf-a",
		WorkflowStepID: "step-1",
		Title:          "cross-ws",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: "repo-b", BaseBranch: "main"},
		},
	}
	_, err := svc.CreateTask(ctx, req)
	if err == nil {
		t.Fatal("expected cross-workspace repository error, got nil")
	}
	if !strings.Contains(err.Error(), "does not belong to workspace") {
		t.Errorf("expected workspace-ownership error, got: %v", err)
	}
}

func TestService_CreateTask_RewritesTaskWorktreeRepositoryIDToProviderRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const (
		workspaceID    = "ws-1"
		workflowID     = "wf-1"
		badRepoID      = "repo-task-worktree"
		providerRepoID = "repo-provider"
		prHeadBranch   = "feature/adding-a-download-ot-5sl"
	)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            badRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev task worktree",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            providerRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-1",
		Title:          "PR review",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: badRepoID, BaseBranch: prHeadBranch},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("expected 1 task repository, got %d", len(task.Repositories))
	}
	if task.Repositories[0].RepositoryID != providerRepoID {
		t.Fatalf("expected provider repository %q, got %q", providerRepoID, task.Repositories[0].RepositoryID)
	}
	if task.Repositories[0].BaseBranch != prHeadBranch {
		t.Fatalf("expected base branch %q, got %q", prHeadBranch, task.Repositories[0].BaseBranch)
	}
}

func TestService_CreateTask_RewritesTaskWorktreeRepositoryIDToSafeLocalRepository(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const (
		workspaceID    = "ws-1"
		workflowID     = "wf-1"
		badRepoID      = "repo-task-worktree"
		localRepoID    = "repo-local-source"
		providerRepoID = "repo-provider"
		prHeadBranch   = "feature/adding-a-download-ot-5sl"
	)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            badRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev task worktree",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            localRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/workspaces/kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            providerRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev provider",
		SourceType:    sourceTypeProvider,
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-1",
		Title:          "PR review",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: badRepoID, BaseBranch: prHeadBranch},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("expected 1 task repository, got %d", len(task.Repositories))
	}
	if task.Repositories[0].RepositoryID != localRepoID {
		t.Fatalf("expected safe local repository %q, got %q", localRepoID, task.Repositories[0].RepositoryID)
	}
	repos, listErr := repo.ListRepositories(ctx, workspaceID)
	if listErr != nil {
		t.Fatalf("ListRepositories: %v", listErr)
	}
	if len(repos) != 3 {
		t.Fatalf("expected no additional repository to be created, got %d repositories", len(repos))
	}
}

func TestService_CreateTask_CreatesProviderRepositoryForTaskWorktreeRepositoryID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const (
		workspaceID = "ws-1"
		workflowID  = "wf-1"
		badRepoID   = "repo-task-worktree"
	)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            badRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev task worktree",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-1",
		Title:          "PR review",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: badRepoID, BaseBranch: "feature/pr-head"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("expected 1 task repository, got %d", len(task.Repositories))
	}
	createdRepoID := task.Repositories[0].RepositoryID
	if createdRepoID == badRepoID {
		t.Fatal("expected task to use a provider repository, got the task worktree repository")
	}
	createdRepo, err := repo.GetRepository(ctx, createdRepoID)
	if err != nil {
		t.Fatalf("GetRepository(%q): %v", createdRepoID, err)
	}
	if createdRepo.SourceType != sourceTypeProvider {
		t.Fatalf("expected provider source type, got %q", createdRepo.SourceType)
	}
	if createdRepo.LocalPath != "" {
		t.Fatalf("expected provider repository without local path, got %q", createdRepo.LocalPath)
	}
	if createdRepo.Provider != "github" || createdRepo.ProviderOwner != "kdlbs" || createdRepo.ProviderName != "kandev" {
		t.Fatalf("unexpected provider identity: %+v", createdRepo)
	}
}

func TestService_CreateTask_ErrorsForTaskWorktreeRepositoryWithoutProviderIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const (
		workspaceID = "ws-1"
		workflowID  = "wf-1"
		badRepoID   = "repo-task-worktree"
	)

	svc.discoveryConfig.TaskWorktreeRoots = []string{"/data/tasks"}

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:          badRepoID,
		WorkspaceID: workspaceID,
		Name:        "task worktree without provider identity",
		SourceType:  sourceTypeLocal,
		LocalPath:   "/data/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
	})

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-1",
		Title:          "PR review",
		Repositories: []TaskRepositoryInput{
			{RepositoryID: badRepoID, BaseBranch: "feature/pr-head"},
		},
	})
	if err == nil {
		t.Fatal("expected missing provider identity error")
	}
	if !strings.Contains(err.Error(), "points at a Kandev task worktree") {
		t.Fatalf("expected task worktree provider identity error, got %v", err)
	}
	repos, listErr := repo.ListRepositories(ctx, workspaceID)
	if listErr != nil {
		t.Fatalf("ListRepositories: %v", listErr)
	}
	if len(repos) != 1 {
		t.Fatalf("expected no provider repository to be created, got %d repositories", len(repos))
	}
}

func TestService_CreateTask_GitHubURLIgnoresTaskWorktreeProviderMatch(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const (
		workspaceID = "ws-1"
		workflowID  = "wf-1"
		badRepoID   = "repo-task-worktree"
	)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "WF"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            badRepoID,
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev task worktree",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:    workspaceID,
		WorkflowID:     workflowID,
		WorkflowStepID: "step-1",
		Title:          "PR review",
		Repositories: []TaskRepositoryInput{
			{GitHubURL: "https://github.com/kdlbs/kandev/pull/1567"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if len(task.Repositories) != 1 {
		t.Fatalf("expected 1 task repository, got %d", len(task.Repositories))
	}
	createdRepoID := task.Repositories[0].RepositoryID
	if createdRepoID == badRepoID {
		t.Fatal("expected GitHub URL resolution to ignore the task worktree repository")
	}
	createdRepo, err := repo.GetRepository(ctx, createdRepoID)
	if err != nil {
		t.Fatalf("GetRepository(%q): %v", createdRepoID, err)
	}
	if createdRepo.SourceType != sourceTypeProvider {
		t.Fatalf("expected provider source type, got %q", createdRepo.SourceType)
	}
	if createdRepo.LocalPath != "" {
		t.Fatalf("expected provider repository without local path, got %q", createdRepo.LocalPath)
	}
}

func TestService_FindOrCreateRepository_ReturnsCreatedForTaskWorktreeReplacement(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const workspaceID = "ws-1"

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            "repo-task-worktree",
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev task worktree",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})

	resolved, created, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   workspaceID,
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("FindOrCreateRepository: %v", err)
	}
	if !created {
		t.Fatal("expected created=true for provider replacement row")
	}
	if resolved.SourceType != sourceTypeProvider {
		t.Fatalf("expected provider source type, got %q", resolved.SourceType)
	}
	if resolved.LocalPath != "" {
		t.Fatalf("expected provider repository without local path, got %q", resolved.LocalPath)
	}
}

func TestService_FindOrCreateRepository_ErrorsWhenTaskWorktreeReplacementDisappears(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	const workspaceID = "ws-1"

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"})
	_ = repo.CreateRepository(ctx, &models.Repository{
		ID:            "repo-task-worktree",
		WorkspaceID:   workspaceID,
		Name:          "kdlbs/kandev task worktree",
		SourceType:    sourceTypeLocal,
		LocalPath:     "/root/.kandev/tasks/pr-1541-fix-skip-cle_3bm/kdlbs-kandev",
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})
	svc.repoEntities = missingReplacementLookupRepository{
		RepositoryEntityRepository: repo,
		preserveID:                 "repo-task-worktree",
	}

	_, _, err := svc.FindOrCreateRepository(ctx, &FindOrCreateRepositoryRequest{
		WorkspaceID:   workspaceID,
		Provider:      "github",
		ProviderOwner: "kdlbs",
		ProviderName:  "kandev",
		DefaultBranch: "main",
	})
	if err == nil {
		t.Fatal("expected missing replacement repository error")
	}
	if !strings.Contains(err.Error(), "no longer exists") {
		t.Fatalf("expected missing replacement error, got %v", err)
	}
}

type missingReplacementLookupRepository struct {
	repository.RepositoryEntityRepository
	preserveID string
}

func (r missingReplacementLookupRepository) GetRepository(ctx context.Context, id string) (*models.Repository, error) {
	if id != r.preserveID {
		return nil, nil
	}
	return r.RepositoryEntityRepository.GetRepository(ctx, id)
}

func TestService_CreateRepository_DefaultWorktreeBranchPrefix(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID: "ws-1",
		Name:        "Test Repo",
	})
	if err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}
	if created.WorktreeBranchPrefix != worktree.DefaultBranchPrefix {
		t.Fatalf("expected default prefix %q, got %q", worktree.DefaultBranchPrefix, created.WorktreeBranchPrefix)
	}
	if !created.PullBeforeWorktree {
		t.Fatalf("expected pull_before_worktree to default to true")
	}

	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if stored.WorktreeBranchPrefix != worktree.DefaultBranchPrefix {
		t.Fatalf("expected stored prefix %q, got %q", worktree.DefaultBranchPrefix, stored.WorktreeBranchPrefix)
	}
	if !stored.PullBeforeWorktree {
		t.Fatalf("expected stored pull_before_worktree to default to true")
	}
}

func TestService_CreateRepository_PullBeforeWorktreeFalse(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})

	pullFalse := false
	created, err := svc.CreateRepository(ctx, &CreateRepositoryRequest{
		WorkspaceID:        "ws-1",
		Name:               "Test Repo",
		PullBeforeWorktree: &pullFalse,
	})
	if err != nil {
		t.Fatalf("CreateRepository failed: %v", err)
	}
	if created.PullBeforeWorktree {
		t.Fatalf("expected pull_before_worktree to be false when explicitly set")
	}

	stored, err := repo.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository failed: %v", err)
	}
	if stored.PullBeforeWorktree {
		t.Fatalf("expected stored pull_before_worktree to be false")
	}
}

func TestService_GetTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	// Create required entities
	setupTestTask(t, repo)

	retrieved, err := svc.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if retrieved.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", retrieved.Title)
	}
}

func TestService_GetTaskNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	_, err := svc.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestService_UpdateTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Original", Priority: "medium"})
	eventBus.ClearEvents()

	newTitle := "Updated Title"
	req := &UpdateTaskRequest{Title: &newTitle}

	updated, err := svc.UpdateTask(ctx, "task-123", req)
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}
	if updated.Title != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got %s", updated.Title)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.updated" {
		t.Errorf("expected event type 'task.updated', got %s", events[0].Type)
	}
}

func TestService_DeleteTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	eventBus.ClearEvents()

	err := svc.DeleteTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("failed to delete task: %v", err)
	}

	// Verify task is deleted
	_, err = svc.GetTask(ctx, "task-123")
	if err == nil {
		t.Error("expected task to be deleted")
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestService_DeleteTaskWithReason_PublishesReason(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	eventBus.ClearEvents()

	if err := svc.DeleteTaskWithReason(ctx, "task-123", "pr_approved_by_user"); err != nil {
		t.Fatalf("DeleteTaskWithReason: %v", err)
	}

	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.deleted" {
		t.Fatalf("expected event type 'task.deleted', got %s", events[0].Type)
	}
	data, ok := events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data is not map[string]interface{}, got %T", events[0].Data)
	}
	if got := data["reason"]; got != "pr_approved_by_user" {
		t.Errorf("expected reason 'pr_approved_by_user' in payload, got %v", got)
	}
}

func TestService_DeleteTask_OmitsReasonWhenUnset(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	eventBus.ClearEvents()

	if err := svc.DeleteTask(ctx, "task-123"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	data, ok := events[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data is not map[string]interface{}, got %T", events[0].Data)
	}
	if _, present := data["reason"]; present {
		t.Errorf("expected no reason key when deleting without a reason, got %v", data["reason"])
	}
}

func TestService_DeleteTaskStopsExecutorRunningForTerminalSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	stopper := newRecordingTaskExecutionStopper()
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-completed",
		TaskID: "task-123",
		State:  models.TaskSessionStateCompleted,
	})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session-completed",
		SessionID:        "session-completed",
		TaskID:           "task-123",
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
		AgentExecutionID: "exec-terminal",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	if err := svc.DeleteTask(ctx, "task-123"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	call := stopper.waitForStopExecution(t)
	if call.executionID != "exec-terminal" {
		t.Fatalf("StopExecution executionID = %q, want exec-terminal", call.executionID)
	}
	if call.reason != "task deleted" {
		t.Fatalf("StopExecution reason = %q, want task deleted", call.reason)
	}
	if !call.force {
		t.Fatal("StopExecution force = false, want true")
	}
	waitForCleanupDone(t, svc)
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-completed"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("executor row should be removed after successful stop, got %v", err)
	}
}

func TestService_DeleteTaskPreservesExecutorRunningWhenStopFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	stopper := newRecordingTaskExecutionStopper()
	stopper.stopExecutionErr = errors.New("runtime still shutting down")
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	quickChatDir := t.TempDir()
	svc.SetQuickChatDir(quickChatDir)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-completed",
		TaskID: "task-123",
		State:  models.TaskSessionStateCompleted,
	})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session-completed",
		SessionID:        "session-completed",
		TaskID:           "task-123",
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
		AgentExecutionID: "exec-terminal",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}
	sessionDir := filepath.Join(quickChatDir, "session-completed")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	if err := svc.DeleteTask(ctx, "task-123"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	_ = stopper.waitForStopExecution(t)
	waitForCleanupDone(t, svc)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-completed"); err != nil {
		t.Fatalf("executor row should remain retryable after stop failure: %v", err)
	}
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("quick-chat directory should remain when stop fails: %v", err)
	}
}

func TestService_DeleteTaskCleansSuccessfulSessionResourcesOnPartialStopFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	stopper := newRecordingTaskExecutionStopper()
	stopper.stopExecutionErrByID = map[string]error{
		"exec-failed": errors.New("runtime still shutting down"),
	}
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	quickChatDir := t.TempDir()
	svc.SetQuickChatDir(quickChatDir)
	cleanup := &recordingWorktreeCleanup{
		worktrees: []*worktree.Worktree{
			{ID: "wt-failed", TaskID: "task-123", SessionID: "session-failed"},
			{ID: "wt-ok", TaskID: "task-123", SessionID: "session-ok"},
		},
	}
	svc.SetWorktreeCleanup(cleanup)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	for _, sessionID := range []string{"session-failed", "session-ok"} {
		_ = repo.CreateTaskSession(ctx, &models.TaskSession{
			ID:     sessionID,
			TaskID: "task-123",
			State:  models.TaskSessionStateCompleted,
		})
		sessionDir := filepath.Join(quickChatDir, sessionID)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatalf("mkdir session dir: %v", err)
		}
	}
	for _, row := range []*models.ExecutorRunning{
		{
			ID:               "session-failed",
			SessionID:        "session-failed",
			TaskID:           "task-123",
			ExecutorID:       "executor-1",
			Runtime:          agentruntime.RuntimeStandalone,
			Status:           models.ExecutorRunningStatusStarting,
			AgentExecutionID: "exec-failed",
		},
		{
			ID:               "session-ok",
			SessionID:        "session-ok",
			TaskID:           "task-123",
			ExecutorID:       "executor-1",
			Runtime:          agentruntime.RuntimeStandalone,
			Status:           models.ExecutorRunningStatusStarting,
			AgentExecutionID: "exec-ok",
		},
	} {
		if err := repo.UpsertExecutorRunning(ctx, row); err != nil {
			t.Fatalf("seed executor running: %v", err)
		}
	}

	if err := svc.DeleteTask(ctx, "task-123"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	calls := map[string]bool{}
	for i := 0; i < 2; i++ {
		calls[stopper.waitForStopExecution(t).executionID] = true
	}
	if !calls["exec-failed"] || !calls["exec-ok"] {
		t.Fatalf("unexpected StopExecution calls: %#v", calls)
	}
	waitForCleanupDone(t, svc)
	if _, err := os.Stat(filepath.Join(quickChatDir, "session-ok")); !os.IsNotExist(err) {
		t.Fatalf("successful session quick-chat directory should be removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(quickChatDir, "session-failed")); err != nil {
		t.Fatalf("failed session quick-chat directory should remain: %v", err)
	}
	cleanedIDs := cleanup.cleanedIDs()
	if len(cleanedIDs) != 1 || cleanedIDs[0] != "wt-ok" {
		t.Fatalf("expected only successful session worktree cleanup, got %#v", cleanedIDs)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-failed"); err != nil {
		t.Fatalf("failed session executor row should remain retryable: %v", err)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-ok"); err == nil {
		t.Fatal("successful session executor row should be removed")
	}
}

func TestService_DeleteTaskCleansMissingSessionExecutorRowAfterStop(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	stopper := newRecordingTaskExecutionStopper()
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session-missing",
		SessionID:        "session-missing",
		TaskID:           "task-123",
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
		AgentExecutionID: "exec-missing",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	if err := svc.DeleteTask(ctx, "task-123"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	call := stopper.waitForStopExecution(t)
	if call.executionID != "exec-missing" {
		t.Fatalf("StopExecution executionID = %q, want exec-missing", call.executionID)
	}
	waitForCleanupDone(t, svc)
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-missing"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("missing-session executor row should be removed after successful stop, got %v", err)
	}
}

func TestService_ArchiveTaskStopsExecutorRunningForTerminalSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	stopper := newRecordingTaskExecutionStopper()
	svc.SetExecutionStopper(stopper)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-completed",
		TaskID: "task-123",
		State:  models.TaskSessionStateCompleted,
	})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session-completed",
		SessionID:        "session-completed",
		TaskID:           "task-123",
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
		AgentExecutionID: "exec-terminal",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	if err := svc.ArchiveTask(ctx, "task-123"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	call := stopper.waitForStopExecution(t)
	if call.executionID != "exec-terminal" {
		t.Fatalf("StopExecution executionID = %q, want exec-terminal", call.executionID)
	}
	if call.reason != "task archived" {
		t.Fatalf("StopExecution reason = %q, want task archived", call.reason)
	}
	if !call.force {
		t.Fatal("StopExecution force = false, want true")
	}
	waitForCleanupDone(t, svc)
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-completed"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("executor row should be removed after successful stop, got %v", err)
	}
}

func TestService_DeleteTaskFailsClosedWhenRuntimeInventoryFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	inventoryErr := errors.New("inventory unavailable")
	svc.SetExecutionStopper(newRecordingTaskExecutionStopper())
	svc.executors = failingExecutorRepository{
		ExecutorRepository: repo,
		listByTaskErr:      inventoryErr,
	}

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})

	err := svc.DeleteTask(ctx, "task-123")
	if !errors.Is(err, inventoryErr) {
		t.Fatalf("DeleteTask error = %v, want inventory error", err)
	}
	if _, err := repo.GetTask(ctx, "task-123"); err != nil {
		t.Fatalf("task should remain when runtime inventory fails: %v", err)
	}
}

func TestService_ArchiveTaskFailsClosedWhenRuntimeInventoryFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	inventoryErr := errors.New("inventory unavailable")
	svc.SetExecutionStopper(newRecordingTaskExecutionStopper())
	svc.executors = failingExecutorRepository{
		ExecutorRepository: repo,
		listByTaskErr:      inventoryErr,
	}

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})

	err := svc.ArchiveTask(ctx, "task-123")
	if !errors.Is(err, inventoryErr) {
		t.Fatalf("ArchiveTask error = %v, want inventory error", err)
	}
	task, err := repo.GetTask(ctx, "task-123")
	if err != nil {
		t.Fatalf("task should remain when runtime inventory fails: %v", err)
	}
	if task.ArchivedAt != nil {
		t.Fatal("task should not be archived when runtime inventory fails")
	}
}

func TestService_CleanupTaskResourcesFailsClosedWhenRuntimeInventoryFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	inventoryErr := errors.New("inventory unavailable")
	svc.SetExecutionStopper(newRecordingTaskExecutionStopper())
	svc.executors = failingExecutorRepository{
		ExecutorRepository: repo,
		listByTaskErr:      inventoryErr,
	}
	quickChatDir := t.TempDir()
	svc.SetQuickChatDir(quickChatDir)
	cleanup := &recordingWorktreeCleanup{
		worktrees: []*worktree.Worktree{
			{ID: "wt-1", TaskID: "task-123", SessionID: "session-completed"},
		},
	}
	svc.SetWorktreeCleanup(cleanup)

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", Priority: "medium"})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:     "session-completed",
		TaskID: "task-123",
		State:  models.TaskSessionStateCompleted,
	})
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session-completed",
		SessionID:        "session-completed",
		TaskID:           "task-123",
		ExecutorID:       "executor-1",
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusStarting,
		AgentExecutionID: "exec-terminal",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}
	sessionDir := filepath.Join(quickChatDir, "session-completed")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	svc.CleanupTaskResources(ctx, "task-123", true)

	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session-completed"); err != nil {
		t.Fatalf("executor row should remain when runtime inventory fails: %v", err)
	}
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("quick-chat directory should remain when runtime inventory fails: %v", err)
	}
	if cleanedIDs := cleanup.cleanedIDs(); len(cleanedIDs) != 0 {
		t.Fatalf("worktrees should not be cleaned when runtime inventory fails, got %#v", cleanedIDs)
	}
}

func TestService_ListTasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 1", Priority: "medium"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Task 2", Priority: "medium"})

	tasks, err := svc.ListTasks(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

type stopExecutionCall struct {
	executionID string
	reason      string
	force       bool
}

type recordingTaskExecutionStopper struct {
	stopExecutionCh      chan stopExecutionCall
	stopExecutionErr     error
	stopExecutionErrByID map[string]error
}

func newRecordingTaskExecutionStopper() *recordingTaskExecutionStopper {
	return &recordingTaskExecutionStopper{stopExecutionCh: make(chan stopExecutionCall, 8)}
}

func (s *recordingTaskExecutionStopper) StopTask(context.Context, string, string, bool) error {
	return nil
}

func (s *recordingTaskExecutionStopper) StopSession(context.Context, string, string, bool) error {
	return nil
}

func (s *recordingTaskExecutionStopper) StopExecution(_ context.Context, executionID, reason string, force bool) error {
	s.stopExecutionCh <- stopExecutionCall{executionID: executionID, reason: reason, force: force}
	if s.stopExecutionErrByID != nil {
		if err := s.stopExecutionErrByID[executionID]; err != nil {
			return err
		}
	}
	return s.stopExecutionErr
}

func (s *recordingTaskExecutionStopper) waitForStopExecution(t *testing.T) stopExecutionCall {
	t.Helper()
	select {
	case call := <-s.stopExecutionCh:
		return call
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for StopExecution")
		return stopExecutionCall{}
	}
}

type failingExecutorRepository struct {
	repository.ExecutorRepository
	listByTaskErr error
}

func (r failingExecutorRepository) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	return nil, r.listByTaskErr
}

type recordingWorktreeCleanup struct {
	mu                sync.Mutex
	worktrees         []*worktree.Worktree
	worktreesByTaskID map[string][]*worktree.Worktree
	cleaned           []*worktree.Worktree
}

func (c *recordingWorktreeCleanup) OnTaskDeleted(context.Context, string) error {
	return nil
}

func (c *recordingWorktreeCleanup) GetAllByTaskID(_ context.Context, taskID string) ([]*worktree.Worktree, error) {
	if c.worktreesByTaskID != nil {
		return c.worktreesByTaskID[taskID], nil
	}
	return c.worktrees, nil
}

func (c *recordingWorktreeCleanup) CleanupWorktrees(_ context.Context, worktrees []*worktree.Worktree) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleaned = append(c.cleaned, worktrees...)
	return nil
}

func (c *recordingWorktreeCleanup) cleanedIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]string, 0, len(c.cleaned))
	for _, wt := range c.cleaned {
		if wt != nil {
			ids = append(ids, wt.ID)
		}
	}
	return ids
}

func waitForCleanupDone(t *testing.T, svc *Service) {
	t.Helper()
	select {
	case <-svc.cleanupDoneForTest:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for async task cleanup")
	}
}

func TestService_UpdateTaskState(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test", State: v1.TaskStateTODO, Priority: "medium"}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	updated, err := svc.UpdateTaskState(ctx, "task-123", v1.TaskStateInProgress)
	if err != nil {
		t.Fatalf("failed to update task state: %v", err)
	}
	if updated.State != v1.TaskStateInProgress {
		t.Errorf("expected state IN_PROGRESS, got %s", updated.State)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "task.state_changed" {
		t.Errorf("expected event type 'task.state_changed', got %s", events[0].Type)
	}
}

func TestService_MoveTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})

	task := &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-todo", Title: "Test", State: v1.TaskStateTODO, Priority: "medium"}
	_ = repo.CreateTask(ctx, task)
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-123", "wf-123", "step-done", 0)
	if err != nil {
		t.Fatalf("failed to move task: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-done" {
		t.Errorf("expected workflow step 'step-done', got %s", moved.Task.WorkflowStepID)
	}
}

// Workflow tests

func TestService_CreateWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	req := &CreateWorkflowRequest{
		WorkspaceID: "ws-1",
		Name:        "Test Workflow",
		Description: "A test workflow",
	}

	workflow, err := svc.CreateWorkflow(ctx, req)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	if workflow.ID == "" {
		t.Error("expected workflow ID to be set")
	}
	if workflow.Name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %s", workflow.Name)
	}
}

func TestService_GetWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	retrieved, err := svc.GetWorkflow(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to get workflow: %v", err)
	}
	if retrieved.Name != "Test Workflow" {
		t.Errorf("expected name 'Test Workflow', got %s", retrieved.Name)
	}
}

func TestService_UpdateWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Original"}
	_ = repo.CreateWorkflow(ctx, workflow)

	newName := "Updated"
	req := &UpdateWorkflowRequest{Name: &newName}

	updated, err := svc.UpdateWorkflow(ctx, "wf-123", req)
	if err != nil {
		t.Fatalf("failed to update workflow: %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %s", updated.Name)
	}
}
func TestService_DeleteWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	workflow := &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	err := svc.DeleteWorkflow(ctx, "wf-123")
	if err != nil {
		t.Fatalf("failed to delete workflow: %v", err)
	}

	_, err = svc.GetWorkflow(ctx, "wf-123")
	if err == nil {
		t.Error("expected workflow to be deleted")
	}
}

func TestService_ListWorkflows(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Workflow 1"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-1", Name: "Workflow 2"})

	workflows, err := svc.ListWorkflows(ctx, "ws-1", false)
	if err != nil {
		t.Fatalf("failed to list workflows: %v", err)
	}
	if len(workflows) != 2 {
		t.Errorf("expected 2 workflows, got %d", len(workflows))
	}
}

// Message tests

func setupTestTask(t *testing.T, repo *sqliterepo.Repository) {
	t.Helper()
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-123", WorkspaceID: "ws-1", Name: "Workflow"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-123", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123", Title: "Test Task", Priority: "medium"})
}

func setupTestSession(t *testing.T, repo *sqliterepo.Repository) string {
	t.Helper()
	ctx := context.Background()
	session := &models.TaskSession{
		ID:             "session-123",
		TaskID:         "task-123",
		AgentProfileID: "profile-123",
		State:          models.TaskSessionStateStarting,
	}
	_ = repo.CreateTaskSession(ctx, session)
	return session.ID
}

func TestService_DismissLastAgentError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	occurredAt := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	lastErr := models.LastAgentError{
		Message:          "peer disconnected before response",
		OccurredAt:       occurredAt,
		AgentExecutionID: "exec-1",
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, lastErr); err != nil {
		t.Fatalf("seed last agent error: %v", err)
	}

	session, err := svc.DismissLastAgentError(ctx, sessionID, lastErr.Stamp())
	if err != nil {
		t.Fatalf("dismiss last agent error: %v", err)
	}
	got, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		t.Fatalf("expected last agent error metadata after dismiss")
	}
	if !got.IsDismissed() {
		t.Fatalf("expected dismissed last agent error, got %#v", got)
	}
}

func TestService_DismissLastAgentErrorAcceptsBrowserTimestampStamp(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	occurredAt, err := time.Parse(time.RFC3339Nano, "2026-06-14T12:00:00.310Z")
	if err != nil {
		t.Fatalf("parse occurred_at: %v", err)
	}
	lastErr := models.LastAgentError{
		Message:    "peer disconnected before response",
		OccurredAt: occurredAt,
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, lastErr); err != nil {
		t.Fatalf("seed last agent error: %v", err)
	}

	session, err := svc.DismissLastAgentError(ctx, sessionID, "2026-06-14T12:00:00.310Z:"+lastErr.Message)
	if err != nil {
		t.Fatalf("dismiss last agent error: %v", err)
	}
	got, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		t.Fatalf("expected last agent error metadata after dismiss")
	}
	if !got.IsDismissed() {
		t.Fatalf("expected dismissed last agent error, got %#v", got)
	}
}

func TestService_DismissLastAgentErrorIgnoresStaleStamp(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	lastErr := models.LastAgentError{
		Message:    "fresh error",
		OccurredAt: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC),
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, lastErr); err != nil {
		t.Fatalf("seed last agent error: %v", err)
	}

	session, err := svc.DismissLastAgentError(ctx, sessionID, "stale:error")
	if err != nil {
		t.Fatalf("dismiss last agent error: %v", err)
	}
	got, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		t.Fatalf("expected last agent error metadata after stale dismiss")
	}
	if got.IsDismissed() {
		t.Fatalf("expected stale dismiss to leave error visible, got %#v", got)
	}
}

func setupTestTurn(t *testing.T, repo *sqliterepo.Repository, sessionID, taskID, turnID string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("failed to create test turn: %v", err)
	}
	return turn.ID
}

func TestService_CreateMessage(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	// Create a task first
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")
	eventBus.ClearEvents()

	req := &CreateMessageRequest{
		TaskSessionID: sessionID,
		TurnID:        turnID,
		Content:       "This is a test comment",
		AuthorType:    "user",
		AuthorID:      "user-123",
	}

	comment, err := svc.CreateMessage(ctx, req)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.ID == "" {
		t.Error("expected comment ID to be set")
	}
	if comment.Content != "This is a test comment" {
		t.Errorf("expected content 'This is a test comment', got %s", comment.Content)
	}
	if comment.AuthorType != models.MessageAuthorUser {
		t.Errorf("expected author type 'user', got %s", comment.AuthorType)
	}

	// Check event was published
	events := eventBus.GetPublishedEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "message.added" {
		t.Errorf("expected event type 'message.added', got %s", events[0].Type)
	}
}

func TestService_CreateAgentMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	req := &CreateMessageRequest{
		TaskSessionID: sessionID,
		TurnID:        turnID,
		Content:       "What should I do next?",
		AuthorType:    "agent",
		AuthorID:      "agent-123",
		RequestsInput: true,
	}

	comment, err := svc.CreateMessage(ctx, req)
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}

	if comment.AuthorType != models.MessageAuthorAgent {
		t.Errorf("expected author type 'agent', got %s", comment.AuthorType)
	}
	if !comment.RequestsInput {
		t.Error("expected RequestsInput to be true")
	}
}

type failingTurnCreator struct {
	repository.TurnRepository
	err error
}

func (f failingTurnCreator) CreateTurn(context.Context, *models.Turn) error {
	return f.err
}

func TestService_CreateMessageReturnsTurnStartError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnErr := errors.New("turn table unavailable")
	svc.turns = failingTurnCreator{TurnRepository: repo, err: turnErr}

	_, err := svc.CreateMessage(ctx, &CreateMessageRequest{
		TaskSessionID: sessionID,
		Content:       "new prompt",
		AuthorType:    "user",
	})

	if !errors.Is(err, turnErr) {
		t.Fatalf("CreateMessage error = %v, want wrapped turn error", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("CreateMessage returned masked FK error: %v", err)
	}
}

func TestService_CreateMessageWithIDReturnsTurnStartError(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnErr := errors.New("turn table unavailable")
	svc.turns = failingTurnCreator{TurnRepository: repo, err: turnErr}

	_, err := svc.CreateMessageWithID(ctx, "message-123", &CreateMessageRequest{
		TaskSessionID: sessionID,
		Content:       "streamed output",
		AuthorType:    "agent",
	})

	if !errors.Is(err, turnErr) {
		t.Fatalf("CreateMessageWithID error = %v, want wrapped turn error", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("CreateMessageWithID returned masked FK error: %v", err)
	}
}

func TestService_CreateMessageSessionNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	req := &CreateMessageRequest{
		TaskSessionID: "nonexistent",
		Content:       "Test comment",
	}

	_, err := svc.CreateMessage(ctx, req)
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestService_GetMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	comment := &models.Message{ID: "comment-123", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Test"}
	_ = repo.CreateMessage(ctx, comment)

	retrieved, err := svc.GetMessage(ctx, "comment-123")
	if err != nil {
		t.Fatalf("failed to get comment: %v", err)
	}
	if retrieved.Content != "Test" {
		t.Errorf("expected content 'Test', got %s", retrieved.Content)
	}
}

func TestService_ListMessages(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-1", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Comment 1"})
	_ = repo.CreateMessage(ctx, &models.Message{ID: "comment-2", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorAgent, Content: "Comment 2"})

	comments, err := svc.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
}

func TestService_DeleteMessage(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-123")

	comment := &models.Message{ID: "comment-123", TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID, AuthorType: models.MessageAuthorUser, Content: "Test"}
	_ = repo.CreateMessage(ctx, comment)

	err := svc.DeleteMessage(ctx, "comment-123")
	if err != nil {
		t.Fatalf("failed to delete comment: %v", err)
	}

	_, err = svc.GetMessage(ctx, "comment-123")
	if err == nil {
		t.Error("expected comment to be deleted")
	}
}
