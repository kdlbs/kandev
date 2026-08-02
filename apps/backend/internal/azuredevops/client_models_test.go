package azuredevops

import (
	"encoding/json"
	"testing"
)

func TestWorkItemCommentOmitsAbsentTimestamps(t *testing.T) {
	payload, err := json.Marshal(WorkItemComment{ID: 1, Content: "No timestamps"})
	if err != nil {
		t.Fatalf("marshal work item comment: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("unmarshal work item comment: %v", err)
	}
	if _, found := fields["publishedAt"]; found {
		t.Fatalf("publishedAt serialized for absent timestamp: %s", payload)
	}
	if _, found := fields["updatedAt"]; found {
		t.Fatalf("updatedAt serialized for absent timestamp: %s", payload)
	}
}
