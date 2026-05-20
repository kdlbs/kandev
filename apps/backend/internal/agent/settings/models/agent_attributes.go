package models

// AgentRole is the organisational role of an agent profile under the
// office model. Empty role marks a "shallow" kanban-flavour profile;
// populated roles mark "rich" office-flavour profiles.
//
// ADR 0005 Wave D moved this type from internal/office/models so
// agent_profiles (the unified storage) owns its typed attributes. The
// office package re-exports the type and constants as aliases so the
// ~270 callsites that read models.AgentRole stay valid; new code
// should depend on this package directly.
type AgentRole string

// Office agent role values. Stored verbatim in the agent_profiles.role
// column so DB rows survive the wave-D move unchanged.
const (
	AgentRoleCEO        AgentRole = "ceo"
	AgentRoleWorker     AgentRole = "worker"
	AgentRoleSpecialist AgentRole = "specialist"
	AgentRoleAssistant  AgentRole = "assistant"
	AgentRoleSecurity   AgentRole = "security"
	AgentRoleQA         AgentRole = "qa"
	AgentRoleDevOps     AgentRole = "devops"
)

// AgentStatus represents the runtime status of an agent profile. Stored
// in agent_profiles.status; read by the office scheduler to decide
// whether an agent is eligible for new runs.
type AgentStatus string

// Office agent status values. Stored verbatim in the agent_profiles.status
// column.
const (
	AgentStatusIdle            AgentStatus = "idle"
	AgentStatusWorking         AgentStatus = "working"
	AgentStatusPaused          AgentStatus = "paused"
	AgentStatusStopped         AgentStatus = "stopped"
	AgentStatusPendingApproval AgentStatus = "pending_approval"
)
