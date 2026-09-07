package costs

import (
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// windowStart computes the UTC start of period's spend window ending at at,
// per REQ-OFFICE-BUDGET-002 (docs/specs/office/requirements/budget-measurement-integrity.md).
//
// ok is false for any period value not explicitly handled below, including
// one this build has never seen — callers treat that as "skip this policy"
// (AC-OFFICE-BUDGET-002.5), never as an unbounded lifetime window.
func windowStart(period models.BudgetPeriod, at time.Time) (start time.Time, ok bool) {
	u := at.UTC()
	switch period {
	case models.BudgetPeriodDaily:
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC), true
	case models.BudgetPeriodMonthly:
		return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC), true
	case models.BudgetPeriodYearly:
		return time.Date(u.Year(), time.January, 1, 0, 0, 0, 0, time.UTC), true
	case models.BudgetPeriodTotal:
		return time.Time{}, true
	default:
		return time.Time{}, false
	}
}
