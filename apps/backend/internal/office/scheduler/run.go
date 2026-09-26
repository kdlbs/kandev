// Package scheduler orchestrates run processing for the office domain.
// It wraps service.Service and owns retry logic and dispatch routing.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/pause"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	runsservice "github.com/kandev/kandev/internal/runs/service"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// ErrRoutingNotSupported is returned by TaskStarter.StartTaskWithRoute
// when the implementing starter cannot honour a routing override. The
// scheduler falls back to the legacy concrete-profile launch path on
// this sentinel.
var ErrRoutingNotSupported = errors.New("routing not supported by task starter")

// RouteOverride carries a fully resolved provider profile for one launch.
// It is intentionally richer than (providerID, model) because cross-
// provider routing means CLI mode, flags, and env are provider-scoped
// — they cannot safely be inherited from a base AgentProfile authored
// against a different provider.
//
// Permission knobs are NOT in this struct: the launch path's permission
// model is the base profile's CLIFlags + AutoApprove booleans, not a
// preset-by-name. Per-provider permission overrides would require
// re-modelling that surface and are deferred.
type RouteOverride = service.RouteOverride

// LaunchContext is an alias for service.LaunchContext so dispatch
// callsites inside this package can spell the type without re-imports.
// The scheduler-side dispatcher takes a service.LaunchContext directly.
// See service.LaunchContext for field semantics.
type LaunchContext = service.LaunchContext

// Run reason constants. Aliases of shared's canonical declarations
// (AC-OFFICE-BACKPRESSURE-001.8) — see shared/runreasons.go.
const (
	RunReasonTaskAssigned          = shared.RunReasonTaskAssigned
	RunReasonTaskComment           = shared.RunReasonTaskComment
	RunReasonTaskBlockersResolved  = shared.RunReasonTaskBlockersResolved
	RunReasonTaskChildrenCompleted = shared.RunReasonTaskChildrenCompleted
	RunReasonApprovalResolved      = shared.RunReasonApprovalResolved
	RunReasonRoutineTrigger        = shared.RunReasonRoutineTrigger
	RunReasonHeartbeat             = shared.RunReasonHeartbeat
	RunReasonBudgetAlert           = shared.RunReasonBudgetAlert
	RunReasonAgentError            = shared.RunReasonAgentError

	// Reactivity-pipeline reasons.
	RunReasonTaskUnblocked         = shared.RunReasonTaskUnblocked         // status: blocked → not blocked
	RunReasonTaskReopened          = shared.RunReasonTaskReopened          // silent reopen (status only)
	RunReasonTaskReopenedComment   = shared.RunReasonTaskReopenedComment   // user comment on closed task or resume:true
	RunReasonTaskMentioned         = shared.RunReasonTaskMentioned         // @mention in comment, additive to assignee wake
	RunReasonStagePending          = shared.RunReasonStagePending          // execution policy advanced to a new stage
	RunReasonStageChangesRequested = shared.RunReasonStageChangesRequested // reviewer asked for rework

	// Approval-flow reactivity reasons (B5).
	RunReasonTaskReviewRequested  = shared.RunReasonTaskReviewRequested  // task entered in_review; ping reviewers/approvers
	RunReasonTaskChangesRequested = shared.RunReasonTaskChangesRequested // a reviewer/approver asked for changes
	RunReasonTaskReadyToClose     = shared.RunReasonTaskReadyToClose     // all approvers have approved; assignee may close
)

// RunContext is the structured payload attached to every run the
// reactivity pipeline produces. It is JSON-serialised into the
// Run.Payload column so the agent runtime can pick the right
// system prompt template based on the reason.
type RunContext struct {
	Reason      string `json:"reason"`
	TaskID      string `json:"task_id"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	// WorkflowStepID is the parent's workflow step at wake time. An
	// engine-routed producer's request always carries it (runs/service's
	// runPayload copies it from the typed field), and
	// evaluateRunStaleness reads it to cancel a queued run whose parent
	// has since moved to a different step. Left empty, that guard never
	// applies — so cascade must set this whenever it can be resolved, or
	// the two producers' wakes for the same wave are not equivalent.
	WorkflowStepID        string   `json:"workflow_step_id,omitempty"`
	ActorID               string   `json:"actor_id,omitempty"`
	ActorType             string   `json:"actor_type,omitempty"` // "user" | "agent"
	CommentID             string   `json:"comment_id,omitempty"`
	ResolvedBlockerTaskID string   `json:"resolved_blocker_task_id,omitempty"`
	ChildTaskID           string   `json:"child_task_id,omitempty"`
	StageID               string   `json:"stage_id,omitempty"`
	AllowedActions        []string `json:"allowed_actions,omitempty"`
	// Role is the participant role the recipient holds for the task
	// (reviewer|approver). Set by the approval-flow reactivity hooks
	// so the agent's prompt builder can render an appropriate
	// "you are the reviewer/approver" framing.
	Role string `json:"role,omitempty"`
	// DecisionComment carries the comment text supplied with a
	// changes_requested decision so the assignee receiving
	// task_changes_requested has the context inline.
	DecisionComment string `json:"decision_comment,omitempty"`

	// IdempotencyKey is the dedup identity QueueRunCtx passes through
	// verbatim. Empty means no dedup — the run enqueues keyless — not
	// "derive one for me": QueueRunCtx no longer synthesises a
	// "{reason}:{taskID}:{agentID}" default, which was permanently unique
	// per (reason, task, agent) and silently swallowed every later
	// legitimate occurrence for the same triple. Callers that want dedup
	// build a key that changes with the thing that makes each occurrence
	// distinct. Excluded from the JSON payload: it must never change
	// encodeRunContext's output shape, which CoalesceRun compares for
	// equality and taskIDFromPayload parses.
	IdempotencyKey string `json:"-"`

	// WaveKey and WaveString carry a completion-wave identity onto the
	// persisted run (models.Run.WakeWaveKey / WakeWaveString), not into
	// the JSON payload: they gate admission via idx_run_wake_wave and
	// coalescing, not agent-facing content. Empty means "no wave
	// identity" — the ordinary idempotency-key path applies instead.
	WaveKey    string `json:"-"`
	WaveString string `json:"-"`

	// ExtraPayload, when non-empty, is merged onto the JSON-encoded
	// payload by encodeRunContext (workflow-authored keys win over any
	// struct field of the same name). Left nil, encodeRunContext's
	// output is byte-identical to a plain struct marshal.
	ExtraPayload map[string]any `json:"-"`
}

// Run status constants.
const (
	RunStatusQueued   = "queued"
	RunStatusClaimed  = "claimed"
	RunStatusFinished = "finished"
	RunStatusFailed   = "failed"
)

// CoalesceWindowSeconds is the default coalescing window.
const CoalesceWindowSeconds = 5

// IdempotencyWindowHours is the deduplication window.
const IdempotencyWindowHours = 24

// AssignmentWakeAllowanceN and AssignmentWakeAllowanceWindow are the fixed
// N and W of REQ-OFFICE-ASSIGN-RATE-001: at most N agent-initiated
// assignment wakes admitted per task within any rolling window of duration
// W. Fixed values of this capability rather than operator-configurable;
// referenced by the tests rather than restated.
const (
	AssignmentWakeAllowanceN      = 5
	AssignmentWakeAllowanceWindow = 10 * time.Minute
)

// TaskStarter launches agent sessions on behalf of the office scheduler.
// Implemented by the orchestrator; the scheduler depends only on this interface.
type TaskStarter interface {
	StartTask(
		ctx context.Context,
		taskID, agentProfileID, executorID, executorProfileID string,
		priority string, prompt, workflowStepID string,
		planMode bool, attachments []interface{},
	) error

	// StartTaskWithRoute launches a task with a fully resolved provider
	// override. Implementations that cannot apply a route override return
	// ErrRoutingNotSupported so the scheduler can fall through to the
	// legacy StartTask path. LaunchContext carries the Office-built
	// prompt, env, workflow step, attachments, and plan-mode flag so
	// routed launches do not lose role framing / AGENTS.md / wake
	// context vs the legacy launch path.
	StartTaskWithRoute(
		ctx context.Context,
		taskID, agentProfileID string,
		launch LaunchContext,
		route RouteOverride,
	) error
}

// SchedulerService orchestrates run processing.
// It holds the service layer and the SQLite repository.
// The unexported service helpers (kandevBasePath, resolveAgentType,
// resolveProjectSkillDir) are stored as function callbacks configured via setters.
type SchedulerService struct {
	repo                    *sqlite.Repository
	logger                  *logger.Logger
	svc                     *service.Service
	runsService             *runsservice.Service
	taskStarter             TaskStarter
	runSessionLauncher      service.RunSessionLauncher
	resolver                *routing.Resolver
	eb                      bus.EventBus
	apiBaseURL              string
	agentctlPath            string
	kandevBasePathFn        func() string
	agentTypeResolver       func(profileID string) string
	projectSkillDirResolver func(agentTypeID string) string
	workflowStepGetter      WorkflowStepGetter
	participantStore        engine.ParticipantStore
	pauseGate               shared.PauseGate
}

// WorkflowStepGetter resolves a workflow step by ID. Implemented by
// workflow/service.Service.GetStep; wired via SetWorkflowStepGetter so the
// cascade producer can resolve the parent's current step without an
// engine dependency, for payload parity with the engine-routed producers.
type WorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
}

// SetWorkflowStepGetter wires the workflow step lookup used for payload
// parity. Left nil, cascade wakes queue without a merged action payload.
func (ss *SchedulerService) SetWorkflowStepGetter(g WorkflowStepGetter) {
	ss.workflowStepGetter = g
}

// SetParticipantStore wires the participant seat resolution used for
// queue_run_for_each_participant payload parity — the same
// engine.ParticipantStore instance the workflow engine itself uses
// (workflow/adapters.ParticipantAdapter in production). Left nil, cascade
// never attaches a for-each-participant action's payload.
func (ss *SchedulerService) SetParticipantStore(store engine.ParticipantStore) {
	ss.participantStore = store
}

// NewSchedulerService creates a new SchedulerService.
func NewSchedulerService(
	repo *sqlite.Repository,
	log *logger.Logger,
	svc *service.Service,
) *SchedulerService {
	return &SchedulerService{
		repo:   repo,
		logger: log.WithFields(zap.String("component", "office-scheduler")),
		svc:    svc,
	}
}

// SetTaskStarter wires the orchestrator task starter.
func (ss *SchedulerService) SetTaskStarter(ts TaskStarter) {
	ss.taskStarter = ts
}

// SetRunSessionLauncher wires the Office-owned taskless launch seam.
func (ss *SchedulerService) SetRunSessionLauncher(launcher service.RunSessionLauncher) {
	ss.runSessionLauncher = launcher
}

// SetResolver wires the routing resolver. When set, dispatch goes through
// dispatchWithRouting; when nil, the legacy concrete-profile path runs.
func (ss *SchedulerService) SetResolver(r *routing.Resolver) {
	ss.resolver = r
}

// SetEventBus wires the bus used to publish routing-side WS events
// (provider_health_changed, route_attempt_appended). Optional; nil keeps
// the scheduler silent and tests don't need to stand up a bus.
func (ss *SchedulerService) SetEventBus(eb bus.EventBus) {
	ss.eb = eb
}

// SetRunsService wires the shared runs queue service (AC-CONSOLIDATION-001.6).
// When set, QueueRun delegates its insert + publish + signal to it instead of
// its own ss.repo.CreateRun, so this path gains causation resolution,
// priority stamping, and the launch-safety refusal gates for free. Optional;
// nil keeps the legacy inline path this package has always used, so existing
// tests that never call this still pass.
func (ss *SchedulerService) SetRunsService(svc *runsservice.Service) {
	ss.runsService = svc
}

// Resolver returns the wired routing resolver (may be nil).
func (ss *SchedulerService) Resolver() *routing.Resolver { return ss.resolver }

// Repo returns the underlying repository. Exposed so the service-tier
// SchedulerIntegration can call the routing-specific repo methods without
// holding an independent handle.
func (ss *SchedulerService) Repo() *sqlite.Repository { return ss.repo }

// SetAPIBaseURL sets the base URL injected into KANDEV_API_URL.
func (ss *SchedulerService) SetAPIBaseURL(url string) {
	ss.apiBaseURL = url
}

// SetAgentctlBinaryPath sets the host path to the agentctl binary.
func (ss *SchedulerService) SetAgentctlBinaryPath(path string) {
	ss.agentctlPath = path
}

// SetKandevBasePathFn sets the function used to resolve the kandev base path.
func (ss *SchedulerService) SetKandevBasePathFn(fn func() string) {
	ss.kandevBasePathFn = fn
}

// SetAgentTypeResolver sets the function that maps agent profile IDs to type IDs.
func (ss *SchedulerService) SetAgentTypeResolver(fn func(profileID string) string) {
	ss.agentTypeResolver = fn
}

// SetProjectSkillDirResolver sets the function that maps agent type IDs to
// their CWD-relative skill directories.
func (ss *SchedulerService) SetProjectSkillDirResolver(fn func(agentTypeID string) string) {
	ss.projectSkillDirResolver = fn
}

// SetPauseGate wires the workspace-pause read used by QueueRun to
// enforce the operator kill switch. Optional — when nil the gate is
// not enforced.
func (ss *SchedulerService) SetPauseGate(g shared.PauseGate) {
	ss.pauseGate = g
}

// QueueRun enqueues a run request for an agent instance, attributed to the
// system actor. It exists for shared.RunQueuer callers that predate the
// actor contract (AC-OFFICE-RUN-CAUSATION-001.15) and have no actor to
// declare; QueueRunCtx callers thread their RunContext's real actor
// through queueRunAsActor instead.
// It checks agent status, idempotency, and attempts coalescing before inserting.
// Implements shared.RunQueuer.
func (ss *SchedulerService) QueueRun(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) (shared.QueueOutcome, error) {
	outcome, err := ss.queueRunAsActor(ctx, agentInstanceID, reason, payload, idempotencyKey, "", "", models.ActorKindSystem, "")
	return outcome, err
}

// queueRunAsActor is QueueRun's actor- and wave-aware core. actorKind/
// actorID flow into the delegated runs/service causation resolution
// (workspace, priority class, causation-depth and self-trigger refusal
// gates) and, on the legacy inline fallback, into ClassifyPriority — so a
// real actor gets the same priority treatment whether or not a runs
// service is wired. A non-empty waveKey (a) skips CoalesceRun — a
// wave-carrying request is never coalesced in and a wave-carrying queued
// run is never coalesced into — and (b) classifies CreateRun's
// idx_run_wake_wave violation as an already-delivered wake rather than an
// error: the legacy inline fallback inserts directly (not through
// runs/service), so ReportInsertResult's own idx_run_idempotency-only
// classification doesn't cover it. A concurrent duplicate of this same
// request can just as well lose the race on idx_run_idempotency instead of
// idx_run_wake_wave — both keys identify the identical operation for the
// identical row, so either violation means the wake is already recorded
// and neither is an error, mirroring runs/service.QueueRun's own two-way
// classification.
func (ss *SchedulerService) queueRunAsActor(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey, waveKey, waveString string,
	actorKind models.ActorKind, actorID string,
) (runsservice.QueueOutcome, error) {
	agent, err := ss.guardAgentStatus(ctx, agentInstanceID)
	if err != nil {
		return runsservice.QueueOutcomeNone, err
	}
	if err := ss.checkPauseGate(ctx, agent); err != nil {
		return runsservice.QueueOutcomeNone, err
	}

	if ss.runsService == nil {
		// AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6: a delegating caller
		// without the authoritative API available fails its enqueue and
		// surfaces the error, rather than falling back to an insert of
		// its own. A fallback insert would bypass causation resolution
		// and the causation-depth/self-trigger refusal gates entirely —
		// precisely the ungated path this requirement removes. Production
		// always wires a runs service alongside the scheduler
		// (backendapp.startSchedulingRuntime), so this is reachable only
		// from a test that constructs a SchedulerService without calling
		// SetRunsService.
		return runsservice.QueueOutcomeNone, fmt.Errorf("queue run: no runs service configured")
	}

	causingRunID, err := ss.resolveCausingRunID(ctx, actorKind, actorID)
	if err != nil {
		return runsservice.QueueOutcomeNone, err
	}

	// The assignment allowance is scoped to the scheduler's mutation
	// producer. Other producers do not carry actor_type=agent and therefore
	// pass through this fail-open gate without consuming its allowance.
	if refused := ss.checkAssignmentWakeAllowance(ctx, agentInstanceID, reason, payload); refused {
		return runsservice.QueueOutcomeRateLimited, nil
	}

	return ss.runsService.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:         reason,
		IdempotencyKey: idempotencyKey,
		Payload:        service.PayloadWithAgent(payload, agentInstanceID),
		ActorKind:      actorKind,
		ActorID:        actorID,
		CausingRunID:   causingRunID,
		WakeWaveKey:    waveKey,
		WakeWaveString: waveString,
	})
}

// resolveCausingRunID looks up the acting agent's own live claimed run,
// so a reactivity-triggered wake (no explicit CausingRunID or task-
// boundary carrier the way a workflow-engine action has) still inherits
// the causation chain of whatever the actor was doing when it caused
// this wake, instead of every such wake resolving as a fresh root
// cause. Returns "" for a non-agent actor or an agent with no live
// claimed run, in which case causation resolution roots the new run
// exactly as it did before this lookup existed.
func (ss *SchedulerService) resolveCausingRunID(ctx context.Context, actorKind models.ActorKind, actorID string) (string, error) {
	if actorKind != models.ActorKindAgent || actorID == "" {
		return "", nil
	}
	run, err := ss.repo.RunsRepository().GetClaimedRunForCausationAttribution(ctx, actorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("resolve causing run: %w", err)
	}
	return run.ID, nil
}

// QueueRunCtx is the typed variant of QueueRun that takes a structured
// RunContext. The context is JSON-encoded into the payload column so the
// agent runtime can deserialise it. The idempotency key is c.IdempotencyKey
// verbatim — an empty key enqueues with no dedup identity rather than
// falling back to a "{reason}:{taskID}:{agentID}" default that would be
// permanently unique per (reason, task, agent) and silently swallow every
// later legitimate occurrence for the same triple.
//
// Unlike QueueRun, this threads c's real actor (ActorType/ActorID) through
// to causation resolution instead of defaulting to system
// (AC-OFFICE-RUN-CAUSATION-001.15) — every QueueRunCtx call site (approval
// resolution, reactivity) already knows the human or agent that caused the
// wake; it was only ever discarded at this boundary.
func (ss *SchedulerService) QueueRunCtx(
	ctx context.Context, agentInstanceID string, c RunContext,
) (runsservice.QueueOutcome, error) {
	payload, err := encodeRunContext(c)
	if err != nil {
		return runsservice.QueueOutcomeNone, fmt.Errorf("encode run context: %w", err)
	}
	actorKind, actorID := actorFromRunContext(c)
	return ss.queueRunAsActor(ctx, agentInstanceID, c.Reason, payload, c.IdempotencyKey, c.WaveKey, c.WaveString, actorKind, actorID)
}

// actorFromRunContext maps RunContext's loosely-typed ActorType string
// ("user" | "agent" | "", set independently across many reactivity/
// approval call sites) onto models.ActorKind, applying the same
// fail-restrictive rule runs/service.normalizeActor applies for a
// delegated request: an unrecognised ActorType, or an "agent" ActorType
// with no ActorID, resolves to ActorKindSystem and is counted
// (AC-OFFICE-RUN-CAUSATION-001.16) so a caller failing to declare its
// actor stays visible. A "user" ActorType is never downgraded for a
// missing ActorID — a browser-originated status/assignee change has none
// to give — so it always resolves to ActorKindUser.
func actorFromRunContext(c RunContext) (models.ActorKind, string) {
	switch c.ActorType {
	case "user":
		return models.ActorKindUser, c.ActorID
	case assignmentWakeActorTypeAgent:
		if c.ActorID != "" {
			return models.ActorKindAgent, c.ActorID
		}
	}
	shared.LaunchActorMissingTotal.Add(shared.LaunchSafetyLabel("reason", c.Reason), 1)
	return models.ActorKindSystem, ""
}

// encodeRunContext JSON-encodes c. When c.ExtraPayload is empty the output
// is a plain struct marshal, byte-identical to before ExtraPayload existed.
// Otherwise ExtraPayload's keys are overlaid onto the encoded object, then
// c's own envelope fields are re-applied on top — workflow-authored content
// keys win, but a workflow-authored payload can never redirect the run's
// identity. This mirrors runs/service.runPayload's precedence for task_id
// and workflow_step_id: P1 never goes through that function (it inserts via
// ss.repo.CreateRun directly), so encodeRunContext is the only place that
// guarantee can be enforced for the cascade path. Without it, a queue_run
// action's payload.task_id would silently override task.ParentID and
// misdirect the wake to a foreign task — task_id is what
// SchedulerIntegration.extractTaskID reads to check out and budget the run.
//
// Unlike runPayload, this does not re-assert agent_profile_id: RunContext
// carries no typed recipient field to re-assert from (the run's actual
// AgentProfileID column is set separately, from queueRun's own
// agentInstanceID parameter, never from this payload). A workflow-authored
// ExtraPayload["agent_profile_id"] therefore passes through unfiltered —
// currently inert, since no reader in this codebase consults
// payload["agent_profile_id"] for dispatch or routing (both use the DB
// column instead). See
// TestQueueRunCtx_ExtraPayloadAgentProfileID_PassesThroughUnfiltered, which
// pins this as a known non-guarantee rather than an oversight.
func encodeRunContext(c RunContext) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	if len(c.ExtraPayload) == 0 {
		return string(b), nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return "", err
	}
	for k, v := range c.ExtraPayload {
		m[k] = v
	}
	m["task_id"] = c.TaskID
	m["reason"] = c.Reason
	if c.WorkspaceID != "" {
		m["workspace_id"] = c.WorkspaceID
	} else {
		delete(m, "workspace_id")
	}
	if c.ChildTaskID != "" {
		m["child_task_id"] = c.ChildTaskID
	} else {
		delete(m, "child_task_id")
	}
	if c.WorkflowStepID != "" {
		m["workflow_step_id"] = c.WorkflowStepID
	} else {
		delete(m, "workflow_step_id")
	}
	merged, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}

// guardAgentStatus returns an error if the agent is paused or stopped,
// and otherwise the resolved agent — checkPauseGate reuses this fetch
// instead of looking the agent up a second time.
func (ss *SchedulerService) guardAgentStatus(ctx context.Context, agentInstanceID string) (*models.AgentInstance, error) {
	agent, err := ss.svc.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return nil, fmt.Errorf("get agent instance: %w", err)
	}
	switch agent.Status {
	case models.AgentStatusPaused:
		return nil, fmt.Errorf("agent %s is paused", agentInstanceID)
	case models.AgentStatusStopped:
		return nil, fmt.Errorf("agent %s is stopped", agentInstanceID)
	case models.AgentStatusPendingApproval:
		return nil, fmt.Errorf("agent %s is pending approval", agentInstanceID)
	}
	return agent, nil
}

// checkPauseGate blocks queuing when agent's workspace is paused (the
// operator kill switch). Takes the already-resolved agent — usually
// guardAgentStatus's return value — rather than re-resolving it, so the
// two checks can never see two different snapshots of the agent's
// workspace. Fails closed on a gate-read error
// (shared.ErrPauseGateUnavailable) — this write hasn't happened yet, so
// failing the call is the whole retry story; the caller's own retry (or
// the next event) tries again.
func (ss *SchedulerService) checkPauseGate(ctx context.Context, agent *models.AgentInstance) error {
	if ss.pauseGate == nil {
		return nil
	}
	active, err := ss.pauseGate.PauseState(ctx, agent.WorkspaceID)
	if err != nil {
		pause.RecordGateError("scheduler_queue_run")
		ss.logger.Warn("queue run: pause gate read failed",
			zap.String("agent", agent.ID), zap.Error(err))
		return shared.ErrPauseGateUnavailable
	}
	if active != nil {
		pause.RecordBlocked("scheduler_queue_run")
		return shared.ErrWorkspacePaused
	}
	return nil
}
