package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type restartInventoryRepository struct {
	*sqliterepo.Repository
	reads chan struct{}
}

func (r *restartInventoryRepository) ListSilentRestoreCandidates(context.Context, string, int) ([]models.SilentRestoreCandidate, error) {
	r.reads <- struct{}{}
	return nil, nil
}

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
func TestSilentRestoreRecoveryRescansOnTrigger(t *testing.T) {
	service, repo := newServiceWithRealRepo(t)
	service.repo = &restartInventoryRepository{Repository: repo, reads: make(chan struct{}, 3)}
	store := service.repo.(*restartInventoryRepository)
	service.SetStartupRecoveryOwnerContextResolver(func(ctx context.Context, _, _ string) (context.Context, error) { return ctx, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	t.Cleanup(service.stopSilentRestoreRecovery)
	service.startSilentRestoreRecovery(ctx)
	select {
	case <-store.reads:
	case <-ctx.Done():
		t.Fatal("startup inventory was not read")
	}
	service.startSilentRestoreRecovery(ctx)
	select {
	case <-store.reads:
	case <-ctx.Done():
		t.Fatal("runtime replacement did not rescan after startup")
	}
	service.stopSilentRestoreRecovery()
	service.startSilentRestoreRecovery(ctx)
	select {
	case <-store.reads:
		t.Fatal("stopped recovery worker restarted")
	default:
	}
	require.True(t, service.silentRestoreStopped)
}
