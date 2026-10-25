package executor

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type admissionSubmissionManager struct {
	*mockAgentManager
	submissionID string
}

func (m *admissionSubmissionManager) PromptAgentWithAdmissionCallbackAndSubmissionID(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	beforeAdmission func(context.Context, string, []v1.MessageAttachment) error,
	onDispatched func(),
	submissionID string,
) (*PromptResult, error) {
	m.submissionID = submissionID
	if beforeAdmission != nil {
		if err := beforeAdmission(ctx, prompt, attachments); err != nil {
			return nil, err
		}
	}
	if onDispatched != nil {
		onDispatched()
	}
	return &PromptResult{StopReason: "complete"}, nil
}

func TestPromptWithAdmissionCallbackAndSubmissionIDPreservesBothCapabilities(t *testing.T) {
	repo := newMockRepository()
	baseManager := &mockAgentManager{isPassthroughSessionFunc: func(context.Context, string) bool { return false }}
	manager := &admissionSubmissionManager{mockAgentManager: baseManager}
	seedPassthroughSession(t, repo, baseManager, "task-1", "session-1", "execution-1")
	exec := newTestExecutor(t, manager, repo)

	var admitted, dispatched bool
	result, err := exec.PromptWithAdmissionCallbackAndSubmissionID(
		context.Background(), "task-1", "session-1", "prompt", nil, true,
		func(context.Context, string, []v1.MessageAttachment) error {
			admitted = true
			return nil
		},
		func() { dispatched = true },
		"submission-42", nil,
	)
	if err != nil {
		t.Fatalf("PromptWithAdmissionCallbackAndSubmissionID: %v", err)
	}
	if !admitted || !dispatched {
		t.Fatalf("callback state: admitted=%v dispatched=%v", admitted, dispatched)
	}
	if manager.submissionID != "submission-42" {
		t.Fatalf("submission ID = %q, want submission-42", manager.submissionID)
	}
	if result == nil || result.StopReason != "complete" {
		t.Fatalf("prompt result = %#v, want completed result", result)
	}
}

func TestPromptWithAdmissionCallbackAndSubmissionIDFailsClosedWithoutCapability(t *testing.T) {
	repo := newMockRepository()
	manager := &mockAgentManager{isPassthroughSessionFunc: func(context.Context, string) bool { return false }}
	seedPassthroughSession(t, repo, manager, "task-1", "session-1", "execution-1")
	exec := newTestExecutor(t, manager, repo)

	var callbackCalled bool
	_, err := exec.PromptWithAdmissionCallbackAndSubmissionID(
		context.Background(), "task-1", "session-1", "prompt", nil, true,
		func(context.Context, string, []v1.MessageAttachment) error { callbackCalled = true; return nil },
		func() { callbackCalled = true },
		"submission-42", nil,
	)
	if !errors.Is(err, ErrPromptAdmissionCallbackUnsupported) {
		t.Fatalf("error = %v, want unsupported combined admission capability", err)
	}
	if callbackCalled || manager.promptAgentCallCount != 0 {
		t.Fatalf("unsupported path ran callbacks or prompt: callback=%v prompt_calls=%d", callbackCalled, manager.promptAgentCallCount)
	}
}

func TestPromptPassthroughRejectsBeforeInputAndAcceptance(t *testing.T) {
	for _, rejectAdmission := range []bool{true, false} {
		t.Run(map[bool]string{true: "admission", false: "running publication"}[rejectAdmission], func(t *testing.T) {
			repo := newMockRepository()
			manager := &mockAgentManager{isPassthroughSessionFunc: func(context.Context, string) bool { return true }}
			seedPassthroughSession(t, repo, manager, "task-1", "session-1", "execution-1")
			exec := newTestExecutor(t, manager, repo)
			rejection := errors.New("admission ownership rejected")
			var publications, accepted int
			result, err := exec.PromptWithAdmissionCallbackAndSubmissionID(
				context.Background(), "task-1", "session-1", "prompt", nil, false,
				func(context.Context, string, []v1.MessageAttachment) error {
					if rejectAdmission {
						return rejection
					}
					return nil
				},
				func() { accepted++ }, "", func() error { publications++; return rejection },
			)
			if !errors.Is(err, rejection) || result != nil {
				t.Fatalf("result=%v error=%v, want rejection", result, err)
			}
			if accepted != 0 || len(manager.writePassthroughStdinCalls) != 0 {
				t.Fatalf("rejected prompt accepted=%d writes=%d", accepted, len(manager.writePassthroughStdinCalls))
			}
			wantPublications := 1
			if rejectAdmission {
				wantPublications = 0
			}
			if publications != wantPublications {
				t.Fatalf("publications=%d, want %d", publications, wantPublications)
			}
		})
	}
}
