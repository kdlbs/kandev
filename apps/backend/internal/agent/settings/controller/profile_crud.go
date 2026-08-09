package controller

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/profileconfig"
	"github.com/kandev/kandev/internal/secrets"
)

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
	Force         bool
}

func enabledOnlyUpdate(req UpdateProfileRequest) bool {
	return req.Enabled != nil && req.Name == nil && req.Model == nil &&
		req.FallbackModel == nil && req.AutoFallback == nil && req.Mode == nil &&
		req.ConfigOptions == nil && req.AllowIndexing == nil && req.AutoApprove == nil &&
		req.CLIPassthrough == nil && req.CLIFlags == nil && req.EnvVars == nil &&
		req.CommandPrefix == nil
}

func (c *Controller) UpdateProfile(ctx context.Context, req UpdateProfileRequest) (*dto.AgentProfileDTO, error) {
	profile, err := c.repo.GetAgentProfile(ctx, req.ID)
	if err != nil {
		return nil, ErrAgentProfileNotFound
	}
	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Model != nil {
		profile.Model = *req.Model
		if req.Name == nil {
			if newName := c.resolveProfileNameForModel(ctx, profile.AgentID, *req.Model); newName != "" {
				profile.Name = newName
			}
		}
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
	if err := c.repo.UpdateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
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
		if strings.Contains(err.Error(), "agent profile not found") {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
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
		if strings.Contains(err.Error(), "agent profile not found") {
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
			return &ErrProfileInUseDetail{UtilityAgents: utilityRefs}
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
				ActiveSessions: activeTasks,
				Watchers:       watcherRefs,
				Automations:    automationRefs,
				UtilityAgents:  utilityRefs,
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
		}
	}
	return result
}

func toProfileDTO(profile *models.AgentProfile) dto.AgentProfileDTO {
	return dto.AgentProfileDTO{
		ID:               profile.ID,
		AgentID:          profile.AgentID,
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

// resolveProfileNameForModel looks up the agent by ID, fetches its model list (using cache),
// and returns the display name for the given model ID. Returns empty string on failure.
func (c *Controller) resolveProfileNameForModel(ctx context.Context, agentID, modelID string) string {
	agent, err := c.repo.GetAgent(ctx, agentID)
	if err != nil {
		return ""
	}
	if _, ok := c.agentRegistry.Get(agent.Name); !ok {
		return ""
	}

	// Look up the model's display name from the host utility capability
	// cache. If the cache isn't populated yet (probes not finished, agent
	// not probed) we fall through to the raw model ID — better than
	// blocking the save.
	if c.hostUtility != nil {
		if caps, ok := c.hostUtility.Get(agent.Name); ok {
			for _, m := range caps.Models {
				if m.ID == modelID {
					return m.Name
				}
			}
		}
	}
	return modelID
}
