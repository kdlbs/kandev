package controller

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/cliflags"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func normalizeAntigravityCLIFlags(agentID string, flags []models.CLIFlag) []models.CLIFlag {
	if agentID != agents.AntigravityACPAgentID {
		return flags
	}
	return cliflags.NormalizeExactFlag(flags, cliflags.AntigravitySkipPermissionsFlag)
}
