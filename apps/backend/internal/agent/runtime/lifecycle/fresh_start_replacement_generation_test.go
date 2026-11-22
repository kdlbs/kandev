package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func freshStartReplacementOwner() (*AgentExecution, *models.HarnessSessionGeneration) {
	execution, _ := retentionBootstrapOwner()
	execution.deliveryBootstrap.Store(false)
	execution.DeliveryHarnessGeneration = 2
	execution.ReplacesNativeConversation = true
	return execution, retentionBootstrapGeneration()
}

func TestFreshStartReplacementWaitsForCommittedSuccessor(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	execution, predecessor := freshStartReplacementOwner()
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
		t.Fatalf("replacement event rejected before its SQL commit: %v", err)
	default:
	}
	successor := *predecessor
	successor.Generation = 2
	successor.PredecessorGeneration = 1
	successor.NativeSessionID = "replacement-native"
	successor.CreationReason = "native_replaced"
	repo.generation.Store(&successor)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("committed replacement did not release its event")
	}
	if repo.receipts.Load() != 1 {
		t.Fatalf("received %d replacement events", repo.receipts.Load())
	}
}

func TestFreshStartReplacementKeepsUnmarkedAndForeignGenerationsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AgentExecution, *models.HarnessSessionGeneration)
	}{
		{"unmarked replacement", func(e *AgentExecution, _ *models.HarnessSessionGeneration) { e.ReplacesNativeConversation = false }},
		{"foreign incarnation", func(_ *AgentExecution, g *models.HarnessSessionGeneration) { g.IncarnationID = "foreign" }},
		{"advanced generation", func(_ *AgentExecution, g *models.HarnessSessionGeneration) { g.Generation = 3 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, g := freshStartReplacementOwner()
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
				t.Fatalf("unmarked or foreign owner: %v", err)
			}
			if repo.receipts.Load() != 0 {
				t.Fatal("unmarked or foreign owner received an event")
			}
		})
	}
}

func TestReplacementExecutionsRetirePredecessorSubmissions(t *testing.T) {
	require := func(ok bool, msg string) {
		t.Helper()
		if !ok {
			t.Fatal(msg)
		}
	}
	require(!(&AgentExecution{}).replacesPredecessorGeneration(), "process replacement must keep predecessor submissions")
	require((&AgentExecution{ForceContextContinuation: true}).replacesPredecessorGeneration(), "context continuation must retire predecessor submissions")
	require((&AgentExecution{ReplacesNativeConversation: true}).replacesPredecessorGeneration(), "fresh start must retire predecessor submissions")
}

func TestToAgentExecutionRecordsNativeConversationReplacement(t *testing.T) {
	execution := (&ExecutorInstance{InstanceID: "execution"}).ToAgentExecution(&ExecutorCreateRequest{
		ReplacesNativeConversation: true,
	})
	if !execution.ReplacesNativeConversation {
		t.Fatal("native conversation replacement was dropped from the execution")
	}
}

func TestACPSessionCreatedPayloadCarriesNativeConversationReplacement(t *testing.T) {
	execution := &AgentExecution{ID: "execution", SessionID: "session", DeliveryHarnessGeneration: 2, ReplacesNativeConversation: true}
	payload := newACPSessionCreatedPayload(execution, "replacement-native", "attempt")
	if !payload.DeliveryReplacesNativeConversation || payload.DeliveryHarnessGeneration != 2 {
		t.Fatalf("payload = %+v, want replacement generation 2", payload)
	}
}
