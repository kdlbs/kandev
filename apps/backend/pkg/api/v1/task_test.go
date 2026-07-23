package v1

import (
	"encoding/json"
	"testing"
)

func TestMessageAttachmentHasValidDeliveryMode(t *testing.T) {
	tests := []struct {
		name         string
		deliveryMode string
		want         bool
	}{
		{name: "empty defaults to prompt", deliveryMode: "", want: true},
		{name: "prompt", deliveryMode: "prompt", want: true},
		{name: "path", deliveryMode: "path", want: true},
		{name: "invalid", deliveryMode: "inline", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (MessageAttachment{DeliveryMode: tt.deliveryMode}).HasValidDeliveryMode()
			if got != tt.want {
				t.Fatalf("HasValidDeliveryMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskSerializesWorkspaceFolders(t *testing.T) {
	payload, err := json.Marshal(Task{WorkspaceFolders: []TaskWorkspaceFolder{{
		ID: "folder-1", TaskID: "task-1", LocalPath: "/canonical/docs", DisplayName: "docs", Position: 1,
	}}})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	if _, ok := fields["workspace_folders"]; !ok {
		t.Fatalf("workspace_folders missing from task JSON: %s", payload)
	}
}
