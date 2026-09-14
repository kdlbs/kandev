// @covers AC-TASKS-PLAN-COMMENTS-002.8
package handlers

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/plancomments"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestPlanCommentQueueSizeError(t *testing.T) {
	response := planCommentQueueError(&ws.Message{ID: "size-check", Action: "queue.add"}, plancomments.ErrRenderedTooLarge)
	if response == nil {
		t.Fatal("rendered size rejection was not mapped")
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Code != ws.ErrorCodeValidation {
		t.Fatalf("size rejection code = %s", payload.Code)
	}
}
