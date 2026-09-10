package backendapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/credentials"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

func provideLifecycleManager(
	ctx context.Context,
	cfg *config.Config,
	log *logger.Logger,
	eventBus bus.EventBus,
	agentSettingsRepo settingsstore.Repository,
	legacyWorkspaceSource legacyMCPWorkspaceSource,
	agentRegistry *registry.Registry,
	rawSecretStore secrets.SecretStore,
	baseBranchProvider lifecycle.BaseBranchProvider,
	comparisonTargetProvider lifecycle.ComparisonTargetProvider,
	managedRuntimeSelections managedruntime.SelectionReader,
	mcpIdentityScoper lifecycle.MCPIdentityScoper,
	mcpPrincipalScoper lifecycle.MCPPrincipalScoper,
) (*lifecycle.Manager, error) {
	log.Info("Initializing Agent Manager...")
	secretStores := newLifecycleSecretStores(rawSecretStore)

	// Create runtime registry to manage multiple runtimes
	executorRegistry := lifecycle.NewExecutorRegistry(log)

	// Standalone runtime is always available (agentctl is a core service)
	controlClient := agentctl.NewControlClient(
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
		agentctl.WithControlAuthToken(cfg.Agent.StandaloneAuthToken),
	)
	standaloneExec := lifecycle.NewStandaloneExecutor(
		controlClient,
		cfg.Agent.StandaloneHost,
		cfg.Agent.StandalonePort,
		log,
	)
	standaloneExec.SetAuthToken(cfg.Agent.StandaloneAuthToken)

	// Create InteractiveRunner for passthrough mode (no WorkspaceTracker, uses callbacks)
	interactiveRunner := process.NewInteractiveRunner(nil, log, 2*1024*1024) // 2MB buffer
	standaloneExec.SetInteractiveRunner(interactiveRunner)

	executorRegistry.Register(standaloneExec)
	log.Info("Standalone runtime registered with passthrough support",
		zap.String("host", cfg.Agent.StandaloneHost),
		zap.Int("port", cfg.Agent.StandalonePort))

	// Register Docker runtime if enabled (client is created lazily on first use)
	if cfg.Docker.Enabled {
		dockerExec := lifecycle.NewDockerExecutor(cfg.Docker, cfg.ResolvedHomeDir(), log)
		executorRegistry.Register(dockerExec)
		log.Info("Docker runtime registered (lazy initialization)")
	}

	// Register Remote Docker runtime (always available, instances are created lazily per host)
	remoteDockerExec := lifecycle.NewRemoteDockerExecutor(log)
	executorRegistry.Register(remoteDockerExec)
	log.Info("Remote Docker runtime registered")

	// Register Sprites runtime (remote sandboxes via Sprites.dev)
	agentctlResolver := lifecycle.NewAgentctlResolver(log)
	spritesExec := lifecycle.NewSpritesExecutor(secretStores.credentials, agentRegistry, agentctlResolver, 8765, log)
	executorRegistry.Register(spritesExec)
	log.Info("Sprites runtime registered")

	// Register SSH runtime (run an agent on any Linux box reachable over SSH).
	sshExec := lifecycle.NewSSHExecutor(secretStores.credentials, agentRegistry, agentctlResolver, log)
	executorRegistry.Register(sshExec)
	log.Info("SSH runtime registered")
	registerKubernetesBackend(executorRegistry, agentctlResolver, log)

	credsMgr := credentials.NewManager(log)
	if secretStores.credentials != nil {
		credsMgr.AddProvider(secrets.NewSecretStoreProvider(secretStores.credentials))
	}
	credsMgr.AddProvider(credentials.NewEnvProvider("KANDEV_"))
	credsMgr.AddProvider(credentials.NewAugmentSessionProvider())
	if credsFile := credentialFilePath(cfg); credsFile != "" {
		credsMgr.AddProvider(credentials.NewFileProvider(credsFile))
	}

	profileResolver := lifecycle.NewStoreProfileResolver(agentSettingsRepo, agentRegistry)
	mcpService := mcpconfig.NewService(agentSettingsRepo)

	lifecycleMgr := lifecycle.NewManager(
		agentRegistry,
		eventBus,
		executorRegistry,
		credsMgr,
		profileResolver,
		mcpService,
		lifecycle.ExecutorFallbackWarn,
		cfg.ResolvedHomeDir(),
		log,
	)
	configureMCPResolver(lifecycleMgr, agentSettingsRepo, mcpService, secretStores.runtime)
	if stateRepo, ok := agentSettingsRepo.(mcpconfig.SessionMCPSelectionStateRepository); ok {
		lifecycleMgr.SetMCPSelectionStateRepository(stateRepo)
	}
	importLegacyMCPProfiles(ctx, log, agentSettingsRepo, legacyWorkspaceSource, mcpService)
	registerMCPDeletionCleanup(eventBus, agentSettingsRepo, log)

	// Register environment preparers (keyed by ExecutorType — the
	// "local"/"worktree"/"local_docker"/"sprites" taxonomy, not Runtime).
	// The Worktree preparer is registered separately in
	// Manager.SetWorktreeManager once a worktree.Manager is wired.
	preparerRegistry := lifecycle.NewPreparerRegistry(log)
	localPreparer := lifecycle.NewLocalPreparer(log)
	preparerRegistry.Register(models.ExecutorTypeLocal, localPreparer)
	preparerRegistry.Register(models.ExecutorTypeMockRemote, localPreparer)
	preparerRegistry.Register(models.ExecutorTypeLocalDocker, lifecycle.NewDockerPreparer(log))
	preparerRegistry.Register(models.ExecutorTypeSprites, lifecycle.NewSpritesPreparer(log))
	preparerRegistry.Register(models.ExecutorTypeSSH, lifecycle.NewSSHPreparer(log))
	registerKubernetesPreparer(preparerRegistry, log)
	lifecycleMgr.SetPreparerRegistry(preparerRegistry)
	lifecycleMgr.SetSecretStore(secretStores.runtime)
	if err := lifecycleMgr.SetAgentctlStartupConfig(cfg.ManagedAgentctlStartupConfig()); err != nil {
		return nil, fmt.Errorf("configure managed agentctl startup: %w", err)
	}
	// Record the standalone agentctl control-server PID (populated by
	// provideAgentctlLauncher, which runs before this) so local/standalone
	// executor rows carry a real host-local liveness handle.
	lifecycleMgr.SetStandaloneHostPID(cfg.Agent.StandalonePID)
	// Wire the agent_profiles reader so the launch-prep skill deploy hook
	// (ADR 0005 Wave A) can resolve full profile rows including the office
	// enrichment fields. Without a wired SkillDeployer this is a no-op,
	// but the reader still lets future Wave-B/C consumers light up.
	lifecycleMgr.SetAgentProfileReader(agentSettingsRepo)
	lifecycleMgr.SetManagedRuntimeSelectionStore(managedRuntimeSelections)

	// MCP handler is set later in main.go after MCP handlers are registered
	// via lifecycleMgr.SetMCPHandler(gateway.Dispatcher)
	// Wire the base-branch provider before Start so recovered executions are
	// seeded during startup as well as newly-created executions.
	lifecycleMgr.SetBaseBranchProvider(baseBranchProvider)
	// Wire the comparison-target provider before Start so recovered executions
	// hydrate the exact provider-qualified ref before their first status poll.
	lifecycleMgr.SetComparisonTargetProvider(comparisonTargetProvider)
	// Recovered executions can dispatch MCP calls as soon as Start resumes
	// their streams. Install both trusted scopes before Start, not after route
	// registration, so the first recovered call has the same authority boundary
	// as every later call.
	if mcpIdentityScoper != nil {
		lifecycleMgr.SetMCPIdentityScoper(mcpIdentityScoper)
	}
	if mcpPrincipalScoper != nil {
		lifecycleMgr.SetMCPPrincipalScoper(mcpPrincipalScoper)
	}

	if err := lifecycleMgr.Start(ctx); err != nil {
		return nil, err
	}

	log.Info("Agent Manager initialized",
		zap.Int("runtimes", len(executorRegistry.List())),
		zap.Int("agent_types", len(agentRegistry.List())))
	return lifecycleMgr, nil
}

type mcpTaskDataCleaner interface {
	DeleteMCPTaskData(context.Context, string, []string) error
}

type mcpWorkspaceDataCleaner interface {
	DeleteMCPWorkspaceData(context.Context, string) error
}

func registerMCPDeletionCleanup(eventBus bus.EventBus, repo interface{}, log *logger.Logger) {
	if eventBus == nil || repo == nil {
		return
	}
	if cleaner, ok := repo.(mcpTaskDataCleaner); ok {
		if _, err := eventBus.Subscribe(events.TaskDeleted, func(ctx context.Context, event *bus.Event) error {
			taskID, sessionIDs := mcpTaskDeletionData(event)
			if taskID == "" {
				return nil
			}
			if err := cleaner.DeleteMCPTaskData(ctx, taskID, sessionIDs); err != nil {
				return fmt.Errorf("delete MCP task data: %w", err)
			}
			return nil
		}); err != nil {
			log.Error("failed to subscribe to MCP task deletion cleanup", zap.Error(err))
		}
	}
	if cleaner, ok := repo.(mcpWorkspaceDataCleaner); ok {
		if _, err := eventBus.Subscribe(events.WorkspaceDeleted, func(ctx context.Context, event *bus.Event) error {
			workspaceID := mcpEventString(event, "id")
			if workspaceID == "" {
				return nil
			}
			if err := cleaner.DeleteMCPWorkspaceData(ctx, workspaceID); err != nil {
				return fmt.Errorf("delete MCP workspace data: %w", err)
			}
			return nil
		}); err != nil {
			log.Error("failed to subscribe to MCP workspace deletion cleanup", zap.Error(err))
		}
	}
}

func mcpTaskDeletionData(event *bus.Event) (string, []string) {
	taskID := mcpEventString(event, "task_id")
	if event == nil {
		return taskID, nil
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return taskID, nil
	}
	return taskID, mcpEventStrings(data["mcp_session_ids"])
}

func mcpEventString(event *bus.Event, key string) string {
	if event == nil {
		return ""
	}
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := data[key].(string)
	return strings.TrimSpace(value)
}

func mcpEventStrings(value interface{}) []string {
	var values []string
	switch typed := value.(type) {
	case []string:
		values = typed
	case []interface{}:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	default:
		return nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

type legacyMCPWorkspaceSource interface {
	ListWorkspaces(context.Context) ([]*models.Workspace, error)
}

type legacyMCPWorkspaceLister struct {
	source   legacyMCPWorkspaceSource
	profiles settingsstore.Repository
}

func (l legacyMCPWorkspaceLister) ListMCPProfileWorkspaces(ctx context.Context, profileID string) ([]string, error) {
	if l.profiles != nil {
		profile, err := l.profiles.GetAgentProfile(ctx, profileID)
		if err != nil {
			return nil, err
		}
		if profile != nil && strings.TrimSpace(profile.WorkspaceID) != "" {
			return []string{profile.WorkspaceID}, nil
		}
	}
	workspaces, err := l.source.ListWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		if workspace != nil && strings.TrimSpace(workspace.ID) != "" {
			ids = append(ids, workspace.ID)
		}
	}
	return ids, nil
}

func importLegacyMCPProfiles(
	ctx context.Context,
	log *logger.Logger,
	agentSettingsRepo settingsstore.Repository,
	workspaceSource legacyMCPWorkspaceSource,
	mcpService *mcpconfig.Service,
) {
	if workspaceSource == nil || mcpService == nil {
		return
	}
	importer, ok := newLegacyMCPImporter(agentSettingsRepo, workspaceSource, mcpService)
	if !ok {
		return
	}
	agents, err := agentSettingsRepo.ListAgents(ctx)
	if err != nil {
		log.Warn("failed to list agents for legacy MCP import", zap.Error(err))
		return
	}
	for _, agent := range agents {
		if agent == nil || strings.TrimSpace(agent.ID) == "" {
			continue
		}
		importLegacyMCPAgentProfiles(ctx, log, agentSettingsRepo, mcpService, importer, agent.ID)
	}
}

func newLegacyMCPImporter(
	agentSettingsRepo settingsstore.Repository,
	workspaceSource legacyMCPWorkspaceSource,
	mcpService *mcpconfig.Service,
) (*mcpconfig.LegacyImporter, bool) {
	catalogRepo, catalogOK := agentSettingsRepo.(mcpconfig.CatalogRepository)
	selectionRepo, selectionOK := agentSettingsRepo.(mcpconfig.SelectionRepository)
	if !catalogOK || !selectionOK {
		return nil, false
	}
	catalog := mcpconfig.NewCatalogService(catalogRepo)
	catalog.SetSelectionRepository(selectionRepo)
	selections := mcpconfig.NewSelectionService(selectionRepo, catalogRepo)
	var states mcpconfig.LegacyImportStateRepository
	if stateRepo, ok := agentSettingsRepo.(mcpconfig.LegacyImportStateRepository); ok {
		states = stateRepo
	}
	return mcpconfig.NewLegacyImporter(
		mcpService,
		legacyMCPWorkspaceLister{source: workspaceSource, profiles: agentSettingsRepo},
		catalog,
		selections,
		states,
	), true
}

func importLegacyMCPAgentProfiles(
	ctx context.Context,
	log *logger.Logger,
	agentSettingsRepo settingsstore.Repository,
	mcpService *mcpconfig.Service,
	importer *mcpconfig.LegacyImporter,
	agentID string,
) {
	profiles, err := agentSettingsRepo.ListAgentProfiles(ctx, agentID)
	if err != nil {
		log.Warn("failed to list agent profiles for legacy MCP import",
			zap.String("agent_id", agentID), zap.Error(err))
		return
	}
	for _, profile := range profiles {
		if profile == nil || strings.TrimSpace(profile.ID) == "" {
			continue
		}
		if shouldSkipLegacyMCPProfile(ctx, log, mcpService, profile.ID) {
			continue
		}
		if _, err := importer.ImportProfile(ctx, profile.ID); err != nil {
			log.Warn("legacy MCP import requires attention",
				zap.String("profile_id", profile.ID), zap.Error(err))
		}
	}
}

func shouldSkipLegacyMCPProfile(
	ctx context.Context,
	log *logger.Logger,
	mcpService *mcpconfig.Service,
	profileID string,
) bool {
	if _, err := mcpService.GetConfigByProfileID(ctx, profileID); err == nil {
		return false
	} else if errors.Is(err, mcpconfig.ErrAgentMcpUnsupported) {
		return true
	} else {
		log.Warn("failed to read legacy MCP profile",
			zap.String("profile_id", profileID), zap.Error(err))
		return true
	}
}

func configureMCPResolver(
	lifecycleMgr *lifecycle.Manager,
	agentSettingsRepo settingsstore.Repository,
	mcpService *mcpconfig.Service,
	secretStore secrets.SecretStore,
) {
	catalogRepo, ok := agentSettingsRepo.(mcpconfig.CatalogRepository)
	if !ok {
		return
	}
	selectionRepo, ok := agentSettingsRepo.(mcpconfig.SelectionRepository)
	if !ok {
		return
	}
	mcpResolver := mcpconfig.NewResolver(catalogRepo, selectionRepo)
	var importStates mcpconfig.LegacyImportStateReader
	if stateRepo, ok := agentSettingsRepo.(mcpconfig.LegacyImportStateReader); ok {
		importStates = stateRepo
	}
	mcpResolver.SetLegacyProvider(mcpService, importStates)
	if secretStore != nil {
		mcpResolver.SetSecretResolver(mcpSecretResolver(secretStore))
	}
	lifecycleMgr.SetMCPResolver(mcpResolver)
}

func mcpSecretResolver(store secrets.SecretStore) mcpconfig.MCPSecretResolver {
	return func(ctx context.Context, secretID, workspaceID string) (string, error) {
		if scoped, ok := store.(secrets.ScopedSecretStore); ok {
			return scoped.RevealForWorkspace(ctx, secretID, workspaceID)
		}
		return store.Reveal(ctx, secretID)
	}
}

type lifecycleSecretStores struct {
	runtime     secrets.SecretStore
	credentials secrets.SecretStore
}

func newLifecycleSecretStores(raw secrets.SecretStore) lifecycleSecretStores {
	return lifecycleSecretStores{
		runtime:     raw,
		credentials: secrets.NewUserVisibleStore(raw),
	}
}

func registerKubernetesBackend(
	registry *lifecycle.ExecutorRegistry,
	agentctlResolver *lifecycle.AgentctlResolver,
	log *logger.Logger,
) {
	registry.Register(lifecycle.NewKubernetesExecutor(agentctlResolver, log))
	log.Info("Kubernetes runtime registered (lazy initialization)")
}

func registerKubernetesPreparer(registry *lifecycle.PreparerRegistry, log *logger.Logger) {
	registry.Register(models.ExecutorTypeKubernetes, lifecycle.NewKubernetesPreparer(log))
	log.Info("Kubernetes preparer registered")
}

func credentialFilePath(cfg *config.Config) string {
	if cfg != nil {
		return strings.TrimSpace(cfg.Credentials.File)
	}
	return strings.TrimSpace(os.Getenv("KANDEV_CREDENTIALS_FILE"))
}
