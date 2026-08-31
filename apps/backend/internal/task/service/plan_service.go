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
}

// PlanService provides task plan business logic.
type PlanService struct {
	repo           planRepo
	eventBus       bus.EventBus
	logger         *logger.Logger
	coalesceWindow time.Duration
	// authorizeTask gates plan access by the task's workspace ownership
	// (opt-in auth). Nil = unscoped (internal callers / auth disabled).
	authorizeTask func(ctx context.Context, taskID string) error
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
	}
}

// SetTaskAuthorizer wires the per-user task-access check (opt-in auth). The
// authorizer must return nil for contexts without a request identity.
func (s *PlanService) SetTaskAuthorizer(fn func(ctx context.Context, taskID string) error) {
	s.authorizeTask = fn
}

func (s *PlanService) authorize(ctx context.Context, taskID string) error {
	if s.authorizeTask == nil {
		return nil
	}
	return s.authorizeTask(ctx, taskID)
}

func resolveCoalesceWindow(window time.Duration) time.Duration {
	if window < 0 {
		return defaultCoalesceWindow
	}
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
}

// CreatePlan upserts a plan and appends or coalesces a revision.
func (s *PlanService) CreatePlan(ctx context.Context, req CreatePlanRequest) (*models.TaskPlan, error) {
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}
	return s.upsertPlan(ctx, req)
}

// UpdatePlanRequest mirrors CreatePlanRequest; kept as a distinct type for API clarity.
type UpdatePlanRequest struct {
	TaskID           string
	Title            string
	Content          string
	CreatedBy        string
	AuthorKind       string
	AuthorName       string
	ForceNewRevision bool
}

// UpdatePlan updates an existing plan (errors if missing).
func (s *PlanService) UpdatePlan(ctx context.Context, req UpdatePlanRequest) (*models.TaskPlan, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}
	existing, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrTaskPlanNotFound
	}
	title := req.Title
	if title == "" {
		title = existing.Title
	}
	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = existing.CreatedBy
	}
	return s.upsertPlan(ctx, CreatePlanRequest{
		TaskID:           req.TaskID,
		Title:            title,
		Content:          req.Content,
		CreatedBy:        createdBy,
		AuthorKind:       req.AuthorKind,
		AuthorName:       req.AuthorName,
		ForceNewRevision: req.ForceNewRevision,
	})
}

// upsertPlan is the shared write path. It upserts the task_plans HEAD row and either
// coalesces into the latest revision (same author within window) or appends a new revision
// — both steps run in one write transaction via WritePlanRevision so HEAD and history
// cannot diverge under concurrent writers or partial failures.
func (s *PlanService) upsertPlan(ctx context.Context, req CreatePlanRequest) (*models.TaskPlan, error) {
	if req.TaskID == "" {
		return nil, ErrTaskIDRequired
	}

	title := req.Title
	if title == "" {
		title = "Plan"
	}
	// Resolve a missing AuthorName for agent writes from the active session's
	// profile snapshot before falling back to the literal "Agent". The MCP path
	// (handleCreateTaskPlan / handleUpdateTaskPlan) doesn't carry the agent's
	// display name in the request, so without this lookup every agent revision
	// would render as "Agent" in the history UI.
	if req.AuthorName == "" {
		kindHint := req.AuthorKind
		if kindHint == "" {
			kindHint = req.CreatedBy
		}
		if kindHint == createdByAgent {
			req.AuthorName = s.resolveAgentDisplayName(ctx, req.TaskID)
		}
	}
	authorKind, authorName, createdBy := resolveAuthor(req)

	existing, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil {
		s.logger.Error("get existing plan", zap.String("task_id", req.TaskID), zap.Error(err))
		return nil, err
	}
	eventType := events.TaskPlanCreated
	if existing != nil {
		eventType = events.TaskPlanUpdated
	}

	plan := &models.TaskPlan{
		TaskID:    req.TaskID,
		Title:     title,
		Content:   req.Content,
		CreatedBy: createdBy,
	}
	if existing != nil {
		plan.ID = existing.ID
		plan.CreatedAt = existing.CreatedAt
	}

	latest, err := s.repo.GetLatestTaskPlanRevision(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	coalesce := !req.ForceNewRevision && s.canCoalesce(latest, authorKind, authorName, now)

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
		// Preserve the original revision's author + number on merge.
		rev.RevisionNumber = latest.RevisionNumber
		rev.AuthorKind = latest.AuthorKind
		rev.AuthorName = latest.AuthorName
		rev.CreatedAt = latest.CreatedAt
	}

	if err := s.repo.WritePlanRevision(ctx, plan, rev, coalesceID); err != nil {
		s.logPlanWriteError(req.TaskID, err)
		return nil, err
	}

	saved, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, ErrTaskPlanNotFound
	}
	s.publishPlanEvent(ctx, eventType, saved)
	s.publishRevisionEvent(ctx, rev, coalesce)
	return saved, nil
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
func (s *PlanService) DeletePlan(ctx context.Context, taskID string) error {
	if taskID == "" {
		return ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return err
	}
	existing, err := s.repo.GetTaskPlan(ctx, taskID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrTaskPlanNotFound
	}
	if err := s.repo.DeleteTaskPlan(ctx, taskID); err != nil {
		return err
	}
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
// number (e.g. the plan write guard) should use this instead.
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
	target, err := s.repo.GetTaskPlanRevision(ctx, req.TargetRevisionID)
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

	head, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	plan := &models.TaskPlan{
		TaskID:    req.TaskID,
		Title:     target.Title,
		Content:   target.Content,
		CreatedBy: createdByUser,
	}
	if head != nil {
		plan.ID = head.ID
		plan.CreatedAt = head.CreatedAt
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
	if err := s.repo.WritePlanRevision(ctx, plan, rev, nil); err != nil {
		s.logPlanWriteError(req.TaskID, err)
		return nil, err
	}

	saved, err := s.repo.GetTaskPlan(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if saved == nil {
		return nil, ErrTaskPlanNotFound
	}
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
		"created_at":      rev.CreatedAt,
		"updated_at":      rev.UpdatedAt,
	}
	if rev.RevertOfRevisionID != nil {
		p["revert_of_revision_id"] = *rev.RevertOfRevisionID
	}
	return p
}
