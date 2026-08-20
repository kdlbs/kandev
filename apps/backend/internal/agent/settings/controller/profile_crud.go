package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/profileconfig"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/secrets"
)

const dynamicProfileKind = "dynamic"

type CreateProfileRequest struct {
	AgentID        string
	Name           string
	Model          string
	FallbackModel  string
	AutoFallback   bool
	Mode           string
	ConfigOptions  map[string]string
	AllowIndexing  bool
	AutoApprove    bool
	CLIPassthrough bool
	// CLIFlags is the explicit list to persist. When nil, the profile is
	// seeded from the agent's curated PermissionSettings() list so a fresh
	// profile opens with the agent's recommended flags (all disabled by
	// default unless the curated entry specifies Default: true).
	CLIFlags []dto.CLIFlagDTO
	EnvVars  []dto.ProfileEnvVarDTO
	// CommandPrefix is an optional launcher prefix prepended to the agent
	// command (e.g. "greywall --"). Shell-tokenised at launch time.
	CommandPrefix string
	Dynamic       *dto.DynamicAgentProfileDTO
}

func (c *Controller) CreateProfile(ctx context.Context, req CreateProfileRequest) (*dto.AgentProfileDTO, error) {
	// Model is optional — the profile reconciler fills it from the host
	// utility probe cache on boot, and session start applies it via
	// ACP model selection. An empty model means "use the agent's default".
	agent, err := c.repo.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, err
	}
	agentConfig, agOk := c.agentRegistry.Get(agent.Name)
	if !agOk {
		return nil, fmt.Errorf("unknown agent: %s", agent.Name)
	}
	displayName, err := c.resolveDisplayName(agentConfig, agent.Name)
	if err != nil {
		return nil, err
	}
	if agent.Name == agents.DynamicAgentID {
		return c.createDynamicProfile(ctx, agent, displayName, req)
	}
	cliFlags := cliFlagsFromDTO(req.CLIFlags)
	if req.CLIFlags == nil {
		cliFlags = seedCLIFlags(agentConfig)
	} else if err := validateCLIFlagDTOs(req.CLIFlags); err != nil {
		return nil, err
	}
	if err := validateProfileEnvVarDTOs(req.EnvVars); err != nil {
		return nil, err
	}
	if err := c.validateGlobalSecretRefs(ctx, req.EnvVars); err != nil {
		return nil, err
	}
	if err := validateCommandPrefix(req.CommandPrefix); err != nil {
		return nil, err
	}
	profile := &models.AgentProfile{
		AgentID:          req.AgentID,
		Name:             req.Name,
		AgentDisplayName: displayName,
		Model:            req.Model,
		FallbackModel:    strings.TrimSpace(req.FallbackModel),
		AutoFallback:     req.AutoFallback,
		Mode:             req.Mode,
		ConfigOptions:    profileconfig.SanitizeConfigOptions(req.ConfigOptions),
		AllowIndexing:    req.AllowIndexing,
		AutoApprove:      req.AutoApprove,
		CLIPassthrough:   req.CLIPassthrough,
		Enabled:          true,
		CLIFlags:         cliFlags,
		EnvVars:          envVarsFromDTO(req.EnvVars),
		CommandPrefix:    strings.TrimSpace(req.CommandPrefix),
		UserModified:     true,
	}
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
}

func (c *Controller) createDynamicProfile(
	ctx context.Context,
	agent *models.Agent,
	displayName string,
	req CreateProfileRequest,
) (*dto.AgentProfileDTO, error) {
	if !c.dynamicAgentRoutingEnabled {
		return nil, ErrDynamicAgentRoutingDisabled
	}
	dynamicRepo, err := c.dynamicProfileRepository()
	if err != nil {
		return nil, err
	}
	if err := validateDynamicAgentProfile(req.Dynamic); err != nil {
		return nil, err
	}
	profile := &models.AgentProfile{
		ID:               uuid.NewString(),
		AgentID:          agent.ID,
		Name:             strings.TrimSpace(req.Name),
		AgentDisplayName: displayName,
		Enabled:          true,
		CLIFlags:         []models.CLIFlag{},
		EnvVars:          []models.ProfileEnvVar{},
		UserModified:     true,
	}
	routes, err := c.validateDynamicCandidates(ctx, profile.ID, req.Dynamic)
	if err != nil {
		return nil, err
	}
	if err := c.repo.CreateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	dynamic := &models.DynamicAgentProfile{ProfileID: profile.ID, Version: 1}
	if err := dynamicRepo.CreateDynamicAgentProfile(ctx, dynamic, routes); err != nil {
		if cleanupErr := c.repo.DeleteAgentProfile(ctx, profile.ID); cleanupErr != nil {
			return nil, fmt.Errorf("%w; cleanup dynamic profile parent: %v", err, cleanupErr)
		}
		return nil, err
	}
	result := toProfileDTO(profile)
	result.Dynamic, err = dynamicProfileDTO(dynamic, routes)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Controller) dynamicProfileRepository() (store.DynamicProfileRepository, error) {
	dynamicRepo, ok := c.repo.(store.DynamicProfileRepository)
	if !ok {
		return nil, fmt.Errorf("dynamic profile store is unavailable")
	}
	return dynamicRepo, nil
}

func (c *Controller) validateDynamicCandidates(
	ctx context.Context,
	profileID string,
	dynamic *dto.DynamicAgentProfileDTO,
) ([]models.DynamicAgentRoute, error) {
	routes, err := dynamicRoutesFromDTO(profileID, dynamic)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if route.ExecutionProfileID == profileID {
			return nil, fmt.Errorf("%w: a profile cannot reference itself", ErrDynamicProfileCandidate)
		}
		if _, ok := seen[route.ExecutionProfileID]; ok {
			return nil, fmt.Errorf("%w: duplicate execution profile %q", ErrDynamicProfileCandidate, route.ExecutionProfileID)
		}
		seen[route.ExecutionProfileID] = struct{}{}
		candidate, err := c.repo.GetAgentProfileIncludingDeleted(ctx, route.ExecutionProfileID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: profile %q was not found", ErrDynamicProfileCandidate, route.ExecutionProfileID)
			}
			return nil, err
		}
		if candidate.DeletedAt != nil {
			return nil, fmt.Errorf("%w: profile %q is deleted", ErrDynamicProfileCandidate, route.ExecutionProfileID)
		}
		candidateAgent, err := c.repo.GetAgent(ctx, candidate.AgentID)
		if err != nil {
			return nil, fmt.Errorf("%w: profile %q has no agent family", ErrDynamicProfileCandidate, route.ExecutionProfileID)
		}
		if candidate.AgentID == agents.DynamicAgentID || candidateAgent.Name == agents.DynamicAgentID {
			return nil, fmt.Errorf("%w: dynamic profiles cannot be candidates", ErrDynamicProfileCandidate)
		}
		if candidate.WorkspaceID != "" || candidate.Role != "" {
			return nil, fmt.Errorf("%w: rich Office profiles cannot be candidates", ErrDynamicProfileCandidate)
		}
		if candidate.AutoFallback {
			return nil, fmt.Errorf("%w: AutoFallback profiles cannot be candidates", ErrDynamicProfileCandidate)
		}
		if _, ok := c.agentRegistry.GetInferenceAgent(candidateAgent.Name); !ok {
			return nil, fmt.Errorf("%w: profile %q is not launchable", ErrDynamicProfileCandidate, route.ExecutionProfileID)
		}
	}
	return routes, nil
}

// seedCLIFlags builds the default cli_flags list for a new profile from the
// agent's curated PermissionSettings() catalogue. Only entries that target a
// CLI flag are included; per-flag metadata (description, flag text, default
// enabled) is copied into the row so the profile is self-contained.
func seedCLIFlags(agent agents.Agent) []models.CLIFlag {
	settings := agents.CatalogPermissionSettings(agent)
	flags := make([]models.CLIFlag, 0, len(settings))
	for key, s := range settings {
		if !s.Supported || s.ApplyMethod != agents.PermissionApplyMethodCLIFlag || s.CLIFlag == "" {
			continue
		}
		// dangerously_skip_permissions is wired to the profile's dedicated
		// DangerouslySkipPermissions column; the passthrough launch path emits
		// the flag via PermissionValues. Seeding it as a curated cli_flag too
		// would surface a duplicate toggle in the UI and double-emit the flag.
		if key == agents.PermissionKeyDangerouslySkipPermissions {
			continue
		}
		flagText := s.CLIFlag
		if s.CLIFlagValue != "" {
			flagText = s.CLIFlag + " " + s.CLIFlagValue
		}
		flags = append(flags, models.CLIFlag{
			Description: firstNonEmpty(s.Description, s.Label),
			Flag:        flagText,
			Enabled:     s.Default,
		})
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Flag < flags[j].Flag })
	return flags
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

const (
	dynamicRouteActionRetrySame = "retry_same"
	dynamicRouteActionTryNext   = "try_next"
	dynamicRouteActionStop      = "stop"
)

func validateDynamicAgentProfile(profile *dto.DynamicAgentProfileDTO) error {
	if profile == nil || len(profile.Candidates) == 0 {
		return ErrDynamicProfileCandidatesRequired
	}
	for position, candidate := range profile.Candidates {
		if candidate.Position != position {
			return fmt.Errorf("%w: candidate %d has position %d", ErrDynamicProfilePositions, position, candidate.Position)
		}
		if strings.TrimSpace(candidate.ExecutionProfileID) == "" {
			return fmt.Errorf("%w: candidate %d has no execution profile", ErrDynamicProfileCandidate, position)
		}
		if err := normalizeDynamicCandidatePolicy(&profile.Candidates[position]); err != nil {
			return err
		}
	}
	return nil
}

func dynamicRoutesFromDTO(profileID string, profile *dto.DynamicAgentProfileDTO) ([]models.DynamicAgentRoute, error) {
	if err := validateDynamicAgentProfile(profile); err != nil {
		return nil, err
	}
	routes := make([]models.DynamicAgentRoute, 0, len(profile.Candidates))
	for _, candidate := range profile.Candidates {
		if candidate.Policies == nil {
			return nil, fmt.Errorf("%w: candidate %d has no normalized policy", ErrDynamicProfileRule, candidate.Position)
		}
		policyJSON, err := json.Marshal(candidate.Policies)
		if err != nil {
			return nil, fmt.Errorf("marshal dynamic route policy: %w", err)
		}
		routes = append(routes, models.DynamicAgentRoute{
			DynamicProfileID:   profileID,
			Position:           candidate.Position,
			ExecutionProfileID: strings.TrimSpace(candidate.ExecutionProfileID),
			Enabled:            candidate.Enabled,
			RulesJSON:          string(policyJSON),
		})
	}
	return routes, nil
}

func dynamicProfileDTO(profile *models.DynamicAgentProfile, routes []models.DynamicAgentRoute) (*dto.DynamicAgentProfileDTO, error) {
	if profile == nil {
		return nil, nil
	}
	result := &dto.DynamicAgentProfileDTO{
		Version:    profile.Version,
		Candidates: make([]dto.DynamicAgentCandidateDTO, 0, len(routes)),
	}
	for _, route := range routes {
		policy, err := decodeDynamicPolicyDocument(route.RulesJSON, route.Position)
		if err != nil {
			return nil, err
		}
		result.Candidates = append(result.Candidates, dto.DynamicAgentCandidateDTO{
			Position:           route.Position,
			ExecutionProfileID: route.ExecutionProfileID,
			Enabled:            route.Enabled,
			Policies:           &policy,
		})
	}
	return result, nil
}

type UpdateProfileRequest struct {
	ID             string
	Name           *string
	Model          *string
	FallbackModel  *string
	AutoFallback   *bool
	Mode           *string
	ConfigOptions  *map[string]string
	AllowIndexing  *bool
	AutoApprove    *bool
	CLIPassthrough *bool
	// Enabled replaces the value when non-nil. Nil means "leave unchanged".
	Enabled *bool
	// CLIFlags replaces the entire list when non-nil. Nil means "leave
	// unchanged" — the UI always sends the full desired list on save.
	CLIFlags *[]dto.CLIFlagDTO
	// EnvVars replaces the entire list when non-nil.
	EnvVars *[]dto.ProfileEnvVarDTO
	// CommandPrefix replaces the value when non-nil. Nil means "leave
	// unchanged" — the UI always sends the desired value on save.
	CommandPrefix *string
	Dynamic       *dto.DynamicAgentProfileDTO
	Force         bool
}

func enabledOnlyUpdate(req UpdateProfileRequest) bool {
	return req.Enabled != nil && req.Name == nil && req.Model == nil &&
		req.FallbackModel == nil && req.AutoFallback == nil && req.Mode == nil &&
		req.ConfigOptions == nil && req.AllowIndexing == nil && req.AutoApprove == nil &&
		req.CLIPassthrough == nil && req.CLIFlags == nil && req.EnvVars == nil &&
		req.CommandPrefix == nil && req.Dynamic == nil
}

func (c *Controller) UpdateProfile(ctx context.Context, req UpdateProfileRequest) (*dto.AgentProfileDTO, error) {
	profile, err := c.repo.GetAgentProfile(ctx, req.ID)
	if err != nil {
		return nil, ErrAgentProfileNotFound
	}
	isDynamic := profileKind(profile) == dynamicProfileKind
	if isDynamic && !c.dynamicAgentRoutingEnabled {
		return nil, ErrDynamicAgentRoutingDisabled
	}
	var (
		dynamicRepo   store.DynamicProfileRepository
		dynamic       *models.DynamicAgentProfile
		dynamicRoutes []models.DynamicAgentRoute
	)
	if isDynamic && req.Dynamic != nil {
		if req.Dynamic.Version <= 0 {
			return nil, fmt.Errorf("dynamic profile version is required")
		}
		dynamicRoutes, err = c.validateDynamicCandidates(ctx, profile.ID, req.Dynamic)
		if err != nil {
			return nil, err
		}
		dynamicRepo, err = c.dynamicProfileRepository()
		if err != nil {
			return nil, err
		}
		dynamic = &models.DynamicAgentProfile{ProfileID: profile.ID}
	}
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Model != nil {
		profile.Model = *req.Model
	}
	if req.FallbackModel != nil {
		profile.FallbackModel = strings.TrimSpace(*req.FallbackModel)
	}
	if req.AutoFallback != nil {
		profile.AutoFallback = *req.AutoFallback
	}
	if req.Mode != nil {
		profile.Mode = *req.Mode
	}
	if req.ConfigOptions != nil {
		profile.ConfigOptions = profileconfig.SanitizeConfigOptions(*req.ConfigOptions)
	}
	if req.AllowIndexing != nil {
		profile.AllowIndexing = *req.AllowIndexing
	}
	if req.AutoApprove != nil {
		profile.AutoApprove = *req.AutoApprove
	}
	if req.CLIPassthrough != nil {
		profile.CLIPassthrough = *req.CLIPassthrough
	}
	if req.Enabled != nil {
		dynamicRefs, err := c.listDynamicProfileReferences(ctx, req.ID)
		if err != nil {
			return nil, err
		}
		if !*req.Enabled && !req.Force && len(dynamicRefs) > 0 {
			return nil, &ErrProfileInUseDetail{DynamicProfiles: dynamicRefs}
		}
		if !*req.Enabled && !req.Force && c.utilityDeps != nil {
			refs, err := c.utilityDeps.ListUtilityAgentsByAgentProfile(ctx, req.ID)
			if err != nil {
				return nil, fmt.Errorf("check utility agents using this profile: %w", err)
			}
			if len(refs) > 0 {
				return nil, &ErrProfileInUseDetail{UtilityAgents: refs}
			}
		}
		profile.Enabled = *req.Enabled
	}
	if enabledOnlyUpdate(req) {
		profile.UserModified = true
		updatedAt, err := c.repo.UpdateAgentProfileEnabled(ctx, profile.ID, profile.Enabled)
		if err != nil {
			return nil, err
		}
		profile.UpdatedAt = updatedAt
		result := toProfileDTO(profile)
		return &result, nil
	}
	if req.CLIFlags != nil {
		if err := validateCLIFlagDTOs(*req.CLIFlags); err != nil {
			return nil, err
		}
		profile.CLIFlags = cliFlagsFromDTO(*req.CLIFlags)
	}
	if req.EnvVars != nil {
		if err := validateProfileEnvVarDTOs(*req.EnvVars); err != nil {
			return nil, err
		}
		if err := c.validateGlobalSecretRefs(ctx, *req.EnvVars); err != nil {
			return nil, err
		}
		profile.EnvVars = envVarsFromDTO(*req.EnvVars)
	}
	if req.CommandPrefix != nil {
		if err := validateCommandPrefix(*req.CommandPrefix); err != nil {
			return nil, err
		}
		profile.CommandPrefix = strings.TrimSpace(*req.CommandPrefix)
	}
	profile.UserModified = true
	if dynamic != nil {
		result, handled, atomicErr := c.updateDynamicProfileAtomically(
			ctx, profile, dynamic, req.Dynamic.Version, dynamicRoutes,
		)
		if atomicErr != nil {
			return nil, atomicErr
		}
		if handled {
			return result, nil
		}
	}
	if dynamicRepo != nil && dynamic != nil {
		if err := dynamicRepo.UpdateDynamicAgentProfile(ctx, dynamic, req.Dynamic.Version, dynamicRoutes); err != nil {
			return nil, err
		}
	}
	if err := c.repo.UpdateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	if dynamicRepo != nil && dynamic != nil {
		result.Dynamic, err = dynamicProfileDTO(dynamic, dynamicRoutes)
		if err != nil {
			return nil, err
		}
	}
	return &result, nil
}

func (c *Controller) updateDynamicProfileAtomically(
	ctx context.Context,
	profile *models.AgentProfile,
	dynamic *models.DynamicAgentProfile,
	version int64,
	routes []models.DynamicAgentRoute,
) (*dto.AgentProfileDTO, bool, error) {
	atomicRepo, ok := c.repo.(store.AtomicDynamicProfileRepository)
	if !ok {
		return nil, false, nil
	}
	if err := atomicRepo.UpdateAgentProfileWithDynamic(ctx, profile, dynamic, version, routes); err != nil {
		return nil, true, err
	}
	result := toProfileDTO(profile)
	var err error
	result.Dynamic, err = dynamicProfileDTO(dynamic, routes)
	if err != nil {
		return nil, true, err
	}
	return &result, true, nil
}

type DuplicateProfileRequest struct {
	ID string
}

// DuplicateProfile creates an independent copy of an existing profile. The
// copy keeps every configuration field (model, mode, config options, CLI
// flags, env vars, launcher prefix, auto-approve flags, enabled state, MCP
// config) under a fresh row named "<source> Copy", so a user can start a
// variant from a working profile without re-entering it. Runtime state is
// intentionally not copied: the copy starts idle with no pause reason,
// last-run timestamp, or consecutive-failure count. No in-use checks apply —
// a brand-new row cannot be referenced by sessions, watchers, automations,
// or routing tiers yet.
//
// The copy is committed in one repository transaction (row + MCP config), so
// a failure leaves no partial profile and a disabled source never becomes
// briefly selectable.
//
// The source profile and MCP row are read up front, then the repository
// re-verifies those revisions inside the transaction; a concurrent writer
// between the reads and the insert aborts with a retryable error, so the
// copy always reflects one consistent snapshot of the source.
func (c *Controller) DuplicateProfile(ctx context.Context, req DuplicateProfileRequest) (*dto.AgentProfileDTO, error) {
	source, err := c.repo.GetAgentProfile(ctx, req.ID)
	if err != nil {
		if isProfileNotFoundErr(err) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	// Office-scoped profiles are owned by the workspace-scoped office API
	// surface and hidden from this instance-level settings surface (the UI
	// filters them via filterGlobalProfiles). Refuse to duplicate them here so
	// the endpoint cannot read or clone another workspace's configuration;
	// 404 keeps the existence of office profiles hidden.
	if source.WorkspaceID != "" {
		return nil, ErrAgentProfileNotFound
	}
	if profileKind(source) == dynamicProfileKind {
		return nil, ErrDynamicProfileDuplicationUnsupported
	}
	for attempt := 0; ; attempt++ {
		// A source without an MCP row leaves the copy without one: the
		// default-config semantics and boot EnsureDefaultMcpConfig cover
		// MCP-supporting agents.
		var sourceMcp *models.AgentProfileMcpConfig
		sourceMcp, err = c.repo.GetAgentProfileMcpConfig(ctx, source.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if errors.Is(err, sql.ErrNoRows) {
			sourceMcp = nil
		}
		clone := duplicateClone(source)
		var mcpCopy *models.AgentProfileMcpConfig
		if sourceMcp != nil {
			mcpCopy = &models.AgentProfileMcpConfig{
				Enabled: sourceMcp.Enabled,
				Servers: cloneStringInterfaceMap(sourceMcp.Servers),
				Meta:    cloneStringInterfaceMap(sourceMcp.Meta),
			}
		}
		err = c.repo.DuplicateAgentProfile(ctx, store.DuplicateAgentProfileInput{
			Source:    source,
			SourceMcp: sourceMcp,
			Profile:   clone,
			McpConfig: mcpCopy,
		})
		if err == nil {
			result := toProfileDTO(clone)
			return &result, nil
		}
		// A source deleted between the snapshot read and the transaction is a
		// deterministic 404, not a retryable race: the copy has nothing to
		// reflect, so surface it immediately instead of re-reading the source
		// up to maxDuplicateRetries times.
		if errors.Is(err, store.ErrSourceProfileNotFound) {
			return nil, ErrAgentProfileNotFound
		}
		if !isRetryableDuplicateErr(err) || attempt >= maxDuplicateRetries {
			return nil, err
		}
		source, err = c.repo.GetAgentProfile(ctx, req.ID)
		if err != nil {
			if isProfileNotFoundErr(err) {
				return nil, ErrAgentProfileNotFound
			}
			return nil, err
		}
	}
}

// maxDuplicateRetries bounds the number of re-attempts after a concurrent
// source change. One retry covers the common single-writer race; the cap
// prevents a hot loop under sustained concurrent edits.
const maxDuplicateRetries = 2

// isRetryableDuplicateErr reports whether a duplicate attempt failed because
// the source changed concurrently (revision mismatch or a WAL snapshot-isolation
// busy error on the write) rather than deterministically. A deleted source is
// intentionally absent: DuplicateProfile maps ErrSourceProfileNotFound to 404
// before consulting this set.
func isRetryableDuplicateErr(err error) bool {
	return errors.Is(err, store.ErrProfileChanged) ||
		strings.Contains(err.Error(), "database is locked") ||
		strings.Contains(err.Error(), "database table is locked")
}

// duplicateClone builds the copy row from the source profile. Runtime state
// is intentionally not copied: the copy starts idle with no pause reason,
// last-run timestamp, or consecutive-failure count.
func duplicateClone(source *models.AgentProfile) *models.AgentProfile {
	return &models.AgentProfile{
		AgentID:                    source.AgentID,
		Name:                       strings.TrimSpace(source.Name) + " Copy",
		AgentDisplayName:           source.AgentDisplayName,
		Model:                      source.Model,
		FallbackModel:              strings.TrimSpace(source.FallbackModel),
		AutoFallback:               source.AutoFallback,
		Mode:                       source.Mode,
		ConfigOptions:              profileconfig.SanitizeConfigOptions(cloneStringMap(source.ConfigOptions)),
		AllowIndexing:              source.AllowIndexing,
		AutoApprove:                source.AutoApprove,
		CLIPassthrough:             source.CLIPassthrough,
		CLIFlags:                   cloneCLIFlags(source.CLIFlags),
		EnvVars:                    cloneEnvVars(source.EnvVars),
		CommandPrefix:              source.CommandPrefix,
		UserModified:               true,
		Enabled:                    source.Enabled,
		WorkspaceID:                source.WorkspaceID,
		Role:                       source.Role,
		Icon:                       source.Icon,
		ReportsTo:                  source.ReportsTo,
		SkillIDs:                   source.SkillIDs,
		DesiredSkills:              source.DesiredSkills,
		MaxConcurrentSessions:      source.MaxConcurrentSessions,
		CooldownSec:                source.CooldownSec,
		SkipIdleRuns:               source.SkipIdleRuns,
		FailureThreshold:           cloneIntPtr(source.FailureThreshold),
		ExecutorPreference:         source.ExecutorPreference,
		BudgetMonthlyCents:         source.BudgetMonthlyCents,
		Settings:                   source.Settings,
		Permissions:                source.Permissions,
		DangerouslySkipPermissions: source.DangerouslySkipPermissions,
		CustomPrompt:               source.CustomPrompt,
	}
}

// isProfileNotFoundErr reports whether err means "no live profile row with
// that ID". The sqlite store surfaces it as sql.ErrNoRows from GetAgentProfile
// and as an "agent profile not found" message from the update/delete paths;
// fakes and future stores may use either shape.
func isProfileNotFoundErr(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "agent profile not found")
}

// cloneCLIFlags returns a copy of the profile's CLI flag list so the
// duplicated profile never shares slice memory with the source.
func cloneCLIFlags(in []models.CLIFlag) []models.CLIFlag {
	out := make([]models.CLIFlag, len(in))
	copy(out, in)
	return out
}

// cloneEnvVars returns a copy of the profile's env-var list (secret
// references included) so the duplicated profile never shares slice memory
// with the source.
func cloneEnvVars(in []models.ProfileEnvVar) []models.ProfileEnvVar {
	out := make([]models.ProfileEnvVar, len(in))
	copy(out, in)
	return out
}

// cloneStringMap returns a shallow copy of a string map, preserving nil so a
// source with no config options stays nil on the copy.
func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// cloneStringInterfaceMap returns a shallow copy of a string-keyed interface
// map (used for MCP servers/meta), preserving nil.
func cloneStringInterfaceMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// cloneIntPtr returns a copy of an int pointer so the duplicated profile never
// aliases the source's field, preserving nil.
func cloneIntPtr(in *int) *int {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func (c *Controller) validateGlobalSecretRefs(ctx context.Context, envVars []dto.ProfileEnvVarDTO) error {
	if c.secretStore == nil {
		return nil
	}
	for i, envVar := range envVars {
		if strings.TrimSpace(envVar.SecretID) == "" {
			continue
		}
		if err := secrets.ValidateGlobalReference(ctx, c.secretStore, envVar.SecretID); err != nil {
			return fmt.Errorf("%w: env_vars[%d] secret must be global", ErrInvalidProfileEnvVars, i)
		}
	}
	return nil
}

// validateCLIFlagDTOs rejects entries with an empty flag string or malformed
// shell tokens (unterminated quotes, trailing backslash). Empty descriptions
// are allowed (custom flags often don't have one). Tokenising here keeps the
// launch path's cliflags.Resolve error branch unreachable in practice: a
// single bad entry must not silently drop every other enabled flag at task
// start, which is what would happen if we let it slip through to the
// subprocess builder.
func validateCLIFlagDTOs(in []dto.CLIFlagDTO) error {
	for i, f := range in {
		if strings.TrimSpace(f.Flag) == "" {
			return fmt.Errorf("cli_flags[%d].flag is required", i)
		}
		tokens, err := cliflags.Tokenise(f.Flag)
		if err != nil {
			return fmt.Errorf("cli_flags[%d]: %w", i, err)
		}
		// Reject entries where the primary token (the flag name itself) is
		// empty — e.g. `""` or `''` passes TrimSpace but tokenises to a
		// single blank argv element, which would reach the subprocess
		// argv and likely confuse the agent. Secondary tokens can still
		// be empty (`--empty ""` legitimately passes an empty value).
		if len(tokens) == 0 || tokens[0] == "" {
			return fmt.Errorf("cli_flags[%d].flag is required", i)
		}
	}
	return nil
}

// validateCommandPrefix rejects a launcher prefix with malformed shell tokens
// (unterminated quotes, trailing backslash). An empty/whitespace-only prefix is
// valid — it means "run the agent command unwrapped". Tokenising here keeps the
// launch path's cliflags.Tokenise error branch unreachable in practice, so a
// bad prefix surfaces at save time rather than silently dropping at task start.
func validateCommandPrefix(prefix string) error {
	if err := cliflags.ValidateCommandPrefix(prefix); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCommandPrefix, err)
	}
	return nil
}

func (c *Controller) DeleteProfile(ctx context.Context, id string, force bool) (*dto.AgentProfileDTO, error) {
	profile, err := c.repo.GetAgentProfile(ctx, id)
	if err != nil {
		// The read path reports a missing (or already soft-deleted) row as
		// sql.ErrNoRows, not as the "agent profile not found" message the
		// update/delete paths use, so this has to go through the helper that
		// knows both shapes or the sentinel is never returned.
		if isProfileNotFoundErr(err) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	if profileKind(profile) == dynamicProfileKind && !c.dynamicAgentRoutingEnabled {
		return nil, ErrDynamicAgentRoutingDisabled
	}
	if err := c.prepareProfileDeletion(ctx, id, force); err != nil {
		return nil, err
	}
	// Automations are disabled BEFORE the row is deleted and a failure here
	// aborts the whole delete. See disableReferencingAutomations for why this
	// pass runs in the opposite order to the watcher pass below.
	if err := c.disableReferencingAutomations(ctx, id); err != nil {
		return nil, err
	}
	if err := c.repo.DeleteAgentProfile(ctx, id); err != nil {
		if isProfileNotFoundErr(err) {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	// Watchers, by contrast, are disabled only AFTER the row is gone, so a
	// failed delete never strands watchers disabled against a still-live
	// profile. They can afford that ordering because they have a genuine second
	// chance: the dispatch coordinator's preflight re-resolves the profile on
	// every watcher poll and disables the row itself when the profile has
	// vanished. So a failure here is logged and ignored — the next poll fixes
	// it. Automations have no such preflight, which is the whole reason they
	// are handled above instead of here.
	if force {
		if c.utilityDeps != nil {
			if err := c.utilityDeps.ClearUtilityAgentProfileBindings(ctx, id); err != nil {
				return nil, fmt.Errorf("clear utility agents using this profile: %w", err)
			}
		}
		c.disableReferencingWatchers(ctx, id, profile.Name)
	}
	result := toProfileDTO(profile)
	return &result, nil
}

// prepareProfileDeletion blocks every routing-tier reference, then checks for
// active sessions and referencing watchers before cleaning up ephemeral tasks.
// Routing-tier references are hard blockers even when force=true because a
// deleted profile would orphan workspace tier mappings. When force is false,
// active sessions and watchers return *ErrProfileInUseDetail so the UI can
// render a confirmation dialog. force=true skips only those soft blockers.
// Neither branch disables anything: DeleteProfile disables referencing
// automations before the row goes away and referencing watchers after it does.
func (c *Controller) prepareProfileDeletion(ctx context.Context, profileID string, force bool) error {
	dynamicRefs, err := c.listDynamicProfileReferences(ctx, profileID)
	if err != nil {
		return err
	}
	if !force && len(dynamicRefs) > 0 {
		return &ErrProfileInUseDetail{DynamicProfiles: dynamicRefs}
	}
	routingTierRefs, err := c.listRoutingTierReferences(ctx, profileID)
	if err != nil {
		return err
	}
	if len(routingTierRefs) > 0 {
		return &ErrProfileInUseDetail{RoutingTiers: routingTierRefs}
	}
	var utilityRefs []UtilityAgentReference
	if c.utilityDeps != nil {
		refs, err := c.utilityDeps.ListUtilityAgentsByAgentProfile(ctx, profileID)
		if err != nil {
			return fmt.Errorf("check utility agents using this profile: %w", err)
		}
		utilityRefs = refs
	}
	if c.sessionChecker == nil {
		if !force && len(utilityRefs) > 0 {
			return &ErrProfileInUseDetail{UtilityAgents: utilityRefs, DynamicProfiles: dynamicRefs}
		}
		return nil
	}
	if !force {
		activeTasks, err := c.sessionChecker.GetActiveTaskInfoByAgentProfile(ctx, profileID)
		if err != nil {
			return err
		}
		var watcherRefs []WatcherReference
		if c.watcherDeps != nil {
			refs, err := c.watcherDeps.ListWatchersByAgentProfile(ctx, profileID)
			if err != nil {
				c.logger.Warn("watcher deps lookup failed; proceeding without watcher info",
					zap.String("profile_id", profileID), zap.Error(err))
			} else {
				watcherRefs = refs
			}
		}
		var automationRefs []AutomationReference
		if c.automationDeps != nil {
			refs, err := c.automationDeps.ListEnabledAutomationsByAgentProfile(ctx, profileID)
			if err != nil {
				// Fails closed, unlike the watcher lookup above. A watcher this
				// path misses is disabled anyway by the force pass afterwards;
				// an automation is not, so a lookup that silently returned
				// nothing would orphan exactly the references this check was
				// added to catch. Refusing is recoverable — the user retries.
				return fmt.Errorf("check automations using this profile: %w", err)
			}
			automationRefs = refs
		}
		// An enabled automation is a standing instruction to launch against this
		// profile. Nothing is running, so it never appears in the active-session
		// list, but its next firing would go looking for a profile that is gone —
		// and a schedule fails quietly, hours later, with nobody watching.
		if len(activeTasks) > 0 || len(watcherRefs) > 0 || len(automationRefs) > 0 || len(utilityRefs) > 0 {
			return &ErrProfileInUseDetail{
				ActiveSessions:  activeTasks,
				Watchers:        watcherRefs,
				Automations:     automationRefs,
				UtilityAgents:   utilityRefs,
				DynamicProfiles: dynamicRefs,
			}
		}
	}
	// Clean up ephemeral tasks (quick chat, config chat) using this profile.
	// Done after the force check since these don't need user confirmation.
	c.cleanupEphemeralTasks(ctx, profileID)
	return nil
}

func (c *Controller) listRoutingTierReferences(ctx context.Context, profileID string) ([]RoutingTierReference, error) {
	if c.routingTierDeps == nil {
		return nil, nil
	}
	refs, err := c.routingTierDeps.ListRoutingTierReferencesByAgentProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func (c *Controller) listDynamicProfileReferences(ctx context.Context, profileID string) ([]DynamicProfileReference, error) {
	dynamicRepo, ok := c.repo.(store.DynamicProfileRepository)
	if !ok {
		return nil, nil
	}
	refs, err := dynamicRepo.ListDynamicProfileReferencesByExecutionProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("check dynamic profiles using this profile: %w", err)
	}
	out := make([]DynamicProfileReference, 0, len(refs))
	for _, ref := range refs {
		out = append(out, DynamicProfileReference{
			ID:      ref.ProfileID,
			Name:    ref.Name,
			Deleted: ref.DeletedAt != nil,
		})
	}
	return out, nil
}

// disableReferencingAutomations turns off every automation bound to the profile
// about to be deleted, and reports failure so DeleteProfile can abort.
//
// A force-delete is the user saying "yes, break these" — but an automation is
// not broken loudly. It is a standing instruction on a schedule, so left
// enabled it keeps firing into a profile that no longer exists, hours later,
// with nobody watching. Disabling is the honest outcome of the confirmation
// they just gave, and it is reversible: the automation is still there, and
// re-enabling it after picking a new profile is one toggle.
//
// This runs before the delete and is not best-effort, because the two failure
// directions cost wildly different amounts:
//
//   - disable fails, delete aborted — automation still enabled, profile still
//     there. Nothing is inconsistent, the error reaches the caller, and a retry
//     costs one click.
//   - disable fails, delete proceeds — automation enabled and bound to a row
//     that is gone. Nothing ever notices. Automations have no equivalent of the
//     watcher preflight to re-resolve the profile, so the binding is permanent
//     and every future schedule fails quietly.
//
// The residual case is disable-succeeds-then-delete-fails, which leaves an
// automation disabled against a still-live profile. That is the direction we
// choose to lose in: it is visible on the automation page and one toggle to
// undo, where the other direction is silent and unrecoverable.
//
// Runs on both the force and non-force paths. On the non-force path
// prepareProfileDeletion has already refused the delete if any automation was
// enabled, so this is normally a no-op — except in the window where one is
// enabled between that check and this call, which it also closes.
func (c *Controller) disableReferencingAutomations(ctx context.Context, profileID string) error {
	if c.automationDeps == nil {
		return nil
	}
	disabled, err := c.automationDeps.DisableAutomationsByAgentProfile(ctx, profileID)
	if err != nil {
		return fmt.Errorf("disable automations using this profile: %w", err)
	}
	if len(disabled) > 0 {
		c.logger.Info("disabled referencing automations before profile delete",
			zap.String("profile_id", profileID), zap.Int("count", len(disabled)))
	}
	return nil
}

// disableReferencingWatchers stamps the deletion cause onto every watcher
// row that referenced this profile so the UI shows "disabled because the
// agent profile was deleted" the moment the request returns. Without this
// eager disable, watchers whose filter no longer matches anything after the
// profile is gone would stay enabled-but-orphaned indefinitely — the
// dispatch coordinator's preflight only runs when a new external event
// fires the watcher.
//
// Best-effort: a failure is logged and ignored so the delete still proceeds.
// The preflight remains as a safety net for reconciler-driven deletes that
// don't pass through this path.
func (c *Controller) disableReferencingWatchers(ctx context.Context, profileID, profileName string) {
	if c.watcherDeps == nil {
		return
	}
	cause := formatDeletedProfileCause(profileID, profileName)
	disabled, err := c.watcherDeps.DisableWatchersByAgentProfile(ctx, profileID, cause)
	if err != nil {
		c.logger.Warn("failed to disable referencing watchers on force-delete",
			zap.String("profile_id", profileID), zap.Error(err))
		return
	}
	if len(disabled) > 0 {
		c.logger.Info("disabled referencing watchers on profile force-delete",
			zap.String("profile_id", profileID), zap.Int("count", len(disabled)))
	}
}

// profileNameCauseMaxLen caps the rendered profile name in the deletion
// cause. Mirrors the orchestrator preflight's cap (80 runes); both strings
// land in the same settings-page watcher banner, and the name is user-typed
// with no DB-level length constraint.
const profileNameCauseMaxLen = 80

// formatDeletedProfileCause renders the human-readable string stamped onto a
// watcher's last_error when its profile is force-deleted. Includes the profile
// name (truncated) so the settings banner shows "Kilo Profile" rather than a
// bare UUID — matching the shape of the orchestrator preflight's cause.
func formatDeletedProfileCause(profileID, profileName string) string {
	name := profileName
	if runes := []rune(name); len(runes) > profileNameCauseMaxLen {
		name = string(runes[:profileNameCauseMaxLen-1]) + "…"
	}
	if name != "" {
		return fmt.Sprintf("agent profile %q (%s) was deleted", name, profileID)
	}
	return fmt.Sprintf("agent profile %s was deleted", profileID)
}

// cleanupEphemeralTasks removes ephemeral tasks (quick chat, config chat) associated with a profile.
func (c *Controller) cleanupEphemeralTasks(ctx context.Context, profileID string) {
	if c.sessionChecker == nil {
		return
	}
	deleted, err := c.sessionChecker.DeleteEphemeralTasksByAgentProfile(ctx, profileID)
	if err != nil {
		c.logger.Warn("failed to delete ephemeral tasks for profile",
			zap.String("profile_id", profileID), zap.Error(err))
		return
	}
	if deleted > 0 {
		c.logger.Info("deleted ephemeral tasks for profile deletion",
			zap.String("profile_id", profileID), zap.Int64("count", deleted))
	}
}

func toAgentDTO(agent *models.Agent, profiles []*models.AgentProfile) dto.AgentDTO {
	profileDTOs := make([]dto.AgentProfileDTO, 0, len(profiles))
	for _, profile := range profiles {
		profileDTOs = append(profileDTOs, toProfileDTO(profile))
	}
	result := dto.AgentDTO{
		ID:            agent.ID,
		Name:          agent.Name,
		WorkspaceID:   agent.WorkspaceID,
		SupportsMCP:   agent.SupportsMCP,
		MCPConfigPath: agent.MCPConfigPath,
		Profiles:      profileDTOs,
		CreatedAt:     agent.CreatedAt,
		UpdatedAt:     agent.UpdatedAt,
	}
	if agent.TUIConfig != nil {
		result.TUIConfig = &dto.TUIConfigDTO{
			Command:         agent.TUIConfig.Command,
			DisplayName:     agent.TUIConfig.DisplayName,
			Model:           agent.TUIConfig.Model,
			Description:     agent.TUIConfig.Description,
			CommandArgs:     agent.TUIConfig.CommandArgs,
			WaitForTerminal: agent.TUIConfig.WaitForTerminal,
			MCPStrategy:     agent.TUIConfig.MCPStrategy,
		}
	}
	return result
}

// decorateAgentDTO attaches the dynamic document only after the parent
// profile list has been assembled. Concrete profiles stay on the existing
// lightweight path, while settings clients get the version and ordered
// candidates for dynamic profiles without reading secrets.
func (c *Controller) decorateAgentDTO(ctx context.Context, result *dto.AgentDTO) error {
	if result == nil {
		return nil
	}
	dynamicRepo, ok := c.repo.(store.DynamicProfileRepository)
	if !ok {
		for _, profile := range result.Profiles {
			if profile.Kind == dynamicProfileKind {
				return fmt.Errorf("dynamic profile store is unavailable")
			}
		}
		return nil
	}
	for index := range result.Profiles {
		if result.Profiles[index].Kind != dynamicProfileKind {
			continue
		}
		config, routes, err := dynamicRepo.GetDynamicAgentProfile(ctx, result.Profiles[index].ID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		result.Profiles[index].Dynamic, err = dynamicProfileDTO(config, routes)
		if err != nil {
			return err
		}
	}
	return nil
}

func toProfileDTO(profile *models.AgentProfile) dto.AgentProfileDTO {
	return dto.AgentProfileDTO{
		ID:               profile.ID,
		AgentID:          profile.AgentID,
		Kind:             profileKind(profile),
		Name:             profile.Name,
		AgentDisplayName: profile.AgentDisplayName,
		Model:            profile.Model,
		FallbackModel:    profile.FallbackModel,
		AutoFallback:     profile.AutoFallback,
		Mode:             profile.Mode,
		ConfigOptions:    profileconfig.SanitizeConfigOptions(profile.ConfigOptions),
		AllowIndexing:    profile.AllowIndexing,
		AutoApprove:      profile.AutoApprove,
		CLIFlags:         cliFlagsToDTO(profile.CLIFlags),
		EnvVars:          envVarsToDTO(profile.EnvVars),
		CLIPassthrough:   profile.CLIPassthrough,
		Enabled:          profile.Enabled,
		CommandPrefix:    profile.CommandPrefix,
		UserModified:     profile.UserModified,
		WorkspaceID:      profile.WorkspaceID,
		CreatedAt:        profile.CreatedAt,
		UpdatedAt:        profile.UpdatedAt,
	}
}

func profileKind(profile *models.AgentProfile) string {
	if profile != nil && profile.AgentID == agents.DynamicAgentID {
		return dynamicProfileKind
	}
	return "concrete"
}

func cliFlagsToDTO(in []models.CLIFlag) []dto.CLIFlagDTO {
	out := make([]dto.CLIFlagDTO, len(in))
	for i, f := range in {
		out[i] = dto.CLIFlagDTO{Description: f.Description, Flag: f.Flag, Enabled: f.Enabled}
	}
	return out
}

func cliFlagsFromDTO(in []dto.CLIFlagDTO) []models.CLIFlag {
	out := make([]models.CLIFlag, len(in))
	for i, f := range in {
		out[i] = models.CLIFlag{Description: f.Description, Flag: f.Flag, Enabled: f.Enabled}
	}
	return out
}

func envVarsToDTO(in []models.ProfileEnvVar) []dto.ProfileEnvVarDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]dto.ProfileEnvVarDTO, len(in))
	for i, ev := range in {
		out[i] = dto.ProfileEnvVarDTO{Key: ev.Key, Value: ev.Value, SecretID: ev.SecretID}
	}
	return out
}

func envVarsFromDTO(in []dto.ProfileEnvVarDTO) []models.ProfileEnvVar {
	out := make([]models.ProfileEnvVar, 0, len(in))
	for _, ev := range in {
		if strings.TrimSpace(ev.Key) == "" {
			continue
		}
		out = append(out, models.ProfileEnvVar{
			Key:      strings.TrimSpace(ev.Key),
			Value:    ev.Value,
			SecretID: ev.SecretID,
		})
	}
	return out
}

const (
	maxProfileEnvVars           = 100
	maxProfileEnvVarKeyLen      = 256
	maxProfileEnvVarValueLen    = 8 * 1024
	reservedProfileEnvVarKey    = "TASK_DESCRIPTION"
	reservedProfileEnvVarPrefix = "KANDEV_"
)

var posixEnvIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validateProfileEnvVarDTOs(in []dto.ProfileEnvVarDTO) error {
	if len(in) > maxProfileEnvVars {
		return fmt.Errorf("%w: at most %d entries allowed", ErrInvalidProfileEnvVars, maxProfileEnvVars)
	}
	seen := make(map[string]int, len(in))
	for i, ev := range in {
		key := strings.TrimSpace(ev.Key)
		if key != ev.Key {
			return fmt.Errorf("%w: env_vars[%d].key must be a POSIX environment identifier", ErrInvalidProfileEnvVars, i)
		}
		if err := validateEnvVarKey(key, i, seen); err != nil {
			return err
		}
		seen[key] = i
		if err := validateEnvVarValue(ev, i); err != nil {
			return err
		}
	}
	return nil
}

func validateEnvVarKey(key string, i int, seen map[string]int) error {
	if key == "" {
		return fmt.Errorf("%w: env_vars[%d].key is required", ErrInvalidProfileEnvVars, i)
	}
	if len(key) > maxProfileEnvVarKeyLen {
		return fmt.Errorf("%w: env_vars[%d].key exceeds %d characters", ErrInvalidProfileEnvVars, i, maxProfileEnvVarKeyLen)
	}
	if !posixEnvIdentifier.MatchString(key) {
		return fmt.Errorf("%w: env_vars[%d].key must be a POSIX environment identifier", ErrInvalidProfileEnvVars, i)
	}
	if strings.HasPrefix(key, reservedProfileEnvVarPrefix) || key == reservedProfileEnvVarKey {
		return fmt.Errorf("%w: env_vars[%d].key %q is reserved", ErrInvalidProfileEnvVars, i, key)
	}
	if first, exists := seen[key]; exists {
		return fmt.Errorf("%w: env_vars[%d].key duplicates env_vars[%d].key", ErrInvalidProfileEnvVars, i, first)
	}
	return nil
}

func validateEnvVarValue(ev dto.ProfileEnvVarDTO, i int) error {
	if ev.SecretID != "" && ev.Value != "" {
		return fmt.Errorf("%w: env_vars[%d]: set value or secret_id, not both", ErrInvalidProfileEnvVars, i)
	}
	if ev.SecretID == "" && ev.Value == "" {
		return fmt.Errorf("%w: env_vars[%d]: must set either value or secret_id", ErrInvalidProfileEnvVars, i)
	}
	if ev.Value != "" {
		if len(ev.Value) > maxProfileEnvVarValueLen {
			return fmt.Errorf("%w: env_vars[%d].value exceeds %d characters", ErrInvalidProfileEnvVars, i, maxProfileEnvVarValueLen)
		}
		if strings.Contains(ev.Value, "\x00") {
			return fmt.Errorf("%w: env_vars[%d].value must not contain null bytes", ErrInvalidProfileEnvVars, i)
		}
	}
	return nil
}
