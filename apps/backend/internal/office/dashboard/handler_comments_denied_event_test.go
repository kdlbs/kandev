package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// recordingRunEventAppender is a fake dashboard.RunEventAppender that
// records every call so a test can assert exactly one event was recorded
// with the expected shape.
type recordingRunEventAppender struct {
	mu     sync.Mutex
	events []recordedRunEvent
}

type recordedRunEvent struct {
	runID     string
	eventType string
	level     string
	payload   map[string]interface{}
}

func (a *recordingRunEventAppender) AppendRunEvent(
	_ context.Context, runID, eventType, level string, payload map[string]interface{},
) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, recordedRunEvent{runID: runID, eventType: eventType, level: level, payload: payload})
}

// TestListComments_DeniedAgentReadAppendsRunEvent covers
// AC-OFFICE-AGENT-COMMENT-READS-009.5: a refused agent comment read from a
// caller whose JWT carries a run identifier appends exactly one
// runtime.denied/warn event to that run, for both a task-bound caller
// refused by the relation guard and a taskless run caller refused by the
// workspace guard.
func TestListComments_DeniedAgentReadAppendsRunEvent(t *testing.T) {
	f := newCommentSecurityFixture(t)
	appender := &recordingRunEventAppender{}
	f.svc.SetRunEventAppender(appender)

	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)
	seedCommentTask(t, f.repo, "task-2", "ws-1", "")
	seedCommentTask(t, f.repo, "task-foreign", "ws-2", "")

	// Task-bound caller, unrelated same-workspace target.
	taskBoundToken, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint task-bound jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-2", taskBoundToken, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("task-bound status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// Taskless run caller, foreign-workspace target.
	tasklessToken, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "", agent.WorkspaceID, "run-2", "sess-2", "")
	if err != nil {
		t.Fatalf("mint taskless jwt: %v", err)
	}
	rec2 := httptest.NewRecorder()
	f.router.ServeHTTP(rec2, getCommentsReq("task-foreign", tasklessToken, ""))
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("taskless status = %d, want 403; body=%s", rec2.Code, rec2.Body.String())
	}

	appender.mu.Lock()
	defer appender.mu.Unlock()
	if len(appender.events) != 2 {
		t.Fatalf("events = %+v, want exactly 2", appender.events)
	}
	first := appender.events[0]
	if first.runID != "run-1" || first.eventType != "runtime.denied" || first.level != "warn" {
		t.Fatalf("first event = %+v, want run-1/runtime.denied/warn", first)
	}
	if first.payload["action"] != "read_comments" || first.payload["target_type"] != "task" {
		t.Fatalf("first payload = %+v", first.payload)
	}
	if first.payload["target_id"] != "task-2" || first.payload["agent_id"] != agent.ID || first.payload["session_id"] != "sess-1" {
		t.Fatalf("first payload identity = %+v", first.payload)
	}
	if first.payload["error"] != "document access denied" {
		t.Fatalf("first payload error = %+v, want 'document access denied'", first.payload["error"])
	}

	second := appender.events[1]
	if second.runID != "run-2" || second.eventType != "runtime.denied" || second.level != "warn" {
		t.Fatalf("second event = %+v, want run-2/runtime.denied/warn", second)
	}
	if second.payload["target_id"] != "task-foreign" || second.payload["session_id"] != "sess-2" {
		t.Fatalf("second payload identity = %+v", second.payload)
	}
}

// TestListComments_NoDeniedEventWithoutRunOrOnSuccess covers
// AC-OFFICE-AGENT-COMMENT-READS-009.6: no event is appended for a refused
// caller with no run identifier, an accepted read, or a dependency error.
func TestListComments_NoDeniedEventWithoutRunOrOnSuccess(t *testing.T) {
	f := newCommentSecurityFixture(t)
	appender := &recordingRunEventAppender{}
	f.svc.SetRunEventAppender(appender)

	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)

	// Refused, but with no run claim at all (empty task and empty run) —
	// AC-001.13's blanket denial, not a taskless run caller.
	noRunToken, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "", agent.WorkspaceID, "", "sess-1", "")
	if err != nil {
		t.Fatalf("mint no-run jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-1", noRunToken, ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-run status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}

	// Accepted read.
	okToken, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-3", "sess-3", "")
	if err != nil {
		t.Fatalf("mint ok jwt: %v", err)
	}
	rec2 := httptest.NewRecorder()
	f.router.ServeHTTP(rec2, getCommentsReq("task-1", okToken, ""))
	if rec2.Code != http.StatusOK {
		t.Fatalf("accepted status = %d, want 200; body=%s", rec2.Code, rec2.Body.String())
	}

	appender.mu.Lock()
	defer appender.mu.Unlock()
	if len(appender.events) != 0 {
		t.Fatalf("events = %+v, want none", appender.events)
	}
}

// TestListComments_DeniedResponseUnchangedWithoutAppender covers
// AC-OFFICE-AGENT-COMMENT-READS-009.7: with no appender wired, the refused
// response is unchanged (still 403, same body).
func TestListComments_DeniedResponseUnchangedWithoutAppender(t *testing.T) {
	f := newCommentSecurityFixture(t)
	agent := seedCommentAgent(t, f.agentsSvc, f.repo, "agent-a", "ws-1")
	seedCommentTask(t, f.repo, "task-1", "ws-1", agent.ID)
	seedCommentTask(t, f.repo, "task-2", "ws-1", "")

	token, err := f.agentsSvc.MintRuntimeJWT(agent.ID, "task-1", agent.WorkspaceID, "run-1", "sess-1", "")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, getCommentsReq("task-2", token, ""))

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"error":"document access denied"}` {
		t.Fatalf("body = %s, want unchanged access-denied body", rec.Body.String())
	}
}
