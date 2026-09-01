package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	analyticsservice "github.com/kandev/kandev/internal/analytics/service"
	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/azuredevops"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/integrations/secretadapter"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	"github.com/kandev/kandev/internal/mentions"
	"github.com/kandev/kandev/internal/plugins"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/sentry"
	"github.com/kandev/kandev/internal/steptelemetry"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/share"
	userservice "github.com/kandev/kandev/internal/user/service"
	utilitymodels "github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/profilebinding"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/workflowsync"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	canonicalKandevOwner = "kdlbs"
	canonicalKandevName  = "kandev"
)

func provideServices(cfg *config.Config, log *logger.Logger, repos *Repositories, dbPool *db.Pool, eventBus bus.EventBus, agentRegistry *registry.Registry, version string) (*Services, *agentsettingscontroller.Controller, error) {
	// Load custom TUI agents from DB into registry before discovery
	loadCustomTUIAgents(context.Background(), repos, agentRegistry, log)

	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), agentRegistry, log)
	if err != nil {
		return nil, nil, err
	}
	userSecretStore := secrets.NewUserVisibleStore(repos.Secrets)
	agentSettingsController := agentsettingscontroller.NewController(repos.AgentSettings, discoveryRegistry, agentRegistry, repos.Task, log)
	agentSettingsController.SetDynamicAgentRoutingEnabled(cfg.Features.DynamicAgentRouting)
	agentSettingsController.SetSecretStore(userSecretStore)
	managedRuntimeSettings, err := systemsettings.NewStore(dbPool)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize managed runtime settings: %w", err)
	}
	managedRuntimeSelections := managedruntime.NewStore(managedRuntimeSettings)
	agentSettingsController.SetManagedRuntimeSelectionStore(managedRuntimeSelections)

	userSvc := userservice.NewService(repos.User, eventBus, log)
	editorSvc := editorservice.NewService(repos.Editor, repos.Task, userSvc)
	promptSvc := promptservice.NewService(repos.Prompts)
	utilitySvc := utilityservice.NewService(repos.Utility)
	utilitySvc.SetProfileResolver(profilebinding.New(repos.AgentSettings, func(agentID string) bool {
		if agentID == agents.DynamicAgentID {
			return cfg.Features.DynamicAgentRouting
		}
		_, ok := agentRegistry.GetInferenceAgent(agentID)
		return ok
	}))
	dynamicCircuits := dynamicruntime.NewCircuitRegistry(
		dynamicruntime.WithCircuitPersistence(repos.Task),
	)
	if err := dynamicCircuits.Restore(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("restore dynamic routing health: %w", err)
	}
	dynamicEngine := dynamicruntime.NewEngine(
		dynamicruntime.WithPersistence(repos.Task),
		dynamicruntime.WithStateLoader(repos.Task),
		dynamicruntime.WithCircuitRegistry(dynamicCircuits),
	)
	dynamicBindingResolver, err := dynamicruntime.NewPersistentCredentialBindingResolver(
		context.Background(), repos.Task,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("initialize dynamic routing installation key: %w", err)
	}
	dynamicResolver := agentruntime.NewProfileExecutionResolver(
		repos.AgentSettings,
		dynamicEngine,
		cfg.Features.DynamicAgentRouting,
	)
	dynamicResolver.SetCredentialBindingResolver(dynamicBindingResolver)
	utilitySvc.SetExecutionProfileResolver(dynamicResolver)
	workflowSvc := workflowservice.NewService(repos.Workflow, log)
	pendingActionProjectionEpoch, err := repos.Task.NextPendingActionProjectionEpoch(context.Background())
	if err != nil {
		return nil, nil, fmt.Errorf("allocate pending-action projection epoch: %w", err)
	}
	taskSvc := taskservice.NewService(
		taskservice.Repos{
			Workspaces:        repos.Task,
			Tasks:             repos.Task,
			TaskRepos:         repos.Task,
			WorkspaceFolders:  repos.Task,
			Workflows:         repos.Task,
			Messages:          repos.Task,
			Attachments:       repos.Task,
			Turns:             repos.Task,
			Sessions:          repos.Task,
			GitSnapshots:      repos.Task,
			RepoEntities:      repos.Task,
			RepositorySets:    repos.Task,
			BranchPolicies:    repos.Task,
			RepositoryCleanup: repos.Task,
			Executors:         repos.Task,
			Environments:      repos.Task,
			TaskEnvironments:  repos.Task,
			Reviews:           repos.Task,
			ResourceCleanups:  repos.Task,
			StatusSummaries:   repos.Task,
			TaskActivity:      repos.Task,
			SubagentContexts:  repos.Task,
			Usage:             repos.Task,
		},
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{
			Roots:             cfg.RepositoryDiscovery.Roots,
			MaxDepth:          cfg.RepositoryDiscovery.MaxDepth,
			TaskWorktreeRoots: []string{filepath.Join(cfg.ResolvedHomeDir(), "tasks")},
		},
	)
	taskSvc.SetPendingActionProjectionEpoch(pendingActionProjectionEpoch)
	taskSvc.SetSecretStore(userSecretStore)
	if deleter, ok := userSecretStore.(taskservice.WorkspaceSecretDeleter); ok {
		taskSvc.SetWorkspaceSecretDeleter(deleter)
	}

	// Wire workflow step creator to task service for board creation
	taskSvc.SetWorkflowStepCreator(workflowSvc)
	// Standard Kanban workspace creation is coordinated by the task SQLite
	// repository so workspace, workflow, and steps share one transaction.
	taskSvc.SetWorkspaceBootstrapper(repos.Task)

	// Wire workflow step getter to task service for MoveTask
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})

	// Wire start step resolver to task service for CreateTask
	taskSvc.SetStartStepResolver(&startStepResolverAdapter{svc: workflowSvc})
	// Session history is owned by workflow service, but access is owned by the
	// task service. Keep the authorization check at the service boundary.
	workflowSvc.SetSessionAccessChecker(taskSvc.AuthorizeSessionAccess)
	// Same split for the rest of the workflow-step surface: the workflow
	// package owns steps, templates and export/import, but a workflow's owner
	// is its workspace's owner, which only the task service can resolve.
	workflowSvc.SetWorkflowAccessChecker(taskSvc.AuthorizeWorkflowAccess)
	workflowSvc.SetWorkspaceAccessChecker(taskSvc.AuthorizeWorkspaceAccess)
	// A step's queue_run action names the task it starts work on, so the
	// step-write API accepts task IDs as well.
	workflowSvc.SetTaskAccessChecker(taskSvc.AuthorizeTaskAccess)

	// Wire the ADR 0015 audit-trail writer for manual step transitions.
	// workflowSvc.CreateStepTransition already matches
	// taskservice.StepHistoryRecorder structurally, so no adapter is needed.
	taskSvc.SetStepHistoryRecorder(workflowSvc)

	// Wire workflow provider to workflow service for export/import
	workflowSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})
	// Wire workspace provider for the read-only guard (Improve Kandev workspace)
	workflowSvc.SetWorkspaceProvider(&workflowProviderAdapter{svc: taskSvc})

	// Wire agent profile resolver/matcher for workflow export/import
	workflowSvc.SetAgentProfileFuncs(
		buildAgentProfileResolver(repos),
		buildAgentProfileMatcher(repos, log),
	)

	githubSvc := initGitHubService(cfg, dbPool, eventBus, repos.Secrets, log)
	if githubSvc != nil {
		taskSvc.SetTaskStatusSummaryPRReader(&githubTaskStatusSummaryPRReader{gh: githubSvc})
		githubSvc.SetComparisonTargetObserver(taskSvc)
		githubSvc.SetPromptResolver(promptSvc)
		taskSvc.SetContributionDestinationPreparer(&githubContributionDestinationPreparer{service: githubSvc, taskSvc: taskSvc})
		if brokerErr := githubSvc.ConfigureCredentialBroker(&githubBrokerScopeAuthorizer{repo: repos.Task, provider: githubSvc}); brokerErr != nil {
			log.Warn("GitHub credential broker initialization failed", zap.Error(brokerErr))
		}
	}
	gitlabSvc, gitlabCleanup := initGitLabService(dbPool, eventBus, repos.Secrets, log)
	if gitlabSvc != nil {
		gitlabSvc.SetPromptResolver(promptSvc)
		gitlabSvc.SetComparisonTargetObserver(taskSvc)
	}
	azureDevOpsSvc := initAzureDevOpsService(dbPool, eventBus, repos.Secrets, log)
	if azureDevOpsSvc != nil {
		azureDevOpsSvc.SetRepositoryLookup(&repositoryLookupAdapter{svc: taskSvc})
		azureDevOpsSvc.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
	}
	jiraSvc := initJiraService(dbPool, eventBus, repos.Secrets, log)
	linearSvc := initLinearService(dbPool, eventBus, repos.Secrets, log)
	sentrySvc := initSentryService(dbPool, eventBus, repos.Secrets, log)
	workflowSyncSvc := initWorkflowSyncService(dbPool, githubSvc, gitlabSvc, workflowSvc, taskSvc, log)
	pluginsSvc := initPluginsService(cfg, dbPool, eventBus, repos.Secrets, log)
	if pluginsSvc != nil {
		// The ldflags-injected build version, so Install can enforce a
		// package's manifest.min_kandev_version. This is the only production
		// caller; without it the check stays a no-op. An un-stamped local
		// build passes "dev", which the service treats as "don't enforce".
		pluginsSvc.SetKandevVersion(version)
		pluginsSvc.SetDataSources(taskSvc, taskSvc, workflowSvc, agentSettingsController, analyticsservice.New(repos.Analytics), taskSvc, taskSvc, pluginsTaskWriterAdapter{svc: taskSvc})
		// Separate from SetDataSources: githubSvc is optional (nil when github
		// is unconfigured), and a nil source leaves tasks with no PullRequests
		// rather than failing every task read.
		if githubSvc != nil {
			pluginsSvc.SetTaskPRSource(githubSvc)
		}
		taskSvc.SetRepositorySelectionResolver(pluginRepositorySelectionResolver{inspector: pluginsSvc})
	}
	gitCredentialBroker := newGitCredentialBroker(githubSvc, pluginsSvc, repos.Task, cfg.GitHubCredentialBroker.ReissueSigningKey)
	if pluginsSvc != nil {
		pluginsSvc.SetGitCredentialLeaseRevoker(gitCredentialBroker.RevokeProvider)
	}
	if githubSvc != nil {
		githubSvc.SetCredentialBroker(github.NewCredentialBrokerFromBroker(gitCredentialBroker))
	}
	shareHTTP := initShareHandlers(dbPool, repos.Task, taskSvc, githubSvc, log, version)

	// Plumb code-host branch listing into the task service so provider-backed
	// ("Remote") repos serve branches from their owning provider rather than relying
	// on a local clone that may not exist yet (or ever - some executors clone
	// inside their own container).
	if githubSvc != nil || pluginsSvc != nil {
		taskSvc.SetRemoteBranchLister(codeHostBranchListerAdapter{github: githubSvc, plugins: pluginsSvc})
	}
	if githubSvc != nil {
		taskSvc.SetPRTaskResolver(githubSvc)
		githubSvc.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
		taskSvc.SetWorkspaceDefaultsInitializer(githubSvc)
		startupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := githubSvc.InitializeFreshWorkspaceDefaults(startupCtx)
		cancel()
		if err != nil {
			log.Warn("GitHub fresh workspace defaults initialization failed", zap.Error(err))
		}
	}

	// Initialize Automation service
	automationComponents, automationErr := automation.Provide(dbPool.Writer(), dbPool.Reader(), eventBus, githubSvc, log)
	if automationErr != nil {
		log.Warn("Automation service initialization failed (non-fatal)", zap.Error(automationErr))
	}
	if automationComponents != nil {
		automationComponents.Service.SetTaskDeleter(&automationTaskDeleterAdapter{svc: taskSvc})
		// Per-user workspace scoping for the automation HTTP/WS surface.
		automationComponents.Service.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
		// A UI filter is not an authorization boundary: reject a workflow owned
		// by another workspace even when a request names it directly.
		automationWorkflowLocator := &automationWorkflowLocatorAdapter{svc: taskSvc, workflows: workflowSvc}
		automationComponents.Service.SetWorkflowLocator(automationWorkflowLocator)
		automationComponents.Service.SetWorkflowStepLocator(automationWorkflowLocator)
		automationComponents.Service.SetTaskOriginLookup(&automationTaskOriginLookupAdapter{svc: taskSvc, log: log})
		// Profile deletion disables the automations bound to a profile before
		// the row goes, but nothing ever checked that the binding pointed at a
		// real profile in the first place — so a create or rebind naming an id
		// that never existed produced the same orphan without any delete
		// involved.
		automationComponents.Service.SetAgentProfileLookup(&automationAgentProfileLookupAdapter{store: repos.AgentSettings})
		// YAML export descriptor resolution (AC-29): each Set* below is
		// satisfied directly by an existing repository's Tx-accepting method,
		// so the export's single read transaction can pass straight through
		// without an adapter shim.
		automationComponents.Service.SetExportAgentProfileLookup(repos.AgentSettings)
		automationComponents.Service.SetExportExecutorProfileLookup(repos.Task)
		automationComponents.Service.SetExportWorkflowLookup(repos.Task)
		automationComponents.Service.SetExportWorkflowStepLookup(repos.Workflow)
		automationComponents.Service.SetExportRepositoryLookup(repos.Task)
		automationComponents.Service.SetExportWorkspaceLookup(&automationExportWorkspaceLookupAdapter{svc: taskSvc})
	}

	services := &Services{
		ManagedRuntimeSelections: managedRuntimeSelections,
		DynamicProfileResolver:   dynamicResolver,
		DynamicBindingResolver:   dynamicBindingResolver,
		Task:                     taskSvc,
		User:                     userSvc,
		Editor:                   editorSvc,
		Prompts:                  promptSvc,
		Utility:                  utilitySvc,
		Workflow:                 workflowSvc,
		GitHub:                   githubSvc,
		GitLab:                   gitlabSvc,
		GitLabCleanup:            gitlabCleanup,
		AzureDevOps:              azureDevOpsSvc,
		Jira:                     jiraSvc,
		Linear:                   linearSvc,
		Sentry:                   sentrySvc,
		WorkflowSync:             workflowSyncSvc,
		Share:                    shareHTTP,
		Automation:               automationComponents,
		Plugins:                  pluginsSvc,
		GitCredentials:           gitCredentialBroker,
		// Office is constructed later in initOfficeServices once all
		// of its dependencies (config loader, task integrations, etc.) are available.
		Office: nil,
		// Notification service is initialized after gateway is available.
		Notification: nil,
	}
	mentionProviders := builtinMentionProviders(services, repos.Task)
	reserveBuiltinMentionIdentities(pluginsSvc, mentionProviders)
	mentionComponents, err := newMentionComponents(
		log,
		taskSvc,
		taskSvc,
		mentionProviders...,
	)
	if err != nil {
		return nil, nil, err
	}
	services.Mentions = mentionComponents
	return services, agentSettingsController, nil
}

func reserveBuiltinMentionIdentities(
	pluginService *plugins.Service, providers []mentions.MentionProvider,
) {
	if pluginService == nil {
		return
	}
	identities := make([]plugins.ReferenceIdentity, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if _, dynamic := provider.(mentions.SourceRegistrar); dynamic {
			continue
		}
		descriptor := provider.Descriptor()
		identities = append(identities, plugins.ReferenceIdentity{
			Source: descriptor.Source, Provider: descriptor.Provider, Kind: descriptor.Kind,
		})
	}
	pluginService.SetReservedReferenceIdentities(identities)
}

type githubContributionDestinationPreparer struct {
	service *github.Service
	taskSvc *taskservice.Service
}

func (p *githubContributionDestinationPreparer) PrepareContributionDestination(
	ctx context.Context,
	req *taskservice.CreateTaskRequest,
	workflow *taskmodels.Workflow,
	repositories []*taskmodels.Repository,
) error {
	if p == nil || p.service == nil || req == nil || workflow == nil ||
		workflow.WorkflowTemplateID == nil || *workflow.WorkflowTemplateID != "improve-kandev" {
		return nil
	}
	policy, err := p.service.DescribeTaskGitCredentialPolicy(ctx, req.WorkspaceID)
	if err != nil {
		return fmt.Errorf("resolve Improve Kandev GitHub credential policy: %w", err)
	}
	if policy.Mode != github.TaskGitCredentialsModeManaged {
		return nil
	}
	for index := range req.Repositories {
		if err := p.prepareRepository(ctx, req, repositories, index); err != nil {
			return err
		}
	}
	return nil
}

func (p *githubContributionDestinationPreparer) prepareRepository(
	ctx context.Context,
	req *taskservice.CreateTaskRequest,
	repositories []*taskmodels.Repository,
	index int,
) error {
	input := &req.Repositories[index]
	if input.RemoteContribution != nil || input.ContributionDestination != nil ||
		!isCanonicalKandevRepositoryInput(input, repositoryAt(repositories, index)) {
		return nil
	}
	resolution, err := p.service.ResolveContributionForkForWorkspace(
		ctx, req.WorkspaceID, canonicalKandevOwner, canonicalKandevName, true,
	)
	if err != nil {
		return fmt.Errorf("prepare Improve Kandev contribution destination: %w", err)
	}
	if resolution.Repository == nil || resolution.Repository.ID <= 0 {
		return fmt.Errorf("prepare Improve Kandev contribution destination: canonical provider ID is missing")
	}
	providerRepoID := strconv.FormatInt(resolution.Repository.ID, 10)
	destination := resolution.Destination
	if destination != nil && strings.TrimSpace(destination.SourceRepository.ProviderID) != providerRepoID {
		return fmt.Errorf("prepare Improve Kandev contribution destination: canonical provider ID is inconsistent")
	}
	if err := p.reconcileProviderRepoID(ctx, repositoryAt(repositories, index), providerRepoID); err != nil {
		return err
	}
	input.ProviderRepoID = providerRepoID
	input.ContributionDestination = destination
	return nil
}

func (p *githubContributionDestinationPreparer) reconcileProviderRepoID(
	ctx context.Context,
	repository *taskmodels.Repository,
	providerRepoID string,
) error {
	if repository == nil {
		return nil
	}
	if repository.ProviderRepoID != "" && !strings.EqualFold(repository.ProviderRepoID, providerRepoID) {
		return fmt.Errorf("prepare Improve Kandev contribution destination: canonical provider ID changed")
	}
	if repository.ProviderRepoID == "" && p.taskSvc != nil {
		if _, err := p.taskSvc.UpdateRepository(ctx, repository.ID, &taskservice.UpdateRepositoryRequest{ProviderRepoID: &providerRepoID}); err != nil {
			return fmt.Errorf("backfill Improve Kandev canonical provider ID: %w", err)
		}
		repository.ProviderRepoID = providerRepoID
	}
	return nil
}

func repositoryAt(repositories []*taskmodels.Repository, index int) *taskmodels.Repository {
	if index < 0 || index >= len(repositories) {
		return nil
	}
	return repositories[index]
}

func isCanonicalKandevRepositoryInput(
	input *taskservice.TaskRepositoryInput,
	repository *taskmodels.Repository,
) bool {
	if repository != nil {
		return repository.Provider == gitCredentialGitHubProviderID &&
			isPublicGitHubProviderHost(repository.ProviderHost) &&
			repository.ProviderOwner == canonicalKandevOwner && repository.ProviderName == canonicalKandevName
	}
	return input != nil && input.Provider == gitCredentialGitHubProviderID &&
		isPublicGitHubProviderHost(input.ProviderHost) &&
		input.ProviderOwner == canonicalKandevOwner && input.ProviderName == canonicalKandevName
}

func isPublicGitHubProviderHost(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "github.com") && parsed.Port() == "" &&
		(parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && parsed.Fragment == ""
}

type githubBrokerScopeAuthorizer struct {
	repo interface {
		GetTask(context.Context, string) (*taskmodels.Task, error)
		GetTaskSession(context.Context, string) (*taskmodels.TaskSession, error)
		GetRepository(context.Context, string) (*taskmodels.Repository, error)
		ListTaskRepositories(context.Context, string) ([]*taskmodels.TaskRepository, error)
	}
	provider interface {
		VerifyContributionDestinationForWorkspace(
			context.Context, string, string, string, string, string, string, string,
		) error
	}
}

func (a *githubBrokerScopeAuthorizer) AuthorizeGitHubRepository(
	ctx context.Context,
	workspaceID, taskID, sessionID, repositoryID, owner, repoName string,
) error {
	return a.authorizeGitHubRepository(ctx, workspaceID, taskID, sessionID, repositoryID, owner, repoName, "", "", false)
}

func (a *githubBrokerScopeAuthorizer) AuthorizeGitHubRepositoryWithIdentity(
	ctx context.Context,
	workspaceID, taskID, sessionID, repositoryID, owner, repoName, providerID, parentProviderID string,
) error {
	return a.authorizeGitHubRepository(
		ctx, workspaceID, taskID, sessionID, repositoryID, owner, repoName, providerID, parentProviderID, true,
	)
}

func (a *githubBrokerScopeAuthorizer) authorizeGitHubRepository(
	ctx context.Context,
	workspaceID, taskID, sessionID, repositoryID, owner, repoName, providerID, parentProviderID string,
	strictIdentity bool,
) error {
	if a == nil || a.repo == nil {
		return fmt.Errorf("task repository is unavailable")
	}
	if err := a.authorizeTaskSession(ctx, workspaceID, taskID, sessionID); err != nil {
		return err
	}
	link, err := a.authorizeTaskRepository(ctx, taskID, repositoryID)
	if err != nil {
		return err
	}
	repository, err := a.repo.GetRepository(ctx, repositoryID)
	if err != nil {
		return err
	}
	if repository == nil || repository.WorkspaceID != workspaceID ||
		!strings.EqualFold(repository.Provider, "github") || !isPublicGitHubProviderHost(repository.ProviderHost) {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	if strings.EqualFold(repository.ProviderOwner, owner) && strings.EqualFold(repository.ProviderName, repoName) {
		return authorizeCanonicalGitHubRepository(repository, providerID, parentProviderID)
	}
	if link == nil {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	binding, found, err := taskmodels.LoadRemoteContribution(link.Metadata)
	if err != nil {
		return fmt.Errorf("validate remote contribution scope: %w", err)
	}
	if handled, destinationErr := a.authorizeContributionDestination(
		ctx, workspaceID, repository, link, owner, repoName, providerID, parentProviderID, strictIdentity,
	); handled {
		return destinationErr
	}
	return authorizeRemoteContribution(binding, found, owner, repoName, providerID, strictIdentity)
}

func authorizeCanonicalGitHubRepository(
	repository *taskmodels.Repository,
	providerID, parentProviderID string,
) error {
	if providerID != "" && !strings.EqualFold(repository.ProviderRepoID, providerID) {
		return fmt.Errorf("repository provider identity does not match lease scope")
	}
	if parentProviderID != "" {
		return fmt.Errorf("repository parent identity does not match lease scope")
	}
	return nil
}

func (a *githubBrokerScopeAuthorizer) authorizeContributionDestination(
	ctx context.Context,
	workspaceID string,
	repository *taskmodels.Repository,
	link *taskmodels.TaskRepository,
	owner, repoName, providerID, parentProviderID string,
	strictIdentity bool,
) (bool, error) {
	destination, found, err := taskmodels.LoadContributionDestination(link.Metadata)
	if err != nil {
		return true, fmt.Errorf("validate contribution destination scope: %w", err)
	}
	if !found || destination.Provider != taskmodels.ContributionDestinationProviderGitHub ||
		!strings.EqualFold(destination.TargetRepository.Host, "github.com") {
		return false, nil
	}
	if !contributionDestinationScopeMatches(
		destination, repository, owner, repoName, providerID, parentProviderID, strictIdentity,
	) {
		return false, nil
	}
	if !strictIdentity {
		return true, nil
	}
	return true, a.verifyContributionDestination(
		ctx, workspaceID, destination, owner, repoName,
	)
}

func contributionDestinationScopeMatches(
	destination taskmodels.ContributionDestination,
	repository *taskmodels.Repository,
	owner, repoName, providerID, parentProviderID string,
	strictIdentity bool,
) bool {
	parts := strings.Split(destination.TargetRepository.Path, "/")
	canonical := strings.TrimSpace(repository.ProviderOwner) + "/" + strings.TrimSpace(repository.ProviderName)
	if len(parts) != 2 || !strings.EqualFold(parts[0], owner) || !strings.EqualFold(parts[1], repoName) ||
		!strings.EqualFold(destination.SourceRepository.Path, canonical) {
		return false
	}
	if repository.ProviderRepoID != "" &&
		!strings.EqualFold(destination.SourceRepository.ProviderID, repository.ProviderRepoID) {
		return false
	}
	if !strictIdentity {
		return true
	}
	return providerID != "" && parentProviderID != "" &&
		strings.EqualFold(destination.TargetRepository.ProviderID, providerID) &&
		strings.EqualFold(destination.SourceRepository.ProviderID, parentProviderID) &&
		strings.TrimSpace(repository.ProviderRepoID) != "" &&
		strings.EqualFold(destination.SourceRepository.ProviderID, repository.ProviderRepoID)
}

func (a *githubBrokerScopeAuthorizer) verifyContributionDestination(
	ctx context.Context,
	workspaceID string,
	destination taskmodels.ContributionDestination,
	owner, repoName string,
) error {
	if a.provider == nil {
		return fmt.Errorf("contribution destination provider verifier is unavailable")
	}
	sourceParts := strings.Split(destination.SourceRepository.Path, "/")
	if len(sourceParts) != 2 {
		return fmt.Errorf("contribution destination provider identity does not match lease scope")
	}
	if err := a.provider.VerifyContributionDestinationForWorkspace(
		ctx, workspaceID, sourceParts[0], sourceParts[1], destination.SourceRepository.ProviderID,
		owner, repoName, destination.TargetRepository.ProviderID,
	); err != nil {
		return fmt.Errorf("contribution destination provider identity does not match lease scope")
	}
	return nil
}

func authorizeRemoteContribution(
	binding taskmodels.RemoteContribution,
	found bool,
	owner, repoName, providerID string,
	strictIdentity bool,
) error {
	if !found || binding.Provider != taskmodels.RemoteContributionProviderGitHub ||
		!binding.CollaborationAllowed || !strings.EqualFold(binding.SourceRepository.Host, "github.com") {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	parts := strings.Split(binding.SourceRepository.Path, "/")
	if len(parts) != 2 || !strings.EqualFold(parts[0], owner) || !strings.EqualFold(parts[1], repoName) {
		return fmt.Errorf("repository identity does not match lease scope")
	}
	if strictIdentity && binding.SourceRepository.ProviderID != "" &&
		!strings.EqualFold(binding.SourceRepository.ProviderID, providerID) {
		return fmt.Errorf("repository provider identity does not match lease scope")
	}
	return nil
}

func (a *githubBrokerScopeAuthorizer) authorizeTaskSession(
	ctx context.Context,
	workspaceID, taskID, sessionID string,
) error {
	task, err := a.repo.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if task == nil || task.WorkspaceID != workspaceID {
		return fmt.Errorf("task does not belong to workspace")
	}
	session, err := a.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != taskID {
		return fmt.Errorf("session does not belong to task")
	}
	switch session.State {
	case taskmodels.TaskSessionStateCompleted,
		taskmodels.TaskSessionStateFailed,
		taskmodels.TaskSessionStateCancelled:
		return fmt.Errorf("session is terminal")
	}
	return nil
}

func (a *githubBrokerScopeAuthorizer) authorizeTaskRepository(
	ctx context.Context,
	taskID, repositoryID string,
) (*taskmodels.TaskRepository, error) {
	links, err := a.repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, link := range links {
		if link != nil && link.RepositoryID == repositoryID {
			return link, nil
		}
	}
	return nil, fmt.Errorf("repository is not linked to task")
}

// loadCustomTUIAgents loads user-defined TUI agents from the database into the registry.
// Non-fatal: logs warnings but continues if any individual agent fails.
func loadCustomTUIAgents(ctx context.Context, repos *Repositories, agentRegistry *registry.Registry, log *logger.Logger) {
	tuiAgents, err := repos.AgentSettings.ListTUIAgents(ctx)
	if err != nil {
		log.Warn("failed to load custom TUI agents from database", zap.Error(err))
		return
	}
	for _, agent := range tuiAgents {
		if agent.TUIConfig == nil {
			continue
		}
		cfg := agent.TUIConfig
		if regErr := agentRegistry.RegisterCustomTUIAgent(registry.CustomTUIAgentSpec{
			Slug:           agent.Name,
			DisplayName:    cfg.DisplayName,
			Command:        cfg.Command,
			Description:    cfg.Description,
			Model:          cfg.Model,
			CommandArgs:    cfg.CommandArgs,
			MCPStrategyKey: cfg.MCPStrategy,
		}); regErr != nil {
			log.Warn("failed to register custom TUI agent",
				zap.String("name", agent.Name), zap.Error(regErr))
		}
	}
}

// workflowStepGetterAdapter adapts workflow service to task service's WorkflowStepGetter interface.
// Since task service now uses wfmodels.WorkflowStep directly, the adapter simply delegates to the service.
type workflowStepGetterAdapter struct {
	svc *workflowservice.Service
}

// GetStep implements taskservice.WorkflowStepGetter.
func (a *workflowStepGetterAdapter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetStep(ctx, stepID)
}

// GetNextStepByPosition implements taskservice.WorkflowStepGetter.
func (a *workflowStepGetterAdapter) GetNextStepByPosition(ctx context.Context, boardID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetNextStepByPosition(ctx, boardID, currentPosition)
}

// ListStepsByWorkflow lets task admission find WIP-limited steps that pull
// newly-created work from a feeder step.
func (a *workflowStepGetterAdapter) ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*wfmodels.WorkflowStep, error) {
	return a.svc.ListStepsByWorkflow(ctx, workflowID)
}

// startStepResolverAdapter adapts workflow service to task service's StartStepResolver interface.
type startStepResolverAdapter struct {
	svc *workflowservice.Service
}

// ResolveStartStep implements taskservice.StartStepResolver.
func (a *startStepResolverAdapter) ResolveStartStep(ctx context.Context, workflowID string) (string, error) {
	step, err := a.svc.ResolveStartStep(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return step.ID, nil
}

// ResolveFirstStep implements taskservice.StartStepResolver.
func (a *startStepResolverAdapter) ResolveFirstStep(ctx context.Context, workflowID string) (string, error) {
	step, err := a.svc.ResolveFirstStep(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return step.ID, nil
}

// ResolveAutoStartStep implements taskservice.StartStepResolver.
func (a *startStepResolverAdapter) ResolveAutoStartStep(ctx context.Context, workflowID string) (string, error) {
	step, err := a.svc.ResolveAutoStartStep(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return step.ID, nil
}

// githubSecretAdapter adapts secrets.SecretStore to github.SecretProvider and github.SecretManager.
type githubSecretAdapter struct {
	store secrets.SecretStore
}

func (a *githubSecretAdapter) List(ctx context.Context) ([]*github.SecretListItem, error) {
	items, err := a.store.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*github.SecretListItem, len(items))
	for i, item := range items {
		result[i] = &github.SecretListItem{
			ID:       item.ID,
			Name:     item.Name,
			HasValue: item.HasValue,
		}
	}
	return result, nil
}

func (a *githubSecretAdapter) Reveal(ctx context.Context, id string) (string, error) {
	return a.store.Reveal(ctx, id)
}

// Create creates a new secret with the given name and value.
func (a *githubSecretAdapter) Create(ctx context.Context, name, value string) (string, error) {
	secret := &secrets.SecretWithValue{
		Secret: secrets.Secret{Name: name},
		Value:  value,
	}
	if err := a.store.Create(ctx, secret); err != nil {
		return "", err
	}
	return secret.ID, nil
}

// Update updates an existing secret's value.
func (a *githubSecretAdapter) Update(ctx context.Context, id, value string) error {
	return a.store.Update(ctx, id, &secrets.UpdateSecretRequest{Value: &value})
}

// Delete removes a secret by ID.
func (a *githubSecretAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

// initGitHubService wires up the GitHub integration. Failures are non-fatal:
// the rest of the backend still boots without GitHub configured.
func initGitHubService(
	cfg *config.Config,
	dbPool *db.Pool,
	eventBus bus.EventBus,
	secretsStore secrets.SecretStore,
	log *logger.Logger,
) *github.Service {
	adapter := &githubSecretAdapter{store: secretsStore}
	svc, _, err := github.Provide(dbPool.Writer(), dbPool.Reader(), adapter, eventBus, log)
	if err != nil {
		log.Warn("GitHub service initialization failed (non-fatal)", zap.Error(err))
	}
	if svc != nil {
		// GitHub takes both a SecretProvider (read-only) and a SecretManager
		// (mutating) — same adapter satisfies both interfaces, but the
		// service needs the mutating one wired explicitly.
		svc.SetSecretManager(adapter)
		svc.SetConnectionSecretStore(secretadapter.New(secretsStore))
		if authErr := svc.InitializeAppRegistrationLifecycle(); authErr != nil {
			log.Warn("GitHub App registration lifecycle failed to initialize", zap.Error(authErr))
		}
		if authErr := svc.InitializeAppRegistrationRuntimes(context.Background()); authErr != nil {
			log.Warn("one or more GitHub App registrations failed to initialize", zap.Error(authErr))
		}
	}
	return svc
}

// gitlabSecretAdapter adapts secrets.SecretStore to the GitLab integration's
// SecretProvider and SecretManager interfaces. Mirrors githubSecretAdapter
// — kept separate so the two packages can evolve independently.
type gitlabSecretAdapter struct {
	store secrets.SecretStore
}

func (a *gitlabSecretAdapter) List(ctx context.Context) ([]*gitlab.SecretListItem, error) {
	items, err := a.store.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*gitlab.SecretListItem, len(items))
	for i, item := range items {
		result[i] = &gitlab.SecretListItem{
			ID:       item.ID,
			Name:     item.Name,
			HasValue: item.HasValue,
		}
	}
	return result, nil
}

func (a *gitlabSecretAdapter) Reveal(ctx context.Context, id string) (string, error) {
	return a.store.Reveal(ctx, id)
}

func (a *gitlabSecretAdapter) Create(ctx context.Context, name, value string) (string, error) {
	secret := &secrets.SecretWithValue{
		Secret: secrets.Secret{Name: name},
		Value:  value,
	}
	if err := a.store.Create(ctx, secret); err != nil {
		return "", err
	}
	return secret.ID, nil
}

func (a *gitlabSecretAdapter) Update(ctx context.Context, id, value string) error {
	return a.store.Update(ctx, id, &secrets.UpdateSecretRequest{Value: &value})
}

func (a *gitlabSecretAdapter) Delete(ctx context.Context, id string) error {
	return a.store.Delete(ctx, id)
}

const gitlabHostSettingKey = "gitlab_host"

type gitlabHostStore struct {
	settings *systemsettings.Store
}

func newGitLabHostStore(dbPool *db.Pool) (gitlab.HostStore, error) {
	settingsStore, err := systemsettings.NewStore(dbPool)
	if err != nil {
		return nil, err
	}
	return &gitlabHostStore{settings: settingsStore}, nil
}

func (s *gitlabHostStore) GetHost(ctx context.Context) (string, error) {
	raw, found, err := s.settings.Get(ctx, gitlabHostSettingKey)
	if err != nil || !found {
		return "", err
	}
	return string(raw), nil
}

func (s *gitlabHostStore) SetHost(ctx context.Context, host string) error {
	return s.settings.Save(ctx, gitlabHostSettingKey, []byte(host))
}

// initGitLabService wires up the GitLab integration. Failures are non-fatal:
// the rest of the backend still boots without GitLab configured.
func initGitLabService(dbPool *db.Pool, eventBus bus.EventBus, secretsStore secrets.SecretStore, log *logger.Logger) (*gitlab.Service, func() error) {
	adapter := &gitlabSecretAdapter{store: secretsStore}
	hostStore, hostStoreErr := newGitLabHostStore(dbPool)
	if hostStoreErr != nil {
		log.Warn("GitLab host store unavailable (non-fatal)", zap.Error(hostStoreErr))
		return nil, nil
	}
	svc, cleanup, err := gitlab.Provide(context.Background(), adapter, hostStore, log)
	if err != nil {
		log.Warn("GitLab service initialization failed (non-fatal)", zap.Error(err))
	}
	if svc != nil {
		svc.SetSecretManager(adapter)
		svc.SetWorkspaceSecretStore(secretadapter.New(secretsStore))
		svc.SetEventBus(eventBus)
		if store, storeErr := gitlab.NewStore(dbPool.Writer(), dbPool.Reader()); storeErr == nil {
			svc.SetStore(store)
			if migrationErr := gitlab.MigrateLegacyConnection(
				context.Background(), store, secretadapter.New(secretsStore), adapter, adapter, hostStore, log,
			); migrationErr != nil {
				log.Warn("GitLab legacy connection migration failed (non-fatal)", zap.Error(migrationErr))
			}
		} else {
			log.Warn("GitLab task-mr store unavailable (non-fatal)", zap.Error(storeErr))
		}
	}
	return svc, cleanup
}

// initJiraService wires up the Jira integration. Failures are non-fatal.
func initJiraService(dbPool *db.Pool, eventBus bus.EventBus, secretsStore secrets.SecretStore, log *logger.Logger) *jira.Service {
	svc, _, err := jira.Provide(dbPool.Writer(), dbPool.Reader(), secretadapter.New(secretsStore), eventBus, log)
	if err != nil {
		log.Warn("JIRA service initialization failed (non-fatal)", zap.Error(err))
	}
	return svc
}

// initAzureDevOpsService wires the workspace-scoped Azure integration.
// Failures are non-fatal so unrelated providers and the backend keep working.
func initAzureDevOpsService(
	dbPool *db.Pool,
	eventBus bus.EventBus,
	secretsStore secrets.SecretStore,
	log *logger.Logger,
) *azuredevops.Service {
	svc, _, err := azuredevops.Provide(
		dbPool.Writer(), dbPool.Reader(), secretadapter.New(secretsStore), eventBus, log,
	)
	if err != nil {
		log.Warn("Azure DevOps service initialization failed (non-fatal)", zap.Error(err))
		return nil
	}
	return svc
}

// initWorkflowSyncService wires the workflow-sync service. Either integration
// may be nil; a workspace configured for the unavailable one gets an
// actionable failure at sync time rather than the service failing to boot.
// Failures are non-fatal; the service is nil only if the store itself fails.
func initWorkflowSyncService(
	dbPool *db.Pool, githubSvc *github.Service, gitlabSvc *gitlab.Service,
	workflowSvc *workflowservice.Service, taskSvc *taskservice.Service, log *logger.Logger,
) *workflowsync.Service {
	if githubSvc == nil && gitlabSvc == nil {
		log.Warn("workflow sync disabled: no GitHub or GitLab service available")
		return nil
	}
	workflowSvc.SetSyncWorkflowOps(taskSvc)
	var githubClients workflowsync.GitHubClientProvider
	if githubSvc != nil {
		githubClients = githubSvc
	}
	var gitlabClients workflowsync.GitLabClientProvider
	if gitlabSvc != nil {
		gitlabClients = gitlabSvc
	}
	svc, _, err := workflowsync.Provide(dbPool.Writer(), dbPool.Reader(), githubClients, gitlabClients, workflowSvc, log)
	if err != nil {
		log.Warn("workflow sync service initialization failed (non-fatal)", zap.Error(err))
		return nil
	}
	svc.SetWorkspaceAuthorizer(taskSvc.AuthorizeWorkspaceAccess)
	return svc
}

// initLinearService wires up the Linear integration. Failures are non-fatal.
func initLinearService(dbPool *db.Pool, eventBus bus.EventBus, secretsStore secrets.SecretStore, log *logger.Logger) *linear.Service {
	svc, _, err := linear.Provide(dbPool.Writer(), dbPool.Reader(), secretadapter.New(secretsStore), eventBus, log)
	if err != nil {
		log.Warn("Linear service initialization failed (non-fatal)", zap.Error(err))
	}
	return svc
}

// initSentryService wires up the Sentry integration. Failures are non-fatal.
func initSentryService(dbPool *db.Pool, eventBus bus.EventBus, secretsStore secrets.SecretStore, log *logger.Logger) *sentry.Service {
	svc, _, err := sentry.Provide(dbPool.Writer(), dbPool.Reader(), secretadapter.New(secretsStore), eventBus, log)
	if err != nil {
		log.Warn("Sentry service initialization failed (non-fatal)", zap.Error(err))
	}
	return svc
}

// initShareHandlers wires up the public-share-links HTTP surface. Failures
// are non-fatal: the rest of the backend boots without the share endpoints.
// GitHub access resolves from the owning task workspace on every operation.
func initShareHandlers(
	dbPool *db.Pool,
	taskRepo share.TaskReader,
	authorizer share.TaskAccessAuthorizer,
	githubSvc *github.Service,
	log *logger.Logger,
	version string,
) *share.HTTPHandlers {
	h, _, err := share.Provide(
		dbPool.Writer(), dbPool.Reader(), taskRepo, authorizer, githubSvc, log,
		share.Config{KandevVersion: version},
	)
	if err != nil {
		log.Warn("Share handlers initialization failed (non-fatal)", zap.Error(err))
		return nil
	}
	return h
}

// portsBackendDefault is the default backend HTTP port. We don't import
// internal/common/ports here to avoid pulling its transitive deps into
// services.go's import graph; the value is a fallback for when
// cfg.Server.Port is left at zero (which shouldn't happen in practice).
const portsBackendDefault = 38429

// initPluginsService wires up the plugin system's core Service
// (registration registry, config, plugin_state store). Failures are
// non-fatal: the rest of the backend still boots without plugins.
//
// This only constructs the Service — event delivery (delivery.Deliverer)
// and health monitoring (plugins.HealthMonitor) are wired separately by
// startPluginsSubsystems (plugins.go), once addCleanup and ctx are
// available, mirroring how the Jira/Linear/Sentry pollers are started in
// startAgentInfrastructure rather than inside their init*Service functions.
func initPluginsService(
	cfg *config.Config,
	dbPool *db.Pool,
	eventBus bus.EventBus,
	secretsStore secrets.SecretStore,
	log *logger.Logger,
) *plugins.Service {
	svc, _, err := plugins.Provide(cfg, dbPool, secretadapter.New(secretsStore), eventBus, log)
	if err != nil {
		log.Warn("Plugins service initialization failed (non-fatal)", zap.Error(err))
		return nil
	}
	return svc
}

type pluginsHostUtilityAdapter struct {
	mgr *hostutility.Manager
}

func (a pluginsHostUtilityAdapter) ExecuteProfilePrompt(ctx context.Context, profileID, prompt string) (string, error) {
	res, err := a.mgr.ExecuteProfilePrompt(ctx, profileID, prompt)
	if err != nil {
		return "", err
	}
	return res.Response, nil
}

type pluginsUtilityAgentAdapter struct {
	svc     *utilityservice.Service
	userSvc *userservice.Service
}

func (a pluginsUtilityAgentAdapter) GetAgentByID(ctx context.Context, id string) (*plugins.UtilityAgent, error) {
	agent, err := a.svc.GetAgentByID(ctx, id)
	if err != nil {
		if errors.Is(err, utilityservice.ErrAgentNotFound) {
			return nil, plugins.ErrUtilityAgentNotFound
		}
		return nil, err
	}
	profileID := agent.AgentProfileID
	bindingState := agent.ProfileBindingState
	if utilitymodels.UsesDefaultProfile(agent) && a.userSvc != nil {
		profileID, err = a.userSvc.GetDefaultUtilityAgentProfileID(ctx)
		if err != nil {
			return nil, err
		}
		if profileID != "" {
			bindingState = utilitymodels.ProfileBindingExplicit
		}
	}
	return &plugins.UtilityAgent{Name: agent.Name, AgentID: agent.AgentID, Model: agent.Model, AgentProfileID: profileID, ProfileBindingState: bindingState, Enabled: agent.Enabled}, nil
}

// pluginsTaskWriterAdapter adapts the task service to the plugins package's
// taskWriter interface (Host data API CreateTask/UpdateTask write RPCs, ADR
// 0043 phase 2). It translates the plugins-local TaskCreateInput/TaskUpdateInput
// — which internal/plugins can't express as service.CreateTaskRequest without
// an import cycle — into the real service requests, so writes route through the
// same task.*-event-publishing service methods the REST/MCP API uses. The
// plugin's provenance is stamped into task metadata (`source`), since the Task
// model has no dedicated source column.
// pluginTaskWriteService is the narrow slice of the task service the write
// adapter needs, so the adapter's field mapping + state validation are
// unit-testable with a fake. *taskservice.Service satisfies it.
type pluginTaskWriteService interface {
	CreateTask(ctx context.Context, req *taskservice.CreateTaskRequest) (taskservice.CreateTaskResult, error)
	UpdateTask(ctx context.Context, id string, req *taskservice.UpdateTaskRequest) (*taskmodels.Task, error)
	DeleteTask(ctx context.Context, id string) error
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	MoveTaskWithOptions(ctx context.Context, id, workflowID, workflowStepID string, position int, opts taskservice.MoveTaskOptions) (*taskservice.MoveTaskResult, error)
}

type pluginsTaskWriterAdapter struct {
	svc pluginTaskWriteService
}

func (a pluginsTaskWriterAdapter) CreateTask(ctx context.Context, in plugins.TaskCreateInput) (*taskmodels.Task, error) {
	metadata, err := pluginTaskMetadata(in)
	if err != nil {
		return nil, err
	}
	repositories, err := pluginTaskRepositoryInputs(in.Repositories)
	if err != nil {
		return nil, err
	}
	result, err := a.svc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID:    in.WorkspaceID,
		WorkflowID:     in.WorkflowID,
		WorkflowStepID: in.WorkflowStepID,
		Title:          in.Title,
		Description:    in.Description,
		ParentID:       in.ParentID,
		Metadata:       metadata,
		Repositories:   repositories,
		PlanMode:       in.PlanMode,
		StartAgent:     in.StartAgent,
	})
	if err != nil {
		return nil, err
	}
	return result.Task, nil
}

func (a pluginsTaskWriterAdapter) DeleteTask(ctx context.Context, id string) error {
	return a.svc.DeleteTask(ctx, id)
}

func pluginTaskMetadata(in plugins.TaskCreateInput) (map[string]interface{}, error) {
	if in.Metadata == nil && in.Source == "" {
		return nil, nil
	}
	metadata := make(map[string]interface{}, len(in.Metadata)+1)
	for key, value := range in.Metadata {
		metadata[key] = value
	}
	if in.Source == "" {
		return metadata, nil
	}
	if source, found := metadata["source"]; found && source != in.Source {
		return nil, fmt.Errorf("plugin task metadata source does not match host provenance")
	}
	metadata["source"] = in.Source
	return metadata, nil
}

func pluginTaskRepositoryInputs(repositories []pluginsdk.PluginTaskRepository) ([]taskservice.TaskRepositoryInput, error) {
	if len(repositories) == 0 {
		return nil, nil
	}
	inputs := make([]taskservice.TaskRepositoryInput, len(repositories))
	for index, repository := range repositories {
		input, err := pluginTaskRepositoryInput(repository)
		if err != nil {
			return nil, err
		}
		inputs[index] = input
	}
	return inputs, nil
}

func pluginTaskRepositoryInput(repository pluginsdk.PluginTaskRepository) (taskservice.TaskRepositoryInput, error) {
	if repository.Remote == nil {
		return taskservice.TaskRepositoryInput{
			RepositoryID:   repository.RepositoryID,
			BaseBranch:     stringValue(repository.BaseBranch),
			CheckoutBranch: stringValue(repository.CheckoutBranch),
			PRNumber:       pluginPRNumber(repository.PullRequestNumber),
		}, nil
	}
	remote := repository.Remote
	if err := repoclone.ValidateHTTPSCloneOrigin(remote.CloneURL, remote.ProviderHost); err != nil {
		return taskservice.TaskRepositoryInput{}, fmt.Errorf("plugin repository descriptor: %w", err)
	}
	return taskservice.TaskRepositoryInput{
		RepositoryID:              repository.RepositoryID,
		BaseBranch:                firstStringValue(repository.BaseBranch, remote.BaseBranch),
		CheckoutBranch:            firstStringValue(repository.CheckoutBranch, remote.HeadBranch),
		PRNumber:                  firstPluginPRNumber(repository.PullRequestNumber, remote.PullRequestNumber),
		RemoteURL:                 remote.CloneURL,
		Provider:                  remote.ProviderID,
		ProviderHost:              remote.ProviderHost,
		ProviderScope:             remote.ProviderScope,
		ProviderRepoID:            remote.ProviderRepositoryID,
		ProviderOwner:             remote.OwnerOrProject,
		ProviderName:              remote.Name,
		DefaultBranch:             stringValue(remote.DefaultBranch),
		TrustedProviderDescriptor: true,
	}, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstStringValue(values ...*string) string {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return ""
}

func pluginPRNumber(value *int64) int {
	if value == nil || *value < 1 || *value > int64(^uint(0)>>1) {
		return 0
	}
	return int(*value)
}

func firstPluginPRNumber(values ...*int64) int {
	for _, value := range values {
		if number := pluginPRNumber(value); number > 0 {
			return number
		}
	}
	return 0
}

func (a pluginsTaskWriterAdapter) UpdateTask(ctx context.Context, in plugins.TaskUpdateInput) (*taskmodels.Task, error) {
	// workflow_step_id is rejected on this path regardless of value (including
	// a present but empty string) — Update never calls publishTaskMovedEvent,
	// so writing the column directly would move the card without firing
	// on_enter actions like auto-start. Use Move instead, which routes through
	// MoveTaskWithOptions. Checked before the state validation below so a
	// request that also carries an invalid state is still named as a
	// rejected move, not a state error.
	if in.WorkflowStepID != nil {
		return nil, status.Error(codes.InvalidArgument,
			"workflow_step_id cannot be set via UpdateTask: use MoveTask to transition a task between workflow steps")
	}
	req := &taskservice.UpdateTaskRequest{
		Title:       in.Title,
		Description: in.Description,
	}
	if in.State != nil {
		// v1.TaskState is a string type, so the cast can't fail — validate the
		// value against the known enum here (the REST/MCP path validates state
		// at its HTTP handler) so a plugin typo can't forward a bogus state to
		// the service and risk persisting it.
		state := v1.TaskState(*in.State)
		if !validPluginTaskState(state) {
			return nil, status.Errorf(codes.InvalidArgument, "invalid task state %q", *in.State)
		}
		req.State = &state
	}
	return a.svc.UpdateTask(ctx, in.ID, req)
}

func (a pluginsTaskWriterAdapter) MoveTask(ctx context.Context, in plugins.TaskMoveInput) (*plugins.TaskMoveResult, error) {
	workflowID, err := a.resolvePluginMoveWorkflowID(ctx, in)
	if err != nil {
		return nil, err
	}

	opts := taskservice.MoveTaskOptions{
		StepHistoryTrigger: wfmodels.StepTransitionTriggerPluginMove,
		StepHistoryActor:   wfmodels.StepTransitionActorSystem,
	}
	if in.WorkflowID == nil {
		// workflowID above was inherited from a separate GetTask pre-read
		// (resolvePluginMoveWorkflowID), not named explicitly by the plugin.
		// Guard against a concurrent reassignment landing between that read
		// and the write below: MoveTaskWithOptions re-checks this against the
		// task's WorkflowID from its own fresh GetTask and fails the move
		// instead of silently reverting the concurrent change.
		opts.ExpectedWorkflowID = &workflowID
	}

	// Attach attribution before calling MoveTaskWithOptions: that method only
	// falls back to its own default attribution when none is already present
	// on the context (steptelemetry.HasTrigger), so setting it here — exactly
	// like BulkMoveTasks does — survives untouched into the recorded ledger row.
	ctx = steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		Trigger:   steptelemetry.TriggerPluginMove,
		ActorKind: steptelemetry.ActorIntegration,
		ActorID:   in.Source,
	})

	result, err := a.svc.MoveTaskWithOptions(ctx, in.TaskID, workflowID, in.WorkflowStepID, int(in.Position), opts)
	if err != nil {
		return nil, classifyPluginMoveError(err)
	}
	return &plugins.TaskMoveResult{
		Task:         result.Task,
		Transitioned: result.Transitioned,
		FromStepID:   result.FromStepID,
	}, nil
}

// resolvePluginMoveWorkflowID resolves the effective workflow id for a plugin
// move: an explicit WorkflowID is used as-is; a nil WorkflowID inherits the
// task's current workflow, requiring a lookup MoveTaskWithOptions doesn't do
// on its own (it takes a plain workflow id with no "inherit current" case).
//
// A task with no current workflow resolves to "", passed through rather than
// rejected here (AC-005.11 still ends in InvalidArgument, just not from this
// function) — the binding precedence ladder requires the task's own state
// (archived/active-session, AC-001.7/001.8) to be checked before its
// destination (AC-001.6/005.11) is judged. Those state checks live inside
// MoveTaskWithOptions' validateTaskMove, called after this function returns,
// so failing fast here on an empty workflow id would answer InvalidArgument
// for an archived task instead of FailedPrecondition. Passing "" through
// instead lets validateTaskMove's GetWorkflow("") fail naturally — after the
// state gates — with a "workflow not found" error classifyPluginMoveError
// already maps to InvalidArgument.
func (a pluginsTaskWriterAdapter) resolvePluginMoveWorkflowID(ctx context.Context, in plugins.TaskMoveInput) (string, error) {
	if in.WorkflowID != nil {
		return *in.WorkflowID, nil
	}
	task, err := a.svc.GetTask(ctx, in.TaskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			// classifyPluginMoveError's fixed "task not found" message, not a
			// %q-interpolated one: MoveTask returns this error directly to the
			// plugin without routing it through that classifier (see the
			// caller), so building the status here has to match its no-leaked-
			// identifiers convention itself rather than relying on a wrapper
			// that never runs for this branch.
			return "", classifyPluginMoveError(err)
		}
		// Do not forward repository or driver details across the plugin gRPC
		// boundary. The classifier preserves cancellation codes and maps every
		// other unexpected lookup failure to a fixed Internal status.
		return "", classifyPluginMoveError(err)
	}
	return task.WorkflowID, nil
}

// classifyPluginMoveError maps MoveTaskWithOptions' bare validation errors to
// the plugin MoveTask RPC's binding error-mapping table. It intentionally does
// NOT reuse mcp/handlers.classifyMoveTaskError: that classifier lowercases and
// buckets archived/active-session/different-workspace/step-not-in-workflow
// into a single Conflict code, but this table requires FailedPrecondition for
// the first two and InvalidArgument for the latter two.
//
// Every branch returns a fixed, generic message rather than the underlying
// err.Error() text: validateTaskMove's messages are meant for trusted
// board/MCP callers and can name internal identifiers, so forwarding them
// verbatim to a plugin (an external, less-trusted caller) over the gRPC
// status would leak implementation detail the plugin has no legitimate need
// for. This mirrors classifyMoveTaskError's own fixed-message convention.
func classifyPluginMoveError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return status.Error(codes.Canceled, "move task canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "move task deadline exceeded")
	}
	if errors.Is(err, repoerrors.ErrTaskNotFound) {
		return status.Error(codes.NotFound, "task not found")
	}
	if errors.Is(err, taskservice.ErrWorkflowResolutionConflict) {
		return status.Error(codes.Aborted, "task workflow changed concurrently, retry the move")
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "archived tasks cannot be moved"):
		return status.Error(codes.FailedPrecondition, "task is archived and cannot be moved")
	case strings.Contains(msg, "task has an active session"):
		return status.Error(codes.FailedPrecondition, "task has an active session and cannot be moved")
	case strings.Contains(msg, "workflow not found"),
		strings.Contains(msg, "workflow step not found"),
		strings.Contains(msg, "target workflow is in a different workspace"),
		strings.Contains(msg, "target workflow step does not belong to target workflow"):
		return status.Error(codes.InvalidArgument, "invalid move_task request: unknown or mismatched workflow, step, or workspace")
	default:
		return status.Error(codes.Internal, "failed to move task")
	}
}

// validPluginTaskState reports whether state is a state a plugin may set via
// UpdateTask. SCHEDULING is intentionally excluded — it is an orchestrator-owned
// transient state, not a value a plugin should assign directly.
func validPluginTaskState(state v1.TaskState) bool {
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateInProgress,
		v1.TaskStateReview, v1.TaskStateBlocked, v1.TaskStateWaitingForInput,
		v1.TaskStateCompleted, v1.TaskStateFailed, v1.TaskStateCancelled:
		return true
	default:
		return false
	}
}

// workflowProviderAdapter adapts task service to workflow service's WorkflowProvider interface.
type workflowProviderAdapter struct {
	svc *taskservice.Service
}

// ListWorkflows implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*taskmodels.Workflow, error) {
	return a.svc.ListWorkflows(ctx, workspaceID, includeHidden)
}

// GetWorkflow implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
	return a.svc.GetWorkflow(ctx, id)
}

// GetWorkspace implements workflowservice.WorkspaceProvider.
func (a *workflowProviderAdapter) GetWorkspace(ctx context.Context, id string) (*taskmodels.Workspace, error) {
	return a.svc.GetWorkspace(ctx, id)
}

// CreateWorkflow implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	return a.svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
	})
}

// UpdateWorkflow implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) UpdateWorkflow(ctx context.Context, workflow *taskmodels.Workflow) error {
	prompt := workflow.Prompt
	_, err := a.svc.UpdateWorkflow(ctx, workflow.ID, &taskservice.UpdateWorkflowRequest{
		Name:           &workflow.Name,
		Description:    &workflow.Description,
		Prompt:         &prompt,
		AgentProfileID: &workflow.AgentProfileID,
	})
	return err
}

// buildAgentProfileResolver creates a resolver that converts profile IDs to portable form for export.
func buildAgentProfileResolver(repos *Repositories) wfmodels.AgentProfileResolver {
	return func(profileID string) *wfmodels.AgentProfilePortable {
		if profileID == "" {
			return nil
		}
		profile, err := repos.AgentSettings.GetAgentProfile(context.Background(), profileID)
		if err != nil || profile == nil {
			return nil
		}
		return &wfmodels.AgentProfilePortable{
			AgentName: profile.AgentDisplayName,
			Model:     profile.Model,
			Mode:      profile.Mode,
		}
	}
}

// agentProfileStillMatches reports whether the profile with id still has the
// exact (agent_display_name, model, mode) triple, ignoring Enabled and
// WorkspaceID - those only gate candidate *selection*, not a binding that
// already exists.
func agentProfileStillMatches(repos *Repositories, id, agentName, model, mode string) bool {
	p, err := repos.AgentSettings.GetAgentProfile(context.Background(), id)
	if err != nil || p == nil {
		return false
	}
	return p.AgentDisplayName == agentName && p.Model == model && p.Mode == mode
}

// buildAgentProfileMatcher creates a matcher that finds profiles by agent
// name, model, and mode for import.
//
// The (agent_display_name, model, mode) triple is not unique: duplicating a
// profile through the UI produces a byte-identical triple for the copy.
// Candidates are filtered to enabled, global (non-workspace-scoped) profiles,
// and ties are broken by oldest CreatedAt (then ID for a total order). Oldest
// wins so that duplicating a profile can never steal an existing synced
// workflow step's binding - a copy is always newer than its source.
//
// currentID, when non-empty, is checked first: if that profile still has the
// exact descriptor, it's kept as-is without re-running candidate selection -
// even when it's disabled or workspace-scoped. Reconciliation must not treat
// "profile got disabled" as "profile needs a new binding" (profile-disable.md
// promises existing bindings survive disabling); candidate selection only
// applies when picking a profile for new work.
func buildAgentProfileMatcher(repos *Repositories, log *logger.Logger) wfmodels.AgentProfileMatcher {
	return func(agentName, model, mode, currentID string) string {
		if currentID != "" && agentProfileStillMatches(repos, currentID, agentName, model, mode) {
			return currentID
		}
		return selectAgentProfileCandidate(repos, log, agentName, model, mode)
	}
}

// selectAgentProfileCandidate scans enabled, global profiles for an exact
// (agent_display_name, model, mode) match and returns the oldest one (see
// buildAgentProfileMatcher), logging when the triple was ambiguous.
func selectAgentProfileCandidate(repos *Repositories, log *logger.Logger, agentName, model, mode string) string {
	agents, err := repos.AgentSettings.ListAgents(context.Background())
	if err != nil {
		return ""
	}
	var best *agentsettingsmodels.AgentProfile
	matches := 0
	for _, agent := range agents {
		profiles, pErr := repos.AgentSettings.ListAgentProfiles(context.Background(), agent.ID)
		if pErr != nil {
			continue
		}
		for _, p := range profiles {
			if p.AgentDisplayName != agentName || p.Model != model || p.Mode != mode {
				continue
			}
			if !p.Enabled || p.WorkspaceID != "" {
				continue
			}
			matches++
			if best == nil || isOlderAgentProfileMatch(p, best) {
				best = p
			}
		}
	}
	if best == nil {
		return ""
	}
	if matches > 1 {
		log.Debug("agent profile matcher: multiple candidates for workflow step sync",
			zap.String("agent_display_name", agentName),
			zap.String("model", model),
			zap.String("mode", mode),
			zap.Int("candidates", matches),
			zap.String("selected_profile_id", best.ID))
	}
	return best.ID
}

// isOlderAgentProfileMatch reports whether candidate should replace current
// as the matcher's selection: earlier CreatedAt wins, ties broken by ID so
// the result is stable across repeated calls.
func isOlderAgentProfileMatch(candidate, current *agentsettingsmodels.AgentProfile) bool {
	if !candidate.CreatedAt.Equal(current.CreatedAt) {
		return candidate.CreatedAt.Before(current.CreatedAt)
	}
	return candidate.ID < current.ID
}

// codeHostBranchListerAdapter routes first-party GitHub repositories directly
// and manifest-owned providers through their standardized branch action. It maps
// both responses into the task
// service's Branch shape with Type="remote" so the dialog renders branches
// the same way URL-mode does - bare names without an "origin/" prefix, since
// there is no checked-out clone whose tracking config could disambiguate.
type codeHostBranchListerAdapter struct {
	github  *github.Service
	plugins *plugins.Service
}

const codeHostRemoteBranchType = "remote"

func (a codeHostBranchListerAdapter) ListRepoBranches(
	ctx context.Context, source taskservice.RemoteBranchSource,
) ([]taskservice.Branch, error) {
	if strings.EqualFold(source.Provider, "github") {
		if a.github == nil {
			return nil, fmt.Errorf("GitHub branch provider is unavailable")
		}
		remote, err := a.github.ListRepoBranchesForWorkspace(ctx, source.WorkspaceID, source.Owner, source.Name)
		if err != nil {
			return nil, err
		}
		out := make([]taskservice.Branch, 0, len(remote))
		for _, branch := range remote {
			out = append(out, taskservice.Branch{Name: branch.Name, Type: codeHostRemoteBranchType})
		}
		return out, nil
	}
	if a.plugins == nil {
		return nil, fmt.Errorf("plugin repository branch provider is unavailable")
	}
	remote, err := a.plugins.ListRepositoryProviderBranches(ctx, source.WorkspaceID, plugins.RepositoryProviderSource{
		Provider:             source.Provider,
		ProviderHost:         source.ProviderHost,
		ProviderScope:        source.ProviderScope,
		ProviderRepositoryID: source.ProviderRepositoryID,
		OwnerOrProject:       source.Owner,
		Name:                 source.Name,
		CloneURL:             source.RemoteURL,
		DefaultBranch:        source.DefaultBranch,
	})
	if err != nil {
		return nil, err
	}
	out := make([]taskservice.Branch, 0, len(remote))
	for _, branch := range remote {
		out = append(out, taskservice.Branch{Name: branch.Name, Type: codeHostRemoteBranchType})
	}
	return out, nil
}
