package dashboard_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestListComments_AgentCallerGetsGuardedWindow covers
// REQ-OFFICE-AGENT-COMMENT-READS-001/002/003: an agent reading its own
// task's comments over the dashboard route gets the guarded, bounded,
// reduced-projection response instead of the browser's unbounded shape.
func TestListComments_AgentCallerGetsGuardedWindow(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)
	seedComment(t, f.repo, "c1", "task-1", "user", "user", "hello", "user", time.Unix(1000, 0))
	seedComment(t, f.repo, "c2", "task-1", "agent", agent.ID, "world", "agent", time.Unix(2000, 0))

	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-1", token, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var window struct {
		Comments []struct {
			ID            string `json:"id"`
			TaskID        string `json:"task_id"`
			AuthorType    string `json:"author_type"`
			AuthorID      string `json:"author_id"`
			Source        string `json:"source"`
			Body          string `json:"body"`
			BodyTruncated bool   `json:"body_truncated"`
			BodyBytes     int    `json:"body_bytes"`
			RunID         string `json:"run_id"`
		} `json:"comments"`
		Total    int  `json:"total"`
		Returned int  `json:"returned"`
		HasMore  bool `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &window); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if window.Total != 2 || window.Returned != 2 || window.HasMore {
		t.Fatalf("window = %+v, want total=2 returned=2 has_more=false", window)
	}
	if len(window.Comments) != 2 {
		t.Fatalf("comments = %+v, want 2", window.Comments)
	}
	// AC-002.4: the agent projection omits run-lifecycle fields present on
	// the browser's CommentDTO — a stray run_id in the JSON means the
	// browser projection leaked onto the agent path.
	for _, c := range window.Comments {
		if c.RunID != "" {
			t.Fatalf("comment %+v carries a run_id; agent projection must omit run lifecycle fields", c)
		}
	}
}

// TestListComments_AgentCallerDeniedForUnrelatedTask covers
// REQ-OFFICE-AGENT-COMMENT-READS-005: an agent scoped to one task cannot
// read an unrelated task's comments through the dashboard route.
func TestListComments_AgentCallerDeniedForUnrelatedTask(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)
	seedCommentTask(t, f.repo, "task-2", "ws-1", "")
	seedComment(t, f.repo, "c1", "task-2", "user", "user", "secret", "user", time.Unix(1000, 0))

	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-2", token, ""))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestListComments_AgentCallerLimitClampsWindow covers
// REQ-OFFICE-AGENT-COMMENT-READS-003: an agent-supplied limit bounds the
// returned window while total still reflects the full comment count.
func TestListComments_AgentCallerLimitClampsWindow(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)
	seedComment(t, f.repo, "c1", "task-1", "user", "user", "one", "user", time.Unix(1000, 0))
	seedComment(t, f.repo, "c2", "task-1", "user", "user", "two", "user", time.Unix(2000, 0))

	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-1", token, "?limit=1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var window struct {
		Total    int `json:"total"`
		Returned int `json:"returned"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &window); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if window.Total != 2 || window.Returned != 1 {
		t.Fatalf("window = %+v, want total=2 returned=1", window)
	}
}

// TestListComments_BrowserCallerUnaffectedByAgentBranch pins the existing
// unguarded, unbounded browser shape: adding the agent branch must not
// change status code, response shape, or the ignored `limit` query param
// for a request carrying no agent token.
func TestListComments_BrowserCallerUnaffectedByAgentBranch(t *testing.T) {
	f := newCommentSecurityFixture(t)
	seedCommentTask(t, f.repo, "task-1", "ws-1", "")
	seedComment(t, f.repo, "c1", "task-1", "user", "user", "hello", "user", time.Unix(1000, 0))
	seedComment(t, f.repo, "c2", "task-1", "user", "user", "world", "user", time.Unix(2000, 0))

	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-1", "", "?limit=1"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Comments []json.RawMessage `json:"comments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if len(resp.Comments) != 2 {
		t.Fatalf("comments = %d, want 2 (browser branch must ignore ?limit)", len(resp.Comments))
	}
}

func seedComment(
	t *testing.T, repo *sqlite.Repository,
	id, taskID, authorType, authorID, body, source string, createdAt time.Time,
) {
	t.Helper()
	_, err := repo.ExecRaw(context.Background(),
		`INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, taskID, authorType, authorID, body, source, createdAt.UTC(),
	)
	if err != nil {
		t.Fatalf("seed comment %q: %v", id, err)
	}
}

func getCommentsReq(taskID, token, query string) *http.Request {
	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/office/tasks/"+taskID+"/comments"+query,
		nil,
	)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}
