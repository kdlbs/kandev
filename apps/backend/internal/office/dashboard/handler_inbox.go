package dashboard

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/models"
)

// -- Inbox, Activity, Runs --
//
// Split out of handler.go so that file stays under revive's
// file-length-limit. Methods hang off *Handler and use the shared service,
// DTOs (InboxResponse, ActivityListResponse, RunListResponse, etc.),
// and helpers (parseRunPayload, dashboardUserID).

func (h *Handler) getInbox(c *gin.Context) {
	wsID := c.Param("wsId")

	if c.Query("count") == "true" {
		count, err := h.svc.GetInboxCount(c.Request.Context(), wsID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, InboxCountResponse{Count: count})
		return
	}

	items, err := h.svc.GetInboxItems(c.Request.Context(), wsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*models.InboxItem{}
	}
	c.JSON(http.StatusOK, InboxResponse{Items: items, TotalCount: len(items)})
}

// dismissInboxRequest mirrors the JSON the frontend Mark-fixed
// button posts.
type dismissInboxRequest struct {
	Kind   string `json:"kind"`
	ItemID string `json:"item_id"`
}

func (h *Handler) dismissInboxItem(c *gin.Context) {
	if h.svc.MarkFixedHandler() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "inbox dismiss not configured"})
		return
	}
	var req dismissInboxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.Kind == "" || req.ItemID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind and item_id are required"})
		return
	}
	ctx := c.Request.Context()
	switch req.Kind {
	case "agent_run_failed":
		if err := h.svc.MarkFixedHandler().MarkAgentRunFailedFixed(ctx, dashboardUserID, req.ItemID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	case "agent_paused_after_failures":
		if err := h.svc.MarkFixedHandler().MarkAgentPausedFixed(ctx, dashboardUserID, req.ItemID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported kind: " + req.Kind})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// -- Activity --

func (h *Handler) listActivity(c *gin.Context) {
	targetID := c.Query("target_id")
	if targetID != "" {
		entries, err := h.svc.ListActivityForTarget(c.Request.Context(), c.Param("wsId"), targetID, 50)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, ActivityListResponse{Activity: entries})
		return
	}

	filterType := c.Query("type")
	entries, err := h.svc.ListActivityFiltered(c.Request.Context(), c.Param("wsId"), filterType, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ActivityListResponse{Activity: entries})
}

// -- Runs --

func (h *Handler) listRuns(c *gin.Context) {
	runs, err := h.svc.ListRuns(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]RunListItem, len(runs))
	for i, r := range runs {
		items[i] = runToListItem(r)
	}
	c.JSON(http.StatusOK, RunListResponse{Runs: items})
}

// runToListItem flattens a *models.Run into RunListItem and pulls task_id
// out of the JSON-encoded payload (best effort). Times are formatted as
// RFC3339 strings; nil pointer times stay nil.
func runToListItem(r *models.Run) RunListItem {
	if r == nil {
		return RunListItem{}
	}
	taskID, _ := parseRunPayload(r.Payload)
	routingBlocked := ""
	if r.RoutingBlockedStatus != nil {
		routingBlocked = *r.RoutingBlockedStatus
	}
	item := RunListItem{
		ID:                   r.ID,
		AgentProfileID:       r.AgentProfileID,
		Reason:               r.Reason,
		Payload:              r.Payload,
		Status:               r.Status,
		CoalescedCount:       r.CoalescedCount,
		IdempotencyKey:       r.IdempotencyKey,
		ContextSnapshot:      r.ContextSnapshot,
		RetryCount:           r.RetryCount,
		CancelReason:         r.CancelReason,
		ErrorMessage:         r.ErrorMessage,
		RequestedAt:          r.RequestedAt.UTC().Format(time.RFC3339),
		TaskID:               taskID,
		RoutingBlockedStatus: routingBlocked,
	}
	if r.ScheduledRetryAt != nil {
		s := r.ScheduledRetryAt.UTC().Format(time.RFC3339)
		item.ScheduledRetryAt = &s
	}
	if r.ClaimedAt != nil {
		s := r.ClaimedAt.UTC().Format(time.RFC3339)
		item.ClaimedAt = &s
	}
	if r.FinishedAt != nil {
		s := r.FinishedAt.UTC().Format(time.RFC3339)
		item.FinishedAt = &s
	}
	return item
}
