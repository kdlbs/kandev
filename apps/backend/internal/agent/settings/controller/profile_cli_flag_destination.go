package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

// ErrPassthroughOnlyCLIFlag marks a CLI flag saved on a profile that launches
// over ACP while the agent declares it reachable only in CLI passthrough mode.
var ErrPassthroughOnlyCLIFlag = errors.New("cli flag is only available in CLI passthrough mode")

// validatePassthroughOnlyCLIFlags rejects a flag that would be appended to the
// ACP bridge process rather than the agent CLI it wraps.
//
// Enabling such a flag used to succeed and change nothing: the token landed on
// a process that ignores it while the profile editor reported it as enabled.
// The error names the ACP control that achieves the same thing, because a
// refusal without a next step is not actionable.
func validatePassthroughOnlyCLIFlags(agentConfig agents.Agent, flags []dto.CLIFlagDTO, cliPassthrough bool) error {
	if agentConfig == nil || cliPassthrough || len(flags) == 0 {
		return nil
	}
	restricted := map[string]agents.PermissionSetting{}
	for _, setting := range agents.CatalogPermissionSettings(agentConfig) {
		if setting.PassthroughOnly && setting.CLIFlag != "" {
			restricted[setting.CLIFlag] = setting
		}
	}
	if len(restricted) == 0 {
		return nil
	}
	// Judge the resolved tokens, not the raw entry: a multi-token entry would
	// otherwise hide a restricted flag behind its neighbours.
	tokens, _ := cliflags.Resolve(cliFlagsFromDTO(flags))
	for _, token := range tokens {
		setting, restrictedFlag := restricted[token]
		if !restrictedFlag {
			continue
		}
		equivalent := setting.ACPEquivalent
		if equivalent == "" {
			equivalent = "this agent's equivalent permission control"
		}
		return fmt.Errorf("%w: %s reaches %s only in CLI passthrough mode; use %s instead",
			ErrPassthroughOnlyCLIFlag, token, agentConfig.DisplayName(), equivalent)
	}
	return nil
}

// agentConfigForProfile resolves the registry entry backing a stored profile.
func (c *Controller) agentConfigForProfile(ctx context.Context, profile *models.AgentProfile) (agents.Agent, bool) {
	if profile == nil || c.agentRegistry == nil {
		return nil, false
	}
	agent, err := c.repo.GetAgent(ctx, profile.AgentID)
	if err != nil || agent == nil {
		return nil, false
	}
	return c.agentRegistry.Get(agent.Name)
}
