package onboarding

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

// Repository abstracts the persistence operations needed by OnboardingService.
type Repository interface {
	GetFirstCompletedOnboarding(ctx context.Context) (*sqlite.OnboardingState, error)
	MarkOnboardingComplete(ctx context.Context, workspaceID, ceoAgentID, firstTaskID string) error
	UpsertAgentRuntime(ctx context.Context, agentID, status, pauseReason string) error
	ListAgentInstances(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error)
	// UpsertWorkspaceRouting seeds the workspace routing config row
	// during onboarding so enabling routing later requires zero edits
	// to make the chosen CLI + model the default.
	UpsertWorkspaceRouting(ctx context.Context, workspaceID string, cfg *routing.WorkspaceConfig) error
	// UpdateAgentSettings persists the agent's settings JSON blob,
	// used by onboarding to write explicit routing.inherit markers on
	// the freshly created CEO agent.
	UpdateAgentSettings(ctx context.Context, agentID, settings string) error
}

// WorkspaceCreator creates a DB workspace row for kanban compatibility.
type WorkspaceCreator interface {
	CreateWorkspace(ctx context.Context, name, description string) error
	// FindWorkspaceIDByName returns the kanban workspace UUID for a given name,
	// or empty string if not found.
	FindWorkspaceIDByName(ctx context.Context, name string) (string, error)
	ListWorkspaceNames(ctx context.Context) ([]string, error)
}

// TaskCreator creates a task in the kanban system.
//
// CreateOfficeTask routes the task through the workspace's office workflow
// (workspaces.office_workflow_id). CreateOfficeTaskInWorkflow targets a
// specific workflow id explicitly — used by the routines dispatcher to
// pin tasks to the dedicated routine workflow.
type TaskCreator interface {
	CreateOfficeTask(ctx context.Context, workspaceID, projectID, assigneeAgentID, title, description string) (taskID string, err error)
	CreateOfficeTaskInWorkflow(ctx context.Context, workspaceID, projectID, assigneeAgentID, workflowID, title, description string) (taskID string, err error)
}

// AgentCreator creates a new agent instance with validation.
type AgentCreator interface {
	CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error
}

// CoordinatorRoutineInstaller installs the pre-baked coordinator-heartbeat
// routine for a freshly created coordinator agent. The routines service's
// CreateDefaultCoordinatorRoutine method satisfies this directly. Optional —
// when nil the onboarding flow skips routine install (useful for tests
// that don't construct the routines service).
type CoordinatorRoutineInstaller interface {
	CreateDefaultCoordinatorRoutine(ctx context.Context, workspaceID, agentID string) (*models.Routine, error)
}

// SourceProfileReader looks up an existing kanban CLI profile by id so
// onboarding can copy its CLI fields onto the new office agent.
type SourceProfileReader interface {
	GetAgentProfile(ctx context.Context, id string) (*models.AgentInstance, error)
}

// WorkflowEnsurer creates the system office workflows for a workspace.
//
// EnsureOfficeWorkflow materialises the office-default YAML workflow and
// stamps it onto workspaces.office_workflow_id so office tasks resolve to
// it by default. EnsureOfficeDefaultWorkflow materialises the office-default
// built-in workflow from embedded YAML.
type WorkflowEnsurer interface {
	EnsureOfficeWorkflow(ctx context.Context, workspaceID string) (string, error)
	EnsureOfficeDefaultWorkflow(ctx context.Context, workspaceID string) (string, error)
}

// ConfigSyncer applies filesystem config to the database.
type ConfigSyncer interface {
	ApplyIncoming(ctx context.Context, workspaceID string) (*ApplyResult, error)
}

// ApplyResult holds the counts of created and updated entities from a sync.
type ApplyResult struct {
	CreatedCount int
	UpdatedCount int
}

// RunReason for a newly assigned task.
const runReasonTaskAssigned = "task_assigned"

// OnboardingService provides onboarding state, completion, and FS import logic.
type OnboardingService struct {
	repo             Repository
	cfgLoader        *configloader.ConfigLoader
	cfgWriter        *configloader.FileWriter
	logger           *logger.Logger
	agents           shared.AgentReader
	sourceProfile    SourceProfileReader
	agentCreator     AgentCreator
	workspaceCreator WorkspaceCreator
	workflowEnsurer  WorkflowEnsurer
	taskCreator      TaskCreator
	runQueuer        shared.RunQueuer
	configSyncer     ConfigSyncer
	routineInstaller CoordinatorRoutineInstaller
}

// SetCoordinatorRoutineInstaller wires the routines-service hook used
// to install the default "Coordinator heartbeat" routine for a newly
// created coordinator agent. Called by the app composition layer after
// both services are constructed (the onboarding flow runs BEFORE
// routines exist on the first boot, so the wiring is post-build).
func (s *OnboardingService) SetCoordinatorRoutineInstaller(i CoordinatorRoutineInstaller) {
	s.routineInstaller = i
}

// NewOnboardingService creates a new OnboardingService.
func NewOnboardingService(
	repo Repository,
	cfgLoader *configloader.ConfigLoader,
	cfgWriter *configloader.FileWriter,
	log *logger.Logger,
	agents shared.AgentReader,
	sourceProfile SourceProfileReader,
	agentCreator AgentCreator,
	workspaceCreator WorkspaceCreator,
	workflowEnsurer WorkflowEnsurer,
	taskCreator TaskCreator,
	runQueuer shared.RunQueuer,
	configSyncer ConfigSyncer,
) *OnboardingService {
	return &OnboardingService{
		repo:             repo,
		cfgLoader:        cfgLoader,
		cfgWriter:        cfgWriter,
		logger:           log.WithFields(zap.String("component", "office-onboarding")),
		agents:           agents,
		sourceProfile:    sourceProfile,
		agentCreator:     agentCreator,
		workspaceCreator: workspaceCreator,
		workflowEnsurer:  workflowEnsurer,
		taskCreator:      taskCreator,
		runQueuer:        runQueuer,
		configSyncer:     configSyncer,
	}
}

// FSWorkspace represents a workspace found on the filesystem.
type FSWorkspace struct {
	Name string `json:"name"`
}

// OnboardingState holds the current onboarding state.
type OnboardingState struct {
	Completed    bool          `json:"completed"`
	WorkspaceID  string        `json:"workspaceId,omitempty"`
	CEOAgentID   string        `json:"ceoAgentId,omitempty"`
	FSWorkspaces []FSWorkspace `json:"fsWorkspaces"`
}

// CompleteRequest holds the inputs for completing onboarding.
type CompleteRequest struct {
	WorkspaceName      string
	TaskPrefix         string
	AgentName          string
	AgentProfileID     string
	ExecutorPreference string
	TaskTitle          string
	TaskDescription    string
	// DefaultTier is the workspace routing default tier captured by the
	// onboarding wizard. Empty / unknown values fall back to balanced.
	DefaultTier string
}

// CompleteResult holds the IDs of entities created during onboarding.
type CompleteResult struct {
	WorkspaceID string
	AgentID     string
	TaskID      string
}

// ImportFromFSResult holds the result of importing FS workspaces.
type ImportFromFSResult struct {
	WorkspaceIDs  []string
	ImportedCount int
}

// GetOnboardingState checks whether onboarding has been completed.
// Always scans the filesystem for unimported workspaces so the mode=new
// import prompt works even after initial onboarding is complete.
func (s *OnboardingService) GetOnboardingState(ctx context.Context) (*OnboardingState, error) {
	row, err := s.repo.GetFirstCompletedOnboarding(ctx)
	if err != nil {
		return nil, fmt.Errorf("check onboarding state: %w", err)
	}

	fsWorkspaces := s.unimportedFSWorkspaces(ctx)

	if row != nil {
		return &OnboardingState{
			Completed:    true,
			WorkspaceID:  row.WorkspaceID,
			CEOAgentID:   row.CEOAgentID,
			FSWorkspaces: fsWorkspaces,
		}, nil
	}
	return &OnboardingState{Completed: false, FSWorkspaces: fsWorkspaces}, nil
}

// unimportedFSWorkspaces returns filesystem workspaces that don't have a
// matching DB row. Used by the setup page to show an import prompt.
func (s *OnboardingService) unimportedFSWorkspaces(ctx context.Context) []FSWorkspace {
	if s.cfgLoader == nil || s.workspaceCreator == nil {
		return []FSWorkspace{}
	}
	dbNames, err := s.workspaceCreator.ListWorkspaceNames(ctx)
	if err != nil {
		s.logger.Warn("failed to list workspace names for FS filtering", zap.Error(err))
		dbNames = nil
	}
	dbSet := make(map[string]struct{}, len(dbNames))
	for _, n := range dbNames {
		dbSet[n] = struct{}{}
	}
	var out []FSWorkspace
	for _, ws := range s.cfgLoader.GetWorkspaces() {
		if _, imported := dbSet[ws.Name]; !imported {
			out = append(out, FSWorkspace{Name: ws.Name})
		}
	}
	if out == nil {
		out = []FSWorkspace{}
	}
	return out
}

// CompleteOnboarding creates workspace, CEO agent, project, optional task,
// and marks onboarding as finished.
func (s *OnboardingService) CompleteOnboarding(ctx context.Context, req CompleteRequest) (*CompleteResult, error) {
	result := &CompleteResult{}

	if err := s.createOnboardingWorkspace(ctx, req.WorkspaceName, req.TaskPrefix); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	wsID, err := s.resolveKanbanWorkspaceID(ctx, req.WorkspaceName)
	if err != nil || wsID == "" {
		return nil, fmt.Errorf("workspace ID not found after creation")
	}
	result.WorkspaceID = wsID

	// Phase 6 (ADR-0004) — materialise the built-in office-default workflow
	// from embedded YAML and stamp it onto workspaces.office_workflow_id so
	// office tasks land on the templated workflow by default. Idempotent.
	s.ensureBuiltinWorkflows(ctx, wsID)

	agentID, err := s.createOnboardingAgent(ctx, wsID, req)
	if err != nil {
		return nil, fmt.Errorf("create CEO agent: %w", err)
	}
	result.AgentID = agentID

	if rtErr := s.repo.UpsertAgentRuntime(ctx, agentID, string(models.AgentStatusIdle), ""); rtErr != nil {
		s.logger.Warn("create agent runtime failed", zap.Error(rtErr))
	}

	// Seed workspace routing config + CEO inherit markers so enabling
	// routing later requires zero edits to make the chosen CLI + model
	// the workspace default. Failures are warn-logged and ignored —
	// onboarding must succeed even if routing seed misbehaves.
	s.seedWorkspaceRouting(ctx, wsID, agentID, req)

	result.TaskID = s.maybeCreateOnboardingTask(ctx, wsID, agentID, req)

	if err := s.repo.MarkOnboardingComplete(ctx, wsID, agentID, ""); err != nil {
		return nil, fmt.Errorf("mark onboarding complete: %w", err)
	}

	s.logger.Info("onboarding completed",
		zap.String("workspace_id", wsID),
		zap.String("agent_id", agentID),
		zap.String("task_id", result.TaskID))
	return result, nil
}

// ensureBuiltinWorkflows materialises the office-default workflow for a
// workspace and stamps it onto workspaces.office_workflow_id so office
// tasks resolve to it by default. Errors are logged and ignored — a
// workspace still completes onboarding even when the call fails.
func (s *OnboardingService) ensureBuiltinWorkflows(ctx context.Context, wsID string) {
	if s.workflowEnsurer == nil {
		return
	}
	if _, err := s.workflowEnsurer.EnsureOfficeWorkflow(ctx, wsID); err != nil {
		s.logger.Warn("ensure office workflow failed",
			zap.String("workspace_id", wsID), zap.Error(err))
	}
	if _, err := s.workflowEnsurer.EnsureOfficeDefaultWorkflow(ctx, wsID); err != nil {
		s.logger.Warn("create office-default workflow failed",
			zap.String("workspace_id", wsID), zap.Error(err))
	}
}

// maybeCreateOnboardingTask creates a task and queues a run if configured.
// Returns the created task ID, or empty string on skip or error. The task
// is created without a project — projects are created on demand by the user
// or the coordinator agent.
func (s *OnboardingService) maybeCreateOnboardingTask(
	ctx context.Context, wsID, agentID string, req CompleteRequest,
) string {
	if req.TaskTitle == "" || s.taskCreator == nil {
		return ""
	}
	taskID, err := s.taskCreator.CreateOfficeTask(ctx, wsID, "", agentID, req.TaskTitle, req.TaskDescription)
	if err != nil {
		s.logger.Warn("create onboarding task failed", zap.Error(err))
		return ""
	}
	if s.runQueuer != nil {
		if wakeErr := s.runQueuer.QueueRun(ctx, agentID, runReasonTaskAssigned,
			fmt.Sprintf(`{"task_id":%q}`, taskID), ""); wakeErr != nil {
			s.logger.Warn("enqueue onboarding run failed", zap.Error(wakeErr))
		}
	}
	return taskID
}

// ImportFromFS creates DB workspace rows for each FS workspace,
// imports all config entities, and marks onboarding complete.
func (s *OnboardingService) ImportFromFS(ctx context.Context) (*ImportFromFSResult, error) {
	if s.cfgLoader == nil {
		return nil, fmt.Errorf("config loader not initialized")
	}
	fsWorkspaces := s.cfgLoader.GetWorkspaces()
	if len(fsWorkspaces) == 0 {
		return nil, fmt.Errorf("no workspaces found on filesystem")
	}

	dbSet := s.buildExistingWorkspaceSet(ctx)
	result := &ImportFromFSResult{}
	var firstWSID, firstAgentID string

	for _, ws := range fsWorkspaces {
		if _, imported := dbSet[ws.Name]; imported {
			continue
		}
		wsID, imported := s.importSingleWorkspace(ctx, ws, result)
		if !imported {
			continue
		}
		if firstWSID == "" {
			firstWSID = wsID
			firstAgentID = s.findCEOAgentID(ctx, wsID)
		}
	}

	if firstWSID != "" {
		if err := s.repo.MarkOnboardingComplete(ctx, firstWSID, firstAgentID, ""); err != nil {
			return nil, fmt.Errorf("mark onboarding complete: %w", err)
		}
	}

	s.logger.Info("FS import onboarding completed",
		zap.Int("workspaces", len(result.WorkspaceIDs)),
		zap.Int("imported", result.ImportedCount))
	return result, nil
}

// buildExistingWorkspaceSet returns a set of workspace names already in the DB.
func (s *OnboardingService) buildExistingWorkspaceSet(ctx context.Context) map[string]struct{} {
	var dbNames []string
	if s.workspaceCreator != nil {
		var err error
		dbNames, err = s.workspaceCreator.ListWorkspaceNames(ctx)
		if err != nil {
			s.logger.Warn("failed to list workspace names for import filtering", zap.Error(err))
		}
	}
	dbSet := make(map[string]struct{}, len(dbNames))
	for _, n := range dbNames {
		dbSet[n] = struct{}{}
	}
	return dbSet
}

// importSingleWorkspace creates the DB row, syncs config, and appends to result.
// It returns the workspace ID and true if the workspace was successfully imported.
func (s *OnboardingService) importSingleWorkspace(
	ctx context.Context, ws configloader.WorkspaceConfig, result *ImportFromFSResult,
) (string, bool) {
	if s.workspaceCreator != nil {
		if err := s.workspaceCreator.CreateWorkspace(ctx, ws.Name, ws.Settings.Description); err != nil {
			s.logger.Warn("DB workspace creation failed during FS import",
				zap.String("name", ws.Name), zap.Error(err))
		}
	}
	wsID, err := s.resolveKanbanWorkspaceID(ctx, ws.Name)
	if err != nil || wsID == "" {
		s.logger.Warn("could not resolve workspace ID after creation",
			zap.String("name", ws.Name), zap.Error(err))
		return "", false
	}
	result.WorkspaceIDs = append(result.WorkspaceIDs, wsID)

	if s.configSyncer != nil {
		importResult, syncErr := s.configSyncer.ApplyIncoming(ctx, wsID)
		if syncErr != nil {
			s.logger.Warn("config import failed for workspace",
				zap.String("workspace", ws.Name), zap.Error(syncErr))
			return "", false
		}
		result.ImportedCount += importResult.CreatedCount + importResult.UpdatedCount
	}
	return wsID, true
}

// findCEOAgentID returns the ID of the CEO agent in the workspace, or empty string.
func (s *OnboardingService) findCEOAgentID(ctx context.Context, wsID string) string {
	agents, _ := s.repo.ListAgentInstances(ctx, wsID)
	for _, a := range agents {
		if a.Role == models.AgentRoleCEO {
			return a.ID
		}
	}
	return ""
}

func (s *OnboardingService) createOnboardingWorkspace(ctx context.Context, name, taskPrefix string) error {
	slug := generateSlug(name)
	if s.cfgWriter != nil {
		settings := &configloader.WorkspaceSettings{
			Name:       name,
			Slug:       slug,
			TaskPrefix: taskPrefix,
		}
		if err := s.writeWorkspaceConfig(name, settings); err != nil {
			return err
		}
	}
	if s.workspaceCreator != nil {
		if err := s.workspaceCreator.CreateWorkspace(ctx, name, "Office workspace"); err != nil {
			s.logger.Warn("DB workspace creation failed",
				zap.String("name", name), zap.Error(err))
		}
	}
	return nil
}

func (s *OnboardingService) writeWorkspaceConfig(name string, settings *configloader.WorkspaceSettings) error {
	if !isValidPathComponent(name) {
		return fmt.Errorf("invalid workspace name")
	}
	data, err := configloader.MarshalSettings(*settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	wsDir := filepath.Join(s.cfgLoader.BasePath(), "workspaces", name)
	if mkErr := os.MkdirAll(wsDir, 0o755); mkErr != nil {
		return fmt.Errorf("create dir: %w", mkErr)
	}
	settingsPath := filepath.Join(wsDir, "kandev.yml")
	if writeErr := os.WriteFile(settingsPath, data, 0o644); writeErr != nil {
		return fmt.Errorf("write settings: %w", writeErr)
	}
	if reloadErr := s.cfgLoader.Reload(name); reloadErr != nil {
		return fmt.Errorf("reload config: %w", reloadErr)
	}
	return nil
}

var (
	slugNonAlphanumRe = regexp.MustCompile(`[^a-z0-9-]`)
	slugMultiDashRe   = regexp.MustCompile(`-+`)
)

func generateSlug(name string) string {
	slug := strings.ToLower(name)
	slug = slugNonAlphanumRe.ReplaceAllString(slug, "-")
	slug = slugMultiDashRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	if len(slug) > 50 {
		slug = slug[:50]
	}
	return slug
}

func (s *OnboardingService) resolveKanbanWorkspaceID(ctx context.Context, name string) (string, error) {
	if s.workspaceCreator != nil {
		id, err := s.workspaceCreator.FindWorkspaceIDByName(ctx, name)
		if err != nil {
			return "", err
		}
		if id != "" {
			return id, nil
		}
	}
	return name, nil
}

func (s *OnboardingService) createOnboardingAgent(ctx context.Context, wsID string, req CompleteRequest) (string, error) {
	execPref := "{}"
	if req.ExecutorPreference != "" {
		execPref = fmt.Sprintf(`{"type":%q}`, req.ExecutorPreference)
	}
	// The office agent gets a fresh row in agent_profiles. When the user
	// picked an existing kanban CLI profile (req.AgentProfileID), copy its
	// CLI fields (agent_id, model, mode, permission flags) onto the new
	// office row — agent_profiles.agent_id is FK to agents and may not be
	// empty. Reusing the source id would collide with the existing row.
	agent := &models.AgentInstance{
		ID:                 uuid.New().String(),
		WorkspaceID:        wsID,
		Name:               req.AgentName,
		Role:               models.AgentRoleCEO,
		Status:             models.AgentStatusIdle,
		AllowIndexing:      true,
		Permissions:        shared.DefaultPermissions(shared.AgentRoleCEO),
		ExecutorPreference: execPref,
	}
	if req.AgentProfileID != "" && s.sourceProfile != nil {
		src, err := s.sourceProfile.GetAgentProfile(ctx, req.AgentProfileID)
		if err != nil {
			return "", fmt.Errorf("look up source profile %s: %w", req.AgentProfileID, err)
		}
		agent.AgentID = src.AgentID
		agent.AgentDisplayName = src.AgentDisplayName
		agent.Model = src.Model
		agent.Mode = src.Mode
		agent.AutoApprove = src.AutoApprove
		agent.DangerouslySkipPermissions = src.DangerouslySkipPermissions
		agent.AllowIndexing = src.AllowIndexing
		agent.CLIPassthrough = src.CLIPassthrough
	}
	if err := s.agentCreator.CreateAgentInstance(ctx, agent); err != nil {
		return "", err
	}
	s.installCoordinatorRoutine(ctx, wsID, agent.ID, agent.Role)
	return agent.ID, nil
}

// seedWorkspaceRouting writes the workspace routing config row + the
// CEO agent's inherit markers. It looks the freshly created CEO agent
// up by ID so the chosen CLI + model end up in the routing seed even
// though the wizard only forwards an opaque AgentProfileID. Failures
// are warn-logged: onboarding must succeed even if seeding fails.
func (s *OnboardingService) seedWorkspaceRouting(
	ctx context.Context, workspaceID, agentID string, req CompleteRequest,
) {
	if s.repo == nil {
		return
	}
	tier := normalizeDefaultTier(req.DefaultTier)
	agent, err := s.lookupOnboardingAgent(ctx, workspaceID, agentID)
	if err != nil {
		s.logger.Warn("routing seed: lookup CEO agent failed",
			zap.String("workspace_id", workspaceID),
			zap.String("agent_id", agentID), zap.Error(err))
		return
	}
	cfg := buildOnboardingRoutingConfig(tier, agent)
	if err := s.repo.UpsertWorkspaceRouting(ctx, workspaceID, cfg); err != nil {
		s.logger.Warn("routing seed: upsert workspace routing failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
	if err := s.writeAgentInheritMarkers(ctx, agentID); err != nil {
		s.logger.Warn("routing seed: write CEO inherit markers failed",
			zap.String("agent_id", agentID), zap.Error(err))
	}
}

// normalizeDefaultTier maps a free-text wizard value onto a valid
// routing.Tier. Empty / unknown values silently fall back to balanced
// so the wizard never fails because of a typo.
func normalizeDefaultTier(raw string) routing.Tier {
	switch routing.Tier(raw) {
	case routing.TierFrontier, routing.TierBalanced, routing.TierEconomy:
		return routing.Tier(raw)
	}
	return routing.TierBalanced
}

// lookupOnboardingAgent fetches the freshly created CEO agent by ID so
// the routing seed can pull its agent_id / model / mode without
// re-threading those values from createOnboardingAgent.
func (s *OnboardingService) lookupOnboardingAgent(
	ctx context.Context, workspaceID, agentID string,
) (*models.AgentInstance, error) {
	agents, err := s.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, a := range agents {
		if a != nil && a.ID == agentID {
			return a, nil
		}
	}
	return nil, fmt.Errorf("agent %s not found in workspace %s", agentID, workspaceID)
}

// buildOnboardingRoutingConfig produces the disabled-by-default routing
// seed for a workspace. The chosen tier is always persisted so the
// Workspace → Provider routing settings page lands on the right
// default. When the CEO agent has a concrete provider/model attached,
// that pairing is also seeded into the order + tier_map so enabling
// routing later requires zero edits to make the chosen CLI the default.
func buildOnboardingRoutingConfig(tier routing.Tier, agent *models.AgentInstance) *routing.WorkspaceConfig {
	cfg := &routing.WorkspaceConfig{
		Enabled:          false,
		DefaultTier:      tier,
		ProviderOrder:    []routing.ProviderID{},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{},
		// Seed sensible defaults so heartbeats / scheduled routines /
		// budget-alert wake-ups cheap out automatically the moment the
		// user enables routing. Mirrors the legacy cheap_profile policy
		// that the wake-reason tier policy replaces.
		TierPerReason: routing.TierPerReason{
			routing.WakeReasonHeartbeat:      routing.TierEconomy,
			routing.WakeReasonRoutineTrigger: routing.TierEconomy,
			routing.WakeReasonBudgetAlert:    routing.TierEconomy,
		},
	}
	if agent == nil || agent.AgentID == "" {
		return cfg
	}
	providerID := routing.ProviderID(agent.AgentID)
	tierMap := routing.TierMap{}
	switch tier {
	case routing.TierFrontier:
		tierMap.Frontier = agent.Model
	case routing.TierEconomy:
		tierMap.Economy = agent.Model
	default:
		tierMap.Balanced = agent.Model
	}
	cfg.ProviderOrder = []routing.ProviderID{providerID}
	cfg.ProviderProfiles[providerID] = routing.ProviderProfile{
		TierMap: tierMap, Mode: agent.Mode,
	}
	return cfg
}

// writeAgentInheritMarkers stamps explicit routing.tier_source = inherit
// and provider_order_source = inherit onto the CEO agent's settings JSON.
// The values are literal "inherit" strings (the resolver/validator treat
// any non-"override" value as inherit) so the routing UI can show the
// agent's inheritance state without inferring from a missing key.
func (s *OnboardingService) writeAgentInheritMarkers(
	ctx context.Context, agentID string,
) error {
	overrides := routing.AgentOverrides{
		ProviderOrderSource: "inherit",
		TierSource:          "inherit",
	}
	settings, err := routing.WriteAgentOverrides("", overrides)
	if err != nil {
		return fmt.Errorf("marshal overrides: %w", err)
	}
	return s.repo.UpdateAgentSettings(ctx, agentID, settings)
}

// installCoordinatorRoutine triggers the default coordinator-heartbeat
// routine install for CEO/coordinator agents. Failures are warn-logged
// and ignored — the agent is still created successfully and the user
// can install a routine manually from the UI later.
func (s *OnboardingService) installCoordinatorRoutine(
	ctx context.Context, workspaceID, agentID string, role models.AgentRole,
) {
	if s.routineInstaller == nil {
		return
	}
	if role != models.AgentRoleCEO {
		return
	}
	if _, err := s.routineInstaller.CreateDefaultCoordinatorRoutine(ctx, workspaceID, agentID); err != nil {
		s.logger.Warn("install default coordinator routine",
			zap.String("workspace_id", workspaceID),
			zap.String("agent_id", agentID),
			zap.Error(err))
	}
}

func isValidPathComponent(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !strings.Contains(s, "\\") && !strings.Contains(s, "..")
}
