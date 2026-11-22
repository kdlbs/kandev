package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func continuationSuccessorDeliveryOwner() (*AgentExecution, *models.HarnessSessionGeneration) {
	execution, _ := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	execution.DeliveryHarnessGeneration = 2
	execution.ForceContextContinuation = true
	return execution, retentionBootstrapGeneration()
}

func TestContinuationSuccessorEventWaitsForGenerationCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	execution, predecessor := continuationSuccessorDeliveryOwner()
	ownerCtx, cancelOwner := context.WithCancel(ctx)
	defer cancelOwner()
	execution.bindSuccessorGenerationOwner(ownerCtx, &LaunchRequest{
		CandidateExecutionID:     execution.ID,
		ForceContextContinuation: true,
	})
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repo.generation.Store(predecessor)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	result := make(chan error, 1)
	go func() {
		_, event := retentionBootstrapOwner()
		event.DeliveryHarnessGeneration = 2
		_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
		result <- err
	}()
	select {
	case <-repo.firstRead:
	case <-ctx.Done():
		t.Fatal("continuation event did not read its predecessor generation")
	}
	select {
	case err := <-result:
		t.Fatalf("continuation event was rejected before its SQL commit: %v", err)
	default:
	}
	if repo.receipts.Load() != 0 {
		t.Fatal("continuation event was received before its SQL generation commit")
	}
	successor := *predecessor
	successor.Generation = 2
	successor.PredecessorGeneration = 1
	repo.generation.Store(&successor)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("committed continuation generation did not release the event")
	}
	if repo.receipts.Load() != 1 {
		t.Fatalf("received %d continuation events", repo.receipts.Load())
	}
}

func TestContinuationSuccessorEventFailsClosedWithoutCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	execution, predecessor := continuationSuccessorDeliveryOwner()
	ownerCtx, cancelOwner := context.WithCancel(t.Context())
	defer cancelOwner()
	execution.bindSuccessorGenerationOwner(ownerCtx, &LaunchRequest{
		CandidateExecutionID:     execution.ID,
		ForceContextContinuation: true,
	})
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repo.generation.Store(predecessor)
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	result := make(chan error, 1)
	go func() {
		_, event := retentionBootstrapOwner()
		event.DeliveryHarnessGeneration = 2
		_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
		result <- err
	}()
	select {
	case <-repo.firstRead:
	case <-ctx.Done():
		t.Fatal("continuation event did not read its predecessor generation")
	}
	cancelOwner()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("uncommitted continuation owner cancellation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("continuation wait outlived its launch owner")
	}
	if repo.receipts.Load() != 0 {
		t.Fatal("cancelled continuation event was received")
	}
}

func TestOrdinaryResumeEventStillRejectsForeignGeneration(t *testing.T) {
	execution, predecessor := continuationSuccessorDeliveryOwner()
	execution.ForceContextContinuation = false
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repo.generation.Store(predecessor)
	_, event := retentionBootstrapOwner()
	event.DeliveryHarnessGeneration = 2
	manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)

	_, err := manager.prepareAgentEvent(t.Context(), execution, nil, repo, event)
	if !errors.Is(err, ErrDurableAdoptionIdentityMismatch) {
		t.Fatalf("ordinary resume foreign generation: %v", err)
	}
	if repo.receipts.Load() != 0 {
		t.Fatal("ordinary resume received a foreign-generation event")
	}
}
