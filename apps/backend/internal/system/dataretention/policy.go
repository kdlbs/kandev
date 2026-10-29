package dataretention

import (
	"errors"
	"time"
)

// Age is a retention window expressed in whole days. Both approved targets are
// day-granular: the message-metadata window (finished or archived tasks) and the
// finished cleanup-job window.
type Age struct {
	Days int `json:"days"`
}

const maxAgeDays = 3650

func (a Age) Validate() error {
	if a.Days < 1 || a.Days > maxAgeDays {
		return errors.New("invalid_age")
	}
	return nil
}

// Cutoff returns the oldest timestamp a pass may touch. The window is inclusive
// of the cutoff, so a row completed, archived, or last active exactly at the
// boundary is in scope for this pass.
func (a Age) Cutoff(now time.Time) time.Time {
	return now.UTC().AddDate(0, 0, -a.Days)
}

const (
	defaultMessageDays = 14
	defaultCleanupDays = 7
)

// Policy is the administrator's saved choice for both approved targets.
// ArchivedAge is the message-metadata window over finished or archived tasks;
// its persisted name is retained for compatibility. The two ages are independent
// so one can be shortened without touching the other, and Revision makes a stale
// concurrent save a conflict rather than an overwrite.
type Policy struct {
	Enabled     bool  `json:"enabled"`
	ArchivedAge Age   `json:"archived_age"`
	CleanupAge  Age   `json:"cleanup_age"`
	Revision    int64 `json:"revision"`
}

func DefaultPolicy() Policy {
	return Policy{
		ArchivedAge: Age{Days: defaultMessageDays},
		CleanupAge:  Age{Days: defaultCleanupDays},
	}
}
