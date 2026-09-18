package runtime

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type frictionTasks struct {
	*testTasks
	sessions map[string][]*taskmodels.TaskSession
}

func (f *frictionTasks) ListTaskSessions(_ context.Context, task string) ([]*taskmodels.TaskSession, error) {
	return f.sessions[task], nil
}

// @covers AC-ORCHESTRATION-ASSISTANT-008.1
func TestAssistantFrictionNativeObservations(t *testing.T) {
	s, db, b, attention := assistantAttentionFixture(t)
	ctx := context.Background()
	tasks := &frictionTasks{testTasks: s.Tasks.(*testTasks), sessions: map[string][]*taskmodels.TaskSession{
		"worker":     {{ID: "older", TaskID: "worker", AgentProfileID: "personal"}},
		"worker-two": {{ID: "older", TaskID: "worker-two", AgentProfileID: "personal"}},
	}}
	s.Tasks = tasks
	_, err := db.Exec(`INSERT INTO tasks(id,workspace_id,title,created_at,updated_at) VALUES('worker-two','ws','Another sample task',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	objectives, err := s.Repo.Objectives(ctx, b.ID, "", 10)
	require.NoError(t, err)
	require.NoError(t, s.Repo.LinkObjectiveTask(ctx, models.ObjectiveTask{ObjectiveID: objectives[0].ID, TaskID: "worker-two", Role: "implementation", OperationID: "dispatch-two"}))
	tasks.tasks["worker-two"] = &taskmodels.Task{ID: "worker-two", WorkspaceID: "ws"}
	attention.sources = attention.sources[:1]
	attention.sources[0].Summary = "CANARY_PRIVATE_PROMPT must never enter friction"
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	attention.sources[0].SourceRevision = "new-projection-of-same-request"
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	attention.sources[0].SourceID = "second-question"
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker"))
	attention.sources[0].SourceID = "third-question"
	require.NoError(t, s.ReconcileAttentionTask(ctx, "worker-two"))
	rows, err := s.Repo.ImprovementCandidates(ctx, b.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 3, rows[0].IncidentCount)
	require.Equal(t, 2, rows[0].TaskCount)
	require.Equal(t, "native", rows[0].Origin)
	evidence, err := s.Repo.ImprovementEvidence(ctx, b.ID, rows[0].Fingerprint, "", 100)
	require.NoError(t, err)
	require.Len(t, evidence, 3)
	response := runtimeRequest(t, assistantRouter(s), "GET", "/api/v1/orchestration/assistant/improvements", "", "", nil)
	require.Equal(t, 200, response.Code)
	require.NotContains(t, response.Body.String(), "CANARY_PRIVATE_PROMPT")
	require.Equal(t, 404, runtimeRequest(t, assistantRouter(s, "another-owner"), "GET", "/api/v1/orchestration/assistant/improvements", "", "", nil).Code)
	_, err = db.Exec(`INSERT INTO workspaces(id,name,created_at,updated_at) VALUES('other-workspace','Other synthetic workspace',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE orchestration_assistant_bindings SET workspace_id='other-workspace',version=version+1 WHERE id=?`, b.ID)
	require.NoError(t, err)
	rows, err = s.Repo.ImprovementCandidates(ctx, b.ID, "", 10)
	require.NoError(t, err)
	require.Empty(t, rows, "old workspace proposals must not follow a changed binding")
}
