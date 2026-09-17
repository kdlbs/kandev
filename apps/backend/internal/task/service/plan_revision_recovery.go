package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	// DefaultPlanRevisionPageLimit is the default number of metadata rows an
	// agent receives from one history-list call.
	DefaultPlanRevisionPageLimit = 20
	// MaxPlanRevisionPageLimit bounds one agent history-list call.
	MaxPlanRevisionPageLimit = 100
)

var (
	ErrPlanRevisionCursorInvalid = errors.New("revision cursor must be positive")
	ErrPlanRevisionLimitInvalid  = errors.New("revision page limit is invalid")
)

// PlanRevisionMetadataPage is a bounded, content-free history page.
type PlanRevisionMetadataPage struct {
	Revisions             []*models.TaskPlanRevision
	NextBeforeRevisionNum int
}

type planRevisionMetadataRepository interface {
	ListTaskPlanRevisionMetadata(context.Context, string, int, int) ([]*models.TaskPlanRevision, error)
}

type taskScopedPlanRevisionRepository interface {
	GetTaskPlanRevisionForTask(context.Context, string, string) (*models.TaskPlanRevision, error)
}

// ListAgentPlanRevisionMetadata returns newest-first, bounded history without
// loading revision bodies on the production repository path.
func (s *PlanService) ListAgentPlanRevisionMetadata(
	ctx context.Context, taskID string, beforeRevisionNumber, limit int,
) (PlanRevisionMetadataPage, error) {
	if taskID == "" {
		return PlanRevisionMetadataPage{}, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return PlanRevisionMetadataPage{}, err
	}
	limit, err := normalizeRevisionPage(beforeRevisionNumber, limit)
	if err != nil {
		return PlanRevisionMetadataPage{}, err
	}

	readLimit := limit + 1
	var revisions []*models.TaskPlanRevision
	if repo, ok := s.repo.(planRevisionMetadataRepository); ok {
		revisions, err = repo.ListTaskPlanRevisionMetadata(ctx, taskID, beforeRevisionNumber, readLimit)
	} else {
		// Small test doubles may only implement the original repository surface.
		// The production SQLite repository always takes the bounded query above.
		revisions, err = s.repo.ListTaskPlanRevisions(ctx, taskID, 0)
		revisions = filterRevisionPage(revisions, beforeRevisionNumber, readLimit)
	}
	if err != nil {
		return PlanRevisionMetadataPage{}, s.revisionUnavailableError(taskID, err)
	}

	page := PlanRevisionMetadataPage{Revisions: revisions}
	if len(revisions) > limit {
		page.Revisions = revisions[:limit]
		page.NextBeforeRevisionNum = page.Revisions[len(page.Revisions)-1].RevisionNumber
	}
	for _, revision := range page.Revisions {
		if revision.ContentBytes == 0 && revision.Content != "" {
			revision.ContentBytes = len(revision.Content)
		}
	}
	return page, nil
}

// GetAgentPlanRevision reads an exact task-scoped revision and its mutable
// snapshot token. Authorization happens before the identifier lookup.
func (s *PlanService) GetAgentPlanRevision(
	ctx context.Context, taskID, revisionID string,
) (*models.TaskPlanRevision, string, error) {
	if taskID == "" {
		return nil, "", ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, "", err
	}
	if revisionID == "" {
		return nil, "", ErrRevisionIDRequired
	}

	revision, err := s.getTaskScopedRevision(ctx, taskID, revisionID)
	if err != nil {
		return nil, "", s.revisionUnavailableError(taskID, err)
	}
	if revision == nil {
		return nil, "", ErrRevisionNotFound
	}
	return revision, PlanRevisionVersion(revision), nil
}

// RestoreAgentPlanRevision conditionally restores one task-scoped revision.
// It creates a new agent revision and preserves the selected source row.
func (s *PlanService) RestoreAgentPlanRevision(
	ctx context.Context, req RestorePlanRequest,
) (RestorePlanResult, error) {
	if req.TaskID == "" {
		return RestorePlanResult{}, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return RestorePlanResult{}, err
	}
	if req.RevisionID == "" {
		return RestorePlanResult{}, ErrRevisionIDRequired
	}

	release := s.locks.acquire(req.TaskID)
	defer release()
	readCtx := context.WithoutCancel(ctx)
	prepared, err := s.prepareRestore(readCtx, req)
	if err != nil {
		return RestorePlanResult{}, err
	}
	head, source := prepared.head, prepared.source
	if head.Title == source.Title && head.Content == source.Content {
		return RestorePlanResult{
			Plan: head, Revision: source, AlreadyCurrent: true,
		}, nil
	}

	plan := &models.TaskPlan{
		ID:                             head.ID,
		TaskID:                         req.TaskID,
		Title:                          source.Title,
		Content:                        source.Content,
		CreatedBy:                      createdByAgent,
		CreatedAt:                      head.CreatedAt,
		CommentsRevision:               head.CommentsRevision,
		ImplementationStartedAt:        head.ImplementationStartedAt,
		ImplementationStartedSessionID: head.ImplementationStartedSessionID,
		ImplementationStartedBy:        head.ImplementationStartedBy,
	}
	authorName := s.resolveAgentDisplayName(readCtx, req.TaskID)
	if authorName == "" {
		authorName = defaultAgentAuthorFallback
	}
	sourceID := source.ID
	revision := &models.TaskPlanRevision{
		TaskID:             req.TaskID,
		Title:              source.Title,
		Content:            source.Content,
		AuthorKind:         createdByAgent,
		AuthorName:         authorName,
		RevertOfRevisionID: &sourceID,
	}
	revision.WorkflowStepID, revision.WorkflowStepName, revision.WorkflowStepColor =
		s.currentWorkflowStepStamp(readCtx, req.TaskID)
	if err := s.repo.WritePlanRevision(ctx, plan, revision, nil, false, false); err != nil {
		s.logPlanWriteError(req.TaskID, err)
		return RestorePlanResult{}, err
	}

	saved, readErr := s.repo.GetTaskPlan(ctx, req.TaskID)
	if readErr != nil || saved == nil {
		saved = plan
	}
	release()
	s.publishPlanEvent(ctx, events.TaskPlanUpdated, saved)
	s.publishRevisionEvent(ctx, revision, false)
	s.publishReverted(ctx, revision)
	return RestorePlanResult{Plan: saved, Revision: revision}, nil
}

type restorePreparation struct {
	head   *models.TaskPlan
	source *models.TaskPlanRevision
}

func (s *PlanService) prepareRestore(
	ctx context.Context, req RestorePlanRequest,
) (restorePreparation, error) {
	head, state, headErr := s.readPlanHead(ctx, req.TaskID)
	if state == planHeadUnknown {
		return restorePreparation{}, s.headUnavailableError(req.TaskID, headErr)
	}
	if state == planHeadAbsent {
		return restorePreparation{}, ErrTaskPlanNotFound
	}
	if err := s.requireCurrentVersion(CreatePlanRequest{
		TaskID: req.TaskID, ExpectedVersion: req.ExpectedVersion,
	}, head); err != nil {
		return restorePreparation{}, err
	}
	if req.ExpectedRevisionVersion == "" {
		return restorePreparation{}, newPlanSafetyError(
			PlanErrorRevisionVersionRequired, ErrPlanRevisionVersionRequired, req.TaskID,
			"Plan was not changed because expected_revision_version is required.",
			"Fetch the selected revision again and retry with its revision_version and the current plan version.",
		)
	}

	source, err := s.getTaskScopedRevision(ctx, req.TaskID, req.RevisionID)
	if err != nil {
		return restorePreparation{}, s.revisionUnavailableError(req.TaskID, err)
	}
	if source == nil {
		return restorePreparation{}, ErrRevisionNotFound
	}
	sourceVersion := PlanRevisionVersion(source)
	if sourceVersion != req.ExpectedRevisionVersion {
		safety := newPlanSafetyError(
			PlanErrorRevisionChanged, ErrPlanRevisionChanged, req.TaskID,
			"Plan was not changed because the selected revision changed after it was read.",
			"Fetch the selected revision again before deciding whether to restore it.",
		)
		safety.CurrentVersion = head.WriteVersion
		safety.CurrentRevisionVersion = sourceVersion
		return restorePreparation{}, safety
	}

	latest, latestState, latestErr := s.readLatestRevisionDetailed(ctx, req.TaskID)
	if err := verifyPlanHistory(head, latest, latestState, latestErr); err != nil {
		if safety, ok := err.(*PlanSafetyError); ok {
			safety.CurrentVersion = head.WriteVersion
		}
		return restorePreparation{}, err
	}
	return restorePreparation{head: head, source: source}, nil
}

// RestorePlanRequest carries both sides of the conditional restore guard.
type RestorePlanRequest struct {
	TaskID                  string
	RevisionID              string
	ExpectedVersion         string
	ExpectedRevisionVersion string
}

// RestorePlanResult reports the committed HEAD and source revision. No write
// occurs when AlreadyCurrent is true.
type RestorePlanResult struct {
	Plan           *models.TaskPlan
	Revision       *models.TaskPlanRevision
	AlreadyCurrent bool
}

func normalizeRevisionPage(beforeRevisionNumber, limit int) (int, error) {
	if beforeRevisionNumber < 0 {
		return 0, ErrPlanRevisionCursorInvalid
	}
	if limit == 0 {
		limit = DefaultPlanRevisionPageLimit
	}
	if limit < 1 || limit > MaxPlanRevisionPageLimit {
		return 0, ErrPlanRevisionLimitInvalid
	}
	return limit, nil
}

func filterRevisionPage(revisions []*models.TaskPlanRevision, before, limit int) []*models.TaskPlanRevision {
	filtered := make([]*models.TaskPlanRevision, 0, len(revisions))
	for _, revision := range revisions {
		if before > 0 && revision.RevisionNumber >= before {
			continue
		}
		filtered = append(filtered, revision)
		if len(filtered) == limit {
			break
		}
	}
	return filtered
}

func (s *PlanService) getTaskScopedRevision(ctx context.Context, taskID, revisionID string) (*models.TaskPlanRevision, error) {
	if repo, ok := s.repo.(taskScopedPlanRevisionRepository); ok {
		return repo.GetTaskPlanRevisionForTask(ctx, taskID, revisionID)
	}
	revision, err := s.repo.GetTaskPlanRevision(ctx, revisionID)
	if err != nil || revision == nil {
		return revision, err
	}
	if revision.TaskID != taskID {
		return nil, nil
	}
	return revision, nil
}

func (s *PlanService) revisionUnavailableError(taskID string, cause error) error {
	if cause != nil {
		s.logger.Warn("agent plan revision read failed", zap.String("task_id", taskID), zap.Error(cause))
	}
	return newPlanSafetyError(
		PlanErrorRevisionUnavailable, ErrPlanRevisionUnavailable, taskID,
		"Plan was not changed because the selected plan revision could not be read.",
		"Read the revision again and retry only after the read succeeds.",
	)
}
