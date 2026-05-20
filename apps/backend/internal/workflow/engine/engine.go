package engine

import (
	"context"
	"fmt"
	"maps"
)

// MachineState captures runtime workflow state for a task session.
type MachineState struct {
	TaskID          string
	SessionID       string
	WorkflowID      string
	CurrentStepID   string
	SessionState    string
	TaskDescription string
	IsPassthrough   bool
	Data            map[string]any
}

// ActionInput is provided to action callbacks.
type ActionInput struct {
	Trigger Trigger
	State   MachineState
	Step    StepSpec
	Action  Action
	// Payload is the typed trigger payload (one of *Payload structs in
	// payloads.go). Nil for kanban-era triggers; callbacks for the new
	// Phase 2 triggers type-assert to the matching struct.
	Payload any
	// OperationID is the engine's idempotency key for this trigger
	// invocation. Callbacks that themselves need a deterministic
	// idempotency key (e.g. queue_run) should derive theirs from this
	// value combined with action-specific salt.
	OperationID string
}

// ActionResult communicates side effects back to the engine.
type ActionResult struct {
	DataPatch map[string]any
}

// ActionCallback executes side-effect actions.
type ActionCallback interface {
	Execute(ctx context.Context, in ActionInput) (ActionResult, error)
}

// CallbackRegistry resolves callbacks for action kinds.
type CallbackRegistry interface {
	Get(kind ActionKind) (ActionCallback, bool)
}

// TransitionStore abstracts persistence and transition commits.
type TransitionStore interface {
	LoadState(ctx context.Context, taskID, sessionID string) (MachineState, error)
	LoadStep(ctx context.Context, workflowID, stepID string) (StepSpec, error)
	LoadNextStep(ctx context.Context, workflowID string, currentPosition int) (StepSpec, error)
	LoadPreviousStep(ctx context.Context, workflowID string, currentPosition int) (StepSpec, error)
	ApplyTransition(ctx context.Context, taskID, sessionID, fromStepID, toStepID string, trigger Trigger) error
	PersistData(ctx context.Context, sessionID string, data map[string]any) error
	IsOperationApplied(ctx context.Context, operationID string) (bool, error)
	MarkOperationApplied(ctx context.Context, operationID string) error
}

// HandleInput is the input envelope for handling a workflow trigger.
type HandleInput struct {
	TaskID       string
	SessionID    string
	Trigger      Trigger
	OperationID  string
	EvaluateOnly bool // when true, skip ApplyTransition and PersistData; caller handles persistence

	// PreloadedState, when set, skips the LoadState call in the engine.
	// Use this to avoid redundant DB reads when the caller already loaded the session and task.
	PreloadedState *MachineState

	// Payload is the typed trigger payload — nil for kanban-era triggers.
	Payload any
}

// HandleResult summarizes engine work for a trigger.
type HandleResult struct {
	Transitioned bool
	FromStepID   string
	ToStepID     string
	DataPatch    map[string]any
	Idempotent   bool
}

// Option configures an Engine at construction time. Use With* helpers below.
type Option func(*Engine)

// WithRunQueue wires the engine's RunQueueAdapter for queue_run actions.
// When unset, queue_run callbacks return an error — kanban workflows that
// never use queue_run are unaffected.
func WithRunQueue(adapter RunQueueAdapter) Option {
	return func(e *Engine) { e.runQueue = adapter }
}

// WithParticipantStore wires read access to step participants.
func WithParticipantStore(store ParticipantStore) Option {
	return func(e *Engine) { e.participants = store }
}

// WithDecisionStore wires read/write access to step decisions.
func WithDecisionStore(store DecisionStore) Option {
	return func(e *Engine) { e.decisions = store }
}

// WithCEOAgentResolver wires resolution for the "workspace.ceo_agent"
// QueueRun target.
func WithCEOAgentResolver(resolver CEOAgentResolver) Option {
	return func(e *Engine) { e.ceoResolver = resolver }
}

// WithTaskCreator wires the engine's TaskCreator for the create_child_task
// action. When unset, create_child_task callbacks return ErrActionNotYetWired.
func WithTaskCreator(creator TaskCreator) Option {
	return func(e *Engine) { e.taskCreator = creator }
}

// WithWorkflowSwitcher wires the engine's WorkflowSwitcher for the
// switch_workflow action. When unset, switch_workflow callbacks return
// ErrActionNotYetWired.
func WithWorkflowSwitcher(switcher WorkflowSwitcher) Option {
	return func(e *Engine) { e.workflowSwitcher = switcher }
}

// Engine evaluates step actions and applies transitions.
type Engine struct {
	store     TransitionStore
	callbacks CallbackRegistry

	// Phase 2 (ADR-0004) dependencies — nil-safe: an Engine wired only with
	// store + callbacks behaves identically to today's kanban engine.
	runQueue     RunQueueAdapter
	participants ParticipantStore
	decisions    DecisionStore
	ceoResolver  CEOAgentResolver
	// Phase 8 (ADR-0004) dependencies — also nil-safe.
	taskCreator      TaskCreator
	workflowSwitcher WorkflowSwitcher
}

// TaskCreatorAdapter exposes the wired TaskCreator (or nil if unset).
// Used by callbacks that resolve TaskCreator at registration time.
func (e *Engine) TaskCreatorAdapter() TaskCreator { return e.taskCreator }

// WorkflowSwitcherAdapter exposes the wired WorkflowSwitcher (or nil if
// unset). Used by callbacks that resolve the switcher at registration
// time.
func (e *Engine) WorkflowSwitcherAdapter() WorkflowSwitcher { return e.workflowSwitcher }

// New creates a workflow engine. Phase 2 dependencies (RunQueueAdapter,
// ParticipantStore, DecisionStore, CEOAgentResolver) are wired via Option.
// Without them, the engine still serves today's kanban workflows: queue_run
// and friends are inert because no kanban step references them.
func New(store TransitionStore, callbacks CallbackRegistry, opts ...Option) *Engine {
	e := &Engine{store: store, callbacks: callbacks}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// HandleTrigger executes actions for the provided trigger.
func (e *Engine) HandleTrigger(ctx context.Context, in HandleInput) (HandleResult, error) {
	if in.TaskID == "" || in.SessionID == "" {
		return HandleResult{}, fmt.Errorf("task_id and session_id are required")
	}
	idempotent, err := e.isOperationAlreadyApplied(ctx, in.OperationID)
	if err != nil {
		return HandleResult{}, err
	}
	if idempotent {
		return HandleResult{Idempotent: true}, nil
	}

	state, step, err := e.loadExecutionContext(ctx, in)
	if err != nil {
		return HandleResult{}, err
	}

	actions := step.Events[in.Trigger]
	if len(actions) == 0 {
		return HandleResult{}, e.markOperationApplied(ctx, in.OperationID)
	}

	result, err := e.processActions(ctx, in, state, step, actions)
	if err != nil {
		return HandleResult{}, err
	}

	return result, e.markOperationApplied(ctx, in.OperationID)
}

// processActions evaluates actions, persists data, and applies transitions.
func (e *Engine) processActions(
	ctx context.Context,
	in HandleInput,
	state MachineState,
	step StepSpec,
	actions []Action,
) (HandleResult, error) {
	targetStepID, dataPatch, err := e.evaluateActions(ctx, in, state, step, actions)
	if err != nil {
		return HandleResult{}, err
	}

	if len(dataPatch) > 0 && !in.EvaluateOnly {
		if err := e.store.PersistData(ctx, in.SessionID, dataPatch); err != nil {
			return HandleResult{}, err
		}
	}

	result := HandleResult{DataPatch: dataPatch}
	if targetStepID != "" && targetStepID != state.CurrentStepID {
		if !in.EvaluateOnly {
			if err := e.applyTransition(ctx, in, state, targetStepID); err != nil {
				return HandleResult{}, err
			}
		}
		result.Transitioned = true
		result.FromStepID = state.CurrentStepID
		result.ToStepID = targetStepID
	}

	return result, nil
}

func (e *Engine) isOperationAlreadyApplied(ctx context.Context, operationID string) (bool, error) {
	if operationID == "" {
		return false, nil
	}
	return e.store.IsOperationApplied(ctx, operationID)
}

func (e *Engine) markOperationApplied(ctx context.Context, operationID string) error {
	if operationID == "" {
		return nil
	}
	return e.store.MarkOperationApplied(ctx, operationID)
}

func (e *Engine) loadExecutionContext(ctx context.Context, in HandleInput) (MachineState, StepSpec, error) {
	var state MachineState
	if in.PreloadedState != nil {
		state = *in.PreloadedState
	} else {
		var err error
		state, err = e.store.LoadState(ctx, in.TaskID, in.SessionID)
		if err != nil {
			return MachineState{}, StepSpec{}, err
		}
	}
	step, err := e.store.LoadStep(ctx, state.WorkflowID, state.CurrentStepID)
	if err != nil {
		return MachineState{}, StepSpec{}, err
	}
	return state, step, nil
}

func (e *Engine) evaluateActions(
	ctx context.Context,
	in HandleInput,
	state MachineState,
	step StepSpec,
	actions []Action,
) (string, map[string]any, error) {
	var targetStepID string
	dataPatch := map[string]any{}
	for _, action := range actions {
		if targetStepID == "" && isTransitionAction(action.Kind) && !action.RequiresApproval {
			permitted, err := e.evaluateTransitionGuard(ctx, state, action)
			if err != nil {
				return "", nil, err
			}
			if !permitted {
				continue
			}
			resolvedTarget, err := e.resolveTransitionTarget(ctx, state, step, action)
			if err != nil {
				return "", nil, err
			}
			targetStepID = resolvedTarget
			continue
		}
		if err := e.executeCallback(ctx, in, state, step, action, dataPatch); err != nil {
			return "", nil, err
		}
	}
	return targetStepID, dataPatch, nil
}

func (e *Engine) applyTransition(ctx context.Context, in HandleInput, state MachineState, targetStepID string) error {
	return e.store.ApplyTransition(ctx, in.TaskID, in.SessionID, state.CurrentStepID, targetStepID, in.Trigger)
}

func (e *Engine) executeCallback(
	ctx context.Context,
	in HandleInput,
	state MachineState,
	step StepSpec,
	action Action,
	dataPatch map[string]any,
) error {
	callback, ok := e.callbacks.Get(action.Kind)
	if !ok {
		return nil
	}
	res, err := callback.Execute(ctx, ActionInput{
		Trigger:     in.Trigger,
		State:       state,
		Step:        step,
		Action:      action,
		Payload:     in.Payload,
		OperationID: in.OperationID,
	})
	if err != nil {
		return err
	}
	maps.Copy(dataPatch, res.DataPatch)
	return nil
}

func (e *Engine) resolveTransitionTarget(ctx context.Context, state MachineState, step StepSpec, action Action) (string, error) {
	switch action.Kind {
	case ActionMoveToNext:
		next, err := e.store.LoadNextStep(ctx, state.WorkflowID, step.Position)
		if err != nil {
			return "", err
		}
		return next.ID, nil
	case ActionMoveToPrevious:
		prev, err := e.store.LoadPreviousStep(ctx, state.WorkflowID, step.Position)
		if err != nil {
			return "", err
		}
		return prev.ID, nil
	case ActionMoveToStep:
		if action.MoveToStep == nil || action.MoveToStep.StepID == "" {
			return "", fmt.Errorf("move_to_step missing target step_id")
		}
		return action.MoveToStep.StepID, nil
	default:
		return "", nil
	}
}

func isTransitionAction(kind ActionKind) bool {
	return kind == ActionMoveToNext || kind == ActionMoveToPrevious || kind == ActionMoveToStep
}
