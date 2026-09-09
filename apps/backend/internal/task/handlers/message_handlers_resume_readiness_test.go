package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// resumeRetryRepo extends sessionStateSequencer to also record every message
// the handler persists, so tests can assert whether a user-visible error
// message ("Request timed out...") was created.
type resumeRetryRepo struct {
	sessionStateSequencer
	createdMessages []*models.Message
}

func (r *resumeRetryRepo) CreateMessage(_ context.Context, message *models.Message) error {
	r.createdMessages = append(r.createdMessages, message)
	return nil
}

// resumeRetryOrchestrator is a minimal OrchestratorService fake that fails
// PromptTask's first call with a caller-supplied error and succeeds on every
// subsequent call, so tests can observe whether forwardMessageAsPrompt
// retries automatically after a recoverable failure. Setting retryPromptErr
// additionally fails the second (retry) call, so tests can assert that a
// failed retry still surfaces the original error rather than the retry's own.
type resumeRetryOrchestrator struct {
	promptErr      error
	retryPromptErr error
	promptCalls    int
	resumeCalls    int
	resumeErr      error
	callOrder      []string
}

func (o *resumeRetryOrchestrator) PromptTask(
	context.Context, string, string, string, string, bool, []v1.MessageAttachment, bool,
) (*orchestrator.PromptResult, error) {
	o.promptCalls++
	o.callOrder = append(o.callOrder, fmt.Sprintf("prompt:%d", o.promptCalls))
	if o.promptCalls == 1 && o.promptErr != nil {
		return nil, o.promptErr
	}
	if o.promptCalls == 2 && o.retryPromptErr != nil {
		return nil, o.retryPromptErr
	}
	return &orchestrator.PromptResult{}, nil
}

func (o *resumeRetryOrchestrator) ResumeTaskSession(context.Context, string, string) error {
	o.resumeCalls++
	o.callOrder = append(o.callOrder, "resume")
	return o.resumeErr
}

func (o *resumeRetryOrchestrator) StartCreatedSession(
	context.Context, string, string, string, string, bool, bool, bool, []v1.MessageAttachment, []v1.EntityReference,
) error {
	return nil
}

func (o *resumeRetryOrchestrator) ProcessOnTurnStart(context.Context, string, string) (orchestrator.ProcessOnTurnStartResult, error) {
	return orchestrator.ProcessOnTurnStartResult{}, nil
}

func (*resumeRetryOrchestrator) QueueUserPrompt(context.Context, string, string, string, string, bool, []v1.MessageAttachment, map[string]interface{}, bool) error {
	return nil
}

func (o *resumeRetryOrchestrator) StepRequiresCompletionSignal(context.Context, string) bool {
	return false
}

func (o *resumeRetryOrchestrator) ForegroundActivity(string) v1.ForegroundActivity {
	return ""
}

// These tests cover the resume/retry path for a session that is *not*
// generating, so steering never applies. False is the interface's conservative
// default and keeps the prompt-with-resume path under test.
func (*resumeRetryOrchestrator) SteerEligible(string, models.TaskSessionState) bool {
	return false
}

func (*resumeRetryOrchestrator) SteerTask(
	context.Context, string, string, string, string, bool, []v1.MessageAttachment,
) (*orchestrator.PromptResult, error) {
	return &orchestrator.PromptResult{}, nil
}

// newTestMessageHandlersWithOrchestrator mirrors newTestMessageHandlers but
// wires a real OrchestratorService fake, which forwardMessageAsPrompt/
// handlePromptWithResume require (they early-return when h.orchestrator is
// nil, which is how the shared helper is normally used).
func newTestMessageHandlersWithOrchestrator(t *testing.T, repo *resumeRetryRepo, orch OrchestratorService) *MessageHandlers {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return NewMessageHandlers(svc, orch, log)
}

// TestForwardMessageAsPrompt_RetriesOnceWhenAgentNotReadyAfterResume covers
// the case where ensureSessionRunning's post-resume readiness wait times out
// (orchestrator.ErrAgentNotReadyForPrompt, the exact error class behind the
// "agent not ready after resume: ... context deadline exceeded" log line).
// That failure is pre-dispatch: promptTask returns it before ever calling
// executor.PromptWithDispatchCallback, so no prompt has reached the agent
// yet and a retry cannot double-send.
//
// The backend already self-heals this condition (ensureSessionRunning reaps
// the stuck execution and a fresh resume typically succeeds in a few
// seconds), but forwardMessageAsPrompt only retried automatically for
// executor.ErrExecutionNotFound — this timeout class fell straight through
// to createPromptErrorMessage, surfacing "Request timed out. The agent may
// be processing a complex task. Please try again." to the user even though
// one more attempt would have silently succeeded.
func TestForwardMessageAsPrompt_RetriesOnceWhenAgentNotReadyAfterResume(t *testing.T) {
	readinessErr := fmt.Errorf("%w: %w", orchestrator.ErrAgentNotReadyForPrompt, context.DeadlineExceeded)
	resumeErr := fmt.Errorf("agent not ready after resume: %w", readinessErr)
	promptErr := fmt.Errorf("failed to ensure session is running: %w", resumeErr)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{promptErr: promptErr}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)
	h.waitForSessionReadyFn = func(context.Context, string) error { return nil }

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 2, orch.promptCalls, "PromptTask must be retried once after the readiness timeout")
	assert.Equal(t, 1, orch.resumeCalls, "the retry must go through ResumeTaskSession, reusing the existing reap/relaunch recovery")
	assert.Empty(t, repo.createdMessages, "a successful automatic retry must not surface a 'Request timed out' error message to the user")
}

// TestForwardMessageAsPrompt_SurfacesErrorWhenResumeRetryAlsoFails ensures
// the automatic retry does not swallow a genuine, non-recoverable failure:
// if ResumeTaskSession itself fails, the original readiness error must still
// reach the user via createPromptErrorMessage.
func TestForwardMessageAsPrompt_SurfacesErrorWhenResumeRetryAlsoFails(t *testing.T) {
	readinessErr := fmt.Errorf("%w: %w", orchestrator.ErrAgentNotReadyForPrompt, context.DeadlineExceeded)
	resumeErr := fmt.Errorf("agent not ready after resume: %w", readinessErr)
	promptErr := fmt.Errorf("failed to ensure session is running: %w", resumeErr)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{
		promptErr: promptErr,
		resumeErr: fmt.Errorf("resume: no executor record"),
	}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 1, orch.promptCalls, "PromptTask must not be retried when the resume itself fails")
	assert.Equal(t, 1, orch.resumeCalls)
	require.Len(t, repo.createdMessages, 1, "a genuinely unrecoverable failure must still surface an error message")
	assert.Contains(t, repo.createdMessages[0].Content, "Request timed out")
}

// TestForwardMessageAsPrompt_RetryGoesThroughResumeBeforeReprompting locks
// down call order: the retry must resume the session (reusing the existing
// reap/relaunch recovery in ResumeTaskSession) strictly between the failing
// first PromptTask call and the successful second one. Retrying PromptTask
// directly, without going through ResumeTaskSession first, would just
// reproduce the same "not ready" failure.
func TestForwardMessageAsPrompt_RetryGoesThroughResumeBeforeReprompting(t *testing.T) {
	readinessErr := fmt.Errorf("%w: %w", orchestrator.ErrAgentNotReadyForPrompt, context.DeadlineExceeded)
	resumeErr := fmt.Errorf("agent not ready after resume: %w", readinessErr)
	promptErr := fmt.Errorf("failed to ensure session is running: %w", resumeErr)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{promptErr: promptErr}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)
	h.waitForSessionReadyFn = func(context.Context, string) error { return nil }

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, []string{"prompt:1", "resume", "prompt:2"}, orch.callOrder)
}

// TestForwardMessageAsPrompt_DoesNotRetryGenericTimeout is the negative
// counterpart: a plain timeout error that does NOT wrap
// orchestrator.ErrAgentNotReadyForPrompt or executor.ErrExecutionNotFound
// (e.g. a provider/transport timeout that struck after the prompt may
// already have reached the agent) must not trigger the automatic retry.
// The fix is intentionally scoped to the two typed, provably pre-dispatch
// sentinels handlePromptWithResume already checks — broadening it to
// isTimeoutError's substring-based check would risk double-sending a
// prompt the agent already accepted.
func TestForwardMessageAsPrompt_DoesNotRetryGenericTimeout(t *testing.T) {
	promptErr := fmt.Errorf("prompt dispatch: %w", context.DeadlineExceeded)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{promptErr: promptErr}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 1, orch.promptCalls, "a generic timeout unrelated to the typed sentinels must not be retried")
	assert.Equal(t, 0, orch.resumeCalls)
	require.Len(t, repo.createdMessages, 1)
	assert.Contains(t, repo.createdMessages[0].Content, "Request timed out")
}

// TestForwardMessageAsPrompt_SurfacesOrigErrorWhenReadinessWaitFails ensures
// that when the post-resume readiness wait itself fails (e.g. the session
// transitions to FAILED while waiting), handlePromptWithResume surfaces the
// original pre-dispatch error rather than the internal readiness-wait error.
// The two errors classify differently for the user-facing message
// (createPromptErrorMessage), so returning the wrong one would silently
// downgrade the readiness timeout's "Request timed out..." UX to the
// generic "Failed to send message to agent".
func TestForwardMessageAsPrompt_SurfacesOrigErrorWhenReadinessWaitFails(t *testing.T) {
	readinessErr := fmt.Errorf("%w: %w", orchestrator.ErrAgentNotReadyForPrompt, context.DeadlineExceeded)
	resumeErr := fmt.Errorf("agent not ready after resume: %w", readinessErr)
	promptErr := fmt.Errorf("failed to ensure session is running: %w", resumeErr)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{promptErr: promptErr}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)
	h.waitForSessionReadyFn = func(context.Context, string) error {
		return errors.New("session failed after resume: session failed during resume")
	}

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 1, orch.promptCalls, "PromptTask must not be retried when the post-resume readiness wait itself fails")
	assert.Equal(t, 1, orch.resumeCalls)
	require.Len(t, repo.createdMessages, 1, "a failed readiness wait must still surface an error message")
	assert.Contains(t, repo.createdMessages[0].Content, "Request timed out",
		"the surfaced error must be the original readiness timeout, not the internal 'session failed after resume' wait error")
}

// TestForwardMessageAsPrompt_SurfacesRetryErrorWhenRetryPromptFails ensures
// that once the retry's second PromptTask call actually runs, its error is
// authoritative and reaches createPromptErrorMessage — not the original
// pre-dispatch timeout. The retry is a real dispatch attempt, so its error
// may be post-dispatch (e.g. wrap lifecycle.ErrAgentReported, see the next
// test) and must flow through the same isAgentReportedError classification
// as any other prompt failure; silently substituting origErr would mask
// that classification and risk a misleading duplicate message.
func TestForwardMessageAsPrompt_SurfacesRetryErrorWhenRetryPromptFails(t *testing.T) {
	readinessErr := fmt.Errorf("%w: %w", orchestrator.ErrAgentNotReadyForPrompt, context.DeadlineExceeded)
	resumeErr := fmt.Errorf("agent not ready after resume: %w", readinessErr)
	promptErr := fmt.Errorf("failed to ensure session is running: %w", resumeErr)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{
		promptErr:      promptErr,
		retryPromptErr: errors.New("dispatch: connection refused"),
	}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)
	h.waitForSessionReadyFn = func(context.Context, string) error { return nil }

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 2, orch.promptCalls, "the retry must still be attempted even though it goes on to fail")
	assert.Equal(t, 1, orch.resumeCalls)
	require.Len(t, repo.createdMessages, 1, "a failed retry must still surface an error message")
	assert.Contains(t, repo.createdMessages[0].Content, "Failed to send message to agent",
		"a generic retry-dispatch failure must surface its own error, not the unrelated original readiness timeout")
}

// TestForwardMessageAsPrompt_SuppressesMessageWhenRetryIsAgentReported covers
// carlosflorencio's review comment on PR #2250: when the retry's PromptTask
// call fails with an error wrapping lifecycle.ErrAgentReported, the agent's
// own failure path (handleAgentFailed) has already surfaced the error to the
// user via agent-status. forwardMessageAsPrompt's isAgentReportedError check
// must see that sentinel and skip createPromptErrorMessage — which only
// works if handlePromptWithResume returns the retry's real error instead of
// substituting the original (non-agent-reported) readiness timeout.
func TestForwardMessageAsPrompt_SuppressesMessageWhenRetryIsAgentReported(t *testing.T) {
	readinessErr := fmt.Errorf("%w: %w", orchestrator.ErrAgentNotReadyForPrompt, context.DeadlineExceeded)
	resumeErr := fmt.Errorf("agent not ready after resume: %w", readinessErr)
	promptErr := fmt.Errorf("failed to ensure session is running: %w", resumeErr)

	repo := &resumeRetryRepo{
		sessionStateSequencer: sessionStateSequencer{
			states: []models.TaskSessionState{models.TaskSessionStateWaitingForInput},
		},
	}
	orch := &resumeRetryOrchestrator{
		promptErr:      promptErr,
		retryPromptErr: fmt.Errorf("retry failed: %w", lifecycle.ErrAgentReported),
	}
	h := newTestMessageHandlersWithOrchestrator(t, repo, orch)
	h.waitForSessionReadyFn = func(context.Context, string) error { return nil }

	h.forwardMessageAsPrompt(
		context.Background(), "task-1", "session-1", "profile-1", "continue",
		"", false, nil, nil, false, "",
	)

	assert.Equal(t, 2, orch.promptCalls)
	assert.Equal(t, 1, orch.resumeCalls)
	assert.Empty(t, repo.createdMessages,
		"the retry's agent-reported error must be recognized and suppressed, not masked into a duplicate 'Request timed out' message")
}
