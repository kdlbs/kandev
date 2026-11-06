package models

import "time"

// WIPOccupantReason names why an admitted task does or does not hold a slot
// of its workflow step's WIP limit.
type WIPOccupantReason string

const (
	// WIPOccupantReasonLiveSession: at least one session of the task is not
	// terminal, so the task holds its slot.
	WIPOccupantReasonLiveSession WIPOccupantReason = "live_session"
	// WIPOccupantReasonNoLiveSession: no session is live, but the task still
	// holds its slot because the workspace's sessionless release is off or the
	// task's state is not one the release covers.
	WIPOccupantReasonNoLiveSession WIPOccupantReason = "no_live_session"
	// WIPOccupantReasonReleaseGracePending: the release covers the task, and
	// it holds its slot until ReleaseAt.
	WIPOccupantReasonReleaseGracePending WIPOccupantReason = "release_grace_pending"
	// WIPOccupantReasonReleasedNoLiveSession: the release applies, so the task
	// stays admitted in the step but no longer counts toward its limit.
	WIPOccupantReasonReleasedNoLiveSession WIPOccupantReason = "released_no_live_session"
)

// WIPStepOccupant is one admitted task of a workflow step and whether it
// counts toward the step's WIP limit right now.
type WIPStepOccupant struct {
	TaskID           string            `json:"task_id"`
	Title            string            `json:"title"`
	State            string            `json:"state"`
	SessionCount     int               `json:"session_count"`
	LiveSessionCount int               `json:"live_session_count"`
	LastActivityAt   time.Time         `json:"last_activity_at"`
	HoldsSlot        bool              `json:"holds_slot"`
	Reason           WIPOccupantReason `json:"reason"`
	ReleaseAt        *time.Time        `json:"release_at,omitempty"`
}

// WIPStepOccupancy explains a workflow step's WIP slots: its limit, how many
// admitted tasks count toward it, and each admitted task with its reason.
type WIPStepOccupancy struct {
	StepID              string            `json:"step_id"`
	WorkspaceID         string            `json:"workspace_id"`
	WIPLimit            int               `json:"wip_limit"`
	Counted             int               `json:"counted"`
	ReleaseEnabled      bool              `json:"release_enabled"`
	ReleaseGraceMinutes int               `json:"release_grace_minutes"`
	Occupants           []WIPStepOccupant `json:"occupants"`
}
