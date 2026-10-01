package coordinator

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/authz"
)

const (
	measuresDefaultDays = 30
	measuresMaxDays     = MaxSummaryDays
	measuresDaysField   = "days"
)

// Null reasons of an outcome measure.
const (
	NullNoData      = "no_data"
	NullCostUnknown = "cost_unknown"
	NullTooFew      = "too_few"
)

// OutcomeMeasure is one measure of the outcomes read with the counts it was
// computed from. Value is nil, never zero, when NullReason is set.
type OutcomeMeasure struct {
	Value       *float64 `json:"value"`
	Numerator   int64    `json:"numerator"`
	Denominator int64    `json:"denominator"`
	NullReason  string   `json:"null_reason,omitempty"`
	Capped      bool     `json:"capped"`
}

// OutcomeMeasures is the response of the measures route.
type OutcomeMeasures struct {
	Days                int            `json:"days"`
	ApprovalWithoutEdit OutcomeMeasure `json:"approval_without_edit"`
	OverrideRecurrence  OutcomeMeasure `json:"override_recurrence"`
	DollarsPerMerged    OutcomeMeasure `json:"dollars_per_merged_task"`
	MedianWaitSeconds   OutcomeMeasure `json:"median_wait_seconds"`
	Agreement           OutcomeMeasure `json:"agreement"`
}

// OutcomeMeasuresReader computes the measures of one coordinator over the
// window ending at now.
type OutcomeMeasuresReader interface {
	Measures(ctx context.Context, coordinatorID string, days int, now time.Time) (*OutcomeMeasures, error)
}

// SetOutcomeMeasures wires the reader behind the measures route.
func (s *Service) SetOutcomeMeasures(r OutcomeMeasuresReader) {
	s.observerMu.Lock()
	defer s.observerMu.Unlock()
	s.outcomeMeasures = r
}

// OutcomeMeasures answers the measures read for one coordinator of the
// workspace. Any workspace member may read; a coordinator of another
// workspace is not found.
func (s *Service) OutcomeMeasures(ctx context.Context, workspaceID, coordinatorID string, days int) (*OutcomeMeasures, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	s.observerMu.RLock()
	reader := s.outcomeMeasures
	s.observerMu.RUnlock()
	if !s.phase31 || reader == nil {
		return nil, ErrNotFound
	}
	coord, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, readFailed("coordinator", err)
	}
	out, err := reader.Measures(ctx, coord.ID, days, s.store.now().UTC())
	if err != nil {
		return nil, readFailed("measures", err)
	}
	return out, nil
}

// parseMeasureDays reads the single optional days query parameter.
func parseMeasureDays(c *gin.Context) (int, error) {
	values, present := c.Request.URL.Query()[measuresDaysField]
	if !present {
		return measuresDefaultDays, nil
	}
	bad := &FieldError{Field: measuresDaysField, Message: "days must be one integer from 1 to 90"}
	if len(values) != 1 {
		return 0, bad
	}
	days, err := strconv.Atoi(values[0])
	if err != nil || days < 1 || days > measuresMaxDays {
		return 0, bad
	}
	return days, nil
}

// httpGetMeasures backs GET /api/v1/workspaces/:id/coordinators/:cid/measures.
func (h *Handlers) httpGetMeasures(c *gin.Context) {
	days, err := parseMeasureDays(c)
	if err != nil {
		h.respondError(c, err)
		return
	}
	out, err := h.service.OutcomeMeasures(c.Request.Context(), c.Param("id"), c.Param("cid"), days)
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
