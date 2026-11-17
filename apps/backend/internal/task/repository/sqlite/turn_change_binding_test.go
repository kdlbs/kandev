package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestTurnChangeSetUsesSessionEnvironmentBinding(t *testing.T) {
	runTurnChangeSetEnvironmentBinding(t, newRepoForSessionTests(t))
}

func TestPostgresTurnChangeSetUsesSessionEnvironmentBinding(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	runTurnChangeSetEnvironmentBinding(t, repo)
}

func runTurnChangeSetEnvironmentBinding(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	for _, taskID := range []string{"owner", "member", "unrelated"} {
		seedSessionForTurns(t, repo, taskID, taskID+"-session")
		if err := repo.CreateTurn(ctx, &models.Turn{
			ID: taskID + "-turn", TaskID: taskID, TaskSessionID: taskID + "-session",
		}); err != nil {
			t.Fatalf("create %s turn: %v", taskID, err)
		}
		if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
			ID: taskID + "-environment", TaskID: taskID,
		}); err != nil {
			t.Fatalf("create %s environment: %v", taskID, err)
		}
	}
	for sessionID, environmentID := range map[string]string{
		"owner-session": "owner-environment", "member-session": "owner-environment",
		"unrelated-session": "unrelated-environment",
	} {
		if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
			`UPDATE task_sessions SET task_environment_id = ? WHERE id = ?`), environmentID, sessionID); err != nil {
			t.Fatalf("bind %s environment: %v", sessionID, err)
		}
	}
	for _, test := range []struct {
		name, taskID, sessionID, turnID, environmentID string
		wantAccepted                                   bool
	}{
		{"direct owner", "owner", "owner-session", "owner-turn", "owner-environment", true},
		{"bound shared environment", "member", "member-session", "member-turn", "owner-environment", true},
		{"unbound same task environment", "member", "member-session", "member-turn", "member-environment", false},
		{"unbound foreign environment", "member", "member-session", "member-turn", "unrelated-environment", false},
		{"missing environment", "member", "member-session", "member-turn", "missing-environment", false},
		{"wrong task", "owner", "member-session", "member-turn", "owner-environment", false},
		{"wrong session", "member", "owner-session", "member-turn", "owner-environment", false},
		{"wrong turn", "member", "member-session", "owner-turn", "owner-environment", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			changeSet := &models.TurnChangeSet{
				ID: "binding-" + test.name, TaskID: test.taskID, TaskSessionID: test.sessionID,
				TurnID: test.turnID, TaskEnvironmentID: test.environmentID,
				Availability: models.TurnChangeAvailabilityPending,
			}
			err := repo.CreateTurnChangeSet(ctx, changeSet)
			if !test.wantAccepted {
				if !errors.Is(err, ErrTurnChangeRelationship) {
					t.Fatalf("create unbound change set: %v, want relationship error", err)
				}
				if _, getErr := repo.GetTurnChangeSet(ctx, test.taskID, test.sessionID, changeSet.ID); !errors.Is(getErr, ErrTurnChangeSetNotFound) {
					t.Fatalf("rejected change set was persisted: %v", getErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("create bound change set: %v", err)
			}
			stored, err := repo.GetTurnChangeSet(ctx, test.taskID, test.sessionID, changeSet.ID)
			if err != nil || stored.TaskEnvironmentID != test.environmentID || stored.TaskID != test.taskID {
				t.Fatalf("stored change set = %+v, %v; want original task and bound environment", stored, err)
			}
		})
	}
}
