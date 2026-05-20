package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
)

const (
	treeHoldReleasePolicyManual = `{"strategy":"manual"}`
	treeHoldActorSystem         = "system:office"
	treeHoldSkipAlreadyCanceled = "already_cancelled"
)

func (s *Service) PreviewTaskTree(ctx context.Context, rootTaskID string) (*models.TreePreview, error) {
	tasks, err := s.repo.PreviewSubtree(ctx, rootTaskID)
	if err != nil {
		return nil, fmt.Errorf("preview subtree: %w", err)
	}
	taskIDs := make([]string, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
	}
	count, err := s.repo.CountActiveRunsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, fmt.Errorf("count active runs: %w", err)
	}
	hold, err := s.getActiveRootHold(ctx, rootTaskID)
	if err != nil {
		return nil, err
	}
	return &models.TreePreview{
		TaskCount:      len(tasks),
		Tasks:          tasks,
		ActiveRunCount: count,
		ActiveHold:     hold,
	}, nil
}

func (s *Service) PauseTaskTree(ctx context.Context, rootTaskID string) (*models.TreeHold, error) {
	hold, members, err := s.buildTreeHold(ctx, rootTaskID, models.TreeHoldModePause)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateTreeHold(ctx, hold); err != nil {
		return nil, fmt.Errorf("create pause hold: %w", err)
	}
	if err := s.repo.CreateTreeHoldMembers(ctx, hold.ID, members); err != nil {
		return nil, fmt.Errorf("create pause members: %w", err)
	}
	s.cancelTaskExecutions(ctx, members, "tree_paused")
	if _, err := s.repo.CancelRunsForTasks(ctx, memberTaskIDs(members), "tree_paused"); err != nil {
		s.logger.Warn("cancel runs for paused tree failed", zap.Error(err))
	}
	s.publishTreeHoldEvent(ctx, events.OfficeTaskTreeHoldCreated, hold)
	return hold, nil
}

func (s *Service) ResumeTaskTree(ctx context.Context, rootTaskID, releasedBy string) (*models.TreeHold, error) {
	hold, err := s.repo.GetActiveHold(ctx, rootTaskID, models.TreeHoldModePause)
	if err != nil {
		return nil, fmt.Errorf("find pause hold: %w", err)
	}
	if hold == nil {
		return nil, fmt.Errorf("active pause hold not found")
	}
	if releasedBy == "" {
		releasedBy = treeHoldActorSystem
	}
	if err := s.repo.ReleaseHold(ctx, hold.ID, releasedBy, "resumed"); err != nil {
		return nil, fmt.Errorf("release pause hold: %w", err)
	}
	s.publishTreeHoldEvent(ctx, events.OfficeTaskTreeHoldReleased, hold)
	return hold, nil
}

func (s *Service) CancelTaskTree(ctx context.Context, rootTaskID, cancelledBy string) (*models.TreeHold, error) {
	hold, members, err := s.buildTreeHold(ctx, rootTaskID, models.TreeHoldModeCancel)
	if err != nil {
		return nil, err
	}
	s.markAlreadyCancelled(members)
	s.cancelTaskExecutions(ctx, members, "tree_cancelled")
	if err := s.repo.CreateTreeHold(ctx, hold); err != nil {
		return nil, fmt.Errorf("create cancel hold: %w", err)
	}
	if err := s.repo.CreateTreeHoldMembers(ctx, hold.ID, members); err != nil {
		return nil, fmt.Errorf("create cancel members: %w", err)
	}
	if err := s.repo.BulkUpdateTaskState(ctx, restorableTaskIDs(members), "CANCELLED"); err != nil {
		return nil, fmt.Errorf("cancel tasks: %w", err)
	}
	if err := s.repo.BulkReleaseTaskCheckout(ctx, memberTaskIDs(members)); err != nil {
		return nil, fmt.Errorf("release task checkouts: %w", err)
	}
	if _, err := s.repo.CancelRunsForTasks(ctx, memberTaskIDs(members), "tree_cancelled"); err != nil {
		s.logger.Warn("cancel runs for cancelled tree failed", zap.Error(err))
	}
	_ = cancelledBy
	s.publishTreeHoldEvent(ctx, events.OfficeTaskTreeHoldCreated, hold)
	return hold, nil
}

func (s *Service) RestoreTaskTree(ctx context.Context, rootTaskID, restoredBy string) (*models.TreeHold, error) {
	hold, err := s.repo.GetActiveHold(ctx, rootTaskID, models.TreeHoldModeCancel)
	if err != nil {
		return nil, fmt.Errorf("find cancel hold: %w", err)
	}
	if hold == nil {
		return nil, fmt.Errorf("active cancel hold not found")
	}
	members, err := s.repo.ListHoldMembers(ctx, hold.ID)
	if err != nil {
		return nil, fmt.Errorf("list cancel members: %w", err)
	}
	for _, member := range members {
		if member.SkipReason == treeHoldSkipAlreadyCanceled {
			continue
		}
		if err := s.repo.UpdateTaskState(ctx, member.TaskID, member.TaskStatus); err != nil {
			return nil, fmt.Errorf("restore task %s: %w", member.TaskID, err)
		}
	}
	if restoredBy == "" {
		restoredBy = treeHoldActorSystem
	}
	if err := s.repo.ReleaseHold(ctx, hold.ID, restoredBy, "restored"); err != nil {
		return nil, fmt.Errorf("release cancel hold: %w", err)
	}
	s.publishTreeHoldEvent(ctx, events.OfficeTaskTreeHoldReleased, hold)
	return hold, nil
}

func (s *Service) GetSubtreeCostSummary(
	ctx context.Context,
	rootTaskID string,
) (*models.SubtreeCostSummary, error) {
	return s.repo.GetSubtreeCostSummary(ctx, rootTaskID)
}

func (s *Service) buildTreeHold(
	ctx context.Context,
	rootTaskID, mode string,
) (*models.TreeHold, []models.TreeHoldMember, error) {
	subtree, err := s.repo.FindSubtree(ctx, rootTaskID)
	if err != nil {
		return nil, nil, fmt.Errorf("find subtree: %w", err)
	}
	if len(subtree) == 0 {
		return nil, nil, fmt.Errorf("task not found: %s", rootTaskID)
	}
	rootFields, err := s.repo.GetTaskExecutionFields(ctx, rootTaskID)
	if err != nil {
		return nil, nil, err
	}
	hold := &models.TreeHold{
		ID:            uuid.New().String(),
		WorkspaceID:   rootFields.WorkspaceID,
		RootTaskID:    rootTaskID,
		Mode:          mode,
		ReleasePolicy: treeHoldReleasePolicyManual,
	}
	members := make([]models.TreeHoldMember, len(subtree))
	for i, item := range subtree {
		fields, err := s.repo.GetTaskExecutionFields(ctx, item.TaskID)
		if err != nil {
			return nil, nil, err
		}
		members[i] = models.TreeHoldMember{
			HoldID:     hold.ID,
			TaskID:     item.TaskID,
			Depth:      item.Depth,
			TaskStatus: fields.State,
		}
	}
	return hold, members, nil
}

func (s *Service) getActiveRootHold(ctx context.Context, rootTaskID string) (*models.TreeHold, error) {
	if hold, err := s.repo.GetActiveHold(ctx, rootTaskID, models.TreeHoldModeCancel); err != nil || hold != nil {
		return hold, err
	}
	return s.repo.GetActiveHold(ctx, rootTaskID, models.TreeHoldModePause)
}

func (s *Service) markAlreadyCancelled(members []models.TreeHoldMember) {
	for i := range members {
		if members[i].TaskStatus == "CANCELLED" {
			members[i].SkipReason = treeHoldSkipAlreadyCanceled
		}
	}
}

func (s *Service) cancelTaskExecutions(ctx context.Context, members []models.TreeHoldMember, reason string) {
	if s.taskCanceller == nil {
		return
	}
	for _, member := range members {
		if err := s.taskCanceller.CancelTaskExecution(ctx, member.TaskID, reason, true); err != nil {
			s.logger.Warn("tree control task cancellation failed",
				zap.String("task_id", member.TaskID),
				zap.String("reason", reason),
				zap.Error(err))
		}
	}
}

func (s *Service) publishTreeHoldEvent(ctx context.Context, subject string, hold *models.TreeHold) {
	if s.eb == nil || hold == nil {
		return
	}
	event := bus.NewEvent(subject, "office", hold)
	if err := s.eb.Publish(ctx, subject, event); err != nil {
		s.logger.Warn("publish tree hold event failed",
			zap.String("subject", subject),
			zap.Error(err))
	}
}

func memberTaskIDs(members []models.TreeHoldMember) []string {
	ids := make([]string, len(members))
	for i, member := range members {
		ids[i] = member.TaskID
	}
	return ids
}

func restorableTaskIDs(members []models.TreeHoldMember) []string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		if member.SkipReason != treeHoldSkipAlreadyCanceled {
			ids = append(ids, member.TaskID)
		}
	}
	return ids
}
