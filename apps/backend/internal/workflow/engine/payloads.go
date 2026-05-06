package engine

// Phase 2 (ADR-0004) trigger payload types. Each new trigger type carries a
// typed payload struct so callbacks can read the trigger's context (comment
// id, blocker ids, etc.). Payloads are passed via HandleInput.Payload and
// surfaced to callbacks via ActionInput.Payload as `any` — callbacks type-
// assert to the matching struct.
//
// Existing kanban triggers (on_enter, on_turn_start, on_turn_complete,
// on_exit) carry no payload — Payload is nil for those triggers and callers
// MUST NOT depend on it.

// OnCommentPayload accompanies TriggerOnComment.
type OnCommentPayload struct {
	CommentID string
	AuthorID  string
	Body      string
}

// OnBlockerResolvedPayload accompanies TriggerOnBlockerResolved.
type OnBlockerResolvedPayload struct {
	ResolvedBlockerIDs []string
}

// ChildSummary describes one completed child task delivered to its parent.
//
// PRLinks is populated from github_task_prs joined on the child task id.
// When a child has no associated PR the slice is empty (nil-equivalent).
// Multiple PRs are possible for multi-repo children.
type ChildSummary struct {
	TaskID  string
	Status  string
	Summary string
	PRLinks []string
}

// OnChildrenCompletedPayload accompanies TriggerOnChildrenCompleted.
type OnChildrenCompletedPayload struct {
	ChildSummaries []ChildSummary
}

// OnApprovalResolvedPayload accompanies TriggerOnApprovalResolved.
type OnApprovalResolvedPayload struct {
	ApprovalID string
	Status     string
	Note       string
}

// OnHeartbeatPayload accompanies TriggerOnHeartbeat. Empty for now; reserved
// so callers can pass timing context once the heartbeat scheduler lands.
type OnHeartbeatPayload struct{}

// OnBudgetAlertPayload accompanies TriggerOnBudgetAlert.
type OnBudgetAlertPayload struct {
	BudgetPct int
	Scope     string
}

// OnAgentErrorPayload accompanies TriggerOnAgentError.
type OnAgentErrorPayload struct {
	FailedAgentID   string
	FailedSessionID string
	ErrorMessage    string
}
