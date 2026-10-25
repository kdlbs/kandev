package lifecycle

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestStreamDisconnectDoesNotReplaceCompletedPromptOutcome(t *testing.T) {
	manager, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("completed-disconnect", "", "session-1")
	execution.Status = v1.AgentStatusReady
	execution.promptGeneration = 3
	execution.promptCompletionGeneration = 3
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatal(err)
	}

	manager.handleStreamDisconnect(execution, errors.New("use of closed network connection"), 3)

	if execution.Status != v1.AgentStatusReady {
		t.Fatalf("completed prompt status = %s, want ready", execution.Status)
	}
	for _, published := range eventBus.PublishedEvents {
		if published.Subject == events.AgentFailed || published.Subject == events.AgentCompleted {
			t.Fatalf("disconnect replaced completed prompt outcome: %s", published.Subject)
		}
	}
}
