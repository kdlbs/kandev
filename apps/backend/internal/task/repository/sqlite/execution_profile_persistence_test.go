package sqlite

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExecutionProfilePersistenceModelsExposeDistinctIdentity(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		model interface{}
	}{
		{name: "task session", model: models.TaskSession{}},
		{name: "running executor", model: models.ExecutorRunning{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field, ok := reflect.TypeOf(tc.model).FieldByName("ExecutionProfileID")
			if !ok {
				t.Fatal("ExecutionProfileID field is missing")
			}
			if got := field.Tag.Get("json"); got != "execution_profile_id" {
				t.Fatalf("ExecutionProfileID JSON tag = %q, want execution_profile_id", got)
			}
		})
	}
}

func TestExecutionProfileColumnsExistOnFreshSchema(t *testing.T) {
	repo := newRepoForSessionTests(t)

	for _, query := range []string{
		`SELECT execution_profile_id FROM task_sessions LIMIT 0`,
		`SELECT execution_profile_id FROM executors_running LIMIT 0`,
	} {
		rows, err := repo.db.Query(query)
		if err != nil {
			t.Fatalf("fresh schema query %q: %v", query, err)
		}
		_ = rows.Close()
	}
}

func TestExecutionProfileColumnsReplayOnLegacySchema(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-legacy-execution-profile")
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-legacy-execution-profile", TaskID: "task-legacy-execution-profile",
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-legacy-execution-profile", SessionID: "session-legacy-execution-profile",
		TaskID: "task-legacy-execution-profile", ExecutorID: "exec-legacy",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusStarting,
		ResumeToken: "legacy-token",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}

	for _, table := range []string{"task_sessions", "executors_running"} {
		if _, err := repo.db.Exec(`ALTER TABLE ` + table + ` DROP COLUMN execution_profile_id`); err != nil {
			t.Fatalf("drop legacy %s.execution_profile_id: %v", table, err)
		}
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations on legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}

	var sessionExecutionProfileID string
	if err := repo.db.Get(&sessionExecutionProfileID,
		`SELECT execution_profile_id FROM task_sessions WHERE id = 'session-legacy-execution-profile'`); err != nil {
		t.Fatalf("read migrated task session: %v", err)
	}
	if sessionExecutionProfileID != "" {
		t.Fatalf("legacy session execution_profile_id = %q, want empty default", sessionExecutionProfileID)
	}
	var executorExecutionProfileID, resumeToken string
	if err := repo.db.QueryRow(`
		SELECT execution_profile_id, resume_token FROM executors_running
		WHERE session_id = 'session-legacy-execution-profile'
	`).Scan(&executorExecutionProfileID, &resumeToken); err != nil {
		t.Fatalf("read migrated running executor: %v", err)
	}
	if executorExecutionProfileID != "" {
		t.Fatalf("legacy executor execution_profile_id = %q, want empty default", executorExecutionProfileID)
	}
	if resumeToken != "legacy-token" {
		t.Fatalf("migration lost resume token: got %q", resumeToken)
	}
}

func TestExecutionProfileIDsRoundTripThroughRepositories(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedExecutorRunningCleanupTask(t, repo, "task-execution-profile")

	session := &models.TaskSession{
		ID: "session-execution-profile", TaskID: "task-execution-profile",
		AgentProfileID: "office-cto", ExecutionProfileID: "codex-gpt-5-6",
		State: models.TaskSessionStateRunning,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	gotSession, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if gotSession.ExecutionProfileID != "codex-gpt-5-6" {
		t.Fatalf("execution_profile_id = %q, want codex-gpt-5-6", gotSession.ExecutionProfileID)
	}
	if gotSession.AgentProfileID != "office-cto" {
		t.Fatalf("agent_profile_id = %q, want stable office identity", gotSession.AgentProfileID)
	}

	gotSession.ExecutionProfileID = "claude-opus"
	if err := repo.UpdateTaskSession(ctx, gotSession); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
	listedSessions, err := repo.ListTaskSessions(ctx, session.TaskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(listedSessions) != 1 {
		t.Fatalf("ListTaskSessions count = %d, want 1", len(listedSessions))
	}
	if listedSessions[0].ExecutionProfileID != "claude-opus" {
		t.Fatalf("updated execution_profile_id = %q, want claude-opus", listedSessions[0].ExecutionProfileID)
	}
	if listedSessions[0].AgentProfileID != "office-cto" {
		t.Fatalf("updated agent_profile_id = %q, want stable office identity", listedSessions[0].AgentProfileID)
	}

	running := &models.ExecutorRunning{
		ID: session.ID, SessionID: session.ID, TaskID: session.TaskID,
		ExecutionProfileID: "codex-gpt-5-6",
		ExecutorID:         "exec-local", Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, ResumeToken: "codex-token",
	}
	if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	gotRunning, err := repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if gotRunning.ExecutionProfileID != "codex-gpt-5-6" {
		t.Fatalf("executor execution_profile_id = %q, want codex-gpt-5-6", gotRunning.ExecutionProfileID)
	}

	running.ExecutionProfileID = "claude-opus"
	running.ResumeToken = "claude-token"
	if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
		t.Fatalf("UpsertExecutorRunning update: %v", err)
	}
	listedRunning, err := repo.ListExecutorsRunningByTaskID(ctx, session.TaskID)
	if err != nil {
		t.Fatalf("ListExecutorsRunningByTaskID: %v", err)
	}
	if len(listedRunning) != 1 {
		t.Fatalf("ListExecutorsRunningByTaskID count = %d, want 1", len(listedRunning))
	}
	if listedRunning[0].ExecutionProfileID != "claude-opus" {
		t.Fatalf("updated executor execution_profile_id = %q, want claude-opus", listedRunning[0].ExecutionProfileID)
	}
	if listedRunning[0].ResumeToken != "claude-token" {
		t.Fatalf("updated resume token = %q, want claude-token", listedRunning[0].ResumeToken)
	}
}
