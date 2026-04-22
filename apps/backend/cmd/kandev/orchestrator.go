package main

import (
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/secrets"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

const defaultEventNamespace = "default"

func provideOrchestrator(
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	taskRepo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	userSvc *userservice.Service,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	workflowSvc *workflowservice.Service,
	secretStore secrets.SecretStore,
	repoCloner *repoclone.Cloner,
) (*orchestrator.Service, *messageCreatorAdapter, error) {
	if lifecycleMgr == nil {
		return nil, nil, errors.New("lifecycle manager is required: configure agent runtime (docker or standalone)")
	}

	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}
	agentManagerClient := newLifecycleAdapter(lifecycleMgr, agentRegistry, log)

	serviceCfg := orchestrator.DefaultServiceConfig()
	namespace := resolveEventNamespace(cfg)
	serviceCfg.QueueGroup = "orchestrator." + namespace
	busMode := "memory"
	if cfg != nil && strings.TrimSpace(cfg.NATS.URL) != "" {
		busMode = "nats"
	}
	log.Debug("orchestrator queue group resolved",
		zap.String("event_bus", busMode),
		zap.String("event_namespace", namespace),
		zap.String("queue_group", serviceCfg.QueueGroup),
		zap.Int("agent_standalone_port", cfg.Agent.StandalonePort))
	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, agentManagerClient, taskRepoAdapter, taskRepo, userSvc, secretStore, log)
	taskSvc.SetExecutionStopper(orchestratorSvc)
	taskSvc.SetGitArchiveCapture(orchestratorSvc)

	msgCreator := &messageCreatorAdapter{svc: taskSvc, logger: log}
	orchestratorSvc.SetMessageCreator(msgCreator)

	orchestratorSvc.SetTurnService(newTurnServiceAdapter(taskSvc))

	// Route orchestrator task.updated events through the task service, which
	// owns the canonical rich payload. Covers workflow transitions, workflow
	// step moves, and the primary-session-set callback below.
	orchestratorSvc.SetTaskEventPublisher(taskSvc)

	// Publish task.updated when the first session is marked primary so the
	// frontend receives primary_session_id for newly created tasks.
	orchestratorSvc.SetOnPrimarySessionSet(func(ctx context.Context, taskID, _ string) {
		task, err := taskRepo.GetTask(ctx, taskID)
		if err != nil {
			log.Warn("failed to get task for primary session event",
				zap.String("task_id", taskID),
				zap.Error(err))
			return
		}
		taskSvc.PublishTaskUpdated(ctx, task)
	})

	// Wire workflow step getter for prompt building
	if workflowSvc != nil {
		orchestratorSvc.SetWorkflowStepGetter(&orchestratorWorkflowStepGetterAdapter{svc: workflowSvc})
	}

	// Wire review task creator for auto-creating tasks from review watch PRs
	orchestratorSvc.SetReviewTaskCreator(&reviewTaskCreatorAdapter{svc: taskSvc})

	// Wire issue task creator for auto-creating tasks from issue watch events
	orchestratorSvc.SetIssueTaskCreator(&issueTaskCreatorAdapter{svc: taskSvc})

	// Wire repository resolver for auto-cloning repos during review task creation
	if repoCloner != nil {
		orchestratorSvc.SetRepositoryResolver(&repositoryResolverAdapter{
			cloner:   repoCloner,
			protocol: repoclone.DetectGitProtocol(),
			taskSvc:  taskSvc,
			logger:   log,
		})

		// Wire repo cloner into executor for provider-backed repos with no local path
		orchestratorSvc.SetRepoCloner(repoCloner, &repoLocalPathUpdater{svc: taskSvc})
	}

	return orchestratorSvc, msgCreator, nil
}

func resolveEventNamespace(cfg *config.Config) string {
	if cfg == nil {
		return defaultEventNamespace
	}
	if explicit := strings.TrimSpace(cfg.Events.Namespace); explicit != "" {
		return sanitizeNamespace(explicit)
	}
	identity := resolveDatabaseIdentity(cfg)
	if identity == "" {
		return defaultEventNamespace
	}
	return hashNamespace(identity)
}

func resolveDatabaseIdentity(cfg *config.Config) string {
	if strings.EqualFold(cfg.Database.Driver, "sqlite") {
		path := cfg.Database.Path
		if path == "" {
			path = "./kandev.db"
		}
		absPath, err := filepath.Abs(path)
		if err == nil {
			return "sqlite:" + absPath
		}
		return "sqlite:" + path
	}
	return fmt.Sprintf("pg:%s:%d:%s:%s", cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName, cfg.Database.User)
}

func hashNamespace(identity string) string {
	sum := sha1.Sum([]byte(identity))
	return fmt.Sprintf("%x", sum[:6])
}

func sanitizeNamespace(namespace string) string {
	lower := strings.ToLower(namespace)
	var b strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-._")
	if out == "" {
		return defaultEventNamespace
	}
	return out
}

// orchestratorWorkflowStepGetterAdapter adapts workflow service to orchestrator's WorkflowStepGetter interface.
// Since orchestrator now uses wfmodels.WorkflowStep directly, the adapter simply delegates to the service.
type orchestratorWorkflowStepGetterAdapter struct {
	svc *workflowservice.Service
}

// GetStep implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetStep(ctx, stepID)
}

// GetNextStepByPosition implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetNextStepByPosition(ctx, workflowID, currentPosition)
}

// GetPreviousStepByPosition implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetPreviousStepByPosition(ctx, workflowID, currentPosition)
}

// GetWorkflowAgentProfileID implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetWorkflowAgentProfileID(ctx context.Context, workflowID string) (string, error) {
	return a.svc.GetWorkflowAgentProfileID(ctx, workflowID)
}

// reviewTaskCreatorAdapter adapts the task service to the orchestrator's ReviewTaskCreator interface.
type reviewTaskCreatorAdapter struct {
	svc *taskservice.Service
}

// CreateReviewTask implements orchestrator.ReviewTaskCreator.
func (a *reviewTaskCreatorAdapter) CreateReviewTask(ctx context.Context, req *orchestrator.ReviewTaskRequest) (*taskmodels.Task, error) {
	var repos []taskservice.TaskRepositoryInput
	for _, r := range req.Repositories {
		repos = append(repos, taskservice.TaskRepositoryInput{
			RepositoryID:   r.RepositoryID,
			BaseBranch:     r.BaseBranch,
			CheckoutBranch: r.CheckoutBranch,
		})
	}
	return a.svc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Metadata:       req.Metadata,
		Repositories:   repos,
	})
}

// issueTaskCreatorAdapter adapts the task service to the orchestrator's IssueTaskCreator interface.
type issueTaskCreatorAdapter struct {
	svc *taskservice.Service
}

// CreateIssueTask implements orchestrator.IssueTaskCreator.
func (a *issueTaskCreatorAdapter) CreateIssueTask(ctx context.Context, req *orchestrator.IssueTaskRequest) (*taskmodels.Task, error) {
	var repos []taskservice.TaskRepositoryInput
	for _, r := range req.Repositories {
		repos = append(repos, taskservice.TaskRepositoryInput{
			RepositoryID: r.RepositoryID,
			BaseBranch:   r.BaseBranch,
		})
	}
	return a.svc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Metadata:       req.Metadata,
		Repositories:   repos,
	})
}

// repoLocalPathUpdater adapts the task service's UpdateRepository to the executor.RepoUpdater interface.
type repoLocalPathUpdater struct {
	svc *taskservice.Service
}

func (u *repoLocalPathUpdater) UpdateRepositoryLocalPath(ctx context.Context, repositoryID, localPath string) error {
	if repositoryID == "" || localPath == "" {
		return fmt.Errorf("UpdateRepositoryLocalPath: repositoryID and localPath must be non-empty")
	}
	_, err := u.svc.UpdateRepository(ctx, repositoryID, &taskservice.UpdateRepositoryRequest{
		LocalPath: &localPath,
	})
	return err
}

// repositoryResolverAdapter resolves GitHub repos by cloning + finding/creating DB records.
type repositoryResolverAdapter struct {
	cloner   *repoclone.Cloner
	protocol string
	taskSvc  *taskservice.Service
	logger   *logger.Logger
}

// ResolveForReview implements orchestrator.RepositoryResolver.
func (a *repositoryResolverAdapter) ResolveForReview(
	ctx context.Context, workspaceID, provider, owner, name, defaultBranch string,
) (string, string, error) {
	cloneURL, err := repoclone.CloneURL(provider, owner, name, a.protocol)
	if err != nil {
		return "", "", fmt.Errorf("unsupported provider: %w", err)
	}

	localPath, err := a.cloner.EnsureCloned(ctx, cloneURL, owner, name)
	if err != nil {
		return "", "", fmt.Errorf("clone repository: %w", err)
	}

	repo, err := a.taskSvc.FindOrCreateRepository(ctx, &taskservice.FindOrCreateRepositoryRequest{
		WorkspaceID:   workspaceID,
		Provider:      provider,
		ProviderOwner: owner,
		ProviderName:  name,
		DefaultBranch: defaultBranch,
		LocalPath:     localPath,
	})
	if err != nil {
		return "", "", fmt.Errorf("find/create repository: %w", err)
	}

	baseBranch := defaultBranch
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}
	// When no default branch is known (e.g. issue watch with no PR context),
	// detect it from the cloned repo's HEAD.
	if baseBranch == "" && localPath != "" {
		baseBranch = a.detectAndPersistDefaultBranch(ctx, repo, localPath)
	}
	return repo.ID, baseBranch, nil
}

// detectAndPersistDefaultBranch reads the default branch from the local clone
// and persists it to the repository record for future lookups.
func (a *repositoryResolverAdapter) detectAndPersistDefaultBranch(
	ctx context.Context, repo *taskmodels.Repository, localPath string,
) string {
	detected := detectGitDefaultBranch(localPath)
	if detected == "" {
		return ""
	}
	if _, err := a.taskSvc.UpdateRepository(ctx, repo.ID, &taskservice.UpdateRepositoryRequest{
		DefaultBranch: &detected,
	}); err != nil {
		a.logger.Warn("failed to persist detected default branch",
			zap.String("repository_id", repo.ID),
			zap.String("branch", detected),
			zap.Error(err))
	}
	return detected
}

// detectGitDefaultBranch reads the default branch of a git repository.
// It first checks refs/remotes/origin/HEAD (set by `git clone`), then
// falls back to .git/HEAD. Returns empty string on any failure.
func detectGitDefaultBranch(repoPath string) string {
	// Prefer the remote default branch pointer set by `git clone`.
	originHead := filepath.Join(repoPath, ".git", "refs", "remotes", "origin", "HEAD")
	if content, err := os.ReadFile(originHead); err == nil {
		trimmed := strings.TrimSpace(string(content))
		if after, ok := strings.CutPrefix(trimmed, "ref: refs/remotes/origin/"); ok {
			return after
		}
	}
	// Fall back to the local HEAD (works for fresh clones that lack origin/HEAD).
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	content, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(content))
	if after, ok := strings.CutPrefix(trimmed, "ref: refs/heads/"); ok {
		return after
	}
	return ""
}
