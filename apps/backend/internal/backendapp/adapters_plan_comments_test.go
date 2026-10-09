package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	taskhandlers "github.com/kandev/kandev/internal/task/handlers"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

type initialTaskBriefAdapterContract interface {
	orchestrator.DeliverySubmissionPromptStarter
	WithInitialTaskBriefAdmission(context.Context, string, func(context.Context) error) error
	MarkInitialTaskBriefDispatchPending(string)
	InitialTaskBriefDispatchPending(string) bool
	CompleteInitialTaskBriefDispatch(context.Context, string, string)
	PromptTaskWithPromptContext(
		context.Context, string, string, string, string, bool, []v1.MessageAttachment,
		string, bool, []v1.EntityReference, bool,
	) (*orchestrator.PromptResult, error)
	PromptTaskWithPromptContextAndDispatchOwnership(
		context.Context, string, string, string, string, bool, []v1.MessageAttachment,
		string, bool, []v1.EntityReference, bool, bool,
	) (*orchestrator.PromptResult, error)
	ResumeTaskSessionAndPromptWithPromptContext(
		context.Context, string, string, string, string, bool, []v1.MessageAttachment,
		string, bool, []v1.EntityReference, bool, ...string,
	) (*orchestrator.PromptResult, error)
}

func TestOrchestratorWrapperExposesInitialTaskBriefRecoveryContract(t *testing.T) {
	var wrapper any = &orchestratorWrapper{svc: &orchestrator.Service{}}
	if _, ok := wrapper.(initialTaskBriefAdapterContract); !ok {
		t.Fatal("production message adapter omits initial task brief dispatch or recovery context")
	}
	if _, ok := wrapper.(taskhandlers.AtomicQueuedPromptCoordinator); !ok {
		t.Fatal("production message adapter omits atomic queued prompt coordination")
	}
}
