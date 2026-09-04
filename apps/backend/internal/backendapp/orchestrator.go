package backendapp

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	automationpkg "github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/gitref"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/delivery"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitcredentials"
	githubpkg "github.com/kandev/kandev/internal/github"
	jirapkg "github.com/kandev/kandev/internal/jira"
	linearpkg "github.com/kandev/kandev/internal/linear"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator"
	executorpkg "github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/secrets"
	sentrypkg "github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/system/queuesettings"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

const (
	defaultMainBranch   = "main"
	defaultMasterBranch = "master"
)

const defaultEventNamespace = "default"

func provideOrchestrator(
	cfg *config.Config,
	log *logger.Logger,
	pool *db.Pool,
	eventBus bus.EventBus,
	taskRepo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	userSvc *userservice.Service,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	workflowSvc *workflowservice.Service,
	secretStore secrets.SecretStore,
	repoCloner *repoclone.Cloner,
	promptSvc *promptservice.Service,
	githubSvc *githubpkg.Service,
	gitCredentialBroker *gitcredentials.Broker,
) (*orchestrator.Service, *messageCreatorAdapter, error) {
	if lifecycleMgr == nil {
		return nil, nil, errors.New("lifecycle manager is required: configure agent runtime (docker or standalone)")
	}

	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}
	agentManagerClient := newLifecycleAdapter(lifecycleMgr, agentRegistry, log)

	serviceCfg := orchestrator.DefaultServiceConfig()
	serviceCfg.ClaudeBackgroundPromptHandoff =
		cfg != nil && cfg.Features.ClaudeBackgroundPromptHandoff
	serviceCfg.ClaudeMidTurnSteering =
		cfg != nil && cfg.Features.ClaudeMidTurnSteering
	serviceCfg.OfficeSessionIdentity =
		cfg != nil && cfg.Features.OfficeSessionIdentity
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

	queueRepo, err := messagequeue.NewSQLiteRepository(pool.Writer(), pool.Reader())
	if err != nil {
		return nil, nil, fmt.Errorf("init message queue repo: %w", err)
	}
	queueSettings := resolveQueueSettings(pool, log, queueConfiguration(cfg)).Effective
	maxPerSession := queueSettings.MaxPerSession
	mergeEnabled := queueSettings.MergeEnabled
	autoMergeEnabled := queueSettings.AutoMergeEnabled
	msgQueue := messagequeue.NewService(queueRepo, maxPerSession, log)
	msgQueue.SetMergeEnabled(mergeEnabled)
	msgQueue.SetAutoMergeEnabled(autoMergeEnabled)
	log.Info("Message queue initialized",
		zap.Int("max_per_session", maxPerSession),
		zap.Bool("merge_enabled", mergeEnabled),
		zap.Bool("auto_merge_enabled", autoMergeEnabled))
	if taskSvc.AttachmentService() == nil && taskSvc.AttachmentRepository() != nil {
		attachmentSvc, attachmentErr := taskservice.NewAttachmentService(
			taskSvc.AttachmentRepository(), cfg.ResolvedHomeDir(), taskSvc.AuthorizeWorkspaceAccess, log,
		)
		if attachmentErr != nil {
			return nil, nil, fmt.Errorf("initialize prompt attachment storage: %w", attachmentErr)
		}
		taskSvc.SetAttachmentService(attachmentSvc)
	}
	if attachmentSvc := taskSvc.AttachmentService(); attachmentSvc != nil {
		attachmentSvc.SetTaskAuthorizer(taskSvc.AuthorizeTaskAccess)
	}

	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, agentManagerClient, taskRepoAdapter, taskRepo, userSvc, secretStore, msgQueue, log)
	orchestratorSvc.SetAgentProfileRecentUseRecorder(userSvc)
	if gitCredentialBroker != nil {
		orchestratorSvc.SetGitHubCredentialBroker(gitCredentialBroker, githubCredentialBrokerEndpoint(cfg))
	}
	orchestratorSvc.SetAttachmentReader(taskSvc.AttachmentService())
	orchestratorSvc.SetLaunchAttachmentClaimer(taskSvc)
	orchestratorSvc.SetTitleBranchRuntime(lifecycleMgr)
	if githubSvc != nil {
		orchestratorSvc.SetTaskGitCredentialPolicyResolver(githubExecutorCredentialPolicyAdapter{service: githubSvc})
		orchestratorSvc.SetPRBaseResolver(githubPRBaseResolver{service: githubSvc})
	}
	taskSvc.SetExecutionStopper(orchestratorSvc)
	// Runtime-aware liveness lets durable cleanup treat a not-found stop for a
	// confirmed-dead local runtime as already stopped instead of retrying forever.
	taskSvc.SetRowLivenessProber(agentManagerClient)
	taskSvc.SetContextWindowResetter(orchestratorSvc.ResetContextWindow)
	taskSvc.SetGitArchiveCapture(orchestratorSvc)
	// Automation runs keep their worktrees so they stay repliable, which makes
	// them the one task kind nothing else ever cleans up; the orchestrator needs
	// the manager to enforce the per-automation retention window.
	orchestratorSvc.SetWorktreeManager(lifecycleMgr.WorktreeManager())
	orchestratorSvc.SetTaskLaunchRecoveryService(taskSvc)

	msgCreator := &messageCreatorAdapter{svc: taskSvc, logger: log}
	orchestratorSvc.SetMessageCreator(msgCreator)
	orchestratorSvc.SetTransientRetryMessageService(taskSvc)
	orchestratorSvc.SetSubagentContextRecorder(&subagentContextAdapter{svc: taskSvc})

	orchestratorSvc.SetTurnService(newTurnServiceAdapter(taskSvc))

	// Route orchestrator task.updated events through the task service, which
	// owns the canonical rich payload. Covers workflow transitions, workflow
	// step moves, and the primary-session-set callback below.
	orchestratorSvc.SetTaskEventPublisher(taskSvc)
	// Feeder promotion after an admitted manual move must wait for the
	// orchestrator's task lifecycle. The task service keeps ownership of the
	// candidate filter and promotion rules.
	orchestratorSvc.SetFeederPullReconciler(taskSvc)

	// Let the task service read the live per-session busy substate so it can
	// compute the task-level MOST-ACTIVE-WINS activity aggregate carried on the
	// boot payload and task.updated events.
	taskSvc.SetForegroundActivityProvider(orchestratorSvc)

	// Task dependencies gate every automated launch and drive chain advancement.
	// Wired unconditionally: dependencies are a core Kanban relationship, not an
	// Office feature.
	orchestratorSvc.SetTaskDependencyReader(taskSvc)

	// Let the task service stamp status_summary.queued_prompt_count on task
	// list/snapshot payloads (initial-load backstop for the sidebar badge; the
	// status-summary projector keeps the field live between loads).
	taskSvc.SetQueuedPromptCounter(orchestratorSvc.GetMessageQueue())

	// Per-user scoping for the session-keyed WS actions. The orchestrator
	// resolves sessions through its own repo handle, so it does not inherit the
	// task service's authorize* checks.
	orchestratorSvc.SetSessionAccessChecker(taskSvc.AuthorizeSessionAccess)
	orchestratorSvc.SetTaskAccessChecker(taskSvc.AuthorizeTaskAccess)

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

	// Wire workflow step getter for prompt building. The recorder is wired
	// first: SetStepHistoryRecorder's own initWorkflowEngine() call is a
	// no-op while workflowStepGetter is still nil, so the store/engine only
	// get built once, by SetWorkflowStepGetter below, instead of twice.
	if workflowSvc != nil {
		// Wire the ADR 0015 audit-trail writer for auto-advance step
		// transitions. workflowSvc.CreateStepTransition already matches
		// orchestrator.StepHistoryRecorder structurally, so no adapter is
		// needed.
		orchestratorSvc.SetStepHistoryRecorder(workflowSvc)
		orchestratorSvc.SetWorkflowStepGetter(&orchestratorWorkflowStepGetterAdapter{svc: workflowSvc})
	}

	// Wire agent family resolution so configure_session rules can name an agent
	// the way a workflow author writes it ("Claude") and still match the
	// canonical ID a session stores ("claude-acp").
	if agentRegistry != nil {
		orchestratorSvc.SetAgentFamilyResolver(agentRegistry)
	}

	// Wire "@name" saved-prompt reference expansion into workflow-step prompt
	// building.
	if promptSvc != nil {
		orchestratorSvc.SetPromptReferenceExpander(promptSvc)
	}

	// Wire review task creator for auto-creating tasks from review watch PRs
	orchestratorSvc.SetReviewTaskCreator(&reviewTaskCreatorAdapter{svc: taskSvc})

	// Wire issue task creator for auto-creating tasks from issue watch events
	orchestratorSvc.SetIssueTaskCreator(&issueTaskCreatorAdapter{svc: taskSvc})

	// Wire the per-watcher throttle gate. taskRepo exposes the JSON-scoped
	// count of open watcher-created tasks; the orchestrator combines it with
	// an in-process pending counter so a poll-tick burst can't overshoot.
	orchestratorSvc.SetWatcherTaskCounter(taskRepo)

	// Wire repository resolver for auto-cloning repos during review task creation
	if repoCloner != nil {
		orchestratorSvc.SetRepositoryResolver(&repositoryResolverAdapter{
			cloner:  repoCloner,
			taskSvc: taskSvc,
			logger:  log,
		})

		// Wire repo cloner into executor for provider-backed repos with no local path
		orchestratorSvc.SetRepoCloner(repoCloner, &repoLocalPathUpdater{svc: taskSvc})
	}
	// Worktree fallback self-healing updates the exact task repository row
	// after a successful launch. Keep this seam on the task service so the
	// lifecycle and worktree packages remain task-agnostic.
	orchestratorSvc.SetTaskRepositoryBaseBranchUpdater(&repoLocalPathUpdater{svc: taskSvc})

	return orchestratorSvc, msgCreator, nil
}

type githubCredentialPolicyService interface {
	DescribeTaskGitCredentialPolicy(context.Context, string) (githubpkg.TaskGitCredentialPolicy, error)
}

type githubExecutorCredentialPolicyAdapter struct {
	service githubCredentialPolicyService
}

type githubPRBaseResolver struct {
	service *githubpkg.Service
}

func (r githubPRBaseResolver) ResolvePRBaseBranch(
	ctx context.Context, workspaceID, owner, repo string, number int,
) (string, error) {
	pr, err := r.service.GetPRForAutomation(ctx, workspaceID, owner, repo, number)
	if err != nil || pr == nil {
		return "", err
	}
	return pr.BaseBranch, nil
}

func (a githubExecutorCredentialPolicyAdapter) ResolveTaskGitCredentialPolicy(
	ctx context.Context,
	workspaceID string,
) (executorpkg.TaskGitCredentialPolicy, error) {
	policy, err := a.service.DescribeTaskGitCredentialPolicy(ctx, workspaceID)
	if err != nil {
		return executorpkg.TaskGitCredentialPolicy{}, err
	}
	return executorpkg.TaskGitCredentialPolicy{
		Mode:            policy.Mode,
		WorkspaceMethod: policy.WorkspaceMethod,
		WorkspaceActor:  policy.WorkspaceActor,
	}, nil
}

func githubCredentialBrokerEndpoint(cfg *config.Config) string {
	if cfg != nil {
		if publicBaseURL := strings.TrimRight(strings.TrimSpace(cfg.GitHubCredentialBroker.PublicBaseURL), "/"); publicBaseURL != "" {
			return publicBaseURL + "/api/v1/git/credentials/resolve"
		}
		if cfg.Server.Port != 0 {
			return fmt.Sprintf("http://localhost:%d/api/v1/git/credentials/resolve", cfg.Server.Port)
		}
	}
	return fmt.Sprintf("http://localhost:%d/api/v1/git/credentials/resolve", portsBackendDefault)
}

// resolveQueueMaxPerSession honors the KANDEV_QUEUE_MAX_PER_SESSION env var,
// falling back to messagequeue.DefaultMaxPerSession (10) when unset or invalid.
// Values <= 0 disable the cap entirely (callers can still flood queues — only
// useful in tests / specialized deployments).
func resolveQueueMaxPerSession(pool *db.Pool, log *logger.Logger, startup ...queuesettings.Configuration) int {
	return resolveQueueSettings(pool, log, startup...).Effective.MaxPerSession
}

// resolveQueueMergeEnabled honors the persisted message queue setting,
// falling back to enabled (the shipped default) when unset or invalid.
// Unlike max_per_session it has no environment override.
func resolveQueueMergeEnabled(pool *db.Pool, log *logger.Logger) bool {
	return resolveQueueSettings(pool, log).Effective.MergeEnabled
}

// resolveQueueAutoMergeEnabled honors the persisted automatic merge setting,
// defaulting to enabled when unset or invalid.
func resolveQueueAutoMergeEnabled(pool *db.Pool, log *logger.Logger) bool {
	return resolveQueueSettings(pool, log).Effective.AutoMergeEnabled
}

// resolveQueueSettings loads the persisted message queue settings — falling
// back to defaults when unset, invalid, or the store is unavailable — and
// resolves them against the KANDEV_QUEUE_MAX_PER_SESSION environment
// override.
func resolveQueueSettings(
	pool *db.Pool,
	log *logger.Logger,
	startup ...queuesettings.Configuration,
) queuesettings.Resolution {
	var configured *queuesettings.Settings
	if pool != nil {
		rawStore, err := systemsettings.NewStore(pool)
		if err != nil {
			log.Warn("Failed to initialize message queue settings store", zap.Error(err))
		} else {
			configured, err = queuesettings.NewStore(rawStore).Load(context.Background())
			if err != nil {
				log.Warn("Ignoring invalid persisted message queue settings", zap.Error(err))
				configured = nil
			}
		}
	}
	resolution, err := queuesettings.Resolve(configured, queuesettings.ReadEnvironment(), startup...)
	if err != nil {
		log.Warn("Failed to resolve message queue settings, using defaults", zap.Error(err))
		return queuesettings.Resolution{Response: queuesettings.Response{
			Settings: queuesettings.DefaultSettings(),
			Effective: queuesettings.Effective{
				MaxPerSession:    messagequeue.DefaultMaxPerSession,
				MergeEnabled:     true,
				AutoMergeEnabled: true,
			},
		}}
	}
	if resolution.InvalidEnvironment {
		log.Warn("Ignoring invalid message queue capacity environment value",
			zap.String("environment_variable", queuesettings.EnvironmentVariable))
	}
	return resolution
}

func queueConfiguration(cfg *config.Config) queuesettings.Configuration {
	if cfg == nil || cfg.SourceFor("messageQueue.maxPerSession") != config.SourceConfiguration {
		return queuesettings.Configuration{}
	}
	return queuesettings.Configuration{Value: cfg.MessageQueue.MaxPerSession, Present: true}
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

// GetWorkflowMeta implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetWorkflowMeta(ctx context.Context, workflowID string) (orchestrator.WorkflowMeta, error) {
	meta, err := a.svc.GetWorkflowMeta(ctx, workflowID)
	if err != nil {
		return orchestrator.WorkflowMeta{}, err
	}
	return orchestrator.WorkflowMeta{
		AgentProfileID: meta.AgentProfileID,
		Prompt:         meta.Prompt,
	}, nil
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
			PRNumber:       r.PRNumber,
		})
	}
	result, err := a.svc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Metadata:       req.Metadata,
		Repositories:   repos,
		IsEphemeral:    req.IsEphemeral,
		Origin:         req.Origin,
	})
	if err != nil {
		return nil, err
	}
	return result.Task, nil
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
	result, err := a.svc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		WorkflowID:     req.WorkflowID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Metadata:       req.Metadata,
		Repositories:   repos,
	})
	if err != nil {
		return nil, err
	}
	return result.Task, nil
}

// jiraServiceAdapter exposes the JIRA service's issue-watch dedup methods to
// the orchestrator without leaking the rest of the package surface area.
type jiraServiceAdapter struct {
	svc *jirapkg.Service
}

func (a *jiraServiceAdapter) ReserveIssueWatchTask(ctx context.Context, watchID, issueKey, issueURL string) (bool, error) {
	return a.svc.Store().ReserveIssueWatchTask(ctx, watchID, issueKey, issueURL)
}

func (a *jiraServiceAdapter) AssignIssueWatchTaskID(ctx context.Context, watchID, issueKey, taskID string) error {
	return a.svc.Store().AssignIssueWatchTaskID(ctx, watchID, issueKey, taskID)
}

func (a *jiraServiceAdapter) ReleaseIssueWatchTask(ctx context.Context, watchID, issueKey string) error {
	return a.svc.Store().ReleaseIssueWatchTask(ctx, watchID, issueKey)
}

func (a *jiraServiceAdapter) DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error {
	return a.svc.Store().DisableIssueWatchWithError(ctx, watchID, cause)
}

// linearServiceAdapter exposes the Linear service's issue-watch dedup methods
// to the orchestrator without leaking the rest of the package surface area.
type linearServiceAdapter struct {
	svc *linearpkg.Service
}

func (a *linearServiceAdapter) ReserveIssueWatchTask(ctx context.Context, watchID, identifier, issueURL string) (bool, error) {
	return a.svc.Store().ReserveIssueWatchTask(ctx, watchID, identifier, issueURL)
}

func (a *linearServiceAdapter) AssignIssueWatchTaskID(ctx context.Context, watchID, identifier, taskID string) error {
	return a.svc.Store().AssignIssueWatchTaskID(ctx, watchID, identifier, taskID)
}

func (a *linearServiceAdapter) ReleaseIssueWatchTask(ctx context.Context, watchID, identifier string) error {
	return a.svc.Store().ReleaseIssueWatchTask(ctx, watchID, identifier)
}

// sentryServiceAdapter exposes the Sentry service's issue-watch dedup methods
// to the orchestrator without leaking the rest of the package surface area.
type sentryServiceAdapter struct {
	svc *sentrypkg.Service
}

func (a *sentryServiceAdapter) ReserveIssueWatchTask(ctx context.Context, watchID, shortID, issueURL string) (bool, error) {
	return a.svc.Store().ReserveIssueWatchTask(ctx, watchID, shortID, issueURL)
}

func (a *sentryServiceAdapter) AssignIssueWatchTaskID(ctx context.Context, watchID, shortID, taskID string) error {
	return a.svc.Store().AssignIssueWatchTaskID(ctx, watchID, shortID, taskID)
}

func (a *sentryServiceAdapter) ReleaseIssueWatchTask(ctx context.Context, watchID, shortID string) error {
	return a.svc.Store().ReleaseIssueWatchTask(ctx, watchID, shortID)
}

func (a *sentryServiceAdapter) DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error {
	return a.svc.Store().DisableIssueWatchWithError(ctx, watchID, cause)
}

func (a *linearServiceAdapter) DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error {
	return a.svc.Store().DisableIssueWatchWithError(ctx, watchID, cause)
}

// profileLookupAdapter satisfies orchestrator.ProfileLookup by delegating to
// the agent settings store. Returns (true, name, nil) when the row exists
// but is soft-deleted, which the orchestrator treats as a signal to self-heal
// the bound watcher (set enabled=0 + last_error). All other shapes — live
// row, missing row, driver error — are returned verbatim so the dispatch
// pipeline fails open on transient failures.
type profileLookupAdapter struct {
	store settingsstore.Repository
}

func (a *profileLookupAdapter) LookupProfile(ctx context.Context, profileID string) (bool, string, error) {
	if _, err := a.store.GetAgentProfile(ctx, profileID); err == nil {
		return false, "", nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, "", err
	}
	profile, err := a.store.GetAgentProfileIncludingDeleted(ctx, profileID)
	if err != nil || profile == nil || profile.DeletedAt == nil {
		return false, "", err
	}
	return true, profile.Name, nil
}

// watcherDepsAdapter enumerates linear / jira / github_issue / github_review
// watcher rows that reference an agent profile. The profile-delete confirm
// dialog uses the list to render "this will also disable N watchers — are
// you sure?".
//
// Each integration's package is optional in dev mode; nil-safe so the
// adapter degrades gracefully when one isn't wired.
// automationDepsAdapter lets the agent-settings controller name the enabled
// automations bound to a profile without importing the automation package's
// types into it.
type automationDepsAdapter struct {
	store *automationpkg.Store
}

type utilityDepsAdapter struct {
	svc     *utilityservice.Service
	userSvc *userservice.Service
}

func (a *utilityDepsAdapter) ListUtilityAgentsByAgentProfile(ctx context.Context, profileID string) ([]agentsettingscontroller.UtilityAgentReference, error) {
	if a == nil || a.svc == nil {
		return nil, nil
	}
	agents, err := a.svc.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]agentsettingscontroller.UtilityAgentReference, 0)
	defaultProfileID := ""
	if a.userSvc != nil {
		defaultProfileID, err = a.userSvc.GetDefaultUtilityAgentProfileID(ctx)
		if err != nil {
			return nil, err
		}
	}
	for _, agent := range agents {
		if agent != nil && (agent.AgentProfileID == profileID ||
			(utilitymodels.UsesDefaultProfile(agent) && defaultProfileID == profileID)) {
			refs = append(refs, agentsettingscontroller.UtilityAgentReference{ID: agent.ID, Name: agent.Name})
		}
	}
	return refs, nil
}

func (a *utilityDepsAdapter) ClearUtilityAgentProfileBindings(ctx context.Context, profileID string) error {
	if a == nil || a.svc == nil {
		return nil
	}
	if err := a.svc.ClearAgentProfileBindings(ctx, profileID); err != nil {
		return err
	}
	return nil
}

func (a *automationDepsAdapter) ListEnabledAutomationsByAgentProfile(
	ctx context.Context, profileID string,
) ([]agentsettingscontroller.AutomationReference, error) {
	if a == nil || a.store == nil {
		return nil, nil
	}
	bindings, err := a.store.ListEnabledByAgentProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	refs := make([]agentsettingscontroller.AutomationReference, 0, len(bindings))
	for _, b := range bindings {
		refs = append(refs, agentsettingscontroller.AutomationReference{
			ID:          b.ID,
			Name:        b.Name,
			WorkspaceID: b.WorkspaceID,
		})
	}
	return refs, nil
}

func (a *automationDepsAdapter) DisableAutomationsByAgentProfile(
	ctx context.Context, profileID string,
) ([]agentsettingscontroller.AutomationReference, error) {
	if a == nil || a.store == nil {
		return nil, nil
	}
	bindings, err := a.store.DisableByAgentProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	refs := make([]agentsettingscontroller.AutomationReference, 0, len(bindings))
	for _, b := range bindings {
		refs = append(refs, agentsettingscontroller.AutomationReference{
			ID:          b.ID,
			Name:        b.Name,
			WorkspaceID: b.WorkspaceID,
		})
	}
	return refs, nil
}

type watcherDepsAdapter struct {
	linear *linearpkg.Service
	jira   *jirapkg.Service
	github githubWatcherLister
	log    *logger.Logger
}

// githubWatcherLister is the slice of github.Service the adapter needs.
// Local alias so the adapter file stays free of the wider GitHubService
// orchestrator interface, which carries many unrelated methods. Uses the
// enabled-only listers so already-disabled (self-healed) watchers do not
// inflate the dependency count surfaced in ErrProfileInUseDetail. Disable
// methods are the same store-level helpers the dispatch coordinator's
// self-heal path uses.
type githubWatcherLister interface {
	ListEnabledIssueWatches(ctx context.Context) ([]*githubpkg.IssueWatch, error)
	ListEnabledReviewWatches(ctx context.Context) ([]*githubpkg.ReviewWatch, error)
	DisableIssueWatchWithError(ctx context.Context, watchID, cause string) error
	DisableReviewWatchWithError(ctx context.Context, watchID, cause string) error
}

func (a *watcherDepsAdapter) ListWatchersByAgentProfile(ctx context.Context, profileID string) ([]agentsettingscontroller.WatcherReference, error) {
	if profileID == "" {
		return nil, nil
	}
	var refs []agentsettingscontroller.WatcherReference
	more, err := a.listLinearRefs(ctx, profileID)
	if err != nil {
		return nil, err
	}
	refs = append(refs, more...)
	more, err = a.listJiraRefs(ctx, profileID)
	if err != nil {
		return nil, err
	}
	refs = append(refs, more...)
	more, err = a.listGitHubRefs(ctx, profileID)
	if err != nil {
		return nil, err
	}
	refs = append(refs, more...)
	return refs, nil
}

// DisableWatchersByAgentProfile enumerates the enabled watcher rows that
// reference profileID and flips each to enabled=0 with the supplied cause.
// Returns the list it disabled so the caller can log. Best-effort across
// integrations: a failure for one integration logs and continues to the
// next so a single broken store doesn't strand the rest of the user's
// orphaned watchers.
func (a *watcherDepsAdapter) DisableWatchersByAgentProfile(ctx context.Context, profileID, cause string) ([]agentsettingscontroller.WatcherReference, error) {
	refs, err := a.ListWatchersByAgentProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	disabled := make([]agentsettingscontroller.WatcherReference, 0, len(refs))
	for _, ref := range refs {
		if err := a.disableByKind(ctx, ref.Kind, ref.ID, cause); err != nil {
			a.log.Warn("failed to disable referencing watcher",
				zap.String("profile_id", profileID),
				zap.String("watcher_kind", ref.Kind),
				zap.String("watcher_id", ref.ID),
				zap.Error(err))
			continue
		}
		disabled = append(disabled, ref)
	}
	return disabled, nil
}

type routingTierDepsAdapter struct {
	repo *officesqlite.Repository
}

func (a *routingTierDepsAdapter) ListRoutingTierReferencesByAgentProfile(
	ctx context.Context,
	profileID string,
) ([]agentsettingscontroller.RoutingTierReference, error) {
	if a == nil || a.repo == nil || profileID == "" {
		return nil, nil
	}
	refs, err := a.repo.ListRoutingTierReferencesByAgentProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	out := make([]agentsettingscontroller.RoutingTierReference, 0, len(refs))
	for _, ref := range refs {
		out = append(out, agentsettingscontroller.RoutingTierReference{
			WorkspaceID: ref.WorkspaceID,
			ProviderID:  string(ref.ProviderID),
			Tier:        string(ref.Tier),
		})
	}
	return out, nil
}

func (a *watcherDepsAdapter) disableByKind(ctx context.Context, kind, watchID, cause string) error {
	switch kind {
	case "linear":
		if a.linear == nil {
			return nil
		}
		return a.linear.Store().DisableIssueWatchWithError(ctx, watchID, cause)
	case "jira":
		if a.jira == nil {
			return nil
		}
		return a.jira.Store().DisableIssueWatchWithError(ctx, watchID, cause)
	case "github_issue":
		if a.github == nil {
			return nil
		}
		return a.github.DisableIssueWatchWithError(ctx, watchID, cause)
	case "github_review":
		if a.github == nil {
			return nil
		}
		return a.github.DisableReviewWatchWithError(ctx, watchID, cause)
	default:
		return fmt.Errorf("unknown watcher kind: %s", kind)
	}
}

func (a *watcherDepsAdapter) listLinearRefs(ctx context.Context, profileID string) ([]agentsettingscontroller.WatcherReference, error) {
	if a.linear == nil {
		return nil, nil
	}
	watches, err := a.linear.Store().ListEnabledIssueWatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("linear watchers: %w", err)
	}
	var out []agentsettingscontroller.WatcherReference
	for _, w := range watches {
		if w.AgentProfileID == profileID {
			out = append(out, agentsettingscontroller.WatcherReference{
				ID: w.ID, Kind: "linear", Label: linearWatchLabel(w),
			})
		}
	}
	return out, nil
}

func (a *watcherDepsAdapter) listJiraRefs(ctx context.Context, profileID string) ([]agentsettingscontroller.WatcherReference, error) {
	if a.jira == nil {
		return nil, nil
	}
	watches, err := a.jira.Store().ListEnabledIssueWatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("jira watchers: %w", err)
	}
	var out []agentsettingscontroller.WatcherReference
	for _, w := range watches {
		if w.AgentProfileID == profileID {
			out = append(out, agentsettingscontroller.WatcherReference{
				ID: w.ID, Kind: "jira", Label: truncateWatcherLabel(w.JQL),
			})
		}
	}
	return out, nil
}

func (a *watcherDepsAdapter) listGitHubRefs(ctx context.Context, profileID string) ([]agentsettingscontroller.WatcherReference, error) {
	if a.github == nil {
		return nil, nil
	}
	issues, err := a.github.ListEnabledIssueWatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("github issue watchers: %w", err)
	}
	var out []agentsettingscontroller.WatcherReference
	for _, w := range issues {
		if w.AgentProfileID == profileID {
			out = append(out, agentsettingscontroller.WatcherReference{
				ID: w.ID, Kind: "github_issue", Label: githubIssueWatchLabel(w),
			})
		}
	}
	reviews, err := a.github.ListEnabledReviewWatches(ctx)
	if err != nil {
		return nil, fmt.Errorf("github review watchers: %w", err)
	}
	for _, w := range reviews {
		if w.AgentProfileID == profileID {
			out = append(out, agentsettingscontroller.WatcherReference{
				ID: w.ID, Kind: "github_review", Label: githubReviewWatchLabel(w),
			})
		}
	}
	return out, nil
}

// linearWatchLabel renders a Linear filter into a short human-readable label
// for the confirmation dialog. The team key is the most-recognisable scoping
// fact; the query string is a fallback. Capped at watcherLabelMaxLen.
func linearWatchLabel(w *linearpkg.IssueWatch) string {
	switch {
	case w.Filter.TeamKey != "":
		return truncateWatcherLabel("team " + w.Filter.TeamKey)
	case w.Filter.Query != "":
		return truncateWatcherLabel(w.Filter.Query)
	default:
		return "all teams"
	}
}

func githubIssueWatchLabel(w *githubpkg.IssueWatch) string {
	if w.CustomQuery != "" {
		return truncateWatcherLabel(w.CustomQuery)
	}
	return truncateWatcherLabel(githubReposJoin(w.Repos))
}

func githubReviewWatchLabel(w *githubpkg.ReviewWatch) string {
	if w.CustomQuery != "" {
		return truncateWatcherLabel(w.CustomQuery)
	}
	return truncateWatcherLabel(githubReposJoin(w.Repos))
}

func githubReposJoin(repos []githubpkg.RepoFilter) string {
	if len(repos) == 0 {
		return "all repos"
	}
	parts := make([]string, 0, len(repos))
	for _, r := range repos {
		parts = append(parts, r.Owner+"/"+r.Name)
	}
	return strings.Join(parts, ", ")
}

const watcherLabelMaxLen = 80

// truncateWatcherLabel caps a watcher's human-readable label at
// watcherLabelMaxLen runes, appending "…" when truncation happens. Counted
// in runes (not bytes) so a JQL or filter ending in multi-byte UTF-8
// (Japanese, emoji) is not split mid-codepoint — slicing a byte string
// would emit invalid UTF-8 and the UI would render the replacement
// character.
func truncateWatcherLabel(s string) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= watcherLabelMaxLen {
		return s
	}
	return string(runes[:watcherLabelMaxLen-1]) + "…"
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

func (u *repoLocalPathUpdater) UpdateRepositoryDefaultBranch(ctx context.Context, repositoryID, defaultBranch string) error {
	if repositoryID == "" || defaultBranch == "" {
		return fmt.Errorf("UpdateRepositoryDefaultBranch: repositoryID and defaultBranch must be non-empty")
	}
	_, err := u.svc.UpdateRepository(ctx, repositoryID, &taskservice.UpdateRepositoryRequest{
		DefaultBranch: &defaultBranch,
	})
	return err
}

func (u *repoLocalPathUpdater) UpdateTaskRepositoryBaseBranch(ctx context.Context, taskID, taskRepositoryID, baseBranch string) error {
	_, err := u.svc.UpdateRepositoryBaseBranch(ctx, taskservice.UpdateRepositoryBaseBranchRequest{
		TaskID:           taskID,
		TaskRepositoryID: taskRepositoryID,
		BaseBranch:       baseBranch,
	})
	return err
}

// repositoryResolverAdapter resolves GitHub repos by cloning + finding/creating DB records.
type repositoryResolverAdapter struct {
	cloner  reviewRepositoryCloner
	taskSvc *taskservice.Service
	logger  *logger.Logger
}

type reviewRepositoryCloner interface {
	EnsureWorkspaceCloned(context.Context, string, string, string, string, string) (string, error)
	BuildCloneURLWithHost(context.Context, string, string, string, string) (string, error)
}

// ResolveForReview implements orchestrator.RepositoryResolver.
//
// If the workspace already has a Repository configured for the given provider
// info with a non-empty LocalPath, that repo is reused and no clone is
// performed. Otherwise the repo is cloned into the kandev-managed location and
// a Repository entity is created.
func (a *repositoryResolverAdapter) ResolveForReview(
	ctx context.Context, workspaceID, provider, owner, name, defaultBranch string,
) (string, string, error) {
	hostname, err := defaultProviderHostname(provider)
	if err != nil {
		return "", "", err
	}
	providerHost := "https://" + hostname
	existing, err := a.taskSvc.GetRepositoryByProviderInfo(ctx, workspaceID, provider, providerHost, owner, name)
	if err != nil {
		return "", "", fmt.Errorf("lookup repository by provider info: %w", err)
	}
	if existing != nil && existing.LocalPath != "" {
		baseBranch := a.resolveReviewBaseBranch(ctx, existing, existing.LocalPath, defaultBranch)
		return existing.ID, baseBranch, nil
	}

	cloneURL, err := a.cloner.BuildCloneURLWithHost(ctx, provider, providerHost, owner, name)
	if err != nil {
		return "", "", fmt.Errorf("unsupported provider: %w", err)
	}

	localPath, err := a.cloner.EnsureWorkspaceCloned(ctx, workspaceID, provider, cloneURL, owner, name)
	if err != nil {
		return "", "", fmt.Errorf("clone repository: %w", err)
	}

	repo, _, err := a.taskSvc.FindOrCreateRepository(ctx, &taskservice.FindOrCreateRepositoryRequest{
		WorkspaceID:   workspaceID,
		Provider:      provider,
		ProviderHost:  providerHost,
		ProviderOwner: owner,
		ProviderName:  name,
		DefaultBranch: defaultBranch,
		LocalPath:     localPath,
	})
	if err != nil {
		return "", "", fmt.Errorf("find/create repository: %w", err)
	}

	baseBranch := a.resolveReviewBaseBranch(ctx, repo, localPath, defaultBranch)
	return repo.ID, baseBranch, nil
}

func defaultProviderHostname(provider string) (string, error) {
	switch strings.ToLower(provider) {
	case "gitlab":
		return "gitlab.com", nil
	case gitCredentialGitHubProviderID, "":
		return gitCredentialGitHubHost, nil
	default:
		return "", fmt.Errorf("unsupported review repository provider %q", provider)
	}
}

func (a *repositoryResolverAdapter) resolveReviewBaseBranch(
	ctx context.Context,
	repo *taskmodels.Repository,
	localPath string,
	requestedBranch string,
) string {
	if requestedBranch != "" {
		return requestedBranch
	}
	stored := strings.TrimSpace(repo.DefaultBranch)
	if stored == defaultMasterBranch && localPath != "" {
		if detected := detectGitDefaultBranch(localPath); detected == defaultMainBranch {
			return a.persistDetectedDefaultBranch(ctx, repo, detected)
		}
	}
	if stored != "" {
		return stored
	}
	return a.detectAndPersistDefaultBranch(ctx, repo, localPath)
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
	return a.persistDetectedDefaultBranch(ctx, repo, detected)
}

func (a *repositoryResolverAdapter) persistDetectedDefaultBranch(
	ctx context.Context,
	repo *taskmodels.Repository,
	detected string,
) string {
	if strings.TrimSpace(repo.DefaultBranch) == detected {
		return detected
	}
	// detected is still the correct base branch for the review in progress
	// even when the write below fails, so it is always returned. But a
	// failure here is not a transient blip to shrug off: it leaves
	// repositories.default_branch empty, which (per spec "Degraded
	// evaluation") permanently degrades every future delivery-ledger
	// evaluation of this repository to default_branch_unknown until some
	// later write succeeds — and this call site retries with the same
	// detected value on every future invocation, so a rejection driven by
	// validation (as opposed to a transient DB error) will repeat forever.
	// Review round 3, finding #4: this used to log at Warn and nothing
	// else, making that permanent degradation invisible. Error level plus
	// the dedicated counter make it observable the same way ancestry/write
	// failures already are in internal/delivery/metrics.go.
	if _, err := a.taskSvc.UpdateRepository(ctx, repo.ID, &taskservice.UpdateRepositoryRequest{
		DefaultBranch: &detected,
	}); err != nil {
		delivery.RecordDefaultBranchPersistError()
		a.logger.Error("failed to persist detected default branch: repository row keeps its prior "+
			"default_branch and will read as default_branch_unknown in the delivery ledger until a "+
			"future write succeeds",
			zap.String("repository_id", repo.ID),
			zap.String(branchFieldKey, detected),
			zap.Error(err))
	}
	return detected
}

// detectGitDefaultBranch reads the default branch of a git repository.
// Returns empty string on any failure.
func detectGitDefaultBranch(repoPath string) string {
	branch, err := gitref.DefaultBranchOrEmpty(repoPath)
	if err != nil {
		return ""
	}
	return branch
}
