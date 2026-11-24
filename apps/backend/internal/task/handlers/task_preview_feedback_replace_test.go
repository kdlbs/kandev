package handlers

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestTaskPreviewFeedbackUpdateRoutesCaptureToScreenshotReplace(t *testing.T) {
	h := newPlanTestHandlers(t)
	ctx := context.Background()
	itemID := "6f0f9a1e-9f55-4b7f-8f3c-9a8f3c1e2b10"
	if _, err := h.wsCreateTaskPreviewFeedback(ctx, planMsg(t, "task.preview_feedback.create",
		`{"task_id":"`+planTaskID+`","id":"`+itemID+`",`+
			`"kind":"text","comment":"Wording","source_kind":"browser",`+
			`"source_label":"Local app","page_route":"/","page_title":"Home",`+
			`"selected_text":"Buy","text_anchor":{"start":{"path":[0],"offset":0}}}`)); err != nil {
		t.Fatal(err)
	}

	resp, err := h.wsUpdateTaskPreviewFeedback(ctx, planMsg(t, "task.preview_feedback.update",
		`{"task_id":"`+planTaskID+`","id":"`+itemID+`","expected_version":1,`+
			`"capture":{"capture_rect":{"x":1,"y":2,"width":30,"height":40},`+
			`"screenshot_attachment_id":"staged-shot"}}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	// Without an authenticated caller the replacement cannot claim the image;
	// a comment-only update would instead have failed on the missing comment.
	if payload.Code != ws.ErrorCodeConflict {
		t.Fatalf("payload = %#v, want screenshot replacement path", payload)
	}
}
