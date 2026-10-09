package models

import "testing"

func TestReadCeilingWorkflowEntryBinding(t *testing.T) {
	payload := map[string]interface{}{
		CeilingLaunchEntryBindingKey: map[string]interface{}{
			"workflow_id":            "workflow-1",
			"destination_step_id":    "step-2",
			"route_operation_id":     "route-1",
			"entry_identity":         "entry:00000000000000000007",
			"destination_session_id": "session-2",
		},
	}
	binding, present, err := ReadCeilingWorkflowEntryBinding(payload)
	if err != nil || !present {
		t.Fatalf("ReadCeilingWorkflowEntryBinding() = %#v, %v, %v", binding, present, err)
	}
	if binding.DestinationSessionID != "session-2" || !binding.Valid() {
		t.Fatalf("binding = %#v", binding)
	}
}

func TestReadCeilingLaunchClaim(t *testing.T) {
	claimID, owner, ok := ReadCeilingLaunchClaim(map[string]interface{}{
		CeilingLaunchClaimKey: map[string]interface{}{"id": "claim-1", "owner": "send_now"},
	})
	if !ok || claimID != "claim-1" || owner != "send_now" {
		t.Fatalf("claim = %q, %q, %v", claimID, owner, ok)
	}
}

func TestCeilingDeferralSessionIDResolvesSessionlessStartFromRoute(t *testing.T) {
	task := &Task{
		WorkflowStepID: "step-new",
		Metadata: map[string]interface{}{MetaKeyWorkflowSessionRoute: WorkflowSessionRoute{
			OperationID:       "route-new",
			DestinationStepID: "step-new",
			TargetKind:        "new_session",
			DestinationID:     "session-new",
			Phase:             "committed",
		}},
	}
	deferral := CeilingDeferral{Kind: CeilingLaunchStart, Payload: map[string]interface{}{}}
	if got := CeilingDeferralSessionID(task, deferral); got != "session-new" {
		t.Fatalf("sessionless start destination = %q, want session-new", got)
	}
	if !CeilingDeferralTargetsSession(task, deferral, "session-new") {
		t.Fatal("sessionless start should target the committed route destination")
	}
}

func TestCeilingDeferralTargetsSessionRejectsSuccessorRoute(t *testing.T) {
	task := &Task{
		WorkflowID:     "workflow-1",
		WorkflowStepID: "step-new",
		Metadata: map[string]interface{}{MetaKeyWorkflowSessionRoute: WorkflowSessionRoute{
			OperationID:       "route-new",
			DestinationStepID: "step-new",
			EntryIdentity:     "entry:new",
			TargetKind:        "new_session",
			DestinationID:     "session-new",
			Phase:             "committed",
		}},
	}
	deferral := CeilingDeferral{
		Kind: CeilingLaunchStart,
		Payload: map[string]interface{}{
			CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "workflow-1",
				"destination_step_id":    "step-new",
				"route_operation_id":     "route-old",
				"entry_identity":         "entry:old",
				"destination_session_id": "session-old",
			},
		},
	}
	if CeilingDeferralTargetsSession(task, deferral, "session-new") {
		t.Fatal("old workflow entry must not target the successor route")
	}
}

func TestCeilingDeferralTargetsSessionRejectsBoundRecordWithoutCommittedRoute(t *testing.T) {
	task := &Task{
		WorkflowID:     "workflow-1",
		WorkflowStepID: "step-new",
		Metadata:       map[string]interface{}{},
	}
	deferral := CeilingDeferral{
		Kind: CeilingLaunchStartCreated,
		Payload: map[string]interface{}{
			"session_id": "session-new",
			CeilingLaunchEntryBindingKey: map[string]interface{}{
				"workflow_id":            "workflow-1",
				"destination_step_id":    "step-new",
				"route_operation_id":     "route-old",
				"entry_identity":         "entry:old",
				"destination_session_id": "session-new",
			},
		},
	}
	if CeilingDeferralTargetsSession(task, deferral, "session-new") {
		t.Fatal("a bound deferred entry without a committed route must be unavailable")
	}
}
