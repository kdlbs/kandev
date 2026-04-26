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
	// Stub: return 200
	c.JSON(http.StatusOK, gin.H{"ok": true})
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
	approval, err := h.ctrl.Svc.GetApproval(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	approval.Status = req.Status
	approval.DecisionNote = req.DecisionNote
	approval.DecidedBy = req.DecidedBy
	if err := h.ctrl.Svc.UpdateApproval(c.Request.Context(), approval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"approval": approval})
}

// -- Activity handler --

func (h *Handlers) listActivity(c *gin.Context) {
	entries, err := h.ctrl.Svc.ListActivity(c.Request.Context(), c.Param("wsId"), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.ActivityListResponse{Activity: entries})
}

// -- Inbox handler --

func (h *Handlers) getInbox(c *gin.Context) {
	// Stub: computed view, real implementation in later wave
	c.JSON(http.StatusOK, dto.InboxResponse{Items: []interface{}{}})
}

// -- Memory handlers --

func (h *Handlers) listMemory(c *gin.Context) {
	entries, err := h.ctrl.Svc.ListAgentMemory(c.Request.Context(), c.Param("id"))
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
	entries, err := h.ctrl.Svc.ListAgentMemory(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.MemorySummaryResponse{Count: len(entries)})
}

// -- Dashboard handler --

func (h *Handlers) getDashboard(c *gin.Context) {
	total, active, pending, activity, err := h.ctrl.Svc.GetDashboard(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, dto.DashboardResponse{
		AgentCount:     total,
		ActiveAgents:   active,
		PendingItems:   pending,
		RecentActivity: activity,
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
