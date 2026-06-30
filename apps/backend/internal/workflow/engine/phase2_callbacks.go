package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/runs/commentkeys"
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

// PrimaryAgentResolver resolves the task's "primary" agent profile id. The
// engine asks via this interface when a queue_run target is "primary". The
// answer is task-aware so office steps can prefer the task's current runner
// participant while kanban-style steps still fall back to the step primary.
// The indirection keeps the engine package free of model imports.
type PrimaryAgentResolver interface {
	PrimaryAgentProfileID(ctx context.Context, stepID, taskID string) (string, error)
}

// TargetTaskPrimaryAgentResolver resolves "primary" using the target task's
// current workflow step. PrimaryAgentResolver is step-aware for same-task
// triggers where the engine already has the current StepSpec; cross-task
// queue_run actions need the adapter to look up the target task's step.
type TargetTaskPrimaryAgentResolver interface {
	PrimaryAgentProfileIDForTask(ctx context.Context, taskID string) (string, error)
}

// TargetTaskStepResolver resolves a task's current workflow step for cross-task
// queue_run actions. The engine only has the triggering task's StepSpec, so
// adapters that read task storage provide this for target task lookups.
type TargetTaskStepResolver interface {
	WorkflowStepIDForTask(ctx context.Context, taskID string) (string, error)
}

// QueueRunCallback executes the queue_run action by resolving Target/TaskID
// then enqueuing a run via RunQueueAdapter.
type QueueRunCallback struct {
	Adapter      RunQueueAdapter
	Participants ParticipantStore
	CEOResolver  CEOAgentResolver
	Primary      PrimaryAgentResolver
	TaskSteps    TargetTaskStepResolver
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
	agentIDs, workflowStepID, err := c.resolveTarget(ctx, in, taskID)
	if err != nil {
		return ActionResult{}, err
	}
	for _, agentID := range agentIDs {
		req := QueueRunRequest{
			AgentProfileID: agentID,
			TaskID:         taskID,
			WorkflowStepID: workflowStepID,
			Reason:         queueRunReason(in),
			IdempotencyKey: idempotencyKey(in, agentID, taskID),
			Payload:        queueRunPayload(in, in.Action.QueueRun.Payload, taskID),
		}
		if err := c.Adapter.QueueRun(ctx, req); err != nil {
			return ActionResult{}, fmt.Errorf("queue_run for agent %s: %w", agentID, err)
		}
	}
	return ActionResult{}, nil
}

func (c QueueRunCallback) resolveTarget(
	ctx context.Context, in ActionInput, taskID string,
) ([]string, string, error) {
	target := strings.TrimSpace(in.Action.QueueRun.Target)
	switch {
	case target == "" || target == TargetPrimary:
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, pickStepResolver(c.Primary, c.TaskSteps))
		if err != nil {
			return nil, "", err
		}
		agentIDs, err := c.resolvePrimary(ctx, in, taskID)
		return agentIDs, stepID, err
	case strings.HasPrefix(target, TargetParticipant):
		role := strings.TrimPrefix(target, TargetParticipant)
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, pickStepResolver(c.Participants, c.TaskSteps))
		if err != nil {
			return nil, "", err
		}
		agentIDs, err := c.resolveParticipantRole(ctx, stepID, taskID, role)
		return agentIDs, stepID, err
	case strings.HasPrefix(target, TargetAgentProfile):
		id := strings.TrimPrefix(target, TargetAgentProfile)
		if id == "" {
			return nil, "", fmt.Errorf("queue_run agent_profile_id target is empty")
		}
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, c.TaskSteps)
		if err != nil {
			return nil, "", err
		}
		return []string{id}, stepID, nil
	case target == TargetWorkspaceCEO:
		stepID, err := c.resolveTargetStepID(ctx, in, taskID, c.TaskSteps)
		if err != nil {
			return nil, "", err
		}
		agentIDs, err := c.resolveCEO(ctx, taskID)
		return agentIDs, stepID, err
	default:
		return nil, "", fmt.Errorf("queue_run: unsupported target %q", target)
	}
}

func (c QueueRunCallback) resolveTargetStepID(
	ctx context.Context, in ActionInput, taskID string, resolver TargetTaskStepResolver,
) (string, error) {
	if taskID == in.State.TaskID {
		return in.Step.ID, nil
	}
	if resolver == nil {
		return "", fmt.Errorf("%w: queue_run cross-task target requires TargetTaskStepResolver", ErrActionNotYetWired)
	}
	stepID, err := resolver.WorkflowStepIDForTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("queue_run resolve target task step: %w", err)
	}
	if stepID == "" {
		return "", fmt.Errorf("queue_run: task %s has no workflow step", taskID)
	}
	return stepID, nil
}

func pickStepResolver(v any, fallback TargetTaskStepResolver) TargetTaskStepResolver {
	if resolver, ok := v.(TargetTaskStepResolver); ok {
		return resolver
	}
	return fallback
}

func (c QueueRunCallback) resolvePrimary(
	ctx context.Context, in ActionInput, taskID string,
) ([]string, error) {
	if c.Primary == nil {
		return nil, fmt.Errorf("%w: queue_run target=primary requires PrimaryAgentResolver", ErrActionNotYetWired)
	}
	if taskID != in.State.TaskID {
		targetResolver, ok := c.Primary.(TargetTaskPrimaryAgentResolver)
		if !ok {
			return nil, fmt.Errorf("%w: queue_run cross-task target=primary requires TargetTaskPrimaryAgentResolver", ErrActionNotYetWired)
		}
		id, err := targetResolver.PrimaryAgentProfileIDForTask(ctx, taskID)
		if err != nil {
			return nil, fmt.Errorf("queue_run resolve target task primary: %w", err)
		}
		if id == "" {
			return nil, fmt.Errorf("queue_run: task %s has no primary agent profile", taskID)
		}
		return []string{id}, nil
	}
	id, err := c.Primary.PrimaryAgentProfileID(ctx, in.Step.ID, taskID)
	if err != nil {
		return nil, fmt.Errorf("queue_run resolve primary: %w", err)
	}
	if id == "" {
		return nil, fmt.Errorf("queue_run: step %s has no primary agent profile", in.Step.ID)
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
// salt. The canonical same-task primary task_comment wake keeps the exact
// "task_comment:<comment_id>" key so comment status lookups can map the
// single run back to the user comment. Custom reasons, payloads, cross-task
// actions, and multi-agent fan-out keep the salt so one comment can still wake
// every intended target.
// When OperationID is empty, the adapter sees an empty key and is expected to
// dedupe via its own mechanism (or accept the duplicate).
func idempotencyKey(in ActionInput, agentID, taskID string) string {
	if in.OperationID == "" {
		return ""
	}
	if usesExactCommentKey(in) {
		return in.OperationID
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s",
		in.OperationID, in.Step.ID, taskID, agentID, queueActionDigest(in))
}

func usesExactCommentKey(in ActionInput) bool {
	if in.Trigger != TriggerOnComment || !commentkeys.HasTaskCommentPrefix(in.OperationID) {
		return false
	}
	if in.Action.Kind != ActionQueueRun || in.Action.QueueRun == nil {
		return false
	}
	target := strings.TrimSpace(in.Action.QueueRun.Target)
	taskID := strings.TrimSpace(in.Action.QueueRun.TaskID)
	return (target == "" || target == TargetPrimary) &&
		(taskID == "" || taskID == TaskIDThis) &&
		queueRunReason(in) == commentkeys.TaskCommentReason &&
		len(in.Action.QueueRun.Payload) == 0
}

func queueActionDigest(in ActionInput) string {
	key := struct {
		Kind    ActionKind     `json:"kind"`
		Target  string         `json:"target,omitempty"`
		TaskID  string         `json:"task_id,omitempty"`
		Role    string         `json:"role,omitempty"`
		Reason  string         `json:"reason"`
		Payload map[string]any `json:"payload,omitempty"`
	}{
		Kind: in.Action.Kind,
	}
	switch in.Action.Kind {
	case ActionQueueRun:
		if in.Action.QueueRun != nil {
			key.Target = strings.TrimSpace(in.Action.QueueRun.Target)
			key.TaskID = strings.TrimSpace(in.Action.QueueRun.TaskID)
			key.Reason = queueRunReason(in)
			key.Payload = in.Action.QueueRun.Payload
		}
	case ActionQueueRunForEachParticipant:
		if in.Action.QueueRunForEachParticipant != nil {
			cfg := in.Action.QueueRunForEachParticipant
			key.Role = strings.TrimSpace(cfg.Role)
			key.Reason = queueRunForEachParticipantReason(in)
			key.Payload = cfg.Payload
		}
	}
	b, err := json.Marshal(key)
	if err != nil {
		b = []byte(string(key.Kind) + "\x00" + key.Target + "\x00" +
			key.TaskID + "\x00" + key.Role + "\x00" + key.Reason)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func queueRunPayload(in ActionInput, actionPayload map[string]any, targetTaskID string) map[string]any {
	out := make(map[string]any, len(actionPayload)+3)
	for k, v := range actionPayload {
		out[k] = v
	}
	comment, ok := commentPayload(in.Payload)
	if ok {
		if comment.CommentID != "" {
			out["comment_id"] = comment.CommentID
		}
		if comment.AuthorID != "" {
			out["author_id"] = comment.AuthorID
		}
	}
	if targetTaskID != "" &&
		in.State.TaskID != "" &&
		targetTaskID != in.State.TaskID &&
		(ok || commentkeys.HasTaskCommentPrefix(in.OperationID)) {
		out["source_task_id"] = in.State.TaskID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func commentPayload(payload any) (OnCommentPayload, bool) {
	switch p := payload.(type) {
	case OnCommentPayload:
		return p, true
	case *OnCommentPayload:
		if p != nil {
			return *p, true
		}
	}
	return OnCommentPayload{}, false
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
	reason := queueRunForEachParticipantReason(in)
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
			Payload:        queueRunPayload(in, cfg.Payload, taskID),
		}
		if err := c.Adapter.QueueRun(ctx, req); err != nil {
			return ActionResult{}, fmt.Errorf("queue_run for participant %s: %w", p.ID, err)
		}
	}
	return ActionResult{}, nil
}

func queueRunForEachParticipantReason(in ActionInput) string {
	if in.Action.QueueRunForEachParticipant != nil && in.Action.QueueRunForEachParticipant.Reason != "" {
		return in.Action.QueueRunForEachParticipant.Reason
	}
	return string(in.Trigger)
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
