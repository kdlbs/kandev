package runtime

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
	"time"
)

type testAssistantInputs struct {
	input     models.AttentionInput
	calls     int
	stopCalls []string
	failure   error
	actor     models.InputResponse
	onStop    func()
}

func (f *testAssistantInputs) ReadInput(context.Context, *models.AssistantBinding, models.Attention) (*models.AttentionInput, error) {
	value := f.input
	return &value, nil
}
func (f *testAssistantInputs) ResolveInput(_ context.Context, _ *models.AssistantBinding, _ models.Attention, response models.InputResponse) (any, error) {
	f.calls++
	f.actor = response
	if f.failure != nil {
		return nil, f.failure
	}
	f.input.State = "resolved"
	return map[string]string{"status": "resolved"}, nil
}
func (f *testAssistantInputs) StopSession(_ context.Context, _ *models.AssistantBinding, task, session string) (string, error) {
	f.stopCalls = append(f.stopCalls, task+":"+session)
	if f.onStop != nil {
		f.onStop()
	}
	return "stopped", f.failure
}
func assistantInputFixture(t *testing.T) (*Service, *models.AssistantBinding, models.Attention, *testAssistantInputs) {
	t.Helper()
	s, _, b, _ := assistantAttentionFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	var row models.Attention
	for _, r := range rows {
		if r.Kind == "question" {
			row = r
		}
	}
	f := &testAssistantInputs{input: models.AttentionInput{SourceID: row.SourceID, Kind: "question", State: "pending", SourceRevision: row.SourceRevision, TaskID: row.TaskID, SessionID: row.SessionID, PendingID: row.SourceID,
		Questions: []models.InputQuestion{{ID: "color", Prompt: "Choose a sample color", Options: []models.InputOption{{ID: "blue", Label: "Blue"}, {ID: "green", Label: "Green"}}}}}}
	s.Inputs = f
	return s, b, row, f
}
func inputBody(row models.Attention, operation string) map[string]any {
	return map[string]any{"operation_id": operation, "expected_intent_revision": 0, "expected_binding_version": 1, "expected_revision": row.Revision, "source_revision": row.SourceRevision, "session_id": row.SessionID, "answers": []map[string]any{{"question_id": "color", "selected_options": []string{"blue"}}}}
}
func TestAssistantInputHumanNativeParity(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	result := runtimeRequest(t, assistantRouter(s), "POST", path, "", "", inputBody(row, "answer"))
	require.Equal(t, 200, result.Code, result.Body.String())
	require.Equal(t, 1, f.calls)
	require.Equal(t, "user", f.actor.ActorType)
	require.Equal(t, "owner", f.actor.ActorID)
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "POST", path, "", "", inputBody(row, "foreign")).Code)
}
func TestAssistantResolutionIdempotencyAndExpiry(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	body := inputBody(row, "answer")
	first := runtimeRequest(t, router, "POST", path, "", "", body)
	require.Equal(t, 200, first.Code, first.Body.String())
	replay := runtimeRequest(t, router, "POST", path, "", "", body)
	require.Equal(t, 200, replay.Code, replay.Body.String())
	require.JSONEq(t, first.Body.String(), replay.Body.String())
	require.Equal(t, 1, f.calls)
	body["answers"] = []map[string]any{{"question_id": "color", "custom_text": "Different"}}
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	body["operation_id"] = "expired"
	f.input.State = "expired"
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	require.Equal(t, 1, f.calls)
}
func TestAssistantResolutionUnknownDoesNotRepeat(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	f.failure = context.DeadlineExceeded
	router := assistantRouter(s)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	body := inputBody(row, "unknown")
	require.Equal(t, 503, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, "", "", body).Code)
	require.Equal(t, 1, f.calls)
}
func TestAssistantInputRejectsStaleAndWrongSession(t *testing.T) {
	s, _, row, f := assistantInputFixture(t)
	path := "/api/v1/orchestration/assistant/attention/" + row.ID + "/resolve"
	for key, value := range map[string]any{"expected_binding_version": 2, "expected_revision": 2, "session_id": "foreign", "source_revision": "old"} {
		body := inputBody(row, fmt.Sprint(key))
		body[key] = value
		require.Equal(t, 409, runtimeRequest(t, assistantRouter(s), "POST", path, "", "", body).Code)
	}
	require.Zero(t, f.calls)
}

type assistantControlTasks struct {
	*testTasks
	sessions []*taskmodels.TaskSession
}

func (f *assistantControlTasks) ListTaskSessions(_ context.Context, task string) ([]*taskmodels.TaskSession, error) {
	if task != "worker" {
		return nil, nil
	}
	return f.sessions, nil
}
func TestAssistantInputPauseAndStop(t *testing.T) {
	s, b, _, inputs := assistantInputFixture(t)
	ctx := context.Background()
	router := assistantRouter(s)
	updated := 0
	s.AttentionUpdated = func(_ context.Context, id string, _ time.Time) { require.Equal(t, b.ID, id); updated++ }
	s.Tasks = &assistantControlTasks{testTasks: s.Tasks.(*testTasks), sessions: []*taskmodels.TaskSession{{ID: "first", TaskID: "worker", State: taskmodels.TaskSessionStateRunning}, {ID: "second", TaskID: "worker", State: taskmodels.TaskSessionStateWaitingForInput}, {ID: "finished", TaskID: "worker", State: taskmodels.TaskSessionStateCompleted}, {ID: "foreign", TaskID: "unmanaged", State: taskmodels.TaskSessionStateRunning}}}
	control := func(action, op string) *httptest.ResponseRecorder {
		return runtimeRequest(t, router, "POST", "/api/v1/orchestration/assistant/control", "", "", map[string]any{"action": action, "operation_id": op, "expected_binding_version": b.Version, "expected_intent_revision": 0})
	}
	paused := control("pause", "pause")
	require.Equal(t, 200, paused.Code, paused.Body.String())
	require.Empty(t, inputs.stopCalls)
	require.Error(t, s.QueueTurn(ctx, b.OrchestratorID, b.ConversationID, "test", "paused", nil))
	require.Equal(t, 400, runtimeRequest(t, router, "POST", "/api/v1/orchestration/tasks/"+b.ConversationID+"/comments", "", "", map[string]string{"body": "A synthetic request while paused"}).Code)
	resumed := control("resume", "resume")
	require.Equal(t, 200, resumed.Code, resumed.Body.String())
	stopped := control("stop_managed_work", "stop")
	require.Equal(t, 200, stopped.Code, stopped.Body.String())
	require.ElementsMatch(t, []string{"worker:first", "worker:second"}, inputs.stopCalls)
	require.Contains(t, stopped.Body.String(), "already_finished")
	require.NotContains(t, stopped.Body.String(), "unmanaged")
	replay := control("stop_managed_work", "stop")
	require.Equal(t, 200, replay.Code, replay.Body.String())
	require.Len(t, inputs.stopCalls, 2)
	revision, err := s.Repo.IntentRevision(ctx, b.ConversationID)
	require.NoError(t, err)
	require.EqualValues(t, 1, revision)
	require.Equal(t, 3, updated)
}
func TestAssistantInputStopReportsUnknown(t *testing.T) {
	s, b, _, inputs := assistantInputFixture(t)
	inputs.failure = context.DeadlineExceeded
	s.Tasks = &assistantControlTasks{testTasks: s.Tasks.(*testTasks), sessions: []*taskmodels.TaskSession{{ID: "worker-session", TaskID: "worker", State: taskmodels.TaskSessionStateRunning}}}
	response := runtimeRequest(t, assistantRouter(s), "POST", "/api/v1/orchestration/assistant/control", "", "", map[string]any{"action": "stop_managed_work", "operation_id": "stop", "expected_binding_version": b.Version, "expected_intent_revision": 0})
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"status":"unknown"`)
	require.Contains(t, response.Body.String(), `"partial":true`)
}

func TestAssistantInputStopRechecksBindingPerSession(t *testing.T) {
	s, b, _, inputs := assistantInputFixture(t)
	s.Tasks = &assistantControlTasks{testTasks: s.Tasks.(*testTasks), sessions: []*taskmodels.TaskSession{{ID: "first", TaskID: "worker", State: taskmodels.TaskSessionStateRunning}, {ID: "second", TaskID: "worker", State: taskmodels.TaskSessionStateRunning}}}
	inputs.onStop = func() {
		fresh := *b
		fresh.ExecutionMode = "execute"
		require.NoError(t, s.Repo.SelectAssistant(context.Background(), &fresh, b.Version))
	}
	response := runtimeRequest(t, assistantRouter(s), "POST", "/api/v1/orchestration/assistant/control", "", "", map[string]any{"action": "stop_managed_work", "operation_id": "stop-revoked", "expected_binding_version": b.Version, "expected_intent_revision": 0})
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Equal(t, []string{"worker:first"}, inputs.stopCalls)
	require.Contains(t, response.Body.String(), `"partial":true`)
	require.Contains(t, response.Body.String(), `"status":"failed"`)
}
