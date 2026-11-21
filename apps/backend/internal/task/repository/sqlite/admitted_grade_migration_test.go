package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAdmittedGradeSQLiteLegacySessionRebuilds(t *testing.T) {
	for _, workflowColumn := range []bool{false, true} {
		t.Run(map[bool]string{false: "agent_execution", true: "workflow_step"}[workflowColumn], func(t *testing.T) {
			ctx := context.Background()
			repo := newRepoForSessionTests(t)
			if err := repo.CreateTask(ctx, &models.Task{ID: "legacy-grade-task", WorkspaceID: "legacy-grade-workspace", Title: "Retained history"}); err != nil {
				t.Fatal(err)
			}
			session := &models.TaskSession{ID: "legacy-grade-session", TaskID: "legacy-grade-task", AdmittedExecutionGrade: "Legacy-A", AdmittedExecutionGradeKnown: true, AgentProfileSnapshot: map[string]any{"sentinel": "profile-history"}, Metadata: map[string]any{"sentinel": "session-history"}}
			if err := repo.CreateTaskSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			legacySessionColumnsForGrade(t, repo, workflowColumn)
			if workflowColumn {
				if err := repo.migrateSessionsRemoveWorkflowStepID(); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.migrateSessionsRemoveAgentExecutionID(); err != nil {
				t.Fatal(err)
			}
			if _, err := NewWithDB(repo.db, repo.ro, nil); err != nil {
				t.Fatal(err)
			}
			stored, err := repo.GetTaskSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !stored.AdmittedExecutionGradeKnown || stored.AdmittedExecutionGrade != "Legacy-A" || stored.AgentProfileSnapshot["sentinel"] != "profile-history" || stored.Metadata["sentinel"] != "session-history" {
				t.Fatalf("rebuilt snapshot=%+v", stored)
			}
			task, err := repo.GetTask(ctx, session.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Title != "Retained history" {
				t.Fatal("task history changed")
			}
		})
	}
}

func legacySessionColumnsForGrade(t *testing.T, repo *Repository, workflow bool) {
	t.Helper()
	// The historical session columns predate conversation projection triggers.
	// Remove only those triggers in this private fixture; NewWithDB restores them.
	var triggers []string
	if err := repo.db.Select(&triggers, `SELECT name FROM sqlite_master WHERE type = 'trigger' AND sql LIKE '%task_sessions%'`); err != nil {
		t.Fatal(err)
	}
	for _, name := range triggers {
		if _, err := repo.db.Exec(`DROP TRIGGER "` + strings.ReplaceAll(name, `"`, `""`) + `"`); err != nil {
			t.Fatal(err)
		}
	}

	for _, query := range []string{`ALTER TABLE task_sessions ADD COLUMN agent_execution_id TEXT NOT NULL DEFAULT ''`, `ALTER TABLE task_sessions ADD COLUMN container_id TEXT NOT NULL DEFAULT ''`} {
		if _, err := repo.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	if workflow {
		if _, err := repo.db.Exec(`ALTER TABLE task_sessions ADD COLUMN workflow_step_id TEXT NOT NULL DEFAULT ''`); err != nil {
			t.Fatal(err)
		}
	}
}
