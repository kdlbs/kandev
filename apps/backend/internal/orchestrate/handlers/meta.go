package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

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
	})
}
