package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrActionNotYetWired is the sentinel returned by Phase 2 callbacks when a
// required engine dependency (RunQueueAdapter, ParticipantStore, …) has not
// been wired. It is intentionally exported so the orchestrator can detect
// "kanban-only" engines vs office-wired ones in tests, and so callers see a
// loud, distinctive error rather than a silent no-op.
var ErrActionNotYetWired = errors.New("workflow action not yet wired")

// Target prefixes / sentinels recognised by QueueRunCallback.
const (
	TargetPrimary       = "primary"
	TargetParticipant   = "participant_role:"
	TargetAgentProfile  = "agent_profile_id:"
	TargetWorkspaceCEO  = "workspace.ceo_agent"
	TaskIDThis          = "this"
	defaultQueueReasonR = "queue_run"
)

// PrimaryAgentResolver resolves the step's "primary" agent profile id. The
// engine asks via this interface when a queue_run target is "primary" — the
// answer is just step.PrimaryAgentProfileID for kanban-style steps. The
// indirection keeps the engine package free of model imports.
type PrimaryAgentResolver interface {
	PrimaryAgentProfileID(ctx context.Context, stepID string) (string, error)
}

// QueueRunCallback executes the queue_run action by resolving Target/TaskID
// then enqueuing a run via RunQueueAdapter.
type QueueRunCallback struct {
	Adapter      RunQueueAdapter
	Participants ParticipantStore
	CEOResolver  CEOAgentResolver
	Primary      PrimaryAgentResolver
}

// Execute satisfies ActionCallback.
func (c QueueRunCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Adapter == nil {
		return ActionResult{}, fmt.Errorf("%w: queue_run requires RunQueueAdapter", ErrActionNotYetWired)
	}
	if in.Action.QueueRun == nil {
		return ActionResult{}, fmt.Errorf("queue_run action missing QueueRun config")
	}
	taskID := resolveTaskID(in.Action.QueueRun.TaskID, in.State.TaskID)
	agentIDs, err := c.resolveTarget(ctx, in)
	if err != nil {
		return ActionResult{}, err
	}
	for _, agentID := range agentIDs {
		req := QueueRunRequest{
			AgentProfileID: agentID,
			TaskID:         taskID,
			WorkflowStepID: in.Step.ID,
			Reason:         queueRunReason(in),
			IdempotencyKey: idempotencyKey(in, agentID, taskID),
			Payload:        in.Action.QueueRun.Payload,
		}
		if err := c.Adapter.QueueRun(ctx, req); err != nil {
			return ActionResult{}, fmt.Errorf("queue_run for agent %s: %w", agentID, err)
		}
	}
	return ActionResult{}, nil
}

func (c QueueRunCallback) resolveTarget(ctx context.Context, in ActionInput) ([]string, error) {
	target := strings.TrimSpace(in.Action.QueueRun.Target)
	switch {
	case target == "" || target == TargetPrimary:
		return c.resolvePrimary(ctx, in.Step.ID)
	case strings.HasPrefix(target, TargetParticipant):
		role := strings.TrimPrefix(target, TargetParticipant)
		return c.resolveParticipantRole(ctx, in.Step.ID, in.State.TaskID, role)
	case strings.HasPrefix(target, TargetAgentProfile):
		id := strings.TrimPrefix(target, TargetAgentProfile)
		if id == "" {
			return nil, fmt.Errorf("queue_run agent_profile_id target is empty")
		}
		return []string{id}, nil
	case target == TargetWorkspaceCEO:
		return c.resolveCEO(ctx, in.State.TaskID)
	default:
		return nil, fmt.Errorf("queue_run: unsupported target %q", target)
	}
}

func (c QueueRunCallback) resolvePrimary(ctx context.Context, stepID string) ([]string, error) {
	if c.Primary == nil {
		return nil, fmt.Errorf("%w: queue_run target=primary requires PrimaryAgentResolver", ErrActionNotYetWired)
	}
	id, err := c.Primary.PrimaryAgentProfileID(ctx, stepID)
	if err != nil {
		return nil, fmt.Errorf("queue_run resolve primary: %w", err)
	}
	if id == "" {
		return nil, fmt.Errorf("queue_run: step %s has no primary agent profile", stepID)
	}
	return []string{id}, nil
}

func (c QueueRunCallback) resolveParticipantRole(ctx context.Context, stepID, taskID, role string) ([]string, error) {
	if c.Participants == nil {
		return nil, fmt.Errorf("%w: queue_run target=participant_role requires ParticipantStore", ErrActionNotYetWired)
	}
	all, err := c.Participants.ListStepParticipants(ctx, stepID, taskID)
	if err != nil {
		return nil, fmt.Errorf("queue_run list participants: %w", err)
	}
	ids := make([]string, 0, len(all))
	for _, p := range all {
		if p.Role == role {
			ids = append(ids, p.AgentProfileID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("queue_run: no participants with role %q on step %s", role, stepID)
	}
	return ids, nil
}

func (c QueueRunCallback) resolveCEO(ctx context.Context, taskID string) ([]string, error) {
	if c.CEOResolver == nil {
		return nil, fmt.Errorf("%w: queue_run target=workspace.ceo_agent requires CEOAgentResolver", ErrActionNotYetWired)
	}
	id, err := c.CEOResolver.ResolveCEOAgentProfileID(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("queue_run resolve workspace.ceo_agent: %w", err)
	}
	if id == "" {
		return nil, fmt.Errorf("queue_run: workspace has no CEO agent profile for task %s", taskID)
	}
	return []string{id}, nil
}

// resolveTaskID maps the action's TaskID string into a concrete id, honouring
// the "this" sentinel and the default-empty-means-this convention.
func resolveTaskID(target, currentTaskID string) string {
	t := strings.TrimSpace(target)
	if t == "" || t == TaskIDThis {
		return currentTaskID
	}
	return t
}

// queueRunReason picks the action-supplied reason, falling back to the
// trigger type so logs and telemetry get a meaningful default.
func queueRunReason(in ActionInput) string {
	if in.Action.QueueRun != nil && in.Action.QueueRun.Reason != "" {
		return in.Action.QueueRun.Reason
	}
	if in.Trigger != "" {
		return string(in.Trigger)
	}
	return defaultQueueReasonR
}

// idempotencyKey synthesises a deterministic key from the engine's
// operation id (already idempotent across retries) plus action-specific
// salt. When OperationID is empty, the adapter sees an empty key and is
// expected to dedupe via its own mechanism (or accept the duplicate).
func idempotencyKey(in ActionInput, agentID, taskID string) string {
	if in.OperationID == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s:%s", in.OperationID, in.Step.ID, taskID, agentID)
}

// ClearDecisionsCallback executes the clear_decisions action by deleting all
// recorded decisions for the trigger's (task, step) pair.
type ClearDecisionsCallback struct {
	Decisions DecisionStore
}

// Execute satisfies ActionCallback.
func (c ClearDecisionsCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Decisions == nil {
		return ActionResult{}, fmt.Errorf("%w: clear_decisions requires DecisionStore", ErrActionNotYetWired)
	}
	if _, err := c.Decisions.ClearStepDecisions(ctx, in.State.TaskID, in.Step.ID); err != nil {
		return ActionResult{}, fmt.Errorf("clear_decisions: %w", err)
	}
	return ActionResult{}, nil
}

// QueueRunForEachParticipantCallback fans out queue_run over every participant
// on the step matching the configured role.
type QueueRunForEachParticipantCallback struct {
	Adapter      RunQueueAdapter
	Participants ParticipantStore
}

// Execute satisfies ActionCallback.
func (c QueueRunForEachParticipantCallback) Execute(ctx context.Context, in ActionInput) (ActionResult, error) {
	if c.Adapter == nil {
		return ActionResult{}, fmt.Errorf("%w: queue_run_for_each_participant requires RunQueueAdapter", ErrActionNotYetWired)
	}
	if c.Participants == nil {
		return ActionResult{}, fmt.Errorf("%w: queue_run_for_each_participant requires ParticipantStore", ErrActionNotYetWired)
	}
	cfg := in.Action.QueueRunForEachParticipant
	if cfg == nil || cfg.Role == "" {
		return ActionResult{}, fmt.Errorf("queue_run_for_each_participant missing role")
	}
	taskID := in.State.TaskID
	all, err := c.Participants.ListStepParticipants(ctx, in.Step.ID, taskID)
	if err != nil {
		return ActionResult{}, fmt.Errorf("queue_run_for_each_participant list participants: %w", err)
	}
	reason := cfg.Reason
	if reason == "" {
		reason = string(in.Trigger)
	}
	for _, p := range all {
		if p.Role != cfg.Role {
			continue
		}
		req := QueueRunRequest{
			AgentProfileID: p.AgentProfileID,
			TaskID:         taskID,
			WorkflowStepID: in.Step.ID,
			Reason:         reason,
			IdempotencyKey: idempotencyKey(in, p.AgentProfileID, taskID),
			Payload:        cfg.Payload,
		}
		if err := c.Adapter.QueueRun(ctx, req); err != nil {
			return ActionResult{}, fmt.Errorf("queue_run for participant %s: %w", p.ID, err)
		}
	}
	return ActionResult{}, nil
}

// PlaceholderQueueRunCallback is preserved as a typed alias for backward
// compatibility with the Phase 2 prep slice. New code should construct
// QueueRunCallback directly.
//
// Deprecated: Use QueueRunCallback.
type PlaceholderQueueRunCallback struct{}

// Execute returns ErrActionNotYetWired so accidental use is loud.
func (PlaceholderQueueRunCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	return ActionResult{}, ErrActionNotYetWired
}

// PlaceholderClearDecisionsCallback is preserved for backward compatibility.
//
// Deprecated: Use ClearDecisionsCallback.
type PlaceholderClearDecisionsCallback struct{}

// Execute returns ErrActionNotYetWired so accidental use is loud.
func (PlaceholderClearDecisionsCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	return ActionResult{}, ErrActionNotYetWired
}

// PlaceholderQueueRunForEachParticipantCallback is preserved for backward
// compatibility.
//
// Deprecated: Use QueueRunForEachParticipantCallback.
type PlaceholderQueueRunForEachParticipantCallback struct{}

// Execute returns ErrActionNotYetWired so accidental use is loud.
func (PlaceholderQueueRunForEachParticipantCallback) Execute(_ context.Context, _ ActionInput) (ActionResult, error) {
	return ActionResult{}, ErrActionNotYetWired
}

// Compile-time interface assertions.
var (
	_ ActionCallback = QueueRunCallback{}
	_ ActionCallback = ClearDecisionsCallback{}
	_ ActionCallback = QueueRunForEachParticipantCallback{}
	_ ActionCallback = PlaceholderQueueRunCallback{}
	_ ActionCallback = PlaceholderClearDecisionsCallback{}
	_ ActionCallback = PlaceholderQueueRunForEachParticipantCallback{}
)
