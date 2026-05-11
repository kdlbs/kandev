package main

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/integrations/secretadapter"
	"github.com/kandev/kandev/internal/jira"
	"github.com/kandev/kandev/internal/linear"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/slack"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	utilityservice "github.com/kandev/kandev/internal/utility/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

func provideServices(cfg *config.Config, log *logger.Logger, repos *Repositories, dbPool *db.Pool, eventBus bus.EventBus, agentRegistry *registry.Registry) (*Services, *agentsettingscontroller.Controller, error) {
	// Load custom TUI agents from DB into registry before discovery
	loadCustomTUIAgents(context.Background(), repos, agentRegistry, log)

	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), agentRegistry, log)
	if err != nil {
		return nil, nil, err
	}
	agentSettingsController := agentsettingscontroller.NewController(repos.AgentSettings, discoveryRegistry, agentRegistry, repos.Task, log)

	userSvc := userservice.NewService(repos.User, eventBus, log)
	editorSvc := editorservice.NewService(repos.Editor, repos.Task, userSvc)
	promptSvc := promptservice.NewService(repos.Prompts)
	utilitySvc := utilityservice.NewService(repos.Utility)
	workflowSvc := workflowservice.NewService(repos.Workflow, log)
	taskSvc := taskservice.NewService(
		taskservice.Repos{
			Workspaces:       repos.Task,
			Tasks:            repos.Task,
			TaskRepos:        repos.Task,
			Workflows:        repos.Task,
			Messages:         repos.Task,
			Turns:            repos.Task,
			Sessions:         repos.Task,
			GitSnapshots:     repos.Task,
			RepoEntities:     repos.Task,
			Executors:        repos.Task,
			Environments:     repos.Task,
			TaskEnvironments: repos.Task,
			Reviews:          repos.Task,
		},
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{
			Roots:    cfg.RepositoryDiscovery.Roots,
			MaxDepth: cfg.RepositoryDiscovery.MaxDepth,
		},
	)

	// Wire workflow step creator to task service for board creation
	taskSvc.SetWorkflowStepCreator(workflowSvc)

	// Wire workflow step getter to task service for MoveTask
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})

	// Wire start step resolver to task service for CreateTask
	taskSvc.SetStartStepResolver(&startStepResolverAdapter{svc: workflowSvc})

	// Wire workflow provider to workflow service for export/import
	workflowSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})

	// Wire agent profile resolver/matcher for workflow export/import
	workflowSvc.SetAgentProfileFuncs(
		buildAgentProfileResolver(repos),
		buildAgentProfileMatcher(repos),
	)

	githubSvc := initGitHubService(dbPool, eventBus, repos.Secrets, log)
	jiraSvc := initJiraService(dbPool, eventBus, repos.Secrets, log)
	linearSvc := initLinearService(dbPool, eventBus, repos.Secrets, log)
	slackSvc := initSlackService(dbPool, repos.Secrets, log)

	// Plumb GitHub branch listing into the task service so provider-backed
	// ("Remote") repos serve branches from the GitHub API rather than relying
	// on a local clone that may not exist yet (or ever - some executors clone
	// inside their own container).
	if githubSvc != nil {
		taskSvc.SetRemoteBranchLister(githubBranchListerAdapter{svc: githubSvc})
	}

	return &Services{
		Task:     taskSvc,
		User:     userSvc,
		Editor:   editorSvc,
		Prompts:  promptSvc,
		Utility:  utilitySvc,
		Workflow: workflowSvc,
		GitHub:   githubSvc,
		Jira:     jiraSvc,
		Linear:   linearSvc,
		Slack:    slackSvc,
		// Notification service is initialized after gateway is available.
		Notification: nil,
	}, agentSettingsController, nil
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
		if regErr := agentRegistry.RegisterCustomTUIAgent(
			agent.Name, cfg.DisplayName, cfg.Command, cfg.Description, cfg.Model, cfg.CommandArgs,
		); regErr != nil {
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
func initGitHubService(dbPool *db.Pool, eventBus bus.EventBus, secretsStore secrets.SecretStore, log *logger.Logger) *github.Service {
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
	}
	return svc
}

// initJiraService wires up the Jira integration. Failures are non-fatal.
func initJiraService(dbPool *db.Pool, eventBus bus.EventBus, secretsStore secrets.SecretStore, log *logger.Logger) *jira.Service {
	svc, _, err := jira.Provide(dbPool.Writer(), dbPool.Reader(), secretadapter.New(secretsStore), eventBus, log)
	if err != nil {
		log.Warn("JIRA service initialization failed (non-fatal)", zap.Error(err))
	}
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

// initSlackService wires up the Slack integration. Failures are non-fatal.
// The agent runner is wired post-construction by main.go once hostutility +
// utility services exist.
func initSlackService(dbPool *db.Pool, secretsStore secrets.SecretStore, log *logger.Logger) *slack.Service {
	svc, _, err := slack.Provide(dbPool.Writer(), dbPool.Reader(), secretadapter.New(secretsStore), log)
	if err != nil {
		log.Warn("Slack service initialization failed (non-fatal)", zap.Error(err))
	}
	return svc
}

// buildKandevMCPURL is the URL passed to the Slack triage agent for the
// Kandev MCP server. The MCP server is mounted on the same port as the rest
// of the backend's HTTP API; this just centralises the path so it stays in
// sync with internal/mcp/server's mount point ("/mcp").
func buildKandevMCPURL(port int) string {
	if port == 0 {
		port = portsBackendDefault
	}
	return fmt.Sprintf("http://localhost:%d/mcp", port)
}

// portsBackendDefault is the default backend HTTP port. We don't import
// internal/common/ports here to avoid pulling its transitive deps into
// services.go's import graph; the value is duplicated only as a fallback for
// when cfg.Server.Port is left at zero (which shouldn't happen in practice).
const portsBackendDefault = 38429

// slackHostUtilityAdapter adapts *hostutility.Manager to slack.HostUtilityRunner.
// The slack package can't import hostutility without a transitive cycle (it
// would need to import agentctl + lifecycle), so we shim through the agentctl
// utility DTO here in the cmd package where both are already imported.
type slackHostUtilityAdapter struct {
	mgr *hostutility.Manager
}

func (a slackHostUtilityAdapter) ExecutePromptWithMCP(
	ctx context.Context,
	agentType, model, mode, prompt string,
	mcpServers []agentctlutil.MCPServerDTO,
) (slack.HostPromptResult, error) {
	res, err := a.mgr.ExecutePromptWithMCP(ctx, agentType, model, mode, prompt, mcpServers)
	if err != nil {
		return slack.HostPromptResult{}, err
	}
	return slack.HostPromptResult{
		Response:       res.Response,
		Model:          res.Model,
		PromptTokens:   res.PromptTokens,
		ResponseTokens: res.ResponseTokens,
		DurationMs:     res.DurationMs,
	}, nil
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
	_, err := a.svc.UpdateWorkflow(ctx, workflow.ID, &taskservice.UpdateWorkflowRequest{
		Name:           &workflow.Name,
		Description:    &workflow.Description,
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

// buildAgentProfileMatcher creates a matcher that finds profiles by agent name, model, and mode for import.
func buildAgentProfileMatcher(repos *Repositories) wfmodels.AgentProfileMatcher {
	return func(agentName, model, mode string) string {
		agents, err := repos.AgentSettings.ListAgents(context.Background())
		if err != nil {
			return ""
		}
		for _, agent := range agents {
			profiles, pErr := repos.AgentSettings.ListAgentProfiles(context.Background(), agent.ID)
			if pErr != nil {
				continue
			}
			for _, p := range profiles {
				if p.AgentDisplayName == agentName && p.Model == model && p.Mode == mode {
					return p.ID
				}
			}
		}
		return ""
	}
}

// githubBranchListerAdapter bridges github.Service to the task service's
// RemoteBranchLister interface. It maps github.RepoBranch into the task
// service's Branch shape with Type="remote" so the dialog renders branches
// the same way URL-mode does - bare names without an "origin/" prefix, since
// there is no checked-out clone whose tracking config could disambiguate.
type githubBranchListerAdapter struct {
	svc *github.Service
}

func (a githubBranchListerAdapter) ListRepoBranches(ctx context.Context, owner, repo string) ([]taskservice.Branch, error) {
	remote, err := a.svc.ListRepoBranches(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	out := make([]taskservice.Branch, 0, len(remote))
	for _, b := range remote {
		out = append(out, taskservice.Branch{Name: b.Name, Type: "remote"})
	}
	return out, nil
}
