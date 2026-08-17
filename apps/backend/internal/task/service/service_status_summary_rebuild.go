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
// optional batch readers, so startup does not scan every historical task.
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
	failedTaskIDs, reconcileErr := s.reconcileExistingSummaries(
		ctx, tasks, sessionsByTask, pendingBySession, summaries,
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
		ctx, rebuildTasks, sessionsByTask, pendingBySession, summaries,
	)
	return summaries, reconcileErr
}

func (s *Service) reconcileExistingSummaries(
	ctx context.Context,
	tasks []*models.Task,
	sessionsByTask map[string][]*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	summaries map[string]*statussummary.TaskStatusSummary,
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
		)
		if err != nil {
			if reconciled != nil {
				summaries[task.ID] = reconciled
			} else {
				delete(summaries, task.ID)
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
) map[string]*statussummary.TaskStatusSummary {
	missing := missingSummaryTasks(tasks, summaries)
	if len(missing) == 0 {
		return summaries
	}
	prByTask, prObserved := s.loadSummaryPRs(ctx, taskIDs(missing))
	gitBySession, gitObserved := s.loadSummaryGit(ctx, sessionIDsForTasks(missing, sessionsByTask))
	queuedByTask := s.loadQueuedSummaryCounts(ctx, taskIDs(missing))
	now := time.Now().UTC()
	for _, task := range missing {
		s.rebuildMissingSummary(ctx, task, summaries, s.rebuildInput(
			sessionsByTask[task.ID],
			pendingBySession,
			gitBySession,
			gitObserved,
			prByTask[task.ID],
			prObserved,
			queuedByTask[task.ID],
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
) (*statussummary.TaskStatusSummary, error) {
	for attempt := 0; attempt < maxSummaryReconcileAttempts && current != nil; attempt++ {
		if current.PendingAction == pendingAction {
			return current, nil
		}
		if err := prepareSummaryReconcileAttempt(ctx, attempt, current.Revision); err != nil {
			return nil, err
		}
		next := *current
		next.PendingAction = pendingAction
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
		rows, err := s.statusSummaries.LoadTaskStatusSummaries(ctx, []string{task.ID})
		if err != nil {
			return nil, fmt.Errorf("reload after compare-and-set rejection: %w", err)
		}
		current = rows[task.ID]
		if current == nil {
			return nil, nil
		}
		if s.sessions == nil {
			return nil, errors.New("reload sessions: session repository unavailable")
		}
		refreshedSessions, err := s.sessions.ListTaskSessions(ctx, task.ID)
		if err != nil {
			return nil, fmt.Errorf("reload sessions: %w", err)
		}
		pendingBySession, err := s.GetPendingActionsForSessions(ctx, taskSessionIDs(refreshedSessions))
		if err != nil {
			return nil, fmt.Errorf("reload pending actions: %w", err)
		}
		pendingAction = pendingActionForTask(refreshedSessions, pendingBySession)
	}
	if current != nil && current.PendingAction == pendingAction {
		return current, nil
	}
	s.logSummaryReconcileExhaustion(task.ID, current)
	return nil, errors.New("exhausted compare-and-set retries")
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

func sessionIDsForTasks(tasks []*models.Task, sessionsByTask map[string][]*models.TaskSession) []string {
	seen := make(map[string]struct{})
	ids := make([]string, 0)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		for _, session := range sessionsByTask[task.ID] {
			if session == nil || session.ID == "" {
				continue
			}
			if _, ok := seen[session.ID]; ok {
				continue
			}
			seen[session.ID] = struct{}{}
			ids = append(ids, session.ID)
		}
	}
	return ids
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
	sessionIDs []string,
) (map[string]*models.GitSnapshot, bool) {
	if s.gitSnapshots == nil || len(sessionIDs) == 0 {
		return nil, false
	}
	snapshots, err := s.gitSnapshots.GetLatestGitSnapshotsBySessionIDs(ctx, sessionIDs)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to load Git state for status summary repair", zap.Error(err))
		}
		return nil, false
	}
	return snapshots, true
}

func (s *Service) rebuildInput(
	sessions []*models.TaskSession,
	pendingBySession map[string]models.TaskPendingAction,
	gitBySession map[string]*models.GitSnapshot,
	gitObserved bool,
	prs []statussummary.PullRequestInput,
	prObserved bool,
	queuedPromptCount int,
	now time.Time,
) statussummary.RebuildInput {
	input := statussummary.RebuildInput{
		Sessions:          make([]statussummary.RebuildSession, 0, len(sessions)),
		PendingActions:    make(map[string]string),
		ActivityObserved:  s.foregroundActivity != nil,
		PullRequests:      prs,
		PRObserved:        prObserved,
		GitObserved:       gitObserved,
		QueuedPromptCount: maxInt(queuedPromptCount, 0),
		Now:               now,
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
				SessionID:  session.ID,
				Stamp:      lastError.Stamp(),
				OccurredAt: lastError.OccurredAt,
				Preview:    lastError.Message,
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
		if snapshot := gitBySession[session.ID]; snapshot != nil {
			input.Git = append(input.Git, statussummary.RebuildGit{
				Repository: snapshotRepositoryKey(snapshot, session.ID),
				Summary:    gitSummaryFromSnapshot(snapshot),
			})
		}
	}
	return input
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
		Additions:    nonNegative(snapshot.Metadata, "branch_additions"),
		Deletions:    nonNegative(snapshot.Metadata, "branch_deletions"),
		ChangedFiles: changedFilesFromSnapshot(snapshot),
		Ahead:        maxInt(snapshot.Ahead, 0),
		Behind:       maxInt(snapshot.Behind, 0),
	}
}

func changedFilesFromSnapshot(snapshot *models.GitSnapshot) int {
	if snapshot == nil {
		return 0
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
