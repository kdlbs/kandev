package models

import "time"

// FailedInboxTaskRow is one row of the Inbox Failed-tab projection: a task
// whose own state is the terminal failed state (AC-UI-INBOX-FAILED-001.6),
// joined to the session AC-UI-INBOX-FAILED-001.30 selects for its failure
// instant and reason. FailureInstant is nil when unresolvable (no session, or
// the chosen session records no completion instant); Reason is "" when no
// reason can be resolved, mirroring the wire contract's own omit/empty rule.
type FailedInboxTaskRow struct {
	TaskID         string
	Title          string
	WorkspaceID    string
	Origin         string
	FailureInstant *time.Time
	Reason         string
}

// ListFailedInboxOptions bounds a single failed-inbox read
// (AC-UI-INBOX-FAILED-001.5, .12, .13). Limit must be >= 1.
type ListFailedInboxOptions struct {
	WorkspaceID string
	Limit       int
}

// FailedInboxPage is one bounded page of failed tasks, ordered per
// AC-UI-INBOX-FAILED-001.10/.10a. HasMore mirrors the bundle read's Limit+1
// probe-row convention (AC-UI-INBOX-FAILED-001.12's truncation signal).
type FailedInboxPage struct {
	Rows    []FailedInboxTaskRow
	HasMore bool
}
