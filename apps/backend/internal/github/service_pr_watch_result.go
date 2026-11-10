package github

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func (s *Service) applyPRWatchStatusResult(
	ctx context.Context, watch *PRWatch, status *PRStatus, checkedAt time.Time, publishFeedback bool,
) (bool, bool, error) {
	if watch == nil || status == nil || status.PR == nil || watch.PRNumber == 0 {
		return false, false, nil
	}
	unlock := s.lockPRWatchLifecycle(watch.ID)
	defer unlock()
	effectiveTaskID := s.resolveEffectiveAssociationTaskIDForSession(
		ctx, watch.SessionID, watch.TaskID, watch.RepositoryID,
	)
	if err := s.validateTaskRepositoryID(ctx, effectiveTaskID, watch.RepositoryID); err != nil {
		return false, false, err
	}
	preparedStatus := *status
	if preparedStatus.RequiredReviews == nil {
		fetchTarget := &TaskPR{
			WorkspaceID: watch.WorkspaceID,
			Owner:       status.PR.RepoOwner,
			Repo:        status.PR.RepoName,
		}
		preparedStatus.RequiredReviews = s.fetchRequiredReviewsForTaskPR(ctx, fetchTarget, status.PR.BaseBranch)
	}
	terminal := status.PR.State == prStateMerged || status.PR.State == prStateClosed
	holdTerminal := false
	if terminal {
		var err error
		holdTerminal, err = s.ShouldHoldTerminalPRWatch(
			ctx, effectiveTaskID, watch.RepositoryID, watch.PRNumber, status.PR.State,
		)
		if err != nil {
			return false, false, err
		}
	}
	application := &prWatchStatusResultApplication{}
	_, applyErr := s.store.ApplyPRWatchResult(ctx, watch.ID, func(tx *sqlx.Tx, current *PRWatch) error {
		return s.applyPRWatchStatusResultTx(
			ctx, tx, current, watch, effectiveTaskID, status, &preparedStatus,
			checkedAt, terminal, holdTerminal, publishFeedback, application,
		)
	})
	if applyErr != nil {
		return false, false, applyErr
	}
	if !application.applied {
		return false, false, nil
	}
	s.publishPRWatchResultEvents(ctx, application.events)
	if application.taskPR != nil {
		s.propagateTaskRepositoryBaseBranch(ctx, application.taskPR, application.baseBranchChanged)
		s.reconcileComparisonTargetFromSync(ctx, effectiveTaskID, status.PR)
	}
	return true, application.changed, nil
}

type prWatchStatusResultApplication struct {
	applied           bool
	changed           bool
	baseBranchChanged bool
	taskPR            *TaskPR
	events            []*bus.Event
}

func (s *Service) applyPRWatchStatusResultTx(
	ctx context.Context,
	tx *sqlx.Tx,
	current, expected *PRWatch,
	effectiveTaskID string,
	status, preparedStatus *PRStatus,
	checkedAt time.Time,
	terminal, holdTerminal, publishFeedback bool,
	application *prWatchStatusResultApplication,
) error {
	if current.PRNumber != expected.PRNumber || current.TaskID != expected.TaskID {
		return nil
	}
	retiredBranch, err := s.checkEffectiveRetiredBranchTx(ctx, tx, current, effectiveTaskID)
	if err != nil || retiredBranch {
		return err
	}
	feedbackChanged := prWatchFeedbackUpdatedSinceWatch(current, status)
	if err := updatePRWatchTimestampsTx(
		ctx, tx, current.ID, checkedAt, prWatchFeedbackWatermark(current, status),
		status.ChecksState, status.ReviewState,
	); err != nil {
		return err
	}
	if err := s.reconcilePRWatchResultTaskOwnershipTx(ctx, tx, current, effectiveTaskID, current.PRNumber); err != nil {
		return err
	}
	row, created, err := s.ensureTaskPRForWatchResultTx(
		ctx, tx, current, effectiveTaskID, status.PR, preparedStatus,
	)
	if err != nil {
		return fmt.Errorf("associate PR from watch result: %w", err)
	}
	if err := s.syncTaskPRFromWatchResultTx(
		ctx, tx, effectiveTaskID, row, preparedStatus, created, application,
	); err != nil {
		return err
	}
	if terminal && !holdTerminal {
		if err := resetTerminalPRWatchTx(ctx, tx, current); err != nil {
			return err
		}
	}
	if shouldPublishPRWatchFeedback(publishFeedback, terminal, application.changed, feedbackChanged) {
		application.events = append(application.events, newPRWatchFeedbackEvent(current, effectiveTaskID, status))
	}
	application.applied = true
	return nil
}

func shouldPublishPRWatchFeedback(publishFeedback, terminal, taskPRChanged, feedbackChanged bool) bool {
	return publishFeedback && (terminal || taskPRChanged || feedbackChanged)
}

func (s *Service) ensureTaskPRForWatchResultTx(
	ctx context.Context, tx *sqlx.Tx, watch *PRWatch, taskID string, pr *PR, status *PRStatus,
) (*TaskPR, bool, error) {
	row, err := getTaskPRByRepoAndNumberTx(ctx, tx, taskID, watch.RepositoryID, pr.Number)
	if err != nil || row != nil {
		return row, false, err
	}
	row, err = taskPRFromWatch(watch, taskID, pr)
	if err != nil {
		return nil, false, err
	}
	if err := s.store.ReplaceTaskPRTx(ctx, tx, row, status); err != nil {
		return nil, false, err
	}
	row, err = getTaskPRByRepoAndNumberTx(ctx, tx, taskID, watch.RepositoryID, pr.Number)
	return row, true, err
}

func (s *Service) syncTaskPRFromWatchResultTx(
	ctx context.Context, tx *sqlx.Tx, taskID string, row *TaskPR, status *PRStatus,
	created bool, application *prWatchStatusResultApplication,
) error {
	if row == nil || row.DetachedAt != nil {
		return nil
	}
	prepared := s.prepareTaskPRSyncFromRowWithRequiredReviews(ctx, taskID, row, status, false)
	application.changed = prepared.changed
	application.baseBranchChanged = prepared.baseBranchChanged
	var err error
	application.taskPR, err = s.persistTaskPRSyncTx(ctx, tx, taskID, status, prepared)
	if err == nil && application.taskPR != nil && (prepared.changed || created) {
		application.events = append(application.events, newTaskPRUpdatedEvent(application.taskPR))
	}
	return err
}

func resetTerminalPRWatchTx(ctx context.Context, tx *sqlx.Tx, watch *PRWatch) error {
	var searchingID string
	err := tx.GetContext(ctx, &searchingID, tx.Rebind(`
		SELECT id FROM github_pr_watches
		WHERE task_id = ? AND repository_id = ? AND branch = ? AND pr_number = 0
			AND retired_at IS NULL AND id <> ?
		LIMIT 1`), watch.TaskID, watch.RepositoryID, watch.Branch, watch.ID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("find searching PR watch: %w", err)
	}
	if err == nil {
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`DELETE FROM github_pr_watches WHERE id = ? AND retired_at IS NULL`), watch.ID); err != nil {
			return fmt.Errorf("remove redundant terminal PR watch: %w", err)
		}
		return nil
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_pr_watches SET pr_number = 0, last_check_status = '', last_review_state = '', updated_at = ?
		WHERE id = ? AND retired_at IS NULL`), time.Now().UTC(), watch.ID); err != nil {
		return fmt.Errorf("reset terminal PR watch: %w", err)
	}
	return nil
}

func (s *Service) applyPRWatchDiscoveryResult(
	ctx context.Context, watch *PRWatch, pr *PR, checkedAt time.Time,
) (bool, bool, error) {
	if watch == nil {
		return false, false, nil
	}
	if pr == nil {
		return s.applyEmptyPRWatchDiscoveryResult(ctx, watch, checkedAt)
	}
	if pr.Number <= 0 {
		return false, false, nil
	}
	unlock := s.lockPRWatchLifecycle(watch.ID)
	defer unlock()
	effectiveTaskID := s.resolveEffectiveAssociationTaskIDForSession(
		ctx, watch.SessionID, watch.TaskID, watch.RepositoryID,
	)
	if err := s.validateTaskRepositoryID(ctx, effectiveTaskID, watch.RepositoryID); err != nil {
		return false, false, err
	}
	application := &prWatchDiscoveryResultApplication{}
	accepted, err := s.store.ApplyPRWatchResult(ctx, watch.ID, func(tx *sqlx.Tx, current *PRWatch) error {
		return s.applyPRWatchDiscoveryResultTx(
			ctx, tx, current, watch, effectiveTaskID, pr, checkedAt, application,
		)
	})
	if err != nil {
		return false, false, err
	}
	if application.applied && accepted {
		s.publishPRWatchResultEvents(ctx, application.events)
		s.reconcileComparisonTarget(ctx, effectiveTaskID, pr)
	}
	if application.promoted {
		watch.Owner, watch.Repo, watch.PRNumber = application.owner, application.repo, pr.Number
	}
	return application.applied && accepted, application.promoted && accepted, nil
}

type prWatchDiscoveryResultApplication struct {
	applied  bool
	promoted bool
	owner    string
	repo     string
	events   []*bus.Event
}

func (s *Service) applyEmptyPRWatchDiscoveryResult(
	ctx context.Context, watch *PRWatch, checkedAt time.Time,
) (bool, bool, error) {
	unlock := s.lockPRWatchLifecycle(watch.ID)
	defer unlock()
	effectiveTaskID := s.resolveEffectiveAssociationTaskIDForSession(
		ctx, watch.SessionID, watch.TaskID, watch.RepositoryID,
	)
	applied := false
	accepted, err := s.store.ApplyPRWatchResult(ctx, watch.ID, func(tx *sqlx.Tx, current *PRWatch) error {
		if current.PRNumber != watch.PRNumber || current.Branch != watch.Branch {
			return nil
		}
		retiredBranch, err := s.checkEffectiveRetiredBranchTx(ctx, tx, current, effectiveTaskID)
		if err != nil || retiredBranch {
			return err
		}
		if err := updatePRWatchTimestampsTx(ctx, tx, current.ID, checkedAt, current.LastCommentAt, "", ""); err != nil {
			return err
		}
		applied = true
		return nil
	})
	return applied && accepted, false, err
}

func (s *Service) applyPRWatchDiscoveryResultTx(
	ctx context.Context, tx *sqlx.Tx, current, expected *PRWatch, taskID string, pr *PR,
	checkedAt time.Time, application *prWatchDiscoveryResultApplication,
) error {
	if current.PRNumber != expected.PRNumber || current.Branch != expected.Branch {
		return nil
	}
	retiredBranch, err := s.checkEffectiveRetiredBranchTx(ctx, tx, current, taskID)
	if err != nil || retiredBranch {
		return err
	}
	updated, accepted, err := s.preparePRWatchDiscoveryTargetTx(ctx, tx, current, pr, checkedAt, application)
	if err != nil || !accepted {
		return err
	}
	if err := s.reconcilePRWatchResultTaskOwnershipTx(ctx, tx, current, taskID, pr.Number); err != nil {
		return err
	}
	row, created, err := s.ensureTaskPRForWatchResultTx(
		ctx, tx, &updated, taskID, pr, &PRStatus{PR: pr},
	)
	if err != nil {
		return fmt.Errorf("associate discovered PR with task: %w", err)
	}
	if created {
		application.events = append(application.events, newTaskPRUpdatedEvent(row))
	}
	application.applied = true
	return nil
}

func (s *Service) preparePRWatchDiscoveryTargetTx(
	ctx context.Context, tx *sqlx.Tx, current *PRWatch, pr *PR, checkedAt time.Time,
	application *prWatchDiscoveryResultApplication,
) (PRWatch, bool, error) {
	searching := current.PRNumber == 0
	if !searching && current.PRNumber != pr.Number {
		return PRWatch{}, false, nil
	}
	updated := *current
	if searching {
		if err := updatePRWatchTimestampsTx(ctx, tx, current.ID, checkedAt, nil, "", ""); err != nil {
			return PRWatch{}, false, err
		}
		updated.PRNumber = pr.Number
	}
	if strings.TrimSpace(pr.RepoOwner) != "" {
		updated.Owner = pr.RepoOwner
	}
	if strings.TrimSpace(pr.RepoName) != "" {
		updated.Repo = pr.RepoName
	}
	if err := lockPRWatchIdentities(ctx, tx, s.store.db.DriverName(), &updated); err != nil {
		return PRWatch{}, false, err
	}
	if searching {
		return s.promoteSearchingPRWatchTx(ctx, tx, current, updated, application)
	}
	if err := updatePRWatchRepositoryIdentityTx(ctx, tx, current, updated); err != nil {
		return PRWatch{}, false, err
	}
	return updated, true, nil
}

func (s *Service) promoteSearchingPRWatchTx(
	ctx context.Context, tx *sqlx.Tx, current *PRWatch, updated PRWatch,
	application *prWatchDiscoveryResultApplication,
) (PRWatch, bool, error) {
	var retiredTarget bool
	err := tx.QueryRowContext(ctx, tx.Rebind(`
		SELECT retired_at IS NOT NULL FROM github_pr_watches
		WHERE task_id = ? AND repository_id = ? AND pr_number = ? AND id <> ?`),
		current.TaskID, current.RepositoryID, updated.PRNumber, current.ID,
	).Scan(&retiredTarget)
	if err != nil && err != sql.ErrNoRows {
		return PRWatch{}, false, err
	}
	if err == nil && retiredTarget {
		return PRWatch{}, false, nil
	}
	if err == nil {
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`DELETE FROM github_pr_watches WHERE id = ? AND retired_at IS NULL`), current.ID); err != nil {
			return PRWatch{}, false, err
		}
		return updated, true, nil
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_pr_watches SET owner = ?, repo = ?, pr_number = ?, updated_at = ?
		WHERE id = ? AND retired_at IS NULL`),
		updated.Owner, updated.Repo, updated.PRNumber, time.Now().UTC(), current.ID); err != nil {
		return PRWatch{}, false, fmt.Errorf("promote discovered PR watch: %w", err)
	}
	application.promoted = true
	application.owner = updated.Owner
	application.repo = updated.Repo
	return updated, true, nil
}

func updatePRWatchRepositoryIdentityTx(ctx context.Context, tx *sqlx.Tx, current *PRWatch, updated PRWatch) error {
	if updated.Owner == current.Owner && updated.Repo == current.Repo {
		return nil
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE github_pr_watches SET owner = ?, repo = ?, updated_at = ?
		WHERE id = ? AND retired_at IS NULL`), updated.Owner, updated.Repo, time.Now().UTC(), current.ID); err != nil {
		return fmt.Errorf("rebind PR watch repository: %w", err)
	}
	return nil
}

func (s *Service) reconcilePRWatchResultTaskOwnershipTx(
	ctx context.Context, tx *sqlx.Tx, current *PRWatch, taskID string, prNumber int,
) error {
	if taskID == current.TaskID {
		return nil
	}
	reconciler, ok := any(s.store).(interface {
		ReconcileTaskPROwnershipTx(context.Context, *sqlx.Tx, string, string, string, int) (bool, error)
	})
	if !ok {
		return nil
	}
	_, err := reconciler.ReconcileTaskPROwnershipTx(
		ctx, tx, current.TaskID, taskID, current.RepositoryID, prNumber,
	)
	return err
}

func (s *Service) checkEffectiveRetiredBranchTx(
	ctx context.Context, tx *sqlx.Tx, current *PRWatch, effectiveTaskID string,
) (bool, error) {
	if effectiveTaskID == current.TaskID {
		return false, nil
	}
	target := *current
	target.TaskID = effectiveTaskID
	if err := lockPRWatchIdentities(ctx, tx, s.store.db.DriverName(), &target); err != nil {
		return false, err
	}
	return hasRetiredPRWatchByTaskRepoBranchTx(
		ctx, tx, effectiveTaskID, current.RepositoryID, current.Branch, current.ID,
	)
}

func taskPRFromWatch(watch *PRWatch, taskID string, pr *PR) (*TaskPR, error) {
	if watch == nil || pr == nil || pr.Number <= 0 {
		return nil, fmt.Errorf("watch result has incomplete PR association data")
	}
	owner, repo := pr.RepoOwner, pr.RepoName
	if strings.TrimSpace(owner) == "" {
		owner = watch.Owner
	}
	if strings.TrimSpace(repo) == "" {
		repo = watch.Repo
	}
	return &TaskPR{
		ID:                uuid.NewString(),
		WorkspaceID:       watch.WorkspaceID,
		TaskID:            taskID,
		RepositoryID:      watch.RepositoryID,
		Owner:             owner,
		Repo:              repo,
		PRNumber:          pr.Number,
		PRURL:             pr.HTMLURL,
		PRTitle:           pr.Title,
		HeadBranch:        pr.HeadBranch,
		BaseBranch:        pr.BaseBranch,
		HeadSHA:           pr.HeadSHA,
		AuthorLogin:       pr.AuthorLogin,
		State:             pr.State,
		Additions:         pr.Additions,
		Deletions:         pr.Deletions,
		CreatedAt:         pr.CreatedAt,
		MergedAt:          pr.MergedAt,
		ClosedAt:          pr.ClosedAt,
		Source:            TaskPRSourceWatch,
		HasMergeConflicts: observedTaskPRMergeConflict(nil, pr, pr.MergeableState),
	}, nil
}

func newPRWatchFeedbackEvent(watch *PRWatch, taskID string, status *PRStatus) *bus.Event {
	if watch == nil || status == nil {
		return nil
	}
	eventData := &PRFeedbackEvent{
		SessionID:      watch.SessionID,
		TaskID:         taskID,
		PRNumber:       watch.PRNumber,
		Owner:          watch.Owner,
		Repo:           watch.Repo,
		NewCheckStatus: status.ChecksState,
		NewReviewState: status.ReviewState,
	}
	return bus.NewEvent(events.GitHubPRFeedback, "github_poller", eventData)
}

func newTaskPRUpdatedEvent(row *TaskPR) *bus.Event {
	if row == nil {
		return nil
	}
	return bus.NewEvent(events.GitHubTaskPRUpdated, "github", cloneTaskPRSnapshot(row))
}

func (s *Service) publishPRWatchResultEvents(ctx context.Context, staged []*bus.Event) {
	if s.eventBus == nil {
		return
	}
	for _, event := range staged {
		if event == nil {
			continue
		}
		if err := s.eventBus.Publish(ctx, event.Type, event); err != nil && s.logger != nil {
			s.logger.Debug("failed to publish committed PR watch result event",
				zap.String("event_type", event.Type), zap.Error(err))
		}
	}
}

func cloneTaskPRSnapshot(row *TaskPR) *TaskPR {
	if row == nil {
		return nil
	}
	snapshot := *row
	snapshot.HasMergeConflicts = clonePointer(row.HasMergeConflicts)
	snapshot.MergeQueuePosition = clonePointer(row.MergeQueuePosition)
	snapshot.MergeQueueEstimatedTimeToMergeSeconds = clonePointer(row.MergeQueueEstimatedTimeToMergeSeconds)
	snapshot.MergeQueueLastRemovedAt = clonePointer(row.MergeQueueLastRemovedAt)
	snapshot.RequiredReviews = clonePointer(row.RequiredReviews)
	snapshot.MergedAt = clonePointer(row.MergedAt)
	snapshot.ClosedAt = clonePointer(row.ClosedAt)
	snapshot.LastSyncedAt = clonePointer(row.LastSyncedAt)
	snapshot.DetachedAt = clonePointer(row.DetachedAt)
	snapshot.IsDraft = clonePointer(row.IsDraft)
	snapshot.ChangedFiles = clonePointer(row.ChangedFiles)
	snapshot.MergedByLogin = clonePointer(row.MergedByLogin)
	snapshot.ClosedByLogin = clonePointer(row.ClosedByLogin)
	snapshot.AutoMergeObservedAt = clonePointer(row.AutoMergeObservedAt)
	if row.WorkflowAttention != nil {
		attention := *row.WorkflowAttention
		attention.Runs = append([]WorkflowAttentionRun(nil), row.WorkflowAttention.Runs...)
		snapshot.WorkflowAttention = &attention
	}
	return &snapshot
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
