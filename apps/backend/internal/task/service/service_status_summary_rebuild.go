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
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TaskStatusSummaryPRReader supplies already-persisted provider state for the
// one-time repair of a task whose summary row predates the projector.
type TaskStatusSummaryPRReader interface {
	ListTaskStatusSummaryPullRequests(
		context.Context,
		[]string,
	) (map[string][]statussummary.PullRequestInput, error)
}

// SetTaskStatusSummaryPRReader wires the optional provider-backed PR reader.
// The task service keeps the interface narrow so it does not depend on the
// GitHub package.
func (s *Service) SetTaskStatusSummaryPRReader(reader TaskStatusSummaryPRReader) {
	if s != nil {
		s.statusSummaryPRs = reader
	}
}

// ReconcileTaskStatusSummaries repairs stale pending state in existing rows and
// builds absent rows. All durable inputs are batch-loaded by the caller or by
// optional batch readers, so startup does not scan every historical task. A
// present nil result is an explicit cache invalidation; an absent key remains
// an ordinary partial-response omission.
func (s *Service) ReconcileTaskStatusSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
) (map[string]*statussummary.TaskStatusSummary, error) {
	if summaries == nil {
		summaries = make(map[string]*statussummary.TaskStatusSummary)
	}
	if s == nil || s.statusSummaries == nil || len(tasks) == 0 {
		return summaries, nil
	}
	activityByTask, activityObserved := s.loadSummaryActivity(ctx, taskIDs(tasks))
	failedTaskIDs, reconcileErr := s.reconcileExistingSummaries(
		ctx, tasks, sessionsByTask, pendingBySession, summaries, activityByTask, activityObserved,
	)
	rebuildTasks := tasks
	if len(failedTaskIDs) > 0 {
		rebuildTasks = make([]*models.Task, 0, len(tasks)-len(failedTaskIDs))
		for _, task := range tasks {
			if task == nil {
				continue
			}
			if _, failed := failedTaskIDs[task.ID]; !failed {
				rebuildTasks = append(rebuildTasks, task)
			}
		}
	}
	summaries = s.rebuildMissingSummaries(
		ctx, rebuildTasks, sessionsByTask, pendingBySession, summaries, activityByTask, activityObserved,
	)
	return summaries, reconcileErr
}

func (s *Service) reconcileExistingSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
	activityByTask map[string]time.Time,
	activityObserved bool,
) (map[string]struct{}, error) {
	var reconcileErr error
	failedTaskIDs := make(map[string]struct{})
	for _, task := range tasks {
		if task == nil || task.ID == "" || summaries[task.ID] == nil {
			continue
		}
		sessions := sessionsByTask[task.ID]
		action := pendingActionForTask(sessions, pendingBySession)
		reconciled, err := s.reconcileExistingSummary(
			ctx,
			task,
			summaries[task.ID],
			action,
			activityByTask[task.ID],
			activityObserved,
		)
		if err != nil {
			if reconciled != nil {
				summaries[task.ID] = reconciled
			} else {
				// A present nil entry is an explicit invalidation. DTO assembly
				// distinguishes it from an ordinarily absent partial projection so
				// clients can clear a known-stale cached summary.
				summaries[task.ID] = nil
			}
			failedTaskIDs[task.ID] = struct{}{}
			s.logSummaryRepairFailure(task.ID, "reconcile", err)
			reconcileErr = errors.Join(
				reconcileErr,
				fmt.Errorf("reconcile task %s status summary: %w", task.ID, err),
			)
			continue
		}
		if reconciled == nil {
			delete(summaries, task.ID)
			continue
		}
		summaries[task.ID] = reconciled
	}
	return failedTaskIDs, reconcileErr
}

func (s *Service) rebuildMissingSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
	activityByTask map[string]time.Time,
	activityObserved bool,
) map[string]*statussummary.TaskStatusSummary {
	missing := missingSummaryTasks(tasks, summaries)
	if len(missing) == 0 {
		return summaries
	}
	prByTask, prObserved := s.loadSummaryPRs(ctx, taskIDs(missing))
	environmentIDsByTask, environmentIDs := s.taskEnvironmentIDsForTasks(ctx, missing, sessionsByTask)
	gitByEnvironment, gitObserved := s.loadSummaryGit(ctx, environmentIDs)
	queuedByTask := s.loadQueuedSummaryCounts(ctx, taskIDs(missing))
	activityAtByTask := activityByTask
	now := time.Now().UTC()
	for _, task := range missing {
		activityAt := activityAtByTask[task.ID]
		s.rebuildMissingSummary(ctx, task, summaries, s.rebuildInput(
			taskLaunchErrorSummary(task),
			sessionsByTask[task.ID],
			environmentIDsByTask[task.ID],
			pendingBySession,
			gitByEnvironment,
			gitObserved,
			prByTask[task.ID],
			prObserved,
			queuedByTask[task.ID],
			activityAt,
			activityObserved,
			now,
		))
	}
	return summaries
}

func (s *Service) loadQueuedSummaryCounts(ctx context.Context, taskIDs []string) map[string]int {
	queuedByTask, err := s.CountPendingQueuedByTaskIDs(ctx, taskIDs)
	if err == nil {
		return queuedByTask
	}
	if s.logger != nil {
		s.logger.Warn("failed to load queued prompt counts for status summary repair", zap.Error(err))
	}
	return map[string]int{}
}

func (s *Service) loadSummaryActivity(ctx context.Context, taskIDs []string) (map[string]time.Time, bool) {
	if s.taskActivity == nil || len(taskIDs) == 0 {
		return nil, false
	}
	activityByTask, err := s.taskActivity.LoadTaskLastActivity(ctx, taskIDs)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to load task activity for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	return activityByTask, true
}

func (s *Service) rebuildMissingSummary(
	ctx context.Context,
	task *models.Task,
	summaries map[string]*statussummary.TaskStatusSummary,
	input statussummary.RebuildInput,
) {
	if task == nil || task.ID == "" {
		return
	}
	next := statussummary.BuildFromAuthoritative(input)
	next.Revision = 1
	next.UpdatedAt = input.Now
	if err := next.Validate(); err != nil {
		s.logSummaryRepairFailure(task.ID, "validate", err)
		return
	}
	accepted, err := s.statusSummaries.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Summary:     next,
	})
	if err != nil {
		s.logSummaryRepairFailure(task.ID, "persist", err)
		return
	}
	if accepted {
		summaries[task.ID] = &next
		s.publishReconciledSummary(ctx, task, next)
		return
	}
	// A projector event may have won the race while this repair was running.
	// Return that authoritative row instead of exposing a stale repair.
	rows, err := s.statusSummaries.LoadTaskStatusSummaries(ctx, []string{task.ID})
	if err != nil {
		s.logSummaryRepairFailure(task.ID, "reload", err)
		return
	}
	if stored := rows[task.ID]; stored != nil {
		summaries[task.ID] = stored
	}
}

const maxSummaryReconcileAttempts = 3

func (s *Service) reconcileExistingSummary(
	ctx context.Context,
	task *models.Task,
	current *statussummary.TaskStatusSummary,
	pendingAction string,
	authoritativeActivity time.Time,
	activityObserved bool,
) (*statussummary.TaskStatusSummary, error) {
	for attempt := 0; attempt < maxSummaryReconcileAttempts && current != nil; attempt++ {
		if !summaryNeedsReconcile(current, pendingAction, authoritativeActivity, activityObserved) {
			return current, nil
		}
		if err := prepareSummaryReconcileAttempt(ctx, attempt, current.Revision); err != nil {
			return nil, err
		}
		next := *current
		next.PendingAction = pendingAction
		if activityObserved && authoritativeActivity.After(time.Time{}) {
			next.LastActivityAt = maxSummaryActivity(current.LastActivityAt, authoritativeActivity)
		}
		next.Revision = current.Revision + 1
		next.UpdatedAt = advancedSummaryTime(current.UpdatedAt, time.Now().UTC())
		if err := next.Validate(); err != nil {
			return nil, fmt.Errorf("validate repair: %w", err)
		}
		accepted, err := s.statusSummaries.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
			TaskID:      task.ID,
			WorkspaceID: task.WorkspaceID,
			Summary:     next,
		})
		if err != nil {
			return nil, fmt.Errorf("persist repair: %w", err)
		}
		if accepted {
			s.publishReconciledSummary(ctx, task, next)
			return &next, nil
		}
		current, pendingAction, err = s.reloadSummaryReconcileState(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, nil
		}
	}
	if !summaryNeedsReconcile(current, pendingAction, authoritativeActivity, activityObserved) {
		return current, nil
	}
	s.logSummaryReconcileExhaustion(task.ID, current)
	return nil, errors.New("exhausted compare-and-set retries")
}

func summaryNeedsReconcile(
	current *statussummary.TaskStatusSummary,
	pendingAction string,
	authoritativeActivity time.Time,
	activityObserved bool,
) bool {
	if current == nil {
		return false
	}
	if current.PendingAction != pendingAction {
		return true
	}
	return activityObserved && authoritativeActivity.After(time.Time{}) &&
		(current.LastActivityAt == nil || authoritativeActivity.After(*current.LastActivityAt))
}

func (s *Service) reloadSummaryReconcileState(
	ctx context.Context,
	taskID string,
) (*statussummary.TaskStatusSummary, string, error) {
	rows, err := s.statusSummaries.LoadTaskStatusSummaries(ctx, []string{taskID})
	if err != nil {
		return nil, "", fmt.Errorf("reload after compare-and-set rejection: %w", err)
	}
	current := rows[taskID]
	if current == nil {
		return nil, "", nil
	}
	if s.sessions == nil {
		return nil, "", errors.New("reload sessions: session repository unavailable")
	}
	refreshedSessions, err := s.sessions.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, "", fmt.Errorf("reload sessions: %w", err)
	}
	pendingBySession, err := s.GetPendingActionsForSessions(ctx, taskSessionIDs(refreshedSessions))
	if err != nil {
		return nil, "", fmt.Errorf("reload pending actions: %w", err)
	}
	return current, pendingActionForTask(refreshedSessions, pendingBySession), nil
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
	payload := statussummary.SummaryUpdated{
		TaskID:      task.ID,
		WorkspaceID: task.WorkspaceID,
		Summary:     summary,
	}
	if err := s.eventBus.Publish(ctx, events.TaskStatusSummaryUpdated,
		bus.NewEvent(events.TaskStatusSummaryUpdated, "task-status-summary-reconciler", payload)); err != nil {
		s.logSummaryRepairFailure(task.ID, "publish", err)
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

func (s *Service) logSummaryRepairFailure(taskID, stage string, err error) {
	if s.logger != nil {
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
		if task == nil {
			continue
		}
		if s != nil && s.taskEnvironments != nil {
			environment, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, task.ID)
			if err != nil {
				if s.logger != nil {
					s.logger.Warn("failed to load task environment for status summary repair",
						zap.String("task_id", task.ID), zap.Error(err))
				}
			} else if environment != nil {
				add(task.ID, environment.ID)
			}
		}
		for _, session := range sessionsByTask[task.ID] {
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
	if s.statusSummaryPRs == nil || len(taskIDs) == 0 {
		return nil, false
	}
	prs, err := s.statusSummaryPRs.ListTaskStatusSummaryPullRequests(ctx, taskIDs)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to load task PR state for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	return prs, true
}

func (s *Service) loadSummaryGit(
	ctx context.Context,
	taskEnvironmentIDs []string,
) (map[string][]*models.GitSnapshot, bool) {
	if s.gitSnapshots == nil || len(taskEnvironmentIDs) == 0 {
		return nil, false
	}
	snapshots, err := s.gitSnapshots.GetLatestGitStatusSnapshotsByTaskEnvironmentIDs(ctx, taskEnvironmentIDs)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to load Git state for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	byEnvironment := make(map[string][]*models.GitSnapshot, len(taskEnvironmentIDs))
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.TaskEnvironmentID == "" {
			continue
		}
		byEnvironment[snapshot.TaskEnvironmentID] = append(byEnvironment[snapshot.TaskEnvironmentID], snapshot)
	}
	return byEnvironment, true
}

func (s *Service) rebuildInput(
	taskError *statussummary.ActiveErrorSummary,
	sessions []*models.TaskSession,
	taskEnvironmentIDs []string,
	pendingBySession map[string]models.TaskPendingAction,
	gitByEnvironment map[string][]*models.GitSnapshot,
	gitObserved bool,
	prs []statussummary.PullRequestInput,
	prObserved bool,
	queuedPromptCount int,
	lastActivityAt time.Time,
	activityObserved bool,
	now time.Time,
) statussummary.RebuildInput {
	input := statussummary.RebuildInput{
		Sessions:          make([]statussummary.RebuildSession, 0, len(sessions)),
		TaskError:         taskError,
		PendingActions:    make(map[string]string),
		ActivityObserved:  s.foregroundActivity != nil,
		LastActivityAt:    nil,
		PullRequests:      prs,
		PRObserved:        prObserved,
		GitObserved:       gitObserved,
		QueuedPromptCount: maxInt(queuedPromptCount, 0),
		Now:               now,
	}
	if activityObserved && !lastActivityAt.IsZero() {
		activityCopy := lastActivityAt.UTC()
		input.LastActivityAt = &activityCopy
	}
	countProvider, hasCountProvider := s.foregroundActivity.(activeSubagentCountProvider)
	for _, session := range sessions {
		if session == nil || session.ID == "" {
			continue
		}
		activity := ""
		activeSubagentCount := 0
		if s.foregroundActivity != nil {
			value := s.foregroundActivity.ForegroundActivity(session.ID)
			if session.State == models.TaskSessionStateRunning || value == v1.ForegroundActivityBackground {
				activity = string(value)
			}
			if hasCountProvider {
				activeSubagentCount = countProvider.ActiveSubagentCount(session.ID)
			}
		}
		var activeError *statussummary.ActiveErrorSummary
		if lastError, ok := models.LoadLastAgentError(session.Metadata); ok && !lastError.IsDismissed() {
			activeError = &statussummary.ActiveErrorSummary{
				SessionID:        session.ID,
				TaskRepositoryID: lastError.TaskRepositoryID,
				Stamp:            lastError.Stamp(),
				OccurredAt:       lastError.OccurredAt,
				Preview:          lastError.Message,
				Category:         lastError.Code,
				RecoveryActions:  lastError.RecoveryActions,
			}
		}
		input.Sessions = append(input.Sessions, statussummary.RebuildSession{
			ID:                  session.ID,
			State:               string(session.State),
			IsPrimary:           session.IsPrimary,
			ForegroundActivity:  activity,
			ActiveSubagentCount: activeSubagentCount,
			ActiveError:         activeError,
		})
		if action := string(pendingBySession[session.ID]); strings.TrimSpace(action) != "" {
			input.PendingActions[session.ID] = action
		}
	}
	for _, environmentID := range taskEnvironmentIDs {
		for _, snapshot := range gitByEnvironment[environmentID] {
			if snapshot == nil {
				continue
			}
			input.Git = append(input.Git, statussummary.RebuildGit{
				Repository: snapshotRepositoryKey(snapshot, ""),
				Summary:    gitSummaryFromSnapshot(snapshot),
			})
		}
	}
	return input
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
		TaskRepositoryID: errorValue.TaskRepositoryID,
		Stamp:            errorValue.Stamp(),
		OccurredAt:       errorValue.OccurredAt,
		Preview:          errorValue.Message,
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
