package models

import (
	"encoding/json"
	"fmt"
)

// CeilingWorkflowEntryBinding identifies the workflow entry that owns a
// deferred launch. The binding is nested in the replay payload so the payload
// remains one immutable launch description while the record's bookkeeping can
// change during capacity retries.
type CeilingWorkflowEntryBinding struct {
	WorkflowID           string `json:"workflow_id"`
	DestinationStepID    string `json:"destination_step_id"`
	RouteOperationID     string `json:"route_operation_id"`
	EntryIdentity        string `json:"entry_identity"`
	DestinationSessionID string `json:"destination_session_id,omitempty"`
}

// Valid reports whether the binding has the identity needed to compare a
// deferred workflow entry with the current committed route. A session ID is
// optional for session-less step starts, where the destination is created only
// after admission.
func (b CeilingWorkflowEntryBinding) Valid() bool {
	return b.WorkflowID != "" &&
		b.DestinationStepID != "" &&
		b.RouteOperationID != "" &&
		b.EntryIdentity != ""
}

// ReadCeilingWorkflowEntryBinding decodes the optional nested binding from a
// replay payload. An absent binding is not an error because older records are
// handled by the legacy unambiguous-route recovery path.
func ReadCeilingWorkflowEntryBinding(payload map[string]interface{}) (CeilingWorkflowEntryBinding, bool, error) {
	if payload == nil {
		return CeilingWorkflowEntryBinding{}, false, nil
	}
	raw, ok := payload[CeilingLaunchEntryBindingKey]
	if !ok || raw == nil {
		return CeilingWorkflowEntryBinding{}, false, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return CeilingWorkflowEntryBinding{}, true, fmt.Errorf("encode workflow entry binding: %w", err)
	}
	var binding CeilingWorkflowEntryBinding
	if err := json.Unmarshal(encoded, &binding); err != nil {
		return CeilingWorkflowEntryBinding{}, true, fmt.Errorf("decode workflow entry binding: %w", err)
	}
	if !binding.Valid() {
		return CeilingWorkflowEntryBinding{}, true, fmt.Errorf("workflow entry binding is incomplete")
	}
	return binding, true, nil
}

// ReadCeilingLaunchClaim reads the short-lived CAS claim from a ceiling
// record. Unknown or malformed values are treated as absent so a damaged
// claim cannot make an otherwise valid launch permanently undispatchable.
func ReadCeilingLaunchClaim(record map[string]interface{}) (claimID, owner string, ok bool) {
	if record == nil {
		return "", "", false
	}
	raw, ok := record[CeilingLaunchClaimKey]
	if !ok || raw == nil {
		return "", "", false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", "", false
	}
	var claim struct {
		ID    string `json:"id"`
		Owner string `json:"owner"`
	}
	if err := json.Unmarshal(encoded, &claim); err != nil {
		return "", "", false
	}
	if claim.ID == "" || claim.Owner == "" {
		return "", "", false
	}
	return claim.ID, claim.Owner, true
}

// CeilingDeferralSessionID returns the durable destination session for a
// ceiling launch when one exists. Seam-1 start records predate a session and
// therefore resolve their destination from the task's committed route.
func CeilingDeferralSessionID(task *Task, deferral CeilingDeferral) string {
	if binding, present, err := ReadCeilingWorkflowEntryBinding(deferral.Payload); err == nil && present {
		if binding.DestinationSessionID != "" {
			return binding.DestinationSessionID
		}
	} else if err != nil {
		return ""
	}
	if deferral.Kind != CeilingLaunchStart {
		value, _ := deferral.Payload["session_id"].(string)
		return value
	}
	if task == nil {
		return ""
	}
	route, present := LoadWorkflowSessionRoute(task.Metadata)
	if !present {
		return ""
	}
	return route.DestinationID
}

// CeilingDeferralTargetsSession checks the destination and, when a nested
// binding is present, the committed route identity. It is used by message
// admission as well as replay so an old task snapshot cannot preserve REVIEW
// or consume a queue entry for a successor workflow entry.
func CeilingDeferralTargetsSession(task *Task, deferral CeilingDeferral, sessionID string) bool {
	if task == nil || sessionID == "" || CeilingDeferralSessionID(task, deferral) != sessionID {
		return false
	}
	binding, present, err := ReadCeilingWorkflowEntryBinding(deferral.Payload)
	if err != nil || !present {
		return err == nil
	}
	route, routePresent := LoadWorkflowSessionRoute(task.Metadata)
	if !routePresent {
		return task.WorkflowStepID == binding.DestinationStepID &&
			(task.WorkflowID == "" || task.WorkflowID == binding.WorkflowID)
	}
	return route.OperationID == binding.RouteOperationID &&
		route.DestinationStepID == binding.DestinationStepID &&
		route.EntryIdentity == binding.EntryIdentity &&
		(route.DestinationID == "" || binding.DestinationSessionID == "" || route.DestinationID == binding.DestinationSessionID)
}
