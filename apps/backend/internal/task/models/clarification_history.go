package models

import "time"

// ClarificationHistoryReason is AC-UI-INBOX-HISTORY-001.2's exclusion reason:
// the one, precedence-ordered explanation for why a bundle is unanswerable
// rather than live. Exactly these three values exist; a bundle matching none
// of them is live and is not a history bundle.
type ClarificationHistoryReason string

const (
	// ClarificationHistoryReasonSuperseded: the bundle's turn is no longer
	// its session's current conversational turn, or it is a permission
	// request superseded by a newer one on the same turn.
	ClarificationHistoryReasonSuperseded ClarificationHistoryReason = "superseded"
	// ClarificationHistoryReasonSessionEnded: the owning session ended
	// (COMPLETED/FAILED/CANCELLED) and no later turn superseded it.
	ClarificationHistoryReasonSessionEnded ClarificationHistoryReason = "session_ended"
	// ClarificationHistoryReasonUnreadable: a clarification-only reason -- at
	// least one question in the bundle carries no resolvable identifier.
	ClarificationHistoryReasonUnreadable ClarificationHistoryReason = "unreadable"
)

// ClarificationHistoryBundleSummary is one row of ListInboxHistoryBundles:
// the same bundle identity as ClarificationBundleSummary plus the AC .2
// exclusion reason and the AC .10 turn identity fields.
type ClarificationHistoryBundleSummary struct {
	PendingID string
	SessionID string
	TaskID    string
	CreatedAt time.Time
	// Reason is the AC .2 exclusion reason, already resolved by precedence
	// (superseded, then session_ended, then unreadable).
	Reason ClarificationHistoryReason
	// AskingTurnID is the turn that asked the bundle's question(s).
	AskingTurnID string
	// SupersedingTurnID is the turn that superseded AskingTurnID, set only
	// when Reason is superseded AND the supersession is a turn-level
	// supersession (a later turn started). A same-turn permission
	// supersession (AC .2's second superseded clause) shares AskingTurnID
	// and has no separate superseding turn to name, so this stays empty --
	// AC .10 requires the field omitted, never a fabricated identifier.
	SupersedingTurnID string
	// PermissionGroupKey identifies which logical request this bundle's rows
	// belong to when a permission pending_id is reused across two distinct
	// requests: the request's own request_id, falling back to a message id
	// when request_id is absent. Empty for a clarification bundle, whose
	// pending_id is never reused. Message hydration must filter a
	// pending_id's messages down to this key before rendering, since
	// FindMessagesByPendingIDs returns every message sharing the raw
	// pending_id regardless of which logical request it belongs to.
	PermissionGroupKey string
}

// ListClarificationHistoryOptions filters and paginates
// ListInboxHistoryBundles / CountInboxHistoryBundles. WorkspaceID is
// required (AC .25/R4-3): the history read is always workspace-scoped by an
// already-authorized caller, mirroring the shipped Needs-you Inbox reads.
type ListClarificationHistoryOptions struct {
	WorkspaceID string
	// CursorCreatedAt/CursorPendingID are the last returned (created_at,
	// pending_id) pair (AC .20). An empty CursorPendingID means the first
	// page. Ignored by CountInboxHistoryBundles, which is always the
	// unbounded workspace-wide total.
	CursorCreatedAt time.Time
	CursorPendingID string
	// Limit is the page size, already resolved by the caller to the AC .20
	// default/cap (default 50, capped at 200). Must be >= 1. Ignored by
	// CountInboxHistoryBundles.
	Limit int
}

// ClarificationHistoryPage is one page of ListInboxHistoryBundles, ordered
// per AC .18 (created_at ascending, then pending_id ascending).
type ClarificationHistoryPage struct {
	Bundles []ClarificationHistoryBundleSummary
	// HasMore is true when at least one further bundle exists beyond this
	// page under the same filters (AC .20's next_cursor).
	HasMore bool
}
