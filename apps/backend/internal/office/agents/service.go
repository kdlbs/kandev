package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/shared"
	officeskills "github.com/kandev/kandev/internal/office/skills"

	"go.uber.org/zap"
)

// UsageProvider fetches subscription utilization for an agent profile.
// The provider is optional — if nil, utilization is always nil.
type UsageProvider interface {
	GetUsage(ctx context.Context, profileID string) (*agentusage.ProviderUsage, error)
}

// Sentinel errors for agent validation.
var (
	ErrAgentNameRequired     = errors.New("agent name is required")
	ErrAgentRoleInvalid      = errors.New("invalid agent role")
	ErrAgentCEOAlreadyExists = errors.New("workspace already has a CEO agent")
	ErrAgentReportsToInvalid = errors.New("reports_to agent does not exist in this workspace")
	ErrAgentReportsToSelf    = errors.New("agent cannot report to itself")
	ErrAgentStatusTransition = errors.New("invalid status transition")
)

// GovernanceSettingsReader reads workspace governance settings.
type GovernanceSettingsReader interface {
	GetRequireApprovalForNewAgents(ctx context.Context, workspaceID string) (bool, error)
}

// GovernanceApprovalCreator creates approval records for governance-gated actions.
type GovernanceApprovalCreator interface {
	CreateApprovalWithActivity(ctx context.Context, approval *models.Approval) error
}

// SessionTerminator cascades termination of an agent's office task sessions
// when the agent instance is deleted at the workspace level. Optional —
// when nil, deletion proceeds without flipping any session rows (legacy).
type SessionTerminator interface {
	TerminateAllForAgent(ctx context.Context, agentInstanceID, reason string) error
}

// CoordinatorRoutineInstaller installs the pre-baked coordinator-heartbeat
// routine for a freshly created coordinator agent. Optional — when nil
// the agent-create path skips routine install (e.g. tests that don't
// construct the routines service).
type CoordinatorRoutineInstaller interface {
	CreateDefaultCoordinatorRoutine(ctx context.Context, workspaceID, agentID string) (*models.Routine, error)
}

var validRoles = map[models.AgentRole]bool{
	models.AgentRoleCEO:        true,
	models.AgentRoleWorker:     true,
	models.AgentRoleSpecialist: true,
	models.AgentRoleAssistant:  true,
	models.AgentRoleSecurity:   true,
	models.AgentRoleQA:         true,
	models.AgentRoleDevOps:     true,
}

// allowedTransitions defines which status transitions are valid.
var allowedTransitions = map[models.AgentStatus][]models.AgentStatus{
	models.AgentStatusIdle:            {models.AgentStatusWorking, models.AgentStatusPaused, models.AgentStatusStopped, models.AgentStatusPendingApproval},
	models.AgentStatusWorking:         {models.AgentStatusIdle, models.AgentStatusPaused, models.AgentStatusStopped},
	models.AgentStatusPaused:          {models.AgentStatusIdle, models.AgentStatusStopped},
	models.AgentStatusStopped:         {models.AgentStatusIdle},
	models.AgentStatusPendingApproval: {models.AgentStatusIdle, models.AgentStatusStopped},
}

// AgentListFilter specifies optional filters for listing agents.
type AgentListFilter struct {
	Role      string
	Status    string
	ReportsTo string
}

// AgentService provides agent instance CRUD, status management, and related operations.
type AgentService struct {
	repo               *sqlite.Repository
	logger             *logger.Logger
	activity           shared.ActivityLogger
	auth               *AgentAuth
	cfgWriter          *configloader.FileWriter
	usageProvider      UsageProvider
	governanceSettings GovernanceSettingsReader
	governanceApproval GovernanceApprovalCreator
	sessionTerm        SessionTerminator
	routineInstaller   CoordinatorRoutineInstaller
	knownProvidersFn   func() []routing.ProviderID
}

// SetKnownProvidersFn wires the source of routing-eligible provider IDs
// used by PATCH /agents/:id when validating an AgentOverrides blob.
// Optional; when unset, the handler falls back to routing.KnownProviders(nil)
// which returns the static v1 allow-list.
func (s *AgentService) SetKnownProvidersFn(fn func() []routing.ProviderID) {
	s.knownProvidersFn = fn
}

// KnownProviders returns the routing-eligible provider IDs the agents
// service uses for override validation.
func (s *AgentService) KnownProviders() []routing.ProviderID {
	if s.knownProvidersFn != nil {
		return s.knownProvidersFn()
	}
	return routing.KnownProviders(nil)
}

// GetWorkspaceRouting returns the workspace's routing config so the
// handler can cross-check an agent's overrides against the workspace
// tier map at save time. Delegates to the office repo's defaults-aware
// reader, so callers never need to special-case the "never configured"
// workspace.
func (s *AgentService) GetWorkspaceRouting(
	ctx context.Context, workspaceID string,
) (*routing.WorkspaceConfig, error) {
	return s.repo.GetWorkspaceRouting(ctx, workspaceID)
}

// SetCoordinatorRoutineInstaller wires the routines-service hook used
// to install the default "Coordinator heartbeat" routine for a newly
// created coordinator agent. Called by the app composition layer after
// both services are constructed.
func (s *AgentService) SetCoordinatorRoutineInstaller(i CoordinatorRoutineInstaller) {
	s.routineInstaller = i
}

// SetSessionTerminator wires the office session terminator. Called by the
// app composition layer after both services are constructed (the orchestrator
// owns the terminator implementation; the agents service consumes it).
func (s *AgentService) SetSessionTerminator(t SessionTerminator) {
	s.sessionTerm = t
}

// NewAgentService creates a new AgentService.
func NewAgentService(repo *sqlite.Repository, log *logger.Logger, activity shared.ActivityLogger) *AgentService {
	return &AgentService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "office-agents")),
		activity: activity,
	}
}

// SetUsageProvider wires in a subscription usage provider. Optional — if not set,
// utilization is always nil in responses.
func (s *AgentService) SetUsageProvider(p UsageProvider) {
	s.usageProvider = p
}

// SetGovernanceSettings wires in a governance settings reader for approval checks.
func (s *AgentService) SetGovernanceSettings(g GovernanceSettingsReader) {
	s.governanceSettings = g
}

// SetGovernanceApproval wires in an approval creator for governance-gated agent creation.
func (s *AgentService) SetGovernanceApproval(g GovernanceApprovalCreator) {
	s.governanceApproval = g
}

// GetAgentUtilization returns the current subscription utilization for the agent,
// or nil if the agent is not a subscription agent or no usage provider is configured.
func (s *AgentService) GetAgentUtilization(ctx context.Context, agentInstanceID string) (*agentusage.ProviderUsage, error) {
	if s.usageProvider == nil {
		return nil, nil
	}
	agent, err := s.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return nil, err
	}
	// Wave G: agent_profiles.id IS the profile id, exposed via AgentInstance.ID.
	if agent.ID == "" {
		return nil, nil
	}
	return s.usageProvider.GetUsage(ctx, agent.ID)
}

// GetAgentInstance looks up an agent by ID or name.
// Implements shared.AgentReader.
func (s *AgentService) GetAgentInstance(ctx context.Context, idOrName string) (*models.AgentInstance, error) {
	return s.GetAgentFromConfig(ctx, idOrName)
}

// ListAgentInstances returns all agent instances for a workspace.
// Implements shared.AgentReader.
func (s *AgentService) ListAgentInstances(ctx context.Context, wsID string) ([]*models.AgentInstance, error) {
	return s.ListAgentsFromConfig(ctx, wsID)
}

// ListAgentInstancesByIDs returns agent instances whose ids are in `ids`.
// Implements shared.AgentReader.
func (s *AgentService) ListAgentInstancesByIDs(ctx context.Context, ids []string) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstancesByIDs(ctx, ids)
}

// UpdateAgentStatusFields persists a new status and optional pause reason for an agent.
// Implements shared.AgentWriter.
func (s *AgentService) UpdateAgentStatusFields(ctx context.Context, agentID, status, pauseReason string) error {
	return s.repo.UpdateAgentStatusFields(ctx, agentID, status, pauseReason)
}

// GetAgentFromConfig looks up an agent by ID or name.
func (s *AgentService) GetAgentFromConfig(ctx context.Context, idOrName string) (*models.AgentInstance, error) {
	if agent, err := s.repo.GetAgentInstance(ctx, idOrName); err == nil {
		return agent, nil
	}
	agent, err := s.repo.GetAgentInstanceByNameAny(ctx, idOrName)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %s", idOrName)
	}
	return agent, nil
}

// ListAgentsFromConfig returns all agent instances for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *AgentService) ListAgentsFromConfig(ctx context.Context, workspaceID string) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstances(ctx, workspaceID)
}

// CreateAgentInstance validates and creates a new agent instance in the DB.
func (s *AgentService) CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentCreate(ctx, agent); err != nil {
		return err
	}
	s.prepareAgentDefaults(agent)
	s.seedDefaultSkills(ctx, agent)
	return s.persistAgent(ctx, agent)
}

// CreateAgentInstanceWithCaller validates governance and creates a new agent instance.
// If governance requires approval for new agents, it creates a hire_agent approval record
// after persisting the new instance in pending_approval status.
func (s *AgentService) CreateAgentInstanceWithCaller(
	ctx context.Context,
	agent *models.AgentInstance,
	callerAgent *models.AgentInstance,
	reason string,
) error {
	if err := s.validateAgentCreate(ctx, agent); err != nil {
		return err
	}
	s.prepareAgentDefaults(agent)
	s.seedDefaultSkills(ctx, agent)

	if s.requiresHireApproval(ctx, agent.WorkspaceID, callerAgent) {
		agent.Status = models.AgentStatusPendingApproval
		if err := s.persistAgent(ctx, agent); err != nil {
			return err
		}
		return s.createHireApproval(ctx, agent, callerAgent, reason)
	}
	return s.persistAgent(ctx, agent)
}

func (s *AgentService) prepareAgentDefaults(agent *models.AgentInstance) {
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}
	if agent.Permissions == "" || agent.Permissions == "{}" {
		agent.Permissions = shared.DefaultPermissions(shared.AgentRole(agent.Role))
	}
	if agent.MaxConcurrentSessions < 1 {
		agent.MaxConcurrentSessions = 1
	}
	if agent.CooldownSec <= 0 {
		agent.CooldownSec = 10
	}
	if agent.DesiredSkills == "" {
		agent.DesiredSkills = "[]"
	}
	if agent.ExecutorPreference == "" {
		agent.ExecutorPreference = "{}"
	}
	if agent.Status == "" {
		agent.Status = models.AgentStatusIdle
	}
}

// BackfillDefaultSkillsForWorkspace seeds default system skills onto
// every existing agent in `workspaceID` whose `desired_skills` is
// currently empty (`""` / `"[]"`). Use case: an agent that pre-dates
// the system-skill rollout boots with no skills attached even though
// matching defaults exist. Runs once at startup per workspace
// (called from the office service init path), and again whenever
// `SyncSystemSkills` reports inserts / removes for a workspace.
//
// Agents with any populated skill list are left alone — that array
// represents the user's curated choice, including the deliberate
// choice of "no system skills".
func (s *AgentService) BackfillDefaultSkillsForWorkspace(ctx context.Context, workspaceID string) {
	if workspaceID == "" {
		return
	}
	// Ensure bundled system skills are present in this workspace before
	// reading them. A workspace created after backend startup (e.g. via
	// onboarding) skips the boot-time sync; the agent backfill must not
	// silently noop on it.
	if _, err := officeskills.SyncSystemSkills(
		ctx, s.repo, []string{workspaceID}, nil, s.logger,
	); err != nil {
		s.logger.Warn("backfill default skills: ensure system skills",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
	systemSkills, err := s.repo.ListSystemSkills(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("backfill default skills: list system skills",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return
	}
	if len(systemSkills) == 0 {
		return
	}
	agents, err := s.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		s.logger.Warn("backfill default skills: list agents",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return
	}
	var updated int
	for _, agent := range agents {
		if !needsDefaultSkillSeed(agent.DesiredSkills) {
			continue
		}
		picks := pickDefaultSkillsForRole(systemSkills, string(agent.Role))
		if len(picks.IDs) == 0 {
			continue
		}
		if err := applyDefaultSkillPicks(agent, picks); err != nil {
			s.logger.Warn("backfill default skills: encode ids", zap.Error(err))
			continue
		}
		if err := s.repo.UpdateAgentInstance(ctx, agent); err != nil {
			s.logger.Warn("backfill default skills: persist",
				zap.String("agent_id", agent.ID), zap.Error(err))
			continue
		}
		updated++
	}
	if updated > 0 {
		s.logger.Info("backfilled default skills",
			zap.String("workspace_id", workspaceID),
			zap.Int("agents_updated", updated))
	}
}

// seedDefaultSkills auto-attaches every system skill whose
// `default_for_roles` includes this agent's role. Runs only when
// the caller hasn't already populated DesiredSkills (so explicit
// API requests with their own skill set are respected). Failures
// are warn-logged: a fresh workspace might not have system skills
// synced yet, but missing defaults shouldn't block agent creation.
func (s *AgentService) seedDefaultSkills(ctx context.Context, agent *models.AgentInstance) {
	if !needsDefaultSkillSeed(agent.DesiredSkills) {
		return
	}
	// Lazy sync mirrors the SkillService.ListSkillsFromConfig path so
	// onboarding-created workspaces — which the boot-time sync hasn't
	// touched — still surface the bundled set before we read it.
	if _, err := officeskills.SyncSystemSkills(
		ctx, s.repo, []string{agent.WorkspaceID}, nil, s.logger,
	); err != nil {
		s.logger.Warn("seed default skills: ensure system skills",
			zap.String("workspace_id", agent.WorkspaceID), zap.Error(err))
	}
	systemSkills, err := s.repo.ListSystemSkills(ctx, agent.WorkspaceID)
	if err != nil {
		s.logger.Warn("list system skills for default seed",
			zap.String("workspace_id", agent.WorkspaceID), zap.Error(err))
		return
	}
	picks := pickDefaultSkillsForRole(systemSkills, string(agent.Role))
	if len(picks.IDs) == 0 {
		return
	}
	if err := applyDefaultSkillPicks(agent, picks); err != nil {
		s.logger.Warn("encode default skill ids", zap.Error(err))
	}
}

// defaultSkillPicks bundles the parallel id+slug lists for the
// system skills auto-attached to an agent. SkillIDs feeds the
// UI's per-agent toggle (it compares against `skill.id`); the
// legacy DesiredSkills slug list is what the runtime materializer
// resolves at launch time. Writing both keeps the two surfaces in
// sync — the alternative left the UI showing "no skills" even
// when the agent was already attached to the bundled set.
type defaultSkillPicks struct {
	IDs   []string
	Slugs []string
}

func applyDefaultSkillPicks(agent *models.AgentInstance, picks defaultSkillPicks) error {
	encodedIDs, err := json.Marshal(picks.IDs)
	if err != nil {
		return err
	}
	encodedSlugs, err := json.Marshal(picks.Slugs)
	if err != nil {
		return err
	}
	agent.SkillIDs = string(encodedIDs)
	agent.DesiredSkills = string(encodedSlugs)
	return nil
}

// needsDefaultSkillSeed returns true when DesiredSkills is empty or
// the JSON sentinel `[]`. Any other value means the caller picked
// an explicit set and we leave it alone.
func needsDefaultSkillSeed(desired string) bool {
	trimmed := strings.TrimSpace(desired)
	return trimmed == "" || trimmed == "[]"
}

// pickDefaultSkillsForRole returns the parallel (id, slug) lists for
// every system skill whose `default_for_roles` JSON array contains
// the given role. Skills without the column populated (legacy rows)
// or that fail to parse are skipped silently.
func pickDefaultSkillsForRole(skills []*models.Skill, role string) defaultSkillPicks {
	picks := defaultSkillPicks{}
	if role == "" {
		return picks
	}
	for _, sk := range skills {
		if !sk.IsSystem || sk.DefaultForRoles == "" {
			continue
		}
		var roles []string
		if err := json.Unmarshal([]byte(sk.DefaultForRoles), &roles); err != nil {
			continue
		}
		for _, r := range roles {
			if r == role {
				picks.IDs = append(picks.IDs, sk.ID)
				picks.Slugs = append(picks.Slugs, sk.Slug)
				break
			}
		}
	}
	return picks
}

func (s *AgentService) persistAgent(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.repo.CreateAgentInstance(ctx, agent); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	if err := s.CreateDefaultInstructions(ctx, agent.ID, string(agent.Role)); err != nil {
		s.logger.Warn("failed to create default instructions", zap.Error(err))
	}
	s.installCoordinatorRoutine(ctx, agent)
	return nil
}

// installCoordinatorRoutine triggers the default coordinator-heartbeat
// routine install for CEO/coordinator agents. Pending-approval agents
// skip the install — their routine is materialised once the approval
// flips them to idle (a future iteration; for now they fire on demand
// via comments / mentions). Failures are warn-logged and ignored.
func (s *AgentService) installCoordinatorRoutine(ctx context.Context, agent *models.AgentInstance) {
	if s.routineInstaller == nil || agent == nil {
		return
	}
	if agent.Role != models.AgentRoleCEO {
		return
	}
	if agent.Status == models.AgentStatusPendingApproval {
		return
	}
	if _, err := s.routineInstaller.CreateDefaultCoordinatorRoutine(ctx, agent.WorkspaceID, agent.ID); err != nil {
		s.logger.Warn("install default coordinator routine",
			zap.String("workspace_id", agent.WorkspaceID),
			zap.String("agent_id", agent.ID),
			zap.Error(err))
	}
}

func (s *AgentService) requiresHireApproval(
	ctx context.Context,
	workspaceID string,
	callerAgent *models.AgentInstance,
) bool {
	if callerAgent == nil || s.governanceSettings == nil || s.governanceApproval == nil {
		return false
	}
	required, err := s.governanceSettings.GetRequireApprovalForNewAgents(ctx, workspaceID)
	return err == nil && required
}

func (s *AgentService) createHireApproval(
	ctx context.Context,
	agent *models.AgentInstance,
	callerAgent *models.AgentInstance,
	reason string,
) error {
	b, err := json.Marshal(map[string]interface{}{
		"agent_profile_id":   agent.ID,
		"creator_agent_id":   callerAgent.ID,
		"creator_agent_name": callerAgent.Name,
		"name":               agent.Name,
		"role":               string(agent.Role),
		"permissions":        agent.Permissions,
		"reason":             reason,
	})
	if err != nil {
		return fmt.Errorf("marshal hire approval payload: %w", err)
	}
	approval := &models.Approval{
		WorkspaceID:               agent.WorkspaceID,
		Type:                      models.ApprovalTypeHireAgent,
		RequestedByAgentProfileID: callerAgent.ID,
		Payload:                   string(b),
	}
	if err := s.governanceApproval.CreateApprovalWithActivity(ctx, approval); err != nil {
		return fmt.Errorf("create hire_agent approval: %w", err)
	}
	return nil
}

func (s *AgentService) ListAgentInstancesFiltered(
	ctx context.Context, workspaceID string, filter AgentListFilter,
) ([]*models.AgentInstance, error) {
	return s.repo.ListAgentInstancesFiltered(ctx, workspaceID, sqlite.AgentListFilter{
		Role:      filter.Role,
		Status:    filter.Status,
		ReportsTo: filter.ReportsTo,
	})
}

// UpdateAgentInstance validates and updates an existing agent instance in the DB.
func (s *AgentService) UpdateAgentInstance(ctx context.Context, agent *models.AgentInstance) error {
	if err := s.validateAgentUpdate(ctx, agent); err != nil {
		return err
	}
	if err := s.repo.UpdateAgentInstance(ctx, agent); err != nil {
		return fmt.Errorf("update agent: %w", err)
	}
	return nil
}

// UpdateAgentStatus validates a status transition and persists the new state to the DB.
func (s *AgentService) UpdateAgentStatus(
	ctx context.Context, id string, newStatus models.AgentStatus, pauseReason string,
) (*models.AgentInstance, error) {
	agent, err := s.GetAgentFromConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateStatusTransition(agent.Status, newStatus); err != nil {
		return nil, err
	}
	if dbErr := s.repo.UpdateAgentStatusFields(ctx, agent.ID, string(newStatus), pauseReason); dbErr != nil {
		return nil, fmt.Errorf("persist agent status: %w", dbErr)
	}
	agent.Status = newStatus
	agent.PauseReason = pauseReason
	return agent, nil
}

// DeleteAgentInstance deletes an agent instance from the DB and cascades
// termination of every office task session belonging to that agent. The
// cascade runs after the DB row is gone so a concurrent EnsureSessionForAgent
// can't resurrect the row by inserting a fresh CREATED state.
func (s *AgentService) DeleteAgentInstance(ctx context.Context, id string) error {
	agent, err := s.GetAgentFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteAgentInstance(ctx, agent.ID); err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	if s.sessionTerm != nil {
		if err := s.sessionTerm.TerminateAllForAgent(ctx, agent.ID, "agent_instance_deleted"); err != nil {
			s.logger.Warn("cascade-terminate office sessions on agent deletion failed",
				zap.String("agent_profile_id", agent.ID),
				zap.Error(err))
		}
	}
	return nil
}

// DefaultPermissionsJSON returns the default permissions JSON for a role.
func DefaultPermissionsJSON(role models.AgentRole) string {
	perms := defaultPermsForRole(role)
	b, _ := json.Marshal(perms)
	return string(b)
}

func defaultPermsForRole(role models.AgentRole) map[string]interface{} {
	switch role {
	case models.AgentRoleCEO:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      true,
			"can_create_agents":     true,
			"can_approve":           true,
			"can_manage_own_skills": true,
			"max_subtask_depth":     3,
		}
	case models.AgentRoleAssistant:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      true,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": true,
			"max_subtask_depth":     1,
		}
	case models.AgentRoleWorker:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      true,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": false,
			"max_subtask_depth":     1,
		}
	case models.AgentRoleSecurity:
		return map[string]interface{}{
			"can_create_tasks":      false,
			"can_assign_tasks":      true,
			"can_create_agents":     false,
			"can_approve":           true,
			"can_manage_own_skills": true,
			"max_subtask_depth":     1,
		}
	case models.AgentRoleQA:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      false,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": true,
			"max_subtask_depth":     1,
		}
	case models.AgentRoleDevOps:
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      false,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": true,
			"max_subtask_depth":     1,
		}
	default: // specialist and any unknown roles
		return map[string]interface{}{
			"can_create_tasks":      true,
			"can_assign_tasks":      false,
			"can_create_agents":     false,
			"can_approve":           false,
			"can_manage_own_skills": false,
			"max_subtask_depth":     1,
		}
	}
}

// validateAgentCreate checks all business rules for creating an agent.
func (s *AgentService) validateAgentCreate(ctx context.Context, agent *models.AgentInstance) error {
	if agent.Name == "" {
		return ErrAgentNameRequired
	}
	if !validRoles[agent.Role] {
		return ErrAgentRoleInvalid
	}
	if agent.Role == models.AgentRoleCEO {
		if s.countAgentsByRoleInWorkspace(ctx, models.AgentRoleCEO, agent.WorkspaceID, "") > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if err := s.validateAgentNameUnique(ctx, agent.Name, agent.WorkspaceID, ""); err != nil {
		return err
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(ctx, agent.ReportsTo, "")
	}
	return nil
}

// validateAgentUpdate checks business rules for updating an agent.
func (s *AgentService) validateAgentUpdate(ctx context.Context, agent *models.AgentInstance) error {
	if agent.Name == "" {
		return ErrAgentNameRequired
	}
	if !validRoles[agent.Role] {
		return ErrAgentRoleInvalid
	}
	if agent.Role == models.AgentRoleCEO {
		if s.countAgentsByRoleInWorkspace(ctx, models.AgentRoleCEO, agent.WorkspaceID, agent.ID) > 0 {
			return ErrAgentCEOAlreadyExists
		}
	}
	if agent.ReportsTo != "" {
		return s.validateReportsTo(ctx, agent.ReportsTo, agent.ID)
	}
	return nil
}

// countAgentsByRoleInWorkspace counts agents with a role within a workspace,
// optionally excluding one ID. workspaceID must be non-empty to scope the query.
func (s *AgentService) countAgentsByRoleInWorkspace(
	ctx context.Context, role models.AgentRole, workspaceID, excludeID string,
) int {
	count, err := s.repo.CountAgentInstancesByRole(ctx, workspaceID, string(role), excludeID)
	if err != nil {
		return 0
	}
	return count
}

// validateAgentNameUnique ensures no other agent in the workspace has the same name.
func (s *AgentService) validateAgentNameUnique(ctx context.Context, name, workspaceID, excludeID string) error {
	exists, err := s.repo.AgentInstanceExistsByName(ctx, workspaceID, name, excludeID)
	if err != nil {
		return fmt.Errorf("check agent name uniqueness: %w", err)
	}
	if exists {
		return fmt.Errorf("agent name %q already exists", name)
	}
	return nil
}

// validateReportsTo ensures the target agent exists.
func (s *AgentService) validateReportsTo(ctx context.Context, reportsTo, selfID string) error {
	if selfID != "" && reportsTo == selfID {
		return ErrAgentReportsToSelf
	}
	_, err := s.GetAgentFromConfig(ctx, reportsTo)
	if err != nil {
		return ErrAgentReportsToInvalid
	}
	return nil
}

// validateStatusTransition checks if a status transition is allowed.
func validateStatusTransition(from, to models.AgentStatus) error {
	if from == to {
		return nil
	}
	allowed, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("%w: unknown current status %q", ErrAgentStatusTransition, from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("%w: cannot transition from %q to %q", ErrAgentStatusTransition, from, to)
}
