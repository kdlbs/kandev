package costs

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
)

// errorJSONKey is the JSON field name every handler in this file reports an
// error under. Named to avoid adding a fresh "error" string literal on top
// of this file's many pre-existing gin.H{"error": ...} call sites (goconst).
const errorJSONKey = "error"

// writeBudgetPolicyError maps validateBudgetPolicyWrite's rejection
// (AC-OFFICE-BUDGET-002.6/.7) to 400; any other error (e.g. a repository
// failure) stays 500.
func writeBudgetPolicyError(c *gin.Context, err error) {
	if errors.Is(err, ErrInvalidBudgetPolicy) {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{errorJSONKey: err.Error()})
}

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
	api.GET("/workspaces/:wsId/budgets/default", h.getDefaultCeiling)
	api.PUT("/workspaces/:wsId/budgets/default", h.setDefaultCeiling)
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
	scopeType := models.BudgetScopeType(req.ScopeType)
	period := models.BudgetPeriod(req.Period)
	action := models.BudgetActionOnExceed(req.ActionOnExceed)
	if !scopeType.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_type: " + req.ScopeType})
		return
	}
	if !period.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period: " + req.Period})
		return
	}
	if !action.Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action_on_exceed: " + req.ActionOnExceed})
		return
	}
	policy := &BudgetPolicy{
		WorkspaceID:       c.Param("wsId"),
		ScopeType:         scopeType,
		ScopeID:           req.ScopeID,
		LimitSubcents:     req.LimitSubcents,
		Period:            period,
		AlertThresholdPct: req.AlertThresholdPct,
		ActionOnExceed:    action,
	}
	if err := h.svc.CreateBudgetPolicy(c.Request.Context(), policy); err != nil {
		writeBudgetPolicyError(c, err)
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
	if req.ScopeType != nil && !models.BudgetScopeType(*req.ScopeType).Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scope_type: " + *req.ScopeType})
		return
	}
	if req.Period != nil && !models.BudgetPeriod(*req.Period).Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid period: " + *req.Period})
		return
	}
	if req.ActionOnExceed != nil && !models.BudgetActionOnExceed(*req.ActionOnExceed).Valid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action_on_exceed: " + *req.ActionOnExceed})
		return
	}
	policy, err := h.svc.GetBudgetPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	applyBudgetPatch(policy, &req)
	if err := h.svc.UpdateBudgetPolicy(c.Request.Context(), policy); err != nil {
		writeBudgetPolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"budget": policy})
}

func applyBudgetPatch(policy *BudgetPolicy, req *UpdateBudgetRequest) {
	if req.ScopeType != nil {
		policy.ScopeType = models.BudgetScopeType(*req.ScopeType)
	}
	if req.ScopeID != nil {
		policy.ScopeID = *req.ScopeID
	}
	if req.LimitSubcents != nil {
		policy.LimitSubcents = *req.LimitSubcents
	}
	if req.Period != nil {
		policy.Period = models.BudgetPeriod(*req.Period)
	}
	if req.AlertThresholdPct != nil {
		policy.AlertThresholdPct = *req.AlertThresholdPct
	}
	if req.ActionOnExceed != nil {
		policy.ActionOnExceed = models.BudgetActionOnExceed(*req.ActionOnExceed)
	}
}

func (h *Handler) deleteBudget(c *gin.Context) {
	if err := h.svc.DeleteBudgetPolicy(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// getDefaultCeiling reports the built-in default ceiling's current
// effective limit, visible without inspecting the database
// (AC-OFFICE-BUDGET-003.5).
func (h *Handler) getDefaultCeiling(c *gin.Context) {
	limitSubcents, err := h.svc.GetWorkspaceBudgetDefault(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, DefaultCeilingResponse{LimitSubcents: limitSubcents})
}

// setDefaultCeiling writes the built-in default ceiling's limit through the
// same surface that manages budget policies (AC-OFFICE-BUDGET-003.5), never
// touching office_budget_policies (AC-OFFICE-BUDGET-003.7).
func (h *Handler) setDefaultCeiling(c *gin.Context) {
	var req SetDefaultCeilingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: err.Error()})
		return
	}
	if err := h.svc.SetWorkspaceBudgetDefault(c.Request.Context(), c.Param("wsId"), req.LimitSubcents); err != nil {
		writeBudgetPolicyError(c, err)
		return
	}
	c.JSON(http.StatusOK, DefaultCeilingResponse(req))
}
