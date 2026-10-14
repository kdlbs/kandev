package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/statussummary"
)

func (s *Service) reconcileExistingSummary(
	ctx context.Context,
	task *models.Task,
	current *statussummary.TaskStatusSummary,
	pendingAction string,
	authoritativeActivity time.Time,
	activityObserved bool,
	runningSessions runningSessionObservation,
	launchQueueValues ...*statussummary.LaunchQueueSummary,
) (*statussummary.TaskStatusSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	completionGate, completionGateObserved, err := s.completionGateSummary(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("load completion gate: %w", err)
	}
	return s.reconcileExistingSummaryWithGateObservation(
		ctx, task, current, pendingAction, authoritativeActivity, activityObserved,
		runningSessions, completionGateSummaryObservation{summary: completionGate, observed: completionGateObserved}, launchQueueValues...,
	)
}

func (s *Service) reconcileExistingSummaryWithGateObservation(
	ctx context.Context,
	task *models.Task,
	current *statussummary.TaskStatusSummary,
	pendingAction string,
	authoritativeActivity time.Time,
	activityObserved bool,
	runningSessions runningSessionObservation,
	gateObservation completionGateSummaryObservation,
	launchQueueValues ...*statussummary.LaunchQueueSummary,
) (*statussummary.TaskStatusSummary, error) {
	completionGate, completionGateObserved, err := completionGateObservationValue(ctx, gateObservation)
	if err != nil {
		return nil, err
	}

	launchQueue, launchQueueObserved := summaryReconcileLaunchQueue(task, current, launchQueueValues)
	for attempt := 0; attempt < maxSummaryReconcileAttempts && current != nil; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !summaryNeedsReconcile(current, pendingAction, authoritativeActivity, activityObserved, runningSessions, launchQueue, launchQueueObserved, completionGate, completionGateObserved) {
			return overlayLaunchQueueObservation(current, launchQueue, launchQueueObserved), nil
		}
		if err := prepareSummaryReconcileAttempt(ctx, attempt, current.Revision); err != nil {
			return nil, err
		}
		next, err := nextReconciledSummary(current, pendingAction, runningSessions, authoritativeActivity, activityObserved, launchQueue, completionGate, completionGateObserved)
		if err != nil {
			return nil, err
		}
		accepted, err := s.persistReconciledSummary(ctx, task, next)
		if err != nil {
			return nil, err
		}
		if accepted {
			return &next, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		current, pendingAction, runningSessions, launchQueue, err = s.reloadSummaryReconcileState(ctx, task.ID, launchQueueObserved)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, nil
		}
		completionGate, completionGateObserved, err = s.completionGateSummary(ctx, task.ID)
		if err != nil {
			return nil, fmt.Errorf("reload completion gate: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return s.finishSummaryReconcile(ctx, task.ID, current, pendingAction, authoritativeActivity, activityObserved,
		runningSessions, launchQueue, launchQueueObserved, completionGate, completionGateObserved)
}

// overlayLaunchQueueObservation returns a response-only copy when the live
// queue has only a newer capacity sample. Observation time is intentionally
// excluded from the durable equality contract, but task-list consumers still
// need the fresh timestamp to render age and connectivity state accurately.
func overlayLaunchQueueObservation(
	current *statussummary.TaskStatusSummary,
	observed *statussummary.LaunchQueueSummary,
	observedEnabled bool,
) *statussummary.TaskStatusSummary {
	if !observedEnabled || current == nil || observed == nil || current.LaunchQueue == nil ||
		!statussummaryLaunchQueueEqual(current.LaunchQueue, observed) {
		return current
	}
	if current.LaunchQueue.Capacity == nil || observed.Capacity == nil ||
		current.LaunchQueue.Capacity.ObservedAt.Equal(observed.Capacity.ObservedAt) {
		return current
	}
	copy := *current
	copy.LaunchQueue = cloneLaunchQueueForService(observed)
	return &copy
}

func summaryNeedsReconcile(
	current *statussummary.TaskStatusSummary,
	pendingAction string,
	authoritativeActivity time.Time,
	activityObserved bool,
	runningSessions runningSessionObservation,
	launchQueue *statussummary.LaunchQueueSummary,
	launchQueueObserved bool,
	completionGate *statussummary.CompletionGateSummary,
	completionGateObserved bool,
) bool {
	if current == nil {
		return false
	}
	if current.PendingAction != pendingAction {
		return true
	}
	if runningSessions.observed && (current.HasRunningSession == nil || *current.HasRunningSession != runningSessions.running) {
		return true
	}
	if launchQueueObserved && !statussummaryLaunchQueueEqual(current.LaunchQueue, launchQueue) {
		return true
	}
	if completionGateObserved && !equalCompletionGateSummary(current.CompletionGate, completionGate) {
		return true
	}
	return activityObserved && authoritativeActivity.After(time.Time{}) &&
		(current.LastActivityAt == nil || authoritativeActivity.After(*current.LastActivityAt))
}

func cloneCompletionGateForService(gate *statussummary.CompletionGateSummary) *statussummary.CompletionGateSummary {
	if gate == nil {
		return nil
	}
	copy := *gate
	return &copy
}

func equalCompletionGateSummary(left, right *statussummary.CompletionGateSummary) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func (s *Service) reloadSummaryReconcileState(
	ctx context.Context,
	taskID string,
	loadLaunchQueue bool,
) (*statussummary.TaskStatusSummary, string, runningSessionObservation, *statussummary.LaunchQueueSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", runningSessionObservation{}, nil, err
	}
	rows, err := s.statusSummaries.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		return nil, "", runningSessionObservation{}, nil, fmt.Errorf("reload after compare-and-set rejection: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", runningSessionObservation{}, nil, err
	}
	current := rows[taskID]
	if current == nil {
		return nil, "", runningSessionObservation{}, nil, nil
	}
	if s.sessions == nil {
		return nil, "", runningSessionObservation{}, nil, errors.New("reload sessions: session repository unavailable")
	}
	refreshedSessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, "", runningSessionObservation{}, nil, fmt.Errorf("reload sessions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", runningSessionObservation{}, nil, err
	}
	runningSessions := runningSessionSummary(refreshedSessions, true)
	pendingBySession, err := s.GetPendingActionsForSessions(ctx, taskSessionIDs(refreshedSessions))
	if err != nil {
		return nil, "", runningSessionObservation{}, nil, fmt.Errorf("reload pending actions: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", runningSessionObservation{}, nil, err
	}
	if !loadLaunchQueue {
		return current, pendingActionForTask(refreshedSessions, pendingBySession), runningSessions, nil, nil
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, "", runningSessionObservation{}, nil, fmt.Errorf("reload task for launch queue: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", runningSessionObservation{}, nil, err
	}
	queues := s.launchQueueSummaries(ctx, []*models.Task{task})
	if err := ctx.Err(); err != nil {
		return nil, "", runningSessionObservation{}, nil, err
	}
	return current, pendingActionForTask(refreshedSessions, pendingBySession), runningSessions, queues[taskID], nil
}

func maxSummaryActivity(current *time.Time, candidate time.Time) *time.Time {
	if candidate.IsZero() {
		if current == nil {
			return nil
		}
		copy := current.UTC()
		return &copy
	}
	if current != nil && !candidate.After(*current) {
		copy := current.UTC()
		return &copy
	}
	copy := candidate.UTC()
	return &copy
}

func (s *Service) logSummaryReconcileExhaustion(
	taskID string,
	current *statussummary.TaskStatusSummary,
) {
	if s.logger != nil {
		lastRevision := uint64(0)
		if current != nil {
			lastRevision = current.Revision
		}
		s.logger.Warn("task status summary compare-and-set retries exhausted",
			zap.String("task_id", taskID),
			zap.Int("attempts", maxSummaryReconcileAttempts),
			zap.Uint64("last_revision", lastRevision))
	}
}

const summaryReconcileInitialRetryDelay = time.Millisecond

func prepareSummaryReconcileAttempt(ctx context.Context, attempt int, revision uint64) error {
	if revision == ^uint64(0) {
		return errors.New("revision overflow")
	}
	if err := waitForSummaryReconcileRetry(ctx, attempt); err != nil {
		return fmt.Errorf("wait before compare-and-set retry: %w", err)
	}
	return nil
}

func waitForSummaryReconcileRetry(ctx context.Context, attempt int) error {
	if attempt <= 0 {
		return nil
	}
	timer := time.NewTimer(summaryReconcileInitialRetryDelay << (attempt - 1))
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func advancedSummaryTime(previous, now time.Time) time.Time {
	if !now.After(previous) {
		return previous.Add(time.Nanosecond)
	}
	return now
}

func (s *Service) publishReconciledSummary(
	ctx context.Context,
	task *models.Task,
	summary statussummary.TaskStatusSummary,
) {
	if s.eventBus == nil {
		return
	}
	// A committed summary needs its matching event even if the requesting list
	// operation was canceled after the repository accepted the write.
	publishCtx := context.WithoutCancel(ctx)
	payload := statussummary.SummaryUpdated{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Summary:     summary,
	}
	if err := s.eventBus.Publish(publishCtx, events.TaskStatusSummaryUpdated,
		bus.NewEvent(events.TaskStatusSummaryUpdated, "task-status-summary-reconciler", payload)); err != nil {
		s.logSummaryRepairFailure(publishCtx, task.ID, "publish", err)
	}
}

func pendingActionForTask(
	sessions []*models.TaskSession,
	actions map[string]models.TaskPendingAction,
) string {
	hasClarification := false
	for _, session := range sessions {
		if session == nil || (session.State != models.TaskSessionStateRunning &&
			session.State != models.TaskSessionStateWaitingForInput) {
			continue
		}
		switch actions[session.ID] {
		case models.TaskPendingActionPermission:
			return string(models.TaskPendingActionPermission)
		case models.TaskPendingActionClarification:
			hasClarification = true
		}
	}
	if hasClarification {
		return string(models.TaskPendingActionClarification)
	}
	return ""
}

func (s *Service) logSummaryRepairFailure(ctx context.Context, taskID, stage string, err error) {
	if s.logger != nil && !isRequestCancellationError(ctx, err) {
		s.logger.Warn("failed to repair task status summary; continuing task list hydration",
			zap.String("task_id", taskID), zap.String("stage", stage), zap.Error(err))
	}
}

func missingSummaryTasks(tasks []*models.Task, summaries map[string]*statussummary.TaskStatusSummary) []*models.Task {
	missing := make([]*models.Task, 0, len(tasks))
	for _, task := range tasks {
		if task != nil && task.ID != "" && summaries[task.ID] == nil {
			missing = append(missing, task)
		}
	}
	return missing
}

func taskIDs(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task != nil && task.ID != "" {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func (s *Service) taskEnvironmentIDsForTasks(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
) (map[string][]string, []string) {
	seen := make(map[string]struct{})
	seenByTask := make(map[string]map[string]struct{}, len(tasks))
	idsByTask := make(map[string][]string, len(tasks))
	ids := make([]string, 0)
	add := func(taskID, environmentID string) {
		if environmentID == "" {
			return
		}
		taskSeen := seenByTask[taskID]
		if taskSeen == nil {
			taskSeen = make(map[string]struct{})
			seenByTask[taskID] = taskSeen
		}
		if _, ok := taskSeen[environmentID]; ok {
			return
		}
		taskSeen[environmentID] = struct{}{}
		idsByTask[taskID] = append(idsByTask[taskID], environmentID)
		if _, ok := seen[environmentID]; ok {
			return
		}
		seen[environmentID] = struct{}{}
		ids = append(ids, environmentID)
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return idsByTask, ids
		}
		if task == nil {
			continue
		}
		environmentID := s.summaryTaskEnvironmentID(ctx, task.ID)
		if ctx.Err() != nil {
			return idsByTask, ids
		}
		add(task.ID, environmentID)
		for _, session := range sessionsByTask[task.ID] {
			if ctx.Err() != nil {
				return idsByTask, ids
			}
			if session == nil || session.TaskEnvironmentID == "" {
				continue
			}
			add(task.ID, session.TaskEnvironmentID)
		}
	}
	return idsByTask, ids
}

func (s *Service) loadSummaryPRs(
	ctx context.Context,
	taskIDs []string,
) (map[string][]statussummary.PullRequestInput, bool) {
	if ctx.Err() != nil || s.statusSummaryPRs == nil || len(taskIDs) == 0 {
		return nil, false
	}
	prs, err := s.statusSummaryPRs.ListTaskStatusSummaryPullRequests(ctx, taskIDs)
	if err != nil {
		if s.logger != nil && !isRequestCancellationError(ctx, err) {
			s.logger.Warn("failed to load task PR state for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	return prs, true
}

func (s *Service) loadSummaryGit(
	ctx context.Context,
	taskEnvironmentIDs []string,
) (map[string][]*models.GitSnapshot, bool) {
	if ctx.Err() != nil || s.gitSnapshots == nil || len(taskEnvironmentIDs) == 0 {
		return nil, false
	}
	snapshots, err := s.gitSnapshots.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, taskEnvironmentIDs)
	if err != nil {
		if s.logger != nil && !isRequestCancellationError(ctx, err) {
			s.logger.Warn("failed to load Git state for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	byEnvironment := make(map[string][]*models.GitSnapshot, len(taskEnvironmentIDs))
	for _, snapshot := range snapshots {
		if ctx.Err() != nil {
			return nil, false
		}
		if snapshot == nil || snapshot.TaskEnvironmentID == "" {
			continue
		}
		byEnvironment[snapshot.TaskEnvironmentID] = append(byEnvironment[snapshot.TaskEnvironmentID], snapshot)
	}
	return byEnvironment, true
}

func taskLaunchErrorSummary(task *models.Task) *statussummary.ActiveErrorSummary {
	if task == nil {
		return nil
	}
	errorValue, ok := models.LoadTaskLaunchError(task.Metadata)
	if !ok {
		return nil
	}
	return &statussummary.ActiveErrorSummary{
		Scope:            models.ErrorScopeTask,
		SessionID:        errorValue.SessionID,
		TaskRepositoryID: errorValue.TaskRepositoryID,
		Stamp:            errorValue.Stamp(),
		OccurredAt:       errorValue.OccurredAt,
		Preview:          errorValue.Message,
		Details:          errorValue.Details,
		Category:         errorValue.Code,
		RecoveryActions:  errorValue.RecoveryActions,
	}
}

func snapshotRepositoryKey(snapshot *models.GitSnapshot, fallback string) string {
	if snapshot != nil && snapshot.Metadata != nil {
		if repository, ok := snapshot.Metadata["repository_name"].(string); ok && strings.TrimSpace(repository) != "" {
			return repository
		}
	}
	return fallback
}

func gitSummaryFromSnapshot(snapshot *models.GitSnapshot) statussummary.GitSummary {
	if snapshot == nil {
		return statussummary.GitSummary{}
	}
	return statussummary.GitSummary{
		Additions:             nonNegative(snapshot.Metadata, "branch_additions"),
		Deletions:             nonNegative(snapshot.Metadata, "branch_deletions"),
		ChangedFiles:          changedFilesFromSnapshot(snapshot),
		Ahead:                 maxInt(snapshot.Ahead, 0),
		Behind:                maxInt(snapshot.Behind, 0),
		ComparisonUnavailable: snapshotComparisonUnavailable(snapshot),
	}
}

func snapshotComparisonUnavailable(snapshot *models.GitSnapshot) bool {
	if snapshot == nil || snapshot.Metadata == nil {
		return false
	}
	value, _ := snapshot.Metadata["comparison_status"].(string)
	return value == "unavailable"
}

func changedFilesFromSnapshot(snapshot *models.GitSnapshot) int {
	if snapshot == nil {
		return 0
	}
	if _, ok := snapshot.Metadata["changed_files"]; ok {
		return nonNegative(snapshot.Metadata, "changed_files")
	}
	count := 0
	for _, key := range []string{"modified", "added", "deleted", "untracked", "renamed"} {
		count += collectionLength(snapshot.Metadata[key])
	}
	if count == 0 {
		count = len(snapshot.Files)
	}
	return count
}

func nonNegative(values map[string]interface{}, key string) int {
	if values == nil {
		return 0
	}
	value := values[key]
	switch number := value.(type) {
	case int:
		return maxInt(number, 0)
	case int64:
		return maxInt(int(number), 0)
	case float64:
		return maxInt(int(number), 0)
	default:
		return 0
	}
}

func collectionLength(value interface{}) int {
	if value == nil {
		return 0
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return reflected.Len()
	default:
		return 0
	}
}

func maxInt(value, minimum int) int {
	if value < minimum {
		return minimum
	}
	return value
}

func nextReconciledSummary(current *statussummary.TaskStatusSummary, pendingAction string, runningSessions runningSessionObservation, authoritativeActivity time.Time, activityObserved bool, launchQueue *statussummary.LaunchQueueSummary, completionGate *statussummary.CompletionGateSummary, completionGateObserved bool) (statussummary.TaskStatusSummary, error) {
	next := *current
	next.PendingAction = pendingAction
	if runningSessions.observed {
		running := runningSessions.running
		next.HasRunningSession = &running
	}
	if activityObserved && authoritativeActivity.After(time.Time{}) {
		next.LastActivityAt = maxSummaryActivity(current.LastActivityAt, authoritativeActivity)
	}
	next.LaunchQueue = cloneLaunchQueueForService(launchQueue)
	if completionGateObserved {
		next.CompletionGate = cloneCompletionGateForService(completionGate)
	}
	next.Revision = current.Revision + 1
	next.UpdatedAt = advancedSummaryTime(current.UpdatedAt, time.Now().UTC())
	if err := next.Validate(); err != nil {
		return statussummary.TaskStatusSummary{}, fmt.Errorf("validate repair: %w", err)
	}
	return next, nil
}

func completionGateObservationValue(ctx context.Context, observation completionGateSummaryObservation) (*statussummary.CompletionGateSummary, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if observation.err != nil {
		return nil, false, fmt.Errorf("load completion gate: %w", observation.err)
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return observation.summary, observation.observed, nil
}

func (s *Service) persistReconciledSummary(ctx context.Context, task *models.Task, next statussummary.TaskStatusSummary) (bool, error) {
	accepted, err := s.statusSummaries.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Summary:     next,
	})
	if err != nil {
		return false, fmt.Errorf("persist repair: %w", err)
	}
	if accepted {
		s.publishReconciledSummary(ctx, task, next)
	}
	return accepted, nil
}

func (s *Service) summaryTaskEnvironmentID(ctx context.Context, taskID string) string {
	if s == nil || s.taskEnvironments == nil {
		return ""
	}
	environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil {
		if s.logger != nil && !isRequestCancellationError(ctx, err) {
			s.logger.Warn("failed to load task environment for status summary repair", zap.String("task_id", taskID), zap.Error(err))
		}
		return ""
	}
	if ctx.Err() != nil || environment == nil {
		return ""
	}
	return environment.ID
}

func summaryReconcileLaunchQueue(task *models.Task, current *statussummary.TaskStatusSummary, values []*statussummary.LaunchQueueSummary) (*statussummary.LaunchQueueSummary, bool) {
	// Partial task snapshots without metadata cannot establish queue authority.
	observed := len(values) > 0 && (task.Metadata != nil || current.LaunchQueue != nil)
	if !observed {
		return nil, false
	}
	return values[0], true
}

func (s *Service) finishSummaryReconcile(
	ctx context.Context,
	taskID string,
	current *statussummary.TaskStatusSummary,
	pendingAction string,
	authoritativeActivity time.Time,
	activityObserved bool,
	runningSessions runningSessionObservation,
	launchQueue *statussummary.LaunchQueueSummary,
	launchQueueObserved bool,
	completionGate *statussummary.CompletionGateSummary,
	completionGateObserved bool,
) (*statussummary.TaskStatusSummary, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !summaryNeedsReconcile(current, pendingAction, authoritativeActivity, activityObserved, runningSessions, launchQueue, launchQueueObserved, completionGate, completionGateObserved) {
		return overlayLaunchQueueObservation(current, launchQueue, launchQueueObserved), nil
	}
	s.logSummaryReconcileExhaustion(taskID, current)
	return nil, errors.New("exhausted compare-and-set retries")
}
