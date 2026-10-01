package projects

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const maxActiveProjectWorkers = 8

var (
	ErrWorkerCapacity           = errors.New("agent project has reached its active worker limit")
	ErrInvalidWorkerRequest     = errors.New("invalid agent project worker request")
	ErrCoordinatorRequired      = errors.New("project coordinator session is required")
	ErrWorkerStarterUnavailable = errors.New("project worker launcher is unavailable")
)

type projectWorkerStarter interface {
	StartAgentProjectWorker(context.Context, string, string, string, string) error
}

type projectBranchLister interface {
	ListBranches(context.Context, string, string) ([]taskservice.Branch, error)
}

type projectWorkerSessionReader interface {
	ListTaskSessions(context.Context, string) ([]*models.TaskSession, error)
	GetLastAgentMessage(context.Context, string) (string, error)
}

type projectWorkerChangeRequestReader interface {
	ListWorkerChangeRequests(context.Context, string) ([]WorkerChangeRequest, error)
}

// SetWorkerStarter wires the server-owned launch path used after worker
// admission. Agents cannot supply profile or executor identifiers.
func (s *Service) SetWorkerStarter(starter projectWorkerStarter) {
	s.workerStarter = starter
}

// SetProjectWorkspaceProfileSupportChecker wires the runtime registry's
// capability check for profiles that can receive project workspace roots.
func (s *Service) SetProjectWorkspaceProfileSupportChecker(checker func(context.Context, string) bool) {
	s.projectWorkspaceProfileSupported = checker
}

func (s *Service) SetWorkerChangeRequestReader(reader projectWorkerChangeRequestReader) {
	s.workerChangeRequestReader = reader
}

func (s *Service) GetCoordinatorProject(ctx context.Context, coordinatorTaskID string) (*ProjectView, error) {
	_, project, err := s.resolveCoordinator(ctx, coordinatorTaskID)
	if err != nil {
		return nil, err
	}
	return s.getView(ctx, project)
}

func (s *Service) ListWorkers(ctx context.Context, coordinatorTaskID string) ([]WorkerView, error) {
	coordinator, project, err := s.resolveCoordinator(ctx, coordinatorTaskID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.store.ListTasks(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	workers := make([]WorkerView, 0, len(tasks))
	for _, task := range tasks {
		if task.ParentID != coordinator.ID {
			continue
		}
		worker, err := s.workerView(ctx, project.ID, task)
		if err != nil {
			return nil, err
		}
		workers = append(workers, worker)
	}
	return workers, nil
}

func (s *Service) GetWorker(ctx context.Context, coordinatorTaskID, workerTaskID string) (*WorkerView, error) {
	coordinator, project, err := s.resolveCoordinator(ctx, coordinatorTaskID)
	if err != nil {
		return nil, err
	}
	return s.getWorkerByParent(ctx, project, coordinator, workerTaskID)
}

func (s *Service) GetCurrentProjectTask(ctx context.Context, taskID string) (*WorkerView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.AgentProjectID == "" || task.AgentProjectTier == models.AgentProjectTierCoordinator || task.ParentID == "" {
		return nil, ErrInvalidProject
	}
	project, err := s.store.Get(ctx, task.WorkspaceID, task.AgentProjectID)
	if err != nil || project.ArchivedAt != nil || project.MainTaskID != task.ParentID {
		return nil, ErrInvalidProject
	}
	worker, err := s.workerView(ctx, project.ID, TaskSummary{
		ID: task.ID, Title: task.Title, State: string(task.State), ParentID: task.ParentID,
		ArchivedAt: task.ArchivedAt, UpdatedAt: task.UpdatedAt,
	})
	if err != nil {
		return nil, err
	}
	return &worker, nil
}

// CreateWorker validates the session-bound coordinator, repository selection,
// remote base refs, and active-worker limit before creating or launching a
// direct child task.
func (s *Service) CreateWorker(ctx context.Context, coordinatorTaskID string, req CreateWorkerRequest) (*WorkerView, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, err
	}
	if s.workerStarter == nil {
		return nil, ErrWorkerStarterUnavailable
	}
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	coordinator, project, err := s.resolveCoordinator(ctx, coordinatorTaskID)
	if err != nil {
		return nil, err
	}
	if project.ArchivedAt != nil {
		return nil, ErrInvalidProject
	}
	req.Tier = strings.TrimSpace(req.Tier)
	req.Title = strings.TrimSpace(req.Title)
	req.Prompt = strings.TrimSpace(req.Prompt)
	profileID, err := s.validateWorkerAdmission(ctx, coordinator, project, req)
	if err != nil {
		return nil, err
	}
	repositories, err := s.resolveWorkerRepositories(ctx, project, req.Repositories)
	if err != nil {
		return nil, err
	}
	return s.createWorkerTask(ctx, coordinator, project, req, profileID, repositories)
}

func (s *Service) validateWorkerAdmission(
	ctx context.Context,
	coordinator *models.Task,
	project *Project,
	req CreateWorkerRequest,
) (string, error) {
	if (req.Tier != models.AgentProjectTierEconomy && req.Tier != models.AgentProjectTierFrontier) || req.Title == "" || req.Prompt == "" {
		return "", fmt.Errorf("%w: tier, title, and prompt are required", ErrInvalidWorkerRequest)
	}
	profileID := project.EconomyProfileID
	if req.Tier == models.AgentProjectTierFrontier {
		profileID = project.FrontierProfileID
	}
	profile, err := s.profiles.GetAgentProfile(ctx, profileID)
	if err != nil || !agentProfileAvailableInWorkspace(profile, project.WorkspaceID) || !profile.Enabled || profile.DeletedAt != nil {
		return "", fmt.Errorf("%w: agent profile %s", ErrDependenciesMissing, profileID)
	}
	if !s.supportsProjectWorkspaceProfile(ctx, profile) {
		return "", fmt.Errorf("%w: agent profile %s", ErrUnsupportedProjectAgentProfile, profileID)
	}
	active, err := s.store.CountActiveWorkers(ctx, project.ID, coordinator.ID)
	if err != nil {
		return "", fmt.Errorf("count active project workers: %w", err)
	}
	if active >= maxActiveProjectWorkers {
		return "", ErrWorkerCapacity
	}
	return profileID, nil
}

func (s *Service) createWorkerTask(
	ctx context.Context,
	coordinator *models.Task,
	project *Project,
	req CreateWorkerRequest,
	profileID string,
	repositories []taskservice.TaskRepositoryInput,
) (*WorkerView, error) {
	created, err := s.tasks.CreateAgentProjectTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: project.WorkspaceID, AgentProjectID: project.ID,
		AgentProjectTier: req.Tier, AgentProjectProfileID: profileID,
		WorkflowID: "", WorkflowStepID: "", ParentID: coordinator.ID,
		Title: req.Title, Description: req.Prompt, Autopilot: true, StartAgent: true,
		AssigneeAgentProfileID: profileID, Repositories: repositories,
		WorkspacePolicy: &taskservice.WorkspacePolicy{Mode: "new_workspace"},
	})
	if err != nil {
		return nil, fmt.Errorf("create project worker: %w", err)
	}
	if created.Task == nil {
		return nil, errors.New("create project worker returned no task")
	}
	if err := s.workerStarter.StartAgentProjectWorker(ctx, created.Task.ID, profileID, project.ExecutorProfileID, req.Prompt); err != nil {
		return nil, fmt.Errorf("start project worker %s: %w", created.Task.ID, err)
	}
	return s.GetWorker(ctx, coordinator.ID, created.Task.ID)
}

func (s *Service) resolveCoordinator(ctx context.Context, taskID string) (*models.Task, *Project, error) {
	if err := s.requireEnabled(); err != nil {
		return nil, nil, err
	}
	if taskID == "" {
		return nil, nil, ErrCoordinatorRequired
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.AgentProjectID == "" || task.AgentProjectTier != models.AgentProjectTierCoordinator || task.ParentID != "" {
		return nil, nil, ErrCoordinatorRequired
	}
	if err := s.tasks.AuthorizeWorkspaceScope(ctx, task.WorkspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, nil, err
	}
	project, err := s.store.Get(ctx, task.WorkspaceID, task.AgentProjectID)
	if err != nil || project.MainTaskID != task.ID {
		return nil, nil, ErrCoordinatorRequired
	}
	return task, project, nil
}

func (s *Service) getWorkerByParent(
	ctx context.Context, project *Project, coordinator *models.Task, workerTaskID string,
) (*WorkerView, error) {
	task, err := s.tasks.GetTask(ctx, workerTaskID)
	if err != nil || task == nil || task.AgentProjectID != project.ID || task.ParentID != coordinator.ID || task.AgentProjectTier == models.AgentProjectTierCoordinator {
		return nil, ErrInvalidProject
	}
	worker, err := s.workerView(ctx, project.ID, TaskSummary{
		ID: task.ID, Title: task.Title, State: string(task.State), ParentID: task.ParentID,
		ArchivedAt: task.ArchivedAt, UpdatedAt: task.UpdatedAt,
	})
	if err != nil {
		return nil, err
	}
	return &worker, nil
}

func (s *Service) resolveWorkerRepositories(
	ctx context.Context, project *Project, requested []WorkerRepositoryInput,
) ([]taskservice.TaskRepositoryInput, error) {
	selected := selectedWorkerRepositories(project, requested)
	if len(selected) == 0 {
		return nil, fmt.Errorf("%w: project has no repositories", ErrInvalidWorkerRequest)
	}
	allowed := make(map[string]struct{}, len(project.RepositoryIDs))
	for _, id := range project.RepositoryIDs {
		allowed[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(selected))
	branchLister, ok := s.tasks.(projectBranchLister)
	if !ok {
		return nil, ErrDependenciesMissing
	}
	resolved := make([]taskservice.TaskRepositoryInput, 0, len(selected))
	for _, item := range selected {
		repository, err := s.resolveWorkerRepository(ctx, project, item, allowed, seen, branchLister)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, repository)
	}
	return resolved, nil
}

func selectedWorkerRepositories(project *Project, requested []WorkerRepositoryInput) []WorkerRepositoryInput {
	if len(requested) > 0 {
		return requested
	}
	ids := projectRepositoryIDsWithPrimary(project)
	selected := make([]WorkerRepositoryInput, 0, len(ids))
	for _, id := range ids {
		selected = append(selected, WorkerRepositoryInput{RepositoryID: id})
	}
	return selected
}

func (s *Service) resolveWorkerRepository(
	ctx context.Context,
	project *Project,
	item WorkerRepositoryInput,
	allowed, seen map[string]struct{},
	branchLister projectBranchLister,
) (taskservice.TaskRepositoryInput, error) {
	id := strings.TrimSpace(item.RepositoryID)
	if _, exists := allowed[id]; !exists || id == "" {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("%w: repository is not selected on this project", ErrInvalidWorkerRequest)
	}
	if _, exists := seen[id]; exists {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("%w: duplicate repository", ErrInvalidWorkerRequest)
	}
	seen[id] = struct{}{}
	repository, err := s.tasks.GetRepository(ctx, id)
	if err != nil || repository == nil || repository.WorkspaceID != project.WorkspaceID || !remoteBackedProjectRepository(repository) {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("%w: repository %s", ErrDependenciesMissing, id)
	}
	baseBranch := strings.TrimSpace(item.BaseBranch)
	if baseBranch == "" {
		baseBranch = repository.DefaultBranch
	}
	if !securityutil.IsValidBranchName(baseBranch) {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("%w: invalid base branch for repository %s", ErrInvalidWorkerRequest, id)
	}
	branches, err := branchLister.ListBranches(ctx, id, "")
	if err != nil {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("list remote branches for repository %s: %w", id, err)
	}
	if !slices.ContainsFunc(branches, func(branch taskservice.Branch) bool {
		return branch.Name == baseBranch && branch.Type == "remote"
	}) {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("%w: remote branch %q is unavailable for repository %s", ErrInvalidWorkerRequest, baseBranch, id)
	}
	return taskservice.TaskRepositoryInput{RepositoryID: id, BaseBranch: baseBranch}, nil
}

func (s *Service) workerView(ctx context.Context, projectID string, summary TaskSummary) (WorkerView, error) {
	task, err := s.tasks.GetTask(ctx, summary.ID)
	if err != nil || task == nil || task.AgentProjectID != projectID {
		return WorkerView{}, ErrInvalidProject
	}
	view := WorkerView{
		TaskSummary: summary,
		Tier:        task.AgentProjectTier, ProfileID: task.AgentProjectProfileID,
		Repositories: s.workerRepositories(ctx, task.Repositories),
	}
	if err := s.populateWorkerSession(ctx, task.ID, &view); err != nil {
		return WorkerView{}, err
	}
	view.ChangeRequests = s.workerChangeRequests(ctx, task.ID)
	return view, nil
}

func (s *Service) workerRepositories(ctx context.Context, repositories []*models.TaskRepository) []WorkerRepository {
	result := make([]WorkerRepository, 0, len(repositories))
	for _, repository := range repositories {
		if repository == nil {
			continue
		}
		name := repository.RepositoryID
		if resolved, err := s.tasks.GetRepository(ctx, repository.RepositoryID); err == nil && resolved != nil {
			name = resolved.Name
		}
		result = append(result, WorkerRepository{
			RepositoryID: repository.RepositoryID, Name: name,
			BaseBranch: repository.BaseBranch, CheckoutBranch: repository.CheckoutBranch,
		})
	}
	return result
}

func (s *Service) populateWorkerSession(ctx context.Context, taskID string, view *WorkerView) error {
	if reader, ok := s.tasks.(projectWorkerSessionReader); ok {
		sessions, err := reader.ListTaskSessions(ctx, taskID)
		if err != nil {
			return err
		}
		var latest *models.TaskSession
		for _, session := range sessions {
			if session != nil && (latest == nil || session.UpdatedAt.After(latest.UpdatedAt)) {
				latest = session
			}
		}
		if latest != nil {
			view.SessionState = string(latest.State)
			view.SessionError = latest.ErrorMessage
			view.LatestResult, _ = reader.GetLastAgentMessage(ctx, latest.ID)
			view.LatestResult = boundResult(view.LatestResult, 4000)
		}
	}
	return nil
}

func (s *Service) workerChangeRequests(ctx context.Context, taskID string) []WorkerChangeRequest {
	if s.workerChangeRequestReader == nil {
		return nil
	}
	changeRequests, err := s.workerChangeRequestReader.ListWorkerChangeRequests(ctx, taskID)
	if err != nil {
		return nil
	}
	return boundWorkerChangeRequests(changeRequests)
}

func boundWorkerChangeRequests(changeRequests []WorkerChangeRequest) []WorkerChangeRequest {
	if len(changeRequests) > MaxWorkerChangeRequests {
		changeRequests = changeRequests[:MaxWorkerChangeRequests]
	}
	bounded := make([]WorkerChangeRequest, 0, len(changeRequests))
	for _, change := range changeRequests {
		change.Provider = boundResult(change.Provider, 40)
		change.URL = boundResult(change.URL, 2048)
		change.Title = boundResult(change.Title, 500)
		change.State = boundResult(change.State, 40)
		bounded = append(bounded, change)
	}
	return bounded
}

func boundResult(value string, maxRunes int) string {
	if len([]rune(value)) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes]) + "…"
}
