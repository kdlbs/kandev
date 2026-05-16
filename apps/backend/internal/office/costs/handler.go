package costs

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler provides HTTP handlers for cost and budget routes.
type Handler struct {
	svc *CostService
}

// NewHandler creates a new Handler.
func NewHandler(svc *CostService) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers cost and budget routes on the given router group.
func (h *Handler) RegisterRoutes(api *gin.RouterGroup) {
	registerCostRoutes(api, h)
	registerBudgetRoutes(api, h)
}

func registerCostRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/costs", h.listCosts)
	api.GET("/workspaces/:wsId/costs/summary", h.costSummary)
	api.GET("/workspaces/:wsId/costs/by-agent", h.costsByAgent)
	api.GET("/workspaces/:wsId/costs/by-project", h.costsByProject)
	api.GET("/workspaces/:wsId/costs/by-model", h.costsByModel)
	api.GET("/workspaces/:wsId/costs/by-provider", h.costsByProvider)
	api.GET("/workspaces/:wsId/costs/breakdown", h.costsBreakdown)
}

func registerBudgetRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/budgets", h.listBudgets)
	api.POST("/workspaces/:wsId/budgets", h.createBudget)
	api.PATCH("/budgets/:id", h.updateBudget)
	api.DELETE("/budgets/:id", h.deleteBudget)
}

// -- Cost handlers --

func (h *Handler) listCosts(c *gin.Context) {
	costs, err := h.svc.ListCostEvents(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostListResponse{Costs: costs})
}

func (h *Handler) costsByAgent(c *gin.Context) {
	breakdown, err := h.svc.GetCostsByAgent(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostBreakdownResponse{Breakdown: breakdown})
}

func (h *Handler) costsByProject(c *gin.Context) {
	breakdown, err := h.svc.GetCostsByProject(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostBreakdownResponse{Breakdown: breakdown})
}

func (h *Handler) costSummary(c *gin.Context) {
	total, err := h.svc.GetCostSummary(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total_subcents": total})
}

func (h *Handler) costsByModel(c *gin.Context) {
	breakdown, err := h.svc.GetCostsByModel(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostBreakdownResponse{Breakdown: breakdown})
}

func (h *Handler) costsByProvider(c *gin.Context) {
	breakdown, err := h.svc.GetCostsByProvider(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostBreakdownResponse{Breakdown: breakdown})
}

// costsBreakdown returns the composed cost overview (total + by-agent /
// by-project / by-model / by-provider) in a single response so the costs
// page renders in one round-trip (Stream D of office optimization).
func (h *Handler) costsBreakdown(c *gin.Context) {
	total, byAgent, byProject, byModel, byProvider, err := h.svc.GetCostsBreakdown(
		c.Request.Context(), c.Param("wsId"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostBreakdownComposedResponse{
		TotalSubcents: total,
		ByAgent:       byAgent,
		ByProject:     byProject,
		ByModel:       byModel,
		ByProvider:    byProvider,
	})
}

// -- Budget handlers --

func (h *Handler) listBudgets(c *gin.Context) {
	budgets, err := h.svc.ListBudgetPolicies(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, BudgetListResponse{Budgets: budgets})
}

func (h *Handler) createBudget(c *gin.Context) {
	var req CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy := &BudgetPolicy{
		WorkspaceID:       c.Param("wsId"),
		ScopeType:         req.ScopeType,
		ScopeID:           req.ScopeID,
		LimitSubcents:     req.LimitSubcents,
		Period:            req.Period,
		AlertThresholdPct: req.AlertThresholdPct,
		ActionOnExceed:    req.ActionOnExceed,
	}
	if err := h.svc.CreateBudgetPolicy(c.Request.Context(), policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"budget": policy})
}

func (h *Handler) updateBudget(c *gin.Context) {
	var req UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy, err := h.svc.GetBudgetPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	applyBudgetPatch(policy, &req)
	if err := h.svc.UpdateBudgetPolicy(c.Request.Context(), policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"budget": policy})
}

func applyBudgetPatch(policy *BudgetPolicy, req *UpdateBudgetRequest) {
	if req.ScopeType != nil {
		policy.ScopeType = *req.ScopeType
	}
	if req.ScopeID != nil {
		policy.ScopeID = *req.ScopeID
	}
	if req.LimitSubcents != nil {
		policy.LimitSubcents = *req.LimitSubcents
	}
	if req.Period != nil {
		policy.Period = *req.Period
	}
	if req.AlertThresholdPct != nil {
		policy.AlertThresholdPct = *req.AlertThresholdPct
	}
	if req.ActionOnExceed != nil {
		policy.ActionOnExceed = *req.ActionOnExceed
	}
}

func (h *Handler) deleteBudget(c *gin.Context) {
	if err := h.svc.DeleteBudgetPolicy(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
