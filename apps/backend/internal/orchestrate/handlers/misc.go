package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
	"github.com/kandev/kandev/internal/orchestrate/models"
)

// -- Cost handlers --

func (h *Handlers) listCosts(c *gin.Context) {
	costs, err := h.ctrl.Svc.ListCostEvents(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.CostListResponse{Costs: costs})
}

func (h *Handlers) costsByAgent(c *gin.Context) {
	breakdown, err := h.ctrl.Svc.GetCostsByAgent(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.CostBreakdownResponse{Breakdown: breakdown})
}

func (h *Handlers) costsByProject(c *gin.Context) {
	breakdown, err := h.ctrl.Svc.GetCostsByProject(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.CostBreakdownResponse{Breakdown: breakdown})
}

func (h *Handlers) costSummary(c *gin.Context) {
	total, err := h.ctrl.Svc.GetCostSummary(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"total_cents": total})
}

func (h *Handlers) costsByModel(c *gin.Context) {
	breakdown, err := h.ctrl.Svc.GetCostsByModel(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.CostBreakdownResponse{Breakdown: breakdown})
}

// -- Budget handlers --

func (h *Handlers) listBudgets(c *gin.Context) {
	budgets, err := h.ctrl.Svc.ListBudgetPolicies(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.BudgetListResponse{Budgets: budgets})
}

func (h *Handlers) createBudget(c *gin.Context) {
	var req dto.CreateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy := &models.BudgetPolicy{
		WorkspaceID:       c.Param("wsId"),
		ScopeType:         req.ScopeType,
		ScopeID:           req.ScopeID,
		LimitCents:        req.LimitCents,
		Period:            req.Period,
		AlertThresholdPct: req.AlertThresholdPct,
		ActionOnExceed:    req.ActionOnExceed,
	}
	if err := h.ctrl.Svc.CreateBudgetPolicy(c.Request.Context(), policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"budget": policy})
}

func (h *Handlers) updateBudget(c *gin.Context) {
	var req dto.UpdateBudgetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy, err := h.ctrl.Svc.GetBudgetPolicy(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	applyBudgetPatch(policy, &req)
	if err := h.ctrl.Svc.UpdateBudgetPolicy(c.Request.Context(), policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"budget": policy})
}

func applyBudgetPatch(policy *models.BudgetPolicy, req *dto.UpdateBudgetRequest) {
	if req.ScopeType != nil {
		policy.ScopeType = *req.ScopeType
	}
	if req.ScopeID != nil {
		policy.ScopeID = *req.ScopeID
	}
	if req.LimitCents != nil {
		policy.LimitCents = *req.LimitCents
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

func (h *Handlers) deleteBudget(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteBudgetPolicy(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// -- Approval handlers --

func (h *Handlers) listApprovals(c *gin.Context) {
	approvals, err := h.ctrl.Svc.ListApprovals(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ApprovalListResponse{Approvals: approvals})
}

func (h *Handlers) decideApproval(c *gin.Context) {
	var req dto.DecideApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	approval, err := h.ctrl.Svc.DecideApproval(
		c.Request.Context(), c.Param("id"),
		req.Status, req.DecidedBy, req.DecisionNote,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"approval": approval})
}

// -- Activity handler --

func (h *Handlers) listActivity(c *gin.Context) {
	filterType := c.Query("type")
	entries, err := h.ctrl.Svc.ListActivityFiltered(c.Request.Context(), c.Param("wsId"), filterType, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ActivityListResponse{Activity: entries})
}

// -- Inbox handler --

func (h *Handlers) getInbox(c *gin.Context) {
	wsID := c.Param("wsId")

	// If ?count=true, return just the count for badge display.
	if c.Query("count") == "true" {
		count, err := h.ctrl.Svc.GetInboxCount(c.Request.Context(), wsID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, dto.InboxCountResponse{Count: count})
		return
	}

	items, err := h.ctrl.Svc.GetInboxItems(c.Request.Context(), wsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.InboxResponse{Items: items})
}

// -- Memory handlers --

func (h *Handlers) listMemory(c *gin.Context) {
	layer := c.Query("layer")
	entries, err := h.ctrl.Svc.ListMemory(c.Request.Context(), c.Param("id"), layer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.MemoryListResponse{Memory: entries})
}

func (h *Handlers) upsertMemory(c *gin.Context) {
	var req dto.UpsertMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agentID := c.Param("id")
	for _, entry := range req.Entries {
		mem := &models.AgentMemory{
			AgentInstanceID: agentID,
			Layer:           entry.Layer,
			Key:             entry.Key,
			Content:         entry.Content,
			Metadata:        entry.Metadata,
		}
		if err := h.ctrl.Svc.UpsertAgentMemory(c.Request.Context(), mem); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) deleteMemory(c *gin.Context) {
	if err := h.ctrl.Svc.DeleteAgentMemory(c.Request.Context(), c.Param("entryId")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) memorySummary(c *gin.Context) {
	entries, err := h.ctrl.Svc.GetMemorySummary(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(entries), "memory": entries})
}

// -- Dashboard handler --

func (h *Handlers) getDashboard(c *gin.Context) {
	data, err := h.ctrl.Svc.GetDashboardData(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.DashboardResponse{
		AgentCount:       data.AgentCount,
		RunningCount:     data.RunningCount,
		PausedCount:      data.PausedCount,
		ErrorCount:       data.ErrorCount,
		MonthSpendCents:  data.MonthSpendCents,
		PendingApprovals: data.PendingApprovals,
		RecentActivity:   data.RecentActivity,
	})
}

// -- Wakeup handler --

func (h *Handlers) listWakeups(c *gin.Context) {
	wakeups, err := h.ctrl.Svc.ListWakeupRequests(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.WakeupListResponse{Wakeups: wakeups})
}
