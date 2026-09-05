package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type planCommentHandlerContract interface {
	wsListTaskPlanComments(context.Context, *ws.Message) (*ws.Message, error)
	wsCreateTaskPlanComment(context.Context, *ws.Message) (*ws.Message, error)
	wsUpdateTaskPlanComment(context.Context, *ws.Message) (*ws.Message, error)
	wsDeleteTaskPlanComment(context.Context, *ws.Message) (*ws.Message, error)
}

func requirePlanCommentHandlers(t *testing.T, handlers *TaskHandlers) planCommentHandlerContract {
	t.Helper()
	contract, ok := any(handlers).(planCommentHandlerContract)
	if !ok {
		t.Fatal("TaskHandlers does not implement task plan comment actions")
	}
	return contract
}

func TestTaskPlanCommentHandlersCRUDAndConflictSnapshot(t *testing.T) {
	h := newPlanTestHandlers(t)
	ctx := context.Background()
	plan, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: planTaskID, Content: "A plan", CreatedBy: "user",
	})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	handlers := requirePlanCommentHandlers(t, h)

	created, err := handlers.wsCreateTaskPlanComment(ctx, planMsg(t, ws.ActionTaskPlanCommentCreate,
		`{"task_id":"`+planTaskID+`","plan_id":"`+plan.Plan.ID+`",`+
			`"id":"e587551e-cb61-4c12-b1b6-e205ad9c65fb","body":"Clarify",`+
			`"selected_text":"plan","anchor_from":2,"anchor_to":6}`))
	if err != nil {
		t.Fatalf("wsCreateTaskPlanComment: %v", err)
	}
	snapshot := decodePlanCommentSnapshot(t, created)
	if snapshot.Revision != 1 || len(snapshot.Comments) != 1 || snapshot.Comments[0].Version != 1 {
		t.Fatalf("created snapshot = %#v", snapshot)
	}

	updated, err := handlers.wsUpdateTaskPlanComment(ctx, planMsg(t, ws.ActionTaskPlanCommentUpdate,
		`{"task_id":"`+planTaskID+`","plan_id":"`+plan.Plan.ID+`",`+
			`"id":"e587551e-cb61-4c12-b1b6-e205ad9c65fb","body":"Clarify more","expected_version":1}`))
	if err != nil {
		t.Fatalf("wsUpdateTaskPlanComment: %v", err)
	}
	snapshot = decodePlanCommentSnapshot(t, updated)
	if snapshot.Revision != 2 || snapshot.Comments[0].Body != "Clarify more" || snapshot.Comments[0].Version != 2 {
		t.Fatalf("updated snapshot = %#v", snapshot)
	}

	conflict, err := handlers.wsDeleteTaskPlanComment(ctx, planMsg(t, ws.ActionTaskPlanCommentDelete,
		`{"task_id":"`+planTaskID+`","plan_id":"`+plan.Plan.ID+`",`+
			`"id":"e587551e-cb61-4c12-b1b6-e205ad9c65fb","expected_version":1}`))
	assertPlanError(t, conflict, err, ws.ErrorCodePlanCommentsChanged, "Task plan comments changed")
	var conflictPayload ws.ErrorPayload
	if err := json.Unmarshal(conflict.Payload, &conflictPayload); err != nil {
		t.Fatal(err)
	}
	current, ok := conflictPayload.Details["snapshot"].(map[string]any)
	if !ok || current["revision"] != float64(2) {
		t.Fatalf("conflict details = %#v, want current snapshot", conflictPayload.Details)
	}

	listed, err := handlers.wsListTaskPlanComments(ctx, planMsg(t, ws.ActionTaskPlanCommentsList,
		`{"task_id":"`+planTaskID+`"}`))
	if err != nil {
		t.Fatalf("wsListTaskPlanComments: %v", err)
	}
	if got := decodePlanCommentSnapshot(t, listed); got.Revision != 2 || len(got.Comments) != 1 {
		t.Fatalf("listed snapshot = %#v", got)
	}
}

func TestTaskPlanCommentHandlersHideMalformedPayloadDetails(t *testing.T) {
	handlers := requirePlanCommentHandlers(t, newPlanTestHandlers(t))
	response, err := handlers.wsListTaskPlanComments(t.Context(), &ws.Message{
		ID: "malformed", Action: ws.ActionTaskPlanCommentsList, Payload: json.RawMessage(`{"task_id":`),
	})
	if err != nil {
		t.Fatalf("wsListTaskPlanComments: %v", err)
	}
	var payload ws.ErrorPayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Code != ws.ErrorCodeBadRequest || payload.Message != "Invalid payload" {
		t.Fatalf("malformed payload response = %#v", payload)
	}
}

type planCommentSnapshotPayload struct {
	TaskID   string `json:"task_id"`
	PlanID   string `json:"plan_id"`
	Revision int64  `json:"revision"`
	Comments []struct {
		ID      string `json:"id"`
		Body    string `json:"body"`
		Version int64  `json:"version"`
	} `json:"comments"`
}

func decodePlanCommentSnapshot(t *testing.T, msg *ws.Message) planCommentSnapshotPayload {
	t.Helper()
	if msg.Type != ws.MessageTypeResponse {
		t.Fatalf("message type = %q, want response (payload %s)", msg.Type, msg.Payload)
	}
	var snapshot planCommentSnapshotPayload
	if err := json.Unmarshal(msg.Payload, &snapshot); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return snapshot
}
