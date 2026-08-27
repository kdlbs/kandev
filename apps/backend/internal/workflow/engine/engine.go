package engine

import (
	"context"
	"fmt"
	"maps"

	"github.com/kandev/kandev/internal/common/logger"
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
	// EntryID is the step-transition ledger row's own identifier for the
	// arrival being dispatched, set only by DispatchStepEntry. Callbacks
	// that need a durable, entry-scoped idempotency key (queue_run,
	// run_code_review) prefer this over OperationID — see idempotencyKey
	// in phase2_callbacks.go. Empty for every trigger other than step
	// entry.
	EntryID string
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
	// ApplyTransitionIfAtStep is the AC-46/48 compare-and-swap transition
	// commit used only by the AC-46 quorum re-evaluation apply path. It
	// applies the transition only when the task is still at expectedStepID,
	// reading and writing inside the same transaction so a concurrent mover
	// cannot race past the check. applied=false ("abandoned") means the task
	// had already left expectedStepID by the time this call ran — not an
	// error. applied=true means the task left expectedStepID (not
	// necessarily WIP-admitted at toStepID — a full step still counts as a
	// completed transition, queuing there). ApplyTransition (and every
	// existing caller of it) is unaffected: this is an additive method, not
	// a replacement.
	ApplyTransitionIfAtStep(ctx context.Context, taskID, sessionID, expectedStepID, toStepID string, trigger Trigger) (applied bool, err error)
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

	// EntryID mirrors ActionInput.EntryID — always empty for HandleTrigger /
	// HandleTriggerSessionShapedOnly callers. DispatchStepEntry populates
	// the ActionInput field directly through its own action loop, not
	// through this struct.
	EntryID string
}

// HandleResult summarizes engine work for a trigger.
type HandleResult struct {
	Transitioned bool
	FromStepID   string
	ToStepID     string
	// Guards is the read-only quorum state captured while evaluating a
	// guarded transition. Decision recording returns this snapshot even when
	// the successful evaluation moves the task to the next step.
	Guards      []QuorumGuardState
	DataPatch   map[string]any
	Idempotent  bool
	ActionCount int

	// TransitionAbandoned is true when a guarded transition's outcome was
	// satisfied but ApplyTransitionIfAtStep (AC-46/48) lost the race — the
	// task had already left FromStepID by the time the CAS ran. Distinct
	// from both a successful transition (Transitioned=true) and an error:
	// abandoning is an expected outcome of concurrent re-evaluation, not a
	// failure.
	TransitionAbandoned bool
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

// WithLogger wires the AC-24/24a structured log emitted when a guarded
// transition is evaluated and does not fire. The engine holds no logger
// today, so wiring one is part of this feature — without it, the engine
// still increments the quorummetrics counter, it just doesn't log.
func WithLogger(log *logger.Logger) Option {
	return func(e *Engine) { e.logger = log }
}

// WithAgentProfileResolver wires the quorum guard's AgentProfileResolver
// (REQ-OFFICE-REVIEW-SEATS-004.3). When unset, the guard counts every seat
// in the required slate unchanged, matching pre-existing behavior.
func WithAgentProfileResolver(resolver AgentProfileResolver) Option {
	return func(e *Engine) { e.agentProfiles = resolver }
}

// Engine evaluates step actions and applies transitions.
type Engine struct {
	store     TransitionStore
	callbacks CallbackRegistry

	// Phase 2 (ADR-0004) dependencies — nil-safe: an Engine wired only with
	// store + callbacks behaves identically to today's kanban engine.
	runQueue      RunQueueAdapter
	participants  ParticipantStore
	decisions     DecisionStore
	ceoResolver   CEOAgentResolver
	agentProfiles AgentProfileResolver
	// Phase 8 (ADR-0004) dependencies — also nil-safe.
	taskCreator      TaskCreator
	workflowSwitcher WorkflowSwitcher
	// logger is nil-safe (AC-24): *logger.Logger methods are not nil-safe
	// themselves, so every use is guarded by an explicit nil check.
	logger *logger.Logger
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

// HandleTrigger executes every action the trigger's step declares.
func (e *Engine) HandleTrigger(ctx context.Context, in HandleInput) (HandleResult, error) {
	return e.handleTrigger(ctx, in, nil)
}

// HandleTriggerSessionShapedOnly executes only the session-shaped subset of
// a step's declared on_enter actions (plan mode, session mode, auto-start
// agent, agent-context reset) — the kinds a route with a live arriving
// session already executes through this call. Session-independent kinds
// (clear_decisions, the participant fan-out, queue_run, run_code_review) are
// skipped here: they run exactly once, from the ledger-driven
// Engine.DispatchStepEntry path, never from this one. See
// docs/specs/office/system-design/step-entry-sequence-execution.md
// ("Exactly one component dispatches").
func (e *Engine) HandleTriggerSessionShapedOnly(ctx context.Context, in HandleInput) (HandleResult, error) {
	return e.handleTrigger(ctx, in, isSessionShapedActionKind)
}

// handleTrigger is HandleTrigger's implementation, parameterized by an
// optional action-kind filter. A nil filter executes every declared action,
// which is HandleTrigger's contract; every other trigger (on_turn_start,
// on_turn_complete, on_exit) always calls it with a nil filter, so their
// dispatch and abort-on-first-failure semantics are unchanged.
func (e *Engine) handleTrigger(ctx context.Context, in HandleInput, filter func(ActionKind) bool) (HandleResult, error) {
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

	result, err := e.processActions(ctx, in, state, step, actions, filter)
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
	filter func(ActionKind) bool,
) (HandleResult, error) {
	targetStepID, dataPatch, err := e.evaluateActions(ctx, in, state, step, actions, filter)
	if err != nil {
		return HandleResult{}, err
	}

	if len(dataPatch) > 0 && !in.EvaluateOnly {
		if err := e.store.PersistData(ctx, in.SessionID, dataPatch); err != nil {
			return HandleResult{}, err
		}
	}

	result := HandleResult{DataPatch: dataPatch, ActionCount: len(actions)}
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
	filter func(ActionKind) bool,
) (string, map[string]any, error) {
	var targetStepID string
	dataPatch := map[string]any{}
	for _, action := range actions {
		if filter != nil && !filter(action.Kind) {
			continue
		}
		if targetStepID == "" && isTransitionAction(action.Kind) && !action.RequiresApproval {
			outcome := e.evaluateTransitionGuard(ctx, state, action)
			if !outcome.Satisfied {
				e.recordGuardNotFired(state, action, outcome)
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
		EntryID:     in.EntryID,
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
