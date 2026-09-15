package handlers

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type previewFeedbackHandlerContract interface {
	wsListTaskPreviewFeedback(context.Context, *ws.Message) (*ws.Message, error)
	wsCreateTaskPreviewFeedback(context.Context, *ws.Message) (*ws.Message, error)
	wsUpdateTaskPreviewFeedback(context.Context, *ws.Message) (*ws.Message, error)
	wsDeleteTaskPreviewFeedback(context.Context, *ws.Message) (*ws.Message, error)
	wsClearTaskPreviewFeedback(context.Context, *ws.Message) (*ws.Message, error)
}

func requirePreviewFeedbackHandlers(t *testing.T, handlers *TaskHandlers) previewFeedbackHandlerContract {
	t.Helper()
	contract, ok := any(handlers).(previewFeedbackHandlerContract)
	if !ok {
		t.Fatal("TaskHandlers does not implement task preview feedback actions")
	}
	return contract
}

func TestTaskPreviewFeedbackHandlersCRUDAndConflictSnapshot(t *testing.T) {
	h := newPlanTestHandlers(t)
	handlers := requirePreviewFeedbackHandlers(t, h)
	ctx := context.Background()

	created, err := handlers.wsCreateTaskPreviewFeedback(ctx, planMsg(t, "task.preview_feedback.create",
		`{"task_id":"`+planTaskID+`","id":"e587551e-cb61-4c12-b1b6-e205ad9c65fb",`+
			`"kind":"text","comment":"Increase contrast","source_kind":"browser",`+
			`"source_label":"Local app","page_route":"/products","page_title":"Products",`+
			`"selected_text":"Choose a plan","text_anchor":{"start":{"path":[0],"offset":0}}}`))
	if err != nil {
		t.Fatalf("wsCreateTaskPreviewFeedback: %v", err)
	}
	snapshot := decodePreviewFeedbackSnapshot(t, created)
	if snapshot.Revision != 1 || len(snapshot.Items) != 1 || snapshot.Items[0].Version != 1 {
		t.Fatalf("created snapshot = %#v", snapshot)
	}

	updated, err := handlers.wsUpdateTaskPreviewFeedback(ctx, planMsg(t, "task.preview_feedback.update",
		`{"task_id":"`+planTaskID+`","id":"e587551e-cb61-4c12-b1b6-e205ad9c65fb",`+
			`"comment":"Use stronger contrast","expected_version":1}`))
	if err != nil {
		t.Fatalf("wsUpdateTaskPreviewFeedback: %v", err)
	}
	snapshot = decodePreviewFeedbackSnapshot(t, updated)
	if snapshot.Revision != 2 || snapshot.Items[0].Comment != "Use stronger contrast" {
		t.Fatalf("updated snapshot = %#v", snapshot)
	}

	conflict, err := handlers.wsDeleteTaskPreviewFeedback(ctx, planMsg(t, "task.preview_feedback.delete",
		`{"task_id":"`+planTaskID+`","id":"e587551e-cb61-4c12-b1b6-e205ad9c65fb",`+
			`"expected_version":1}`))
	if err != nil {
		t.Fatalf("wsDeleteTaskPreviewFeedback: %v", err)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(conflict.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	current, ok := payload.Details["snapshot"].(map[string]any)
	if payload.Code != "preview_feedback_changed" || !ok || current["revision"] != float64(2) {
		t.Fatalf("conflict payload = %#v", payload)
	}
}

type previewFeedbackSnapshotPayload struct {
	TaskID   string `json:"task_id"`
	Revision int64  `json:"revision"`
	Items    []struct {
		ID      string `json:"id"`
		Comment string `json:"comment"`
		Version int64  `json:"version"`
	} `json:"items"`
}

func decodePreviewFeedbackSnapshot(t *testing.T, msg *ws.Message) previewFeedbackSnapshotPayload {
	t.Helper()
	if msg.Type != ws.MessageTypeResponse {
		t.Fatalf("message type = %q, want response (payload %s)", msg.Type, msg.Payload)
	}
	var snapshot previewFeedbackSnapshotPayload
	if err := json.Unmarshal(msg.Payload, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return snapshot
}
