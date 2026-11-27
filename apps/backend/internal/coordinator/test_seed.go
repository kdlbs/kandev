package coordinator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	maxSeedRows        = 200
	seedOldestMinDays  = 30
	seedWindowDays     = 29
	seedDetailTemplate = "Seeded decided create_task row %d"
)

// SeedHistoryRequest describes the decided create_task history a test
// coordinator is given: one row OldestDaysAgo days old, then WindowRows rows
// inside the evidence window of which the last EditedRows were approved with
// edits.
type SeedHistoryRequest struct {
	WindowRows    int `json:"window_rows"`
	EditedRows    int `json:"edited_rows"`
	OldestDaysAgo int `json:"oldest_days_ago"`
}

func (r SeedHistoryRequest) validate() error {
	switch {
	case r.WindowRows < 0 || r.WindowRows > maxSeedRows:
		return fmt.Errorf("window_rows must be between 0 and %d", maxSeedRows)
	case r.EditedRows < 0 || r.EditedRows > r.WindowRows:
		return errors.New("edited_rows must be between 0 and window_rows")
	case r.OldestDaysAgo < 0 || r.OldestDaysAgo > 365:
		return errors.New("oldest_days_ago must be between 0 and 365")
	}
	return nil
}

// SeedCreateTaskHistory appends approved, manager-authorized create_task rows
// to the coordinator's log. It exists for the end-to-end suite, which cannot
// wait 30 days; the eligibility rules read the rows exactly as they read a
// real history.
func (s *Service) SeedCreateTaskHistory(ctx context.Context, coordinatorID string, req SeedHistoryRequest) error {
	if err := req.validate(); err != nil {
		return err
	}
	coordinator, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return err
	}
	now := s.store.now().UTC()
	insert := func(n int, at time.Time, edited bool) error {
		return s.store.InsertActivity(ctx, s.store.db, ActivityRow{
			CoordinatorID: coordinator.ID,
			WorkspaceID:   coordinator.WorkspaceID,
			ActionClass:   ActionCreateTask,
			Outcome:       ActivityApproved,
			Authorization: AuthRequiresApproval,
			Detail:        fmt.Sprintf(seedDetailTemplate, n),
			Edited:        edited,
			CreatedAt:     at,
			UpdatedAt:     at,
		})
	}
	if req.OldestDaysAgo > 0 {
		if err := insert(0, now.AddDate(0, 0, -req.OldestDaysAgo), false); err != nil {
			return err
		}
	}
	for i := 0; i < req.WindowRows; i++ {
		daysAgo := seedWindowDays - i%seedWindowDays
		at := now.AddDate(0, 0, -daysAgo).Add(time.Duration(i) * time.Second)
		if err := insert(i+1, at, i >= req.WindowRows-req.EditedRows); err != nil {
			return err
		}
	}
	return nil
}

// RegisterTestSeedRoutes mounts the end-to-end history seed. The caller
// mounts it only while the test harness is enabled.
func RegisterTestSeedRoutes(router gin.IRouter, svc *Service) {
	router.POST("/api/v1/_test/coordinators/:cid/seed-create-task-history", func(c *gin.Context) {
		var req SeedHistoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errorKey: "invalid JSON: " + err.Error()})
			return
		}
		if err := svc.SeedCreateTaskHistory(c.Request.Context(), c.Param("cid"), req); err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrNotFound) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{errorKey: err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})
}
