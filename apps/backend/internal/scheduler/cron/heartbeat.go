package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// onHeartbeatKey is the JSON key the engine will look up on a step's
// events map for heartbeat actions. Phase 6 lands template authoring
// that writes this key; in Phase 5 the key may simply not be present
// on any step, in which case the heartbeat handler is a silent no-op.
const onHeartbeatKey = "on_heartbeat"

// defaultHeartbeatCadence is the floor cadence used when a step has
// on_heartbeat configured but no explicit cadence_seconds. 60s matches
// the plan's "Standing step" default for coordination tasks.
const defaultHeartbeatCadence = 60 * time.Second

// HeartbeatStepInfo is a lightweight projection of one workflow step
// whose events JSON includes on_heartbeat. The cron handler treats the
// raw JSON as opaque — only the cadence_seconds key is interpreted at
// this layer; everything else is the engine's concern when it
// evaluates the trigger.
type HeartbeatStepInfo struct {
	StepID         string
	WorkflowID     string
	CadenceSeconds int
}

// HeartbeatTaskInfo is the minimum the heartbeat handler needs about
// each candidate task. It is intentionally narrower than the full
// task.models.Task so the handler can be tested without the SQLite
// task repository.
type HeartbeatTaskInfo struct {
	TaskID                 string
	WorkflowStepID         string
	AssigneeAgentProfileID string
}

// HeartbeatStepLister returns workflow steps whose events JSON includes
// the on_heartbeat key. Implementations typically use a SQL LIKE
// filter so the handler only iterates relevant steps; an in-memory
// implementation may scan all steps.
type HeartbeatStepLister interface {
	ListHeartbeatSteps(ctx context.Context) ([]HeartbeatStepInfo, error)
}

// HeartbeatTaskLister returns the active (non-archived) tasks at a
// given workflow step. Used after the step lister has narrowed the
// search to steps with on_heartbeat configured.
type HeartbeatTaskLister interface {
	ListActiveTasksAtStep(ctx context.Context, stepID string) ([]HeartbeatTaskInfo, error)
}

// HeartbeatAgentRuntime answers "may I wake this agent now?" for the
// heartbeat gate. AllowFire returns true when the agent is permitted
// to receive a new heartbeat-driven run: status is not paused/stopped,
// cooldown_sec since last_run_finished_at has elapsed, and any other
// runtime override (e.g. a workspace-level pause) is satisfied.
//
// The gate is intentionally separate from the runs scheduler's own
// guards — we want to skip even queueing a run when the agent is
// known to be unavailable, so the queue stays free of stale entries.
type HeartbeatAgentRuntime interface {
	AllowFire(ctx context.Context, agentInstanceID string, now time.Time) (bool, error)
}

// HeartbeatEngineDispatcher fires engine.TriggerOnHeartbeat for the
// (task, step) pair. Implementations resolve the active session id
// (engine state is keyed on session id) and forward to
// engine.HandleTrigger.
//
// Mirrors office/service.WorkflowEngineDispatcher to keep wiring
// trivial — the office's existing engine_dispatcher.Dispatcher
// satisfies this interface unchanged.
type HeartbeatEngineDispatcher interface {
	HandleTrigger(
		ctx context.Context,
		taskID string,
		trigger engine.Trigger,
		payload any,
		operationID string,
	) error
}

// HeartbeatHandler implements Handler for engine.TriggerOnHeartbeat.
type HeartbeatHandler struct {
	steps      HeartbeatStepLister
	tasks      HeartbeatTaskLister
	runtime    HeartbeatAgentRuntime
	dispatcher HeartbeatEngineDispatcher
	now        func() time.Time
	log        *logger.Logger

	// lastFiredAt tracks the last time this process fired
	// on_heartbeat for a given (task, step) pair. The map is
	// process-local and bounded by the number of active heartbeat
	// pairs, so its memory cost is negligible. On restart it resets
	// — at worst a single extra heartbeat fires per pair, which the
	// engine's idempotency layer absorbs via OperationID.
	lastFiredAt map[string]time.Time
}

// NewHeartbeatHandler builds a HeartbeatHandler. Any nil dependency
// makes Tick a no-op for that branch — useful so the cron loop can
// start before every collaborator is wired.
func NewHeartbeatHandler(
	steps HeartbeatStepLister,
	tasks HeartbeatTaskLister,
	runtime HeartbeatAgentRuntime,
	dispatcher HeartbeatEngineDispatcher,
	now func() time.Time,
	log *logger.Logger,
) *HeartbeatHandler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &HeartbeatHandler{
		steps:       steps,
		tasks:       tasks,
		runtime:     runtime,
		dispatcher:  dispatcher,
		now:         now,
		log:         log.WithFields(zap.String("handler", "heartbeat")),
		lastFiredAt: make(map[string]time.Time),
	}
}

// Name implements Handler.
func (h *HeartbeatHandler) Name() string { return "heartbeat" }

// Tick implements Handler. The flow:
//
//  1. List all workflow steps with an on_heartbeat trigger.
//  2. For each step: list active tasks, then fire engine trigger for
//     each task whose runtime gate permits and whose cadence has
//     elapsed since the last in-process fire.
//
// Errors per step / task are logged but never abort the rest of the
// pass — one bad task should not block the rest of the workspace.
func (h *HeartbeatHandler) Tick(ctx context.Context) error {
	if !h.ready() {
		return nil
	}
	steps, err := h.steps.ListHeartbeatSteps(ctx)
	if err != nil {
		return fmt.Errorf("list heartbeat steps: %w", err)
	}
	if len(steps) == 0 {
		return nil
	}
	now := h.now()
	for i := range steps {
		h.tickStep(ctx, &steps[i], now)
	}
	return nil
}

// ready reports whether every collaborator is wired. A handler with
// missing pieces returns true from Name() but does nothing on Tick.
func (h *HeartbeatHandler) ready() bool {
	return h.steps != nil && h.tasks != nil &&
		h.runtime != nil && h.dispatcher != nil
}

// tickStep iterates active tasks at the step and fires heartbeats
// where the cadence and runtime gates permit.
func (h *HeartbeatHandler) tickStep(ctx context.Context, step *HeartbeatStepInfo, now time.Time) {
	tasks, err := h.tasks.ListActiveTasksAtStep(ctx, step.StepID)
	if err != nil {
		h.log.Warn("list tasks at heartbeat step failed",
			zap.String("step_id", step.StepID), zap.Error(err))
		return
	}
	cadence := stepCadence(step.CadenceSeconds)
	for _, task := range tasks {
		if !h.shouldFire(ctx, task, step.StepID, cadence, now) {
			continue
		}
		h.fireHeartbeat(ctx, task, step.StepID, now)
	}
}

// shouldFire checks the cadence floor + agent-runtime gate. Returns
// true only when both pass.
func (h *HeartbeatHandler) shouldFire(
	ctx context.Context,
	task HeartbeatTaskInfo,
	stepID string,
	cadence time.Duration,
	now time.Time,
) bool {
	key := pairKey(task.TaskID, stepID)
	if last, ok := h.lastFiredAt[key]; ok && now.Sub(last) < cadence {
		return false
	}
	if task.AssigneeAgentProfileID == "" {
		return false
	}
	allow, err := h.runtime.AllowFire(ctx, task.AssigneeAgentProfileID, now)
	if err != nil {
		h.log.Warn("agent runtime gate failed",
			zap.String("task_id", task.TaskID),
			zap.String("agent_id", task.AssigneeAgentProfileID),
			zap.Error(err))
		return false
	}
	return allow
}

// fireHeartbeat dispatches engine.TriggerOnHeartbeat and records the
// fire timestamp for the (task, step) pair. The operation id keeps
// engine idempotency in seconds resolution — two fires within a
// single second collapse into one engine evaluation.
func (h *HeartbeatHandler) fireHeartbeat(
	ctx context.Context,
	task HeartbeatTaskInfo,
	stepID string,
	now time.Time,
) {
	opID := fmt.Sprintf("heartbeat:%s:%s:%d", task.TaskID, stepID, now.Unix())
	err := h.dispatcher.HandleTrigger(ctx,
		task.TaskID,
		engine.TriggerOnHeartbeat,
		engine.OnHeartbeatPayload{},
		opID,
	)
	if err != nil {
		// ErrNoSession from the dispatcher is expected for tasks
		// that have not yet started a session — the cadence
		// timestamp still moves forward so we don't spam the log
		// every tick.
		h.log.Debug("heartbeat trigger dispatch failed",
			zap.String("task_id", task.TaskID),
			zap.String("step_id", stepID),
			zap.Error(err))
	}
	h.lastFiredAt[pairKey(task.TaskID, stepID)] = now
}

// stepCadence returns the configured cadence or the default when the
// step did not specify one. Negative / zero values fall back to the
// default to keep one bad config from busy-looping the engine.
func stepCadence(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultHeartbeatCadence
	}
	return time.Duration(seconds) * time.Second
}

// pairKey is the in-process map key for cadence tracking. Using
// "task|step" rather than concatenating ids prevents collision when
// either id contains a colon.
func pairKey(taskID, stepID string) string {
	return taskID + "|" + stepID
}

// ParseCadenceFromEvents extracts the cadence_seconds field from a
// step's raw events JSON. Returns 0 when the field is missing or
// malformed; callers MUST treat 0 as "use default".
//
// Exposed at package level so the office step lister adapter can call
// it once per step row instead of duplicating JSON parsing logic.
//
// Expected shape:
//
//	{
//	    "on_heartbeat": [
//	        {"type": "queue_run", "config": {"cadence_seconds": 60, ...}}
//	    ]
//	}
//
// The cadence is read from the first heartbeat action's config; mixing
// multiple heartbeat actions on a single step is unusual but the
// scheduler's behaviour is well-defined: only the first action's
// cadence is honoured.
func ParseCadenceFromEvents(eventsJSON string) int {
	if eventsJSON == "" {
		return 0
	}
	if !strings.Contains(eventsJSON, onHeartbeatKey) {
		return 0
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(eventsJSON), &raw); err != nil {
		return 0
	}
	hb, ok := raw[onHeartbeatKey]
	if !ok {
		return 0
	}
	var actions []struct {
		Config map[string]any `json:"config"`
	}
	if err := json.Unmarshal(hb, &actions); err != nil {
		return 0
	}
	for _, a := range actions {
		if v, ok := a.Config["cadence_seconds"]; ok {
			return coerceInt(v)
		}
	}
	return 0
}

// HasHeartbeatTrigger reports whether a step's raw events JSON
// configures the on_heartbeat trigger. Cheap pre-filter for the SQL
// adapter so the in-memory parser only runs against rows that already
// matched a LIKE pattern.
func HasHeartbeatTrigger(eventsJSON string) bool {
	if eventsJSON == "" {
		return false
	}
	if !strings.Contains(eventsJSON, onHeartbeatKey) {
		return false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(eventsJSON), &raw); err != nil {
		return false
	}
	hb, ok := raw[onHeartbeatKey]
	if !ok {
		return false
	}
	var actions []json.RawMessage
	if err := json.Unmarshal(hb, &actions); err != nil {
		return false
	}
	return len(actions) > 0
}

// coerceInt accepts the JSON number representations Go decodes into.
// JSON numbers land as float64 by default; integers may also arrive as
// int / int64 when callers populate map[string]any from typed
// structs. Anything else returns 0.
func coerceInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}
