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
