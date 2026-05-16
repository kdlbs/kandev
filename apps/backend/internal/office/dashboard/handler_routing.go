package dashboard

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/routing"
)

// retryProviderRequest is the body for POST /routing/retry.
type retryProviderRequest struct {
	ProviderID string `json:"provider_id"`
}

// validationErrorResponse is the structured 400 the UI consumes when
// ValidateWorkspaceConfig / ValidateAgentOverrides rejects the body.
type validationErrorResponse struct {
	Error   string                     `json:"error"`
	Field   string                     `json:"field"`
	Details []routing.ValidationDetail `json:"details,omitempty"`
}

// getWorkspaceRouting serves GET /workspaces/:wsId/routing.
func (h *Handler) getWorkspaceRouting(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	if rp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing provider not configured"})
		return
	}
	cfg, known, err := rp.GetConfig(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutingConfigResponse{Config: cfg, KnownProviders: known})
}

// updateWorkspaceRouting serves PUT /workspaces/:wsId/routing. Strict
// validation runs when cfg.Enabled is true; structured ValidationError
// becomes a 400 with per-field details.
func (h *Handler) updateWorkspaceRouting(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	if rp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing provider not configured"})
		return
	}
	var cfg routing.WorkspaceConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	wsID := c.Param("wsId")
	if err := rp.UpdateConfig(c.Request.Context(), wsID, cfg); err != nil {
		respondRoutingValidationError(c, err)
		return
	}
	h.publishRoutingSettingsUpdated(c.Request.Context(), wsID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// retryWorkspaceProvider serves POST /workspaces/:wsId/routing/retry.
func (h *Handler) retryWorkspaceProvider(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	if rp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing provider not configured"})
		return
	}
	var req retryProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ProviderID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_id is required"})
		return
	}
	status, retryAt, err := rp.Retry(c.Request.Context(), c.Param("wsId"), req.ProviderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp := RoutingRetryResponse{Status: status}
	if retryAt != nil {
		s := retryAt.UTC().Format(time.RFC3339)
		resp.RetryAt = &s
	}
	c.JSON(http.StatusOK, resp)
}

// listWorkspaceRoutingHealth serves GET /workspaces/:wsId/routing/health.
func (h *Handler) listWorkspaceRoutingHealth(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	if rp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing provider not configured"})
		return
	}
	rows, err := rp.Health(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutingHealthResponse{Health: rows})
}

// getWorkspaceRoutingPreview serves GET /workspaces/:wsId/routing/preview.
func (h *Handler) getWorkspaceRoutingPreview(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	if rp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing provider not configured"})
		return
	}
	items, err := rp.Preview(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RoutingPreviewResponse{Agents: previewItemsToDTOs(items)})
}

// getAgentRoute serves GET /agents/:id/route. Returns the per-agent
// preview shape so the agent detail UI can render configured + current
// route + last failure reason without re-implementing the resolver
// logic.
func (h *Handler) getAgentRoute(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	if rp == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "routing provider not configured"})
		return
	}
	agentID := c.Param("id")
	item, err := rp.PreviewAgent(c.Request.Context(), agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	preview := previewItemsToDTOs([]routing.PreviewItem{*item})[0]
	overrides, ovErr := rp.AgentOverrides(c.Request.Context(), agentID)
	if ovErr != nil {
		// Don't fail the route lookup over a malformed settings blob —
		// log and fall through with the zero overrides so the UI still
		// renders the preview row.
		h.logger.Warn("read agent overrides",
			zap.String("agent_id", agentID), zap.Error(ovErr))
	}
	lastCode, lastRun := lastFailureForAgent(c.Request.Context(), h.svc.RouteAttemptListerImpl(), agentID)
	c.JSON(http.StatusOK, AgentRouteResponse{
		Preview:         preview,
		Overrides:       overrides,
		LastFailureCode: lastCode,
		LastFailureRun:  lastRun,
	})
}

// lastFailureForAgent returns the most recent failed attempt's error
// code + run id for this agent, or ("", "") when there are none. Best
// effort — errors from the lister degrade to no-data so the route page
// still renders.
func lastFailureForAgent(
	_ context.Context, _ RouteAttemptLister, _ string,
) (string, string) {
	// We do not maintain an index on (agent, attempt) today; the
	// per-agent runs paged endpoint is the read path for that
	// telemetry. Surfacing the last failure here would require a
	// dedicated repo helper; v1 returns empty so the UI can
	// silently hide the row when no last-failure is known.
	return "", ""
}

// listRunAttempts serves GET /runs/:id/attempts. Returns the raw
// route_attempts list for a run as DTOs so the UI can refresh without
// fetching the whole run-detail aggregate.
func (h *Handler) listRunAttempts(c *gin.Context) {
	lister := h.svc.RouteAttemptListerImpl()
	if lister == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "route attempts not configured"})
		return
	}
	attempts, err := lister.ListRouteAttempts(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]RouteAttemptDTO, len(attempts))
	for i, a := range attempts {
		out[i] = routeAttemptToDTO(a)
	}
	c.JSON(http.StatusOK, RouteAttemptsResponse{Attempts: out})
}

// previewItemsToDTOs maps the routing-package preview items to the
// dashboard's JSON-shaped DTOs.
func previewItemsToDTOs(items []routing.PreviewItem) []AgentRoutePreview {
	out := make([]AgentRoutePreview, len(items))
	for i, it := range items {
		chain := make([]ProviderModelPair, len(it.FallbackChain))
		for j, p := range it.FallbackChain {
			chain[j] = ProviderModelPair{
				ProviderID: p.ProviderID,
				Model:      p.Model,
				Tier:       p.Tier,
			}
		}
		missing := it.Missing
		if missing == nil {
			missing = []string{}
		}
		out[i] = AgentRoutePreview{
			AgentID:           it.AgentID,
			AgentName:         it.AgentName,
			TierSource:        it.TierSource,
			EffectiveTier:     it.EffectiveTier,
			PrimaryProviderID: it.PrimaryProviderID,
			PrimaryModel:      it.PrimaryModel,
			CurrentProviderID: it.CurrentProviderID,
			CurrentModel:      it.CurrentModel,
			FallbackChain:     chain,
			Missing:           missing,
			Degraded:          it.Degraded,
		}
	}
	return out
}

// respondRoutingValidationError writes a structured 400 for routing
// validation errors. Falls through to 500 for unexpected errors.
func respondRoutingValidationError(c *gin.Context, err error) {
	var ve *routing.ValidationError
	if errors.As(err, &ve) {
		c.JSON(http.StatusBadRequest, validationErrorResponse{
			Error:   ve.Message,
			Field:   ve.Field,
			Details: ve.Details,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// publishRoutingSettingsUpdated emits the routing_settings_updated WS
// event. No-op when no event bus is wired on the dashboard service.
func (h *Handler) publishRoutingSettingsUpdated(
	ctx context.Context, workspaceID string,
) {
	eb := h.svc.EventBus()
	if eb == nil {
		return
	}
	payload := map[string]interface{}{"workspace_id": workspaceID}
	ev := bus.NewEvent(events.OfficeRoutingSettingsUpdated, "office-dashboard", payload)
	if err := eb.Publish(ctx, events.OfficeRoutingSettingsUpdated, ev); err != nil {
		h.logger.Warn("publish routing_settings_updated failed", zap.Error(err))
	}
}
