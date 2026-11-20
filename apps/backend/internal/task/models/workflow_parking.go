package models

import (
	"encoding/json"
	"strings"
	"time"
)

const SessionMetaKeyWorkflowParking = "workflow_parking"

// WorkflowParking records the workflow ownership of a session that was parked
// while another workflow step became the active destination. The marker is
// durable even when the source has no runtime after a restart.
type WorkflowParking struct {
	Stamp            string    `json:"stamp"`
	ParkedAt         time.Time `json:"parked_at"`
	WorkflowID       string    `json:"workflow_id"`
	WorkflowStepID   string    `json:"workflow_step_id"`
	RouteOperationID string    `json:"route_operation_id"`
	EntryIdentity    string    `json:"entry_identity"`
	SourceSessionID  string    `json:"source_session_id"`
	DestinationID    string    `json:"destination_session_id,omitempty"`
}

// LoadWorkflowParking decodes the bounded parking marker stored in session
// metadata. Invalid or incomplete values are unavailable to callers.
func LoadWorkflowParking(metadata map[string]interface{}) (WorkflowParking, bool) {
	if metadata == nil {
		return WorkflowParking{}, false
	}
	value, ok := metadata[SessionMetaKeyWorkflowParking]
	if !ok || value == nil {
		return WorkflowParking{}, false
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return WorkflowParking{}, false
	}
	var parking WorkflowParking
	if err := json.Unmarshal(payload, &parking); err != nil {
		return WorkflowParking{}, false
	}
	if strings.TrimSpace(parking.Stamp) == "" ||
		parking.ParkedAt.IsZero() ||
		strings.TrimSpace(parking.SourceSessionID) == "" {
		return WorkflowParking{}, false
	}
	return parking, true
}

// WorkflowParkingSupersedesLaunch reports whether current contains a new park
// for the session represented by launch. A matching existing stamp remains
// valid for an explicit resume; a new stamp revokes an older launch even when
// the session state has not changed yet.
func WorkflowParkingSupersedesLaunch(
	launch, current *TaskSession,
	expectedState TaskSessionState,
) bool {
	if launch == nil || current == nil || launch.ID == "" || launch.ID != current.ID {
		return false
	}
	parking, ok := LoadWorkflowParking(current.Metadata)
	if !ok || parking.SourceSessionID != launch.ID {
		return false
	}
	observed, observedOK := LoadWorkflowParking(launch.Metadata)
	if observedOK && observed.SourceSessionID == launch.ID && observed.Stamp == parking.Stamp {
		return false
	}
	if current.State == TaskSessionStateWaitingForInput && current.State != expectedState {
		return true
	}
	return !observedOK || observed.SourceSessionID != launch.ID || observed.Stamp != parking.Stamp
}

// WorkflowParkingSourceStamp returns the stamp observed by a launch for its
// own session. An absent or foreign marker has the empty expected stamp.
func WorkflowParkingSourceStamp(session *TaskSession) string {
	if session == nil {
		return ""
	}
	parking, ok := LoadWorkflowParking(session.Metadata)
	if !ok || parking.SourceSessionID != session.ID {
		return ""
	}
	return parking.Stamp
}
