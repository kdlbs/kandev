package handlers

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type mcpPlanEventRecorder struct {
	bus.EventBus
	events []*bus.Event
}

func (r *mcpPlanEventRecorder) Publish(_ context.Context, _ string, event *bus.Event) error {
	r.events = append(r.events, event)
	return nil
}

func TestMCPPlanExactEditRejectsMissingOrNullNewTextWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{name: "omitted", payload: `{"task_id":"` + mcpPlanTaskID + `","old_text":"remove"}`},
		{name: "null", payload: `{"task_id":"` + mcpPlanTaskID + `","old_text":"remove","new_text":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo := newMCPPlanTestHandlersWithRepo(t)
			recorder := &mcpPlanEventRecorder{}
			h.planService = service.NewPlanService(repo, recorder, h.logger)
			ctx := context.Background()
			createdOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
				mustMarshalPlanPayload(t, map[string]any{
					"task_id": mcpPlanTaskID, "content": "keep remove",
				})))
			if err != nil {
				t.Fatalf("handleCreateTaskPlan: %v", err)
			}
			version := decodeMCPPlanPayload(t, createdOut)["version"].(string)
			tc.payload = strings.Replace(tc.payload, `"old_text"`, `"expected_version":"`+version+`","old_text"`, 1)
			recorder.events = nil

			beforePlan, err := h.planService.GetPlan(ctx, mcpPlanTaskID)
			if err != nil {
				t.Fatalf("GetPlan before invalid edit: %v", err)
			}
			beforeHistory, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
			if err != nil {
				t.Fatalf("ListRevisions before invalid edit: %v", err)
			}

			out, handleErr := h.handleEditTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPEditTaskPlan, tc.payload))
			if handleErr != nil {
				t.Fatalf("handleEditTaskPlan: %v", handleErr)
			}
			if out.Type != ws.MessageTypeError {
				t.Fatalf("message type = %q, want error", out.Type)
			}
			var payload ws.ErrorPayload
			if err := json.Unmarshal(out.Payload, &payload); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if payload.Code != ws.ErrorCodeValidation || payload.Message != "new_text is required; use an empty string to delete the match" {
				t.Fatalf("error payload = %#v, want validation for missing new_text", payload)
			}

			gotPlan, err := h.planService.GetPlan(ctx, mcpPlanTaskID)
			if err != nil {
				t.Fatalf("GetPlan after invalid edit: %v", err)
			}
			if !reflect.DeepEqual(gotPlan, beforePlan) {
				t.Fatalf("plan changed after invalid edit: got %#v, want %#v", gotPlan, beforePlan)
			}
			gotHistory, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
			if err != nil {
				t.Fatalf("ListRevisions after invalid edit: %v", err)
			}
			if !reflect.DeepEqual(gotHistory, beforeHistory) {
				t.Fatalf("history changed after invalid edit: got %#v, want %#v", gotHistory, beforeHistory)
			}
			if len(recorder.events) != 0 {
				t.Fatalf("invalid edit published %d events", len(recorder.events))
			}
		})
	}
}

func TestMCPPlanReplaceRequiresVersionAndReturnsSafeDetails(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	large := strings.Repeat("x", 4000)
	createdOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{"task_id": mcpPlanTaskID, "content": large})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	created := decodeMCPPlanPayload(t, createdOut)
	version, ok := created["version"].(string)
	if !ok || version == "" {
		t.Fatalf("create response version = %v, want a non-empty string", created["version"])
	}

	updateOut, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": strings.Repeat("y", 800), "mode": "replace",
		})))
	if err != nil {
		t.Fatalf("handleUpdateTaskPlan: %v", err)
	}
	var rejection ws.ErrorPayload
	if err := json.Unmarshal(updateOut.Payload, &rejection); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if updateOut.Type != ws.MessageTypeError || rejection.Code != ws.ErrorCodeValidation {
		t.Fatalf("rejection = type %q/code %q, want error/validation", updateOut.Type, rejection.Code)
	}
	if rejection.Details["reason"] != "plan_version_required" || rejection.Details["write_applied"] != false {
		t.Fatalf("rejection details = %#v, want version-required and no write", rejection.Details)
	}
	if rejection.Details["next_action"] == "" {
		t.Fatal("rejection omitted next_action correction guidance")
	}
	plan, err := h.planService.GetPlan(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("GetPlan after rejection: %v", err)
	}
	if plan.Content != large || plan.WriteVersion != version {
		t.Fatalf("plan changed after rejection: len=%d/version=%q", len(plan.Content), plan.WriteVersion)
	}
}

func TestMCPPlanReplaceEchoesCommittedVersionAndAppendDoesNotRequireIt(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	createdOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{"task_id": mcpPlanTaskID, "content": "before"})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	created := decodeMCPPlanPayload(t, createdOut)
	version := created["version"].(string)

	updatedOut, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": "after", "mode": "replace", "expected_version": version,
		})))
	if err != nil {
		t.Fatalf("handleTaskPlan update: %v", err)
	}
	updated := decodeMCPPlanPayload(t, updatedOut)
	updatedVersion, ok := updated["version"].(string)
	if !ok || updatedVersion == "" || updatedVersion == version {
		t.Fatalf("update response version = %v, want a new non-empty version", updated["version"])
	}

	appendOut, err := h.handleUpdateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPUpdateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": "appended", "mode": "append",
		})))
	if err != nil {
		t.Fatalf("append without expected version: %v", err)
	}
	appended := decodeMCPPlanPayload(t, appendOut)
	if appended["content"] != "after\n\nappended" {
		t.Fatalf("appended content = %v, want composed plan", appended["content"])
	}
}

func TestMCPPlanExactEditBridgesVersionAndPreservesContent(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	createdOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "content": "before\r\n- [ ] task\r\nafter",
		})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	version := decodeMCPPlanPayload(t, createdOut)["version"].(string)

	out, err := h.handleEditTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPEditTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "expected_version": version,
			"old_text": "- [ ] task", "new_text": "- [x] task",
		})))
	if err != nil {
		t.Fatalf("handleEditTaskPlan: %v", err)
	}
	updated := decodeMCPPlanPayload(t, out)
	if updated["content"] != "before\r\n- [x] task\r\nafter" {
		t.Fatalf("edited content = %v", updated["content"])
	}
	if newVersion, ok := updated["version"].(string); !ok || newVersion == version || newVersion == "" {
		t.Fatalf("edited response version = %v, want a fresh token", updated["version"])
	}
}

func TestMCPPlanExactEditRejectsAmbiguousAndMissingMatches(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	createdOut, err := h.handleCreateTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPCreateTaskPlan,
		mustMarshalPlanPayload(t, map[string]any{"task_id": mcpPlanTaskID, "content": "one\nstone"})))
	if err != nil {
		t.Fatalf("handleCreateTaskPlan: %v", err)
	}
	version := decodeMCPPlanPayload(t, createdOut)["version"].(string)
	for _, tc := range []struct {
		name string
		old  string
		code string
	}{
		{name: "ambiguous", old: "one", code: "plan_edit_ambiguous"},
		{name: "missing", old: "none", code: "plan_edit_not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := h.handleEditTaskPlan(ctx, mcpPlanMsg(t, ws.ActionMCPEditTaskPlan,
				mustMarshalPlanPayload(t, map[string]any{
					"task_id": mcpPlanTaskID, "expected_version": version,
					"old_text": tc.old, "new_text": "replacement",
				})))
			if err != nil {
				t.Fatalf("handleEditTaskPlan: %v", err)
			}
			var payload ws.ErrorPayload
			if err := json.Unmarshal(out.Payload, &payload); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if payload.Details["reason"] != tc.code || payload.Details["write_applied"] != false {
				t.Fatalf("details = %#v, want reason %q and no write", payload.Details, tc.code)
			}
		})
	}
}

func TestMCPPlanRevisionRecoveryHandlers(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	ctx := context.Background()
	first, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Title: "Original", Content: "complete plan",
	})
	if err != nil {
		t.Fatalf("CreatePlan(first): %v", err)
	}
	second, err := h.planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Title: "Current", Content: "short plan", ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("CreatePlan(second): %v", err)
	}
	history, err := h.planService.ListRevisions(ctx, mcpPlanTaskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	source := history[len(history)-1]

	listOut, err := h.handleListTaskPlanRevisions(ctx, mcpPlanMsg(t, ws.ActionMCPListTaskPlanRevisions,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "limit": 1,
		})))
	if err != nil {
		t.Fatalf("handleListTaskPlanRevisions: %v", err)
	}
	var listPayload struct {
		Revisions []map[string]interface{} `json:"revisions"`
	}
	if err := json.Unmarshal(listOut.Payload, &listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listPayload.Revisions) != 1 || listPayload.Revisions[0]["content"] != nil {
		t.Fatalf("metadata response = %#v, want one content-free row", listPayload)
	}

	getOut, err := h.handleGetTaskPlanRevision(ctx, mcpPlanMsg(t, ws.ActionMCPGetTaskPlanRevision,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "revision_id": source.ID,
		})))
	if err != nil {
		t.Fatalf("handleGetTaskPlanRevision: %v", err)
	}
	var sourcePayload map[string]interface{}
	if err := json.Unmarshal(getOut.Payload, &sourcePayload); err != nil {
		t.Fatalf("decode revision: %v", err)
	}
	if sourcePayload["content"] != source.Content || sourcePayload["revision_version"] == "" {
		t.Fatalf("revision response = %#v, want exact content and token", sourcePayload)
	}

	restoreOut, err := h.handleRestoreTaskPlanRevision(ctx, mcpPlanMsg(t, ws.ActionMCPRestoreTaskPlanRevision,
		mustMarshalPlanPayload(t, map[string]any{
			"task_id": mcpPlanTaskID, "revision_id": source.ID,
			"expected_version":          second.Plan.WriteVersion,
			"expected_revision_version": sourcePayload["revision_version"],
		})))
	if err != nil {
		t.Fatalf("handleRestoreTaskPlanRevision: %v", err)
	}
	restored := decodeMCPPlanPayload(t, restoreOut)
	if restored["status"] != "restored" || restored["version"] == second.Plan.WriteVersion {
		t.Fatalf("restore response = %#v, want committed fresh version", restored)
	}
	plan, err := h.planService.GetPlan(ctx, mcpPlanTaskID)
	if err != nil || plan.Content != first.Plan.Content {
		t.Fatalf("restored plan = %#v, %v; want source content", plan, err)
	}
}
