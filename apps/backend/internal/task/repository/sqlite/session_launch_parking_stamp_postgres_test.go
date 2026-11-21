package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresUpdateTaskSessionIfCurrentStateForLaunch keeps launch parking
// guards valid on both database dialects. It skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskSessionIfCurrentStateForLaunch(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	parkedAt := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)

	for _, test := range []struct {
		name          string
		launchStamp   string
		currentStamp  string
		wantCommitted bool
	}{
		{name: "absent parking record", wantCommitted: true},
		{name: "matching parking stamp", launchStamp: "park-same", currentStamp: "park-same", wantCommitted: true},
		{name: "changed parking stamp", launchStamp: "park-old", currentStamp: "park-new", wantCommitted: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			taskID := "task-launch-parking-pg-" + test.name
			sessionID := "session-launch-parking-pg-" + test.name
			seedPostgresTaskSession(t, repo, taskID, sessionID)
			if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateStarting, ""); err != nil {
				t.Fatalf("set session starting: %v", err)
			}
			if test.launchStamp != "" {
				if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyWorkflowParking, models.WorkflowParking{
					Stamp: test.launchStamp, ParkedAt: parkedAt, SourceSessionID: sessionID,
				}); err != nil {
					t.Fatalf("set launch parking marker: %v", err)
				}
			}
			launch, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("load launch snapshot: %v", err)
			}
			if test.currentStamp != "" && test.currentStamp != test.launchStamp {
				if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyWorkflowParking, models.WorkflowParking{
					Stamp: test.currentStamp, ParkedAt: parkedAt.Add(time.Minute), SourceSessionID: sessionID,
				}); err != nil {
					t.Fatalf("set current parking marker: %v", err)
				}
			}
			launch.State = models.TaskSessionStateRunning

			committed, err := repo.UpdateTaskSessionIfCurrentStateForLaunch(
				ctx, launch, models.TaskSessionStateStarting, "attempt-"+sessionID,
			)
			if err != nil {
				t.Fatalf("update launch state: %v", err)
			}
			if committed != test.wantCommitted {
				t.Fatalf("committed = %t, want %t", committed, test.wantCommitted)
			}
			current, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("reload session: %v", err)
			}
			wantState := models.TaskSessionStateStarting
			if test.wantCommitted {
				wantState = models.TaskSessionStateRunning
			}
			if current.State != wantState {
				t.Fatalf("state = %q, want %q", current.State, wantState)
			}
		})
	}
}

func TestPostgresUpdateTaskSessionAgentStartAttemptIfCurrentStateForLaunch(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-agent-attempt-only-pg"
	const sessionID = "session-agent-attempt-only-pg"
	seedPostgresTaskSession(t, repo, taskID, sessionID)

	launch, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load launch snapshot: %v", err)
	}
	launch.TaskEnvironmentID = "environment-not-yet-persisted"
	changed, err := repo.UpdateTaskSessionAgentStartAttemptIfCurrentStateForLaunch(
		ctx, launch, launch.State, "attempt-pg",
	)
	if err != nil {
		t.Fatalf("claim start attempt: %v", err)
	}
	if !changed {
		t.Fatal("start attempt was not claimed")
	}
	current, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if current.TaskEnvironmentID != "" {
		t.Fatalf("task environment binding = %q, want unchanged empty binding", current.TaskEnvironmentID)
	}
	if got := models.StringFromAny(current.Metadata[models.SessionMetaKeyAgentStartAttemptID]); got != "attempt-pg" {
		t.Fatalf("stored attempt = %q, want attempt-pg", got)
	}
}

func TestPostgresUpdateTaskSessionStateIfCurrentStartAttempt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-launch-attempt-pg"
	const sessionID = "session-launch-attempt-pg"
	seedPostgresTaskSession(t, repo, taskID, sessionID)
	if err := repo.UpdateTaskSessionState(ctx, sessionID, models.TaskSessionStateStarting, ""); err != nil {
		t.Fatalf("set session starting: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyAgentStartAttemptID, "attempt-current"); err != nil {
		t.Fatalf("set current start attempt: %v", err)
	}

	changed, _, err := repo.UpdateTaskSessionStateIfCurrentStartAttempt(
		ctx, taskID, sessionID, "attempt-old", models.TaskSessionStateStarting,
		models.TaskSessionStateFailed, "stale launch failure",
	)
	if err != nil {
		t.Fatalf("reject stale attempt: %v", err)
	}
	if changed {
		t.Fatal("stale attempt changed the session")
	}
	current, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("read session after stale attempt: %v", err)
	}
	if current.State != models.TaskSessionStateStarting {
		t.Fatalf("stale attempt state = %q, want STARTING", current.State)
	}

	changed, _, err = repo.UpdateTaskSessionStateIfCurrentStartAttempt(
		ctx, taskID, sessionID, "attempt-current", models.TaskSessionStateStarting,
		models.TaskSessionStateFailed, "current launch failure",
	)
	if err != nil {
		t.Fatalf("commit current attempt: %v", err)
	}
	if !changed {
		t.Fatal("current attempt did not change the session")
	}
	current, err = repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("read session after current attempt: %v", err)
	}
	if current.State != models.TaskSessionStateFailed || current.ErrorMessage != "current launch failure" {
		t.Fatalf("current attempt session = %+v, want FAILED with current error", current)
	}
}
