package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestTaskExecutionGradeRegisteredPatches(t *testing.T) {
	for _, transport := range []string{"REST", "WS"} {
		t.Run(transport, func(t *testing.T) {
			pair := fieldProtocolPair(t)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			for _, tc := range []struct {
				payload map[string]any
				want    string
			}{
				{map[string]any{"execution_grade": "Tier-A"}, "Tier-A"},
				{map[string]any{"title": "Rename"}, "Tier-A"},
				{map[string]any{"execution_grade": nil}, "Tier-A"},
				{map[string]any{"execution_grade": ""}, ""},
				{map[string]any{"execution_grade": "Tier-B", "priority": "high"}, "Tier-B"},
			} {
				response, ok, err := pair[0].request(ctx, transport, tc.payload)
				if err != nil || !ok {
					t.Fatalf("patch=%v %t %v", response, ok, err)
				}
				if response["execution_grade"] != tc.want {
					t.Fatalf("response grade=%v want %q", response["execution_grade"], tc.want)
				}
				select {
				case event := <-pair[0].published:
					payload, ok := event.Data.(map[string]interface{})
					if !ok || payload["execution_grade"] != tc.want {
						t.Fatalf("event data=%v want grade %q", event.Data, tc.want)
					}
				case <-ctx.Done():
					t.Fatal("task event missing", ctx.Err())
				}
			}
			for _, bad := range []any{" bad", 17} {
				requireTaskGradeRejection(t, pair[0], ctx, transport, bad)
				stored, err := pair[1].gate.GetTask(ctx, "protocol-fields")
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(stored)
				if !strings.Contains(string(raw), `"execution_grade":"Tier-B"`) {
					t.Fatalf("invalid update changed task: %s", raw)
				}
			}
		})
	}
}

func TestTaskExecutionGradeRegisteredCreates(t *testing.T) {
	pair := fieldProtocolPair(t)
	ctx := context.Background()
	repo := pair[0].gate.Repository
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "grade-wf", WorkspaceID: "protocol-ws", Name: "Grade"}); err != nil {
		t.Fatal(err)
	}
	for _, transport := range []string{"REST", "WS"} {
		payload := map[string]any{"workspace_id": "protocol-ws", "workflow_id": "grade-wf", "title": "Grade " + transport, "execution_grade": "Explicit"}
		body := createTaskGradeResponse(t, pair[0], ctx, transport, payload)
		if body["execution_grade"] != "Explicit" {
			t.Fatalf("%s create grade lost: %v", transport, body)
		}
		id := fmt.Sprint(body["id"])
		stored, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := json.Marshal(stored)
		if !strings.Contains(string(data), `"execution_grade":"Explicit"`) {
			t.Fatalf("stored=%s", data)
		}
	}
}

func requireTaskGradeRejection(t *testing.T, f fieldProtocolFixture, ctx context.Context, transport string, bad any) {
	t.Helper()
	payload := map[string]any{"execution_grade": bad}
	if transport == "REST" {
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/protocol-fields", strings.NewReader(string(data))).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		f.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid grade status=%d body=%s", rec.Code, rec.Body.String())
		}
		return
	}
	body, ok, err := f.request(ctx, transport, payload)
	want := ws.ErrorCodeValidation
	if _, isString := bad.(string); !isString {
		want = ws.ErrorCodeBadRequest
	}
	if err != nil || ok || body["code"] != want {
		t.Fatalf("invalid grade WS=%v %t %v want %s", body, ok, err, want)
	}
}

func createTaskGradeResponse(t *testing.T, f fieldProtocolFixture, ctx context.Context, transport string, payload map[string]any) map[string]any {
	t.Helper()
	if transport == "REST" {
		return createTaskGradeRESTResponse(t, f, ctx, payload)
	}
	msg, err := ws.NewRequest("grade-create", ws.ActionTaskCreate, payload)
	if err != nil {
		t.Fatal(err)
	}
	response, err := f.dispatcher.Dispatch(ctx, msg)
	if err != nil || response.Type != ws.MessageTypeResponse {
		t.Fatalf("create=%v %v", response, err)
	}
	var body map[string]any
	if err := response.ParsePayload(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func createTaskGradeRESTResponse(t *testing.T, f fieldProtocolFixture, ctx context.Context, payload map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(string(data))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("create=%d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
