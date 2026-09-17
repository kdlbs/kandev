package runtime

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type attentionFixture struct {
	sources []models.AttentionSource
	err     error
	reads   int
}

func (f *attentionFixture) ReadAttention(_ context.Context, _ *models.AssistantBinding, _ string, _ []models.Attention) ([]models.AttentionSource, error) {
	f.reads++
	return f.sources, f.err
}

func assistantAttentionFixture(t *testing.T) (*Service, *sqlx.DB, *models.AssistantBinding, *attentionFixture) {
	t.Helper()
	s, db, task := newRuntime(t)
	ctx := context.Background()
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	b, err := s.Repo.AssistantBinding(ctx, "owner")
	require.NoError(t, err)
	require.NoError(t, s.Repo.PutComment(ctx, &models.TaskComment{ID: "source", TaskID: task, AuthorType: "user", AuthorID: "owner", Source: "user", Body: "Review a sample task"}))
	o := &models.Objective{BindingID: b.ID, WorkspaceID: "ws", SourceCommentID: "source", Title: "Sample task", Mode: "execute", Status: "active", Acceptance: []models.Criterion{{ID: "checked", Description: "Checks pass"}}}
	require.NoError(t, s.Repo.CreateObjective(ctx, o))
	_, err = db.Exec(`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('worker','ws','Synthetic worker',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	require.NoError(t, s.Repo.LinkObjectiveTask(ctx, models.ObjectiveTask{ObjectiveID: o.ID, TaskID: "worker", Role: "implementation", OperationID: "dispatch"}))
	f := &attentionFixture{sources: []models.AttentionSource{{SourceID: "question-one", SessionID: "older", Kind: "question", State: "pending", SourceRevision: "native-v1", Summary: "Choose a sample color"}, {SourceID: "permission-two", SessionID: "newer", Kind: "permission", State: "pending", SourceRevision: "native-v1", Summary: "Review a tool permission"}}}
	s.Tasks.(*testTasks).tasks["worker"] = &taskmodels.Task{ID: "worker", WorkspaceID: "ws"}
	s.Attention = f
	return s, db, b, f
}

func TestAssistantAttentionAllSessions(t *testing.T) {
	s, db, b, f := assistantAttentionFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NotEqual(t, rows[0].SessionID, rows[1].SessionID)
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.TaskSessionStateChanged, "test", map[string]string{"task_id": "worker", "session_id": "newer"})))
	require.NoError(t, s.ReconcileAttentionTask(ctx, "foreign"))
	require.Equal(t, 2, f.reads)
	var count int
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM runs`))
	require.Equal(t, 2, count)
	response := runtimeRequest(t, assistantRouter(s), "GET", "/api/v1/orchestration/assistant/attention?limit=1", "", "", nil)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "next_cursor")
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "foreign"), "GET", "/api/v1/orchestration/assistant/attention", "", "", nil).Code)
}

func TestAssistantAttentionRestartDedup(t *testing.T) {
	s, db, b, _ := assistantAttentionFixture(t)
	ctx := context.Background()
	queue := s.Queue
	s.Queue = nil
	require.Error(t, s.ReconcileAttentionTask(ctx, "worker"))
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	s.Queue = queue
	_, err = db.Exec(`CREATE TRIGGER lose_wake_ack BEFORE UPDATE ON orchestration_attention_wakes BEGIN SELECT RAISE(ABORT,'lost wake receipt'); END`)
	require.NoError(t, err)
	require.Error(t, s.ReconcileAttentionTask(ctx, "worker"))
	_, err = db.Exec(`DROP TRIGGER lose_wake_ack`)
	require.NoError(t, err)
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	var count int
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM runs`))
	require.Equal(t, 2, count)
	rows, err = s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	for _, row := range rows {
		require.Equal(t, row.Revision, row.LastNotifiedRevision)
	}
}

func TestAssistantAttentionLatencyAndNoop(t *testing.T) {
	s, db, _, f := assistantAttentionFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	require.NoError(t, s.ReconcileAttention(ctx))
	require.Equal(t, 1, f.reads)
	now = now.Add(59 * time.Second)
	require.NoError(t, s.ReconcileAttention(ctx))
	require.Equal(t, 1, f.reads)
	now = now.Add(time.Second)
	require.NoError(t, s.ReconcileAttention(ctx))
	require.Equal(t, 2, f.reads)
	require.NoError(t, s.onEvent(ctx, bus.NewEvent(events.MessageUpdated, "test", map[string]string{"task_id": "worker"})))
	require.Equal(t, 3, f.reads)
	var count int
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM runs`))
	require.Equal(t, 2, count)
}

func TestAssistantAttentionPausedAndExpired(t *testing.T) {
	s, db, b, f := assistantAttentionFixture(t)
	ctx := context.Background()
	_, err := db.Exec(`UPDATE agent_profiles SET status='paused' WHERE id='chief'`)
	require.NoError(t, err)
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	var count int
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM runs`))
	require.Zero(t, count)
	f.sources[0].State = "expired"
	f.sources[0].SourceRevision = "native-v2"
	f.sources[1].State = "resolved"
	_, err = db.Exec(`UPDATE agent_profiles SET status='idle' WHERE id='chief'`)
	require.NoError(t, err)
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM runs`))
	require.Zero(t, count)
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.NotEqual(t, "pending", row.State)
	}
	s.AssistantEnabled = false
	f.sources[0].State = "pending"
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM runs`))
	require.Zero(t, count)
}

func TestAssistantAttentionPartialSourceFailure(t *testing.T) {
	s, _, b, f := assistantAttentionFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	f.err = errors.New("synthetic private diagnostic")
	require.Error(t, s.ReconcileAttentionTask(ctx, "worker"))
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	for _, row := range rows {
		require.Equal(t, "unknown", row.State)
		require.NotContains(t, row.Summary, "private diagnostic")
	}
	f.err = nil
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
}

func TestAssistantAttentionExpiredQueueDoesNotLaunch(t *testing.T) {
	s, _, _, f := assistantAttentionFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	for i := range f.sources {
		f.sources[i].State = "expired"
	}
	launches := 0
	s.Start = func(context.Context, Launch) error { launches++; return nil }
	handled, err := s.Process(ctx, run)
	require.True(t, handled)
	require.ErrorContains(t, err, "no longer current")
	require.Zero(t, launches)
}

func TestAssistantAttentionUnlinkedTaskIsHidden(t *testing.T) {
	s, db, b, _ := assistantAttentionFixture(t)
	ctx := context.Background()
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	_, err := db.Exec(`DELETE FROM orchestration_objective_tasks WHERE task_id='worker'`)
	require.NoError(t, err)
	response := runtimeRequest(t, assistantRouter(s), "GET", "/api/v1/orchestration/assistant/attention", "", "", nil)
	require.Equal(t, 200, response.Code)
	require.NotContains(t, response.Body.String(), "worker")
	rows, err := s.Repo.AttentionPage(ctx, b.ID, "", 100)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	run, err := s.Runs.ClaimNextEligibleRun(ctx)
	require.NoError(t, err)
	launches := 0
	s.Start = func(context.Context, Launch) error { launches++; return nil }
	_, err = s.Process(ctx, run)
	require.ErrorContains(t, err, "no longer managed")
	require.Zero(t, launches)
}
