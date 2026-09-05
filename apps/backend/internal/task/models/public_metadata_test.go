package models

import "testing"

// The transient workflow-move marker carries an internal move ID and encoded
// one-shot instruction text. A public projection (task.updated events, REST
// DTOs while a WIP move is pending) must never expose it.
func TestPublicTaskMetadataStripsWorkflowMoveMarker(t *testing.T) {
	metadata := map[string]interface{}{
		"title_generated": true,
		MetaKeyWorkflowMovePending: map[string]interface{}{
			"from_step_id": "step-1",
			"move_id":      "move-abc",
			"options":      `{"instructions":"secret reviewer note"}`,
		},
	}

	public := PublicTaskMetadata(metadata)

	if _, leaked := public[MetaKeyWorkflowMovePending]; leaked {
		t.Fatal("public metadata must not carry the transient workflow_move_pending marker")
	}
	if _, ok := public["title_generated"]; !ok {
		t.Fatal("public metadata must preserve unrelated keys")
	}
	// The source map is never mutated by the projection.
	if _, ok := metadata[MetaKeyWorkflowMovePending]; !ok {
		t.Fatal("PublicTaskMetadata must not mutate the source metadata map")
	}
}

func TestPublicTaskMetadataNilAndEmpty(t *testing.T) {
	if got := PublicTaskMetadata(nil); got != nil {
		t.Fatalf("nil metadata must project to nil, got %v", got)
	}
	got := PublicTaskMetadata(map[string]interface{}{})
	if got == nil || len(got) != 0 {
		t.Fatalf("empty metadata must project to an empty map, got %v", got)
	}
}
