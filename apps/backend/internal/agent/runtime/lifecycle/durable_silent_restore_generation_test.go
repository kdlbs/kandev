package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func silentRestoreDeliveryOwner() (*AgentExecution, *models.HarnessSessionGeneration) {
	execution, _ := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	execution.DeliveryHarnessGeneration = 2
	execution.RequiredNativeConversationID = "bootstrap-native"
	execution.InterruptedSubmissionID = "interrupted-submission"
	execution.InterruptedStreamID = "predecessor-stream"
	execution.InterruptedHarnessGeneration = 1
	return execution, retentionBootstrapGeneration()
}

func TestSilentRestoreDeliveryWaitsForCommittedSuccessor(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	execution, predecessor := silentRestoreDeliveryOwner()
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repo.generation.Store(predecessor)
	result := make(chan error, 1)
	go func() {
		_, event := retentionBootstrapOwner()
		event.DeliveryHarnessGeneration = 2
		manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
		_, err := manager.prepareAgentEvent(ctx, execution, nil, repo, event)
		result <- err
	}()
	<-repo.firstRead
	select {
	case err := <-result:
		t.Fatalf("successor event rejected before its SQL commit: %v", err)
	default:
	}
	if repo.receipts.Load() != 0 {
		t.Fatal("successor was received before its SQL generation commit")
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
		t.Fatal("committed successor did not release retained event")
	}
	if repo.receipts.Load() != 1 {
		t.Fatalf("received %d successor events", repo.receipts.Load())
	}
}

func TestSilentRestoreDeliveryKeepsUnknownAndForeignGenerationsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AgentExecution, *models.HarnessSessionGeneration)
	}{
		{"unmarked replacement", func(e *AgentExecution, _ *models.HarnessSessionGeneration) { e.RequiredNativeConversationID = "" }},
		{"missing interrupted submission", func(e *AgentExecution, _ *models.HarnessSessionGeneration) { e.InterruptedSubmissionID = "" }},
		{"missing predecessor stream", func(e *AgentExecution, _ *models.HarnessSessionGeneration) { e.InterruptedStreamID = "" }},
		{"wrong predecessor", func(e *AgentExecution, _ *models.HarnessSessionGeneration) { e.InterruptedHarnessGeneration = 4 }},
		{"different native conversation", func(_ *AgentExecution, g *models.HarnessSessionGeneration) { g.NativeSessionID = "foreign-native" }},
		{"advanced generation", func(_ *AgentExecution, g *models.HarnessSessionGeneration) { g.Generation = 3 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, g := silentRestoreDeliveryOwner()
			tc.mutate(e, g)
			repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
			repo.generation.Store(g)
			_, event := retentionBootstrapOwner()
			event.DeliveryHarnessGeneration = 2
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			manager := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			_, err := manager.prepareAgentEvent(ctx, e, nil, repo, event)
			if !errors.Is(err, ErrDurableAdoptionIdentityMismatch) {
				t.Fatalf("foreign or unknown owner: %v", err)
			}
			if repo.receipts.Load() != 0 {
				t.Fatal("foreign owner received an event")
			}
		})
	}
}

func TestSilentRestoreDeliveryCancellationKeepsSuccessorUnreceived(t *testing.T) {
	e, g := silentRestoreDeliveryOwner()
	repo := &retentionBootstrapRepository{firstRead: make(chan struct{})}
	repo.generation.Store(g)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := deliveryHarnessGeneration(ctx, e, repo, e.SessionID, e.DeliveryIncarnationID)
		result <- err
	}()
	<-repo.firstRead
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("uncommitted successor cancellation: %v", err)
	}
	if repo.receipts.Load() != 0 {
		t.Fatal("cancelled restore received an event")
	}
}
