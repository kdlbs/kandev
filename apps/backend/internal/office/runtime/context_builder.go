package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// scopeRunnerSetCap bounds a taskless run's materialized task scope.
const scopeRunnerSetCap = 500

// maxScopeSwapAttempts bounds the derive-and-swap retry loop in
// persistWithCAS at three total compare-and-swap attempts (the first plus
// two retries), after which BuildAndPersist returns an error and the
// caller's own retry (a later launch attempt) starts over.
const maxScopeSwapAttempts = 3

// Scope-derivation run event payload keys, shared by appendScopeEvent's two
// event kinds.
const (
	scopeEventAgent  = "agent_id"
	scopeEventRunKey = "run_id"
	scopeEventError  = "error"
)

// errRunnerSetUnderivable marks a scope that could not even attempt the
// runner-set query (no lister configured, or an empty run workspace) as
// distinct from one whose query failed.
var errRunnerSetUnderivable = fmt.Errorf("runner set unavailable: no reader configured or run workspace is empty")

// RunSnapshotStore persists runtime metadata on a run row.
type RunSnapshotStore interface {
	UpdateRunRuntimeSnapshot(
		ctx context.Context,
		id string,
		capabilities string,
		inputSnapshot string,
		sessionID string,
	) error
	// UpdateRunRuntimeSnapshotCAS is the compare-and-swap sibling used to
	// decide first-write-wins when two processors build context for the
	// same run concurrently. prevCapabilities is the exact capabilities
	// string Build read when it checked the run's task_scope_source marker.
	UpdateRunRuntimeSnapshotCAS(
		ctx context.Context,
		id string,
		prevCapabilities string,
		capabilities string,
		inputSnapshot string,
		sessionID string,
	) (bool, error)
	// GetRunByID re-reads a run so the losing side of a CAS race can adopt
	// the winner's persisted scope.
	GetRunByID(ctx context.Context, id string) (*models.Run, error)
}

// RunnerTaskLister resolves a taskless run's task scope: the ordered,
// capped ids of tasks the agent is already responsible for in one
// workspace (the "runner set"), plus the true total before capping so
// truncation is detectable without a second query. A nil lister is not an
// empty runner set: the two report through different derivation outcomes.
type RunnerTaskLister interface {
	ListRunnerSetTaskIDs(ctx context.Context, agentID, workspaceID string, capAt int) ([]string, int, error)
}

// ScopeEventAppender records a scope-derivation run event
// (runtime.scope_truncated / runtime.scope_unavailable). A nil appender
// skips the event and never fails the build.
type ScopeEventAppender interface {
	AppendRunEvent(ctx context.Context, runID, eventType, level string, payload map[string]interface{})
}

// DecisionSeatResolver reports whether an agent currently holds a decision
// seat (reviewer or approver) at a task's current workflow step. Implemented
// by *service.Service via HoldsDecisionSeat, mirroring the authorization
// RecordAgentDecision itself applies. The result is advisory prompt metadata;
// the runtime decision endpoint performs live authorization when it is called.
type DecisionSeatResolver interface {
	HoldsDecisionSeat(ctx context.Context, taskID, agentProfileID string) (bool, error)
}

// ContextBuilder builds the runtime context for a claimed run.
type ContextBuilder struct {
	Agents       shared.AgentReader
	Runs         RunSnapshotStore
	Seats        DecisionSeatResolver
	RunnerLister RunnerTaskLister
	ScopeEvents  ScopeEventAppender
}

// scopeDerivation carries the internal detail behind a build's task scope:
// the ids, the true total, whether the cap truncated them, the query error
// (if any), the resulting marker, and whether the scope was reused
// verbatim from a persisted final marker rather than freshly derived.
type scopeDerivation struct {
	taskIDs   []string
	total     int
	truncated bool
	err       error
	marker    string
	reused    bool
}

// Build resolves the agent identity and capabilities for a run.
func (b *ContextBuilder) Build(ctx context.Context, run *models.Run) (RunContext, error) {
	runCtx, _, err := b.build(ctx, run)
	return runCtx, err
}

// build is Build's unexported counterpart: it additionally returns the
// scopeDerivation so BuildAndPersist can choose the right event and CAS
// path after the persistence swap decides which build actually won.
func (b *ContextBuilder) build(ctx context.Context, run *models.Run) (RunContext, scopeDerivation, error) {
	if run == nil {
		return RunContext{}, scopeDerivation{}, fmt.Errorf("run is required")
	}
	if b.Agents == nil {
		return RunContext{}, scopeDerivation{}, fmt.Errorf("%w: agents", ErrRuntimeDependencyMissing)
	}
	agent, err := b.Agents.GetAgentInstance(ctx, run.AgentProfileID)
	if err != nil {
		return RunContext{}, scopeDerivation{}, fmt.Errorf("resolve runtime agent: %w", err)
	}
	payload := parsePayload(run.Payload)
	taskID := strings.TrimSpace(payload["task_id"])
	sessionID := firstNonEmpty(run.SessionID, payload["session_id"])

	caps, derivation := b.deriveScope(ctx, run, agent, taskID)
	availableActions, err := b.resolveAvailableActions(ctx, taskID, agent.ID)
	if err != nil {
		return RunContext{}, scopeDerivation{}, err
	}
	runCtx := RunContext{
		WorkspaceID:      agent.WorkspaceID,
		AgentID:          agent.ID,
		TaskID:           taskID,
		RunID:            run.ID,
		SessionID:        sessionID,
		Reason:           run.Reason,
		Capabilities:     caps,
		AvailableActions: availableActions,
	}
	return runCtx, derivation, nil
}

// resolveAvailableActions reports advisory tools for the run prompt. A
// taskless run or missing resolver has no seat-derived actions. A lookup error
// aborts context construction so the scheduler can retry instead of launching
// a prompt that requires a tool which it could not advertise.
func (b *ContextBuilder) resolveAvailableActions(ctx context.Context, taskID, agentID string) ([]string, error) {
	if taskID == "" || b.Seats == nil {
		return nil, nil
	}
	held, err := b.Seats.HoldsDecisionSeat(ctx, taskID, agentID)
	if err != nil {
		return nil, fmt.Errorf("resolve decision seat: %w", err)
	}
	if !held {
		return nil, nil
	}
	return []string{AvailableActionRecordStepDecision}, nil
}

// deriveScope decides a run's task scope: a task-bound run's is always its
// own trimmed task id (no reuse, no workspace filter); a taskless run's is
// reused verbatim from a final persisted marker or, failing that, freshly
// derived from the runner set.
func (b *ContextBuilder) deriveScope(
	ctx context.Context, run *models.Run, agent *models.AgentInstance, taskID string,
) (Capabilities, scopeDerivation) {
	caps := FromAgent(agent)
	if taskID != "" {
		derivation := scopeDerivation{taskIDs: []string{taskID}, marker: TaskScopeSourcePayload}
		return withScope(caps, derivation), derivation
	}
	if persisted, ok := reusePersistedTaskScope(run); ok {
		derivation := scopeDerivation{
			taskIDs: persisted.AllowedTaskIDs,
			marker:  persisted.TaskScopeSource,
			reused:  true,
		}
		return withScope(caps, derivation), derivation
	}
	derivation := b.deriveRunnerSet(ctx, agent.ID, agent.WorkspaceID)
	return withScope(caps, derivation), derivation
}

// deriveRunnerSet issues the runner-set query for a taskless run. Failure
// is closed: no lister, an empty workspace, or a query error each yield an
// empty scope with the provisional unavailable marker rather than an
// empty final one, so the next build retries instead of freezing the gap.
func (b *ContextBuilder) deriveRunnerSet(ctx context.Context, agentID, workspaceID string) scopeDerivation {
	if b.RunnerLister == nil || strings.TrimSpace(workspaceID) == "" {
		return scopeDerivation{marker: TaskScopeSourceUnavailable, err: errRunnerSetUnderivable}
	}
	ids, total, err := b.RunnerLister.ListRunnerSetTaskIDs(ctx, agentID, workspaceID, scopeRunnerSetCap)
	if err != nil {
		return scopeDerivation{marker: TaskScopeSourceUnavailable, err: err}
	}
	return scopeDerivation{
		taskIDs:   ids,
		total:     total,
		truncated: total > len(ids),
		marker:    TaskScopeSourceRunnerSet,
	}
}

// reusePersistedTaskScope reads a run's persisted capabilities and reports
// whether they carry a final task_scope_source marker. An absent,
// provisional, unrecognized, or unparseable snapshot all report false, so
// the caller derives a fresh scope instead of reusing a stale or partial one.
func reusePersistedTaskScope(run *models.Run) (Capabilities, bool) {
	if run == nil || run.Capabilities == "" {
		return Capabilities{}, false
	}
	var persisted Capabilities
	if err := json.Unmarshal([]byte(run.Capabilities), &persisted); err != nil {
		return Capabilities{}, false
	}
	switch persisted.TaskScopeSource {
	case TaskScopeSourcePayload, TaskScopeSourceRunnerSet:
		return persisted, true
	default:
		return Capabilities{}, false
	}
}

func withScope(caps Capabilities, derivation scopeDerivation) Capabilities {
	next := caps.WithTaskScope(derivation.taskIDs...)
	next.TaskScopeSource = derivation.marker
	return next
}

// BuildAndPersist builds context and stores its serialized snapshot on the
// run. It owns the ScopeEventAppender call site outright: Build derives
// the scope and never appends, so a build that loses the CAS race, has no
// appender, or persists nothing (no Runs store) appends no scope event and
// still returns a context.
func (b *ContextBuilder) BuildAndPersist(ctx context.Context, run *models.Run) (RunContext, error) {
	runCtx, derivation, err := b.build(ctx, run)
	if err != nil {
		return RunContext{}, err
	}
	prevCapabilities := run.Capabilities
	if b.Runs == nil {
		return runCtx, nil
	}
	if derivation.reused || derivation.marker == TaskScopeSourcePayload {
		if err := b.persistUnconditional(ctx, run, runCtx); err != nil {
			return RunContext{}, err
		}
		return runCtx, nil
	}
	return b.persistWithCAS(ctx, run, runCtx, derivation, prevCapabilities)
}

// persistUnconditional performs the ordinary unconditional snapshot write
// used whenever this build's task scope needs no concurrency guard: a
// task-bound run's scope is a pure function of its payload, and a reused
// final scope is, by construction, already the scope every concurrent
// build would agree on.
func (b *ContextBuilder) persistUnconditional(ctx context.Context, run *models.Run, runCtx RunContext) error {
	caps, input, err := marshalSnapshot(runCtx)
	if err != nil {
		return err
	}
	if err := b.Runs.UpdateRunRuntimeSnapshot(ctx, run.ID, caps, input, runCtx.SessionID); err != nil {
		return fmt.Errorf("persist runtime snapshot: %w", err)
	}
	run.Capabilities = caps
	run.InputSnapshot = input
	run.SessionID = runCtx.SessionID
	return nil
}

// persistWithCAS guards a freshly-derived taskless scope (runner_set or
// unavailable) with a compare-and-swap so that when two processors build
// context for one run concurrently, the first persisted final marker wins.
// The loser re-reads the run; if it now carries a final marker, the loser
// adopts that scope verbatim and emits no scope event; otherwise it retries
// the derive-and-swap, up to maxScopeSwapAttempts total attempts.
func (b *ContextBuilder) persistWithCAS(
	ctx context.Context,
	run *models.Run,
	runCtx RunContext,
	derivation scopeDerivation,
	prevCapabilities string,
) (RunContext, error) {
	for attempt := 1; ; attempt++ {
		caps, input, err := marshalSnapshot(runCtx)
		if err != nil {
			return RunContext{}, err
		}
		won, err := b.Runs.UpdateRunRuntimeSnapshotCAS(ctx, run.ID, prevCapabilities, caps, input, runCtx.SessionID)
		if err != nil {
			return RunContext{}, fmt.Errorf("persist runtime snapshot: %w", err)
		}
		if won {
			run.Capabilities = caps
			run.InputSnapshot = input
			run.SessionID = runCtx.SessionID
			b.appendScopeEvent(ctx, runCtx, derivation)
			return runCtx, nil
		}

		current, err := b.Runs.GetRunByID(ctx, run.ID)
		if err != nil {
			return RunContext{}, fmt.Errorf("re-read run after lost scope race: %w", err)
		}
		if persisted, ok := reusePersistedTaskScope(current); ok {
			runCtx.Capabilities = withScope(runCtx.Capabilities, scopeDerivation{
				taskIDs: persisted.AllowedTaskIDs,
				marker:  persisted.TaskScopeSource,
			})
			if err := b.persistUnconditional(ctx, run, runCtx); err != nil {
				return RunContext{}, err
			}
			return runCtx, nil
		}
		if attempt >= maxScopeSwapAttempts {
			return RunContext{}, fmt.Errorf("scope snapshot race: exceeded %d attempts", maxScopeSwapAttempts)
		}
		prevCapabilities = current.Capabilities
		derivation = b.deriveRunnerSet(ctx, runCtx.AgentID, runCtx.WorkspaceID)
		runCtx.Capabilities = withScope(runCtx.Capabilities, derivation)
	}
}

func (b *ContextBuilder) appendScopeEvent(ctx context.Context, runCtx RunContext, derivation scopeDerivation) {
	if b.ScopeEvents == nil {
		return
	}
	switch {
	case derivation.marker == TaskScopeSourceRunnerSet && derivation.truncated:
		b.ScopeEvents.AppendRunEvent(ctx, runCtx.RunID, "runtime.scope_truncated", "warn", map[string]interface{}{
			"cap":            scopeRunnerSetCap,
			"total":          derivation.total,
			scopeEventAgent:  runCtx.AgentID,
			scopeEventRunKey: runCtx.RunID,
		})
	case derivation.marker == TaskScopeSourceUnavailable:
		errMsg := ""
		if derivation.err != nil {
			errMsg = derivation.err.Error()
		}
		b.ScopeEvents.AppendRunEvent(ctx, runCtx.RunID, "runtime.scope_unavailable", "warn", map[string]interface{}{
			scopeEventError:  errMsg,
			scopeEventAgent:  runCtx.AgentID,
			scopeEventRunKey: runCtx.RunID,
		})
	}
}

func marshalSnapshot(runCtx RunContext) (capabilities string, inputSnapshot string, err error) {
	caps, err := MarshalCapabilities(runCtx.Capabilities)
	if err != nil {
		return "", "", err
	}
	input, err := MarshalRunContext(runCtx)
	if err != nil {
		return "", "", err
	}
	return caps, input, nil
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
