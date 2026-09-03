package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"go.uber.org/zap"
)

var (
	ErrPlanIDRequired           = errors.New("plan_id is required")
	ErrPlanCommentIDRequired    = errors.New("comment id is required")
	ErrPlanCommentIDInvalid     = errors.New("comment id must be a UUID")
	ErrPlanCommentBodyRequired  = errors.New("comment body is required")
	ErrPlanCommentVersionNeeded = errors.New("expected_version must be positive")
	ErrPlanCommentAnchorInvalid = errors.New("plan comment anchor is invalid")
	ErrTaskPlanCommentsChanged  = errors.New("task plan comments changed")
)

// CreatePlanCommentRequest contains task-owned plan comment fields supplied by a client.
type CreatePlanCommentRequest struct {
	TaskID       string
	PlanID       string
	ID           string
	Body         string
	SelectedText string
	AnchorFrom   int
	AnchorTo     int
}

// UpdatePlanCommentRequest identifies and edits one pending plan comment.
type UpdatePlanCommentRequest struct {
	TaskID          string
	PlanID          string
	ID              string
	Body            string
	ExpectedVersion int64
}

// DeletePlanCommentRequest identifies one version of a pending plan comment.
type DeletePlanCommentRequest struct {
	TaskID          string
	PlanID          string
	ID              string
	ExpectedVersion int64
}

// ListPlanComments returns the authoritative comment snapshot for a task's current plan.
func (s *PlanService) ListPlanComments(ctx context.Context, taskID string) (*models.TaskPlanCommentSnapshot, error) {
	if taskID == "" {
		return nil, ErrTaskIDRequired
	}
	if err := s.authorize(ctx, taskID); err != nil {
		return nil, err
	}
	return s.repo.ListTaskPlanComments(ctx, taskID)
}

// CreatePlanComment persists pending feedback and publishes its committed snapshot.
func (s *PlanService) CreatePlanComment(
	ctx context.Context,
	req CreatePlanCommentRequest,
) (*models.TaskPlanCommentSnapshot, error) {
	if err := validatePlanCommentCreate(req); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}
	comment := &models.TaskPlanComment{
		ID: req.ID, TaskID: req.TaskID, PlanID: req.PlanID, Body: req.Body,
		SelectedText: req.SelectedText, AnchorFrom: req.AnchorFrom, AnchorTo: req.AnchorTo,
	}
	snapshot, err := s.repo.CreateTaskPlanComment(ctx, comment)
	if err != nil {
		return snapshot, mapPlanCommentError(err)
	}
	s.publishPlanCommentSnapshot(ctx, "create", snapshot)
	return snapshot, nil
}

// UpdatePlanComment changes a pending comment using optimistic row versioning.
func (s *PlanService) UpdatePlanComment(
	ctx context.Context,
	req UpdatePlanCommentRequest,
) (*models.TaskPlanCommentSnapshot, error) {
	if err := validatePlanCommentMutation(req.TaskID, req.PlanID, req.ID, req.Body, req.ExpectedVersion, true); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}
	snapshot, err := s.repo.UpdateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: req.ID, TaskID: req.TaskID, PlanID: req.PlanID, Body: req.Body,
	}, req.ExpectedVersion)
	if err != nil {
		return snapshot, mapPlanCommentError(err)
	}
	s.publishPlanCommentSnapshot(ctx, "update", snapshot)
	return snapshot, nil
}

// DeletePlanComment removes one version of a pending comment.
func (s *PlanService) DeletePlanComment(
	ctx context.Context,
	req DeletePlanCommentRequest,
) (*models.TaskPlanCommentSnapshot, error) {
	if err := validatePlanCommentMutation(req.TaskID, req.PlanID, req.ID, "", req.ExpectedVersion, false); err != nil {
		return nil, err
	}
	if err := s.authorize(ctx, req.TaskID); err != nil {
		return nil, err
	}
	snapshot, err := s.repo.DeleteTaskPlanComment(
		ctx, req.TaskID, req.PlanID, req.ID, req.ExpectedVersion,
	)
	if err != nil {
		return snapshot, mapPlanCommentError(err)
	}
	s.publishPlanCommentSnapshot(ctx, "delete", snapshot)
	return snapshot, nil
}

func validatePlanCommentCreate(req CreatePlanCommentRequest) error {
	if err := validatePlanCommentMutation(req.TaskID, req.PlanID, req.ID, req.Body, 1, true); err != nil {
		return err
	}
	if req.SelectedText == "" || req.AnchorFrom < 0 || req.AnchorTo <= req.AnchorFrom {
		return ErrPlanCommentAnchorInvalid
	}
	return nil
}

func validatePlanCommentMutation(taskID, planID, commentID, body string, expectedVersion int64, needsBody bool) error {
	switch {
	case taskID == "":
		return ErrTaskIDRequired
	case planID == "":
		return ErrPlanIDRequired
	case commentID == "":
		return ErrPlanCommentIDRequired
	case uuid.Validate(commentID) != nil:
		return ErrPlanCommentIDInvalid
	case needsBody && strings.TrimSpace(body) == "":
		return ErrPlanCommentBodyRequired
	case expectedVersion <= 0:
		return ErrPlanCommentVersionNeeded
	default:
		return nil
	}
}

func mapPlanCommentError(err error) error {
	switch {
	case errors.Is(err, repository.ErrTaskPlanNotFound):
		return ErrTaskPlanNotFound
	case errors.Is(err, repository.ErrTaskPlanCommentsChanged):
		return ErrTaskPlanCommentsChanged
	default:
		return err
	}
}

func (s *PlanService) publishPlanCommentSnapshot(
	ctx context.Context,
	mutation string,
	snapshot *models.TaskPlanCommentSnapshot,
) {
	if s.eventBus == nil {
		return
	}
	err := s.eventBus.Publish(ctx, events.TaskPlanCommentsChanged,
		bus.NewEvent(events.TaskPlanCommentsChanged, "plan-service", snapshot))
	if err != nil {
		s.logger.Error("publish plan comment snapshot",
			zap.String("task_id", snapshot.TaskID),
			zap.String("plan_id", snapshot.PlanID),
			zap.Int64("comments_revision", snapshot.Revision),
			zap.String("mutation", mutation),
			zap.Int("comment_count", len(snapshot.Comments)),
			zap.Error(err),
		)
	}
}
