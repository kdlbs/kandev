package backendapp

import (
	"context"
	"errors"
	"testing"

	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/dockerstore"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
)

func TestOverviewCacheDoesNotCommitCancelledStorageOverview(t *testing.T) {
	settings, _ := newStorageMaintenanceStores(t)
	workspaceStarted := make(chan struct{})
	overview := &storageOverview{
		settings:   settings,
		quarantine: failingQuarantineSummarizer{err: errors.New("quarantine unavailable")},
		workspaceAnalyze: func(ctx context.Context, _ storagepkg.StorageMaintenanceSettings) (workspaces.Analysis, error) {
			close(workspaceStarted)
			<-ctx.Done()
			return workspaces.Analysis{}, ctx.Err()
		},
		goCacheAnalyze: func(context.Context) (gocache.Analysis, error) {
			return gocache.Analysis{}, nil
		},
		docker: dockerstore.NewProvider(&overviewDockerClient{}, overviewContainerInventory{}, settings),
	}
	updates := make(chan storagepkg.StorageAnalysisUpdated, 16)
	cache := storagepkg.NewOverviewCacheWithOptions(overview, storagepkg.OverviewCacheOptions{
		Publisher: func(update storagepkg.StorageAnalysisUpdated) { updates <- update },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resultCh := make(chan error, 1)
	go func() {
		_, err := cache.Refresh(ctx)
		resultCh <- err
	}()

	<-workspaceStarted
	cancel()
	if err := <-resultCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh error = %v, want context cancellation", err)
	}

	read, err := cache.Read(context.Background())
	if err != nil {
		t.Fatalf("Read after cancellation: %v", err)
	}
	if read.Snapshot != nil {
		t.Fatalf("cancelled snapshot = %#v, want no committed snapshot", read.Snapshot)
	}
	if read.Analysis.State != storagepkg.AnalysisStateFailed {
		t.Fatalf("cancelled analysis state = %q, want failed", read.Analysis.State)
	}
	for {
		select {
		case update := <-updates:
			if update.State == storagepkg.AnalysisStateReady {
				t.Fatal("cancelled overview reported ready")
			}
		default:
			return
		}
	}
}
