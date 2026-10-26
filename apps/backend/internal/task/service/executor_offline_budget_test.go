package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/offlinebudget"
	"github.com/kandev/kandev/internal/task/models"
)

func offlineBudgetExecutor(t *testing.T, svc *Service) *models.Executor {
	t.Helper()
	executor, err := svc.CreateExecutor(t.Context(), &CreateExecutorRequest{
		Name: "Budget executor", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	})
	require.NoError(t, err)
	return executor
}

func TestExecutorProfileOfflineBudgetValidatedOnCreateAndUpdate(t *testing.T) {
	svc, _, _ := createTestService(t)
	executor := offlineBudgetExecutor(t, svc)

	for _, bad := range []string{"0", "1441", "-5", "+5", "1.5", "1e2", "15m", "abc"} {
		_, err := svc.CreateExecutorProfile(t.Context(), &CreateExecutorProfileRequest{
			ExecutorID: executor.ID, Name: "bad " + bad, Config: map[string]string{offlinebudget.Key: bad},
		})
		require.ErrorIs(t, err, ErrInvalidExecutorConfig, bad)
		require.ErrorIs(t, err, offlinebudget.ErrInvalid, bad)
	}

	profile, err := svc.CreateExecutorProfile(t.Context(), &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "good", Config: map[string]string{offlinebudget.Key: " 30 "},
	})
	require.NoError(t, err)

	_, err = svc.UpdateExecutorProfile(t.Context(), profile.ID, &UpdateExecutorProfileRequest{
		Config: map[string]string{offlinebudget.Key: "9999"},
	})
	require.ErrorIs(t, err, offlinebudget.ErrInvalid)

	updated, err := svc.UpdateExecutorProfile(t.Context(), profile.ID, &UpdateExecutorProfileRequest{
		Config: map[string]string{offlinebudget.Key: "45"},
	})
	require.NoError(t, err)
	assert.Equal(t, "45", updated.Config[offlinebudget.Key])

	// A profile without the key keeps the default and is accepted.
	_, err = svc.CreateExecutorProfile(t.Context(), &CreateExecutorProfileRequest{
		ExecutorID: executor.ID, Name: "unset", Config: map[string]string{},
	})
	require.NoError(t, err)
}
