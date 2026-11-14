package dashboard

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"go.uber.org/zap"
)

type controlTaskListRepository interface {
	ListControlTasks(context.Context, string, string, string, int, time.Time) (*sqlite.ControlTaskList, error)
}

// controlTasks serves the opt-in per-task control projection on the existing
// authenticated task list route. The legacy list payload is unchanged when the
// caller does not ask for view=control.
func (h *Handler) controlTasks(c *gin.Context) bool {
	if c.Query("view") != controlView {
		return false
	}
	reader, ok := h.labels.(controlTaskListRepository)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control projection unavailable"})
		return true
	}
	filter := c.DefaultQuery("filter", "all")
	archive := c.DefaultQuery("archive", "active")
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || (limit != -1 && (limit < 1 || limit > 500)) || !controlFilterValid(filter) || !controlArchiveValid(archive) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid control task filter, archive or limit"})
		return true
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	tasks, err := reader.ListControlTasks(ctx, c.Param("wsId"), filter, archive, limit, time.Now().UTC())
	if err != nil {
		h.logger.Warn("control task read failed", zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control projection unavailable"})
		return true
	}
	c.JSON(http.StatusOK, tasks)
	return true
}

func controlFilterValid(filter string) bool {
	switch filter {
	case "all", "active", "sessions", "queued", "hold", "problems", "completed":
		return true
	}
	return false
}

func controlArchiveValid(archive string) bool {
	return archive == "active" || archive == "archived" || archive == "all"
}
