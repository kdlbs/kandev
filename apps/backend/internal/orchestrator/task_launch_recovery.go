package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var (
	ErrTaskLaunchRecoveryStale   = errors.New("task launch recovery request is stale")
	ErrTaskLaunchRecoveryInvalid = errors.New("invalid task launch recovery request")
)

const (
	taskLaunchRecoveryRetryDefault = models.RecoveryActionRetryDefault
	taskLaunchRecoveryPickBranch   = models.RecoveryActionPickBaseBranch
	taskLaunchRecoveryMarkDone     = models.RecoveryActionMarkReviewDone
)

// TaskLaunchRecoveryRequest is the server-side shape of task.launch.recover.
// The WebSocket handler decodes the same fields without adding transport-only
// defaults.
type TaskLaunchRecoveryRequest struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id,omitempty"`
	TaskRepositoryID string `json:"task_repository_id,omitempty"`
	Action           string `json:"action"`
	BaseBranch       string `json:"base_branch,omitempty"`
	ErrorStamp       string `json:"error_stamp"`
}

// TaskLaunchRecoveryResponse is the success shape of task.launch.recover.
type TaskLaunchRecoveryResponse struct {
	OK        bool   `json:"ok"`
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id,omitempty"`
}

type taskLaunchRecoveryWorktree interface {
	ResolveRemoteDefaultBranch(context.Context, string) (string, error)
}

type taskLaunchRecoveryTaskService interface {
	UpdateRepositoryBaseBranch(context.Context, taskservice.UpdateRepositoryBaseBranchRequest) (*models.TaskRepository, error)
	ListBranches(context.Context, string, string) ([]taskservice.Branch, error)
	MoveTaskWithOptions(context.Context, string, string, string, int, taskservice.MoveTaskOptions) (*taskservice.MoveTaskResult, error)
}

type taskLaunchRecoveryRepository interface {
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	GetRepository(context.Context, string) (*models.Repository, error)
	UpdateRepositoryDefaultBranch(context.Context, string, string, string) error
	SetSessionMetadataKeyIfStamp(context.Context, string, string, string, interface{}) (bool, error)
	SetTaskMetadataKeyIfStamp(context.Context, string, string, string, interface{}) (bool, error)
	RemoveSessionMetadataKeyIfStamp(context.Context, string, string, string) (bool, error)
}

type taskLaunchRecoverySource struct {
	task         *models.Task
	session      *models.TaskSession
	taskError    models.TaskLaunchError
	sessionError models.LastAgentError
	sessionOwned bool
	errorStamp   string
}

// SetTaskLaunchRecoveryService wires the task-service operations used by the
// task-scoped recovery action. The narrow interface keeps isolated
// orchestrator tests independent from task-service construction.
func (s *Service) SetTaskLaunchRecoveryService(svc taskLaunchRecoveryTaskService) {
	s.taskLaunchRecoveryTasks = svc
}

// RecoverTaskLaunch validates and executes one task-scoped launch recovery
// action. All reads happen after task authorization, and source-error clearing
// is compare-and-clear so an overlapping failure remains visible.
func (s *Service) RecoverTaskLaunch(ctx context.Context, req *TaskLaunchRecoveryRequest) (*TaskLaunchRecoveryResponse, error) {
	if req == nil {
		return nil, ErrTaskLaunchRecoveryInvalid
	}
	if err := s.validateTaskLaunchRecoveryRequest(req); err != nil {
		return nil, err
	}
	if err := s.authorizeTask(ctx, req.TaskID); err != nil {
		return nil, err
	}

	source, err := s.loadTaskLaunchRecoverySource(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.verifyCurrentTaskLaunchRecoverySource(ctx, source); err != nil {
		return nil, err
	}
	if !containsRecoveryAction(source.recoveryActions(), req.Action) {
		return nil, fmt.Errorf("recovery action %q is not available for the current error", req.Action)
	}

	var responseSessionID string
	switch req.Action {
	case taskLaunchRecoveryRetryDefault, taskLaunchRecoveryPickBranch:
		if err := s.recoverTaskLaunchBranch(ctx, req, source); err != nil {
			return nil, err
		}
		responseSessionID, err = s.relaunchAndClearTaskLaunchRecovery(ctx, req, source)
		if err != nil {
			return nil, err
		}
	case taskLaunchRecoveryMarkDone:
		if err := s.markTaskLaunchReviewDone(ctx, req, source); err != nil {
			return nil, err
		}
		responseSessionID = req.SessionID
		if err := s.clearTaskLaunchRecoverySource(ctx, source); err != nil {
			return nil, err
		}
	default:
		return nil, ErrTaskLaunchRecoveryInvalid
	}

	return &TaskLaunchRecoveryResponse{OK: true, TaskID: req.TaskID, SessionID: responseSessionID}, nil
}

func (s *Service) relaunchAndClearTaskLaunchRecovery(
	ctx context.Context,
	req *TaskLaunchRecoveryRequest,
	source *taskLaunchRecoverySource,
) (string, error) {
	responseSessionID, err := s.relaunchRecoveredTask(ctx, req, source)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(responseSessionID) == "" {
		return "", fmt.Errorf("task launch recovery relaunch did not create a session")
	}
	if err := s.clearTaskLaunchRecoverySource(ctx, source); err != nil && s.logger != nil {
		s.logger.Warn("task launch recovery relaunched successfully but could not clear the source error",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", responseSessionID),
			zap.Error(err))
	}
	return responseSessionID, nil
}

func (s *Service) validateTaskLaunchRecoveryRequest(req *TaskLaunchRecoveryRequest) error {
	if strings.TrimSpace(req.TaskID) == "" || strings.TrimSpace(req.Action) == "" || strings.TrimSpace(req.ErrorStamp) == "" {
		return ErrTaskLaunchRecoveryInvalid
	}
	switch req.Action {
	case taskLaunchRecoveryRetryDefault, taskLaunchRecoveryPickBranch, taskLaunchRecoveryMarkDone:
	default:
		return fmt.Errorf("unsupported task launch recovery action %q", req.Action)
	}
	if req.Action != taskLaunchRecoveryMarkDone && strings.TrimSpace(req.TaskRepositoryID) == "" {
		return fmt.Errorf("task_repository_id is required for %s", req.Action)
	}
	if req.Action == taskLaunchRecoveryPickBranch && strings.TrimSpace(req.BaseBranch) == "" {
		return fmt.Errorf("base_branch is required for %s", req.Action)
	}
	return nil
}

func (s *Service) loadTaskLaunchRecoverySource(ctx context.Context, req *TaskLaunchRecoveryRequest) (*taskLaunchRecoverySource, error) {
	task, err := s.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q was not found", req.TaskID)
	}
	var source *taskLaunchRecoverySource
	if req.SessionID != "" {
		source, err = s.loadSessionTaskLaunchRecoverySource(ctx, req, task)
	} else {
		source, err = loadTaskMetadataLaunchRecoverySource(req, task)
	}
	if err != nil {
		return nil, err
	}

	if req.TaskRepositoryID != "" {
		if _, err := s.taskRepositoryForRecovery(ctx, req.TaskID, req.TaskRepositoryID); err != nil {
			return nil, err
		}
	}
	if req.Action != taskLaunchRecoveryMarkDone {
		activeRepositoryID := source.taskRepositoryID()
		if activeRepositoryID == "" || activeRepositoryID != req.TaskRepositoryID {
			return nil, fmt.Errorf("task_repository_id does not match the active launch error")
		}
	}
	return source, nil
}

func (s *Service) verifyCurrentTaskLaunchRecoverySource(
	ctx context.Context,
	source *taskLaunchRecoverySource,
) error {
	current, err := s.loadAuthoritativeTaskLaunchRecoveryError(ctx, source.task.ID)
	if err != nil {
		return err
	}
	if current == nil || current.SessionID != source.sessionID() || current.Stamp != source.errorStamp {
		return ErrTaskLaunchRecoveryStale
	}
	return nil
}

func (s *Service) loadAuthoritativeTaskLaunchRecoveryError(
	ctx context.Context,
	taskID string,
) (*statussummary.ActiveErrorSummary, error) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("task %q was not found", taskID)
	}
	sessions, err := s.repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	input := statussummary.RebuildInput{Now: time.Now().UTC()}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		lastError, found := models.LoadLastAgentError(session.Metadata)
		if !found || lastError.IsDismissed() {
			continue
		}
		input.Sessions = append(input.Sessions, statussummary.RebuildSession{
			ID: session.ID,
			ActiveError: &statussummary.ActiveErrorSummary{
				SessionID:        session.ID,
				TaskRepositoryID: lastError.TaskRepositoryID,
				Stamp:            lastError.Stamp(),
				OccurredAt:       lastError.OccurredAt,
				Preview:          lastError.Message,
				Category:         lastError.Code,
				RecoveryActions:  lastError.RecoveryActions,
			},
		})
	}
	if launchError, found := models.LoadTaskLaunchError(task.Metadata); found {
		input.TaskError = &statussummary.ActiveErrorSummary{
			TaskRepositoryID: launchError.TaskRepositoryID,
			Stamp:            launchError.Stamp(),
			OccurredAt:       launchError.OccurredAt,
			Preview:          launchError.Message,
			Category:         launchError.Code,
			RecoveryActions:  launchError.RecoveryActions,
		}
	}
	return statussummary.BuildFromAuthoritative(input).ActiveError, nil
}

func (s *Service) loadSessionTaskLaunchRecoverySource(
	ctx context.Context,
	req *TaskLaunchRecoveryRequest,
	task *models.Task,
) (*taskLaunchRecoverySource, error) {
	if err := s.authorizeSession(ctx, req.SessionID); err != nil {
		return nil, err
	}
	session, err := s.repo.GetTaskSession(ctx, req.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.TaskID != req.TaskID {
		return nil, fmt.Errorf("session does not belong to task")
	}
	lastError, found := models.LoadLastAgentError(session.Metadata)
	if !found {
		return nil, fmt.Errorf("session has no active launch error")
	}
	if !lastError.MatchesStamp(req.ErrorStamp) {
		return nil, ErrTaskLaunchRecoveryStale
	}
	return &taskLaunchRecoverySource{
		task:         task,
		session:      session,
		sessionError: lastError,
		sessionOwned: true,
		errorStamp:   lastError.Stamp(),
	}, nil
}

func loadTaskMetadataLaunchRecoverySource(
	req *TaskLaunchRecoveryRequest,
	task *models.Task,
) (*taskLaunchRecoverySource, error) {
	launchError, found := models.LoadTaskLaunchError(task.Metadata)
	if !found {
		return nil, fmt.Errorf("task has no active launch error")
	}
	if !launchError.MatchesStamp(req.ErrorStamp) {
		return nil, ErrTaskLaunchRecoveryStale
	}
	return &taskLaunchRecoverySource{
		task:       task,
		taskError:  launchError,
		errorStamp: launchError.Stamp(),
	}, nil
}

func (s *Service) taskRepositoryForRecovery(ctx context.Context, taskID, taskRepositoryID string) (*models.TaskRepository, error) {
	if s.taskLaunchRecoveryRepo == nil {
		return nil, fmt.Errorf("task launch recovery repository is unavailable")
	}
	rows, err := s.taskLaunchRecoveryRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row != nil && row.ID == taskRepositoryID {
			return row, nil
		}
	}
	return nil, fmt.Errorf("task repository does not belong to task")
}

func (source *taskLaunchRecoverySource) taskRepositoryID() string {
	if source.sessionOwned {
		return source.sessionError.TaskRepositoryID
	}
	return source.taskError.TaskRepositoryID
}

func (source *taskLaunchRecoverySource) sessionID() string {
	if source.sessionOwned && source.session != nil {
		return source.session.ID
	}
	return ""
}

func (source *taskLaunchRecoverySource) recoveryActions() []string {
	if source.sessionOwned {
		return source.sessionError.RecoveryActions
	}
	return source.taskError.RecoveryActions
}

func containsRecoveryAction(actions []string, wanted string) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}

func (s *Service) recoverTaskLaunchBranch(ctx context.Context, req *TaskLaunchRecoveryRequest, source *taskLaunchRecoverySource) error {
	if s.taskLaunchRecoveryTasks == nil {
		return fmt.Errorf("task launch recovery service is unavailable")
	}
	taskRepository, err := s.taskRepositoryForRecovery(ctx, req.TaskID, req.TaskRepositoryID)
	if err != nil {
		return err
	}
	if s.taskLaunchRecoveryRepo == nil {
		return fmt.Errorf("task launch recovery repository is unavailable")
	}
	repository, err := s.taskLaunchRecoveryRepo.GetRepository(ctx, taskRepository.RepositoryID)
	if err != nil {
		return err
	}
	if repository == nil {
		return fmt.Errorf("repository does not exist")
	}

	baseBranch, err := s.resolveTaskLaunchRecoveryBranch(ctx, req, repository, source, taskRepository.ID)
	if err != nil {
		return err
	}

	if req.Action == taskLaunchRecoveryRetryDefault && repository.DefaultBranch != baseBranch {
		if err := s.taskLaunchRecoveryRepo.UpdateRepositoryDefaultBranch(ctx, repository.ID, repository.DefaultBranch, baseBranch); err != nil {
			return fmt.Errorf("update repository default branch: %w", err)
		}
	}
	updatedTaskRepository, err := s.taskLaunchRecoveryTasks.UpdateRepositoryBaseBranch(ctx, taskservice.UpdateRepositoryBaseBranchRequest{
		TaskID:           req.TaskID,
		TaskRepositoryID: taskRepository.ID,
		BaseBranch:       baseBranch,
	})
	if err != nil {
		return fmt.Errorf("update task repository base branch: %w", err)
	}
	s.logger.Debug("task launch recovery updated task repository base branch",
		zap.String("task_id", req.TaskID),
		zap.String("task_repository_id", taskRepository.ID),
		zap.String("requested_base_branch", baseBranch),
		zap.String("stored_base_branch", updatedTaskRepository.BaseBranch),
	)
	return nil
}

func (s *Service) resolveTaskLaunchRecoveryBranch(
	ctx context.Context,
	req *TaskLaunchRecoveryRequest,
	repository *models.Repository,
	source *taskLaunchRecoverySource,
	taskRepositoryID string,
) (string, error) {
	baseBranch := strings.TrimSpace(req.BaseBranch)
	if req.Action != taskLaunchRecoveryRetryDefault {
		if err := s.validateSelectedRecoveryBranch(ctx, repository, baseBranch); err != nil {
			return "", err
		}
		return baseBranch, nil
	}
	if s.taskLaunchRecoveryWorktree == nil {
		return "", fmt.Errorf("live default-branch resolver is unavailable")
	}
	baseBranch, err := s.taskLaunchRecoveryWorktree.ResolveRemoteDefaultBranch(ctx, repository.LocalPath)
	if err == nil && strings.TrimSpace(baseBranch) != "" {
		return baseBranch, nil
	}
	if errors.Is(err, worktree.ErrRemoteDefaultUnresolved) || strings.TrimSpace(baseBranch) == "" {
		return "", s.recordUnresolvedDefaultBranch(ctx, source, taskRepositoryID, err)
	}
	return "", err
}

func (s *Service) validateSelectedRecoveryBranch(ctx context.Context, repository *models.Repository, branch string) error {
	if s.taskLaunchRecoveryTasks == nil || repository == nil {
		return fmt.Errorf("task launch recovery service is unavailable")
	}
	wanted := normalizeTaskPRBranch(branch)
	if wanted == "" {
		return worktree.ErrInvalidBaseBranch
	}
	branches, err := s.taskLaunchRecoveryTasks.ListBranches(ctx, repository.ID, repository.LocalPath)
	if err != nil {
		return fmt.Errorf("list remote branches: %w", err)
	}
	for _, candidate := range branches {
		candidateType := strings.TrimSpace(candidate.Type)
		if strings.EqualFold(candidateType, "remote") && normalizeTaskPRBranch(candidate.Name) == wanted {
			return nil
		}
	}
	return worktree.ErrInvalidBaseBranch
}

func (s *Service) recordUnresolvedDefaultBranch(ctx context.Context, source *taskLaunchRecoverySource, taskRepositoryID string, cause error) error {
	const message = "The remote default branch could not be resolved."
	details := ""
	if cause != nil {
		details = routingerr.SanitizeError(cause).Error()
	}
	stamp := models.StableLaunchErrorStamp("default_branch_unresolved", taskRepositoryID, source.errorStamp)
	if source.sessionOwned {
		return s.recordSessionUnresolvedDefault(ctx, source, taskRepositoryID, details, stamp, message)
	}
	return s.recordTaskUnresolvedDefault(ctx, source, taskRepositoryID, details, stamp, message)
}

func (s *Service) recordSessionUnresolvedDefault(
	ctx context.Context,
	source *taskLaunchRecoverySource,
	taskRepositoryID, details, stamp, message string,
) error {
	value := models.LastAgentError{
		Message:          message,
		OccurredAt:       time.Now().UTC(),
		Code:             models.LaunchErrorCategoryDefaultBranchUnresolved,
		Details:          details,
		RecoveryActions:  []string{models.RecoveryActionPickBaseBranch},
		TaskRepositoryID: taskRepositoryID,
		StampValue:       stamp,
	}
	stored, err := s.taskLaunchRecoveryRepo.SetSessionMetadataKeyIfStamp(
		ctx,
		source.session.ID,
		models.SessionMetaKeyLastAgentError,
		source.errorStamp,
		value,
	)
	if err != nil {
		return err
	}
	if !stored {
		return ErrTaskLaunchRecoveryStale
	}
	if s.eventBus != nil {
		if err := s.publishTaskSessionErrorEvent(ctx, source.task.ID, source.session.ID, true, &value); err != nil {
			s.logger.Warn("failed to publish unresolved-default session error",
				zap.String("task_id", source.task.ID),
				zap.String("session_id", source.session.ID),
				zap.Error(err))
		}
	}
	return fmt.Errorf("%w: choose a base branch", worktree.ErrRemoteDefaultUnresolved)
}

func (s *Service) recordTaskUnresolvedDefault(
	ctx context.Context,
	source *taskLaunchRecoverySource,
	taskRepositoryID, details, stamp, message string,
) error {
	value := models.TaskLaunchError{
		Message:          message,
		OccurredAt:       time.Now().UTC(),
		Code:             models.LaunchErrorCategoryDefaultBranchUnresolved,
		Details:          details,
		RecoveryActions:  []string{models.RecoveryActionPickBaseBranch},
		TaskRepositoryID: taskRepositoryID,
		StampValue:       stamp,
	}
	stored, err := s.taskLaunchRecoveryRepo.SetTaskMetadataKeyIfStamp(
		ctx,
		source.task.ID,
		models.MetaKeyLastLaunchError,
		source.errorStamp,
		value,
	)
	if err != nil {
		return err
	}
	if !stored {
		return ErrTaskLaunchRecoveryStale
	}
	s.publishTaskLaunchErrorUpdate(ctx, source.task.ID)
	return fmt.Errorf("%w: choose a base branch", worktree.ErrRemoteDefaultUnresolved)
}

func (s *Service) relaunchRecoveredTask(ctx context.Context, req *TaskLaunchRecoveryRequest, source *taskLaunchRecoverySource) (string, error) {
	if source.sessionOwned {
		response, err := s.RecoverSession(ctx, req.TaskID, source.session.ID, "fresh_start")
		if err != nil {
			return "", err
		}
		if response == nil {
			return "", nil
		}
		return response.SessionID, nil
	}
	response, err := s.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID:         req.TaskID,
		Intent:         IntentStart,
		WorkflowStepID: source.task.WorkflowStepID,
	})
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", nil
	}
	return response.SessionID, nil
}

func (s *Service) markTaskLaunchReviewDone(ctx context.Context, req *TaskLaunchRecoveryRequest, source *taskLaunchRecoverySource) error {
	if s.taskLaunchRecoveryTasks == nil || s.githubService == nil || s.workflowStepGetter == nil {
		return fmt.Errorf("review completion recovery is unavailable")
	}
	finalStep, err := s.findTerminalFinalWorkflowStep(ctx, source.task)
	if err != nil {
		return err
	}
	if finalStep == nil {
		return fmt.Errorf("workflow has no valid terminal final step")
	}
	if s.taskLaunchRecoveryRepo == nil {
		return fmt.Errorf("task launch recovery repository is unavailable")
	}
	rows, err := s.taskLaunchRecoveryRepo.ListTaskRepositories(ctx, source.task.ID)
	if err != nil {
		return err
	}
	lookupCtx, cancel := context.WithTimeout(ctx, launchFailurePRLookupTimeout)
	defer cancel()
	prsByTask, err := s.githubService.ListTaskPRs(lookupCtx, []string{source.task.ID})
	if err != nil {
		return fmt.Errorf("recheck task pull requests: %w", err)
	}
	if !matchesAreTerminal(selectRelevantTaskPRs(rows, prsByTask[source.task.ID])) {
		return fmt.Errorf("relevant pull requests are not all terminal")
	}
	if source.task.WorkflowStepID == finalStep.ID && source.task.State != v1.TaskStateFailed {
		return nil
	}
	_, err = s.taskLaunchRecoveryTasks.MoveTaskWithOptions(ctx, source.task.ID, source.task.WorkflowID, finalStep.ID, finalStep.Position, taskservice.MoveTaskOptions{
		AllowFailedToCompletedRecovery: true,
		StepHistorySessionID:           req.SessionID,
	})
	return err
}

func (s *Service) findTerminalFinalWorkflowStep(ctx context.Context, task *models.Task) (*wfmodels.WorkflowStep, error) {
	if task == nil || strings.TrimSpace(task.WorkflowID) == "" || s.workflowStepGetter == nil {
		return nil, nil
	}
	step, err := findFinalWorkflowStep(ctx, s.workflowStepGetter, task.WorkflowStepID)
	if err != nil {
		return nil, err
	}
	if step == nil || step.WorkflowID != task.WorkflowID {
		return nil, nil
	}
	return step, nil
}

func (s *Service) clearTaskLaunchRecoverySource(ctx context.Context, source *taskLaunchRecoverySource) error {
	if source.sessionOwned {
		if s.taskLaunchRecoveryRepo == nil {
			return fmt.Errorf("session launch-error compare-and-clear is unavailable")
		}
		_, err := s.taskLaunchRecoveryRepo.RemoveSessionMetadataKeyIfStamp(ctx, source.session.ID, models.SessionMetaKeyLastAgentError, source.errorStamp)
		return err
	}
	s.clearTaskLaunchErrorIfStamp(ctx, source.task.ID, source.errorStamp)
	return nil
}
