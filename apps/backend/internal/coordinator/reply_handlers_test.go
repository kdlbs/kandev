package coordinator

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

func replyRoutes(t *testing.T, env *replyEnv) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, env.svc, newTestLogger(t))
	return router
}

func postJSON(router *gin.Engine, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestReplyRoutesWireShape(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	router := replyRoutes(t, env)
	base := "/api/v1/workspaces/" + p.WorkspaceID + "/coordinators/" + p.CoordinatorID + "/proposals/" + p.ID

	rec := postJSON(router, base+"/reply", `{"text":""}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"text"`) {
		t.Fatalf("empty text = %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(router, base+"/reply", `{"text":"add tests"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reply = %d %s", rec.Code, rec.Body.String())
	}
	var dto ProposalDTO
	decodeBody(t, rec, &dto)
	if dto.Status != ProposalStatusReturned || dto.ProposalPhase3 == nil ||
		dto.ReplyText == nil || *dto.ReplyText != "add tests" || dto.ReplyDeliveredAt == nil {
		t.Fatalf("dto = %+v", dto)
	}
	rec = postJSON(router, base+"/reply", `{"text":"again"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"reply_text":"add tests"`) {
		t.Fatalf("second reply = %d %s", rec.Code, rec.Body.String())
	}
	rec = postJSON(router, base+"/reply/deliver", ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("deliver delivered = %d %s", rec.Code, rec.Body.String())
	}
	if env.messenger.calls != 1 {
		t.Fatalf("sends = %d, want 1", env.messenger.calls)
	}
}

func TestReplyRoutesAbsentWithoutPhase3(t *testing.T) {
	env := newReplyEnv(t)
	env.svc.phase3 = false
	p := env.propose(t)
	router := replyRoutes(t, env)
	base := "/api/v1/workspaces/" + p.WorkspaceID + "/coordinators/" + p.CoordinatorID + "/proposals/" + p.ID
	for _, path := range []string{base + "/reply", base + "/reply/deliver"} {
		if rec := postJSON(router, path, `{"text":"x"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("%s = %d, want 404", path, rec.Code)
		}
	}
}

func TestReplyRoutesForbiddenWithoutManage(t *testing.T) {
	env := newReplyEnv(t)
	p := env.propose(t)
	env.svc.authz = &fakeWorkspaceAuthorizer{err: taskservice.ErrForbidden}
	router := replyRoutes(t, env)
	base := "/api/v1/workspaces/" + p.WorkspaceID + "/coordinators/" + p.CoordinatorID + "/proposals/" + p.ID
	for _, path := range []string{base + "/reply", base + "/reply/deliver"} {
		if rec := postJSON(router, path, `{"text":"x"}`); rec.Code != http.StatusForbidden {
			t.Fatalf("%s = %d, want 403", path, rec.Code)
		}
	}
	if env.messenger.calls != 0 {
		t.Fatal("a forbidden reply must send nothing")
	}
}
