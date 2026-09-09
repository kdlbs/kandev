// Package scheduler orchestrates run processing for the office domain.
// It wraps service.Service and owns the tick loop, retry logic, event
// subscribers, and idle-timeout management.
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
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
type RouteOverride struct {
	ExecutionProfileID string
	ProviderID         string
	Model              string
	Tier               string
	Mode               string
	Flags              []string
	Env                map[string]string
}

// LaunchContext is an alias for service.LaunchContext so dispatch
// callsites inside this package can spell the type without re-imports.
// The scheduler-side dispatcher takes a service.LaunchContext directly.
// See service.LaunchContext for field semantics.
type LaunchContext = service.LaunchContext

// Run reason constants.
const (
	RunReasonTaskAssigned          = "task_assigned"
	RunReasonTaskComment           = "task_comment"
	RunReasonTaskBlockersResolved  = "task_blockers_resolved"
	RunReasonTaskChildrenCompleted = "task_children_completed"
	RunReasonApprovalResolved      = "approval_resolved"
	RunReasonRoutineTrigger        = "routine_trigger"
	// RunReasonHeartbeat aliases shared.RunReasonHeartbeat so this package's
	// local constant and the shared idle-skip classifier cannot drift apart
	// the way the un-aliased pair did before WO-46 (Review round 1, S2).
	RunReasonHeartbeat   = shared.RunReasonHeartbeat
	RunReasonBudgetAlert = "budget_alert"
	RunReasonAgentError  = "agent_error"

	// Reactivity-pipeline reasons.
	RunReasonTaskUnblocked         = "task_unblocked"            // status: blocked → not blocked
	RunReasonTaskReopened          = "task_reopened"             // silent reopen (status only)
	RunReasonTaskReopenedComment   = "task_reopened_via_comment" // user comment on closed task or resume:true
	RunReasonTaskMentioned         = "task_mentioned"            // @mention in comment, additive to assignee wake
	RunReasonStagePending          = "stage_pending"             // execution policy advanced to a new stage
	RunReasonStageChangesRequested = "stage_changes_requested"   // reviewer asked for rework

	// Approval-flow reactivity reasons (B5).
	RunReasonTaskReviewRequested  = "task_review_requested"  // task entered in_review; ping reviewers/approvers
	RunReasonTaskChangesRequested = "task_changes_requested" // a reviewer/approver asked for changes
	RunReasonTaskReadyToClose     = "task_ready_to_close"    // all approvers have approved; assignee may close
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

	// IdempotencyKey, when non-empty, overrides the default
	// "{reason}:{taskID}:{agentID}" key QueueRunCtx mints. The default
	// key is permanently unique per (reason, task, agent) — fine for
	// reasons that fire at most once per task, wrong for a reason that
	// can legitimately recur (e.g. task_children_completed across
	// repeated delegation waves). Callers that recur build their own
	// key that changes with the thing that makes each occurrence
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
	taskStarter             TaskStarter
	resolver                *routing.Resolver
	eb                      bus.EventBus
	apiBaseURL              string
	agentctlPath            string
	kandevBasePathFn        func() string
	agentTypeResolver       func(profileID string) string
	projectSkillDirResolver func(agentTypeID string) string
	workflowStepGetter      WorkflowStepGetter
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

// QueueRun enqueues a run request for an agent instance.
// It checks agent status, idempotency, and attempts coalescing before inserting.
// Implements shared.RunQueuer.
func (ss *SchedulerService) QueueRun(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	return ss.queueRun(ctx, agentInstanceID, reason, payload, idempotencyKey, "", "")
}

// queueRun is QueueRun plus an optional completion-wave identity. A
// non-empty waveKey (a) skips CoalesceRun — a wave-carrying request is
// never coalesced in and a wave-carrying queued run is never coalesced
// into — and (b) classifies CreateRun's idx_run_wake_wave violation as an
// already-delivered wake rather than an error: this call site inserts
// directly (not through runs/service), so runs/service's own
// classification doesn't cover it.
func (ss *SchedulerService) queueRun(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey, waveKey, waveString string,
) error {
	if err := ss.guardAgentStatus(ctx, agentInstanceID); err != nil {
		return err
	}

	if idempotencyKey != "" {
		dup, err := ss.repo.CheckIdempotencyKey(ctx, idempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			ss.logger.Debug("run skipped (idempotent)",
				zap.String("key", idempotencyKey))
			return nil
		}
	}

	if waveKey == "" {
		coalesced, err := ss.repo.CoalesceRun(ctx, agentInstanceID, reason, CoalesceWindowSeconds, payload)
		if err != nil {
			return fmt.Errorf("coalesce check: %w", err)
		}
		if coalesced {
			ss.logger.Debug("run coalesced",
				zap.String("agent", agentInstanceID),
				zap.String("reason", reason))
			return nil
		}
	}

	var idemKeyPtr *string
	if idempotencyKey != "" {
		idemKeyPtr = &idempotencyKey
	}
	req := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: agentInstanceID,
		Reason:         reason,
		Payload:        payload,
		Status:         RunStatusQueued,
		CoalescedCount: 1,
		IdempotencyKey: idemKeyPtr,
		WakeWaveKey:    waveKey,
		WakeWaveString: waveString,
		RequestedAt:    time.Now().UTC(),
	}
	if err := ss.repo.CreateRun(ctx, req); err != nil {
		if waveKey != "" && runssqlite.IsWakeWaveUniqueViolation(err) {
			shared.ParentWakeDedupedTotal.Add(1)
			ss.logger.Debug("run skipped (wave already woken)",
				zap.String("wave_key", waveKey))
			return nil
		}
		return fmt.Errorf("enqueue run: %w", err)
	}

	ss.logger.Info("run queued",
		zap.String("id", req.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", reason))
	return nil
}

// QueueRunCtx is the typed variant of QueueRun that takes a
// structured RunContext. The context is JSON-encoded into the
// payload column so the agent runtime can deserialise it. The
// idempotency key is c.IdempotencyKey when the caller set one;
// otherwise it defaults to "{reason}:{taskID}:{agentID}" so the same
// agent never gets two runs for the same task+reason within the
// idempotency window.
func (ss *SchedulerService) QueueRunCtx(
	ctx context.Context, agentInstanceID string, c RunContext,
) error {
	payload, err := encodeRunContext(c)
	if err != nil {
		return fmt.Errorf("encode run context: %w", err)
	}
	idempotencyKey := c.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("%s:%s:%s", c.Reason, c.TaskID, agentInstanceID)
	}
	return ss.queueRun(ctx, agentInstanceID, c.Reason, payload, idempotencyKey, c.WaveKey, c.WaveString)
}

// encodeRunContext JSON-encodes c. When c.ExtraPayload is empty the output
// is a plain struct marshal, byte-identical to before ExtraPayload existed.
// Otherwise ExtraPayload's keys are overlaid onto the encoded object, then
// c's own envelope fields are re-applied on top — workflow-authored content
// keys win, but a workflow-authored payload can never redirect the run's
// identity. This mirrors runs/service.runPayload's precedence, which copies
// the caller's payload and then unconditionally re-asserts task_id,
// workflow_step_id and agent_profile_id from the typed request: P1 never
// goes through that function (it inserts via ss.repo.CreateRun directly),
// so encodeRunContext is the only place that guarantee can be enforced for
// the cascade path. Without it, a queue_run action's payload.task_id would
// silently override task.ParentID and misdirect the wake to a foreign task
// — task_id is what SchedulerIntegration.extractTaskID reads to check out
// and budget the run.
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

// guardAgentStatus returns an error if the agent is paused or stopped.
func (ss *SchedulerService) guardAgentStatus(ctx context.Context, agentInstanceID string) error {
	agent, err := ss.svc.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return fmt.Errorf("get agent instance: %w", err)
	}
	switch agent.Status {
	case models.AgentStatusPaused:
		return fmt.Errorf("agent %s is paused", agentInstanceID)
	case models.AgentStatusStopped:
		return fmt.Errorf("agent %s is stopped", agentInstanceID)
	case models.AgentStatusPendingApproval:
		return fmt.Errorf("agent %s is pending approval", agentInstanceID)
	}
	return nil
}
