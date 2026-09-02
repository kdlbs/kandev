package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

var (
	ErrTaskPlanNotFound     = errors.New("task plan not found")
	ErrTaskIDRequired       = errors.New("task_id is required")
	ErrContentRequired      = errors.New("content is required")
	ErrSessionIDRequired    = errors.New("session_id is required")
	ErrRevisionNotFound     = errors.New("task plan revision not found")
	ErrRevisionIDRequired   = errors.New("target_revision_id is required")
	ErrRevisionTaskMismatch = errors.New("revision does not belong to given task")
	ErrSessionTaskMismatch  = errors.New("session does not belong to given task")
)

const (
	createdByAgent             = "agent"
	createdByUser              = "user"
	defaultCoalesceWindow      = 5 * time.Minute
	defaultAgentAuthorFallback = "Agent"
	defaultUserAuthorFallback  = "User"
)

// planRepo is the repository surface this service depends on. It combines the
// plan-revision storage with a tiny slice of session lookups used to resolve
// the active session's agent profile name when the MCP path doesn't provide
// an explicit author_name.
type planRepo interface {
	repository.PlanRepository
	GetActiveTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetTaskSessionByTaskID(ctx context.Context, taskID string) (*models.TaskSession, error)
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetTask(ctx context.Context, id string) (*models.Task, error)
}

// PlanWorkflowStepGetter resolves a workflow step's display info so plan
// revisions can be stamped with the step the task was on at write time.
// Narrower than taskservice.WorkflowStepGetter (GetStep only) since that's
// all this stamp needs; callers wire the same underlying adapter.
type PlanWorkflowStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
}

// PlanService provides task plan business logic.
type PlanService struct {
	repo           planRepo
	eventBus       bus.EventBus
	logger         *logger.Logger
	coalesceWindow time.Duration
	locks          *planLockTable
	// authorizeTask gates plan access by the task's workspace ownership
	// (opt-in auth). Nil = unscoped (internal callers / auth disabled).
	authorizeTask func(ctx context.Context, taskID string) error
	// workflowStepGetter resolves the task's current workflow step for the
	// write-time stamp. Nil is safe: stamped fields stay empty.
	workflowStepGetter PlanWorkflowStepGetter
}

// NewPlanService creates a new task plan service. The concrete repository
// passed by callers must implement both PlanRepository and the session-lookup
// methods on planRepo (the SQLite repository does both).

func NewPlanService(repo planRepo, eventBus bus.EventBus, log *logger.Logger, configuredWindow ...time.Duration) *PlanService {
	coalesceWindow := defaultCoalesceWindow
	if len(configuredWindow) > 0 {
		coalesceWindow = resolveCoalesceWindow(configuredWindow[0])
	}
	return &PlanService{
		repo:           repo,
		eventBus:       eventBus,
		logger:         log.WithFields(zap.String("component", "plan-service")),
		coalesceWindow: coalesceWindow,
		locks:          newPlanLockTable(),
	}
}

// SetTaskAuthorizer wires the per-user task-access check (opt-in auth). The
// authorizer must return nil for contexts without a request identity.
func (s *PlanService) SetTaskAuthorizer(fn func(ctx context.Context, taskID string) error) {
	s.authorizeTask = fn
}

// SetWorkflowStepGetter wires the workflow step lookup used to stamp plan
// revisions with the task's step at write time. Optional; leaving it unset
// keeps every existing caller working with the stamp fields left empty.
func (s *PlanService) SetWorkflowStepGetter(getter PlanWorkflowStepGetter) {
	s.workflowStepGetter = getter
}

func (s *PlanService) authorize(ctx context.Context, taskID string) error {
	if s.authorizeTask == nil {
		return nil
	}
	return s.authorizeTask(ctx, taskID)
}

// resolveCoalesceWindow returns an explicitly configured window unchanged,
// including zero or negative values. Only an absent configuration (no
// argument passed to NewPlanService) falls back to the five-minute default;
// a caller who explicitly configures a non-positive window means "never
// coalesce", and canCoalesce's own <= 0 guard already honors that (AC-002.8).
// This function used to clamp a negative value back up to the default,
// which made that configuration unreachable.
func resolveCoalesceWindow(window time.Duration) time.Duration {
	return window
}

// CreatePlanRequest contains parameters for creating/updating a task plan.
// AuthorKind and AuthorName are optional; when absent they are derived from CreatedBy.
type CreatePlanRequest struct {
	TaskID     string
	Title      string
	Content    string
	CreatedBy  string // "agent" | "user"
	AuthorKind string // optional explicit override
	AuthorName string // optional; display snapshot
	// ForceNewRevision skips coalescing for this write even if it would
	// otherwise qualify (same author within the coalesce window). Callers set
	// this when the write looks like an accidental whole-document truncation:
	// coalescing would merge the destructive content into the prior revision
	// row in place, destroying the only recovery path. Zero value (false)
	// preserves today's coalescing behavior for every existing caller.
	ForceNewRevision bool
	// EvaluateTruncation opts this write into the whole-document-truncation
	// guard (AC-003.2/003.4). The MCP/agent write paths set this; the browser
	// write path does not, since the browser plan editor already shows the
	// user a diff and revision history before they save.
	EvaluateTruncation bool
}

// PlanWriteResult is returned by CreatePlan/UpdatePlan. Plan is always
// populated on success. TruncationDetected and the fields below it are only
// meaningful when EvaluateTruncation was set on the request.
type PlanWriteResult struct {
	Plan                *models.TaskPlan
	TruncationDetected  bool
	ReplacedRunes       int
	NewRunes            int
	PriorRevisionNumber int // 0 means not established
}

// planWriteOutcome carries upsertPlan's result plus the event-publishing
// details its caller must emit after releasing the per-task lock.
type planWriteOutcome struct {
	result    PlanWriteResult
	rev       *models.TaskPlanRevision
	coalesced bool
	eventType string
}

// planHeadState disambiguates why a HEAD read produced no usable plan: never
// existed vs. failed to read. The two cases require different behavior
// (AC-001.6, AC-002.11), so collapsing them into a single nil would erase
// that distinction.
type planHeadState int

const (
	planHeadFound planHeadState = iota
	planHeadAbsent
	planHeadUnknown
)

func (s *PlanService) readPlanHead(ctx context.Context, taskID string) (*models.TaskPlan, planHeadState) {
	plan, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		return nil, planHeadUnknown
	}
	if plan == nil {
		return nil, planHeadAbsent
	}
	return plan, planHeadFound
}

// planRevisionState is the revision-history analog of planHeadState.
type planRevisionState int

const (
	planRevisionFound planRevisionState = iota
	planRevisionAbsent
	planRevisionUnknown
)

func (s *PlanService) readLatestRevision(ctx context.Context, taskID string) (*models.TaskPlanRevision, planRevisionState) {
	rev, err := s.repo.GetLatestTaskPlanRevision(ctx, taskID)
	if err != nil {
		return nil, planRevisionUnknown
	}
	if rev == nil {
		return nil, planRevisionAbsent
	}
	return rev, planRevisionFound
}

// CreatePlan upserts a plan and appends or coalesces a revision.
func (s *PlanService) CreatePlan(ctx context.Context, req CreatePlanRequest) (PlanWriteResult, error) {
	if req.TaskID == "" {
		return PlanWriteResult{}, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return PlanWriteResult{}, err
	}
	release := s.locks.acquire(req.TaskID)
	defer release()
	outcome, err := s.upsertPlan(ctx, req, false)
	release()
	if err != nil {
		return PlanWriteResult{}, err
	}
	s.publishPlanEvent(ctx, outcome.eventType, outcome.result.Plan)
	s.publishRevisionEvent(ctx, outcome.rev, outcome.coalesced)
	return outcome.result, nil
}

// UpdatePlanRequest mirrors CreatePlanRequest; kept as a distinct type for API clarity.
type UpdatePlanRequest struct {
	TaskID             string
	Title              string
	Content            string
	CreatedBy          string
	AuthorKind         string
	AuthorName         string
	ForceNewRevision   bool
	EvaluateTruncation bool
}

// UpdatePlan updates an existing plan (errors if missing).
func (s *PlanService) UpdatePlan(ctx context.Context, req UpdatePlanRequest) (PlanWriteResult, error) {
	if req.TaskID == "" {
		return PlanWriteResult{}, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return PlanWriteResult{}, err
	}
	release := s.locks.acquire(req.TaskID)
	defer release()
	outcome, err := s.upsertPlan(ctx, CreatePlanRequest(req), true)
	release()
	if err != nil {
		return PlanWriteResult{}, err
	}
	s.publishPlanEvent(ctx, outcome.eventType, outcome.result.Plan)
	s.publishRevisionEvent(ctx, outcome.rev, outcome.coalesced)
	return outcome.result, nil
}

// upsertPlan is the shared write path. It upserts the task_plans HEAD row and either
// coalesces into the latest revision (same author within window) or appends a new revision
// — both steps run in one write transaction via WritePlanRevision so HEAD and history
// cannot diverge under concurrent writers or partial failures.
//
// Callers must hold this task's per-task lock (s.locks) before calling. This method does not
// acquire or release it, and does not publish events — its caller does both, so that the lock
// is released before any event is published (see PlanService's package doc / the system design's
// "critical section scope" for why: holding the lock across a publish would serialize unrelated
// subscriber work behind a task's write lock for no reason).
//
// requireExistingHead is true for UpdatePlan (fail with ErrTaskPlanNotFound when no HEAD row
// exists) and false for CreatePlan (tolerate an absent HEAD and treat the write as a create). It
// also gates whether an empty Title/CreatedBy on the request falls back to the existing HEAD row's
// value (UpdatePlan's contract) or a literal default (CreatePlan's contract, unchanged - AC-004.1).
func (s *PlanService) upsertPlan(ctx context.Context, req CreatePlanRequest, requireExistingHead bool) (planWriteOutcome, error) {
	// Pre-transaction reads run on a context decoupled from the caller's so that a caller who
	// cancels while queued for the lock cannot have those reads misdiagnosed as HEAD/revision
	// read failures (AC-005.7). The write transaction itself still uses ctx, so a cancelled
	// caller's write still fails - just at the transaction, not at an earlier read.
	readCtx := context.WithoutCancel(ctx)

	headPlan, headState := s.readPlanHead(readCtx, req.TaskID)
	if requireExistingHead && headState == planHeadAbsent {
		return planWriteOutcome{}, ErrTaskPlanNotFound
	}

	req, preserveTitle, preserveCreatedBy := s.resolveHeadFallbacks(ctx, req, headPlan, headState, requireExistingHead)

	title := req.Title
	if title == "" {
		title = "Plan"
	}
	authorKind, authorName, createdBy := resolveAuthor(req)

	eventType := events.TaskPlanCreated
	if headState != planHeadAbsent {
		eventType = events.TaskPlanUpdated
	}

	plan := &models.TaskPlan{
		TaskID:    req.TaskID,
		Title:     title,
		Content:   req.Content,
		CreatedBy: createdBy,
	}
	if headState == planHeadFound {
		plan.ID = headPlan.ID
		plan.CreatedAt = headPlan.CreatedAt
	}

	latest, latestState := s.readLatestRevision(readCtx, req.TaskID)
	rb := s.buildRevision(readCtx, req, headPlan, headState, latest, authorKind, authorName, title)

	if err := s.repo.WritePlanRevision(ctx, plan, rb.rev, rb.coalesceID, preserveTitle, preserveCreatedBy); err != nil {
		s.logPlanWriteError(req.TaskID, err)
		return planWriteOutcome{}, err
	}

	result := PlanWriteResult{TruncationDetected: rb.truncationDetected}
	if rb.truncationDetected {
		result.ReplacedRunes = models.PlanContentLength(rb.replacedContent)
		result.NewRunes = models.PlanContentLength(req.Content)
		if latestState == planRevisionFound && latest.Content == rb.replacedContent {
			result.PriorRevisionNumber = latest.RevisionNumber
		}
	}

	result.Plan = s.finalizePlanIdentity(ctx, req.TaskID, plan, headState)
	return planWriteOutcome{result: result, rev: rb.rev, coalesced: rb.coalesce, eventType: eventType}, nil
}

// resolveHeadFallbacks applies UpdatePlan's "an omitted title/created_by falls back to the
// existing HEAD row's value" contract (AC-004.1, only when requireExistingHead and a HEAD row was
// actually found) and resolves a missing AuthorName for agent writes from the active session's
// profile snapshot. It also computes the preserve-on-unknown-state flags WritePlanRevision needs:
// when the HEAD read failed we cannot tell whether the caller's empty title/created_by should fall
// back to a real stored value or a literal default, so the safest behavior is to leave the stored
// value alone (AC-001.9).
func (s *PlanService) resolveHeadFallbacks(
	ctx context.Context,
	req CreatePlanRequest,
	headPlan *models.TaskPlan,
	headState planHeadState,
	requireExistingHead bool,
) (CreatePlanRequest, bool, bool) {
	origTitleEmpty := req.Title == ""
	origCreatedByEmpty := req.CreatedBy == ""
	preserveTitle := headState == planHeadUnknown && origTitleEmpty
	preserveCreatedBy := headState == planHeadUnknown && origCreatedByEmpty

	if origTitleEmpty && requireExistingHead && headState == planHeadFound {
		req.Title = headPlan.Title
	}
	if origCreatedByEmpty && requireExistingHead && headState == planHeadFound {
		req.CreatedBy = headPlan.CreatedBy
	}

	// The MCP path (handleCreateTaskPlan / handleUpdateTaskPlan) doesn't carry the agent's
	// display name in the request, so without this lookup every agent revision would render as
	// "Agent" in the history UI.
	if req.AuthorName == "" {
		kindHint := req.AuthorKind
		if kindHint == "" {
			kindHint = req.CreatedBy
		}
		if kindHint == createdByAgent {
			req.AuthorName = s.resolveAgentDisplayName(ctx, req.TaskID)
		}
	}

	return req, preserveTitle, preserveCreatedBy
}

// revisionBuild carries buildRevision's outputs: the revision row to write, the optional
// coalesce target, and the truncation-detection results upsertPlan needs for its result.
type revisionBuild struct {
	rev                *models.TaskPlanRevision
	coalesceID         *string
	coalesce           bool
	truncationDetected bool
	replacedContent    string
}

// buildRevision decides whether this write coalesces into the latest revision or appends a new
// one, and assembles the revision row to persist.
func (s *PlanService) buildRevision(
	ctx context.Context,
	req CreatePlanRequest,
	headPlan *models.TaskPlan,
	headState planHeadState,
	latest *models.TaskPlanRevision,
	authorKind, authorName, title string,
) revisionBuild {
	replacedContent := ""
	haveReplacedContent := headState == planHeadFound
	if haveReplacedContent {
		replacedContent = headPlan.Content
	}
	truncationDetected := req.EvaluateTruncation && haveReplacedContent &&
		planTruncationDetected(replacedContent, req.Content)

	// A HEAD read that didn't find an existing row (absent or unknown) forces an append rather
	// than a coalesce decision based on stale/unrelated history (AC-001.6, AC-002.11): there is
	// no known-good "prior" revision this write could safely fold into. Truncation forces an
	// append for a different reason - AC-001.5, so the destructive write can't overwrite the
	// only surviving copy of the content it replaced.
	forceAppend := req.ForceNewRevision || headState != planHeadFound || truncationDetected
	now := time.Now().UTC()
	coalesce := !forceAppend && s.canCoalesce(latest, authorKind, authorName, now)

	rev := &models.TaskPlanRevision{
		TaskID:     req.TaskID,
		Title:      title,
		Content:    req.Content,
		AuthorKind: authorKind,
		AuthorName: authorName,
	}
	var coalesceID *string
	if coalesce {
		coalesceID = &latest.ID
		// Preserve the original revision's author, number, and workflow-step
		// stamp on merge — the DB row's stamp columns aren't touched by the
		// merge UPDATE either, so this keeps the in-memory rev (used for the
		// revision event payload) consistent with what's actually persisted.
		rev.RevisionNumber = latest.RevisionNumber
		rev.AuthorKind = latest.AuthorKind
		rev.AuthorName = latest.AuthorName
		rev.CreatedAt = latest.CreatedAt
		rev.WorkflowStepID = latest.WorkflowStepID
		rev.WorkflowStepName = latest.WorkflowStepName
		rev.WorkflowStepColor = latest.WorkflowStepColor
	} else {
		rev.WorkflowStepID, rev.WorkflowStepName, rev.WorkflowStepColor = s.currentWorkflowStepStamp(ctx, req.TaskID)
	}

	return revisionBuild{
		rev:                rev,
		coalesceID:         coalesceID,
		coalesce:           coalesce,
		truncationDetected: truncationDetected,
		replacedContent:    replacedContent,
	}
}

// finalizePlanIdentity re-reads the just-written HEAD row so callers get authoritative persisted
// values. AC-005.8: the write already committed, so a failed or empty re-read must still report
// success using the in-memory plan the write assembled. When the pre-write HEAD read was itself
// unknown, that in-memory plan's ID/CreatedAt may be a value upsertPlanHead fabricated for an
// insert branch that never actually ran (the real row, if any, kept its own identity via ON
// CONFLICT) - clear both so the caller reads the identity as unknown rather than as a row that
// doesn't exist.
func (s *PlanService) finalizePlanIdentity(
	ctx context.Context,
	taskID string,
	plan *models.TaskPlan,
	headState planHeadState,
) *models.TaskPlan {
	saved, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil || saved == nil {
		if headState == planHeadUnknown {
			plan.ID = ""
			plan.CreatedAt = time.Time{}
		}
		return plan
	}
	return saved
}

// resolveAgentDisplayName returns the agent profile's display name for the
// task's most recent session, or "" if no usable session/snapshot exists.
// Tries the active session first (running/starting/waiting) and falls back to
// the most recent session by started_at so plans written between turns still
// get the right author name.
func (s *PlanService) resolveAgentDisplayName(ctx context.Context, taskID string) string {
	session, err := s.repo.GetActiveTaskSessionByTaskID(ctx, taskID)
	if err != nil || session == nil {
		session, err = s.repo.GetTaskSessionByTaskID(ctx, taskID)
		if err != nil || session == nil {
			return ""
		}
	}
	return agentDisplayNameFromSnapshot(session.AgentProfileSnapshot)
}

// agentDisplayNameFromSnapshot picks the best available display name from a
// session's agent_profile_snapshot. The orchestrator's canonical key is
// "name" (the profile's display name, e.g. "Claude Sonnet 4.5"); we try it
// first so a snapshot that carries both "name" and a stale older "label"
// doesn't render the stale value. Falls back through "label" (older paths)
// and "agent_display_name" (some DTO mappings) before giving up.
func agentDisplayNameFromSnapshot(snapshot map[string]interface{}) string {
	if snapshot == nil {
		return ""
	}
	for _, key := range []string{"name", "label", "agent_display_name"} {
		if v, ok := snapshot[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// currentWorkflowStepStamp resolves the task's current workflow step display
// snapshot for a new revision write. Returns empty strings whenever the
// getter isn't wired, the task or its step can't be read, or the task has no
// step set — this is observability metadata, not a guard, so any failure
// here degrades to "no step recorded" rather than failing the write.
func (s *PlanService) currentWorkflowStepStamp(ctx context.Context, taskID string) (id, name, color string) {
	if s.workflowStepGetter == nil {
		return "", "", ""
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.WorkflowStepID == "" {
		return "", "", ""
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil || step == nil {
		return "", "", ""
	}
	return step.ID, step.Name, step.Color
}

func (s *PlanService) canCoalesce(latest *models.TaskPlanRevision, authorKind, authorName string, now time.Time) bool {
	if latest == nil {
		return false
	}
	if latest.RevertOfRevisionID != nil {
		return false // revert markers are permanent
	}
	if latest.AuthorKind != authorKind || latest.AuthorName != authorName {
		return false
	}
	if s.coalesceWindow <= 0 {
		return false
	}
	return now.Sub(latest.UpdatedAt) < s.coalesceWindow
}

// resolveAuthor derives the authoritative (kind, name, legacyCreatedBy) tuple
// from a write request. Callers may pass explicit AuthorKind/AuthorName; when
// absent we fall back to CreatedBy and a literal display name.
func resolveAuthor(req CreatePlanRequest) (kind, name, createdBy string) {
	createdBy = req.CreatedBy
	kind = req.AuthorKind
	if kind == "" {
		kind = createdBy
	}
	if kind != createdByAgent && kind != createdByUser {
		kind = createdByAgent
	}
	if createdBy == "" {
		createdBy = kind
	}
	name = req.AuthorName
	if name == "" {
		if kind == createdByAgent {
			name = defaultAgentAuthorFallback
		} else {
			name = defaultUserAuthorFallback
		}
	}
	return kind, name, createdBy
}

// GetPlan retrieves a task plan by task ID. Returns nil, nil if missing.
func (s *PlanService) GetPlan(ctx context.Context, taskID string) (*models.TaskPlan, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	return s.repo.GetTaskPlan(ctx, taskID)
}

type MarkImplementationStartedRequest struct {
	TaskID    string
	SessionID string
	Actor     string
}

func (s *PlanService) MarkImplementationStarted(ctx context.Context, req MarkImplementationStartedRequest) (*models.TaskPlan, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}
	if req.SessionID == "" {
		return nil, ErrSessionIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}
	session, err := s.repo.GetTaskSession(ctx, req.SessionID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return nil, ErrSessionTaskMismatch
		}
		return nil, err
	}
	if session == nil || session.TaskID != req.TaskID {
		return nil, ErrSessionTaskMismatch
	}
	actor := req.Actor
	if actor == "" {
		actor = createdByUser
	}
	plan, err := s.repo.MarkTaskPlanImplementationStarted(ctx, req.TaskID, req.SessionID, actor)
	if err != nil {
		if errors.Is(err, repository.ErrTaskPlanNotFound) {
			return nil, ErrTaskPlanNotFound
		}
		return nil, err
	}
	if plan == nil {
		return nil, ErrTaskPlanNotFound
	}
	s.publishPlanEvent(ctx, events.TaskPlanUpdated, plan)
	return plan, nil
}

// DeletePlan removes a plan and all its revisions (cascade via FK when task goes; here we delete only HEAD).
// Historical revisions remain for audit; callers wanting a full wipe should delete the task.
// DeletePlan holds this task's write lock across the existence check and the delete so a
// concurrent CreatePlan/UpdatePlan cannot recreate the row between them (F37). The lock is
// released on every path — including both error returns — before the caller sees a result, and
// the deletion event is published only after release, matching the write paths' publish-outside-
// lock discipline.
func (s *PlanService) DeletePlan(ctx context.Context, taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return err
	}
	release := s.locks.acquire(taskID)
	defer release()
	existing, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		release()
		return err
	}
	if existing == nil {
		release()
		return ErrTaskPlanNotFound
	}
	if err := s.repo.DeleteTaskPlan(ctx, taskID); err != nil {
		release()
		return err
	}
	release()
	s.publishPlanEvent(ctx, events.TaskPlanDeleted, existing)
	return nil
}

// ListRevisions returns every plan revision for a task, newest-first, each
// including its full content.
func (s *PlanService) ListRevisions(ctx context.Context, taskID string) ([]*models.TaskPlanRevision, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	return s.repo.ListTaskPlanRevisions(ctx, taskID, 0)
}

// GetLatestRevision returns the most recent revision for a task, or nil if
// none exist. Unlike ListRevisions, it fetches only the latest revision row
// instead of every revision — callers that only need the latest revision
// number should use this instead.
func (s *PlanService) GetLatestRevision(ctx context.Context, taskID string) (*models.TaskPlanRevision, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	return s.repo.GetLatestTaskPlanRevision(ctx, taskID)
}

// GetRevision returns a single revision with content (for diff/preview).
func (s *PlanService) GetRevision(ctx context.Context, id string) (*models.TaskPlanRevision, error) {
	rev, err := s.repo.GetTaskPlanRevision(ctx, id)
	if err != nil {
		return nil, err
	}
	if rev == nil {
		return nil, ErrRevisionNotFound
	}
	if err := s.authorize(ctx, rev.TaskID); err != nil {
		return nil, err
	}
	return rev, nil
}

// RevertPlanRequest parameters for a revert-to-revision operation.
type RevertPlanRequest struct {
	TaskID           string
	TargetRevisionID string
	AuthorName       string // user display name; "User" fallback when empty
}

// RevertPlan creates a new revision whose content mirrors the target and updates HEAD,
// atomically via WritePlanRevision. Revert revisions are never coalesced (the "restored
// from vK" marker is preserved).
//
// The per-task lock is acquired before the target-revision fetch (F38) rather than just around
// the write, so a concurrent CreatePlan/UpdatePlan/DeletePlan cannot change HEAD between this
// method reading the revert target and it computing the new HEAD row from that snapshot. Both
// the target-revision fetch and the HEAD read use a context decoupled from the caller's
// (context.WithoutCancel) for the same reason as upsertPlan: a caller who cancels while queued
// for the lock must fail at the write transaction, not have an earlier read misdiagnosed as the
// cause (AC-005.7). Unlike upsertPlan, revert never substitutes an empty request field for an
// existing HEAD value — title/content/created_by are always the target revision's own values —
// so there is no ambiguous substitution for a failed HEAD read to gate; RevertPlan therefore
// does not abort when its own HEAD read fails (AC-001.6, F36): an unknown or absent HEAD simply
// means the write's ON CONFLICT branch won't fire (or, for absent, correctly recreates the row
// via INSERT) and this method still commits the revert using the target's own values.
func (s *PlanService) RevertPlan(ctx context.Context, req RevertPlanRequest) (*models.TaskPlanRevision, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}
	if req.TargetRevisionID == "" {
		return nil, ErrRevisionIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}

	release := s.locks.acquire(req.TaskID)
	defer release()

	readCtx := context.WithoutCancel(ctx)

	target, err := s.repo.GetTaskPlanRevision(readCtx, req.TargetRevisionID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, ErrRevisionNotFound
	}
	if target.TaskID != req.TaskID {
		return nil, ErrRevisionTaskMismatch
	}

	authorName := req.AuthorName
	if authorName == "" {
		authorName = defaultUserAuthorFallback
	}

	headPlan, headState := s.readPlanHead(readCtx, req.TaskID)
	plan := &models.TaskPlan{
		TaskID:    req.TaskID,
		Title:     target.Title,
		Content:   target.Content,
		CreatedBy: createdByUser,
	}
	if headState == planHeadFound {
		plan.ID = headPlan.ID
		plan.CreatedAt = headPlan.CreatedAt
	}

	targetID := target.ID
	rev := &models.TaskPlanRevision{
		TaskID:             req.TaskID,
		Title:              target.Title,
		Content:            target.Content,
		AuthorKind:         createdByUser,
		AuthorName:         authorName,
		RevertOfRevisionID: &targetID,
	}
	rev.WorkflowStepID, rev.WorkflowStepName, rev.WorkflowStepColor = s.currentWorkflowStepStamp(ctx, req.TaskID)
	if err := s.repo.WritePlanRevision(ctx, plan, rev, nil, false, false); err != nil {
		s.logPlanWriteError(req.TaskID, err)
		return nil, err
	}

	// AC-005.8, mirroring upsertPlan: the write already committed, so a failed or empty
	// re-read still reports success from the in-memory plan. Clear a fabricated identity
	// only when the pre-write HEAD read was itself unknown (see upsertPlan's longer comment).
	saved, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil || saved == nil {
		if headState == planHeadUnknown {
			plan.ID = ""
			plan.CreatedAt = time.Time{}
		}
		saved = plan
	}

	release()
	s.publishPlanEvent(ctx, events.TaskPlanUpdated, saved)
	s.publishRevisionEvent(ctx, rev, false)
	s.publishReverted(ctx, rev)
	return rev, nil
}

func (s *PlanService) logPlanWriteError(taskID string, err error) {
	fields := []zap.Field{zap.String("task_id", taskID), zap.Error(err)}
	if errors.Is(err, repository.ErrTaskNotFound) {
		s.logger.Debug("write plan revision", fields...)
		return
	}
	s.logger.Error("write plan revision", fields...)
}

func (s *PlanService) publishPlanEvent(ctx context.Context, eventType string, plan *models.TaskPlan) {
	if s.eventBus == nil {
		return
	}
	payload := map[string]interface{}{
		"id":         plan.ID,
		"task_id":    plan.TaskID,
		"title":      plan.Title,
		"content":    plan.Content,
		"created_by": plan.CreatedBy,
		"created_at": plan.CreatedAt,
		"updated_at": plan.UpdatedAt,
	}
	if plan.ImplementationStartedAt != nil {
		payload["implementation_started_at"] = *plan.ImplementationStartedAt
	}
	if plan.ImplementationStartedSessionID != nil {
		payload["implementation_started_session_id"] = *plan.ImplementationStartedSessionID
	}
	if plan.ImplementationStartedBy != nil {
		payload["implementation_started_by"] = *plan.ImplementationStartedBy
	}
	if err := s.eventBus.Publish(ctx, eventType, bus.NewEvent(eventType, "plan-service", payload)); err != nil {
		s.logger.Error("publish plan event", zap.String("event_type", eventType), zap.Error(err))
	}
}

func (s *PlanService) publishRevisionEvent(ctx context.Context, rev *models.TaskPlanRevision, coalesced bool) {
	if s.eventBus == nil {
		return
	}
	payload := revisionPayload(rev)
	payload["coalesced"] = coalesced
	if err := s.eventBus.Publish(ctx, events.TaskPlanRevisionCreated, bus.NewEvent(events.TaskPlanRevisionCreated, "plan-service", payload)); err != nil {
		s.logger.Error("publish revision event", zap.Error(err))
	}
}

func (s *PlanService) publishReverted(ctx context.Context, rev *models.TaskPlanRevision) {
	if s.eventBus == nil {
		return
	}
	payload := revisionPayload(rev)
	if err := s.eventBus.Publish(ctx, events.TaskPlanReverted, bus.NewEvent(events.TaskPlanReverted, "plan-service", payload)); err != nil {
		s.logger.Error("publish reverted event", zap.Error(err))
	}
}

func revisionPayload(rev *models.TaskPlanRevision) map[string]interface{} {
	p := map[string]interface{}{
		"id":              rev.ID,
		"task_id":         rev.TaskID,
		"revision_number": rev.RevisionNumber,
		"title":           rev.Title,
		"author_kind":     rev.AuthorKind,
		"author_name":     rev.AuthorName,
		"content_length":  models.PlanContentLength(rev.Content),
		"created_at":      rev.CreatedAt,
		"updated_at":      rev.UpdatedAt,
	}
	if rev.RevertOfRevisionID != nil {
		p["revert_of_revision_id"] = *rev.RevertOfRevisionID
	}
	if rev.WorkflowStepID != "" {
		p["workflow_step_id"] = rev.WorkflowStepID
	}
	if rev.WorkflowStepName != "" {
		p["workflow_step_name"] = rev.WorkflowStepName
	}
	if rev.WorkflowStepColor != "" {
		p["workflow_step_color"] = rev.WorkflowStepColor
	}
	return p
}
