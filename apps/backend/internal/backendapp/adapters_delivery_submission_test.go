package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
)

func TestLifecycleAdapterExposesInitialDeliverySubmissionBinding(t *testing.T) {
	var manager executor.AgentManagerClient = &lifecycleAdapter{}
	if _, ok := manager.(executor.InitialDeliverySubmissionIDSetter); !ok {
		t.Fatal("workspace-only agent start cannot bind its persisted first prompt")
	}
}
