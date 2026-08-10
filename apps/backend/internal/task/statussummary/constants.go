package statussummary

const (
	statusPending = "pending"

	pendingPermission    = "permission"
	pendingClarification = "clarification"

	messageTypeClarificationRequest = "clarification_request"
	messageTypePermissionRequest    = "permission_request"

	activityBackground = "background"
	activityGenerating = "generating"

	prStateMerged   = "merged"
	prStateClosed   = "closed"
	prStateDraft    = "draft"
	prStateBlocked  = "blocked"
	prStateFailure  = "failure"
	prStatePending  = "pending"
	prStateAwaiting = "awaiting_review"
	prStateReady    = "ready"
	prStatePassing  = "passing"
	prStateNeutral  = "neutral"
	prStateOpen     = "open"
	prStateSuccess  = "success"
	prStateApproved = "approved"
	prStateChanges  = "changes_requested"
	prStateDirty    = "dirty"
)
