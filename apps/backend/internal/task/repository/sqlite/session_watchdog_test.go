package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type watchdogSessionReader interface {
	ListStepSignalWatchdogSessions(context.Context) ([]*models.TaskSession, error)
}

// @covers AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.1
func TestStepSignalWatchdogProjection(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertWatchdogProjection(t, repo)
	assertWatchdogSignalShapes(t, repo)
}

func TestStepSignalWatchdogProjectionPostgres(t *testing.T) {
	repo := openPostgresRepo(t)
	assertWatchdogProjection(t, repo)
	assertWatchdogSignalShapes(t, repo)
}

func assertWatchdogSignalShapes(t *testing.T, repo *Repository) {
	t.Helper()
	reader := any(repo).(watchdogSessionReader)
	for _, raw := range []string{
		`{}`, `null`, ``, `{"pending_step_completion_signal":null}`,
		`{"pending_step_completion_signal":"not-an-object"}`,
		`{"pending_step_completion_signal":[]}`,
		`{"pending_step_completion_signal":{"step_id":"review","signaled_at":"invalid"}}`,
		`{"pending_step_completion_signal":{"step_id":5}}`,
	} {
		t.Run("metadata="+raw, func(t *testing.T) {
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET metadata = ? WHERE id = ?`), raw, "watchdog-0"); err != nil {
				t.Fatal(err)
			}
			full, err := repo.GetTaskSession(t.Context(), "watchdog-0")
			if err != nil {
				t.Fatal(err)
			}
			rows, err := reader.ListStepSignalWatchdogSessions(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.ID != full.ID {
					continue
				}
				signal, valid := models.LoadPendingStepSignal(row.Metadata)
				wantSignal, wantValid := models.LoadPendingStepSignal(full.Metadata)
				if !reflect.DeepEqual(signal, wantSignal) || valid != wantValid {
					t.Fatalf("signal meaning changed for %q: %+v/%v vs %+v/%v", raw, signal, valid, wantSignal, wantValid)
				}
				return
			}
			t.Fatal("session omitted")
		})
	}
	for _, raw := range []string{`not-json`, `[]`, `42`, `"scalar"`} {
		if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE task_sessions SET metadata = ? WHERE id = ?`), raw, "watchdog-0"); err != nil {
			t.Fatal(err)
		}
		if _, err := reader.ListStepSignalWatchdogSessions(t.Context()); err == nil {
			t.Fatalf("invalid metadata %q did not fail closed", raw)
		}
	}
}

func assertWatchdogProjection(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "watchdog-task", Title: "Automation fixture", Origin: models.TaskOriginAutomationRun,
	}); err != nil {
		t.Fatal(err)
	}
	states := []models.TaskSessionState{
		models.TaskSessionStateCreated, models.TaskSessionStateStarting, models.TaskSessionStateRunning,
		models.TaskSessionStateWaitingForInput, models.TaskSessionStateIdle,
		models.TaskSessionStateCompleted, models.TaskSessionStateCancelled, models.TaskSessionStateFailed,
	}
	for i, state := range states {
		metadata := map[string]interface{}{
			"mcp_attachment_state": strings.Repeat("x", 32<<10),
			models.SessionMetaKeyPendingStepCompletion: models.PendingStepCompletionSignal{
				StepID: "review", SignaledAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
				Source: models.StepCompletionSourceAgent, Summary: "done", Handoff: "retained", Blockers: "none",
			},
		}
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: fmt.Sprintf("watchdog-%d", i), TaskID: "watchdog-task", State: state,
			IsPassthrough: i%2 == 0, Metadata: metadata,
			AgentProfileSnapshot: map[string]interface{}{"padding": strings.Repeat("y", 32<<10)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Task archival is not a session-state filter for this scanner.
	if _, err := repo.db.Exec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = 'watchdog-task'`); err != nil {
		t.Fatal(err)
	}
	full, err := repo.ListActiveTaskSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := any(repo).(watchdogSessionReader)
	if !ok {
		t.Fatal("watchdog has no narrow reader; the current scan decodes full metadata and snapshots")
	}
	got, err := reader.ListStepSignalWatchdogSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || len(full) != len(got) {
		t.Fatalf("active scope: full=%d projected=%d, want four active states including archived tasks", len(full), len(got))
	}
	for i, row := range got {
		want := full[i]
		if row.ID != want.ID || row.TaskID != want.TaskID || row.State != want.State || row.IsPassthrough != want.IsPassthrough {
			t.Fatalf("scan identity changed: %+v vs %+v", row, want)
		}
		signal, valid := models.LoadPendingStepSignal(row.Metadata)
		wantSignal, wantValid := models.LoadPendingStepSignal(want.Metadata)
		if !reflect.DeepEqual(signal, wantSignal) || valid != wantValid {
			t.Fatalf("signal changed: %+v vs %+v", signal, wantSignal)
		}
		encoded, _ := json.Marshal(row.Metadata)
		if len(encoded) > 1024 || row.AgentProfileSnapshot != nil || row.Worktrees != nil {
			t.Fatalf("watchdog loaded unrelated payloads: metadata bytes=%d", len(encoded))
		}
	}
	if len(full[0].Metadata["mcp_attachment_state"].(string)) != 32<<10 {
		t.Fatal("general active-session reader lost full metadata")
	}
}
