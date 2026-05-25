package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

type CreateProfileRequest struct {
	AgentID        string
	Name           string
	Model          string
	Mode           string
	AllowIndexing  bool
	CLIPassthrough bool
	// CLIFlags is the explicit list to persist. When nil, the profile is
	// seeded from the agent's curated PermissionSettings() list so a fresh
	// profile opens with the agent's recommended flags (all disabled by
	// default unless the curated entry specifies Default: true).
	CLIFlags []dto.CLIFlagDTO
	EnvVars  []dto.ProfileEnvVarDTO
}

func (c *Controller) CreateProfile(ctx context.Context, req CreateProfileRequest) (*dto.AgentProfileDTO, error) {
	// Model is optional — the profile reconciler fills it from the host
	// utility probe cache on boot, and session start applies it via
	// session/set_model. An empty model means "use the agent's default".
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
	profile := &models.AgentProfile{
		AgentID:          req.AgentID,
		Name:             req.Name,
		AgentDisplayName: displayName,
		Model:            req.Model,
		Mode:             req.Mode,
		AllowIndexing:    req.AllowIndexing,
		CLIPassthrough:   req.CLIPassthrough,
		CLIFlags:         cliFlags,
		EnvVars:          envVarsFromDTO(req.EnvVars),
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
	settings := agent.PermissionSettings()
	flags := make([]models.CLIFlag, 0, len(settings))
	for _, s := range settings {
		if !s.Supported || s.ApplyMethod != agents.PermissionApplyMethodCLIFlag || s.CLIFlag == "" {
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
	Mode           *string
	AllowIndexing  *bool
	CLIPassthrough *bool
	// CLIFlags replaces the entire list when non-nil. Nil means "leave
	// unchanged" — the UI always sends the full desired list on save.
	CLIFlags *[]dto.CLIFlagDTO
	// EnvVars replaces the entire list when non-nil.
	EnvVars *[]dto.ProfileEnvVarDTO
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
	if req.Mode != nil {
		profile.Mode = *req.Mode
	}
	if req.AllowIndexing != nil {
		profile.AllowIndexing = *req.AllowIndexing
	}
	if req.CLIPassthrough != nil {
		profile.CLIPassthrough = *req.CLIPassthrough
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
		profile.EnvVars = envVarsFromDTO(*req.EnvVars)
	}
	profile.UserModified = true
	if err := c.repo.UpdateAgentProfile(ctx, profile); err != nil {
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
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
	if err := c.repo.DeleteAgentProfile(ctx, id); err != nil {
		if strings.Contains(err.Error(), "agent profile not found") {
			return nil, ErrAgentProfileNotFound
		}
		return nil, err
	}
	result := toProfileDTO(profile)
	return &result, nil
}

// prepareProfileDeletion checks for active sessions and cleans up ephemeral tasks before deletion.
func (c *Controller) prepareProfileDeletion(ctx context.Context, profileID string, force bool) error {
	if c.sessionChecker == nil {
		return nil
	}
	// Check for active non-ephemeral sessions (unless force is true)
	// This allows the UI to show a confirmation dialog with affected tasks
	if !force {
		activeTasks, err := c.sessionChecker.GetActiveTaskInfoByAgentProfile(ctx, profileID)
		if err != nil {
			return err
		}
		if len(activeTasks) > 0 {
			return &ErrProfileInUseDetail{ActiveSessions: activeTasks}
		}
	}
	// Clean up ephemeral tasks (quick chat, config chat) using this profile
	// Done after the force check since these don't need user confirmation
	c.cleanupEphemeralTasks(ctx, profileID)
	return nil
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
		Mode:             profile.Mode,
		AllowIndexing:    profile.AllowIndexing,
		CLIFlags:         cliFlagsToDTO(profile.CLIFlags),
		EnvVars:          envVarsToDTO(profile.EnvVars),
		CLIPassthrough:   profile.CLIPassthrough,
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

func validateProfileEnvVarDTOs(in []dto.ProfileEnvVarDTO) error {
	if len(in) > maxProfileEnvVars {
		return fmt.Errorf("%w: at most %d entries allowed", ErrInvalidProfileEnvVars, maxProfileEnvVars)
	}
	seen := make(map[string]int, len(in))
	for i, ev := range in {
		key := strings.TrimSpace(ev.Key)
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
	if strings.ContainsAny(key, "=\x00") {
		return fmt.Errorf("%w: env_vars[%d].key must not contain '=' or null bytes", ErrInvalidProfileEnvVars, i)
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
