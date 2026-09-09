package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// mockGitLabMRAutomationService is a self-contained fake satisfying
// taskMRAgentAutomationService, purpose-built for this test file (mirrors
// the shape of mockGitHubService without depending on a real gitlab.Store).
type mockGitLabMRAutomationService struct {
	mu sync.Mutex

	options *gitlab.TaskMRAutomationResponse
	// optionsByMRIID, when set, returns per-MR evaluation options keyed by MR
	// IID — the shape the real service produces now that the five switches
	// are per linked MR. See optionsForMR.
	optionsByMRIID    map[int]*gitlab.TaskMRAutomationResponse
	optionsErr        error
	checkpoint        *gitlab.TaskMRLifecycleState
	checkpointErr     error
	reviewerUsername  string
	rebindChanged     bool
	rebindErr         error
	reviewRequested   bool
	reviewRequestErr  error
	recordErr         error
	observedStateErr  error
	reviewStateErr    error
	recordedErrorMsgs []string
	responseStarted   chan struct{}
	responseBlocks    bool
	recordPromptStart chan struct{}
	recordPromptBlock bool
	recordErrorStart  chan struct{}
	recordErrorBlock  bool

	prompts []gitlab.TaskMRLifecyclePrompt

	taskMRs    []*gitlab.TaskMR
	taskMRsErr error

	evaluationCalls atomic.Int32
	rebindCalls     atomic.Int32
	reviewerCalls   atomic.Int32

	// checkpointCalls, when non-nil, receives a value on every
	// GetTaskMRLifecycleState call — an observable, timing-independent
	// signal that a detached evaluation goroutine actually reached the eval
	// path, since the goroutine's completion speed is not something a test
	// can otherwise wait on deterministically.
	checkpointCalls chan struct{}

	// Auto-fix / auto-merge (this task).
	snapshot    *gitlab.MRAutomationSnapshot
	snapshotErr error

	mergedMR   *gitlab.MR
	mergeErr   error
	mergeCalls atomic.Int32

	fixAttempts   []gitlab.TaskMRFixAttempt
	fixAttemptErr error

	refreshedCheckpoints []refreshedMRFixCheckpoint
	refreshErr           error

	exhaustedCalls []string
	exhaustedErr   error

	mergeAttempts   []gitlab.TaskMRMergeAttempt
	mergeAttemptErr error
}

type refreshedMRFixCheckpoint struct {
	signature      string
	checkpointJSON string
}

func (m *mockGitLabMRAutomationService) GetTaskMRAutomationResponse(ctx context.Context, _ string) (*gitlab.TaskMRAutomationResponse, error) {
	if m.responseBlocks {
		return nil, waitForGitLabAutomationContext(ctx, m.responseStarted)
	}
	if m.optionsErr != nil {
		return nil, m.optionsErr
	}
	if m.options == nil {
		return &gitlab.TaskMRAutomationResponse{}, nil
	}
	return m.options, nil
}

func (m *mockGitLabMRAutomationService) GetTaskMRAutomationEvaluation(
	ctx context.Context, _ string, _ string, _ string, mrIID int,
) (*gitlab.TaskMRAutomationEvaluation, error) {
	m.evaluationCalls.Add(1)
	if m.optionsErr != nil {
		return nil, m.optionsErr
	}
	if m.checkpointCalls != nil {
		select {
		case m.checkpointCalls <- struct{}{}:
		default:
		}
	}
	if m.checkpointErr != nil {
		return nil, m.checkpointErr
	}
	options := m.optionsForMR(mrIID)
	return &gitlab.TaskMRAutomationEvaluation{Options: options, Checkpoint: m.checkpoint}, nil
}

// optionsForMR models the real service's per-MR resolution: the switches live
// per linked MR, so the evaluation snapshot for one MR carries that MR's own
// values. optionsByMRIID left nil keeps the single-options behaviour every
// other test in this package relies on.
func (m *mockGitLabMRAutomationService) optionsForMR(mrIID int) *gitlab.TaskMRAutomationResponse {
	if m.optionsByMRIID != nil {
		if options, ok := m.optionsByMRIID[mrIID]; ok {
			return options
		}
		return &gitlab.TaskMRAutomationResponse{}
	}
	if m.options == nil {
		return &gitlab.TaskMRAutomationResponse{}
	}
	return m.options
}

func (m *mockGitLabMRAutomationService) GetTaskMRLifecycleState(context.Context, string, string, string, int) (*gitlab.TaskMRLifecycleState, error) {
	if m.checkpointCalls != nil {
		select {
		case m.checkpointCalls <- struct{}{}:
		default:
		}
	}
	if m.checkpointErr != nil {
		return nil, m.checkpointErr
	}
	return m.checkpoint, nil
}

func (m *mockGitLabMRAutomationService) RebindTaskMRReviewer(context.Context, string) (string, bool, error) {
	m.rebindCalls.Add(1)
	if m.rebindErr != nil {
		return "", false, m.rebindErr
	}
	return m.reviewerUsername, m.rebindChanged, nil
}

func (m *mockGitLabMRAutomationService) IsReviewerOnMR(context.Context, string, string, int, string) (bool, error) {
	m.reviewerCalls.Add(1)
	if m.reviewRequestErr != nil {
		return false, m.reviewRequestErr
	}
	return m.reviewRequested, nil
}

func (m *mockGitLabMRAutomationService) SetTaskMRReviewRequestState(context.Context, string, string, string, int, bool) error {
	return m.reviewStateErr
}

func (m *mockGitLabMRAutomationService) SetTaskMRObservedState(context.Context, string, string, string, int, string) error {
	return m.observedStateErr
}

func (m *mockGitLabMRAutomationService) RecordTaskMRLifecyclePrompt(ctx context.Context, prompt gitlab.TaskMRLifecyclePrompt) error {
	if m.recordPromptBlock {
		return waitForGitLabAutomationContext(ctx, m.recordPromptStart)
	}
	if m.recordErr != nil {
		return m.recordErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prompts = append(m.prompts, prompt)
	return nil
}

func (m *mockGitLabMRAutomationService) RecordTaskMRAutomationError(ctx context.Context, _, _, _ string, _ int, message string) error {
	if m.recordErrorBlock {
		return waitForGitLabAutomationContext(ctx, m.recordErrorStart)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordedErrorMsgs = append(m.recordedErrorMsgs, message)
	return nil
}

func (m *mockGitLabMRAutomationService) promptSnapshot() []gitlab.TaskMRLifecyclePrompt {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]gitlab.TaskMRLifecyclePrompt(nil), m.prompts...)
}

func (m *mockGitLabMRAutomationService) recordedErrors() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.recordedErrorMsgs...)
}

func waitForGitLabAutomationContext(ctx context.Context, started chan struct{}) error {
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return ctx.Err()
}

func (m *mockGitLabMRAutomationService) ListTaskMRsByTask(context.Context, string) ([]*gitlab.TaskMR, error) {
	if m.taskMRsErr != nil {
		return nil, m.taskMRsErr
	}
	return m.taskMRs, nil
}

func (m *mockGitLabMRAutomationService) GetMRAutomationSnapshot(context.Context, string, string, string, int) (*gitlab.MRAutomationSnapshot, error) {
	if m.snapshotErr != nil {
		return nil, m.snapshotErr
	}
	return m.snapshot, nil
}

func (m *mockGitLabMRAutomationService) MergeMRForAutomation(context.Context, string, string, string, int) (*gitlab.MR, error) {
	m.mergeCalls.Add(1)
	if m.mergeErr != nil {
		return nil, m.mergeErr
	}
	return m.mergedMR, nil
}

func (m *mockGitLabMRAutomationService) RecordTaskMRFixAttempt(_ context.Context, attempt gitlab.TaskMRFixAttempt) error {
	if m.fixAttemptErr != nil {
		return m.fixAttemptErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fixAttempts = append(m.fixAttempts, attempt)
	return nil
}

func (m *mockGitLabMRAutomationService) RefreshTaskMRFixCheckpoint(_ context.Context, _, _, _ string, _ int, signature, checkpointJSON string) error {
	if m.refreshErr != nil {
		return m.refreshErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refreshedCheckpoints = append(m.refreshedCheckpoints, refreshedMRFixCheckpoint{signature: signature, checkpointJSON: checkpointJSON})
	return nil
}

func (m *mockGitLabMRAutomationService) MarkTaskMRAutoFixExhausted(_ context.Context, taskID, _, _ string, _ int, _ string) error {
	if m.exhaustedErr != nil {
		return m.exhaustedErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exhaustedCalls = append(m.exhaustedCalls, taskID)
	return nil
}

func (m *mockGitLabMRAutomationService) RecordTaskMRMergeAttempt(_ context.Context, attempt gitlab.TaskMRMergeAttempt) error {
	if m.mergeAttemptErr != nil {
		return m.mergeAttemptErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mergeAttempts = append(m.mergeAttempts, attempt)
	return nil
}

func (m *mockGitLabMRAutomationService) ClearTaskMRAutomationError(context.Context, string, string, string, int) error {
	return nil
}

// --- AC10-AC16, AC35: pure decision function ---

func TestDecideTaskMRAgentPrompt(t *testing.T) {
	trueValue := true
	falseValue := false

	tests := []struct {
		name            string
		mrState         string
		options         *gitlab.TaskMRAutomationResponse
		checkpoint      *gitlab.TaskMRLifecycleState
		reviewRequested *bool
		wantEvent       string
		wantReviewStamp *bool
		wantStateStamp  string
	}{
		{
			// AC10: no prior checkpoint, reviewer present -> silent baseline.
			name:            "initial review request establishes silent baseline",
			mrState:         gitlabMRStateOpen,
			options:         &gitlab.TaskMRAutomationResponse{PromptOnReviewRequested: true},
			checkpoint:      nil,
			reviewRequested: &trueValue,
			wantReviewStamp: &trueValue,
			wantStateStamp:  gitlabMRStateOpen,
		},
		{
			// AC11: false -> true edge fires exactly once.
			name:    "false to true review edge fires once",
			mrState: gitlabMRStateOpen,
			options: &gitlab.TaskMRAutomationResponse{PromptOnReviewRequested: true},
			checkpoint: &gitlab.TaskMRLifecycleState{
				ReviewRequestInitialized: true, LastReviewRequested: false, LastObservedState: gitlabMRStateOpen,
			},
			reviewRequested: &trueValue,
			wantEvent:       mrAgentEventReviewRequested,
			wantReviewStamp: &trueValue,
		},
		{
			// AC11 second half: unchanged reviewers fire nothing.
			name:    "unchanged true review fires nothing",
			mrState: gitlabMRStateOpen,
			options: &gitlab.TaskMRAutomationResponse{PromptOnReviewRequested: true},
			checkpoint: &gitlab.TaskMRLifecycleState{
				ReviewRequestInitialized: true, LastReviewRequested: true, LastObservedState: gitlabMRStateOpen,
			},
			reviewRequested: &trueValue,
		},
		{
			// AC12: true -> false re-arms without prompting.
			name:    "true to false rearms without prompting",
			mrState: gitlabMRStateOpen,
			options: &gitlab.TaskMRAutomationResponse{PromptOnReviewRequested: true},
			checkpoint: &gitlab.TaskMRLifecycleState{
				ReviewRequestInitialized: true, LastReviewRequested: true, LastObservedState: gitlabMRStateOpen,
			},
			reviewRequested: &falseValue,
			wantReviewStamp: &falseValue,
		},
		{
			// AC13: open -> merged with the switch on fires merged once.
			name:           "open to merged fires merged",
			mrState:        gitlabMRStateMerged,
			options:        &gitlab.TaskMRAutomationResponse{PromptOnMerged: true},
			checkpoint:     &gitlab.TaskMRLifecycleState{LastObservedState: gitlabMRStateOpen},
			wantEvent:      mrAgentEventMerged,
			wantStateStamp: gitlabMRStateMerged,
		},
		{
			// AC13: re-polling the merged MR fires nothing.
			name:       "repolling merged fires nothing",
			mrState:    gitlabMRStateMerged,
			options:    &gitlab.TaskMRAutomationResponse{PromptOnMerged: true},
			checkpoint: &gitlab.TaskMRLifecycleState{LastObservedState: gitlabMRStateMerged},
		},
		{
			// AC14: open -> closed fires closed once.
			name:           "open to closed fires closed",
			mrState:        gitlabMRStateClosed,
			options:        &gitlab.TaskMRAutomationResponse{PromptOnClosed: true},
			checkpoint:     &gitlab.TaskMRLifecycleState{LastObservedState: gitlabMRStateOpen},
			wantEvent:      mrAgentEventClosed,
			wantStateStamp: gitlabMRStateClosed,
		},
		{
			// AC14: after a reopen (checkpoint now shows "open" again), a
			// second close transition fires closed again.
			name:           "reopen then close fires closed again",
			mrState:        gitlabMRStateClosed,
			options:        &gitlab.TaskMRAutomationResponse{PromptOnClosed: true},
			checkpoint:     &gitlab.TaskMRLifecycleState{LastObservedState: gitlabMRStateOpen, LastLifecycleEvent: mrAgentEventClosed},
			wantEvent:      mrAgentEventClosed,
			wantStateStamp: gitlabMRStateClosed,
		},
		{
			// AC15: locked is non-terminal — no merged/closed event even
			// though the switches are on, since it's not a state change to
			// review-request scope.
			name:           "locked state is non-terminal and silent",
			mrState:        gitlabMRStateLocked,
			options:        &gitlab.TaskMRAutomationResponse{PromptOnMerged: true, PromptOnClosed: true},
			checkpoint:     &gitlab.TaskMRLifecycleState{LastObservedState: gitlabMRStateOpen},
			wantStateStamp: gitlabMRStateLocked,
		},
		{
			// AC16 (decision-level slice): switches off produce no event
			// even with a reviewer present. ObservedState is still stamped —
			// baseline state tracking is independent of the switches; the
			// higher-level AC16 short-circuit (no checkpoint write at all)
			// lives in handleTaskMRLifecycleAutomation, covered separately.
			name:            "no switches enabled fires nothing",
			mrState:         gitlabMRStateOpen,
			options:         &gitlab.TaskMRAutomationResponse{},
			checkpoint:      &gitlab.TaskMRLifecycleState{ReviewRequestInitialized: true, LastReviewRequested: false},
			reviewRequested: &trueValue,
			wantStateStamp:  gitlabMRStateOpen,
		},
		{
			// options == nil is defensive (should not occur in practice).
			name:    "nil options is a no-op",
			mrState: gitlabMRStateOpen,
			options: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := decideTaskMRAgentPrompt(tt.mrState, tt.options, tt.checkpoint, tt.reviewRequested)
			if decision.Event != tt.wantEvent {
				t.Errorf("Event = %q, want %q", decision.Event, tt.wantEvent)
			}
			if (decision.ReviewRequested == nil) != (tt.wantReviewStamp == nil) {
				t.Errorf("ReviewRequested = %v, want %v", decision.ReviewRequested, tt.wantReviewStamp)
			} else if decision.ReviewRequested != nil && *decision.ReviewRequested != *tt.wantReviewStamp {
				t.Errorf("ReviewRequested = %v, want %v", *decision.ReviewRequested, *tt.wantReviewStamp)
			}
			if decision.ObservedState != tt.wantStateStamp {
				t.Errorf("ObservedState = %q, want %q", decision.ObservedState, tt.wantStateStamp)
			}
		})
	}
}

// TestGitLabMRLifecycleConstants_ShareStateConstantValues pins the AC35
// design: the lifecycle event identifiers are separate named constants that
// currently hold the same strings as the state constants. The separation
// itself is enforced by the two const blocks in
// event_handlers_gitlab_mr_automation.go, not by this assertion.
func TestGitLabMRLifecycleConstants_ShareStateConstantValues(t *testing.T) {
	if mrAgentEventMerged != gitlabMRStateMerged {
		t.Fatalf("expected mrAgentEventMerged and gitlabMRStateMerged to share the same string value by design")
	}
	// The real assertion is a compile-time one: mrAgentEventMerged and
	// gitlabMRStateMerged are declared in two separate const blocks (see
	// event_handlers_gitlab_mr_automation.go) specifically so a future
	// rename of one cannot silently change the other's call sites.
}

// --- AC17/AC18: prompt text + canonical URL ---

func TestTaskMRAgentLifecyclePrompt(t *testing.T) {
	mr := &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: 7}
	tests := []struct {
		event string
		want  string
	}{
		{mrAgentEventReviewRequested, "Your review was requested on https://gitlab.example.com/group/project/-/merge_requests/7."},
		{mrAgentEventMerged, "The linked merge request https://gitlab.example.com/group/project/-/merge_requests/7 was merged."},
		{mrAgentEventClosed, "The linked merge request https://gitlab.example.com/group/project/-/merge_requests/7 was closed without merging."},
	}
	for _, tt := range tests {
		got, err := taskMRAgentLifecyclePrompt(tt.event, mr)
		if err != nil {
			t.Fatalf("taskMRAgentLifecyclePrompt(%s): %v", tt.event, err)
		}
		if got != tt.want {
			t.Fatalf("taskMRAgentLifecyclePrompt(%s) = %q, want %q", tt.event, got, tt.want)
		}
	}
}

func TestCanonicalTaskMRURL(t *testing.T) {
	tests := []struct {
		name    string
		mr      *gitlab.TaskMR
		wantURL string
		wantErr bool
	}{
		{name: "nil MR is rejected", mr: nil, wantErr: true},
		{
			name:    "valid host and project path",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: 7},
			wantURL: "https://gitlab.example.com/group/project/-/merge_requests/7",
		},
		{
			name:    "nested subgroup project path",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/subgroup/project", MRIID: 3},
			wantURL: "https://gitlab.example.com/group/subgroup/project/-/merge_requests/3",
		},
		{
			name:    "mr_iid zero is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: 0},
			wantErr: true,
		},
		{
			name:    "negative mr_iid is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: -1},
			wantErr: true,
		},
		{
			name:    "empty project path is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "", MRIID: 1},
			wantErr: true,
		},
		{
			name:    "single-segment project path is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group", MRIID: 1},
			wantErr: true,
		},
		{
			name:    "path traversal in project path is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/../secret", MRIID: 1},
			wantErr: true,
		},
		{
			name:    "injected markup in project path is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com", ProjectPath: "group/proj\"><script>", MRIID: 1},
			wantErr: true,
		},
		{
			name:    "host with a path component is rejected",
			mr:      &gitlab.TaskMR{Host: "https://gitlab.example.com/extra", ProjectPath: "group/project", MRIID: 1},
			wantErr: true,
		},
		{
			name:    "host without scheme is rejected",
			mr:      &gitlab.TaskMR{Host: "gitlab.example.com", ProjectPath: "group/project", MRIID: 1},
			wantErr: true,
		},
		{
			name:    "non-HTTP(S) scheme is rejected",
			mr:      &gitlab.TaskMR{Host: "javascript://gitlab.example.com", ProjectPath: "group/project", MRIID: 1},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalTaskMRURL(tt.mr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("canonicalTaskMRURL(%+v) = %q, want error", tt.mr, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("canonicalTaskMRURL(%+v): %v", tt.mr, err)
			}
			if got != tt.wantURL {
				t.Fatalf("canonicalTaskMRURL(%+v) = %q, want %q", tt.mr, got, tt.wantURL)
			}
		})
	}
}

// --- AC19-AC21: dispatch + badge metadata + archive refusal ---

func TestDispatchTaskMRAgentPrompt_TagsPromptForChatBadge(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: 42}

	if _, err := svc.dispatchTaskMRAgentPrompt(ctx, mr, "the mr merged", mrAgentEventMerged); err != nil {
		t.Fatalf("dispatch lifecycle prompt: %v", err)
	}

	status := svc.messageQueue.GetStatus(ctx, "session-1")
	if status.Count != 1 {
		t.Fatalf("pending lifecycle entries = %d, want 1", status.Count)
	}
	metadata := status.Entries[0].Metadata
	if got, _ := metadata["workflow_message"].(bool); !got {
		t.Errorf("workflow_message = %v, want true", metadata["workflow_message"])
	}
	if got := metadata["workflow_step_name"]; got != taskMRAgentBadgeLabel {
		t.Errorf("workflow_step_name = %v, want %q", got, taskMRAgentBadgeLabel)
	}
	if got := metadata[mrAutomationMetaOrigin]; got != mrAutomationOrigin {
		t.Errorf("origin = %v, want %q", got, mrAutomationOrigin)
	}
	if got := metadata[mrAutomationMetaEvent]; got != mrAgentEventMerged {
		t.Errorf("event = %v, want %q", got, mrAgentEventMerged)
	}
	if got := status.Entries[0].Metadata[messagequeue.MetadataCoalesceKey]; got != "gitlab-mr:repo-1:group/project:42:merged" {
		t.Fatalf("coalesce key = %v, want gitlab-mr:repo-1:group/project:42:merged", got)
	}
}

// TestDispatchTaskMRAgentPrompt_ReadySessionDrainsAndAcknowledges is the AC19
// / AC20 delivery-path regression: it exercises the exact durable-lifecycle
// acknowledge cycle used by dispatchTaskPRAgentPrompt's own passing test
// (TestDispatchTaskPRAgentPrompt_ReadySessionsDrainImmediately) end to end
// for a GitLab-originated message. Before the isLifecycleAutomationOrigin
// fix in event_handlers_agent.go, this hung forever: executeQueuedMessage's
// lifecycle-prompt detection was hardcoded to the GitHub PR automation
// origin string, so a GitLab MR automation entry was never acknowledged.
func TestDispatchTaskMRAgentPrompt_ReadySessionDrainsAndAcknowledges(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session-1", "task-1", "execution-1")
	agent := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo, promptDone: make(chan struct{})}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), agent)
	acknowledgingRepo := &lifecycleAcknowledgingRepository{
		Repository:   messagequeue.NewMemoryRepository(),
		acknowledged: make(chan struct{}),
	}
	svc.messageQueue = messagequeue.NewService(acknowledgingRepo, messagequeue.DefaultMaxPerSession, testLogger())
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: 42}

	if _, err := svc.dispatchTaskMRAgentPrompt(ctx, mr, "the mr merged", mrAgentEventMerged); err != nil {
		t.Fatalf("dispatch lifecycle prompt: %v", err)
	}
	select {
	case <-agent.promptDone:
	case <-time.After(time.Second):
		t.Fatal("ready lifecycle queue did not drain to the agent")
	}
	select {
	case <-acknowledgingRepo.acknowledged:
	case <-time.After(time.Second):
		t.Fatal("ready lifecycle queue did not acknowledge the accepted prompt")
	}
	if got := svc.messageQueue.GetStatus(ctx, "session-1").Count; got != 0 {
		t.Fatalf("pending lifecycle entries after immediate drain = %d, want 0", got)
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if len(agent.capturedPrompts) != 1 || agent.capturedPrompts[0] != "the mr merged" {
		t.Fatalf("drained prompts = %+v, want lifecycle prompt", agent.capturedPrompts)
	}
}

func TestEvalTaskMRLifecycle_ArchivedTaskRefusesAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	require.NoError(t, repo.ArchiveTask(ctx, "task-1"))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	automation := &mockGitLabMRAutomationService{options: &gitlab.TaskMRAutomationResponse{PromptOnMerged: true}}
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.example.com", ProjectPath: "group/project", MRIID: 42, State: gitlabMRStateMerged}

	delivered, err := svc.evalTaskMRLifecycle(ctx, mr, automation.options, automation)
	if err == nil || delivered {
		t.Fatalf("archived task lifecycle eval = delivered %v, err %v; want refusal", delivered, err)
	}
	if got := automation.promptSnapshot(); len(got) != 0 {
		t.Fatalf("checkpoint recorded for an archived task: %+v", got)
	}
}

func TestEvalTaskMRLifecycle_UsesPollReviewerObservationWithoutProviderReads(t *testing.T) {
	automation := &mockGitLabMRAutomationService{
		options: &gitlab.TaskMRAutomationResponse{
			PromptOnReviewRequested: true,
			ReviewReviewerUsername:  "alice",
		},
	}
	mr := &gitlab.TaskMR{
		TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.example.com",
		ProjectPath: "group/project", MRIID: 42, State: gitlabMRStateOpen,
	}

	_, err := (&Service{}).evalTaskMRLifecycleAtCheckpoint(
		context.Background(), mr, automation.options, nil,
		&taskMRReviewerObservation{reviewers: []gitlab.MRReviewer{{Username: "alice"}}},
		automation,
	)
	if err != nil {
		t.Fatalf("evaluation with poll observation: %v", err)
	}
	if got := automation.reviewerCalls.Load(); got != 0 {
		t.Fatalf("provider reviewer reads = %d, want 0 when poll observation is present", got)
	}
	if got := automation.rebindCalls.Load(); got != 0 {
		t.Fatalf("authenticated-user/rebind calls = %d, want 0 during evaluation", got)
	}
}

func TestEvalTaskMRLifecycle_AbsentPollObservationUsesStrictFallback(t *testing.T) {
	automation := &mockGitLabMRAutomationService{
		options: &gitlab.TaskMRAutomationResponse{
			PromptOnReviewRequested: true,
			ReviewReviewerUsername:  "alice",
		},
	}
	mr := &gitlab.TaskMR{
		TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.example.com",
		ProjectPath: "group/project", MRIID: 42, State: gitlabMRStateOpen,
	}

	_, err := (&Service{}).evalTaskMRLifecycleAtCheckpoint(
		context.Background(), mr, automation.options, nil, nil, automation,
	)
	if err != nil {
		t.Fatalf("evaluation without poll observation: %v", err)
	}
	if got := automation.reviewerCalls.Load(); got != 1 {
		t.Fatalf("provider reviewer reads = %d, want 1 for absent-observation fallback", got)
	}
}

// --- AC31: fail-closed on transient errors ---

func TestHandleTaskMRLifecycleAutomation_TransientTaskLookupErrorIsReturned(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.repo = &failingGetTaskRepo{repoStore: repo, err: errors.New("db unavailable")}
	svc.gitlabMRAutomation = &mockGitLabMRAutomationService{options: &gitlab.TaskMRAutomationResponse{PromptOnMerged: true}}
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", MRIID: 1, State: gitlabMRStateMerged}

	err := svc.handleTaskMRLifecycleAutomation(ctx, mr)
	if err == nil {
		t.Fatal("expected a transient task-lookup error to be returned rather than swallowed")
	}
}

func TestHandleTaskMRLifecycleAutomation_TransientOptionsLoadErrorIsReturned(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.gitlabMRAutomation = &mockGitLabMRAutomationService{optionsErr: errors.New("store unavailable")}
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", MRIID: 1, State: gitlabMRStateOpen}

	err := svc.handleTaskMRLifecycleAutomation(ctx, mr)
	if err == nil {
		t.Fatal("expected a transient options-load error to be returned rather than swallowed")
	}
}

func TestHandleTaskMRLifecycleAutomation_ArchivedTaskDiscardsSilently(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	require.NoError(t, repo.ArchiveTask(ctx, "task-1"))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.gitlabMRAutomation = &mockGitLabMRAutomationService{options: &gitlab.TaskMRAutomationResponse{PromptOnMerged: true}}
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", MRIID: 1, State: gitlabMRStateMerged}

	if err := svc.handleTaskMRLifecycleAutomation(ctx, mr); err != nil {
		t.Fatalf("archived task should discard silently, got err: %v", err)
	}
}

// TestHandleTaskMRLifecycleAutomation_PublishesStateOnEvaluationError proves
// a review finding: a lifecycle evaluation failure is recorded via
// RecordTaskMRAutomationError but was never pushed to connected clients, so
// the Review follow-up UI kept a stale MRStates.LastError until a manual
// refetch. Matches GitHub's handleTaskPRLifecycleAutomation, which publishes
// state right after recording its own CI/lifecycle error.
func TestHandleTaskMRLifecycleAutomation_PublishesStateOnEvaluationError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eb := &recordingEventBus{}
	svc.eventBus = eb
	automation := &mockGitLabMRAutomationService{
		options:          &gitlab.TaskMRAutomationResponse{PromptOnReviewRequested: true, ReviewReviewerUsername: "alice"},
		reviewRequestErr: errors.New("reviewer read unavailable"),
	}
	svc.gitlabMRAutomation = automation
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/project", MRIID: 1, State: gitlabMRStateOpen}

	if err := svc.handleTaskMRLifecycleAutomation(ctx, mr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := automation.recordedErrors(); len(got) != 1 {
		t.Fatalf("expected exactly one recorded error, got %+v", got)
	}
	var published bool
	for _, e := range eb.events {
		if e.subject == events.GitLabTaskMROptionsUpdated {
			published = true
		}
	}
	if !published {
		t.Fatalf("expected gitlab.task_mr_options.updated to be published after recording the error, got events: %+v", eb.events)
	}
}

func TestHandleTaskMRLifecycleAutomation_NoSwitchesEnabledSkipsEvaluation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	automation := &mockGitLabMRAutomationService{options: &gitlab.TaskMRAutomationResponse{}}
	svc.gitlabMRAutomation = automation
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", MRIID: 1, State: gitlabMRStateOpen}

	if err := svc.handleTaskMRLifecycleAutomation(ctx, mr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := automation.recordedErrors(); len(got) != 0 {
		t.Fatalf("no evaluation should have run: %+v", got)
	}
}

// failingGetTaskRepo wraps a real repoStore but injects a transient error
// from GetTask, for the AC31 regression above.
type failingGetTaskRepo struct {
	repoStore
	err error
}

func (r *failingGetTaskRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	return nil, r.err
}

// TestGitLabMRStateConstantsMatchNormalizedVocabulary pins the orchestrator's
// state constants to the exact strings gitlab.normalizeMRState emits, which
// is a cross-package contract no other test covers.
//
// Every other assertion in this file compares against the constants
// themselves (mrState: gitlabMRStateOpen, ...), so they all keep passing if a
// constant's *value* drifts from what the gitlab package actually stores on
// TaskMR.State. The failure is silent and total: GitLab's REST API reports an
// open MR as "opened", and only convertRawMR's normalization turns that into
// "open". If either side stops agreeing, currentTaskMRReviewRequest's
// `mr.State != gitlabMRStateOpen` guard short-circuits for every open MR, so
// review-requested notifications never evaluate and never fire — with no test
// failure anywhere. (Observed live during QA against a mock-seeded MR whose
// raw "opened" state bypassed normalization: review_request_initialized
// stayed 0 forever.)
//
// gitlab.TestNormalizeMRState pins the producing side; this pins the
// consuming side. The literals below must not be replaced by the constants.
func TestGitLabMRStateConstantsMatchNormalizedVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		got    string
		want   string
		rawAPI string
	}{
		{name: "open", got: gitlabMRStateOpen, want: "open", rawAPI: "opened"},
		{name: "merged", got: gitlabMRStateMerged, want: "merged", rawAPI: "merged"},
		{name: "closed", got: gitlabMRStateClosed, want: "closed", rawAPI: "closed"},
		{name: "locked", got: gitlabMRStateLocked, want: "locked", rawAPI: "locked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.got,
				"orchestrator constant drifted from gitlab.normalizeMRState(%q) output; "+
					"lifecycle evaluation would silently stop matching this state", tc.rawAPI)
		})
	}
}
