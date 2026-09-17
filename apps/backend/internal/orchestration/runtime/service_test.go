package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	store "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	runstore "github.com/kandev/kandev/internal/runs/repository/sqlite"
	runservice "github.com/kandev/kandev/internal/runs/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskstore "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflow "github.com/kandev/kandev/internal/workflow/repository"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"testing"
)

type testTasks struct {
	tasks map[string]*taskmodels.Task
	text  string
}

func (f *testTasks) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	if t := f.tasks[id]; t != nil {
		return t, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *testTasks) ListTaskSessions(context.Context, string) ([]*taskmodels.TaskSession, error) {
	return nil, nil
}
func (f *testTasks) GetLastAgentMessage(context.Context, string) (string, error) { return f.text, nil }
func (f *testTasks) GetLastAgentMessageForTurn(context.Context, string) (string, error) {
	return f.text, nil
}
func newRuntime(t *testing.T) (*Service, *sqlx.DB, string) {
	t.Helper()
	ctx := context.Background()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
	require.NoError(t, err)
	_, err = taskstore.NewWithDB(db, db, log)
	require.NoError(t, err)
	_, err = workflow.NewWithDB(db, db, log)
	require.NoError(t, err)
	profiles, _, err := settingsstore.Provide(db, db, log)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('ws','Workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	repo := store.New(db, db)
	require.NoError(t, repo.Migrate())
	runs := runstore.NewWithDB(db, db)
	require.NoError(t, runs.Migrate())
	require.NoError(t, profiles.CreateAgent(ctx, &settings.Agent{ID: "claude", Name: "Claude"}))
	require.NoError(t, profiles.CreateAgentProfile(ctx, &settings.AgentProfile{ID: "personal", AgentID: "claude", Name: "Personal", Model: "default"}))
	a := &settings.AgentProfile{ID: "chief", AgentID: "claude", WorkspaceID: "ws", Role: settings.AgentRoleAssistant, Status: settings.AgentStatusIdle, Name: "Chief", Settings: `{"routing":{"execution_profile_id":"personal"}}`, ExecutorPreference: `{"executor_profile_id":"local"}`}
	require.NoError(t, profiles.CreateAgentProfile(ctx, a))
	require.NoError(t, repo.RegisterOrchestrator(ctx, a.ID, "ws", "chief-of-staff"))
	conversation, err := repo.EnsureAgentConversation(ctx, a)
	require.NoError(t, err)
	svc := &Service{AssistantEnabled: true, Authority: &testAssistantAuthority{revision: "fixture"}, Repo: repo, Personas: &personas.Service{Profiles: profiles, Repo: repo}, Runs: runs, Queue: runservice.New(runs, nil, log, nil), Auth: runtimeauth.NewAgentAuth(""), Tasks: &testTasks{tasks: map[string]*taskmodels.Task{}}, APIURL: "http://localhost:1", CLI: "agentctl"}
	return svc, db, conversation.TaskID
}
func TestConversationRunsWithoutOffice(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	var officeTables int
	require.NoError(t, db.Get(&officeTables, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE 'office_%'`))
	require.Zero(t, officeTables)
	s.Start = func(ctx context.Context, l Launch) error {
		require.Equal(t, "personal", l.ProfileID)
		require.Equal(t, "/api/v1/orchestration", l.Env["KANDEV_RUNTIME_API_PREFIX"])
		require.Equal(t, "true", l.Env["KANDEV_PERSONAL_ASSISTANT_ENABLED"])
		return l.OnSessionPrepared(ctx, "session")
	}
	for _, key := range []string{"first", "second"} {
		require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", key, nil))
	}
	first, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	handled, err := s.Process(ctx, first)
	require.True(t, handled)
	require.NoError(t, err)
	_, err = s.Runs.ClaimNextEligibleRun(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)
	event := bus.NewEvent(events.AgentCompleted, "test", map[string]string{"task_id": task, "session_id": "session", "run_id": "old"})
	require.NoError(t, s.onEvent(ctx, event))
	row, err := s.Runs.GetRunByID(ctx, first.ID)
	require.NoError(t, err)
	require.EqualValues(t, "claimed", row.Status)
	event.Data = map[string]string{"task_id": task, "session_id": "session", "run_id": first.ID}
	require.NoError(t, s.onEvent(ctx, event))
	next, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, next.ID)
}
func TestStreamingReplyIsBridgedOncePerTurn(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	s.Tasks.(*testTasks).text = "Completed the requested review."
	event := bus.NewEvent(events.AgentTurnMessageSaved, "test", map[string]string{"task_id": task, "session_id": "session", "turn_id": "turn"})
	require.NoError(t, s.onEvent(ctx, event))
	require.NoError(t, s.onEvent(ctx, event))
	rows, err := s.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, s.Tasks.(*testTasks).text, rows[0].Body)
	event.Data = map[string]string{"task_id": task, "session_id": "session", "turn_id": "next"}
	require.NoError(t, s.onEvent(ctx, event))
	rows, err = s.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NoError(t, s.Repo.UpsertAgentMemory(ctx, &models.AgentMemory{AgentProfileID: "chief", Layer: "user", Key: "preference", Content: "Short updates", Metadata: "{}"}))
	require.NoError(t, s.Repo.Migrate())
	memory, err := s.Repo.ListAgentMemory(ctx, "chief")
	require.NoError(t, err)
	require.Len(t, memory, 1)
}

func TestRestartDoesNotReplayAnInterruptedConversation(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	require.NoError(t, s.QueueTurn(ctx, "chief", task, "task_comment", "interrupted", nil))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	require.NoError(t, s.RecoverInterrupted(ctx))
	row, err := s.Runs.GetRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.EqualValues(t, "failed", row.Status)
	require.NotEmpty(t, row.ErrorMessage)
	_, err = s.Runs.ClaimNextEligibleRun(ctx)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
