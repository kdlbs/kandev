package models

import "time"

// PRWatchTaskActivity is the task-domain activity projection used by
// integration polling. It excludes generic task UpdatedAt values, which
// provider synchronization may update itself.
type PRWatchTaskActivity struct {
	LastActivityAt time.Time
	Running        bool
}
