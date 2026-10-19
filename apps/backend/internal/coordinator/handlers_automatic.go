package coordinator

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// eligibilityConditionDTO is one eligibility condition on the wire.
// errorKey is the JSON key of every error body this file writes.
const (
	errorKey = "error"
	fieldKey = "field"
)

type eligibilityConditionDTO struct {
	Name  string `json:"name"`
	Met   bool   `json:"met"`
	Value any    `json:"value"`
}

// eligibilityResponse is the body of GET .../classes/:class/eligibility.
type eligibilityResponse struct {
	Eligible   bool                      `json:"eligible"`
	Conditions []eligibilityConditionDTO `json:"conditions"`
	Setting    Setting                   `json:"setting"`
	ChangedBy  string                    `json:"changed_by"`
	ChangedAt  *time.Time                `json:"changed_at"`
}

// classReviewResponse is the body of a recorded class review.
type classReviewResponse struct {
	ID          string    `json:"id"`
	Class       string    `json:"class"`
	ReviewedBy  string    `json:"reviewed_by"`
	ReviewedAt  time.Time `json:"reviewed_at"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	RowCount    int       `json:"row_count"`
}

func registerAutomaticRoutes(workspace *gin.RouterGroup, h *Handlers) {
	workspace.GET("/coordinators/:cid/classes/:class/eligibility", h.httpGetEligibility)
	workspace.POST("/coordinators/:cid/classes/:class/reviews", h.httpPostClassReview)
}

// httpGetEligibility backs GET .../coordinators/:cid/classes/:class/eligibility.
func (h *Handlers) httpGetEligibility(c *gin.Context) {
	view, err := h.service.GetEligibility(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("class"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	conds := make([]eligibilityConditionDTO, len(view.Result.Conditions))
	for i, cond := range view.Result.Conditions {
		conds[i] = eligibilityConditionDTO(cond)
	}
	c.JSON(http.StatusOK, eligibilityResponse{
		Eligible: view.Result.Eligible, Conditions: conds, Setting: view.Setting,
		ChangedBy: view.ChangedBy, ChangedAt: view.ChangedAt,
	})
}

// httpPostClassReview backs POST .../coordinators/:cid/classes/:class/reviews.
// The body is never read: the server computes every stored field.
func (h *Handlers) httpPostClassReview(c *gin.Context) {
	review, err := h.service.RecordClassReview(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("class"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, classReviewResponse{
		ID: review.ID, Class: string(review.Class), ReviewedBy: review.ReviewedBy, ReviewedAt: review.ReviewedAt,
		WindowStart: review.WindowStart, WindowEnd: review.WindowEnd, RowCount: review.RowCount,
	})
}

// automaticErrorResponse answers the automatic path's typed errors; ok is false
// for every other error.
func automaticErrorResponse(err error) (status int, body gin.H, ok bool) {
	var refused *RaiseRefusedError
	var classErr *ClassError
	var logErr *DecisionLogUnavailableError
	switch {
	case errors.As(err, &refused):
		return http.StatusConflict, gin.H{errorKey: "not_eligible", "code": "not_eligible", fieldKey: "policy.actions." + string(refused.Class), "condition": refused.Condition}, true
	case errors.As(err, &classErr):
		return http.StatusBadRequest, gin.H{errorKey: classErr.Error(), fieldKey: classField, "class": classErr.Class}, true
	case errors.As(err, &logErr):
		return http.StatusServiceUnavailable, gin.H{errorKey: "decision log unavailable", fieldKey: decisionLogField}, true
	}
	return 0, nil, false
}
