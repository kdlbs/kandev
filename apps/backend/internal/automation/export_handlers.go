package automation

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ExportHandler serves the two read-only YAML/zip automation export
// endpoints (see docs/specs/office/requirements/automations-yaml-export.md § API surface).
type ExportHandler struct {
	svc *Service
	log *logger.Logger
}

// NewExportHandler constructs an ExportHandler.
func NewExportHandler(svc *Service, log *logger.Logger) *ExportHandler {
	return &ExportHandler{svc: svc, log: log}
}

// exportStatus applies AC-44's classification: errors.Is(err,
// repoerrors.ErrWorkspaceNotFound) is a 404 (AC-31, AC-35, covering both
// step 1's authorizer and step 2's workspace lookup); any other non-nil
// error is a 500 (AC-45).
func exportStatus(err error) int {
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// ExportDocument serves GET /api/v1/workspaces/:id/automations/export.
// The body is built completely in memory by Service.ExportAutomationsDocument
// before c.Data writes the status line and body (AC-48).
func (h *ExportHandler) ExportDocument(c *gin.Context) {
	body, err := h.svc.ExportAutomationsDocument(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.respondExportError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/yaml", body)
}

// ExportZip serves GET /api/v1/workspaces/:id/automations/export/zip. The
// archive is built completely in memory by Service.ExportAutomationsZip
// before c.Data writes the status line and body (AC-48) — deliberately not
// Office's io.Copy streaming pattern, which cannot retract a 200 already
// sent once a mid-copy failure occurs.
func (h *ExportHandler) ExportZip(c *gin.Context) {
	body, err := h.svc.ExportAutomationsZip(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.respondExportError(c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=kandev-automations.zip")
	c.Data(http.StatusOK, "application/zip", body)
}

func (h *ExportHandler) respondExportError(c *gin.Context, err error) {
	status := exportStatus(err)
	if status == http.StatusInternalServerError {
		h.log.Error("automation export failed", zap.Error(err))
	}
	c.JSON(status, gin.H{responseErrorKey: http.StatusText(status)})
}
