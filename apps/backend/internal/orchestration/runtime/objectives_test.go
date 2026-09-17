package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantObjectiveInspectCreatesNoDeliveryTask(t *testing.T) {
	s, db, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	require.NoError(t, s.Repo.PutComment(context.Background(), &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Source: "user", Body: "Inspect the current state"}))
	request := map[string]any{"title": "Inspect current state", "mode": "inspect", "source_comment_id": "source", "acceptance": []map[string]string{{"id": "summary", "description": "Summarize with evidence"}}, "operation_id": "objective-inspect", "expected_intent_revision": 0}
	result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives", token, runID, request)
	require.Equal(t, 201, result.Code, result.Body.String())
	var objective map[string]any
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &objective))
	require.Equal(t, "inspect", objective["mode"])
	require.Equal(t, "active", objective["status"])
	require.EqualValues(t, 1, objective["revision"])
	require.Zero(t, manager.creates.Load())
	var tasks int
	require.NoError(t, db.Get(&tasks, "SELECT count(*) FROM tasks WHERE is_ephemeral=0"))
	require.Zero(t, tasks)
	read := runtimeRequest(t, assistantRouter(s), "GET", "/api/v1/orchestration/assistant/objectives", "", "", nil)
	require.Equal(t, 200, read.Code, read.Body.String())
	require.Contains(t, read.Body.String(), objective["id"])
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "other"), "GET", "/api/v1/orchestration/assistant/objectives", "", "", nil).Code)
}

func TestAssistantRoutingAnswerAndInspectCannotCreateDeliveryTasks(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	for _, mode := range []string{"answer", "inspect", "unknown"} {
		request := map[string]any{"title": "No delivery", "execution_mode": mode, "operation_id": mode, "expected_intent_revision": 0}
		result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, request)
		require.Equal(t, 422, result.Code, result.Body.String())
	}
	require.Zero(t, manager.creates.Load())
}

func TestAssistantCompletionRejectsMissingAndStaleEvidence(t *testing.T) {
	s, _, task := newRuntime(t)
	router, token, runID := assistantRuntimeCaller(t, s, task)
	require.NoError(t, s.Repo.PutComment(context.Background(), &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Source: "user", Body: "Answer this"}))
	request := map[string]any{"title": "Answer", "mode": "answer", "source_comment_id": "source", "acceptance": []map[string]string{{"id": "answer", "description": "Answer with evidence"}}, "operation_id": "new-goal", "expected_intent_revision": 0}
	created := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives", token, runID, request)
	require.Equal(t, 201, created.Code, created.Body.String())
	var row map[string]any
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &row))
	path := "/api/v1/orchestration/runtime/objectives/" + row["id"].(string)
	update := map[string]any{"status": "complete", "expected_revision": 1, "operation_id": "finish", "expected_intent_revision": 0}
	require.Equal(t, 422, runtimeRequest(t, router, "PATCH", path, token, runID, update).Code)
	require.NoError(t, s.Repo.PutComment(context.Background(), &models.TaskComment{ID: "answer", TaskID: task, AuthorType: "agent", AuthorID: "chief", Source: "agent", Body: "The answer"}))
	update["evidence"] = []map[string]any{{"criterion_id": "answer", "source_kind": "comment", "source_id": "answer", "acceptance_revision": 0}}
	update["operation_id"] = "finish-stale-evidence"
	require.Equal(t, 422, runtimeRequest(t, router, "PATCH", path, token, runID, update).Code)
	update["evidence"] = []map[string]any{{"criterion_id": "answer", "source_kind": "comment", "source_id": "answer", "acceptance_revision": 1}}
	update["operation_id"] = "finish-valid"
	result := runtimeRequest(t, router, "PATCH", path, token, runID, update)
	require.Equal(t, 200, result.Code, result.Body.String())
	update["operation_id"] = "stale-edit"
	require.Equal(t, 409, runtimeRequest(t, router, "PATCH", path, token, runID, update).Code)
}

func TestAssistantObjectiveDeliveryLinksAndReplaysOriginalReceipt(t *testing.T) {
	s, db, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	ctx := context.Background()
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Source: "user", Body: "Implement this known change"}))
	create := map[string]any{"title": "Known change", "mode": "execute", "source_comment_id": "source", "acceptance": []map[string]string{{"id": "tests", "description": "Tests pass"}}, "operation_id": "goal", "expected_intent_revision": 0}
	result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/objectives", token, runID, create)
	require.Equal(t, 201, result.Code, result.Body.String())
	var o models.Objective
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &o))
	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('created-task','ws','Delegated',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	request := map[string]any{"title": "Known change", "execution_mode": "execute", "objective_id": o.ID, "workflow_step_id": "implementation", "context_ref": "packet", "operation_id": "delegate", "expected_intent_revision": 0}
	path := "/api/v1/orchestration/runtime/tasks"
	invalid := map[string]any{"title": "Known change", "execution_mode": "execute", "objective_id": o.ID, "context_ref": "unknown", "operation_id": "bad-context", "expected_intent_revision": 0}
	rejected := runtimeRequest(t, router, "POST", path, token, runID, invalid)
	require.Equal(t, 422, rejected.Code, rejected.Body.String())
	require.Zero(t, manager.creates.Load())
	packetResponse := runtimeRequest(t, router, "GET", "/api/v1/orchestration/runtime/context/"+o.ID+"?profile_id=personal", token, runID, nil)
	require.Equal(t, 200, packetResponse.Code, packetResponse.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(packetResponse.Body.Bytes(), &packet))
	request["context_ref"] = packet.ID
	first := runtimeRequest(t, router, "POST", path, token, runID, request)
	require.Equal(t, 201, first.Code, first.Body.String())
	require.Equal(t, "implementation", manager.lastSpec.WorkflowStepID)
	require.Equal(t, o.ID, manager.lastSpec.ObjectiveID)
	require.EqualValues(t, 1, manager.lastSpec.AcceptanceRevision)
	links, err := s.Repo.ObjectiveTasks(ctx, o.ID)
	require.NoError(t, err)
	require.Len(t, links, 1)
	require.Equal(t, packet.ID, links[0].ContextRef)
	require.NotNil(t, manager.lastSpec.Packet)
	paused := runtimeRequest(t, router, "PATCH", "/api/v1/orchestration/runtime/objectives/"+o.ID, token, runID, map[string]any{"status": "paused", "expected_revision": 1, "operation_id": "pause-goal", "expected_intent_revision": 0})
	require.Equal(t, 200, paused.Code, paused.Body.String())
	retry := runtimeRequest(t, router, "POST", path, token, runID, request)
	require.Equal(t, 201, retry.Code, retry.Body.String())
	require.JSONEq(t, first.Body.String(), retry.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestAssistantObjectiveRequiredForEveryDelivery(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	response := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{"title": "Untracked delivery", "operation_id": "untracked", "expected_intent_revision": 0})
	require.Equal(t, 422, response.Code, response.Body.String())
	require.Zero(t, manager.creates.Load())
}
