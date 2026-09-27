package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWrapCreatedSessionPrompt_CoordinatorStandingInstructions covers the
// Standing Instructions system block for a coordinator conversation's first
// prompt (docs/specs/coordinator/system-design/copilot.md#standing-instructions).
func TestWrapCreatedSessionPrompt_CoordinatorStandingInstructions(t *testing.T) {
	ctx := context.Background()
	session := &models.TaskSession{ID: "session"}

	t.Run("attaches the reader's content ahead of the prompt", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now()
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
			ID: "ws-1", Name: "Acme Workspace", CreatedAt: now, UpdatedAt: now,
		}))
		dbTask := &models.Task{
			ID: "task", WorkspaceID: "ws-1", Origin: models.TaskOriginCoordinator,
			Metadata: map[string]interface{}{models.MetaKeyCoordinatorID: "coord-1"},
		}

		var gotCoordinatorID, gotWorkspaceName, gotWorkspaceID string
		svc := &Service{repo: repo}
		svc.SetCoordinatorStandingInstructionsReader(
			func(_ context.Context, coordinatorID, workspaceName, workspaceID string) (string, error) {
				gotCoordinatorID, gotWorkspaceName, gotWorkspaceID = coordinatorID, workspaceName, workspaceID
				return "watch the release queue", nil
			},
		)

		got := svc.wrapCreatedSessionPrompt(
			ctx, "hello coordinator", "task", "session", session, dbTask,
			false, false, false, false, nil, "",
		)

		assert.Equal(t, "coord-1", gotCoordinatorID)
		assert.Equal(t, "Acme Workspace", gotWorkspaceName)
		assert.Equal(t, "ws-1", gotWorkspaceID)
		assert.Contains(t, got, "watch the release queue")
		assert.Contains(t, got, "hello coordinator")
	})

	t.Run("falls back to the bare prompt when the reader is unwired", func(t *testing.T) {
		dbTask := &models.Task{
			ID: "task", WorkspaceID: "ws-1", Origin: models.TaskOriginCoordinator,
			Metadata: map[string]interface{}{models.MetaKeyCoordinatorID: "coord-1"},
		}
		svc := &Service{}
		got := svc.wrapCreatedSessionPrompt(
			ctx, "hello coordinator", "task", "session", session, dbTask,
			false, false, false, false, nil, "",
		)
		assert.Equal(t, "hello coordinator", got)
	})

	t.Run("falls back to the bare prompt when the reader errors", func(t *testing.T) {
		dbTask := &models.Task{
			ID: "task", WorkspaceID: "ws-1", Origin: models.TaskOriginCoordinator,
			Metadata: map[string]interface{}{models.MetaKeyCoordinatorID: "coord-1"},
		}
		svc := &Service{}
		svc.SetCoordinatorStandingInstructionsReader(
			func(context.Context, string, string, string) (string, error) {
				return "", errors.New("boom")
			},
		)
		got := svc.wrapCreatedSessionPrompt(
			ctx, "hello coordinator", "task", "session", session, dbTask,
			false, false, false, false, nil, "",
		)
		assert.Equal(t, "hello coordinator", got)
	})

	t.Run("falls back to the bare prompt when the task carries no coordinator id", func(t *testing.T) {
		dbTask := &models.Task{ID: "task", WorkspaceID: "ws-1", Origin: models.TaskOriginCoordinator}
		svc := &Service{}
		svc.SetCoordinatorStandingInstructionsReader(
			func(context.Context, string, string, string) (string, error) {
				t.Fatal("reader should not be called without a coordinator id")
				return "", nil
			},
		)
		got := svc.wrapCreatedSessionPrompt(
			ctx, "hello coordinator", "task", "session", session, dbTask,
			false, false, false, false, nil, "",
		)
		assert.Equal(t, "hello coordinator", got)
	})
}
