package projects

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

var (
	ErrDisabled                       = errors.New("agent projects are disabled")
	ErrInvalidProject                 = errors.New("invalid agent project")
	ErrRevisionConflict               = errors.New("agent project was changed by another request")
	ErrCoordinatorConflict            = errors.New("agent project already has a different coordinator")
	ErrRepositoryInUse                = errors.New("repositories cannot change while a project agent is running")
	ErrProjectRunning                 = errors.New("project tasks must be stopped before project deletion")
	ErrExecutorIncompatible           = errors.New("workspace default executor is not a local worktree executor")
	ErrDependenciesMissing            = errors.New("one or more project dependencies are unavailable")
	ErrLaunchBlocked                  = errors.New("agent project launch is blocked by its current configuration")
	ErrContextUnavailable             = errors.New("project context storage is unavailable")
	ErrUnsupportedProjectAgentProfile = errors.New("agent profile does not support project workspace access")
)

type taskAccess interface {
	AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error
	GetWorkspace(context.Context, string) (*models.Workspace, error)
	GetRepository(context.Context, string) (*models.Repository, error)
	GetExecutorProfile(context.Context, string) (*models.ExecutorProfile, error)
	ListExecutorProfiles(context.Context, string) ([]*models.ExecutorProfile, error)
	GetExecutor(context.Context, string) (*models.Executor, error)
	CreateAgentProjectTask(context.Context, *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error)
	GetTask(context.Context, string) (*models.Task, error)
}

type profileReader interface {
	GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error)
}

type taskLifecycle interface {
	ArchiveAgentProjectTree(context.Context, string, string) (*taskservice.CascadeOutcome, error)
	UnarchiveAgentProjectTree(context.Context, string, string) (*taskservice.CascadeOutcome, error)
	DeleteAgentProjectTree(context.Context, string, string, taskservice.DeleteTaskOptions) (*taskservice.CascadeOutcome, error)
	SetAgentProjectActionAuthorizer(func(context.Context, string, string) error)
}

type Service struct {
	store                            *Store
	context                          *ContextStore
	tasks                            taskAccess
	profiles                         profileReader
	enabled                          bool
	lifecycle                        taskLifecycle
	workerMu                         sync.Mutex
	workerStarter                    projectWorkerStarter
	projectWorkspaceProfileSupported func(context.Context, string) bool
	workerChangeRequestReader        projectWorkerChangeRequestReader
}

func NewService(store *Store, tasks taskAccess, profiles profileReader, enabled bool) *Service {
	service := &Service{store: store, tasks: tasks, profiles: profiles, enabled: enabled}
	if tasks != nil {
		// The task service calls this before admitting the only workflow-free
		// task creation path. It prevents generic task routes from choosing a
		// project identity or parent.
		if setter, ok := tasks.(interface {
			SetAgentProjectTaskAuthorizer(func(context.Context, *taskservice.CreateTaskRequest) error)
		}); ok {
			setter.SetAgentProjectTaskAuthorizer(service.AuthorizeTaskCreation)
		}
		if setter, ok := tasks.(interface {
			SetAgentProjectActionAuthorizer(func(context.Context, string, string) error)
		}); ok {
			setter.SetAgentProjectActionAuthorizer(service.AuthorizeTaskLifecycle)
		}
		if setter, ok := tasks.(interface{ SetAgentProjectsEnabled(bool) }); ok {
			setter.SetAgentProjectsEnabled(enabled)
		}
	}
	return service
}

func (s *Service) SetTaskLifecycleCoordinator(lifecycle taskLifecycle) {
	s.lifecycle = lifecycle
	if lifecycle != nil {
		lifecycle.SetAgentProjectActionAuthorizer(s.AuthorizeTaskLifecycle)
	}
}

func (s *Service) SetContextStore(contextStore *ContextStore) {
	s.context = contextStore
}

func (s *Service) ContextPath(projectID string) (string, error) {
	if s.context == nil {
		return "", ErrContextUnavailable
	}
	return s.context.ContextPath(projectID)
}

func (s *Service) PrimaryRepositoryID(ctx context.Context, workspaceID, projectID string) (string, error) {
	project, err := s.store.Get(ctx, workspaceID, projectID)
	if err != nil {
		return "", err
	}
	return project.PrimaryRepositoryID, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (*ProjectView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	req.WorkspaceID = strings.TrimSpace(req.WorkspaceID)
	req.Name = strings.TrimSpace(req.Name)
	req.RequestKey = strings.TrimSpace(req.RequestKey)
	if req.WorkspaceID == "" || req.Name == "" || req.RequestKey == "" {
		return nil, fmt.Errorf("%w: workspace, name, and request_key are required", ErrInvalidProject)
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, req.WorkspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	if err := validateRepositorySelectionShape(req.RepositoryIDs, req.PrimaryRepositoryID); err != nil {
		return nil, err
	}
	existing, err := s.existingProjectForRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return s.getView(ctx, existing)
	}
	project, err := s.buildProject(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.persistProject(ctx, project, req)
}

func (s *Service) existingProjectForRequest(ctx context.Context, req CreateRequest) (*Project, error) {
	existing, err := s.store.GetByRequestKey(ctx, req.WorkspaceID, req.RequestKey)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !sameCreateRequest(existing, req, existing.ExecutorProfileID) {
		return nil, ErrRevisionConflict
	}
	if existing.MainTaskID == "" && existing.ArchivedAt == nil {
		if err := s.ensureCoordinator(ctx, existing); err != nil {
			return nil, err
		}
	}
	return existing, nil
}

func (s *Service) buildProject(ctx context.Context, req CreateRequest) (*Project, error) {
	workspace, err := s.tasks.GetWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.validateProfiles(ctx, req.WorkspaceID, req.CoordinatorProfileID, req.EconomyProfileID, req.FrontierProfileID); err != nil {
		return nil, err
	}
	repositories, err := s.validateRepositories(ctx, req.WorkspaceID, req.RepositoryIDs, req.PrimaryRepositoryID)
	if err != nil {
		return nil, err
	}
	executorID, err := s.validateWorkspaceExecutor(ctx, workspace)
	if err != nil {
		return nil, err
	}

	project := &Project{
		ID: uuid.NewString(), WorkspaceID: req.WorkspaceID, Name: req.Name,
		RepositoryIDs: repositories, PrimaryRepositoryID: req.PrimaryRepositoryID,
		CoordinatorProfileID: req.CoordinatorProfileID, EconomyProfileID: req.EconomyProfileID,
		FrontierProfileID: req.FrontierProfileID, ExecutorProfileID: executorID, Revision: 1,
	}
	return project, nil
}

func (s *Service) persistProject(ctx context.Context, project *Project, req CreateRequest) (*ProjectView, error) {
	if s.context == nil {
		return nil, ErrContextUnavailable
	}
	if _, err := s.context.Provision(ctx, project.ID); err != nil {
		return nil, err
	}
	stored, existed, err := s.store.CreateOrGet(ctx, project, req.RequestKey)
	if err != nil {
		_ = s.context.RemoveProject(project.ID)
		return nil, err
	}
	if existed && stored.ID != project.ID {
		_ = s.context.RemoveProject(project.ID)
		if _, err := s.context.Provision(ctx, stored.ID); err != nil {
			return nil, err
		}
	}
	if existed && !sameCreateRequest(stored, req, project.ExecutorProfileID) {
		return nil, ErrRevisionConflict
	}
	if err := s.ensureCoordinator(ctx, stored); err != nil {
		if s.recoverCoordinatorAssignment(ctx, stored) {
			return s.getView(ctx, stored)
		}
		return nil, err
	}
	return s.getView(ctx, stored)
}

func (s *Service) recoverCoordinatorAssignment(ctx context.Context, project *Project) bool {
	taskID, err := s.store.FindCoordinatorTaskID(ctx, project.ID)
	if err != nil || taskID == "" {
		return false
	}
	if err := s.store.SetMainTaskID(ctx, project.WorkspaceID, project.ID, taskID); err != nil {
		return false
	}
	project.MainTaskID = taskID
	return true
}

func sameCreateRequest(project *Project, req CreateRequest, executorID string) bool {
	return project.Name == req.Name && project.PrimaryRepositoryID == req.PrimaryRepositoryID &&
		project.CoordinatorProfileID == req.CoordinatorProfileID && project.EconomyProfileID == req.EconomyProfileID &&
		project.FrontierProfileID == req.FrontierProfileID && project.ExecutorProfileID == executorID &&
		slices.Equal(project.RepositoryIDs, normalizeRepositoryIDs(req.RepositoryIDs))
}

func (s *Service) ensureCoordinator(ctx context.Context, project *Project) error {
	if project.MainTaskID != "" {
		if _, err := s.tasks.GetTask(ctx, project.MainTaskID); err == nil {
			return nil
		}
	}
	if taskID, err := s.store.FindCoordinatorTaskID(ctx, project.ID); err != nil {
		return err
	} else if taskID != "" {
		if err := s.store.SetMainTaskID(ctx, project.WorkspaceID, project.ID, taskID); err != nil {
			return err
		}
		project.MainTaskID = taskID
		return nil
	}

	repositoryIDs := projectRepositoryIDsWithPrimary(project)
	taskRepositories := make([]taskservice.TaskRepositoryInput, 0, len(repositoryIDs))
	for _, repositoryID := range repositoryIDs {
		repository, err := s.tasks.GetRepository(ctx, repositoryID)
		if err != nil || repository == nil {
			return fmt.Errorf("%w: repository %s", ErrDependenciesMissing, repositoryID)
		}
		taskRepositories = append(taskRepositories, taskservice.TaskRepositoryInput{
			RepositoryID: repositoryID,
			BaseBranch:   repository.DefaultBranch,
		})
	}
	created, err := s.tasks.CreateAgentProjectTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: reqWorkspace(project), AgentProjectID: project.ID,
		AgentProjectTier: models.AgentProjectTierCoordinator, AgentProjectProfileID: project.CoordinatorProfileID,
		AssigneeAgentProfileID: project.CoordinatorProfileID,
		Title:                  project.Name, Description: "Project coordinator conversation for " + project.Name + ".",
		Repositories: taskRepositories,
	})
	if err != nil {
		return fmt.Errorf("create project coordinator: %w", err)
	}
	if created.Task == nil {
		return errors.New("create project coordinator returned no task")
	}
	if err := s.store.SetMainTaskID(ctx, project.WorkspaceID, project.ID, created.Task.ID); err != nil {
		return err
	}
	project.MainTaskID = created.Task.ID
	return nil
}

func reqWorkspace(project *Project) string { return project.WorkspaceID }

func projectRepositoryIDsWithPrimary(project *Project) []string {
	if project == nil || project.PrimaryRepositoryID == "" {
		return nil
	}
	result := make([]string, 0, len(project.RepositoryIDs))
	result = append(result, project.PrimaryRepositoryID)
	for _, id := range project.RepositoryIDs {
		if id != project.PrimaryRepositoryID {
			result = append(result, id)
		}
	}
	return result
}

func (s *Service) List(ctx context.Context, workspaceID string, archived bool) ([]ProjectView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	projects, err := s.store.List(ctx, workspaceID, archived)
	if err != nil {
		return nil, err
	}
	views := make([]ProjectView, 0, len(projects))
	for _, project := range projects {
		view, err := s.getView(ctx, project)
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func (s *Service) Get(ctx context.Context, workspaceID, projectID string) (*ProjectView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	project, err := s.store.Get(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	return s.getView(ctx, project)
}

func (s *Service) Update(ctx context.Context, workspaceID, projectID string, req UpdateRequest) (*ProjectView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	project, err := s.store.Get(ctx, workspaceID, projectID)
	if err != nil {
		return nil, err
	}
	if project.Revision != req.Revision {
		return nil, ErrRevisionConflict
	}
	if err := s.applyProjectUpdate(ctx, workspaceID, project, req); err != nil {
		return nil, err
	}
	if err := s.store.Update(ctx, project, req.Revision); err != nil {
		return nil, err
	}
	return s.Get(ctx, workspaceID, projectID)
}

func (s *Service) applyProjectUpdate(
	ctx context.Context,
	workspaceID string,
	project *Project,
	req UpdateRequest,
) error {
	if req.Name != nil {
		project.Name = strings.TrimSpace(*req.Name)
	}
	if project.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidProject)
	}
	if req.RepositoryIDs != nil {
		if err := s.updateProjectRepositories(ctx, project, req.RepositoryIDs); err != nil {
			return err
		}
	}
	if req.PrimaryRepositoryID != nil {
		project.PrimaryRepositoryID = strings.TrimSpace(*req.PrimaryRepositoryID)
	}
	if req.CoordinatorProfileID != nil {
		project.CoordinatorProfileID = strings.TrimSpace(*req.CoordinatorProfileID)
	}
	if req.EconomyProfileID != nil {
		project.EconomyProfileID = strings.TrimSpace(*req.EconomyProfileID)
	}
	if req.FrontierProfileID != nil {
		project.FrontierProfileID = strings.TrimSpace(*req.FrontierProfileID)
	}
	if req.ApplyWorkspaceDefaultExecutor {
		if err := s.applyWorkspaceDefaultExecutor(ctx, workspaceID, project); err != nil {
			return err
		}
	}
	if _, err := s.validateProfiles(ctx, workspaceID, project.CoordinatorProfileID, project.EconomyProfileID, project.FrontierProfileID); err != nil {
		return err
	}
	repositories, err := s.validateRepositories(ctx, workspaceID, project.RepositoryIDs, project.PrimaryRepositoryID)
	if err != nil {
		return err
	}
	project.RepositoryIDs = repositories
	return nil
}

func (s *Service) updateProjectRepositories(ctx context.Context, project *Project, requested []string) error {
	proposed := normalizeRepositoryIDs(requested)
	if slices.Equal(project.RepositoryIDs, proposed) {
		return nil
	}
	running, err := s.store.HasRunningTaskSessions(ctx, project.ID)
	if err != nil {
		return err
	}
	if running {
		return ErrRepositoryInUse
	}
	project.RepositoryIDs = proposed
	return nil
}

func (s *Service) applyWorkspaceDefaultExecutor(ctx context.Context, workspaceID string, project *Project) error {
	workspace, err := s.tasks.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return err
	}
	executorProfileID, err := s.validateWorkspaceExecutor(ctx, workspace)
	if err != nil {
		return err
	}
	project.ExecutorProfileID = executorProfileID
	return nil
}

func (s *Service) AuthorizeTaskCreation(ctx context.Context, req *taskservice.CreateTaskRequest) error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	if req == nil || req.AgentProjectID == "" || req.WorkflowID != "" || req.WorkflowStepID != "" || req.ProjectID != "" {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	project, err := s.store.Get(ctx, req.WorkspaceID, req.AgentProjectID)
	if err != nil || project.ArchivedAt != nil {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	if req.ParentID == "" {
		return authorizeProjectCoordinatorCreation(project, req)
	}
	return s.authorizeProjectWorkerCreation(ctx, project, req)
}

func authorizeProjectCoordinatorCreation(project *Project, req *taskservice.CreateTaskRequest) error {
	if project.MainTaskID != "" || req.AgentProjectTier != models.AgentProjectTierCoordinator ||
		req.AgentProjectProfileID != project.CoordinatorProfileID || req.AssigneeAgentProfileID != project.CoordinatorProfileID {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	return nil
}

func (s *Service) authorizeProjectWorkerCreation(
	ctx context.Context,
	project *Project,
	req *taskservice.CreateTaskRequest,
) error {
	profileID := ""
	switch req.AgentProjectTier {
	case models.AgentProjectTierEconomy:
		profileID = project.EconomyProfileID
	case models.AgentProjectTierFrontier:
		profileID = project.FrontierProfileID
	default:
		return taskservice.ErrAgentProjectTaskForbidden
	}
	if req.ParentID != project.MainTaskID || req.AgentProjectProfileID != profileID || req.AssigneeAgentProfileID != profileID {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	parent, err := s.tasks.GetTask(ctx, req.ParentID)
	if err != nil || parent.AgentProjectID != project.ID || parent.ParentID != "" {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	return nil
}

func (s *Service) AuthorizeTaskLifecycle(ctx context.Context, projectID, taskID string) error {
	if err := s.requireEnabled(); err != nil {
		return err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.AgentProjectID != projectID {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	if _, err := s.store.Get(ctx, task.WorkspaceID, projectID); err != nil {
		return taskservice.ErrAgentProjectTaskForbidden
	}
	return nil
}

func (s *Service) ResolveTaskLaunch(
	ctx context.Context,
	task *models.Task,
	requestedProfileID, requestedExecutorID, requestedExecutorProfileID string,
) (string, string, string, error) {
	if err := s.requireEnabled(); err != nil {
		return "", "", "", err
	}
	project, err := s.projectForLaunch(ctx, task)
	if err != nil {
		return "", "", "", err
	}
	configuredProfile, err := configuredProjectProfile(task, project)
	if err != nil {
		return "", "", "", err
	}
	profileID, err := requestedProjectProfile(task, configuredProfile, requestedProfileID)
	if err != nil {
		return "", "", "", err
	}
	if err := s.validateProjectLaunchProfile(ctx, task, profileID); err != nil {
		return "", "", "", err
	}
	executorID, executorProfileID, err := s.validateProjectLaunchExecutor(ctx, project)
	if err != nil {
		return "", "", "", err
	}
	if requestedExecutorProfileID != "" && requestedExecutorProfileID != project.ExecutorProfileID {
		return "", "", "", taskservice.ErrAgentProjectTaskForbidden
	}
	if requestedExecutorID != "" && requestedExecutorID != executorID {
		return "", "", "", taskservice.ErrAgentProjectTaskForbidden
	}
	return profileID, executorID, executorProfileID, nil
}

func (s *Service) projectForLaunch(ctx context.Context, task *models.Task) (*Project, error) {
	if task == nil || task.AgentProjectID == "" || task.AgentProjectTier == "" || task.AgentProjectProfileID == "" {
		return nil, taskservice.ErrAgentProjectTaskForbidden
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, task.WorkspaceID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	project, err := s.store.Get(ctx, task.WorkspaceID, task.AgentProjectID)
	if err != nil || project.ArchivedAt != nil {
		return nil, ErrLaunchBlocked
	}
	if s.context == nil {
		return nil, ErrContextUnavailable
	}
	if _, err := s.context.ContextPath(project.ID); err != nil {
		return nil, err
	}
	return project, nil
}

func configuredProjectProfile(task *models.Task, project *Project) (string, error) {
	switch task.AgentProjectTier {
	case models.AgentProjectTierCoordinator:
		if task.ID != project.MainTaskID || task.ParentID != "" {
			return "", taskservice.ErrAgentProjectTaskForbidden
		}
		return project.CoordinatorProfileID, nil
	case models.AgentProjectTierEconomy, models.AgentProjectTierFrontier:
		if task.ParentID != project.MainTaskID || task.ParentID == "" {
			return "", taskservice.ErrAgentProjectTaskForbidden
		}
		return task.AgentProjectProfileID, nil
	default:
		return "", taskservice.ErrAgentProjectTaskForbidden
	}
}

func requestedProjectProfile(task *models.Task, configured, requested string) (string, error) {
	if configured == "" || task.AgentProjectProfileID == "" {
		return "", ErrLaunchBlocked
	}
	if requested == "" || requested == configured || requested == task.AgentProjectProfileID {
		if requested != "" {
			return requested, nil
		}
		return configured, nil
	}
	return "", taskservice.ErrAgentProjectTaskForbidden
}

func (s *Service) validateProjectLaunchProfile(ctx context.Context, task *models.Task, profileID string) error {
	profile, err := s.profiles.GetAgentProfile(ctx, profileID)
	if err != nil || !agentProfileAvailableInWorkspace(profile, task.WorkspaceID) || !profile.Enabled || profile.DeletedAt != nil {
		return fmt.Errorf("%w: agent profile %s", ErrDependenciesMissing, profileID)
	}
	return nil
}

func (s *Service) validateProjectLaunchExecutor(ctx context.Context, project *Project) (string, string, error) {
	executorProfile, err := s.tasks.GetExecutorProfile(ctx, project.ExecutorProfileID)
	if err != nil || executorProfile == nil {
		return "", "", fmt.Errorf("%w: executor profile %s", ErrDependenciesMissing, project.ExecutorProfileID)
	}
	executor, err := s.tasks.GetExecutor(ctx, executorProfile.ExecutorID)
	if err != nil || executor == nil || executor.DeletedAt != nil || executor.Status != models.ExecutorStatusActive ||
		executor.Type != models.ExecutorTypeWorktree {
		return "", "", ErrExecutorIncompatible
	}
	return executor.ID, executorProfile.ID, nil
}

func (s *Service) ListContext(ctx context.Context, workspaceID, projectID, path string) ([]ContextEntry, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if s.context == nil {
		return nil, ErrContextUnavailable
	}
	if _, err := s.store.Get(ctx, workspaceID, projectID); err != nil {
		return nil, err
	}
	return s.context.List(projectID, path)
}

func (s *Service) ReadContextFile(ctx context.Context, workspaceID, projectID, path string) (string, string, error) {
	if err := s.requireEnabled(); err != nil {
		return "", "", err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return "", "", err
	}
	if s.context == nil {
		return "", "", ErrContextUnavailable
	}
	if _, err := s.store.Get(ctx, workspaceID, projectID); err != nil {
		return "", "", err
	}
	return s.context.ReadFile(projectID, path)
}

func (s *Service) WriteContextFile(ctx context.Context, workspaceID, projectID, path, expectedHash string, content []byte) (string, error) {
	if err := s.requireEnabled(); err != nil {
		return "", err
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeTaskWrite); err != nil {
		return "", err
	}
	if s.context == nil {
		return "", ErrContextUnavailable
	}
	if _, err := s.store.Get(ctx, workspaceID, projectID); err != nil {
		return "", err
	}
	return s.context.WriteFile(projectID, path, expectedHash, content)
}

func (s *Service) validateProfiles(ctx context.Context, workspaceID string, ids ...string) ([]*settingsmodels.AgentProfile, error) {
	if s.profiles == nil {
		return nil, fmt.Errorf("%w: agent profile store unavailable", ErrDependenciesMissing)
	}
	unique := make(map[string]struct{}, len(ids))
	profiles := make([]*settingsmodels.AgentProfile, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%w: all three agent profiles are required", ErrInvalidProject)
		}
		if _, ok := unique[id]; ok {
			continue
		}
		unique[id] = struct{}{}
		profile, err := s.profiles.GetAgentProfile(ctx, id)
		if err != nil || !agentProfileAvailableInWorkspace(profile, workspaceID) || !profile.Enabled || profile.DeletedAt != nil {
			return nil, fmt.Errorf("%w: agent profile %s", ErrDependenciesMissing, id)
		}
		if !s.supportsProjectWorkspaceProfile(ctx, profile) {
			return nil, fmt.Errorf("%w: agent profile %s", ErrUnsupportedProjectAgentProfile, id)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func (s *Service) supportsProjectWorkspaceProfile(ctx context.Context, profile *settingsmodels.AgentProfile) bool {
	return profile != nil && !profile.CLIPassthrough && s.projectWorkspaceProfileSupported != nil &&
		s.projectWorkspaceProfileSupported(ctx, profile.AgentID)
}

func agentProfileAvailableInWorkspace(profile *settingsmodels.AgentProfile, workspaceID string) bool {
	return profile != nil && (profile.WorkspaceID == "" || profile.WorkspaceID == workspaceID)
}

func (s *Service) validateRepositories(ctx context.Context, workspaceID string, ids []string, primaryID string) ([]string, error) {
	ids = normalizeRepositoryIDs(ids)
	primaryID = strings.TrimSpace(primaryID)
	if err := validateRepositorySelectionShape(ids, primaryID); err != nil {
		return nil, err
	}
	if primaryID == "" || !slices.Contains(ids, primaryID) {
		return nil, fmt.Errorf("%w: choose at least one repository and a primary repository from that selection", ErrInvalidProject)
	}
	for _, id := range ids {
		repository, err := s.tasks.GetRepository(ctx, id)
		if err != nil || repository == nil || repository.WorkspaceID != workspaceID || !remoteBackedProjectRepository(repository) {
			return nil, fmt.Errorf("%w: repository %s is unavailable or not a configured remote repository", ErrDependenciesMissing, id)
		}
	}
	return ids, nil
}

func validateRepositorySelectionShape(ids []string, primaryID string) error {
	if len(ids) == 0 || len(ids) != len(normalizeRepositoryIDs(ids)) || strings.TrimSpace(primaryID) == "" || !slices.Contains(normalizeRepositoryIDs(ids), strings.TrimSpace(primaryID)) {
		return fmt.Errorf("%w: choose at least one repository and a primary repository from that selection", ErrInvalidProject)
	}
	return nil
}

func normalizeRepositoryIDs(ids []string) []string {
	result := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (s *Service) validateWorkspaceExecutor(ctx context.Context, workspace *models.Workspace) (string, error) {
	if workspace == nil || workspace.DefaultExecutorID == nil || strings.TrimSpace(*workspace.DefaultExecutorID) == "" {
		return "", fmt.Errorf("%w: choose a local worktree executor as the workspace default", ErrExecutorIncompatible)
	}
	executor, err := s.tasks.GetExecutor(ctx, strings.TrimSpace(*workspace.DefaultExecutorID))
	if err != nil || executor == nil || executor.Status != models.ExecutorStatusActive || executor.DeletedAt != nil {
		return "", fmt.Errorf("%w: workspace default executor is unavailable", ErrDependenciesMissing)
	}
	if executor.Type != models.ExecutorTypeWorktree {
		return "", ErrExecutorIncompatible
	}
	profiles, err := s.tasks.ListExecutorProfiles(ctx, executor.ID)
	if err != nil || len(profiles) == 0 || profiles[0] == nil {
		return "", fmt.Errorf("%w: workspace default executor has no available profile", ErrDependenciesMissing)
	}
	profile := profiles[0]
	return profile.ID, nil
}

func (s *Service) getView(ctx context.Context, project *Project) (*ProjectView, error) {
	tasks, err := s.store.ListTasks(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	return &ProjectView{Project: *project, Tasks: tasks}, nil
}

func (s *Service) requireEnabled() error {
	if s == nil || !s.enabled {
		return ErrDisabled
	}
	if s.store == nil || s.tasks == nil {
		return errors.New("agent project service is unavailable")
	}
	return nil
}
