package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
)

// @covers AC-TASKS-PLAN-COMMENTS-002.2
func TestOrchestratorWrapperExposesAtomicPlanCommentQueue(t *testing.T) {
	wrapper := &orchestratorWrapper{svc: &orchestrator.Service{}}
	coordinator, ok := any(wrapper).(taskhandlers.AtomicQueuedPromptCoordinator)
	if !ok {
		t.Fatal("production message adapter omits atomic plan-comment queue coordination")
	}
	if got := coordinator.MaxQueuedPromptsPerSession(); got != 0 {
		t.Fatalf("unconfigured queue capacity = %d, want 0", got)
	}
}
