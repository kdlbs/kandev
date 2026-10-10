package coordinator

import (
	"encoding/json"
	"testing"
)

func TestCoordinatorUpdatedPayloadJSONShape(t *testing.T) {
	payload := NewCoordinatorUpdatedPayload("ws-1", "co-1", 3)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	const want = `{"workspace_id":"ws-1","coordinator_id":"co-1","open_proposals":3}`
	if string(raw) != want {
		t.Errorf("Marshal() = %s, want %s", raw, want)
	}
}

func TestCoordinatorUpdatedPayloadZeroOpenProposals(t *testing.T) {
	payload := NewCoordinatorUpdatedPayload("ws-1", "co-1", 0)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if _, ok := decoded["open_proposals"]; !ok {
		t.Error("open_proposals key was dropped for a zero count; want it always present")
	}
}

func TestCoordinatorUpdatedPayloadGetWorkspaceID(t *testing.T) {
	var data interface{} = NewCoordinatorUpdatedPayload("ws-1", "co-1", 3)
	p, ok := data.(interface{ GetWorkspaceID() string })
	if !ok || p.GetWorkspaceID() != "ws-1" {
		t.Fatalf("payload must expose its workspace id, ok=%v", ok)
	}
}
