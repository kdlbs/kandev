package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var errExactTaskUpdatesUnavailable = errors.New("exact task updates are unavailable")

// ExactTaskUpdateRequest identifies a task mutation that must be committed
// once and against one observed task version.
type ExactTaskUpdateRequest struct {
	WorkspaceID             string
	ExpectedResourceVersion string
	OperationID             string
	PayloadDigest           string
	Title                   *string
	Description             *string
	State                   *v1.TaskState
	Priority                *string
	Labels                  *[]string
	AssigneeUserID          *string
	ClaimFence              repository.TaskManagementClaimFence
}

// ExactTaskUpdateResult reports whether the operation was applied by this
// call or recovered from a prior committed attempt.
type ExactTaskUpdateResult struct {
	Task           *models.Task
	AlreadyApplied bool
}

type ExactTaskArchiveRequest struct {
	WorkspaceID             string
	ExpectedResourceVersion string
	OperationID             string
	PayloadDigest           string
	ClaimFence              repository.TaskManagementClaimFence
}

type ExactTaskArchiveResult struct {
	Task           *models.Task
	AlreadyApplied bool
}

type ExactTaskRelationRequest struct {
	WorkspaceID                    string
	TaskID                         string
	RelatedTaskID                  string
	ExpectedTaskResourceVersion    string
	ExpectedRelatedResourceVersion string
	ClaimFence                     repository.TaskManagementClaimFence
}

type exactTaskArchiveContextKey struct{}

// ArchiveTaskExact commits a versioned archive through the native archive
// lifecycle and replays its operation result after a lost Host receipt.
//
//nolint:cyclop // The service maps exact archive admission into native task lifecycle behavior.
func (s *Service) ArchiveTaskExact(ctx context.Context, id string, req ExactTaskArchiveRequest) (*ExactTaskArchiveResult, error) {
	if req.WorkspaceID == "" || req.ExpectedResourceVersion == "" || req.OperationID == "" || req.PayloadDigest == "" {
		return nil, errors.New("exact task archive identity is incomplete")
	}
	if _, err := time.Parse(time.RFC3339Nano, req.ExpectedResourceVersion); err != nil {
		return nil, errors.New("exact task archive resource version is invalid")
	}
	if err := s.authorizeTaskScope(ctx, id, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	operationRepo, ok := s.tasks.(repository.ExactTaskArchiveRepository)
	if !ok {
		return nil, errExactTaskUpdatesUnavailable
	}
	if _, found, err := operationRepo.GetTaskCommandOperation(ctx, req.WorkspaceID, id, req.OperationID, req.PayloadDigest); err != nil {
		return nil, err
	} else if found {
		task, err := s.tasks.GetTask(ctx, id)
		if err != nil {
			return nil, err
		}
		return &ExactTaskArchiveResult{Task: task, AlreadyApplied: true}, nil
	}
	current, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	expectedVersion, _ := time.Parse(time.RFC3339Nano, req.ExpectedResourceVersion)
	if current.WorkspaceID != req.WorkspaceID || !current.UpdatedAt.Equal(expectedVersion) || current.ArchivedAt != nil {
		return nil, repoerrors.ErrTaskVersionConflict
	}
	ctx = context.WithValue(ctx, exactTaskArchiveContextKey{}, req)
	if err := s.ArchiveTask(ctx, id); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	return &ExactTaskArchiveResult{Task: task}, nil
}

// UpdateTaskExact applies a versioned task patch and records the operation in
// the same repository transaction. Human assignment is resolved through the
// task service; workflow movement, metadata, and repository writes stay out of
// this patch surface.
func (s *Service) UpdateTaskExact(ctx context.Context, id string, req ExactTaskUpdateRequest) (*ExactTaskUpdateResult, error) {
	if err := s.authorizeTaskScope(ctx, id, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	if err := validateExactTaskUpdateRequest(req); err != nil {
		return nil, err
	}
	operationRepo, ok := s.tasks.(repository.ExactTaskOperationRepository)
	if !ok {
		return nil, errExactTaskUpdatesUnavailable
	}
	task, err := s.tasks.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.WorkspaceID != req.WorkspaceID {
		return nil, repoerrors.ErrTaskVersionConflict
	}
	oldState := task.State
	applyExactTaskPatch(task, req)
	if req.AssigneeUserID != nil {
		assignee, err := s.resolveTaskAssignee(ctx, task, *req.AssigneeUserID)
		if err != nil {
			return nil, err
		}
		task.AssigneeUserID = assignee
	}
	return s.commitExactTaskUpdate(ctx, operationRepo, task, id, req, oldState)
}

//nolint:cyclop // Each optional task field has an explicit version and value contract.
func validateExactTaskUpdateRequest(req ExactTaskUpdateRequest) error {
	if req.WorkspaceID == "" || req.ExpectedResourceVersion == "" || req.OperationID == "" || req.PayloadDigest == "" {
		return errors.New("exact task update identity is incomplete")
	}
	if req.Title == nil && req.Description == nil && req.State == nil && req.Priority == nil && req.Labels == nil && req.AssigneeUserID == nil {
		return errors.New("exact task update has no changes")
	}
	if req.Title != nil {
		if err := validateTaskTitle(*req.Title); err != nil {
			return err
		}
	}
	if req.Priority != nil {
		if err := ValidateTaskPriority(*req.Priority); err != nil {
			return err
		}
	}
	if req.Labels != nil && len(*req.Labels) > 256 {
		return errors.New("too many task labels")
	}
	if req.State != nil && !validExactTaskState(*req.State) {
		return errors.New("invalid task state")
	}
	return nil
}

func applyExactTaskPatch(task *models.Task, req ExactTaskUpdateRequest) {
	if req.Title != nil {
		task.Title = *req.Title
		if task.Metadata != nil {
			delete(task.Metadata, models.MetaKeyAgentTitlePending)
			delete(task.Metadata, models.MetaKeyAgentTitleOwnerSessionID)
		}
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.State != nil {
		task.State = *req.State
	}
	if req.Priority != nil {
		task.Priority = *req.Priority
	}
	if req.Labels != nil {
		labels := make([]string, len(*req.Labels))
		copy(labels, *req.Labels)
		encoded, _ := json.Marshal(labels)
		task.Labels = string(encoded)
	}
}

func (s *Service) commitExactTaskUpdate(
	ctx context.Context,
	operationRepo repository.ExactTaskOperationRepository,
	task *models.Task,
	id string,
	req ExactTaskUpdateRequest,
	oldState v1.TaskState,
) (*ExactTaskUpdateResult, error) {
	alreadyApplied, err := operationRepo.UpdateTaskExactOperation(
		ctx, task, req.WorkspaceID, req.ExpectedResourceVersion, req.OperationID, req.PayloadDigest, req.ClaimFence,
	)
	if err != nil {
		return nil, err
	}
	task = s.reloadTaskAfterMutation(ctx, id, task, "exact update")
	if alreadyApplied {
		return &ExactTaskUpdateResult{Task: task, AlreadyApplied: true}, nil
	}
	if req.State != nil && oldState != task.State {
		s.publishTaskEvent(ctx, events.TaskStateChanged, task, &oldState)
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
	return &ExactTaskUpdateResult{Task: task}, nil
}

func validExactTaskState(state v1.TaskState) bool {
	switch state {
	case v1.TaskStateTODO, v1.TaskStateCreated, v1.TaskStateInProgress,
		v1.TaskStateReview, v1.TaskStateBlocked, v1.TaskStateWaitingForInput,
		v1.TaskStateCompleted, v1.TaskStateFailed, v1.TaskStateCancelled:
		return true
	default:
		return false
	}
}
