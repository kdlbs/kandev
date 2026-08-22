package models

import "time"

// ClarificationBundleSummary identifies one unresolved clarification bundle
// (spec "external-question-answering" D4a): enough to fetch its full
// messages (FindMessagesByPendingID) and to place it in the L6 pagination
// order.
type ClarificationBundleSummary struct {
	PendingID string
	SessionID string
	TaskID    string
	CreatedAt time.Time
}

// ListClarificationBundlesOptions filters and paginates
// ListUnresolvedClarificationBundles. Every predicate here is applied inside
// the bundle query itself (spec L1a), never as a post-query filter.
type ListClarificationBundlesOptions struct {
	// Unscoped SHALL be true only for a caller with no per-user scoping —
	// visibility is then satisfied unconditionally (spec L1c). When false,
	// VisibleWorkspaceIDs SHALL be the caller's resolved visible-workspace
	// set (filterWorkspacesForCaller); an empty set omits disjunct 3
	// entirely rather than issuing an invalid `IN ()` (spec L1b).
	Unscoped            bool
	VisibleWorkspaceIDs []string
	// WorkspaceID optionally narrows to bundles whose resolved task carries
	// exactly this workspace_id (spec L7/L7a). Empty means unfiltered — it
	// is never read as "workspace_id is empty" (L7a.4).
	WorkspaceID string
	// CreatedSince optionally excludes bundles created before this instant
	// (spec L8). Nil means unfiltered.
	CreatedSince *time.Time
	// CursorCreatedAt/CursorPendingID are the last returned (created_at,
	// pending_id) pair (spec L9/D6). An empty CursorPendingID means the
	// first page.
	CursorCreatedAt time.Time
	CursorPendingID string
	// Limit is the page size, already resolved by the caller to the L10
	// default/cap (default 50, capped at 200). Must be >= 1.
	Limit int
}

// ClarificationBundlePage is one page of ListUnresolvedClarificationBundles,
// ordered per spec L6 (created_at ascending, then pending_id ascending).
type ClarificationBundlePage struct {
	Bundles []ClarificationBundleSummary
	// HasMore is true when at least one further bundle exists beyond this
	// page under the same filters (spec L9's next_cursor).
	HasMore bool
}
