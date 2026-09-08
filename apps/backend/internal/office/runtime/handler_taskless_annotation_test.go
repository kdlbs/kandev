package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// tasklessAnnotationHandlerHarness wires the runtime handler around a
// taskless run (empty task_id claim) with comment/task dependencies, so
// POST /runtime/comments can be exercised over HTTP rather than by calling
// Actions.PostComment directly — the wire path has its own request parsing
// and audit-event logging that the Actions-level tests never reach.
type tasklessAnnotationHandlerHarness struct {
	router    *gin.Engine
	token     string
	comments  *handlerCommentWriter
	tasks     *handlerTaskCreator
	runEvents *recordingRunEvents
}

func newTasklessAnnotationHandlerHarness(t *testing.T, claimWorkspaceID string) *tasklessAnnotationHandlerHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	agentSvc := agents.NewAgentService(repo, logger.Default(), nil)
	agentSvc.SetAuth(agents.NewAgentAuth("taskless-annotation-test-key"))
	agent := &models.AgentInstance{
		ID:          "agent-1",
		WorkspaceID: "ws-agent-home",
		Name:        "Coordinator",
		Role:        models.AgentRoleCEO,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	capabilityJSON, err := MarshalCapabilities(Capabilities{CanPostComments: true})
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	token, err := agentSvc.MintRuntimeJWT("agent-1", "", claimWorkspaceID, "run-1", "sess-1", capabilityJSON)
	if err != nil {
		t.Fatalf("mint runtime token: %v", err)
	}
	comments := &handlerCommentWriter{}
	tasks := &handlerTaskCreator{}
	runEvents := &recordingRunEvents{}
	router := gin.New()
	RegisterRoutes(router.Group(""), NewHandler(
		agentSvc,
		NewActions(ActionDependencies{Comments: comments, Tasks: tasks, Agents: agentSvc}),
		nil,
		runEvents,
		nil,
	))
	return &tasklessAnnotationHandlerHarness{
		router: router, token: token, comments: comments, tasks: tasks, runEvents: runEvents,
	}
}

func (h *tasklessAnnotationHandlerHarness) postComment(t *testing.T, taskID, body string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"task_id": taskID, "body": body})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/runtime/comments", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.router.ServeHTTP(resp, req)
	return resp
}

func TestRuntimeHandler_TasklessRunAnnotatesWorkspaceTaskOverHTTP(t *testing.T) {
	h := newTasklessAnnotationHandlerHarness(t, "ws-1")

	resp := h.postComment(t, "task-x", "blocker found")

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.comments.comments) != 1 {
		t.Fatalf("comments = %d, want 1", len(h.comments.comments))
	}
	comment := h.comments.comments[0]
	if comment.TaskID != "task-x" || comment.AuthorID != "agent-1" || comment.AuthorType != "agent" {
		t.Fatalf("comment identity = %#v", comment)
	}
	assertActionRunEvent(t, h.runEvents, "post_comment", "task", "task-x")
}

func TestRuntimeHandler_PostCommentAuditEventNamesTrimmedTarget(t *testing.T) {
	h := newTasklessAnnotationHandlerHarness(t, "ws-1")

	resp := h.postComment(t, "  task-x  ", "blocker found")

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	if len(h.comments.comments) != 1 || h.comments.comments[0].TaskID != "task-x" {
		t.Fatalf("comments = %#v, want one comment on trimmed task-x", h.comments.comments)
	}
	// The audit event must name the task the comment actually landed on,
	// not the padded value the caller supplied.
	assertActionRunEvent(t, h.runEvents, "post_comment", "task", "task-x")
}
