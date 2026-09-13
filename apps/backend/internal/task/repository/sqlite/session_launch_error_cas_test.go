package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSetSessionMetadataKeyIfStampIsAtomicAndPreservesOtherKeys(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "session-cas-write-task", "session-cas-write", "turn-cas-write")
	ctx := context.Background()
	if err := repo.SetSessionMetadataKey(ctx, "session-cas-write", "other_key", "keep me"); err != nil {
		t.Fatalf("seed session metadata: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, "session-cas-write", models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message:    "old error",
		OccurredAt: time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC),
		StampValue: "old-stamp",
	}); err != nil {
		t.Fatalf("seed session error: %v", err)
	}

	stored, err := repo.SetSessionMetadataKeyIfStamp(ctx, "session-cas-write", models.SessionMetaKeyLastAgentError, "old-stamp", models.LastAgentError{
		Message:    "new error",
		OccurredAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
		StampValue: "new-stamp",
	})
	if err != nil {
		t.Fatalf("stamped session write: %v", err)
	}
	if !stored {
		t.Fatal("current session error was not replaced")
	}

	stored, err = repo.SetSessionMetadataKeyIfStamp(ctx, "session-cas-write", models.SessionMetaKeyLastAgentError, "old-stamp", models.LastAgentError{
		Message:    "stale error",
		OccurredAt: time.Date(2026, 8, 20, 11, 0, 0, 0, time.UTC),
		StampValue: "stale-stamp",
	})
	if err != nil {
		t.Fatalf("stale stamped session write: %v", err)
	}
	if stored {
		t.Fatal("stale session write replaced the newer error")
	}

	session, err := repo.GetTaskSession(ctx, "session-cas-write")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.Stamp() != "new-stamp" {
		t.Fatalf("session error = %#v, want new-stamp", lastError)
	}
	if session.Metadata["other_key"] != "keep me" {
		t.Fatalf("other session metadata = %#v, want preserved value", session.Metadata["other_key"])
	}
}

func TestRemoveSessionMetadataKeyIfStampDoesNotEraseNewerError(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-session-error-cas", "session-error-cas", "turn-error-cas")
	ctx := context.Background()
	if err := repo.SetSessionMetadataKey(ctx, "session-error-cas", "last_agent_error", map[string]interface{}{
		"message":     "new error",
		"stamp":       "new-stamp",
		"occurred_at": "2026-08-19T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed session error: %v", err)
	}

	removed, err := repo.RemoveSessionMetadataKeyIfStamp(ctx, "session-error-cas", "last_agent_error", "old-stamp")
	if err != nil {
		t.Fatalf("RemoveSessionMetadataKeyIfStamp: %v", err)
	}
	if removed {
		t.Fatal("stale stamp removed a newer session error")
	}

	removed, err = repo.RemoveSessionMetadataKeyIfStamp(ctx, "session-error-cas", "last_agent_error", "new-stamp")
	if err != nil {
		t.Fatalf("RemoveSessionMetadataKeyIfStamp with current stamp: %v", err)
	}
	if !removed {
		t.Fatal("current stamp did not remove the session error")
	}
}

func TestCommitBootstrapFailureIfCurrentExecutionGuardsStateExecutionAndStamp(t *testing.T) {
	tests := []struct {
		name          string
		seedError     *models.LastAgentError
		expectedStamp string
		seedExecution string
		commitExec    string
		expectedState models.TaskSessionState
		wantChanged   bool
	}{
		{
			name:          "absent error",
			seedExecution: "exec-current",
			commitExec:    "exec-current",
			expectedState: models.TaskSessionStateStarting,
			wantChanged:   true,
		},
		{
			name:          "unchanged stamp",
			seedError:     &models.LastAgentError{Message: "old", OccurredAt: time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC), StampValue: "old-stamp"},
			expectedStamp: "old-stamp",
			seedExecution: "exec-current",
			commitExec:    "exec-current",
			expectedState: models.TaskSessionStateStarting,
			wantChanged:   true,
		},
		{
			name:          "successor execution",
			seedExecution: "exec-successor",
			commitExec:    "exec-old",
			expectedState: models.TaskSessionStateStarting,
			wantChanged:   false,
		},
		{
			name:          "state changed",
			seedExecution: "exec-current",
			commitExec:    "exec-current",
			expectedState: models.TaskSessionStateRunning,
			wantChanged:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			seedForMsgTest(t, repo, "task-bootstrap-atomic", "session-bootstrap-atomic", "turn-bootstrap-atomic")
			ctx := context.Background()
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET state = ?, metadata = ? WHERE id = ?`),
				string(models.TaskSessionStateStarting), `{}`, "session-bootstrap-atomic"); err != nil {
				t.Fatalf("seed session state: %v", err)
			}
			if tt.seedError != nil {
				if err := repo.SetSessionMetadataKey(ctx, "session-bootstrap-atomic", models.SessionMetaKeyLastAgentError, *tt.seedError); err != nil {
					t.Fatalf("seed session error: %v", err)
				}
			}
			if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
				ID: "session-bootstrap-atomic", SessionID: "session-bootstrap-atomic", TaskID: "task-bootstrap-atomic",
				AgentExecutionID: tt.seedExecution,
			}); err != nil {
				t.Fatalf("seed execution: %v", err)
			}

			changed, _, err := repo.CommitBootstrapFailureIfCurrentExecution(
				ctx,
				"task-bootstrap-atomic",
				"session-bootstrap-atomic",
				tt.commitExec,
				tt.expectedState,
				tt.expectedStamp,
				models.LastAgentError{Message: "new", OccurredAt: time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC), StampValue: "new-stamp"},
			)
			if err != nil {
				t.Fatalf("commit bootstrap failure: %v", err)
			}
			if changed != tt.wantChanged {
				t.Fatalf("changed = %t, want %t", changed, tt.wantChanged)
			}
			session, err := repo.GetTaskSession(ctx, "session-bootstrap-atomic")
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			if tt.wantChanged {
				if session.State != models.TaskSessionStateFailed || session.ErrorMessage != "new" {
					t.Fatalf("committed session = %+v, want FAILED/new", session)
				}
				value, ok := models.LoadLastAgentError(session.Metadata)
				if !ok || value.Stamp() != "new-stamp" {
					t.Fatalf("committed error = %+v, found=%t", value, ok)
				}
				return
			}
			if session.State != models.TaskSessionStateStarting {
				t.Fatalf("rejected session state = %q, want STARTING", session.State)
			}
			if value, ok := models.LoadLastAgentError(session.Metadata); ok && value.Stamp() == "new-stamp" {
				t.Fatal("rejected bootstrap failure replaced session error")
			}
		})
	}
}
