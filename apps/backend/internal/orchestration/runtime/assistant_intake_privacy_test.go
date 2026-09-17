package runtime

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/stretchr/testify/require"
)

func TestAssistantPrivacyIntakeRechecksOwnershipAtCommit(t *testing.T) {
	s, _, task := newRuntime(t)
	ctx := context.Background()
	human := assistantRouter(s)
	require.Equal(t, 200, runtimeRequest(t, human, "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	_, _, err := s.Repo.AcceptComment(ctx, "chief", "foreign", &models.TaskComment{TaskID: task, AuthorID: "foreign", Body: "Delayed request authorized before selection"})
	require.ErrorIs(t, err, models.ErrConflict)
	switchPrivateAssistant(t, s, human, 1)
	_, _, err = s.Repo.AcceptComment(ctx, "chief", "inactive", &models.TaskComment{TaskID: task, AuthorID: "owner", Body: "Delayed request authorized before switching"})
	require.ErrorIs(t, err, models.ErrConflict)
	comments, err := s.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Empty(t, comments)
}

func TestAssistantPrivacyPendingIntakeCannotInheritNewOwner(t *testing.T) {
	s, _, task := newRuntime(t)
	queue := s.Queue
	s.Queue = nil
	path := "/api/v1/orchestration/tasks/" + task + "/comments"
	require.Equal(t, 201, runtimeRequest(t, assistantRouter(s, "foreign"), "POST", path, "", "", map[string]string{"body": "Old shared conversation instructions"}).Code)
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	s.Queue = queue
	ctx := context.Background()
	require.NoError(t, s.DispatchIntake(ctx))
	comments, err := s.Repo.ListComments(ctx, task, 10)
	require.NoError(t, err)
	require.Len(t, comments, 1)
	require.Empty(t, comments[0].RunID, "a pending foreign instruction must not acquire the selecting user's authority")
	require.Equal(t, "superseded", comments[0].ReceiptStatus)
	require.ErrorIs(t, s.QueueTurn(ctx, "chief", task, "task_comment", "stale-repair",
		map[string]any{"comment_id": comments[0].ID}), models.ErrConflict)
	pending, err := s.Repo.PendingIntake(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestAssistantBindingReplayPreservesLegacyImportOwner(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	require.Equal(t, 200, runtimeRequest(t, assistantRouter(s), "PUT", "/api/v1/orchestration/assistant", "", "", map[string]any{"orchestrator_id": "chief"}).Code)
	_, err := db.Exec(`CREATE TABLE office_channels AS
		SELECT id,workspace_id,agent_profile_id,platform,config,webhook_secret,status,task_id,created_at,updated_at
		FROM orchestration_conversations`)
	require.NoError(t, err)
	for range 2 {
		require.NoError(t, s.Repo.Migrate())
		owner, err := s.Repo.ConversationUserOwner(ctx, task)
		require.NoError(t, err)
		require.Equal(t, "owner", owner)
	}
}
