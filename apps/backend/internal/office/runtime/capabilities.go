package runtime

import (
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// WildcardTaskScope grants task mutation access to any task in the run's workspace.
const WildcardTaskScope = "*"

// Runtime capability keys. These are the stable syscall vocabulary exposed to
// run tokens, prompts, and runtime API handlers.
const (
	CapabilityPostComment      = "post_comment"
	CapabilityUpdateTaskStatus = "update_task_status"
	CapabilityCreateSubtask    = "create_subtask"
	CapabilityCreateAgent      = "create_agent"
	CapabilityRequestApproval  = "request_approval"
	CapabilityReadMemory       = "read_memory"
	CapabilityWriteMemory      = "write_memory"
	CapabilityListSkills       = "list_skills"
	CapabilitySpawnAgentRun    = "spawn_agent_run"
	CapabilityModifyAgents     = "modify_agents"
	CapabilityDeleteSkills     = "delete_skills"
)

// Allows reports whether the named runtime capability is granted.
func (c Capabilities) Allows(key string) bool {
	switch key {
	case CapabilityPostComment:
		return c.CanPostComments
	case CapabilityUpdateTaskStatus:
		return c.CanUpdateTaskStatus
	case CapabilityCreateSubtask:
		return c.CanCreateSubtasks
	case CapabilityCreateAgent:
		return c.CanCreateAgents
	case CapabilityRequestApproval:
		return c.CanRequestApproval
	case CapabilityReadMemory:
		return c.CanReadMemory
	case CapabilityWriteMemory:
		return c.CanWriteMemory
	case CapabilityListSkills:
		return c.CanListSkills
	case CapabilitySpawnAgentRun:
		return c.CanSpawnAgentRun
	case CapabilityModifyAgents:
		return c.CanModifyAgents
	case CapabilityDeleteSkills:
		return c.CanDeleteSkills
	default:
		return false
	}
}

// WithTaskScope returns a copy of the capabilities with the given task scope.
func (c Capabilities) WithTaskScope(taskIDs ...string) Capabilities {
	next := c
	next.AllowedTaskIDs = append([]string(nil), taskIDs...)
	return next
}

// AllowedKeys returns enabled capability keys in stable order.
func (c Capabilities) AllowedKeys() []string {
	keys := []string{
		CapabilityPostComment,
		CapabilityUpdateTaskStatus,
		CapabilityCreateSubtask,
		CapabilityCreateAgent,
		CapabilityRequestApproval,
		CapabilityReadMemory,
		CapabilityWriteMemory,
		CapabilityListSkills,
		CapabilitySpawnAgentRun,
		CapabilityModifyAgents,
		CapabilityDeleteSkills,
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if c.Allows(key) {
			out = append(out, key)
		}
	}
	return out
}

// FromAgent derives default runtime capabilities from existing Office
// role/permission settings.
func FromAgent(agent *models.AgentInstance) Capabilities {
	if agent == nil {
		return Capabilities{}
	}
	perms := shared.ResolvePermissions(shared.AgentRole(agent.Role), agent.Permissions)
	return Capabilities{
		CanPostComments:     true,
		CanUpdateTaskStatus: true,
		CanCreateSubtasks:   shared.HasPermission(perms, shared.PermCanCreateTasks),
		CanCreateAgents:     shared.HasPermission(perms, shared.PermCanCreateAgents),
		CanRequestApproval:  shared.HasPermission(perms, shared.PermCanApprove),
		CanReadMemory:       true,
		CanWriteMemory:      true,
		CanListSkills:       true,
		CanSpawnAgentRun:    shared.HasPermission(perms, shared.PermCanAssignTasks),
		CanModifyAgents:     shared.HasPermission(perms, shared.PermCanCreateAgents),
		CanDeleteSkills:     false,
	}
}
