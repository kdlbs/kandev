package coordinator

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerGoalRoutes adds the goal routes, which exist only while phase 2 is
// on. Handlers pass the raw body to the service, which decodes it after the
// caller is authorized.
func registerGoalRoutes(workspace *gin.RouterGroup, h *Handlers) {
	goal := workspace.Group("/coordinators/:cid/goal")
	goal.GET("", h.httpGetGoal)
	goal.PUT("", h.httpPutGoal)
	goal.POST("/criteria/:crid", h.httpSetGoalCriterion)
	goal.POST("/met", h.httpMarkGoalMet)
}

// readBody returns the raw request body; a read failure is a 400 with no
// field.
func (h *Handlers) readBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.respondError(c, &FieldError{Message: "request body could not be read"})
		return nil, false
	}
	return body, true
}

// httpGetGoal backs GET .../goal.
func (h *Handlers) httpGetGoal(c *gin.Context) {
	view, err := h.service.GetGoal(c.Request.Context(), c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// httpPutGoal backs PUT .../goal.
func (h *Handlers) httpPutGoal(c *gin.Context) {
	body, ok := h.readBody(c)
	if !ok {
		return
	}
	goal, err := h.service.PutGoal(c.Request.Context(), c.Param("id"), c.Param("cid"), body)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, goal)
}

// httpSetGoalCriterion backs POST .../goal/criteria/:crid.
func (h *Handlers) httpSetGoalCriterion(c *gin.Context) {
	body, ok := h.readBody(c)
	if !ok {
		return
	}
	goal, err := h.service.SetGoalCriterionDone(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("crid"), body)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, goal)
}

// httpMarkGoalMet backs POST .../goal/met.
func (h *Handlers) httpMarkGoalMet(c *gin.Context) {
	body, ok := h.readBody(c)
	if !ok {
		return
	}
	goal, err := h.service.MarkGoalMet(c.Request.Context(), c.Param("id"), c.Param("cid"), body)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, goal)
}
