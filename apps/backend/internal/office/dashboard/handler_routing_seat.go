package dashboard

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/routing"
)

// Errors a SeatAuthorExclusionWriter wraps so the handler can answer with the
// right status. Any other error is a 500.
var (
	ErrSeatChangeInvalid  = errors.New("invalid seat exclusion change")
	ErrSeatChangeNotFound = errors.New("workflow step not found")
	ErrSeatChangeReadOnly = errors.New("workflow is read-only")
	ErrSeatChangeConflict = errors.New("workflow step changed while saving; try again")
)

// SeatAuthorExclusionChange turns exclude_author_provider on or off for one
// step's ensure_participant_seat action for Role.
type SeatAuthorExclusionChange struct {
	StepID  string
	Role    string
	Enabled bool
}

// SeatAuthorExclusionWriter applies seat exclusion changes to a workspace's
// workflow steps. A rejected or failed batch writes nothing.
type SeatAuthorExclusionWriter interface {
	SetSeatAuthorExclusion(ctx context.Context, workspaceID string, changes []SeatAuthorExclusionChange) error
}

// SetSeatAuthorExclusionWriter wires the write path behind
// PUT /workspaces/:wsId/routing/author-exclusion. Optional; nil answers 503.
func (s *DashboardService) SetSeatAuthorExclusionWriter(w SeatAuthorExclusionWriter) {
	s.seatExclusionWriter = w
}

type seatAuthorExclusionRequest struct {
	Changes []struct {
		StepID  string `json:"step_id"`
		Role    string `json:"role"`
		Enabled bool   `json:"enabled"`
	} `json:"changes"`
}

// updateSeatAuthorExclusion serves PUT /workspaces/:wsId/routing/author-exclusion.
func (h *Handler) updateSeatAuthorExclusion(c *gin.Context) {
	rp := h.svc.RoutingProviderImpl()
	writer := h.svc.seatExclusionWriter
	if rp == nil || writer == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "seat exclusion writes are not configured"})
		return
	}
	var req seatAuthorExclusionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	changes := make([]SeatAuthorExclusionChange, 0, len(req.Changes))
	for _, ch := range req.Changes {
		if ch.StepID == "" || ch.Role == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "step_id and role are required"})
			return
		}
		changes = append(changes, SeatAuthorExclusionChange{StepID: ch.StepID, Role: ch.Role, Enabled: ch.Enabled})
	}
	if len(changes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "changes is required"})
		return
	}
	ctx := c.Request.Context()
	wsID := c.Param("wsId")
	if err := writer.SetSeatAuthorExclusion(ctx, wsID, changes); err != nil {
		c.JSON(seatChangeStatus(err), gin.H{"error": err.Error()})
		return
	}
	if anySeatChangeDisables(changes) {
		h.releaseParkedRuns(ctx, rp, wsID)
	}
	steps, err := h.seatSteps(ctx, rp, wsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, SeatAuthorExclusionResponse{
		AuthorExclusionSteps:    steps,
		AuthorExclusionWarnings: h.authorExclusionWarnings(ctx, rp, wsID),
	})
}

// anySeatChangeDisables reports whether a change turns the setting off, which
// can unblock a review parked because only the author's provider was left.
func anySeatChangeDisables(changes []SeatAuthorExclusionChange) bool {
	for _, ch := range changes {
		if !ch.Enabled {
			return true
		}
	}
	return false
}

// releaseParkedRuns re-queues parked runs after the switches are saved. The
// save already succeeded, so a failure here is logged rather than returned;
// the operator can still retry a parked run from its detail page.
func (h *Handler) releaseParkedRuns(ctx context.Context, rp RoutingProvider, workspaceID string) {
	if err := rp.ReleaseParkedRuns(ctx, workspaceID); err != nil && h.logger != nil {
		h.logger.Warn("release parked runs after seat exclusion change failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
	}
}

func seatChangeStatus(err error) int {
	switch {
	case errors.Is(err, ErrSeatChangeInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrSeatChangeNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrSeatChangeReadOnly), errors.Is(err, ErrSeatChangeConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// seatSteps lists the workspace's seat steps for the wire. Never nil.
func (h *Handler) seatSteps(ctx context.Context, rp RoutingProvider, workspaceID string) ([]AuthorExclusionStepDTO, error) {
	steps, err := rp.SeatSteps(ctx, workspaceID)
	if err != nil {
		if h.logger != nil {
			h.logger.Warn("list seat steps failed", zap.String("workspace_id", workspaceID), zap.Error(err))
		}
		return nil, err
	}
	return AuthorExclusionStepDTOs(steps), nil
}

// AuthorExclusionStepDTOs converts seat steps for the wire. Never nil.
func AuthorExclusionStepDTOs(steps []routing.SeatStep) []AuthorExclusionStepDTO {
	out := make([]AuthorExclusionStepDTO, 0, len(steps))
	for _, s := range steps {
		out = append(out, AuthorExclusionStepDTO{
			WorkflowID: s.WorkflowID, WorkflowName: s.WorkflowName, StepID: s.StepID,
			StepName: s.StepName, Role: s.Role, Enabled: s.ExcludeAuthorProvider,
		})
	}
	return out
}
