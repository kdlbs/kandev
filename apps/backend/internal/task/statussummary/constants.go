package statussummary

const (
	statusPending = "pending"

	pendingPermission    = "permission"
	pendingClarification = "clarification"

	messageTypeClarificationRequest = "clarification_request"
	messageTypePermissionRequest    = "permission_request"
	messageTypeUser                 = "user"
	messageTypeError                = "error"
	messageTypeStatus               = "status"
	messageTypeAgent                = "agent"
	messageTypeStatusUpdate         = "status_update"

	sessionStateStarting        = "STARTING"
	sessionStateRunning         = "RUNNING"
	sessionStateWaitingForInput = "WAITING_FOR_INPUT"

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
	prStateQueued   = "queued"
	prStatePassing  = "passing"
	prStateNeutral  = "neutral"
	prStateOpen     = "open"
	prStateSuccess  = "success"
	prStateApproved = "approved"
	prStateChanges  = "changes_requested"
	prStateDirty    = "dirty"
)
