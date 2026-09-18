package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
)

func (h *Handler) retry(c *gin.Context) {
	owner, _, ok := h.scopedConversation(c)
	if !ok {
		return
	}
	if _, ok := c.Get("agent_claims"); ok {
		c.AbortWithStatus(403)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.Action != "resume" && req.Action != "fresh_start" {
		fail(c, fmt.Errorf("invalid recovery action"))
		return
	}
	run, err := h.Service.Runs.LatestRunForSession(c.Request.Context(), req.SessionID)
	if err != nil || run.AgentProfileID != owner {
		c.AbortWithStatus(404)
		return
	}
	if run.Status != statusFailed && run.Status != "cancelled" {
		fail(c, fmt.Errorf("conversation run is not retryable"))
		return
	}
	var payload map[string]any
	if err = json.Unmarshal([]byte(run.Payload), &payload); err != nil {
		fail(c, err)
		return
	}
	if payload["task_id"] != c.Param("id") {
		c.AbortWithStatus(404)
		return
	}
	if err = h.Service.QueueTurn(c.Request.Context(), owner, c.Param("id"), run.Reason, "retry:"+run.ID, payload); err != nil {
		fail(c, err)
		return
	}
	c.JSON(202, gin.H{"ok": true})
}

// RecoverInterrupted runs before subscriptions and dispatch start. An interrupted
// conversation must be explicitly retried, never replayed after an unknown external write.
func (s *Service) RecoverInterrupted(ctx context.Context) error {
	if err := s.Repo.RecoverOperations(ctx); err != nil {
		return err
	}
	if err := s.Repo.RecoverMaintenance(ctx); err != nil {
		return err
	}
	rows, err := s.Repo.InterruptedRuns(ctx)
	if err != nil {
		return err
	}
	for _, run := range rows {
		if err := s.Runs.RecordFailure(ctx, run.ID, "Conversation interrupted by backend restart. Inspect the latest task results before retrying."); err != nil {
			return err
		}
		if err := s.Runs.FinishRun(ctx, run.ID, statusFailed, nil); err != nil {
			return err
		}
		if err := s.Repo.SetRuntimeWorking(ctx, run.AgentProfileID, false); err != nil {
			return err
		}
	}
	if s.Queue != nil {
		return s.DispatchIntake(ctx)
	}
	return nil
}
