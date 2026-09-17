package models

import settings "github.com/kandev/kandev/internal/agent/settings/models"

type AgentInstance = settings.AgentProfile
type AgentStatus = settings.AgentStatus

const (
	AgentRoleAssistant = settings.AgentRoleAssistant
	AgentStatusIdle    = settings.AgentStatusIdle
	AgentStatusWorking = settings.AgentStatusWorking
	AgentStatusPaused  = settings.AgentStatusPaused
)
