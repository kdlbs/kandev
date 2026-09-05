package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// RunSnapshotStore persists runtime metadata on a run row.
type RunSnapshotStore interface {
	UpdateRunRuntimeSnapshot(
		ctx context.Context,
		id string,
		capabilities string,
		inputSnapshot string,
		sessionID string,
	) error
}

// DecisionSeatResolver reports whether an agent currently holds a decision
// seat (reviewer or approver) at a task's current workflow step. Implemented
// by *service.Service via HoldsDecisionSeat, mirroring the authorization
// RecordAgentDecision itself applies, so the runtime capability can never
// over- or under-state who can actually record a decision.
type DecisionSeatResolver interface {
	HoldsDecisionSeat(ctx context.Context, taskID, agentProfileID string) (bool, error)
}

// ContextBuilder builds the runtime context for a claimed run.
type ContextBuilder struct {
	Agents shared.AgentReader
	Runs   RunSnapshotStore
	Seats  DecisionSeatResolver
}

// Build resolves the agent identity and capabilities for a run.
func (b *ContextBuilder) Build(ctx context.Context, run *models.Run) (RunContext, error) {
	if run == nil {
		return RunContext{}, fmt.Errorf("run is required")
	}
	if b.Agents == nil {
		return RunContext{}, fmt.Errorf("%w: agents", ErrRuntimeDependencyMissing)
	}
	agent, err := b.Agents.GetAgentInstance(ctx, run.AgentProfileID)
	if err != nil {
		return RunContext{}, fmt.Errorf("resolve runtime agent: %w", err)
	}
	payload := parsePayload(run.Payload)
	taskID := payload["task_id"]
	sessionID := firstNonEmpty(run.SessionID, payload["session_id"])
	caps := FromAgent(agent).WithTaskScope(taskID)
	caps.CanRecordStepDecision = b.resolveDecisionSeat(ctx, taskID, agent.ID)
	runCtx := RunContext{
		WorkspaceID:  agent.WorkspaceID,
		AgentID:      agent.ID,
		TaskID:       taskID,
		RunID:        run.ID,
		SessionID:    sessionID,
		Reason:       run.Reason,
		Capabilities: caps,
	}
	return runCtx, nil
}

// resolveDecisionSeat reports whether the agent holds a decision seat for
// taskID. A taskless run, a missing resolver, or a lookup error all deny the
// capability rather than granting it speculatively.
func (b *ContextBuilder) resolveDecisionSeat(ctx context.Context, taskID, agentID string) bool {
	if taskID == "" || b.Seats == nil {
		return false
	}
	held, err := b.Seats.HoldsDecisionSeat(ctx, taskID, agentID)
	if err != nil {
		return false
	}
	return held
}

// BuildAndPersist builds context and stores its serialized snapshot on the run.
func (b *ContextBuilder) BuildAndPersist(ctx context.Context, run *models.Run) (RunContext, error) {
	runCtx, err := b.Build(ctx, run)
	if err != nil {
		return RunContext{}, err
	}
	if b.Runs == nil {
		return runCtx, nil
	}
	caps, err := MarshalCapabilities(runCtx.Capabilities)
	if err != nil {
		return RunContext{}, err
	}
	input, err := MarshalRunContext(runCtx)
	if err != nil {
		return RunContext{}, err
	}
	if err := b.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, caps, input, runCtx.SessionID); err != nil {
		return RunContext{}, fmt.Errorf("persist runtime snapshot: %w", err)
	}
	run.Capabilities = caps
	run.InputSnapshot = input
	run.SessionID = runCtx.SessionID
	return runCtx, nil
}

// MarshalCapabilities serializes capabilities for run snapshots and JWT claims.
func MarshalCapabilities(caps Capabilities) (string, error) {
	body, err := json.Marshal(caps)
	if err != nil {
		return "", fmt.Errorf("marshal capabilities: %w", err)
	}
	return string(body), nil
}

// MarshalRunContext serializes run context for run input snapshots.
func MarshalRunContext(runCtx RunContext) (string, error) {
	body, err := json.Marshal(runCtx)
	if err != nil {
		return "", fmt.Errorf("marshal run context: %w", err)
	}
	return string(body), nil
}

func parsePayload(payload string) map[string]string {
	raw := map[string]any{}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &raw)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
