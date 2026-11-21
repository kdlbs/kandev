package models

import (
	"testing"
	"time"
)

func TestLoadWorkflowParkingRoundTripsTypedAndJSONMetadata(t *testing.T) {
	parkedAt := time.Date(2026, 9, 16, 20, 15, 0, 0, time.UTC)
	want := WorkflowParking{
		Stamp:            "parking-1",
		ParkedAt:         parkedAt,
		WorkflowID:       "workflow-1",
		WorkflowStepID:   "step-review",
		RouteOperationID: "route-1",
		EntryIdentity:    "entry:00000000000000000042",
		SourceSessionID:  "session-astra",
		DestinationID:    "session-luna",
	}

	for name, metadata := range map[string]map[string]interface{}{
		"typed": {SessionMetaKeyWorkflowParking: want},
		"json": {
			SessionMetaKeyWorkflowParking: map[string]interface{}{
				"stamp":                  want.Stamp,
				"parked_at":              want.ParkedAt.Format(time.RFC3339Nano),
				"workflow_id":            want.WorkflowID,
				"workflow_step_id":       want.WorkflowStepID,
				"route_operation_id":     want.RouteOperationID,
				"entry_identity":         want.EntryIdentity,
				"source_session_id":      want.SourceSessionID,
				"destination_session_id": want.DestinationID,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := LoadWorkflowParking(metadata)
			if !ok {
				t.Fatal("LoadWorkflowParking returned unavailable")
			}
			if got != want {
				t.Fatalf("parking = %#v, want %#v", got, want)
			}
		})
	}
}

func TestLoadWorkflowParkingRejectsIncompleteMetadata(t *testing.T) {
	for index, metadata := range []map[string]interface{}{
		{},
		{SessionMetaKeyWorkflowParking: map[string]interface{}{"stamp": "parking-1"}},
		{SessionMetaKeyWorkflowParking: map[string]interface{}{
			"stamp":     "parking-1",
			"parked_at": "not-a-time",
		}},
	} {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			if _, ok := LoadWorkflowParking(metadata); ok {
				t.Fatal("incomplete parking metadata was accepted")
			}
		})
	}
}

func TestWorkflowParkingSupersedesLaunch(t *testing.T) {
	parkedAt := time.Date(2026, 9, 16, 20, 15, 0, 0, time.UTC)
	marker := func(stamp, source string) map[string]interface{} {
		return map[string]interface{}{SessionMetaKeyWorkflowParking: WorkflowParking{
			Stamp: stamp, ParkedAt: parkedAt, SourceSessionID: source,
		}}
	}

	for _, test := range []struct {
		name    string
		launch  *TaskSession
		current *TaskSession
		want    bool
	}{
		{
			name:   "new source stamp revokes a launch while state is unchanged",
			launch: &TaskSession{ID: "session-1", State: TaskSessionStateWaitingForInput},
			current: &TaskSession{ID: "session-1", State: TaskSessionStateWaitingForInput,
				Metadata: marker("park-2", "session-1")},
			want: true,
		},
		{
			name: "a changed source stamp revokes a launch before the state changes",
			launch: &TaskSession{ID: "session-1", State: TaskSessionStateStarting,
				Metadata: marker("park-1", "session-1")},
			current: &TaskSession{ID: "session-1", State: TaskSessionStateStarting,
				Metadata: marker("park-2", "session-1")},
			want: true,
		},
		{
			name: "the same stamp permits an explicit resume",
			launch: &TaskSession{ID: "session-1", State: TaskSessionStateWaitingForInput,
				Metadata: marker("park-1", "session-1")},
			current: &TaskSession{ID: "session-1", State: TaskSessionStateWaitingForInput,
				Metadata: marker("park-1", "session-1")},
			want: false,
		},
		{
			name: "the same stamp does not revoke a resumed launch that parks again",
			launch: &TaskSession{ID: "session-1", State: TaskSessionStateStarting,
				Metadata: marker("park-1", "session-1")},
			current: &TaskSession{ID: "session-1", State: TaskSessionStateWaitingForInput,
				Metadata: marker("park-1", "session-1")},
			want: false,
		},
		{
			name:   "another session's marker does not qualify",
			launch: &TaskSession{ID: "session-1", State: TaskSessionStateCreated},
			current: &TaskSession{ID: "session-1", State: TaskSessionStateWaitingForInput,
				Metadata: marker("park-2", "session-2")},
			want: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := WorkflowParkingSupersedesLaunch(test.launch, test.current, test.launch.State); got != test.want {
				t.Fatalf("WorkflowParkingSupersedesLaunch() = %v, want %v", got, test.want)
			}
		})
	}
}
