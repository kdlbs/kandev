package kubernetes

import (
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExecutorOwnershipKubernetesInventoryFailsClosedWithoutPolicy(t *testing.T) {
	repo := &fakeResourceRepository{executor: &models.Executor{ID: "private", Type: models.ExecutorTypeKubernetes, OwnerUserID: "alice"}}
	h := NewHandler(repo, &fakeAccessChecker{}, nil)
	_, err := h.executorForRead(t.Context(), "private")
	require.ErrorIs(t, err, models.ErrExecutorNotFound)
	repo.executor.OwnerUserID = ""
	_, err = h.executorForRead(t.Context(), "private")
	require.NoError(t, err)
}
