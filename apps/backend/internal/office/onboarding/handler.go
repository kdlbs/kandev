package onboarding

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"

	"go.uber.org/zap"
)

// Handler provides HTTP handlers for onboarding routes.
type Handler struct {
	svc    *OnboardingService
	logger *logger.Logger
}

// NewHandler creates a new onboarding Handler.
func NewHandler(svc *OnboardingService, log *logger.Logger) *Handler {
	return &Handler{
		svc:    svc,
		logger: log.WithFields(zap.String("component", "office-onboarding-handler")),
	}
}

// RegisterRoutes registers onboarding routes on the given router group.
func RegisterRoutes(api *gin.RouterGroup, svc *OnboardingService, log *logger.Logger) {
	h := NewHandler(svc, log)
	api.GET("/onboarding-state", h.getOnboardingState)
	api.POST("/onboarding/complete", h.completeOnboarding)
	api.POST("/onboarding/import-fs", h.importFromFS)
}

func (h *Handler) getOnboardingState(c *gin.Context) {
	state, err := h.svc.GetOnboardingState(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	fsWorkspaces := make([]OnboardingFSWorkspace, len(state.FSWorkspaces))
	for i, ws := range state.FSWorkspaces {
		fsWorkspaces[i] = OnboardingFSWorkspace(ws)
	}
	c.JSON(http.StatusOK, OnboardingStateResponse{
		Completed:    state.Completed,
		WorkspaceID:  state.WorkspaceID,
		CEOAgentID:   state.CEOAgentID,
		FSWorkspaces: fsWorkspaces,
	})
}

func (h *Handler) completeOnboarding(c *gin.Context) {
	var req OnboardingCompleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.WorkspaceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceName is required"})
		return
	}
	if req.AgentName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agentName is required"})
		return
	}

	result, err := h.svc.CompleteOnboarding(c.Request.Context(), CompleteRequest(req))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, OnboardingCompleteResponse{
		WorkspaceID: result.WorkspaceID,
		AgentID:     result.AgentID,
		TaskID:      result.TaskID,
	})
}

func (h *Handler) importFromFS(c *gin.Context) {
	result, err := h.svc.ImportFromFS(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, OnboardingImportFSResponse{
		WorkspaceIDs:  result.WorkspaceIDs,
		ImportedCount: result.ImportedCount,
	})
}
