package orchestrator

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
)

// The session-keyed WS actions resolve sessions through the orchestrator's own
// repo handle, so they do not inherit the task service's authorize* checks.
// Without SetSessionAccessChecker a caller supplying another user's
// (task_id, session_id) pair was served: GetTaskSessionStatus returned that
// session's live status, and CheckSessionPR revealed PR association and
// installed a PR watch.

func scopeTestLogger(t *testing.T) *commonlogger.Logger {
	t.Helper()
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

// denyingChecker refuses every session and records that it was consulted.
func denyingChecker(called *bool) func(context.Context, string) error {
	return func(context.Context, string) error {
		*called = true
		return errors.New("task not found")
	}
}

// TestGetTaskSessionStatusDeniesForeignSession also pins that the denial is
// reported in the same shape as a missing session, so the response does not
// distinguish "not yours" from "does not exist". repo stays nil: a denial must
// short-circuit before any session lookup.
func TestGetTaskSessionStatusDeniesForeignSession(t *testing.T) {
	called := false
	s := &Service{logger: scopeTestLogger(t), sessionAccessCheck: denyingChecker(&called)}

	resp, err := s.GetTaskSessionStatus(context.Background(), "task-b", "sess-b")
	if err != nil {
		t.Fatalf("GetTaskSessionStatus: %v", err)
	}

	if !called {
		t.Fatal("session access checker was not consulted")
	}
	if resp.Error != "session not found" {
		t.Errorf("resp.Error = %q, want \"session not found\"", resp.Error)
	}
}

// TestCheckSessionPRDeniesForeignSession keeps the check ahead of the
// unconfigured-GitHub early return, so the gate applies regardless of setup.
func TestCheckSessionPRDeniesForeignSession(t *testing.T) {
	called := false
	s := &Service{logger: scopeTestLogger(t), sessionAccessCheck: denyingChecker(&called)}

	found, err := s.CheckSessionPR(context.Background(), "task-b", "sess-b")
	if err != nil {
		t.Fatalf("CheckSessionPR: %v", err)
	}

	if !called {
		t.Fatal("session access checker was not consulted before the early return")
	}
	if found {
		t.Error("found = true; a denied session must not report a PR")
	}
}

// TestSessionScopeUnwiredStaysUnscoped pins the pre-auth behavior: with no
// checker installed nothing is denied, so single-user instances and tests are
// unaffected. GetTaskSessionStatus is exercised via CheckSessionPR, which
// returns cleanly without a repo.
func TestSessionScopeUnwiredStaysUnscoped(t *testing.T) {
	s := &Service{logger: scopeTestLogger(t)}

	if err := s.authorizeSession(context.Background(), "any-session"); err != nil {
		t.Fatalf("unwired checker must not deny: %v", err)
	}
	found, err := s.CheckSessionPR(context.Background(), "task-b", "sess-b")
	if err != nil || found {
		t.Fatalf("CheckSessionPR = (%v, %v), want (false, nil) with no github service", found, err)
	}
}

// TestSessionScopeCheckerReceivesSessionID guards against wiring the check to
// the task ID by mistake, which would authorize the wrong resource.
func TestSessionScopeCheckerReceivesSessionID(t *testing.T) {
	var got string
	s := &Service{logger: scopeTestLogger(t), sessionAccessCheck: func(_ context.Context, sessionID string) error {
		got = sessionID
		return nil
	}}

	if _, err := s.CheckSessionPR(context.Background(), "task-b", "sess-b"); err != nil {
		t.Fatalf("CheckSessionPR: %v", err)
	}

	if got != "sess-b" {
		t.Errorf("checker received %q, want the session ID sess-b", got)
	}
}
