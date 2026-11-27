package testharness

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type seedTaskManagementClaimRequest struct {
	InstallationID string `json:"installation_id"`
	InstanceKey    string `json:"instance_key"`
}

func seedTaskManagementClaimHandler(repo *sqliterepo.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body seedTaskManagementClaimRequest
		if err := c.ShouldBindJSON(&body); err != nil ||
			strings.TrimSpace(body.InstallationID) == "" || strings.TrimSpace(body.InstanceKey) == "" ||
			len(body.InstallationID) > 256 || len(body.InstanceKey) > 256 {
			errJSON(c, http.StatusBadRequest, "installation_id and instance_key are required")
			return
		}
		task, err := repo.GetTask(c.Request.Context(), c.Param("id"))
		if err != nil || task == nil {
			errJSON(c, http.StatusNotFound, "task not found")
			return
		}
		priorClaim, err := repo.GetTaskManagementClaim(c.Request.Context(), task.ID)
		if err != nil {
			log.Error("failed to read E2E task management claim", zap.Error(err))
			errJSON(c, http.StatusInternalServerError, "task management claim could not be read")
			return
		}
		var expectedClaimResourceVersion string
		if priorClaim != nil {
			expectedClaimResourceVersion = priorClaim.ResourceVersion
		}
		claim, err := repo.ChangeTaskManagementClaim(c.Request.Context(), models.TaskManagementClaimChange{
			Action: models.TaskManagementClaimAcquire, TaskID: task.ID, WorkspaceID: task.WorkspaceID,
			ExpectedTaskResourceVersion:  task.UpdatedAt.UTC().Format(time.RFC3339Nano),
			ExpectedClaimResourceVersion: expectedClaimResourceVersion,
			InstallationID:               strings.TrimSpace(body.InstallationID), InstanceKey: strings.TrimSpace(body.InstanceKey),
			ActorID: "plugin:e2e-fixture", Reason: "E2E unavailable manager",
		})
		if err != nil {
			log.Warn("E2E task management claim seed rejected", zap.Error(err))
			errJSON(c, http.StatusConflict, "task management claim could not be seeded")
			return
		}
		c.JSON(http.StatusOK, claim)
	}
}
