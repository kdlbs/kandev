package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	_ "github.com/mattn/go-sqlite3"
)

type projectTaskStub struct {
	db                *sqlx.DB
	created           map[string]*models.Task
	defaultExecutorID string
	createErr         error
}

type projectWorkerChangeRequestReaderStub struct {
	changes []WorkerChangeRequest
}

func (s projectWorkerChangeRequestReaderStub) ListWorkerChangeRequests(context.Context, string) ([]WorkerChangeRequest, error) {
	return s.changes, nil
}

func (s *projectTaskStub) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error {
	return nil
}

func (s *projectTaskStub) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	executorID := s.defaultExecutorID
	if executorID == "" {
		executorID = "executor-local"
	}
	return &models.Workspace{ID: "workspace-1", DefaultExecutorID: &executorID}, nil
}

func (s *projectTaskStub) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	return &models.Repository{ID: id, WorkspaceID: "workspace-1", Name: id, SourceType: "github", ProviderOwner: "acme", ProviderName: id, DefaultBranch: "main"}, nil
}

func (s *projectTaskStub) ListBranches(context.Context, string, string) ([]taskservice.Branch, error) {
	return []taskservice.Branch{{Name: "main", Type: "remote"}}, nil
}

func (s *projectTaskStub) GetExecutorProfile(context.Context, string) (*models.ExecutorProfile, error) {
	return &models.ExecutorProfile{ID: "executor-profile", ExecutorID: "executor-local"}, nil
}

func (s *projectTaskStub) ListExecutorProfiles(_ context.Context, executorID string) ([]*models.ExecutorProfile, error) {
	profileID := "executor-profile"
	if executorID != "executor-local" {
		profileID = executorID + "-profile"
	}
	return []*models.ExecutorProfile{{ID: profileID, ExecutorID: executorID}}, nil
}

func (s *projectTaskStub) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	return &models.Executor{ID: id, Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive}, nil
}

func (s *projectTaskStub) CreateAgentProjectTask(ctx context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error) {
	if s.createErr != nil {
		return taskservice.CreateTaskResult{}, s.createErr
	}
	task := &models.Task{
		ID: uuid.NewString(), WorkspaceID: req.WorkspaceID, Title: req.Title,
		AgentProjectID: req.AgentProjectID, AgentProjectTier: req.AgentProjectTier,
		AgentProjectProfileID: req.AgentProjectProfileID, Origin: models.TaskOriginAgentProject,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, parent_id, origin, agent_project_id, agent_project_tier, agent_project_profile_id, state, created_at, updated_at)
		VALUES (?, ?, '', '', ?, '', ?, ?, ?, ?, 'CREATED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, task.ID, task.WorkspaceID, task.Title, task.Origin, task.AgentProjectID, task.AgentProjectTier, task.AgentProjectProfileID)
	if err != nil {
		return taskservice.CreateTaskResult{}, err
	}
	if s.created == nil {
		s.created = make(map[string]*models.Task)
	}
	s.created[task.ID] = task
	return taskservice.CreateTaskResult{Task: task}, nil
}

func (s *projectTaskStub) GetTask(_ context.Context, id string) (*models.Task, error) {
	if task := s.created[id]; task != nil {
		return task, nil
	}
	return nil, taskrepo.ErrTaskNotFound
}

type projectProfileStub struct{ workspaceID string }

func (s projectProfileStub) GetAgentProfile(_ context.Context, id string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{ID: id, AgentID: "claude-acp", WorkspaceID: s.workspaceID, Enabled: true}, nil
}

func projectFixture(t *testing.T, enabled bool) (*Service, *sqlx.DB) {
	return projectFixtureWithProfileWorkspace(t, enabled, "workspace-1")
}

func projectFixtureWithProfileWorkspace(t *testing.T, enabled bool, profileWorkspaceID string) (*Service, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "projects.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`
		CREATE TABLE agent_projects (
			id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT NOT NULL,
			repository_ids TEXT NOT NULL, primary_repository_id TEXT NOT NULL,
			coordinator_profile_id TEXT NOT NULL, economy_profile_id TEXT NOT NULL,
			frontier_profile_id TEXT NOT NULL, executor_profile_id TEXT NOT NULL,
			main_task_id TEXT NOT NULL DEFAULT '', request_key TEXT NOT NULL DEFAULT '',
			revision BIGINT NOT NULL DEFAULT 1, archived_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
		);
		CREATE UNIQUE INDEX idx_agent_projects_request_key ON agent_projects(workspace_id, request_key) WHERE request_key != '';
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, workflow_step_id TEXT,
			title TEXT, parent_id TEXT, origin TEXT, agent_project_id TEXT,
			agent_project_tier TEXT, agent_project_profile_id TEXT,
			state TEXT, archived_at TIMESTAMP, created_at TIMESTAMP, updated_at TIMESTAMP
		);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, state TEXT NOT NULL);
	`)
	if err != nil {
		t.Fatalf("create schema: %v", err)
	}
	tasks := &projectTaskStub{db: db}
	service := NewService(NewStore(db), tasks, projectProfileStub{workspaceID: profileWorkspaceID}, enabled)
	service.SetProjectWorkspaceProfileSupportChecker(func(_ context.Context, agentID string) bool {
		return agentID == "claude-acp"
	})
	service.SetContextStore(NewContextStore(filepath.Join(t.TempDir(), "agent-projects")))
	return service, db
}

func TestCreateProjectAcceptsGlobalProfilesAndRejectsOtherWorkspaceProfiles(t *testing.T) {
	globalService, _ := projectFixtureWithProfileWorkspace(t, true, "")
	if _, err := globalService.Create(context.Background(), validCreateRequest()); err != nil {
		t.Fatalf("Create with global profile: %v", err)
	}

	foreignService, _ := projectFixtureWithProfileWorkspace(t, true, "workspace-other")
	if _, err := foreignService.Create(context.Background(), validCreateRequest()); !errors.Is(err, ErrDependenciesMissing) {
		t.Fatalf("Create with another workspace's profile = %v, want ErrDependenciesMissing", err)
	}
}

func TestProjectCreateAndEditRejectProfilesWithoutWorkspaceAccess(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		svc, db := projectFixture(t, true)
		svc.SetProjectWorkspaceProfileSupportChecker(func(context.Context, string) bool { return false })
		if _, err := svc.Create(context.Background(), validCreateRequest()); !errors.Is(err, ErrUnsupportedProjectAgentProfile) {
			t.Fatalf("Create error = %v, want unsupported project profile", err)
		}
		var projectCount int
		if err := db.Get(&projectCount, `SELECT COUNT(*) FROM agent_projects`); err != nil {
			t.Fatal(err)
		}
		if projectCount != 0 {
			t.Fatalf("Create persisted %d projects with an unsupported profile", projectCount)
		}
	})

	t.Run("edit", func(t *testing.T) {
		svc, _ := projectFixture(t, true)
		project, err := svc.Create(context.Background(), validCreateRequest())
		if err != nil {
			t.Fatal(err)
		}
		svc.SetProjectWorkspaceProfileSupportChecker(func(context.Context, string) bool { return false })
		name := "Updated project"
		if _, err := svc.Update(context.Background(), project.WorkspaceID, project.ID, UpdateRequest{
			Revision: project.Revision, Name: &name,
		}); !errors.Is(err, ErrUnsupportedProjectAgentProfile) {
			t.Fatalf("Update error = %v, want unsupported project profile", err)
		}
	})
}

func TestProjectContextServiceUsesWorkspaceAuthorizationAndHashConflict(t *testing.T) {
	svc, _ := projectFixture(t, true)
	ctx := context.Background()
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	entries, err := svc.ListContext(ctx, project.WorkspaceID, project.ID, "")
	if err != nil || len(entries) != 1 || entries[0].Name != "notes.md" {
		t.Fatalf("ListContext = %#v, err %v", entries, err)
	}
	_, hash, err := svc.ReadContextFile(ctx, project.WorkspaceID, project.ID, "notes.md")
	if err != nil {
		t.Fatalf("ReadContextFile: %v", err)
	}
	newHash, err := svc.WriteContextFile(ctx, project.WorkspaceID, project.ID, "notes.md", hash, []byte("new context\n"))
	if err != nil || newHash == hash {
		t.Fatalf("WriteContextFile hash=%q err=%v", newHash, err)
	}
	if _, err := svc.WriteContextFile(ctx, project.WorkspaceID, project.ID, "notes.md", hash, []byte("stale\n")); !errors.Is(err, ErrContextConflict) {
		t.Fatalf("stale context write = %v, want ErrContextConflict", err)
	}
}

func validCreateRequest() CreateRequest {
	return CreateRequest{
		WorkspaceID: "workspace-1", Name: "Release migration",
		RepositoryIDs: []string{"repo-api", "repo-web"}, PrimaryRepositoryID: "repo-api",
		CoordinatorProfileID: "profile-coordinator", EconomyProfileID: "profile-economy",
		FrontierProfileID: "profile-frontier", RequestKey: "create-001",
	}
}

type projectLifecycleStub struct {
	archiveErr     error
	deleteErr      error
	outcome        *taskservice.CascadeOutcome
	archiveStarted chan struct{}
	archiveProceed <-chan struct{}
	delete         func(context.Context, string, string, taskservice.DeleteTaskOptions) (*taskservice.CascadeOutcome, error)
}

func (s projectLifecycleStub) ArchiveAgentProjectTree(context.Context, string, string) (*taskservice.CascadeOutcome, error) {
	if s.archiveStarted != nil {
		close(s.archiveStarted)
	}
	if s.archiveProceed != nil {
		<-s.archiveProceed
	}
	return s.outcome, s.archiveErr
}

func (projectLifecycleStub) UnarchiveAgentProjectTree(context.Context, string, string) (*taskservice.CascadeOutcome, error) {
	return &taskservice.CascadeOutcome{}, nil
}

func (s projectLifecycleStub) DeleteAgentProjectTree(ctx context.Context, projectID, rootID string, options taskservice.DeleteTaskOptions) (*taskservice.CascadeOutcome, error) {
	if s.delete != nil {
		return s.delete(ctx, projectID, rootID, options)
	}
	return s.outcome, s.deleteErr
}

func (projectLifecycleStub) SetAgentProjectActionAuthorizer(func(context.Context, string, string) error) {
}

type projectWorkerStarterStub struct{}

func (projectWorkerStarterStub) StartAgentProjectWorker(context.Context, string, string, string, string) error {
	return nil
}

func TestDeleteProjectRetainsOrRemovesContextByRequest(t *testing.T) {
	svc, _ := projectFixture(t, true)
	svc.SetTaskLifecycleCoordinator(projectLifecycleStub{})
	ctx := context.Background()

	retained, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create retained project: %v", err)
	}
	retainedContext, err := svc.ContextPath(retained.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, retained.WorkspaceID, retained.ID, false, false); err != nil {
		t.Fatalf("Delete with retained context: %v", err)
	}
	if _, err := os.Stat(retainedContext); err != nil {
		t.Fatalf("retained context stat: %v", err)
	}

	request := validCreateRequest()
	request.RequestKey = "create-delete-context"
	removed, err := svc.Create(ctx, request)
	if err != nil {
		t.Fatalf("Create removable project: %v", err)
	}
	removedContext, err := svc.ContextPath(removed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, removed.WorkspaceID, removed.ID, false, true); err != nil {
		t.Fatalf("Delete with context removal: %v", err)
	}
	if _, err := os.Stat(removedContext); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed context stat error = %v, want not-exist", err)
	}
}

func TestProjectCascadeFailureLeavesProjectVisibleAndRetryable(t *testing.T) {
	t.Run("archive preflight failure", func(t *testing.T) {
		svc, _ := projectFixture(t, true)
		project, err := svc.Create(context.Background(), validCreateRequest())
		if err != nil {
			t.Fatal(err)
		}
		svc.SetTaskLifecycleCoordinator(projectLifecycleStub{archiveErr: errors.New("dirty worktree")})

		if _, err := svc.Archive(context.Background(), project.WorkspaceID, project.ID); err == nil {
			t.Fatal("Archive succeeded despite the lifecycle preflight failure")
		}
		assertProjectRemainsActive(t, svc, project)
	})

	t.Run("archive partial cascade failure", func(t *testing.T) {
		svc, _ := projectFixture(t, true)
		project, err := svc.Create(context.Background(), validCreateRequest())
		if err != nil {
			t.Fatal(err)
		}
		svc.SetTaskLifecycleCoordinator(projectLifecycleStub{
			archiveErr: errors.New("post-commit cleanup failed"),
			outcome:    &taskservice.CascadeOutcome{ArchivedTaskIDs: []string{project.MainTaskID}},
		})

		if _, err := svc.Archive(context.Background(), project.WorkspaceID, project.ID); err == nil {
			t.Fatal("Archive succeeded despite the partial cascade failure")
		}
		assertProjectRemainsActive(t, svc, project)
	})

	t.Run("delete preflight failure", func(t *testing.T) {
		svc, _ := projectFixture(t, true)
		project, err := svc.Create(context.Background(), validCreateRequest())
		if err != nil {
			t.Fatal(err)
		}
		svc.SetTaskLifecycleCoordinator(projectLifecycleStub{deleteErr: errors.New("dirty worktree")})

		if err := svc.Delete(context.Background(), project.WorkspaceID, project.ID, false, false); err == nil {
			t.Fatal("Delete succeeded despite the lifecycle preflight failure")
		}
		assertProjectRemainsActive(t, svc, project)
	})

	t.Run("delete partial cascade failure", func(t *testing.T) {
		svc, _ := projectFixture(t, true)
		project, err := svc.Create(context.Background(), validCreateRequest())
		if err != nil {
			t.Fatal(err)
		}
		svc.SetTaskLifecycleCoordinator(projectLifecycleStub{
			deleteErr: errors.New("partial task deletion failed"),
			outcome:   &taskservice.CascadeOutcome{ArchivedTaskIDs: []string{project.MainTaskID}},
		})

		if err := svc.Delete(context.Background(), project.WorkspaceID, project.ID, false, false); err == nil {
			t.Fatal("Delete succeeded despite the partial cascade failure")
		}
		assertProjectRemainsActive(t, svc, project)
	})
}

func TestCreateProjectHidesIncompleteCoordinatorAndRecoversOnRetry(t *testing.T) {
	svc, _ := projectFixture(t, true)
	tasks := svc.tasks.(*projectTaskStub)
	tasks.createErr = errors.New("coordinator task creation failed")
	request := validCreateRequest()
	if _, err := svc.Create(context.Background(), request); err == nil {
		t.Fatal("Create succeeded despite coordinator task creation failure")
	}
	projects, err := svc.List(context.Background(), request.WorkspaceID, false)
	if err != nil {
		t.Fatalf("List after incomplete create: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("incomplete projects = %#v, want hidden from lists", projects)
	}

	tasks.createErr = nil
	project, err := svc.Create(context.Background(), request)
	if err != nil {
		t.Fatalf("retry Create: %v", err)
	}
	if project.MainTaskID == "" {
		t.Fatal("retry returned project without a coordinator")
	}
	projects, err = svc.List(context.Background(), request.WorkspaceID, false)
	if err != nil || len(projects) != 1 || projects[0].ID != project.ID {
		t.Fatalf("List after recovery = %#v, %v; want recovered project", projects, err)
	}
}

func TestProjectDeletionCanRetryAfterCascadeAndContextFailure(t *testing.T) {
	svc, db := projectFixture(t, true)
	project, err := svc.Create(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	tasks := svc.tasks.(*projectTaskStub)
	deleteCalls := 0
	svc.SetTaskLifecycleCoordinator(projectLifecycleStub{delete: func(ctx context.Context, _, rootID string, _ taskservice.DeleteTaskOptions) (*taskservice.CascadeOutcome, error) {
		deleteCalls++
		delete(tasks.created, rootID)
		if _, err := db.ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", rootID); err != nil {
			return nil, err
		}
		return &taskservice.CascadeOutcome{ArchivedTaskIDs: []string{rootID}}, nil
	}})
	contextStore := svc.context
	svc.SetContextStore(nil)
	if err := svc.Delete(context.Background(), project.WorkspaceID, project.ID, false, true); !errors.Is(err, ErrContextUnavailable) {
		t.Fatalf("first Delete error = %v, want context unavailable", err)
	}
	svc.SetContextStore(contextStore)
	if err := svc.Delete(context.Background(), project.WorkspaceID, project.ID, false, true); err != nil {
		t.Fatalf("retry Delete: %v", err)
	}
	if deleteCalls != 1 {
		t.Fatalf("task cascade calls = %d, want one after successful task deletion", deleteCalls)
	}
	if _, err := svc.store.Get(context.Background(), project.WorkspaceID, project.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("project after retry = %v, want deleted", err)
	}
}

func TestProjectArchiveSerializesWorkerAdmission(t *testing.T) {
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	proceed := make(chan struct{})
	svc.SetTaskLifecycleCoordinator(projectLifecycleStub{archiveStarted: started, archiveProceed: proceed})
	svc.SetWorkerStarter(projectWorkerStarterStub{})
	archiveDone := make(chan error, 1)
	go func() {
		_, err := svc.Archive(context.Background(), project.WorkspaceID, project.ID)
		archiveDone <- err
	}()
	<-started
	workerDone := make(chan error, 1)
	go func() {
		_, err := svc.CreateWorker(context.Background(), project.MainTaskID, CreateWorkerRequest{
			Tier: "economy", Title: "Worker", Prompt: "Do the work", Repositories: []WorkerRepositoryInput{{RepositoryID: "repo-api"}},
		})
		workerDone <- err
	}()
	select {
	case err := <-workerDone:
		t.Fatalf("worker admission completed during archive: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(proceed)
	if err := <-archiveDone; err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if err := <-workerDone; !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("worker creation after archive = %v, want invalid project", err)
	}
}

func assertProjectRemainsActive(t *testing.T, svc *Service, project *ProjectView) {
	t.Helper()
	got, err := svc.Get(context.Background(), project.WorkspaceID, project.ID)
	if err != nil {
		t.Fatalf("failed cascade made project unavailable for retry: %v", err)
	}
	if got.ArchivedAt != nil {
		t.Fatalf("failed cascade archived project at %v", got.ArchivedAt)
	}
}

func TestCreateProjectCreatesOneWorkflowFreeCoordinatorAndIsIdempotent(t *testing.T) {
	svc, db := projectFixture(t, true)
	ctx := context.Background()
	first, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("retry Create: %v", err)
	}
	if first.ID != second.ID || first.MainTaskID == "" || first.MainTaskID != second.MainTaskID {
		t.Fatalf("retry changed project/coordinator identity: first=%#v second=%#v", first, second)
	}
	if first.ExecutorProfileID != "executor-profile" {
		t.Fatalf("project executor profile = %q, want resolved workspace-default profile", first.ExecutorProfileID)
	}
	if len(first.Tasks) != 1 || first.Tasks[0].ID != first.MainTaskID || first.Tasks[0].ParentID != "" {
		t.Fatalf("project tasks = %#v, want one coordinator", first.Tasks)
	}
	var projects, tasks int
	if err := db.Get(&projects, `SELECT COUNT(*) FROM agent_projects`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&tasks, `SELECT COUNT(*) FROM tasks WHERE agent_project_id = ?`, first.ID); err != nil {
		t.Fatal(err)
	}
	if projects != 1 || tasks != 1 {
		t.Fatalf("persisted projects=%d tasks=%d, want 1 each", projects, tasks)
	}
}

func TestDisabledProjectCreationHasNoSideEffects(t *testing.T) {
	svc, db := projectFixture(t, false)
	if _, err := svc.Create(context.Background(), validCreateRequest()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Create error = %v, want ErrDisabled", err)
	}
	var projects, tasks int
	_ = db.Get(&projects, `SELECT COUNT(*) FROM agent_projects`)
	_ = db.Get(&tasks, `SELECT COUNT(*) FROM tasks`)
	if projects != 0 || tasks != 0 {
		t.Fatalf("disabled create wrote projects=%d tasks=%d", projects, tasks)
	}
}

func TestProjectUpdateUsesRevisionAndProtectsRunningRepositories(t *testing.T) {
	svc, db := projectFixture(t, true)
	ctx := context.Background()
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	name := "Updated migration"
	updated, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{Revision: project.Revision, Name: &name})
	if err != nil || updated.Revision != project.Revision+1 || updated.Name != name {
		t.Fatalf("Update: got %#v, err %v", updated, err)
	}
	if _, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{Revision: project.Revision, Name: &name}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Update error = %v, want revision conflict", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO task_sessions (id, task_id, state) VALUES ('s-1', ?, 'RUNNING')`, project.MainTaskID); err != nil {
		t.Fatal(err)
	}
	repositories := []string{"repo-api"}
	if _, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{Revision: updated.Revision, RepositoryIDs: repositories}); !errors.Is(err, ErrRepositoryInUse) {
		t.Fatalf("repository edit while running = %v, want ErrRepositoryInUse", err)
	}
}

func TestProjectUpdateExplicitlyAppliesWorkspaceDefaultExecutor(t *testing.T) {
	svc, _ := projectFixture(t, true)
	ctx := context.Background()
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tasks := svc.tasks.(*projectTaskStub)
	tasks.defaultExecutorID = "executor-updated"
	updated, err := svc.Update(ctx, project.WorkspaceID, project.ID, UpdateRequest{
		Revision:                      project.Revision,
		ApplyWorkspaceDefaultExecutor: true,
	})
	if err != nil {
		t.Fatalf("Update with workspace default: %v", err)
	}
	if updated.ExecutorProfileID != "executor-updated-profile" {
		t.Fatalf("updated project executor profile = %q, want executor-updated-profile", updated.ExecutorProfileID)
	}
}

func TestProviderBackedRepositoryWithoutRemoteURLCanBeSelectedByWorker(t *testing.T) {
	svc, _ := projectFixture(t, true)
	project, err := svc.Create(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("Create accepted provider-backed project repositories: %v", err)
	}

	selected, err := svc.resolveWorkerRepositories(context.Background(), &project.Project, []WorkerRepositoryInput{{RepositoryID: "repo-api"}})
	if err != nil {
		t.Fatalf("resolveWorkerRepositories rejected provider-backed repository without remote_url: %v", err)
	}
	if len(selected) != 1 || selected[0].RepositoryID != "repo-api" || selected[0].BaseBranch != "main" {
		t.Fatalf("selected worker repositories = %#v, want repo-api at main", selected)
	}
}

func TestWorkerInspectionIncludesBoundedChangeRequests(t *testing.T) {
	svc, db := projectFixture(t, true)
	ctx := context.Background()
	project, err := svc.Create(ctx, validCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	worker := &models.Task{
		ID: "worker-cr-1", WorkspaceID: project.WorkspaceID, ParentID: project.MainTaskID,
		AgentProjectID: project.ID, AgentProjectTier: models.AgentProjectTierEconomy,
		AgentProjectProfileID: project.EconomyProfileID, Title: "Worker with pull request",
		State: "DONE", UpdatedAt: time.Now().UTC(),
	}
	tasks := svc.tasks.(*projectTaskStub)
	tasks.created[worker.ID] = worker
	if _, err := db.ExecContext(ctx, `
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, parent_id, origin,
			agent_project_id, agent_project_tier, agent_project_profile_id, state, created_at, updated_at)
		VALUES (?, ?, '', '', ?, ?, 'agent_project', ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, worker.ID, worker.WorkspaceID, worker.Title, worker.ParentID, worker.AgentProjectID,
		worker.AgentProjectTier, worker.AgentProjectProfileID, worker.State); err != nil {
		t.Fatal(err)
	}
	changes := make([]WorkerChangeRequest, MaxWorkerChangeRequests+3)
	for i := range changes {
		changes[i] = WorkerChangeRequest{
			Provider: "github", Number: i + 1, URL: "https://github.com/acme/repo/pull/1",
			Title: "Change request", State: "open",
		}
	}
	changes[0].Title = strings.Repeat("x", 520)
	svc.SetWorkerChangeRequestReader(projectWorkerChangeRequestReaderStub{changes: changes})

	listed, err := svc.ListWorkers(ctx, project.MainTaskID)
	if err != nil {
		t.Fatalf("ListWorkers: %v", err)
	}
	got, err := svc.GetWorker(ctx, project.MainTaskID, worker.ID)
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListWorkers returned %d workers, want 1", len(listed))
	}
	if len(got.ChangeRequests) != MaxWorkerChangeRequests || len(listed[0].ChangeRequests) != MaxWorkerChangeRequests {
		t.Fatalf("change request counts list=%d get=%d, want %d", len(listed[0].ChangeRequests), len(got.ChangeRequests), MaxWorkerChangeRequests)
	}
	if len([]rune(got.ChangeRequests[0].Title)) > 501 {
		t.Fatalf("worker change request title has %d runes, want at most 501", len([]rune(got.ChangeRequests[0].Title)))
	}
	if got.ChangeRequests[0].URL != listed[0].ChangeRequests[0].URL || got.ChangeRequests[0].Number != listed[0].ChangeRequests[0].Number {
		t.Fatalf("ListWorkers and GetWorker change request summaries differ: list=%#v get=%#v", listed[0].ChangeRequests[0], got.ChangeRequests[0])
	}
}
