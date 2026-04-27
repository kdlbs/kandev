package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

// allPermissionMeta returns the ordered list of permission definitions
// for the frontend.
func allPermissionMeta() []dto.PermissionMeta {
	return []dto.PermissionMeta{
		{
			Key:         service.PermCanCreateTasks,
			Label:       "Create tasks",
			Description: "Allow this agent to create new tasks",
			Type:        "bool",
		},
		{
			Key:         service.PermCanAssignTasks,
			Label:       "Assign tasks",
			Description: "Allow this agent to assign tasks to other agents",
			Type:        "bool",
		},
		{
			Key:         service.PermCanCreateAgents,
			Label:       "Create agents",
			Description: "Allow this agent to create new agent instances",
			Type:        "bool",
		},
		{
			Key:         service.PermCanApprove,
			Label:       "Approve requests",
			Description: "Allow this agent to approve or reject approval requests",
			Type:        "bool",
		},
		{
			Key:         service.PermCanManageOwnSkills,
			Label:       "Manage own skills",
			Description: "Allow this agent to create and update its own skills",
			Type:        "bool",
		},
		{
			Key:         service.PermMaxSubtaskDepth,
			Label:       "Max subtask depth",
			Description: "Maximum depth of subtasks this agent can create",
			Type:        "int",
		},
	}
}

// allPermissionDefaults returns the default permissions for each role.
func allPermissionDefaults() map[string]map[string]interface{} {
	roles := []models.AgentRole{
		models.AgentRoleCEO,
		models.AgentRoleWorker,
		models.AgentRoleSpecialist,
		models.AgentRoleAssistant,
	}
	defaults := make(map[string]map[string]interface{}, len(roles))
	for _, r := range roles {
		defaults[string(r)] = service.ResolvePermissions(r, "")
	}
	return defaults
}

// getMeta returns all orchestrate metadata the frontend needs in a single call.
func (h *Handlers) getMeta(c *gin.Context) {
	c.JSON(http.StatusOK, dto.MetaResponse{
		Statuses:           models.AllStatuses(),
		Priorities:         models.AllPriorities(),
		Roles:              models.AllRoles(),
		ExecutorTypes:      models.AllExecutorTypes(),
		SkillSourceTypes:   models.AllSkillSourceTypes(),
		ProjectStatuses:    models.AllProjectStatuses(),
		AgentStatuses:      models.AllAgentStatuses(),
		RoutineRunStatuses: models.AllRoutineRunStatuses(),
		InboxItemTypes:     models.AllInboxItemTypes(),
		Permissions:        allPermissionMeta(),
		PermissionDefaults: allPermissionDefaults(),
	})
}
