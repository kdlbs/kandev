package orchestrator

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	terminalPRLaunchMessage = "The linked pull request is already closed or merged."
)

type taskPRMatch struct {
	taskRepositoryID string
	pr               *github.TaskPR
}

// selectRelevantTaskPRs matches PR links to the exact task-repository row that
// would launch the work. A persisted PR number is authoritative; branch
// matching is only a compatibility fallback for rows without that identity.
func selectRelevantTaskPRs(
	taskRepositories []*models.TaskRepository,
	prs []*github.TaskPR,
) []taskPRMatch {
	var matches []taskPRMatch
	for _, taskRepository := range taskRepositories {
		if taskRepository == nil {
			continue
		}
		prNumber := taskRepositoryPRNumber(taskRepository.Metadata)
		if prNumber > 0 {
			for _, pr := range prs {
				if pr == nil || pr.RepositoryID != taskRepository.RepositoryID || pr.PRNumber != prNumber {
					continue
				}
				matches = append(matches, taskPRMatch{
					taskRepositoryID: taskRepository.ID,
					pr:               pr,
				})
			}
			continue
		}

		checkoutBranch := normalizeTaskPRBranch(taskRepository.CheckoutBranch)
		if checkoutBranch == "" {
			continue
		}
		for _, pr := range prs {
			if pr == nil || pr.RepositoryID != taskRepository.RepositoryID {
				continue
			}
			if normalizeTaskPRBranch(pr.HeadBranch) != checkoutBranch {
				continue
			}
			matches = append(matches, taskPRMatch{
				taskRepositoryID: taskRepository.ID,
				pr:               pr,
			})
		}
	}
	return matches
}

func normalizeTaskPRBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	for _, prefix := range []string{
		"refs/heads/",
		"refs/remotes/origin/",
		"origin/",
	} {
		if strings.HasPrefix(branch, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(branch, prefix))
		}
	}
	return branch
}

func shouldSkipForTerminalTaskPRs(prs []*github.TaskPR) bool {
	if len(prs) == 0 {
		return false
	}
	for _, pr := range prs {
		if pr == nil {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(pr.State)) {
		case githubPRStateClosed, githubPRStateMerged:
			continue
		default:
			return false
		}
	}
	return true
}

func matchesAreTerminal(matches []taskPRMatch) bool {
	if len(matches) == 0 {
		return false
	}
	prs := make([]*github.TaskPR, 0, len(matches))
	for _, match := range matches {
		prs = append(prs, match.pr)
	}
	return shouldSkipForTerminalTaskPRs(prs)
}

func taskPRLaunchErrorStamp(matches []taskPRMatch) string {
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		if match.pr == nil {
			continue
		}
		keys = append(keys, strings.Join([]string{
			match.taskRepositoryID,
			match.pr.RepositoryID,
			strconv.Itoa(match.pr.PRNumber),
			strings.ToLower(strings.TrimSpace(match.pr.State)),
		}, "\x00"))
	}
	if len(keys) == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := append([]string{"pr_already_closed"}, keys...)
	return models.StableLaunchErrorStamp(parts...)
}

func taskPRLaunchErrorRepositoryID(matches []taskPRMatch) string {
	var repositoryID string
	for _, match := range matches {
		if strings.TrimSpace(match.taskRepositoryID) == "" {
			return ""
		}
		if repositoryID == "" {
			repositoryID = match.taskRepositoryID
			continue
		}
		if repositoryID != match.taskRepositoryID {
			return ""
		}
	}
	return repositoryID
}

func workflowHasValidTerminalFinalStep(
	ctx context.Context,
	getter WorkflowStepGetter,
	stepID string,
) bool {
	finalStep, err := findFinalWorkflowStep(ctx, getter, stepID)
	return err == nil && finalStep != nil
}

// shouldSkipTerminalPRAutoStart is the pre-session PR gate. Lookup failures
// deliberately return false so a stale integration cannot suppress work.
func (s *Service) shouldSkipTerminalPRAutoStart(
	ctx context.Context,
	task *models.Task,
) bool {
	if task == nil || task.ID == "" || s.githubService == nil {
		return false
	}
	store, ok := s.repo.(interface {
		ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	})
	if !ok {
		return false
	}
	taskRepositories, err := store.ListTaskRepositories(ctx, task.ID)
	if err != nil {
		s.logger.Debug("failed to load task repositories for PR auto-start gate",
			zap.String("task_id", task.ID), zap.Error(err))
		return false
	}
	lookupCtx, cancel := context.WithTimeout(ctx, launchFailurePRLookupTimeout)
	defer cancel()
	prsByTask, err := s.githubService.ListTaskPRs(lookupCtx, []string{task.ID})
	if err != nil {
		s.logger.Debug("failed to load task PRs for auto-start gate",
			zap.String("task_id", task.ID), zap.Error(err))
		return false
	}
	matches := selectRelevantTaskPRs(taskRepositories, prsByTask[task.ID])
	if !matchesAreTerminal(matches) {
		return false
	}

	errorValue := models.TaskLaunchError{
		Message:          terminalPRLaunchMessage,
		OccurredAt:       time.Now().UTC(),
		Code:             models.LaunchErrorCategoryPRAlreadyClosed,
		RecoveryActions:  terminalPRRecoveryActions(ctx, s.workflowStepGetter, task.WorkflowStepID),
		TaskRepositoryID: taskPRLaunchErrorRepositoryID(matches),
		StampValue:       taskPRLaunchErrorStamp(matches),
	}
	return s.persistTaskLaunchError(ctx, task.ID, errorValue)
}

// resolveLaunchFailureReviewEligibility is deliberately narrow. It is used by
// the executor after a session launch fails, so lookup failures are returned to
// the caller and only suppress the optional action.
func (s *Service) resolveLaunchFailureReviewEligibility(
	ctx context.Context,
	taskID string,
) (bool, error) {
	if s.githubService == nil || s.workflowStepGetter == nil {
		return false, nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	if task == nil {
		return false, fmt.Errorf("task %q was not found", taskID)
	}
	store, ok := s.repo.(interface {
		ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
	})
	if !ok {
		return false, fmt.Errorf("task repository reader is unavailable")
	}
	taskRepositories, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return false, err
	}
	prsByTask, err := s.githubService.ListTaskPRs(ctx, []string{taskID})
	if err != nil {
		return false, err
	}
	matches := selectRelevantTaskPRs(taskRepositories, prsByTask[taskID])
	return matchesAreTerminal(matches) && workflowHasValidTerminalFinalStep(
		ctx, s.workflowStepGetter, task.WorkflowStepID,
	), nil
}

func terminalPRRecoveryActions(
	ctx context.Context,
	getter WorkflowStepGetter,
	workflowStepID string,
) []string {
	if !workflowHasValidTerminalFinalStep(ctx, getter, workflowStepID) {
		return nil
	}
	return []string{models.RecoveryActionMarkReviewDone}
}

func (s *Service) persistTaskLaunchError(
	ctx context.Context,
	taskID string,
	errorValue models.TaskLaunchError,
) bool {
	persister, ok := s.repo.(taskLaunchErrorPersister)
	if !ok {
		s.logger.Warn("task repository cannot persist PR auto-start error",
			zap.String("task_id", taskID))
		return false
	}
	stored, noOp, err := persister.SetTaskMetadataKeyIfDifferentStamp(
		ctx, taskID, models.MetaKeyLastLaunchError, errorValue.Stamp(), errorValue,
	)
	if err != nil {
		s.logger.Warn("failed to persist PR auto-start error",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	if stored {
		s.publishTaskLaunchErrorUpdate(ctx, taskID)
	}
	return stored || noOp
}

type taskLaunchErrorPersister interface {
	SetTaskMetadataKeyIfDifferentStamp(
		context.Context, string, string, string, interface{},
	) (stored bool, noOp bool, err error)
}

func (s *Service) publishTaskLaunchErrorUpdate(ctx context.Context, taskID string) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		if err != nil {
			s.logger.Debug("failed to reload task after launch-error metadata update",
				zap.String("task_id", taskID), zap.Error(err))
		}
		return
	}
	s.publishTaskUpdated(ctx, task)
}

type taskLaunchErrorStampRemover interface {
	RemoveTaskMetadataKeyIfStamp(
		ctx context.Context,
		taskID, key, expectedStamp string,
	) (bool, error)
}

// clearTaskLaunchErrorIfStamp removes only the error observed by the launch
// that just succeeded. The repository implementation performs the comparison
// and removal in one SQL statement; the fallback keeps test doubles useful.
func (s *Service) clearTaskLaunchErrorIfStamp(
	ctx context.Context,
	taskID, expectedStamp string,
) bool {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(expectedStamp) == "" {
		return false
	}
	if remover, ok := s.repo.(taskLaunchErrorStampRemover); ok {
		removed, err := remover.RemoveTaskMetadataKeyIfStamp(
			ctx, taskID, models.MetaKeyLastLaunchError, expectedStamp,
		)
		if err != nil {
			s.logger.Warn("failed to clear task launch error",
				zap.String("task_id", taskID), zap.Error(err))
		}
		if removed {
			s.publishTaskLaunchErrorUpdate(ctx, taskID)
		}
		return removed
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return false
	}
	current, found := models.LoadTaskLaunchError(task.Metadata)
	if !found || !current.MatchesStamp(expectedStamp) {
		return false
	}
	if remover, ok := s.repo.(taskMetadataKeyRemover); ok {
		removed, err := remover.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyLastLaunchError)
		if err != nil {
			s.logger.Warn("failed to clear task launch error",
				zap.String("task_id", taskID), zap.Error(err))
			return false
		}
		if removed {
			s.publishTaskLaunchErrorUpdate(ctx, taskID)
		}
		return removed
	}
	return false
}
