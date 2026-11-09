package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

// httpGetTaskGraph handles GET /api/v1/tasks/:id/graph?mode=subtasks|chain.
func (h *TaskHandlers) httpGetTaskGraph(c *gin.Context) {
	graph, err := h.service.BuildTaskGraph(c.Request.Context(), c.Param("id"), c.Query("mode"))
	if errors.Is(err, service.ErrTaskGraphModeUnavailable) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "graph_mode_unavailable"})
		return
	}
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, taskGraphDTO(graph))
}

func taskGraphDTO(graph *service.TaskGraph) dto.TaskGraphDTO {
	out := dto.TaskGraphDTO{
		Mode: graph.Mode, AvailableModes: graph.AvailableModes, FocusTaskID: graph.FocusTaskID,
		Nodes:     make([]dto.TaskGraphNodeDTO, 0, len(graph.Nodes)),
		Edges:     make([]dto.TaskGraphEdgeDTO, 0, len(graph.Edges)),
		Truncated: graph.Truncated,
	}
	if graph.Root != nil {
		out.Root = &dto.TaskGraphRootDTO{ID: graph.Root.ID, Title: graph.Root.Title}
	}
	for _, n := range graph.Nodes {
		out.Nodes = append(out.Nodes, dto.TaskGraphNodeDTO{
			ID: n.Task.ID, Title: n.Task.Title, State: n.Task.State,
			ParentID: n.Task.ParentID, ParentTitle: n.ParentTitle, Depth: n.Depth, Role: n.Role,
			Archived: n.Task.ArchivedAt != nil, Blocked: n.View.Blocked, BlockedReason: n.View.BlockedReason,
		})
	}
	for _, e := range graph.Edges {
		out.Edges = append(out.Edges, dto.TaskGraphEdgeDTO{From: e.From, To: e.To, Kind: e.Kind, Status: e.Status})
	}
	return out
}
