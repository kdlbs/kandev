package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

func TestTurnChangeRetentionSweepUsesBoundedDefaultPolicy(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.FixedZone("local", 2*60*60))
	store := &retentionSweepRepository{}
	svc := NewService(Repos{TurnChanges: store}, nil, logger.Default(), RepositoryDiscoveryConfig{})

	svc.runTurnChangeRetentionSweep(context.Background(), now)

	require.Equal(t, models.TurnChangeRetentionPolicy{
		RetainFor:         30 * 24 * time.Hour,
		TaskBytes:         128 << 20,
		InstallationBytes: 1 << 30,
	}, store.policy)
	require.True(t, store.now.Equal(now.UTC()))
	require.Equal(t, 1, store.calls)
}

func TestTurnChangeRetentionSweepDoesNotStopOnRepositoryFailure(t *testing.T) {
	store := &retentionSweepRepository{err: errors.New("database unavailable")}
	svc := NewService(Repos{TurnChanges: store}, nil, logger.Default(), RepositoryDiscoveryConfig{})

	require.NotPanics(t, func() {
		svc.runTurnChangeRetentionSweep(context.Background(), time.Now())
	})
	require.Equal(t, 1, store.calls)
}

func TestTurnChangeRetentionSweepSkipsWhenStoreIsNotConfigured(t *testing.T) {
	svc := NewService(Repos{}, nil, logger.Default(), RepositoryDiscoveryConfig{})
	require.NotPanics(t, func() {
		svc.runTurnChangeRetentionSweep(context.Background(), time.Now())
	})
}

type retentionSweepRepository struct {
	repository.TurnChangesRepository
	policy models.TurnChangeRetentionPolicy
	now    time.Time
	calls  int
	err    error
}

func (r *retentionSweepRepository) ApplyTurnChangeRetention(_ context.Context, policy models.TurnChangeRetentionPolicy, now time.Time) (models.TurnChangeRetentionResult, error) {
	r.calls++
	r.policy = policy
	r.now = now
	return models.TurnChangeRetentionResult{}, r.err
}
