package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDeleteTaskSessionArchivesTerminalTranscript(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-close", "source-close", "turn-close")
	require.NoError(t, repo.UpdateTaskSessionState(ctx, "source-close", models.TaskSessionStateCompleted, ""))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "primary-close", TaskID: "task-close", State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.SetSessionPrimary(ctx, "primary-close"))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: "message-close", TaskSessionID: "source-close", TaskID: "task-close", TurnID: "turn-close",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "retained result",
	}))
	source, err := repo.GetTaskSession(ctx, "source-close")
	require.NoError(t, err)
	_, err = repo.DeleteTaskSessionWithAttachments(ctx, source)
	require.NoError(t, err)

	archived, err := repo.GetTaskSession(ctx, "source-close")
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt)
	messages, err := repo.ListMessages(ctx, "source-close")
	require.NoError(t, err)
	require.Len(t, messages, 1)
	require.Equal(t, "retained result", messages[0].Content)
	receipt, err := repo.GetTaskSessionCleanupReceipt(ctx, "task-close", "source-close")
	require.NoError(t, err)
	require.Equal(t, "archived", receipt.Disposition)
	require.True(t, receipt.EvidenceRetained)
	require.Zero(t, receipt.QueueBeforeCount)
}

func TestDeleteTaskSessionRejectsPrimaryAndDeletesDisposableTerminalSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-disposable", Title: "Disposable"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "primary-disposable", TaskID: "task-disposable", State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "source-disposable", TaskID: "task-disposable", State: models.TaskSessionStateCompleted,
	}))
	require.NoError(t, repo.SetSessionPrimary(ctx, "primary-disposable"))
	primary, err := repo.GetTaskSession(ctx, "primary-disposable")
	require.NoError(t, err)
	_, err = repo.DeleteTaskSessionWithAttachments(ctx, primary)
	require.Error(t, err)
	source, err := repo.GetTaskSession(ctx, "source-disposable")
	require.NoError(t, err)
	_, err = repo.DeleteTaskSessionWithAttachments(ctx, source)
	require.NoError(t, err)
	_, err = repo.GetTaskSession(ctx, "source-disposable")
	require.Error(t, err)
	receipt, err := repo.GetTaskSessionCleanupReceipt(ctx, "task-disposable", "source-disposable")
	require.NoError(t, err)
	require.Equal(t, "deleted", receipt.Disposition)
	require.False(t, receipt.EvidenceRetained)
}

func TestDeleteTaskSessionRejectsPrimaryPromotionAfterPreflight(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-promoted", Title: "Promoted"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "old-primary", TaskID: "task-promoted", State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "promoted", TaskID: "task-promoted", State: models.TaskSessionStateCompleted,
	}))
	require.NoError(t, repo.SetSessionPrimary(ctx, "old-primary"))
	staleTarget, err := repo.GetTaskSession(ctx, "promoted")
	require.NoError(t, err)
	require.False(t, staleTarget.IsPrimary)
	require.NoError(t, repo.SetSessionPrimary(ctx, "promoted"))
	_, err = repo.DeleteTaskSessionWithAttachments(ctx, staleTarget)
	require.ErrorContains(t, err, "primary")
	current, err := repo.GetTaskSession(ctx, "promoted")
	require.NoError(t, err)
	require.True(t, current.IsPrimary)
}

func TestDeleteTaskSessionArchivesSubagentEvidenceWithoutTranscript(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-subagent", Title: "Subagent evidence"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "primary-subagent", TaskID: "task-subagent", State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "source-subagent", TaskID: "task-subagent", State: models.TaskSessionStateCompleted,
	}))
	require.NoError(t, repo.SetSessionPrimary(ctx, "primary-subagent"))
	require.NoError(t, repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "source-subagent", TaskID: "task-subagent", ToolCallID: "call-subagent", Source: "live",
	}))
	source, err := repo.GetTaskSession(ctx, "source-subagent")
	require.NoError(t, err)
	_, err = repo.DeleteTaskSessionWithAttachments(ctx, source)
	require.NoError(t, err)
	archived, err := repo.GetTaskSession(ctx, "source-subagent")
	require.NoError(t, err)
	require.NotNil(t, archived.ArchivedAt)
	contexts, err := repo.ListSubagentContextsBySession(ctx, "source-subagent")
	require.NoError(t, err)
	require.Len(t, contexts, 1)
}
