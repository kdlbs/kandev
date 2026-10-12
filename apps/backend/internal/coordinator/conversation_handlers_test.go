package coordinator

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

var errCanned = errors.New("canned test error")

func newConversationHandlersForTest(t *testing.T) (*conversationHandlers, *conversationTestDeps) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	deps := newConversationTestDeps(t)
	return &conversationHandlers{service: deps.svc, logger: newTestLogger(t)}, deps
}

func TestHTTPOpenConversationSuccess(t *testing.T) {
	h, deps := newConversationHandlersForTest(t)
	rec := runHandler(h.httpOpenConversation, http.MethodPost,
		"/api/v1/workspaces/ws-1/coordinators/"+deps.coordinator.ID+"/conversation", "",
		workspaceParams(deps.coordinator.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body ConversationResponse
	decodeBody(t, rec, &body)
	if body.TaskID == "" || body.SessionID == "" {
		t.Errorf("body = %+v, want non-empty task and session ids", body)
	}
	if body.ArchiveState {
		t.Error("ArchiveState = true, want false")
	}
}

func TestHTTPOpenConversationUnknownCoordinatorIs404(t *testing.T) {
	h, _ := newConversationHandlersForTest(t)
	rec := runHandler(h.httpOpenConversation, http.MethodPost,
		"/api/v1/workspaces/ws-1/coordinators/does-not-exist/conversation", "",
		workspaceParams("does-not-exist"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHTTPOpenConversationProfileUnavailableIs409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newServiceForTest(t, nil, nil, nil)
	coord := &Coordinator{WorkspaceID: testWorkspaceID, Name: "Coordinator", AgentProfileID: "ap-missing", ExecutorProfileID: "ep-missing"}
	if err := svc.store.CreateCoordinator(context.Background(), coord); err != nil {
		t.Fatalf("seed coordinator: %v", err)
	}
	svc.SetConversationDeps(newFakeConversationTasks(), newFakeSessionEnsurer())
	h := &conversationHandlers{service: svc, logger: newTestLogger(t)}

	rec := runHandler(h.httpOpenConversation, http.MethodPost,
		"/api/v1/workspaces/ws-1/coordinators/"+coord.ID+"/conversation", "", workspaceParams(coord.ID))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	var body CoordinatorProfileUnavailableResponse
	decodeBody(t, rec, &body)
	if body.AgentProfileStatus == "" || body.ExecutorProfileStatus == "" {
		t.Errorf("body = %+v, want both statuses populated", body)
	}
}

func TestHTTPOpenConversationSessionUnavailableIs502(t *testing.T) {
	h, deps := newConversationHandlersForTest(t)
	deps.sessions.err = errCanned
	rec := runHandler(h.httpOpenConversation, http.MethodPost,
		"/api/v1/workspaces/ws-1/coordinators/"+deps.coordinator.ID+"/conversation", "",
		workspaceParams(deps.coordinator.ID))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

func TestHTTPOpenConversationForbiddenIs403(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newServiceForTest(t, nil, nil, service.ErrForbidden)
	svc.SetConversationDeps(newFakeConversationTasks(), newFakeSessionEnsurer())
	h := &conversationHandlers{service: svc, logger: newTestLogger(t)}

	rec := runHandler(h.httpOpenConversation, http.MethodPost,
		"/api/v1/workspaces/ws-1/coordinators/co-1/conversation", "", workspaceParams("co-1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestHTTPOpenConversationWorkspaceNotFoundIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := newServiceForTest(t, nil, nil, repoerrors.ErrWorkspaceNotFound)
	svc.SetConversationDeps(newFakeConversationTasks(), newFakeSessionEnsurer())
	h := &conversationHandlers{service: svc, logger: newTestLogger(t)}

	rec := runHandler(h.httpOpenConversation, http.MethodPost,
		"/api/v1/workspaces/does-not-exist/coordinators/co-1/conversation", "",
		gin.Params{{Key: "id", Value: "does-not-exist"}, {Key: "cid", Value: "co-1"}})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
