package models

import (
	"errors"
	"fmt"
	"time"
)

const (
	SessionWakeDeliveryPending  = "pending"
	SessionWakeDeliverySent     = "sent"
	SessionWakeDeliveryQueued   = "queued"
	SessionWakeDeliveryFailed   = "failed"
	SessionWakeDeliveryTerminal = "terminal"
)

var ErrSessionWakeTerminalDelivery = errors.New("session wake target is terminal")

func NewSessionWakeTerminalDeliveryError(reason string) error {
	return fmt.Errorf("%w: %s", ErrSessionWakeTerminalDelivery, reason)
}

// TaskSessionWake is a durable, task-owned request to wake an existing
// session. It deliberately has no task-creation state: a firing is only a
// prompt delivery to SessionID.
type TaskSessionWake struct {
	ID                 string
	TaskID             string
	SessionID          string
	Marker             string
	Prompt             string
	CronExpression     string
	Timezone           string
	NextRunAt          time.Time
	ExpiresAt          time.Time
	LastAttemptAt      *time.Time
	LastFiredAt        *time.Time
	LastDeliveryStatus string
	LastError          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (w *TaskSessionWake) Expired(now time.Time) bool {
	return w == nil || !w.ExpiresAt.After(now)
}
