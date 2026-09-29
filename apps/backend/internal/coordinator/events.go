package coordinator

// CoordinatorUpdatedPayload is the events.CoordinatorUpdated payload
// (docs/specs/coordinator/system-design/coordinators.md#routes, Build
// decision 13). Publishing sites land with tasks 03, 04 and 07; this type is
// their shared shape so every publisher and the gateway forwarder agree on
// the wire fields.
type CoordinatorUpdatedPayload struct {
	WorkspaceID   string `json:"workspace_id"`
	CoordinatorID string `json:"coordinator_id"`
	OpenProposals int    `json:"open_proposals"`
	// AutonomyChanged is true when the event reports a change to autonomy_enabled
	// or the cost ceiling; omitted otherwise.
	AutonomyChanged bool `json:"autonomy_changed,omitempty"`
}

// NewCoordinatorUpdatedPayload builds the events.CoordinatorUpdated payload.
func NewCoordinatorUpdatedPayload(workspaceID, coordinatorID string, openProposals int) CoordinatorUpdatedPayload {
	return CoordinatorUpdatedPayload{
		WorkspaceID:   workspaceID,
		CoordinatorID: coordinatorID,
		OpenProposals: openProposals,
	}
}
