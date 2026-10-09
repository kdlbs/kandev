package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/admission"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func (r *messageAddSwitchRepo) CreateMessageWithInitialTaskBrief(
	_ context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
) error {
	r.messagesMu.Lock()
	defer r.messagesMu.Unlock()
	if len(r.messages) == 0 {
		message.Content = candidate.Content
		candidate.Selected = true
	} else {
		candidate.Selected = false
	}
	message.PromptIndex = len(r.messages) + 1
	r.messages = append(r.messages, message)
	r.idempotentMessage = message
	return nil
}

type staleInitialTaskBriefRepo struct {
	*messageAddSwitchRepo
	staleDescription string
	admissionCalls   int
}

func (r *staleInitialTaskBriefRepo) CreateMessageWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
) error {
	if r.admissionCalls == 0 {
		r.admissionCalls++
		r.tasks[message.TaskID].Description = r.staleDescription
		return repoerrors.ErrInitialTaskBriefStale
	}
	return r.messageAddSwitchRepo.CreateMessageWithInitialTaskBrief(ctx, message, candidate)
}

func (r *messageAddSwitchRepo) CreateMessageWithPlanCommentsWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	candidate *admission.InitialTaskBriefCandidate,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
) (*models.TaskPlanCommentSnapshot, error) {
	r.messagesMu.Lock()
	if len(r.messages) == 0 {
		message.Content = candidate.Content
		candidate.Selected = true
	} else {
		candidate.Selected = false
	}
	r.messagesMu.Unlock()
	return r.CreateMessageWithPlanComments(ctx, message, refs, requirePrimary, expectedState, claim)
}

func (r *messageAddSwitchRepo) CreateMessageWithPlanCommentsAndQueueWithInitialTaskBrief(
	ctx context.Context,
	message *models.Message,
	queued *messagequeue.QueuedMessage,
	candidate *admission.InitialTaskBriefCandidate,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	expectedState models.TaskSessionState,
	claim *messagequeue.QueueAttachmentClaim,
	maxPerSession int,
) (*models.TaskPlanCommentSnapshot, error) {
	snapshot, err := r.CreateMessageWithPlanCommentsWithInitialTaskBrief(
		ctx, message, candidate, refs, requirePrimary, expectedState, claim,
	)
	if err != nil {
		return nil, err
	}
	queued.Content = message.Content
	copy := *queued
	r.queuedMessage = &copy
	_ = maxPerSession
	return snapshot, nil
}

type initialTaskBriefPromptOrchestrator struct {
	firstTurnCaptureOrchestrator
	expansions    map[string]string
	prepareInputs []string
}

type readySessionPromptCapture struct {
	firstTurnCaptureOrchestrator
	mu                          sync.Mutex
	initialBriefAdmissionMu     sync.Mutex
	initialBriefDispatchPending bool
	promptHold                  <-chan struct{}
	promptErr                   error
	prompts                     chan readyPromptCall
	resumeCalls                 chan readyPromptCall
	expansions                  map[string]string
	prepareInputs               []string
}

type readyPromptCall struct {
	deliverySubmissionID          string
	content                       string
	model                         string
	planMode                      bool
	attachments                   []v1.MessageAttachment
	dispatchOnly                  bool
	promptReferenceContext        string
	promptReferencesPrepared      bool
	references                    []v1.EntityReference
	initialTaskBriefDispatchOwner bool
}

func (o *readySessionPromptCapture) PromptTask(
	_ context.Context,
	_, _ string,
	prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
) (*orchestrator.PromptResult, error) {
	o.prompts <- readyPromptCall{
		content: prompt, model: model, planMode: planMode,
		attachments: append([]v1.MessageAttachment(nil), attachments...), dispatchOnly: dispatchOnly,
	}
	return &orchestrator.PromptResult{}, nil
}

func (o *readySessionPromptCapture) PromptTaskWithPromptContext(
	ctx context.Context,
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	promptReferenceContext string,
	promptReferencesPrepared bool,
	references []v1.EntityReference,
	dispatchOnly bool,
) (*orchestrator.PromptResult, error) {
	return o.promptTaskWithPromptContext(
		prompt, model, planMode, attachments, promptReferenceContext,
		promptReferencesPrepared, references, dispatchOnly, false,
	)
}

func (o *readySessionPromptCapture) PromptTaskWithPromptContextAndDispatchOwnership(
	ctx context.Context,
	taskID, sessionID, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	promptReferenceContext string,
	promptReferencesPrepared bool,
	references []v1.EntityReference,
	dispatchOnly bool,
	initialTaskBriefDispatchOwner bool,
) (*orchestrator.PromptResult, error) {
	return o.promptTaskWithPromptContext(
		prompt, model, planMode, attachments, promptReferenceContext,
		promptReferencesPrepared, references, dispatchOnly, initialTaskBriefDispatchOwner,
	)
}

func (o *readySessionPromptCapture) promptTaskWithPromptContext(
	prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	promptReferenceContext string,
	promptReferencesPrepared bool,
	references []v1.EntityReference,
	dispatchOnly bool,
	initialTaskBriefDispatchOwner bool,
	submissionIDs ...string,
) (*orchestrator.PromptResult, error) {
	submissionID := ""
	if len(submissionIDs) > 0 {
		submissionID = submissionIDs[0]
	}
	o.prompts <- readyPromptCall{
		deliverySubmissionID: submissionID,
		content:              prompt, model: model, planMode: planMode,
		attachments: append([]v1.MessageAttachment(nil), attachments...), dispatchOnly: dispatchOnly,
		promptReferenceContext:        promptReferenceContext,
		promptReferencesPrepared:      promptReferencesPrepared,
		references:                    append([]v1.EntityReference(nil), references...),
		initialTaskBriefDispatchOwner: initialTaskBriefDispatchOwner,
	}
	if o.promptHold != nil {
		<-o.promptHold
	}
	return &orchestrator.PromptResult{}, o.promptErr
}

func (o *readySessionPromptCapture) ResumeTaskSessionAndPromptWithPromptContext(
	_ context.Context,
	_, _, prompt, model string,
	planMode bool,
	attachments []v1.MessageAttachment,
	promptReferenceContext string,
	promptReferencesPrepared bool,
	references []v1.EntityReference,
	initialTaskBriefDispatchOwner bool,
	submissionIDs ...string,
) (*orchestrator.PromptResult, error) {
	submissionID := ""
	if len(submissionIDs) > 0 {
		submissionID = submissionIDs[0]
	}
	o.resumeCalls <- readyPromptCall{
		deliverySubmissionID: submissionID,
		content:              prompt, model: model, planMode: planMode,
		attachments:                   append([]v1.MessageAttachment(nil), attachments...),
		promptReferenceContext:        promptReferenceContext,
		promptReferencesPrepared:      promptReferencesPrepared,
		references:                    append([]v1.EntityReference(nil), references...),
		initialTaskBriefDispatchOwner: initialTaskBriefDispatchOwner,
	}
	return &orchestrator.PromptResult{}, nil
}

func (o *readySessionPromptCapture) WithInitialTaskBriefAdmission(
	ctx context.Context,
	_ string,
	fn func(context.Context) error,
) error {
	o.initialBriefAdmissionMu.Lock()
	defer o.initialBriefAdmissionMu.Unlock()
	return fn(ctx)
}

func (o *readySessionPromptCapture) InitialTaskBriefDispatchPending(string) bool {
	return o.initialBriefDispatchPending
}

func (o *readySessionPromptCapture) MarkInitialTaskBriefDispatchPending(string) {
	o.initialBriefDispatchPending = true
}

func (o *readySessionPromptCapture) CompleteInitialTaskBriefDispatch(context.Context, string, string) {
	o.initialBriefAdmissionMu.Lock()
	o.initialBriefDispatchPending = false
	o.initialBriefAdmissionMu.Unlock()
}

func (o *readySessionPromptCapture) initialBriefDispatchPendingSnapshot() bool {
	o.initialBriefAdmissionMu.Lock()
	defer o.initialBriefAdmissionMu.Unlock()
	return o.initialBriefDispatchPending
}

func (o *readySessionPromptCapture) PrepareDirectPrompt(
	_ context.Context,
	prompt string,
	isPassthrough bool,
) (string, string) {
	o.mu.Lock()
	o.prepareInputs = append(o.prepareInputs, prompt)
	expansions := make(map[string]string, len(o.expansions))
	for name, content := range o.expansions {
		expansions[name] = content
	}
	o.mu.Unlock()
	if isPassthrough {
		return prompt, ""
	}
	trustedContext := initialTaskBriefTrustedContext(prompt, expansions)
	if trustedContext == "" {
		return prompt, ""
	}
	return prompt + "\n\n" + sysprompt.Wrap(trustedContext), trustedContext
}

func (o *readySessionPromptCapture) setExpansion(name, content string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.expansions == nil {
		o.expansions = make(map[string]string)
	}
	o.expansions[name] = content
}

func (o *readySessionPromptCapture) prepareInputsSnapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.prepareInputs...)
}

func (r *messageAddSwitchRepo) HasUserPromptHistory(_ context.Context, _ string) (bool, error) {
	r.messagesMu.Lock()
	r.promptHistoryCalls++
	hasHistory := r.hasUserPromptHistory || len(r.messages) > 0
	historyErr := r.promptHistoryErr
	barrier := r.promptHistoryBarrier
	if barrier != nil && r.promptHistoryCalls == r.promptHistoryBarrierAfter {
		close(barrier)
	}
	r.messagesMu.Unlock()
	if barrier != nil {
		<-barrier
	}
	return hasHistory, historyErr
}

func (r *messageAddSwitchRepo) messageContents() []string {
	r.messagesMu.Lock()
	defer r.messagesMu.Unlock()
	contents := make([]string, 0, len(r.messages))
	for _, message := range r.messages {
		contents = append(contents, message.Content)
	}
	return contents
}

func newReadyInitialTaskBriefHarness(
	t *testing.T,
	brief string,
	validators ...entityrefs.SubmissionValidator,
) (*messageAddSwitchRepo, *readySessionPromptCapture, *MessageHandlers) {
	t.Helper()
	const (
		taskID    = "task-initial-brief-ready-harness"
		sessionID = "session-initial-brief-ready-harness"
	)
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{taskID: {
			ID: taskID, Description: brief, State: v1.TaskStateInProgress, UpdatedAt: now,
		}},
		sessions: map[string]*models.TaskSession{sessionID: {
			ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
			AgentProfileID: "profile-initial-brief-ready-harness", UpdatedAt: now,
		}},
		primaryID: sessionID,
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &readySessionPromptCapture{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 2)},
		prompts:                      make(chan readyPromptCall, 3),
		resumeCalls:                  make(chan readyPromptCall, 3),
		expansions:                   map[string]string{},
	}
	return repo, orch, NewMessageHandlers(svc, orch, log, validators...)
}

func newReadyBriefRequest(t *testing.T, messageID, instruction string, extra map[string]interface{}) *ws.Message {
	t.Helper()
	payload := map[string]interface{}{
		"task_id": "task-initial-brief-ready-harness", "session_id": "session-initial-brief-ready-harness",
		"message_id": messageID, "content": instruction,
	}
	for key, value := range extra {
		payload[key] = value
	}
	request, err := ws.NewRequest(messageID+"-request", ws.ActionMessageAdd, payload)
	require.NoError(t, err)
	return request
}

func (o *initialTaskBriefPromptOrchestrator) PrepareDirectPrompt(
	_ context.Context,
	prompt string,
	_ bool,
) (string, string) {
	o.prepareInputs = append(o.prepareInputs, prompt)
	trustedContext := initialTaskBriefTrustedContext(prompt, o.expansions)
	if trustedContext == "" {
		return prompt, ""
	}
	return prompt + "\n\n" + sysprompt.Wrap(trustedContext), trustedContext
}

func initialTaskBriefTrustedContext(prompt string, expansions map[string]string) string {
	type reference struct {
		name  string
		index int
	}
	refs := make([]reference, 0, len(expansions))
	for name := range expansions {
		if index := strings.Index(prompt, "@"+name); index >= 0 {
			refs = append(refs, reference{name: name, index: index})
		}
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].index < refs[j].index })
	if len(refs) == 0 {
		return ""
	}
	lines := []string{"EXPANDED PROMPT REFERENCES:"}
	for _, ref := range refs {
		lines = append(lines, fmt.Sprintf("### @%s", ref.name), expansions[ref.name])
	}
	return strings.Join(lines, "\n")
}

func (o *firstTurnCaptureOrchestrator) StartCreatedSessionWithPromptContextAndCanvasGuidancePreservingDirectPrompt(
	_ context.Context,
	_, _, _, content string,
	_, _, _ bool,
	_ []v1.MessageAttachment,
	references []v1.EntityReference,
	promptReferenceContext string,
	_ bool,
	_, _ bool,
) (*executor.TaskExecution, error) {
	o.started <- capturedFirstTurn{
		content:                content,
		references:             append([]v1.EntityReference(nil), references...),
		promptReferenceContext: promptReferenceContext,
	}
	return &executor.TaskExecution{}, nil
}

func (o *firstTurnCaptureOrchestrator) StartCreatedSessionWithDeliverySubmission(
	_ context.Context,
	_, _, _, content string,
	options orchestrator.DirectPromptStartOptions,
) (*executor.TaskExecution, error) {
	o.started <- capturedFirstTurn{
		content:                content,
		references:             append([]v1.EntityReference(nil), options.References...),
		promptReferenceContext: options.PromptReferenceContext,
		deliverySubmissionID:   options.DeliverySubmissionID,
	}
	return &executor.TaskExecution{}, nil
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.1, AC-TASKS-INITIAL-TASK-BRIEF-001.2
func TestWSAddMessage_InitialTaskBrief(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			taskID      = "task-initial-brief"
			sessionID   = "session-initial-brief"
			brief       = "Ship the authenticated task view."
			instruction = "Use the existing session and keep the change small."
		)
		now := time.Now().UTC()
		repo := &messageAddSwitchRepo{
			tasks: map[string]*models.Task{
				taskID: {
					ID:          taskID,
					Description: brief,
					State:       v1.TaskStateInProgress,
					UpdatedAt:   now,
				},
			},
			sessions: map[string]*models.TaskSession{
				sessionID: {
					ID:             sessionID,
					TaskID:         taskID,
					State:          models.TaskSessionStateCreated,
					AgentProfileID: "profile-initial-brief",
					UpdatedAt:      now,
				},
			},
			primaryID: sessionID,
		}
		log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		require.NoError(t, err)
		svc := service.NewService(service.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo,
			Workflows: repo, Messages: repo, Turns: repo,
			Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
			Executors: repo, Environments: repo, TaskEnvironments: repo,
			Reviews: repo,
		}, nil, log, service.RepositoryDiscoveryConfig{})
		orch := &firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)}
		h := NewMessageHandlers(svc, orch, log)

		req, err := ws.NewRequest("initial-brief-request", ws.ActionMessageAdd, map[string]interface{}{
			"task_id": taskID, "session_id": sessionID,
			"message_id": "initial-brief-message", "content": instruction,
		})
		require.NoError(t, err)

		resp, err := h.wsAddMessage(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		require.Len(t, repo.messages, 1)

		stored := repo.firstMessageContent()
		require.Contains(t, stored, brief)
		require.Contains(t, stored, instruction)
		require.Equal(t, 1, strings.Count(stored, brief))
		require.Equal(t, 1, strings.Count(stored, instruction))

		synctest.Wait()
		dispatched := <-orch.started
		require.Contains(t, dispatched.content, brief)
		require.Contains(t, dispatched.content, instruction)
		require.Equal(t, stored, dispatched.content)
		require.Equal(t, "initial-brief-message", dispatched.deliverySubmissionID)
	})
}

func (o *switchingTurnStartOrchestrator) StartCreatedSessionWithDeliverySubmission(
	_ context.Context,
	_, sessionID, _, _ string,
	options orchestrator.DirectPromptStartOptions,
) (*executor.TaskExecution, error) {
	o.mu.Lock()
	o.startedSession = sessionID
	o.startedSubmission = options.DeliverySubmissionID
	o.mu.Unlock()
	o.startOnce.Do(func() {
		if o.started != nil {
			close(o.started)
		}
	})
	return &executor.TaskExecution{}, nil
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.1, AC-TASKS-INITIAL-TASK-BRIEF-001.2, AC-TASKS-INITIAL-TASK-BRIEF-001.11, AC-TASKS-INITIAL-TASK-BRIEF-001.12
func TestWSAddMessage_InitialTaskBriefReadySession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			taskID      = "task-initial-brief-ready"
			sessionID   = "session-initial-brief-ready"
			brief       = "Ship the authenticated task view."
			instruction = "Use the existing recovered session and keep the change small."
		)
		now := time.Now().UTC()
		repo := &messageAddSwitchRepo{
			tasks: map[string]*models.Task{taskID: {
				ID: taskID, Description: brief, State: v1.TaskStateInProgress, UpdatedAt: now,
			}},
			sessions: map[string]*models.TaskSession{sessionID: {
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
				AgentProfileID: "profile-initial-brief-ready", AgentExecutionID: "execution-lifecycle-boot",
				DownstreamACPSessionID: "provider-conversation-existing",
				Metadata:               map[string]interface{}{"lifecycle_only_boot": true}, UpdatedAt: now,
			}},
			primaryID: sessionID,
		}
		log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		require.NoError(t, err)
		svc := service.NewService(service.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo,
			Workflows: repo, Messages: repo, Turns: repo,
			Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
			Executors: repo, Environments: repo, TaskEnvironments: repo,
			Reviews: repo,
		}, nil, log, service.RepositoryDiscoveryConfig{})
		orch := &readySessionPromptCapture{
			firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
			prompts:                      make(chan readyPromptCall, 1),
			expansions:                   map[string]string{},
		}
		h := NewMessageHandlers(svc, orch, log)

		req, err := ws.NewRequest("initial-brief-ready-request", ws.ActionMessageAdd, map[string]interface{}{
			"task_id": taskID, "session_id": sessionID,
			"message_id": "initial-brief-ready-message", "content": instruction,
		})
		require.NoError(t, err)

		resp, err := h.wsAddMessage(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		require.Len(t, repo.messages, 1)

		stored := repo.firstMessageContent()
		require.Contains(t, stored, brief)
		require.Contains(t, stored, instruction)
		require.Equal(t, 1, strings.Count(stored, brief))
		require.Equal(t, 1, strings.Count(stored, instruction))

		synctest.Wait()
		dispatched := <-orch.prompts
		require.Equal(t, stored, dispatched.content)
		require.Equal(t, "initial-brief-ready-message", dispatched.deliverySubmissionID)
		require.True(t, dispatched.initialTaskBriefDispatchOwner)
		select {
		case started := <-orch.started:
			t.Fatalf("ready session was relaunched through StartCreatedSession: %+v", started)
		default:
		}
	})
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5, AC-TASKS-INITIAL-TASK-BRIEF-001.6, AC-TASKS-INITIAL-TASK-BRIEF-001.11
func TestWSAddMessage_InitialTaskBriefReadySessionAdmission(t *testing.T) {
	t.Run("prompt history marker suppresses brief after deletion or reservation", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const instruction = "Continue the previously prompted task."
			repo, orch, handler := newReadyInitialTaskBriefHarness(t, "Original task brief")
			// A durable sequence marker can remain after the visible row was deleted,
			// and a zero reservation has no visible row at all.
			repo.hasUserPromptHistory = true

			response, err := handler.wsAddMessage(context.Background(),
				newReadyBriefRequest(t, "ready-history-marker", instruction, nil))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, response.Type)
			require.Equal(t, []string{instruction}, repo.messageContents())
			require.Len(t, orch.prepareInputsSnapshot(), 1, "history must skip candidate preparation")
			require.Equal(t, 1, repo.promptHistoryCalls)

			synctest.Wait()
			prompt := <-orch.prompts
			require.Equal(t, instruction, prompt.content)
			require.Empty(t, orch.queueCalls(), "a later ready-session prompt is not an initial contender")
			select {
			case <-orch.started:
				t.Fatal("ready-session follow-up must not call StartCreatedSession")
			default:
			}
		})
	})

	t.Run("history read error stops before persistence and dispatch", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			repo, orch, handler := newReadyInitialTaskBriefHarness(t, "Original task brief")
			repo.promptHistoryErr = fmt.Errorf("prompt history unavailable")

			response, err := handler.wsAddMessage(context.Background(),
				newReadyBriefRequest(t, "ready-history-error", "Send after recovery.", nil))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, response.Type)
			require.Contains(t, string(response.Payload), "Failed to check session prompt history")
			require.Empty(t, repo.messageContents())
			require.Equal(t, 1, repo.promptHistoryCalls)

			synctest.Wait()
			require.Empty(t, orch.prompts)
			require.Empty(t, orch.queueCalls())
		})
	})

	t.Run("follow-up after the selected first prompt uses ordinary content", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const (
				brief       = "Original task brief"
				instruction = "Start with this instruction."
				followup    = "Continue with only this follow-up."
			)
			repo, orch, handler := newReadyInitialTaskBriefHarness(t, brief)
			firstResponse, err := handler.wsAddMessage(context.Background(),
				newReadyBriefRequest(t, "ready-first-prompt", instruction, nil))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, firstResponse.Type)
			synctest.Wait()
			firstPrompt := <-orch.prompts
			require.Contains(t, firstPrompt.content, brief)
			require.Contains(t, firstPrompt.content, instruction)

			secondResponse, err := handler.wsAddMessage(context.Background(),
				newReadyBriefRequest(t, "ready-followup-prompt", followup, nil))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeResponse, secondResponse.Type)
			synctest.Wait()
			secondPrompt := <-orch.prompts

			contents := repo.messageContents()
			require.Len(t, contents, 2)
			require.Contains(t, contents[0], brief)
			require.Contains(t, contents[0], instruction)
			require.Equal(t, followup, contents[1])
			require.Equal(t, followup, secondPrompt.content)
			require.Equal(t, 2, repo.promptHistoryCalls)
			require.Empty(t, orch.queueCalls())
		})
	})
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.1, AC-TASKS-INITIAL-TASK-BRIEF-001.3
func TestWSAddMessage_InitialTaskBriefReadySessionEligibility(t *testing.T) {
	tests := []struct {
		name             string
		passthrough      bool
		isEphemeral      bool
		isFromOffice     bool
		configMode       bool
		wantInitialBrief bool
		wantPrepareCalls int
	}{
		{name: "structured ready session", wantInitialBrief: true, wantPrepareCalls: 2},
		{name: "passthrough preserves visible brief", passthrough: true, wantInitialBrief: true},
		{name: "Office task excluded", isFromOffice: true, wantPrepareCalls: 1},
		{name: "ephemeral Quick Chat excluded", isEphemeral: true, wantPrepareCalls: 1},
		{name: "config session excluded", configMode: true, wantPrepareCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const (
					brief       = "Original task brief"
					instruction = "Keep this recovered session."
				)
				repo, orch, handler := newReadyInitialTaskBriefHarness(t, brief)
				task := repo.tasks["task-initial-brief-ready-harness"]
				task.IsEphemeral = tt.isEphemeral
				task.IsFromOffice = tt.isFromOffice
				session := repo.sessions["session-initial-brief-ready-harness"]
				session.IsPassthrough = tt.passthrough
				if tt.configMode {
					session.Metadata = map[string]interface{}{"config_mode": true}
				}

				response, err := handler.wsAddMessage(context.Background(),
					newReadyBriefRequest(t, "ready-eligibility", instruction, nil))
				require.NoError(t, err)
				require.Equal(t, ws.MessageTypeResponse, response.Type)
				stored := repo.messageContents()[0]
				require.Contains(t, stored, instruction)
				require.Equal(t, 1, strings.Count(stored, instruction))
				if tt.wantInitialBrief {
					require.Equal(t, 1, strings.Count(stored, brief))
				} else {
					require.NotContains(t, stored, brief)
				}
				require.Len(t, orch.prepareInputsSnapshot(), tt.wantPrepareCalls)
				if tt.isEphemeral || tt.isFromOffice || tt.configMode {
					require.Zero(t, repo.promptHistoryCalls, "excluded sessions do not need the marker lookup")
				} else {
					require.Equal(t, 1, repo.promptHistoryCalls)
				}

				synctest.Wait()
				dispatched := <-orch.prompts
				require.Equal(t, stored, dispatched.content)
				select {
				case <-orch.started:
					t.Fatal("ready-session delivery must not call StartCreatedSession")
				default:
				}
			})
		})
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.4, AC-TASKS-INITIAL-TASK-BRIEF-001.8, AC-TASKS-INITIAL-TASK-BRIEF-001.11
func TestWSAddMessage_InitialTaskBriefReadySessionKeepsAcceptedEmptyContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			brief       = "Implement @brief_rules"
			instruction = "Use the existing conversation."
		)
		repo, orch, handler := newReadyInitialTaskBriefHarness(t, brief)
		response, err := handler.wsAddMessage(context.Background(),
			newReadyBriefRequest(t, "ready-empty-context", instruction, nil))
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, response.Type)
		stored := repo.messageContents()[0]
		require.Contains(t, stored, brief)
		require.Contains(t, stored, instruction)
		require.NotContains(t, stored, "EXPANDED PROMPT REFERENCES")
		require.Len(t, orch.prepareInputsSnapshot(), 2)

		// A late definition must not rewrite the already accepted empty snapshot.
		orch.setExpansion("brief_rules", "definition added after admission")
		synctest.Wait()
		dispatched := <-orch.prompts
		require.Equal(t, stored, dispatched.content)
		require.NotContains(t, dispatched.content, "definition added after admission")
		require.Empty(t, dispatched.promptReferenceContext)
		require.True(t, dispatched.promptReferencesPrepared, "the accepted empty snapshot must reach ordinary prompt delivery")
		require.Empty(t, dispatched.references)
		require.Len(t, orch.prepareInputsSnapshot(), 2, "accepted content is not expanded again during dispatch")
	})
}

func TestWSAddMessage_ReadyInitialBriefRecoveryRetryPreservesAcceptedContext(t *testing.T) {
	const (
		brief       = "Review @brief_rules"
		instruction = "Continue in the recovered conversation."
	)
	reference := v1.EntityReference{
		Version:  v1.EntityReferenceVersion,
		Ref:      entityrefs.CanonicalRef("kandev", "task", "ws1", "retry-reference"),
		Provider: "kandev", Kind: "task", ID: "retry-reference", Title: "Retry reference",
		URL: "/t/retry-reference", Scope: "ws1",
	}
	validator := &fakeReferenceSubmissionValidator{}
	repo, orch, handler := newReadyInitialTaskBriefHarness(t, brief, validator)
	orch.setExpansion("brief_rules", "accepted retry rules")
	orch.promptErr = executor.ErrExecutionNotFound

	response, err := handler.wsAddMessage(context.Background(), newReadyBriefRequest(t,
		"ready-context-retry", instruction,
		map[string]interface{}{"entity_references": []v1.EntityReference{reference}},
	))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	stored := repo.messageContents()[0]
	require.Contains(t, stored, brief)
	require.Contains(t, stored, instruction)
	require.Contains(t, stored, "accepted retry rules")

	dispatched := <-orch.prompts
	require.Equal(t, stored, dispatched.content)
	require.Equal(t, "EXPANDED PROMPT REFERENCES:\n### @brief_rules\naccepted retry rules", dispatched.promptReferenceContext)
	require.True(t, dispatched.promptReferencesPrepared)
	require.Equal(t, []v1.EntityReference{reference}, dispatched.references)
	require.True(t, dispatched.initialTaskBriefDispatchOwner)

	retry := <-orch.resumeCalls
	require.Equal(t, "ready-context-retry", retry.deliverySubmissionID)
	require.Equal(t, stored, retry.content)
	require.Equal(t, dispatched.promptReferenceContext, retry.promptReferenceContext)
	require.True(t, retry.promptReferencesPrepared)
	require.Equal(t, []v1.EntityReference{reference}, retry.references)
	require.True(t, retry.initialTaskBriefDispatchOwner)
	require.Equal(t, []v1.EntityReference{reference}, validator.references)
}

func TestWSAddMessage_ReadyFollowupQueuesBehindInitialTaskBriefDispatch(t *testing.T) {
	repo, orch, handler := newReadyInitialTaskBriefHarness(t, "Keep the recovered task objective.")
	holdDispatch := make(chan struct{})
	orch.promptHold = holdDispatch
	released := false
	defer func() {
		if !released {
			close(holdDispatch)
		}
	}()

	response, err := handler.wsAddMessage(context.Background(), newReadyBriefRequest(
		t, "ready-first-message", "Use the recovered conversation.", nil,
	))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	firstDispatch := <-orch.prompts
	require.Contains(t, firstDispatch.content, "Keep the recovered task objective.")
	require.True(t, orch.initialBriefDispatchPendingSnapshot())

	response, err = handler.wsAddMessage(context.Background(), newReadyBriefRequest(
		t, "ready-followup-message", "This is an ordinary follow-up.", nil,
	))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Eventually(t, func() bool { return len(orch.queueCalls()) == 1 }, time.Second, time.Millisecond)
	queued := orch.queueCalls()[0]
	require.Contains(t, queued.prompt, "This is an ordinary follow-up.")
	select {
	case prompt := <-orch.prompts:
		t.Fatalf("follow-up overtook the initial brief dispatch: %+v", prompt)
	default:
	}

	close(holdDispatch)
	released = true
	require.Eventually(t, func() bool { return !orch.initialBriefDispatchPendingSnapshot() }, time.Second, time.Millisecond)
	_ = repo
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.2, AC-TASKS-INITIAL-TASK-BRIEF-001.4, AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_InitialTaskBriefReadySessionPreservesPromptMetadata(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			brief       = "Review @brief_rules"
			instruction = "Plan the first change and inspect the linked task."
		)
		reference := v1.EntityReference{
			Version:  v1.EntityReferenceVersion,
			Ref:      entityrefs.CanonicalRef("kandev", "task", "ws1", "other"),
			Provider: "kandev", Kind: "task", ID: "other", Title: "Other task",
			URL: "/t/other", Scope: "ws1",
		}
		validator := &fakeReferenceSubmissionValidator{}
		repo, orch, handler := newReadyInitialTaskBriefHarness(t, brief, validator)
		orch.setExpansion("brief_rules", "accepted brief rules")
		attachment := v1.MessageAttachment{Type: "resource", MimeType: "text/plain", Data: "dGVzdA==", Name: "notes.txt"}
		response, err := handler.wsAddMessage(context.Background(), newReadyBriefRequest(t,
			"ready-metadata", instruction, map[string]interface{}{
				"plan_mode": true,
				"attachments": []map[string]interface{}{{
					"type": attachment.Type, "mime_type": attachment.MimeType,
					"data": attachment.Data, "name": attachment.Name,
				}},
				"entity_references": []v1.EntityReference{reference},
			}),
		)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, response.Type)
		require.Len(t, repo.messages, 1)
		stored := repo.messages[0]
		require.Contains(t, stored.Content, brief)
		require.Contains(t, stored.Content, instruction)
		require.Contains(t, stored.Content, "accepted brief rules")
		require.Contains(t, stored.Content, "Validated work-item reference snapshots")
		require.Equal(t, []v1.EntityReference{reference}, validator.references)
		require.Equal(t, []v1.EntityReference{reference}, stored.Metadata["entity_references"])

		synctest.Wait()
		dispatched := <-orch.prompts
		require.Equal(t, stored.Content, dispatched.content)
		require.True(t, dispatched.planMode)
		require.Equal(t, []v1.MessageAttachment{attachment}, dispatched.attachments)
		require.Equal(t,
			"EXPANDED PROMPT REFERENCES:\n### @brief_rules\naccepted brief rules",
			dispatched.promptReferenceContext,
		)
		require.True(t, dispatched.promptReferencesPrepared)
		require.Equal(t, []v1.EntityReference{reference}, dispatched.references)
		require.True(t, dispatched.initialTaskBriefDispatchOwner)
	})
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.4, AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_InitialTaskBriefExpandsCombinedPromptAtAdmission(t *testing.T) {
	tests := []struct {
		name        string
		brief       string
		instruction string
		expansions  map[string]string
		wantVisible []string
		wantTrusted []string
		wantContext string
	}{
		{
			name:        "distinct references in brief and instruction",
			brief:       "Implement @brief_rules",
			instruction: "First follow @prep_rules",
			expansions: map[string]string{
				"brief_rules": "brief rules v1",
				"prep_rules":  "prep rules v1",
			},
			wantVisible: []string{"Implement @brief_rules", "First follow @prep_rules"},
			wantTrusted: []string{"brief rules v1", "prep rules v1"},
			wantContext: "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nbrief rules v1\n### @prep_rules\nprep rules v1",
		},
		{
			name:        "reference only in brief",
			brief:       "Implement @brief_rules",
			instruction: "Keep the existing session",
			expansions:  map[string]string{"brief_rules": "brief rules v1"},
			wantVisible: []string{"Implement @brief_rules", "Keep the existing session"},
			wantTrusted: []string{"brief rules v1"},
			wantContext: "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nbrief rules v1",
		},
		{
			name:        "identical brief and instruction",
			brief:       "Follow @rules",
			instruction: "Follow @rules",
			expansions:  map[string]string{"rules": "shared rules v1"},
			wantVisible: []string{"Follow @rules"},
			wantTrusted: []string{"shared rules v1"},
			wantContext: "EXPANDED PROMPT REFERENCES:\n### @rules\nshared rules v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const (
					taskID    = "task-initial-brief-prepared"
					sessionID = "session-initial-brief-prepared"
				)
				now := time.Now().UTC()
				repo := &messageAddSwitchRepo{
					tasks: map[string]*models.Task{
						taskID: {
							ID: taskID, Description: tt.brief, State: v1.TaskStateInProgress,
							UpdatedAt: now,
						},
					},
					sessions: map[string]*models.TaskSession{
						sessionID: {
							ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
							AgentProfileID: "profile-initial-brief-prepared", UpdatedAt: now,
						},
					},
					primaryID: sessionID,
				}
				log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
				require.NoError(t, err)
				orch := &initialTaskBriefPromptOrchestrator{
					firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{
						started: make(chan capturedFirstTurn, 1),
					},
					expansions: tt.expansions,
				}
				svc := service.NewService(service.Repos{
					Workspaces: repo, Tasks: repo, TaskRepos: repo,
					Workflows: repo, Messages: repo, Turns: repo,
					Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
					Executors: repo, Environments: repo, TaskEnvironments: repo,
					Reviews: repo,
				}, nil, log, service.RepositoryDiscoveryConfig{})
				h := NewMessageHandlers(svc, orch, log)

				req, err := ws.NewRequest("initial-brief-prepared-request", ws.ActionMessageAdd, map[string]interface{}{
					"task_id": taskID, "session_id": sessionID,
					"message_id": "initial-brief-prepared-message", "content": tt.instruction,
				})
				require.NoError(t, err)

				resp, err := h.wsAddMessage(context.Background(), req)
				require.NoError(t, err)
				require.Equal(t, ws.MessageTypeResponse, resp.Type)
				require.Len(t, repo.messages, 1)
				require.Len(t, orch.prepareInputs, 2)
				require.Equal(t, tt.instruction, orch.prepareInputs[0])
				require.Equal(t, composeInitialTaskBrief(tt.brief, tt.instruction), orch.prepareInputs[1])

				stored := repo.firstMessageContent()
				for _, visible := range tt.wantVisible {
					require.Equal(t, 1, strings.Count(stored, visible), "visible prompt %q in %q", visible, stored)
				}
				for _, trusted := range tt.wantTrusted {
					require.Equal(t, 1, strings.Count(stored, trusted), "trusted expansion %q in %q", trusted, stored)
				}
				require.Contains(t, stored, sysprompt.Wrap(tt.wantContext))

				synctest.Wait()
				dispatched := <-orch.started
				require.Equal(t, stored, dispatched.content)
				require.Equal(t, tt.wantContext, dispatched.promptReferenceContext)
			})
		})
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_InitialTaskBriefKeepsAcceptedExpansionWhenDefinitionsChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			taskID      = "task-initial-brief-snapshot"
			sessionID   = "session-initial-brief-snapshot"
			brief       = "Implement @brief_rules"
			instruction = "First follow @prep_rules"
		)
		now := time.Now().UTC()
		repo := &messageAddSwitchRepo{
			tasks: map[string]*models.Task{taskID: {
				ID: taskID, Description: brief, State: v1.TaskStateInProgress, UpdatedAt: now,
			}},
			sessions: map[string]*models.TaskSession{sessionID: {
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
				AgentProfileID: "profile-initial-brief-snapshot", UpdatedAt: now,
			}},
			primaryID: sessionID,
		}
		log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
		require.NoError(t, err)
		orch := &initialTaskBriefPromptOrchestrator{
			firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
			expansions: map[string]string{
				"brief_rules": "brief rules accepted",
				"prep_rules":  "prep rules accepted",
			},
		}
		svc := service.NewService(service.Repos{
			Workspaces: repo, Tasks: repo, TaskRepos: repo,
			Workflows: repo, Messages: repo, Turns: repo,
			Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
			Executors: repo, Environments: repo, TaskEnvironments: repo,
			Reviews: repo,
		}, nil, log, service.RepositoryDiscoveryConfig{})
		h := NewMessageHandlers(svc, orch, log)
		req, err := ws.NewRequest("initial-brief-snapshot-request", ws.ActionMessageAdd, map[string]interface{}{
			"task_id": taskID, "session_id": sessionID,
			"message_id": "initial-brief-snapshot-message", "content": instruction,
		})
		require.NoError(t, err)

		resp, err := h.wsAddMessage(context.Background(), req)
		require.NoError(t, err)
		require.Equal(t, ws.MessageTypeResponse, resp.Type)
		orch.expansions["brief_rules"] = "brief rules changed before dispatch"
		orch.expansions["prep_rules"] = "prep rules changed before dispatch"

		stored := repo.firstMessageContent()
		expectedContext := "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nbrief rules accepted\n### @prep_rules\nprep rules accepted"
		require.Contains(t, stored, sysprompt.Wrap(expectedContext))
		require.NotContains(t, stored, "changed before dispatch")
		synctest.Wait()
		dispatched := <-orch.started
		require.Equal(t, expectedContext, dispatched.promptReferenceContext)
	})
}

func TestWSAddMessage_RejectsMismatchedTaskSessionBeforeReadingTask(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"task-a": {ID: "task-a", State: v1.TaskStateInProgress, UpdatedAt: now},
			"task-b": {ID: "task-b", State: v1.TaskStateInProgress, UpdatedAt: now},
		},
		sessions: map[string]*models.TaskSession{
			"session-b": {
				ID: "session-b", TaskID: "task-b", State: models.TaskSessionStateCreated,
				AgentProfileID: "profile-b", UpdatedAt: now,
			},
		},
		primaryID: "session-b",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)}
	h := NewMessageHandlers(svc, orch, log)

	request, err := ws.NewRequest("mismatched-pair", ws.ActionMessageAdd, map[string]any{
		"task_id": "task-a", "session_id": "session-b", "content": "send this",
	})
	require.NoError(t, err)
	response, err := h.wsAddMessage(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Contains(t, string(response.Payload), "Task and session do not match")
	require.Empty(t, repo.messages)
	require.Zero(t, repo.taskGetCalls)
	require.Zero(t, orch.onTurnStartCount())
}

func TestWSAddMessage_ConcurrentInitialBriefStartsOnlyAdmittedCandidate(t *testing.T) {
	now := time.Now().UTC()
	const taskID = "concurrent-initial-brief-task"
	const sessionID = "concurrent-initial-brief-session"
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			taskID: {
				ID: taskID, Description: "Concurrent task brief", State: v1.TaskStateInProgress,
				UpdatedAt: now,
			},
		},
		sessions: map[string]*models.TaskSession{
			sessionID: {
				ID: sessionID, TaskID: taskID, State: models.TaskSessionStateCreated,
				AgentProfileID: "profile-concurrent", UpdatedAt: now,
			},
		},
		primaryID: sessionID,
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 2)}
	h := NewMessageHandlers(svc, orch, log)
	start := make(chan struct{})
	responses := make(chan *ws.Message, 2)
	var wg sync.WaitGroup
	for index, instruction := range []string{"first contender", "second contender"} {
		request, requestErr := ws.NewRequest(
			fmt.Sprintf("concurrent-initial-brief-%d", index), ws.ActionMessageAdd,
			map[string]any{
				"task_id": taskID, "session_id": sessionID,
				"message_id": fmt.Sprintf("concurrent-initial-brief-message-%d", index),
				"content":    instruction,
			},
		)
		require.NoError(t, requestErr)
		wg.Add(1)
		go func(request *ws.Message) {
			defer wg.Done()
			<-start
			response, requestErr := h.wsAddMessage(context.Background(), request)
			require.NoError(t, requestErr)
			responses <- response
		}(request)
	}
	close(start)
	wg.Wait()
	close(responses)
	for response := range responses {
		require.Equal(t, ws.MessageTypeResponse, response.Type)
	}

	require.Eventually(t, func() bool { return len(orch.queueCalls()) == 1 }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return len(orch.started) == 1 }, time.Second, time.Millisecond)
	require.Len(t, repo.messages, 2)
	queuedCalls := orch.queueCalls()
	require.True(t, queuedCalls[0].userMessageRecorded)
	require.True(t, queuedCalls[0].metadata[orchestrator.MetaKeyInitialTaskBriefDispatchPending].(bool))
	require.Contains(t, queuedCalls[0].prompt, "contender")
	for _, message := range repo.messages {
		if strings.Contains(message.Content, "Concurrent task brief") {
			require.Equal(t, 1, message.PromptIndex)
		} else {
			require.Equal(t, 2, message.PromptIndex)
		}
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5, AC-TASKS-INITIAL-TASK-BRIEF-001.11
func TestWSAddMessage_InitialTaskBriefConcurrentReadySessionQueuesUnselectedCandidate(t *testing.T) {
	repo, orch, handler := newReadyInitialTaskBriefHarness(t, "Concurrent ready-session brief")
	repo.promptHistoryBarrier = make(chan struct{})
	repo.promptHistoryBarrierAfter = 2

	start := make(chan struct{})
	responses := make(chan *ws.Message, 2)
	var wg sync.WaitGroup
	for index, instruction := range []string{"first ready contender", "second ready contender"} {
		request, err := ws.NewRequest(
			fmt.Sprintf("concurrent-ready-brief-%d", index), ws.ActionMessageAdd,
			map[string]interface{}{
				"task_id":    "task-initial-brief-ready-harness",
				"session_id": "session-initial-brief-ready-harness",
				"message_id": fmt.Sprintf("concurrent-ready-brief-message-%d", index),
				"content":    instruction,
			},
		)
		require.NoError(t, err)
		wg.Add(1)
		go func(request *ws.Message) {
			defer wg.Done()
			<-start
			response, requestErr := handler.wsAddMessage(context.Background(), request)
			require.NoError(t, requestErr)
			responses <- response
		}(request)
	}
	close(start)
	wg.Wait()
	close(responses)
	for response := range responses {
		require.Equal(t, ws.MessageTypeResponse, response.Type)
	}

	require.Equal(t, 2, repo.promptHistoryCalls)
	require.Len(t, repo.messages, 2)
	require.Eventually(t, func() bool { return len(orch.queueCalls()) == 1 }, time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return len(orch.prompts) == 1 }, time.Second, time.Millisecond)
	queued := orch.queueCalls()[0]
	require.True(t, queued.userMessageRecorded)
	require.True(t, queued.metadata[orchestrator.MetaKeyInitialTaskBriefDispatchPending].(bool))
	require.Contains(t, queued.prompt, "ready contender")
	selected := 0
	for _, message := range repo.messages {
		if strings.Contains(message.Content, "Concurrent ready-session brief") {
			selected++
			require.Equal(t, 1, message.PromptIndex)
		} else {
			require.Equal(t, 2, message.PromptIndex)
		}
	}
	require.Equal(t, 1, selected)
	dispatched := <-orch.prompts
	require.Contains(t, dispatched.content, "Concurrent ready-session brief")
	select {
	case <-orch.started:
		t.Fatal("ready-session contention must not call StartCreatedSession")
	default:
	}
}

func TestWSAddMessage_RefreshesStaleBriefWithoutRepeatingTurnStart(t *testing.T) {
	now := time.Now().UTC()
	baseRepo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{
			"stale-brief-task": {
				ID: "stale-brief-task", Description: "Original task brief", State: v1.TaskStateInProgress,
				UpdatedAt: now,
			},
		},
		sessions: map[string]*models.TaskSession{
			"stale-brief-session": {
				ID: "stale-brief-session", TaskID: "stale-brief-task", State: models.TaskSessionStateCreated,
				AgentProfileID: "profile-stale", UpdatedAt: now,
			},
		},
		primaryID: "stale-brief-session",
	}
	repo := &staleInitialTaskBriefRepo{messageAddSwitchRepo: baseRepo, staleDescription: "Fresh task brief"}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	orch := &initialTaskBriefPromptOrchestrator{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
		expansions:                   map[string]string{},
	}
	h := NewMessageHandlers(svc, orch, log)
	request, err := ws.NewRequest("stale-brief-request", ws.ActionMessageAdd, map[string]any{
		"task_id": "stale-brief-task", "session_id": "stale-brief-session", "content": "follow the brief",
	})
	require.NoError(t, err)
	response, err := h.wsAddMessage(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Len(t, baseRepo.messages, 1)
	stored := baseRepo.firstMessageContent()
	require.Contains(t, stored, "Fresh task brief")
	require.NotContains(t, stored, "Original task brief")
	require.Contains(t, stored, "follow the brief")
	require.Equal(t, 1, orch.onTurnStartCount())
	require.Eventually(t, func() bool { return len(orch.started) == 1 }, time.Second, time.Millisecond)
	dispatched := <-orch.started
	require.Equal(t, stored, dispatched.content)
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestWSAddMessage_QueuedInitialTaskBriefPersistsAcceptedExpansion(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-initial-brief-queued": {
			ID: "task-initial-brief-queued", Description: "Implement @brief_rules",
			State: v1.TaskStateInProgress, UpdatedAt: now,
		}},
		sessions: map[string]*models.TaskSession{"session-initial-brief-queued": {
			ID: "session-initial-brief-queued", TaskID: "task-initial-brief-queued",
			State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-initial-brief-queued", UpdatedAt: now,
		}},
		primaryID: "session-initial-brief-queued",
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	orch := &initialTaskBriefPromptOrchestrator{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
		expansions:                   map[string]string{"brief_rules": "queued brief rules accepted"},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := NewMessageHandlers(svc, orch, log)
	req, err := ws.NewRequest("initial-brief-queued-request", ws.ActionMessageAdd, map[string]interface{}{
		"task_id": "task-initial-brief-queued", "session_id": "session-initial-brief-queued",
		"message_id": "initial-brief-queued-message", "content": "Keep the existing session",
		"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	resp, err := h.wsAddMessage(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	require.Len(t, repo.messages, 1)
	require.NotNil(t, repo.queuedMessage)
	require.Equal(t, repo.messages[0].Content, repo.queuedMessage.Content)
	expectedContext := "EXPANDED PROMPT REFERENCES:\n### @brief_rules\nqueued brief rules accepted"
	require.Contains(t, repo.queuedMessage.Content, sysprompt.Wrap(expectedContext))
	require.NotContains(t, repo.queuedMessage.Content, "\x00")
	require.Equal(t, 1, repo.promptHistoryCalls)
	require.True(t, orch.queuedNotified, "selected ready-session feedback is delivered from its durable queue row")
	require.Empty(t, orch.started)
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5
func TestWSAddMessage_QueuedReadyFollowupSkipsInitialTaskBrief(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-initial-brief-ready-queued-followup": {
			ID: "task-initial-brief-ready-queued-followup", Description: "Already accepted task brief",
			State: v1.TaskStateInProgress, UpdatedAt: now,
		}},
		sessions: map[string]*models.TaskSession{"session-initial-brief-ready-queued-followup": {
			ID: "session-initial-brief-ready-queued-followup", TaskID: "task-initial-brief-ready-queued-followup",
			State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-ready-queued-followup", UpdatedAt: now,
		}},
		primaryID: "session-initial-brief-ready-queued-followup", hasUserPromptHistory: true,
		messages: []*models.Message{{
			ID: "prior-ready-prompt", TaskID: "task-initial-brief-ready-queued-followup",
			TaskSessionID: "session-initial-brief-ready-queued-followup", AuthorType: models.MessageAuthorUser,
			Content: "Already accepted task brief", PromptIndex: 1,
		}},
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	orch := &initialTaskBriefPromptOrchestrator{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{started: make(chan capturedFirstTurn, 1)},
		expansions:                   map[string]string{},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	handler := NewMessageHandlers(svc, orch, log)
	request, err := ws.NewRequest("ready-followup-queued-request", ws.ActionMessageAdd, map[string]interface{}{
		"task_id":    "task-initial-brief-ready-queued-followup",
		"session_id": "session-initial-brief-ready-queued-followup",
		"message_id": "ready-followup-queued-message", "content": "Only send the follow-up instruction.",
		"plan_comment_refs":       []map[string]interface{}{{"id": "comment-handler", "version": 3}},
		"require_primary_session": true,
	})
	require.NoError(t, err)

	response, err := handler.wsAddMessage(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, repo.promptHistoryCalls)
	require.Len(t, repo.messages, 2)
	require.Contains(t, repo.messages[1].Content, "Only send the follow-up instruction.")
	require.Contains(t, repo.messages[1].Content, "stored feedback")
	require.Equal(t, 2, repo.messages[1].PromptIndex)
	require.NotContains(t, repo.messages[1].Content, "Already accepted task brief")
	require.NotNil(t, repo.queuedMessage)
	require.Equal(t, repo.messages[1].Content, repo.queuedMessage.Content)
	require.True(t, orch.queuedNotified)
	require.Empty(t, orch.started)
}
