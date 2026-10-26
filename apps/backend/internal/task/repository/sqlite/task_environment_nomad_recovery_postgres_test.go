package sqlite

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPublishRecoveredNomadEnvironmentReady_PostgresRacesCheckpointNomadExecutorInventory
// runs publication and an inventory checkpoint concurrently on one
// environment and session under real PostgreSQL row locks.
func TestPublishRecoveredNomadEnvironmentReady_PostgresRacesCheckpointNomadExecutorInventory(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	fx := newPublishFixtureOn(t, repo)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checkpoint := nomadExecutorRunningFixture(publishExecutionID, publishGeneration, "ready")
	state, err := nomad.EnvironmentStateFromMetadata(checkpoint.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	state.AllocationID, state.AllocationCreateIndex, state.ServerIdentity = "allocation-publish", 7, "server-publish"
	if err := nomad.PutEnvironmentState(checkpoint.Metadata, state); err != nil {
		t.Fatal(err)
	}
	checkpoint.ExpectedNomadExecutorRevision = fx.revision

	var wg sync.WaitGroup
	var published bool
	var publishErr, checkpointErr error
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		published, publishErr = repo.PublishRecoveredNomadEnvironmentReady(ctx, fx.params())
	}()
	go func() {
		defer wg.Done()
		<-start
		checkpointErr = repo.CheckpointNomadExecutorInventory(ctx, checkpoint)
	}()
	close(start)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("publish and checkpoint did not finish: possible deadlock")
	}

	// The race has exactly two defined outcomes: the publication wins, or the
	// checkpoint rotated the execution first. Any other error fails the test.
	if !published && !errors.Is(publishErr, models.ErrExecutionRotated) {
		t.Fatalf("publish = (%v, %v), want published or models.ErrExecutionRotated", published, publishErr)
	}
	if checkpointErr != nil {
		t.Logf("checkpoint returned: %v", checkpointErr)
	}
	env := fx.environment(t)
	if published {
		fx.assertPublished(t)
	} else {
		fx.assertUnpublished(t)
	}
	if published && len(env.Repos) != 1 {
		t.Fatalf("repo rows = %d, want 1", len(env.Repos))
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, publishSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := nomad.EnvironmentStateFromMetadata(running.Metadata); err != nil || got.Phase != "ready" {
		t.Fatalf("final inventory = %+v, %v, want a consistent ready state", got, err)
	}
}
