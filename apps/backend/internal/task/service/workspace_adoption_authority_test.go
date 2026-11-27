package service

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceAdoptionServiceRefusesSyntheticAdministrator(t *testing.T) {
	svc := &Service{}
	ctx := ctxSynthetic()
	_, err := svc.PreviewWorkspaceAdoption(ctx, "task", WorkspaceAdoptionRequest{})
	require.ErrorIs(t, err, ErrForbidden)
	_, err = svc.ApplyWorkspaceAdoption(ctx, "task", models.WorkspaceAdoptionPreview{}, "operation")
	require.ErrorIs(t, err, ErrForbidden)
	_, err = svc.ValidateWorkspaceAdoption(ctx, "task", "operation")
	require.ErrorIs(t, err, ErrForbidden)
}
