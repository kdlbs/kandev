package testharness

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// seedRoutineTriggerRequest seeds an office_routine_triggers row directly,
// bypassing RoutineService.CreateRoutineTrigger's cron validation. The public
// create endpoint rejects an unparseable cron expression outright and always
// computes a non-null NextRunAt, so it cannot produce trigger_invalid,
// trigger_unscheduled, or trigger_disabled — this route is the only way an
// E2E fixture can reach every schedule state in the nine-rule table
// (REQ-OFFICE-ROUTINE-ARMING-001).
type seedRoutineTriggerRequest struct {
	RoutineID      string  `json:"routine_id"`
	Kind           string  `json:"kind"`
	CronExpression string  `json:"cron_expression,omitempty"`
	Timezone       string  `json:"timezone,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	NextRunAt      *string `json:"next_run_at,omitempty"`
	LastFiredAt    *string `json:"last_fired_at,omitempty"`
}

// seedRoutineTriggerHandler inserts the requested trigger row as-is (no
// service-layer validation) so tests can construct otherwise-unreachable
// schedule states.
func seedRoutineTriggerHandler(repo *officesqlite.Repository, log *logger.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		req, ok := decodeSeedRoutineTriggerRequest(c)
		if !ok {
			return
		}
		nextRunAt, err := parseOptionalRFC3339(req.NextRunAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{respKeyError: "next_run_at must be RFC3339"})
			return
		}
		lastFiredAt, err := parseOptionalRFC3339(req.LastFiredAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{respKeyError: "last_fired_at must be RFC3339"})
			return
		}
		trigger := buildSeededTrigger(req, nextRunAt, lastFiredAt)
		if err := repo.CreateRoutineTrigger(c.Request.Context(), trigger); err != nil {
			log.Error("test harness: create routine trigger failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{respKeyError: err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"trigger_id": trigger.ID})
	}
}

func decodeSeedRoutineTriggerRequest(c *gin.Context) (seedRoutineTriggerRequest, bool) {
	var req seedRoutineTriggerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{respKeyError: errInvalidJSONPrefix + err.Error()})
		return seedRoutineTriggerRequest{}, false
	}
	if req.RoutineID == "" || req.Kind == "" {
		c.JSON(http.StatusBadRequest, gin.H{respKeyError: "routine_id and kind are required"})
		return seedRoutineTriggerRequest{}, false
	}
	return req, true
}

func buildSeededTrigger(
	req seedRoutineTriggerRequest, nextRunAt, lastFiredAt *time.Time,
) *officemodels.RoutineTrigger {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	return &officemodels.RoutineTrigger{
		RoutineID:      req.RoutineID,
		Kind:           req.Kind,
		CronExpression: req.CronExpression,
		Timezone:       timezone,
		PublicID:       uuid.New().String(),
		Enabled:        enabled,
		NextRunAt:      nextRunAt,
		LastFiredAt:    lastFiredAt,
	}
}

func parseOptionalRFC3339(raw *string) (*time.Time, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, err
	}
	t = t.UTC()
	return &t, nil
}
