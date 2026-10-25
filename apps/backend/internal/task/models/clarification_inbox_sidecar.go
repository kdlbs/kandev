package models

import "time"

// ClarificationSidecarState is the two states a Needs-you Inbox sidecar row
// can hold. There is no third value: restore deletes the row rather than
// storing a "visible" state.
type ClarificationSidecarState string

const (
	ClarificationSidecarDismissed ClarificationSidecarState = "dismissed"
	ClarificationSidecarSnoozed   ClarificationSidecarState = "snoozed"
)

// ClarificationInboxHiddenBundle is one row of the hidden-bundles
// enumeration: the same identity a listed bundle carries, plus which sidecar
// state is hiding it and, when snoozed, its expiry.
type ClarificationInboxHiddenBundle struct {
	PendingID   string
	SessionID   string
	TaskID      string
	CreatedAt   time.Time
	State       ClarificationSidecarState
	SnoozeUntil *time.Time
}

// ClarificationInboxHiddenSummary is the bounded aggregate the main read and
// the hidden-bundles read both consult: a workspace-wide count of bundles one
// operator's sidecar is hiding, and the earliest snooze expiry among them.
type ClarificationInboxHiddenSummary struct {
	HiddenCount      int
	NextSnoozeExpiry *time.Time
}
