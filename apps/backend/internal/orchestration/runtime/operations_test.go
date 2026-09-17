package runtime

import (
	"context"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

type assistantTaskManager struct {
	creates  atomic.Int64
	failure  error
	started  chan struct{}
	release  chan struct{}
	lastSpec models.WorkspaceTaskSpec
}

func (m *assistantTaskManager) CreateWorkspaceTask(_ context.Context, spec models.WorkspaceTaskSpec) (string, error) {
	m.creates.Add(1)
	m.lastSpec = spec
	if m.started != nil {
		close(m.started)
		<-m.release
	}
	return "created-task", m.failure
}

func TestAssistantOperationRestartPreservesUnknownReceipt(t *testing.T) {
	s, db, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	_, err := db.Exec(`CREATE TRIGGER lose_operation_ack BEFORE UPDATE OF state ON orchestration_operations
		WHEN NEW.state='acknowledged' BEGIN SELECT RAISE(ABORT,'lost receipt'); END`)
	require.NoError(t, err)
	path := "/api/v1/orchestration/runtime/tasks"
	body := map[string]any{"title": "Inspect", "operation_id": "lost-ack", "expected_intent_revision": 0}
	require.Equal(t, 503, runtimeRequest(t, router, "POST", path, token, runID, body).Code)
	require.EqualValues(t, 1, manager.creates.Load())
	require.NoError(t, s.RecoverInterrupted(context.Background()))
	var state string
	require.NoError(t, db.Get(&state, "SELECT state FROM orchestration_operations WHERE operation_id='lost-ack'"))
	require.Equal(t, "unknown", state)
	require.EqualValues(t, 1, manager.creates.Load())
}
func (m *assistantTaskManager) ManageWorkspaceTask(context.Context, models.WorkspaceTaskCommand) error {
	return nil
}
func (m *assistantTaskManager) WorkspaceTaskDetails(context.Context, string, string) (any, error) {
	return nil, nil
}
func (m *assistantTaskManager) WorkspaceCatalog(context.Context, string) (any, error) {
	return nil, nil
}

func assistantRuntimeCaller(t *testing.T, s *Service, task string) (*gin.Engine, string, string) {
	t.Helper()
	return assistantRuntimeCallerMode(t, s, task, "execute")
}

func assistantRuntimeCallerMode(t *testing.T, s *Service, task, mode string) (*gin.Engine, string, string) {
	t.Helper()
	ctx := context.Background()
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant",
		"", "", map[string]any{"orchestrator_id": "chief", "execution_mode": mode}).Code)
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "operation-run", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, assistantBrokerAudience, run.Payload, "session"))
	token, err := s.Auth.MintRuntimeJWT("chief", task, "ws", run.ID, "session", assistantBrokerAudience)
	require.NoError(t, err)
	router := gin.New()
	RegisterRoutes(router.Group("/api/v1/orchestration", runtimeauth.Middleware(s.Auth, s.Personas)), &Handler{Service: s})
	return router, token, run.ID
}

func TestAssistantOperationDuplicateReplaysReceiptNotAction(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	request := map[string]any{"title": "Read-only audit", "operation_id": "create-audit", "expected_intent_revision": 0}
	path := "/api/v1/orchestration/runtime/tasks"
	first := runtimeRequest(t, router, "POST", path, token, runID, request)
	require.Equal(t, 201, first.Code, first.Body.String())
	second := runtimeRequest(t, router, "POST", path, token, runID, request)
	require.Equal(t, 201, second.Code, second.Body.String())
	require.JSONEq(t, first.Body.String(), second.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
	request["title"] = "Different task"
	require.Equal(t, 409, runtimeRequest(t, router, "POST", path, token, runID, request).Code)
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestAssistantOperationTimeoutDoesNotRepeatExternalAction(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{failure: context.DeadlineExceeded}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	body := map[string]any{"title": "Read-only audit", "operation_id": "uncertain", "expected_intent_revision": 0}
	path := "/api/v1/orchestration/runtime/tasks"
	result := runtimeRequest(t, router, "POST", path, token, runID, body)
	require.Equal(t, 503, result.Code, result.Body.String())
	retry := runtimeRequest(t, router, "POST", path, token, runID, body)
	require.Equal(t, 409, retry.Code, retry.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
	body["operation_id"] = "fresh-id-cannot-hide-uncertain-delivery"
	retry = runtimeRequest(t, router, "POST", path, token, runID, body)
	require.Equal(t, 409, retry.Code, retry.Body.String())
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestAssistantOperationRequiresIDAndIntentForBoundAssistant(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	for _, body := range []map[string]any{{"title": "Missing identity"}, {"title": "Missing intent", "operation_id": "no-intent"}} {
		result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, body)
		require.Equal(t, 422, result.Code, result.Body.String())
	}
	require.Zero(t, manager.creates.Load())
}

func TestAssistantOperationConcurrentDispatchIsSingleAttempt(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{started: make(chan struct{}), release: make(chan struct{})}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	body := map[string]any{"title": "Inspect", "operation_id": "concurrent", "expected_intent_revision": 0}
	path := "/api/v1/orchestration/runtime/tasks"
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- runtimeRequest(t, router, "POST", path, token, runID, body) }()
	<-manager.started
	second := runtimeRequest(t, router, "POST", path, token, runID, body)
	close(manager.release)
	require.Equal(t, 409, second.Code, second.Body.String())
	require.Equal(t, 201, (<-first).Code)
	require.EqualValues(t, 1, manager.creates.Load())
}

func TestAssistantBindingRuntimeCannotSelectOrReadHumanBinding(t *testing.T) {
	s, _, task := newRuntime(t)
	router, token, runID := assistantRuntimeCaller(t, s, task)
	path := "/api/v1/orchestration/assistant"
	require.Equal(t, 403, runtimeRequest(t, router, "GET", path, token, runID, nil).Code)
	require.Equal(t, 403, runtimeRequest(t, router, "PUT", path, token, runID, map[string]any{"orchestrator_id": "chief", "expected_version": 1}).Code)
	require.Equal(t, 409, runtimeRequest(t, assistantRouter(s, "foreign"), "PUT", path, "", "", map[string]any{"orchestrator_id": "chief"}).Code)
}

func TestAssistantIntentBindingRevisionRevokesOldRun(t *testing.T) {
	s, _, task := newRuntime(t)
	manager := &assistantTaskManager{}
	s.Manager = manager
	router, token, runID := assistantRuntimeCaller(t, s, task)
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief", "expected_version": 1}).Code)
	result := runtimeRequest(t, router, "POST", "/api/v1/orchestration/runtime/tasks", token, runID, map[string]any{"title": "Old selection", "operation_id": "stale-binding", "expected_intent_revision": 0})
	require.Equal(t, 409, result.Code, result.Body.String())
	require.Zero(t, manager.creates.Load())
}
