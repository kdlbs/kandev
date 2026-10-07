package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type workflowStartPromptAttemptContextKey struct{}

type workflowStartPromptAttemptState uint32

const (
	workflowStartPromptAttemptActive workflowStartPromptAttemptState = iota + 1
	workflowStartPromptAttemptRetired
	workflowStartPromptAttemptPreservationClaimed
)

type workflowStartPromptAttemptClaim struct {
	state atomic.Uint32
}

// workflowStartPromptAttempt is the immutable input snapshot for one workflow
// start. claim is shared by context copies made when the initial turn is bound,
// so a callback can settle the logical launch exactly once.
type workflowStartPromptAttempt struct {
	claim *workflowStartPromptAttemptClaim

	taskID                string
	sessionID             string
	workflowStep          workflowMessageOrigin
	launchToken           string
	turnID                string
	queueIdentity         messagequeue.QueueSessionIdentity
	workflowEntry         messagequeue.WorkflowEntryIdentity
	workflowEntryRequired bool
	workflowEntryCaptured bool
	configMode            bool

	prompt               string
	planMode             bool
	dispatchInputPresent bool
	attachments          []v1.MessageAttachment
	references           []v1.EntityReference
	handoffText          string
	userMessageRecorded  bool
}

func newWorkflowStartPromptAttempt(
	taskID, sessionID string,
	origin workflowMessageOrigin,
	prompt string,
	planMode bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
	handoffText string,
	userMessageRecorded bool,
) *workflowStartPromptAttempt {
	attempt := newWorkflowStartPromptAttemptWithAdmission(
		taskID, sessionID, origin, prompt, planMode, attachments, references,
		handoffText, userMessageRecorded,
		strings.TrimSpace(prompt) != "" || planMode || len(attachments) > 0 || len(references) > 0 || strings.TrimSpace(handoffText) != "",
		messagequeue.QueueSessionIdentity{}, messagequeue.WorkflowEntryIdentity{}, false,
	)
	attempt.workflowEntryRequired = false
	return attempt
}

func newWorkflowStartPromptAttemptWithAdmission(
	taskID, sessionID string,
	origin workflowMessageOrigin,
	prompt string,
	planMode bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
	handoffText string,
	userMessageRecorded bool,
	dispatchInputPresent bool,
	queueIdentity messagequeue.QueueSessionIdentity,
	workflowEntry messagequeue.WorkflowEntryIdentity,
	workflowEntryCaptured bool,
) *workflowStartPromptAttempt {
	attempt := &workflowStartPromptAttempt{
		claim:                 &workflowStartPromptAttemptClaim{},
		taskID:                taskID,
		sessionID:             sessionID,
		workflowStep:          origin,
		launchToken:           uuid.NewString(),
		queueIdentity:         queueIdentity,
		workflowEntry:         workflowEntry,
		workflowEntryRequired: true,
		workflowEntryCaptured: workflowEntryCaptured,
		prompt:                prompt,
		planMode:              planMode,
		dispatchInputPresent:  dispatchInputPresent,
		attachments:           append([]v1.MessageAttachment(nil), attachments...),
		references:            append([]v1.EntityReference(nil), references...),
		handoffText:           handoffText,
		userMessageRecorded:   userMessageRecorded,
	}
	attempt.claim.state.Store(uint32(workflowStartPromptAttemptActive))
	return attempt
}

func withWorkflowStartPromptAttempt(ctx context.Context, attempt *workflowStartPromptAttempt) context.Context {
	if ctx == nil || attempt == nil {
		return ctx
	}
	return context.WithValue(ctx, workflowStartPromptAttemptContextKey{}, attempt)
}

func workflowStartPromptAttemptFromContext(ctx context.Context) *workflowStartPromptAttempt {
	if ctx == nil {
		return nil
	}
	attempt, _ := ctx.Value(workflowStartPromptAttemptContextKey{}).(*workflowStartPromptAttempt)
	return attempt
}

func bindWorkflowStartPromptAttemptTurn(ctx context.Context, turnID string) context.Context {
	attempt := workflowStartPromptAttemptFromContext(ctx)
	if attempt == nil || attempt.turnID == turnID {
		return ctx
	}
	bound := *attempt
	bound.turnID = turnID
	return withWorkflowStartPromptAttempt(ctx, &bound)
}

func (a *workflowStartPromptAttempt) retire() {
	if a == nil || a.claim == nil {
		return
	}
	a.claim.state.CompareAndSwap(
		uint32(workflowStartPromptAttemptActive),
		uint32(workflowStartPromptAttemptRetired),
	)
}

func (a *workflowStartPromptAttempt) claimPreservation() bool {
	if a == nil || a.claim == nil {
		return false
	}
	return a.claim.state.CompareAndSwap(
		uint32(workflowStartPromptAttemptActive),
		uint32(workflowStartPromptAttemptPreservationClaimed),
	)
}

// releasePreservationClaim makes a failed queue admission retryable for this
// launch attempt. The claim is shared by every callback context, so a later
// callback can retry only while it still owns the same attempt.
func (a *workflowStartPromptAttempt) releasePreservationClaim() bool {
	if a == nil || a.claim == nil {
		return false
	}
	return a.claim.state.CompareAndSwap(
		uint32(workflowStartPromptAttemptPreservationClaimed),
		uint32(workflowStartPromptAttemptActive),
	)
}

func (a *workflowStartPromptAttempt) hasInput() bool {
	return a != nil && (a.dispatchInputPresent || len(a.attachments) > 0 ||
		len(a.references) > 0 || strings.TrimSpace(a.handoffText) != "")
}

type workflowStartPromptTransitionReader interface {
	GetLatestTaskStepTransitionID(context.Context, string) (int64, error)
}

type workflowStartPromptTransitionEnsurer interface {
	EnsureCurrentTaskStepTransition(context.Context, string) (int64, error)
}

type workflowStartPromptInputSnapshot struct {
	dispatchInputPresent      bool
	dispatchInputPresentKnown bool
	configMode                bool
	configModeKnown           bool
}

func (s workflowStartPromptInputSnapshot) dispatchInputPresentPtr() *bool {
	if !s.dispatchInputPresentKnown {
		return nil
	}
	present := s.dispatchInputPresent
	return &present
}

func (s workflowStartPromptInputSnapshot) configModePtr() *bool {
	if !s.configModeKnown {
		return nil
	}
	configMode := s.configMode
	return &configMode
}

// workflowStartPromptInputSnapshot records actionability at queue admission.
// A raw prompt can be empty while plan mode, config mode, references, or a
// handoff still produces a real dispatch. When session metadata cannot be
// read, only independently known raw input is stamped; the queue drain then
// retains its retryable read-error behavior instead of guessing empty input.
func (s *Service) workflowStartPromptInputSnapshot(
	ctx context.Context,
	sessionID, prompt string,
	planMode bool,
	attachments []v1.MessageAttachment,
	references []v1.EntityReference,
	handoffText string,
) workflowStartPromptInputSnapshot {
	snapshot := workflowStartPromptInputSnapshot{
		dispatchInputPresent: strings.TrimSpace(prompt) != "" || planMode ||
			len(attachments) > 0 || len(references) > 0 || strings.TrimSpace(handoffText) != "",
		dispatchInputPresentKnown: false,
	}
	if snapshot.dispatchInputPresent {
		snapshot.dispatchInputPresentKnown = true
	}
	if s.repo == nil {
		return snapshot
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return snapshot
	}
	snapshot.configMode, _ = session.Metadata["config_mode"].(bool)
	snapshot.configModeKnown = true
	snapshot.dispatchInputPresent = snapshot.dispatchInputPresent || snapshot.configMode
	snapshot.dispatchInputPresentKnown = true
	return snapshot
}

// captureWorkflowStartPromptAdmission records the task-step transition,
// session incarnation, and purge generation observed before the launch. The
// queue repository rechecks all three inside its insertion transaction.
func (s *Service) captureWorkflowStartPromptAdmission(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
) (messagequeue.QueueSessionIdentity, messagequeue.WorkflowEntryIdentity, bool, error) {
	if s.messageQueue == nil || !s.messageQueue.SupportsAtomicDeferredMoveTransition() {
		// Focused and legacy backends without the shared SQL queue/task
		// transaction intentionally use the existing non-fenced recovery path.
		return messagequeue.QueueSessionIdentity{}, messagequeue.WorkflowEntryIdentity{}, false, nil
	}
	if session == nil || taskID == "" || session.ID == "" || session.QueueIncarnationID == "" {
		return messagequeue.QueueSessionIdentity{}, messagequeue.WorkflowEntryIdentity{}, true,
			fmt.Errorf("workflow launch admission identity is incomplete")
	}
	reader, ok := s.repo.(workflowStartPromptTransitionReader)
	if !ok {
		return messagequeue.QueueSessionIdentity{}, messagequeue.WorkflowEntryIdentity{}, true,
			fmt.Errorf("workflow launch admission transition reader is unavailable")
	}
	task, transitionID, err := s.captureWorkflowStartPromptTaskAndEntry(ctx, taskID, reader)
	if err != nil {
		return messagequeue.QueueSessionIdentity{}, messagequeue.WorkflowEntryIdentity{}, true, err
	}
	generation := int64(0)
	if s.messageQueue != nil {
		generation, err = s.messageQueue.LifecycleGeneration(ctx, taskID)
		if err != nil {
			return messagequeue.QueueSessionIdentity{}, messagequeue.WorkflowEntryIdentity{}, true,
				fmt.Errorf("read lifecycle generation for workflow launch admission: %w", err)
		}
	}
	return messagequeue.QueueSessionIdentity{
			TaskID: taskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID,
		}, messagequeue.WorkflowEntryIdentity{
			WorkflowID: task.WorkflowID, WorkflowStepID: task.WorkflowStepID,
			TransitionID: transitionID, LifecycleGeneration: generation,
		}, true, nil
}

func (s *Service) captureWorkflowStartPromptTaskAndEntry(
	ctx context.Context,
	taskID string,
	reader workflowStartPromptTransitionReader,
) (*models.Task, int64, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, 0, fmt.Errorf("read task for workflow launch admission: %w", err)
	}
	if task == nil {
		return nil, 0, fmt.Errorf("task %q is missing for workflow launch admission", taskID)
	}
	transitionID, err := reader.GetLatestTaskStepTransitionID(ctx, taskID)
	if err != nil {
		return nil, 0, fmt.Errorf("read workflow launch entry for task %q: %w", taskID, err)
	}
	if transitionID <= 0 {
		ensurer, ok := s.repo.(workflowStartPromptTransitionEnsurer)
		if !ok {
			return nil, 0, fmt.Errorf("workflow launch admission cannot establish a durable entry for task %q", taskID)
		}
		transitionID, err = ensurer.EnsureCurrentTaskStepTransition(ctx, taskID)
		if err != nil {
			return nil, 0, fmt.Errorf("establish workflow launch entry for task %q: %w", taskID, err)
		}
		if transitionID <= 0 {
			return nil, 0, fmt.Errorf("task %q has no current workflow entry", taskID)
		}
	}
	// The entry read and task projection must describe one durable state. A
	// move may have committed between the first task read and the transition
	// read, so reload after either the normal read or a legacy backfill.
	task, err = s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, 0, fmt.Errorf("reload task after reading workflow launch entry: %w", err)
	}
	if task == nil {
		return nil, 0, fmt.Errorf("task %q disappeared after reading workflow launch entry", taskID)
	}
	latestTransitionID, err := reader.GetLatestTaskStepTransitionID(ctx, taskID)
	if err != nil {
		return nil, 0, fmt.Errorf("re-read workflow launch entry for task %q: %w", taskID, err)
	}
	if latestTransitionID <= 0 {
		return nil, 0, fmt.Errorf("task %q lost its current workflow entry", taskID)
	}
	return task, latestTransitionID, nil
}

func (s *Service) retireWorkflowStartPromptAttempt(ctx context.Context, taskID, sessionID, executionID string) {
	attempt := workflowStartPromptAttemptFromContext(ctx)
	if attempt == nil || attempt.taskID != taskID || attempt.sessionID != sessionID {
		return
	}
	if executionID != "" && s.agentManager != nil {
		liveExecutionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
		if err == nil && liveExecutionID != "" && liveExecutionID != executionID {
			return
		}
	}
	attempt.retire()
}

// preserveWorkflowStartPromptAfterFailure is called while the session's
// cancellation guard is held and before the executor projects the startup
// failure. It validates the captured ownership boundary, claims the attempt,
// and writes the existing queue form without scheduling a new resume.
func (s *Service) preserveWorkflowStartPromptAfterFailure(
	ctx context.Context,
	taskID, sessionID, executionID string,
) {
	attempt := workflowStartPromptAttemptFromContext(ctx)
	if attempt == nil || attempt.taskID != taskID || attempt.sessionID != sessionID || !attempt.hasInput() {
		return
	}
	if attempt.workflowEntryRequired && !attempt.workflowEntryCaptured {
		return
	}
	if !s.workflowStartPromptAttemptIsCurrent(ctx, attempt, executionID) {
		return
	}
	if !attempt.claimPreservation() {
		return
	}
	err := s.persistWorkflowStartPromptAttempt(ctx, attempt)
	if err != nil && s.workflowStartPromptAttemptIsCurrent(ctx, attempt, executionID) {
		// Queue admission can fail before the transaction writes a row. Retry
		// once while this callback still owns the immutable launch attempt. The
		// attempt claim prevents duplicate callbacks from racing this retry.
		err = s.persistWorkflowStartPromptAttempt(ctx, attempt)
	}
	if err != nil {
		attempt.releasePreservationClaim()
		// Keep the startup error as the user-visible recovery signal. This
		// diagnostic carries only ownership identifiers, never prompt content.
		s.logger.Warn("failed to preserve workflow prompt after asynchronous startup failure",
			zap.String("task_id", attempt.taskID),
			zap.String("session_id", attempt.sessionID),
			zap.String("agent_execution_id", executionID),
			zap.String("launch_token", attempt.launchToken),
			zap.Error(err))
	}
}

func (s *Service) persistWorkflowStartPromptAttempt(
	ctx context.Context,
	attempt *workflowStartPromptAttempt,
) error {
	if attempt == nil {
		return fmt.Errorf("workflow start prompt attempt is nil")
	}
	var err error
	if attempt.workflowEntryCaptured {
		_, err = s.persistAutoStartPromptAtWorkflowEntry(
			ctx,
			attempt.taskID,
			attempt.sessionID,
			attempt.prompt,
			attempt.planMode,
			attempt.attachments,
			attempt.workflowStep,
			attempt.userMessageRecorded,
			attempt.references,
			attempt.handoffText,
			attempt.dispatchInputPresent,
			attempt.configMode,
			attempt.queueIdentity,
			attempt.workflowEntry,
		)
	} else {
		_, err = s.persistAutoStartPrompt(
			ctx,
			attempt.taskID,
			attempt.sessionID,
			attempt.prompt,
			attempt.planMode,
			attempt.attachments,
			attempt.workflowStep,
			attempt.userMessageRecorded,
			attempt.references,
			attempt.handoffText,
		)
	}
	return err
}

func (s *Service) workflowStartPromptAttemptIsCurrent(
	ctx context.Context,
	attempt *workflowStartPromptAttempt,
	executionID string,
) bool {
	if attempt == nil || executionID == "" || s.repo == nil {
		return false
	}
	if !s.workflowStartPromptSessionIsCurrent(ctx, attempt, executionID) {
		return false
	}
	if !s.workflowStartPromptExecutionIsCurrent(ctx, attempt.sessionID, executionID) {
		return false
	}
	if !s.workflowStartPromptTaskIsCurrent(ctx, attempt) {
		return false
	}
	if !s.workflowStartPromptEntryIsCurrent(ctx, attempt) {
		return false
	}
	return s.workflowStartPromptTurnIsCurrent(ctx, attempt)
}

func (s *Service) workflowStartPromptSessionIsCurrent(
	ctx context.Context,
	attempt *workflowStartPromptAttempt,
	executionID string,
) bool {
	session, err := s.repo.GetTaskSession(ctx, attempt.sessionID)
	if err != nil || session == nil || session.TaskID != attempt.taskID {
		return false
	}
	if session.State != models.TaskSessionStateStarting {
		return false
	}
	return session.AgentExecutionID == "" || session.AgentExecutionID == executionID
}

func (s *Service) workflowStartPromptExecutionIsCurrent(ctx context.Context, sessionID, executionID string) bool {
	if s.agentManager == nil {
		return false
	}
	liveExecutionID, err := s.agentManager.GetExecutionIDForSession(ctx, sessionID)
	return err == nil && liveExecutionID != "" && liveExecutionID == executionID
}

func (s *Service) workflowStartPromptTaskIsCurrent(ctx context.Context, attempt *workflowStartPromptAttempt) bool {
	task, err := s.repo.GetTask(ctx, attempt.taskID)
	if err != nil || task == nil || taskArchived(task) {
		return false
	}
	if attempt.workflowStep.StepID != "" && task.WorkflowStepID != attempt.workflowStep.StepID {
		return false
	}
	if attempt.workflowEntryCaptured && (task.WorkflowID != attempt.workflowEntry.WorkflowID ||
		task.WorkflowStepID != attempt.workflowEntry.WorkflowStepID) {
		return false
	}
	return true
}

func (s *Service) workflowStartPromptEntryIsCurrent(ctx context.Context, attempt *workflowStartPromptAttempt) bool {
	if !attempt.workflowEntryCaptured {
		return true
	}
	reader, ok := s.repo.(workflowStartPromptTransitionReader)
	if !ok {
		return false
	}
	transitionID, err := reader.GetLatestTaskStepTransitionID(ctx, attempt.taskID)
	if err != nil || transitionID != attempt.workflowEntry.TransitionID {
		return false
	}
	if s.messageQueue == nil {
		return true
	}
	generation, err := s.messageQueue.LifecycleGeneration(ctx, attempt.taskID)
	return err == nil && generation == attempt.workflowEntry.LifecycleGeneration
}

func (s *Service) workflowStartPromptTurnIsCurrent(ctx context.Context, attempt *workflowStartPromptAttempt) bool {
	if attempt.turnID == "" || s.turnService == nil {
		return false
	}
	activeTurnID, err := s.peekActiveTurnID(ctx, attempt.sessionID)
	return err == nil && activeTurnID == attempt.turnID
}
