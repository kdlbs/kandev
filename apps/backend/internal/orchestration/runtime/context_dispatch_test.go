package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestrator/dispatchcontext"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantContextQueuedPacketInvalidatedAfterForget(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	ctx := context.Background()
	agent, human := assistantRuntimeRouter(s), assistantRouter(s)
	updated := runtimeRequest(t, agent, "PATCH", "/api/v1/orchestration/runtime/objectives/"+goal, token, run, map[string]any{
		"mode": "execute", "expected_revision": 1, "operation_id": "execute", "expected_intent_revision": 0,
	})
	require.Equal(t, 200, updated.Code, updated.Body.String())
	memoryPath := "/api/v1/orchestration/assistant/memory/preference"
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", memoryPath, "", "", map[string]any{
		"key": "preference", "content": "OLD_CONTEXT", "scope": "workspace", "source_comment_id": "source", "confirmed": true,
	}).Code)
	response := runtimeRequest(t, agent, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var p models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &p))
	task := &taskmodels.Task{ID: "worker", WorkspaceID: "ws", Metadata: map[string]interface{}{
		"orchestration_binding_id": p.BindingID, "orchestration_objective_id": goal, dispatchcontext.MetadataKey: p.ID,
	}}
	s.Tasks.(*testTasks).tasks[task.ID] = task
	require.NoError(t, s.ValidateDispatchContext(ctx, p.ID, task, "personal"))
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	queueDB, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	queueDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = queueDB.Close() })
	_, err = queueDB.Exec(`
		CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at TIMESTAMP, updated_at TIMESTAMP);
		CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, queue_incarnation_id TEXT NOT NULL);
		INSERT INTO tasks (id, updated_at) VALUES ('worker', CURRENT_TIMESTAMP);
		INSERT INTO task_sessions (id, task_id, queue_incarnation_id) VALUES ('worker-session', 'worker', 'test-incarnation');
	`)
	require.NoError(t, err)
	queueRepo, err := messagequeue.NewSQLiteRepository(queueDB, queueDB)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, 10, log)
	queue.SetDispatchContextResolver(func(context.Context, string) (string, error) { return p.ID, nil })
	queued, err := queue.QueueMessage(ctx, "worker-session", task.ID, "old handoff", "", "user", false, nil)
	require.NoError(t, err)
	// Reconstruct the queue service as after restart. Original metadata survives.
	queue = messagequeue.NewService(queueRepo, 10, log)
	require.Equal(t, 200, runtimeRequest(t, human, "DELETE", memoryPath, "", "", map[string]int{"expected_revision": 1}).Code)
	current := runtimeRequest(t, agent, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal", token, run, nil)
	require.NoError(t, json.Unmarshal(current.Body.Bytes(), &p))
	task.Metadata[dispatchcontext.MetadataKey] = p.ID
	next, ok := queue.TakeQueued(ctx, "worker-session")
	require.True(t, ok)
	require.Equal(t, queued.Metadata, next.Metadata)
	ref, _ := dispatchcontext.Reference(dispatchcontext.FromMetadata(ctx, next.Metadata))
	require.NotEqual(t, p.ID, ref)
	require.Error(t, s.ValidateDispatchContext(ctx, ref, task, "personal"), "refreshing task metadata must not authorize the old queued prompt")
	require.NoError(t, s.ValidateDispatchContext(ctx, p.ID, task, "personal"))
	require.Error(t, s.ValidateDispatchContext(ctx, p.ID, task, "other-account"))
}

func TestAssistantContextScopeNarrowingInvalidatesDispatch(t *testing.T) {
	s, token, run, goal := assistantContextFixture(t)
	ctx := context.Background()
	agent, human := assistantRuntimeRouter(s), assistantRouter(s)
	response := runtimeRequest(t, agent, "PATCH", "/api/v1/orchestration/runtime/objectives/"+goal, token, run, map[string]any{"mode": "execute", "expected_revision": 1, "operation_id": "delivery", "expected_intent_revision": 0})
	require.Equal(t, 200, response.Code, response.Body.String())
	path := "/api/v1/orchestration/assistant/memory/preference"
	edit := map[string]any{"key": "preference", "content": "Example constraint", "scope": "workspace", "source_comment_id": "source", "confirmed": true}
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", path, "", "", edit).Code)
	for _, id := range []string{"first", "second"} {
		s.Tasks.(*testTasks).tasks[id] = &taskmodels.Task{ID: id, WorkspaceID: "ws"}
	}
	response = runtimeRequest(t, agent, "GET", "/api/v1/orchestration/runtime/context/"+goal+"?profile_id=personal&task_id=first", token, run, nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	var packet models.ContextPacket
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &packet))
	task := s.Tasks.(*testTasks).tasks["first"]
	task.Metadata = map[string]interface{}{"orchestration_binding_id": packet.BindingID, "orchestration_objective_id": goal}
	require.NoError(t, s.ValidateDispatchContext(ctx, packet.ID, task, "personal"))
	edit["scope"], edit["scope_id"], edit["expected_revision"] = "task", "second", 1
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", path, "", "", edit).Code)
	require.ErrorContains(t, s.ValidateDispatchContext(ctx, packet.ID, task, "personal"), "context changed")
}
