package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/task/models"
)

// pairStubRepo satisfies sessionExecutorStore with only GetTaskSession usable —
// the pair consistency check calls nothing else, so the embedded nil interface
// is never touched.
type pairStubRepo struct {
	sessionExecutorStore
	sessionTaskID string
}

func (r *pairStubRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: "sess-1", TaskID: r.sessionTaskID}, nil
}

// Structural pin for the orchestrator's session-keyed entry points.
//
// These resolve sessions through the orchestrator's own repo/executor handles,
// so they inherit nothing from the task service's authorize* helpers. Each one
// was reachable straight from a WS action with a caller-supplied ID and no
// ownership check — stopping, deleting, renaming, relaunching or cancelling
// another user's live agent session, or answering their permission prompts.
//
// Every entry asserts the guard fires. Dependencies are left nil on purpose: a
// denial must short-circuit before the method touches repo, executor or agent
// manager, so a nil-pointer panic here means the guard is in the wrong place.

var errDenied = errors.New("task not found")

// deniedCase invokes one entry point and reports whether it refused.
type deniedCase struct {
	name   string
	invoke func(*Service) error
}

func sessionScopeCases() []deniedCase {
	const (
		taskID    = "task-b"
		sessionID = "sess-b"
	)
	return []deniedCase{
		{"StopSession", func(s *Service) error {
			return s.StopSession(context.Background(), sessionID, "reason", false)
		}},
		{"DeleteSession", func(s *Service) error {
			return s.DeleteSession(context.Background(), sessionID)
		}},
		{"SetPrimarySession", func(s *Service) error {
			return s.SetPrimarySession(context.Background(), sessionID)
		}},
		{"RenameSession", func(s *Service) error {
			return s.RenameSession(context.Background(), sessionID, "hijacked")
		}},
		{"ResetAgentContext", func(s *Service) error {
			return s.ResetAgentContext(context.Background(), sessionID)
		}},
		{"CancelAgent", func(s *Service) error {
			return s.CancelAgent(context.Background(), sessionID)
		}},
		{"RespondToPermission", func(s *Service) error {
			return s.RespondToPermission(context.Background(), "task-1", sessionID, "request-1", "pending-1", "allow", false, false)
		}},
		{"SetSessionPlanModeByID", func(s *Service) error {
			return s.SetSessionPlanModeByID(context.Background(), sessionID, true)
		}},
		{"RecoverSession", func(s *Service) error {
			_, err := s.RecoverSession(context.Background(), taskID, sessionID, "resume")
			return err
		}},
		{"LaunchSession", func(s *Service) error {
			_, err := s.LaunchSession(context.Background(), &LaunchSessionRequest{
				TaskID: taskID, SessionID: sessionID,
			})
			return err
		}},
		{"LaunchSession/create-no-session", func(s *Service) error {
			// SessionID is empty when creating, so the task check must carry it.
			_, err := s.LaunchSession(context.Background(), &LaunchSessionRequest{TaskID: taskID})
			return err
		}},
		{"EnsureSession", func(s *Service) error {
			_, err := s.EnsureSession(context.Background(), taskID)
			return err
		}},
		{"SteerTask", func(s *Service) error {
			_, err := s.SteerTask(context.Background(), taskID, sessionID, "steer", "", false, nil)
			return err
		}},
		{"ResumeDetachedClarification", func(s *Service) error {
			return s.ResumeDetachedClarification(
				context.Background(),
				clarification.DetachedClarificationResume{TaskID: taskID, SessionID: sessionID},
			)
		}},
	}
}

// TestSessionKeyedEntryPointsDenyForeignResources is the matrix: with both
// checkers denying, every entry point must refuse.
func TestSessionKeyedEntryPointsDenyForeignResources(t *testing.T) {
	for _, tc := range sessionScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{
				logger:             scopeTestLogger(t),
				sessionAccessCheck: func(context.Context, string) error { return errDenied },
				taskAccessCheck:    func(context.Context, string) error { return errDenied },
			}

			if err := tc.invoke(s); err == nil {
				t.Fatal("entry point did not deny a foreign resource")
			}
		})
	}
}

// TestSessionKeyedEntryPointsGuardBeforeDependencies asserts the guard runs
// before any nil dependency is touched — the run above would panic, not fail,
// if a guard were placed after the first repo/executor call. Kept separate so
// the intent is explicit rather than implied by the absence of a panic.
func TestSessionKeyedEntryPointsGuardBeforeDependencies(t *testing.T) {
	for _, tc := range sessionScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("guard runs too late — panicked on a nil dependency: %v", r)
				}
			}()
			s := &Service{
				logger:             scopeTestLogger(t),
				sessionAccessCheck: func(context.Context, string) error { return errDenied },
				taskAccessCheck:    func(context.Context, string) error { return errDenied },
			}
			_ = tc.invoke(s)
		})
	}
}

// TestCancelTransientRetryDeniesForeignSession is separate because it returns a
// bool rather than an error: a denial must read as "nothing to cancel" so a
// foreign session is indistinguishable from an idle one.
func TestCancelTransientRetryDeniesForeignSession(t *testing.T) {
	s := &Service{
		logger:             scopeTestLogger(t),
		sessionAccessCheck: func(context.Context, string) error { return errDenied },
	}

	if s.CancelTransientRetry(context.Background(), "task-b", "sess-b") {
		t.Error("CancelTransientRetry reported a cancellation for a foreign session")
	}
}

// TestSessionKeyedEntryPointsUnscopedWhenUnwired pins the compatibility
// contract for every entry point at once: with no checkers installed nothing is
// denied by scoping, so identity-less internal callers (schedulers, event bus,
// workflow engine) and pre-auth single-user instances are unaffected.
//
// The methods still fail on their nil dependencies — that is expected and not
// what this asserts. It asserts they get *past* the guard, which is why the
// denial sentinel must not appear.
func TestSessionKeyedEntryPointsUnscopedWhenUnwired(t *testing.T) {
	for _, tc := range sessionScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				// Reaching a nil dependency proves the guard did not deny.
				_ = recover()
			}()
			s := &Service{logger: scopeTestLogger(t)}

			if err := tc.invoke(s); errors.Is(err, errDenied) {
				t.Fatal("unwired checker denied the call; pre-auth behavior broken")
			}
		})
	}
}

// ownSessionForeignTask is the pairing attack: the caller supplies one of their
// own session IDs to satisfy the session check, while pointing taskID at another
// user's task. Entry points that accept both IDs then do their task-scoped work
// against the foreign task — for CheckSessionPR that is PR disclosure plus a
// PR-watch write, for CancelTransientRetry a recoverable-failure write.
func ownSessionForeignTask() *Service {
	return &Service{
		sessionAccessCheck: func(_ context.Context, sessionID string) error {
			if sessionID == "sess-mine" {
				return nil
			}
			return errDenied
		},
		taskAccessCheck: func(_ context.Context, taskID string) error {
			if taskID == "task-mine" {
				return nil
			}
			return errDenied
		},
	}
}

func TestCheckSessionPRDeniesOwnSessionPairedWithForeignTask(t *testing.T) {
	s := ownSessionForeignTask()
	s.logger = scopeTestLogger(t)

	found, err := s.CheckSessionPR(context.Background(), "task-victim", "sess-mine")

	if err != nil {
		t.Fatalf("CheckSessionPR: %v", err)
	}
	if found {
		t.Error("found = true; an owned session must not unlock another user's task")
	}
}

func TestCancelTransientRetryDeniesOwnSessionPairedWithForeignTask(t *testing.T) {
	s := ownSessionForeignTask()
	s.logger = scopeTestLogger(t)

	if s.CancelTransientRetry(context.Background(), "task-victim", "sess-mine") {
		t.Error("an owned session must not unlock another user's task")
	}
}

// TestAuthorizeTaskSessionPairRejectsInconsistentPair covers the consistency
// half: both IDs authorized, but naming a session that belongs to a different
// task must still be refused.
func TestAuthorizeTaskSessionPairRejectsInconsistentPair(t *testing.T) {
	s := &Service{
		logger:             scopeTestLogger(t),
		sessionAccessCheck: func(context.Context, string) error { return nil },
		taskAccessCheck:    func(context.Context, string) error { return nil },
		repo:               &pairStubRepo{sessionTaskID: "task-other"},
	}

	if err := s.authorizeTaskSessionPair(context.Background(), "task-mine", "sess-1"); err == nil {
		t.Error("expected refusal for a session that belongs to a different task")
	}
}

func TestAuthorizeTaskSessionPairAcceptsConsistentPair(t *testing.T) {
	s := &Service{
		logger:             scopeTestLogger(t),
		sessionAccessCheck: func(context.Context, string) error { return nil },
		taskAccessCheck:    func(context.Context, string) error { return nil },
		repo:               &pairStubRepo{sessionTaskID: "task-mine"},
	}

	if err := s.authorizeTaskSessionPair(context.Background(), "task-mine", "sess-1"); err != nil {
		t.Errorf("consistent pair was refused: %v", err)
	}
}
