package testharness

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// recordBackgroundWorkRequest seeds one shared background-workload projection
// row. It lets browser suites cover provider states the deterministic ACP
// producer cannot reach, such as a terminal job whose history must outlive the
// active count.
type recordBackgroundWorkRequest struct {
	SessionID string `json:"session_id"`
	WorkID    string `json:"work_id"`
	RunID     string `json:"run_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	State     string `json:"state"`
	ExitCode  *int   `json:"exit_code"`
	Output    string `json:"output"`
	Revision  int64  `json:"revision"`
}

func recordBackgroundWorkHandler(
	repo *sqliterepo.Repository,
	taskSvc *taskservice.Service,
	eventBus bus.EventBus,
	log *logger.Logger,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req recordBackgroundWorkRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			errJSON(c, http.StatusBadRequest, errInvalidJSONPrefix+err.Error())
			return
		}
		req.SessionID = strings.TrimSpace(req.SessionID)
		req.WorkID = strings.TrimSpace(req.WorkID)
		if req.SessionID == "" || req.WorkID == "" || req.State == "" {
			errJSON(c, http.StatusBadRequest, "session_id, work_id, and state are required")
			return
		}
		session, err := repo.GetTaskSession(c.Request.Context(), req.SessionID)
		if err != nil || session == nil {
			errJSON(c, http.StatusNotFound, "task session not found")
			return
		}
		if taskSvc == nil {
			errJSON(c, http.StatusServiceUnavailable, "task service unavailable")
			return
		}

		obs := streams.WorkloadRunObservation{
			SessionID: req.SessionID,
			WorkID:    req.WorkID,
			RunID:     req.RunID,
			Kind:      streams.NormalizeWorkloadKind(req.Kind),
			Title:     req.Title,
			State:     streams.RunState(req.State),
			ExitCode:  req.ExitCode,
			Output:    req.Output,
			Revision:  req.Revision,
		}
		committed, err := taskSvc.RecordBackgroundWorkloadObservationWithResult(
			c.Request.Context(), obs, session.TaskID, req.SessionID,
		)
		if err != nil {
			log.Warn("failed to seed background workload", zap.Error(err))
			errJSON(c, http.StatusInternalServerError, "failed to record background workload")
			return
		}
		if eventBus != nil {
			subject := events.BuildBackgroundWorkUpdatedSubject(req.SessionID)
			_ = eventBus.Publish(
				c.Request.Context(),
				subject,
				bus.NewEvent(events.BackgroundWorkUpdated, "testharness", &committed),
			)
		}
		c.JSON(http.StatusOK, committed)
	}
}
