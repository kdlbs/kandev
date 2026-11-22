package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type workspaceCloner interface {
	CloneWorkspace(context.Context, string, string) (*models.Workspace, error)
}

type recordingWorkspaceCloner struct {
	target *models.Workspace
	err    error
}

func (r *recordingWorkspaceCloner) CloneWorkspace(_ context.Context, _ *models.Workspace, target *models.Workspace) ([]*models.Workflow, error) {
	r.target = target
	return []*models.Workflow{{ID: "copied-workflow", WorkspaceID: target.ID, Name: "Kanban"}}, r.err
}

func TestWorkspaceCloneAuthorityAndPublication(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	source := &models.Workspace{Name: "Source", OwnerID: "creator"}
	require.NoError(t, repo.CreateWorkspace(t.Context(), source))
	copier := &recordingWorkspaceCloner{}
	svc.SetWorkspaceCloner(copier)
	_, err := svc.CloneWorkspace(ctxAs("stranger"), source.ID, "Copy")
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	require.Nil(t, copier.target)
	require.NoError(t, repo.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{WorkspaceID: source.ID, UserID: "viewer", Role: "viewer"}))
	_, err = svc.CloneWorkspace(ctxAs("viewer"), source.ID, "Copy")
	require.ErrorIs(t, err, ErrForbidden)
	require.Empty(t, eventBus.GetPublishedEvents())
	copier.err = errors.New("rollback")
	_, err = svc.CloneWorkspace(ctxAs("creator"), source.ID, "Copy")
	require.Error(t, err)
	require.Empty(t, eventBus.GetPublishedEvents())
	copier.err = nil
	target, err := svc.CloneWorkspace(ctxAs("creator"), source.ID, "Copy")
	require.NoError(t, err)
	require.Equal(t, "creator", target.OwnerID)
	require.NotEmpty(t, target.UnitID)
	published := eventBus.GetPublishedEvents()
	require.Len(t, published, 2)
	require.Equal(t, events.WorkspaceCreated, published[0].Type)
	require.Equal(t, events.WorkflowCreated, published[1].Type)
}

func TestWorkspaceCloneAdmission(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	source := &models.Workspace{Name: "Source", OfficeWorkflowID: "managed"}
	require.NoError(t, repo.CreateWorkspace(ctx, source))
	cloner, ok := any(svc).(workspaceCloner)
	require.True(t, ok)
	_, err := cloner.CloneWorkspace(ctx, source.ID, " ")
	require.Error(t, err)
	_, err = cloner.CloneWorkspace(ctx, source.ID, "Copy")
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceCloneConfiguration)
	_, err = cloner.CloneWorkspace(ctx, "missing", "Copy")
	require.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
}
