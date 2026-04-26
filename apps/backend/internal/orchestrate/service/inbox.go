package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// GetInboxItems returns a computed view of all items needing user attention:
// pending approvals, budget alerts, and agent errors.
func (s *Service) GetInboxItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
	var items []*models.InboxItem

	approvalItems, err := s.inboxApprovalItems(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("approval items: %w", err)
	}
	items = append(items, approvalItems...)

	alertItems, err := s.inboxBudgetAlertItems(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("budget alert items: %w", err)
	}
	items = append(items, alertItems...)

	errorItems, err := s.inboxAgentErrorItems(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agent error items: %w", err)
	}
	items = append(items, errorItems...)

	if items == nil {
		items = []*models.InboxItem{}
	}
	sortInboxItemsByTime(items)
	return items, nil
}

// GetInboxCount returns the total count of unresolved inbox items.
func (s *Service) GetInboxCount(ctx context.Context, wsID string) (int, error) {
	pending, err := s.repo.CountPendingApprovals(ctx, wsID)
	if err != nil {
		return 0, err
	}
	alerts, err := s.repo.ListActivityEntriesByAction(ctx, wsID, "budget.alert", 50)
	if err != nil {
		return 0, err
	}
	errors, err := s.repo.ListActivityEntriesByAction(ctx, wsID, "agent.error", 50)
	if err != nil {
		return 0, err
	}
	return pending + len(alerts) + len(errors), nil
}

func (s *Service) inboxApprovalItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
	approvals, err := s.repo.ListPendingApprovals(ctx, wsID)
	if err != nil {
		return nil, err
	}
	items := make([]*models.InboxItem, 0, len(approvals))
	for _, a := range approvals {
		item := &models.InboxItem{
			ID:         a.ID,
			Type:       "approval",
			Title:      approvalTitle(a),
			Status:     a.Status,
			EntityID:   a.ID,
			EntityType: "approval",
			CreatedAt:  a.CreatedAt,
		}
		if a.Payload != "" {
			_ = json.Unmarshal([]byte(a.Payload), &item.Payload)
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) inboxBudgetAlertItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
	entries, err := s.repo.ListActivityEntriesByAction(ctx, wsID, "budget.alert", 20)
	if err != nil {
		return nil, err
	}
	items := make([]*models.InboxItem, 0, len(entries))
	for _, e := range entries {
		item := &models.InboxItem{
			ID:          e.ID,
			Type:        "budget_alert",
			Title:       "Budget alert",
			Description: e.Details,
			Status:      "active",
			EntityID:    e.TargetID,
			EntityType:  e.TargetType,
			CreatedAt:   e.CreatedAt,
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Service) inboxAgentErrorItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
	entries, err := s.repo.ListActivityEntriesByAction(ctx, wsID, "agent.error", 20)
	if err != nil {
		return nil, err
	}
	items := make([]*models.InboxItem, 0, len(entries))
	for _, e := range entries {
		item := &models.InboxItem{
			ID:          e.ID,
			Type:        "agent_error",
			Title:       "Agent error",
			Description: e.Details,
			Status:      "active",
			EntityID:    e.TargetID,
			EntityType:  e.TargetType,
			CreatedAt:   e.CreatedAt,
		}
		items = append(items, item)
	}
	return items, nil
}

func approvalTitle(a *models.Approval) string {
	switch a.Type {
	case models.ApprovalTypeHireAgent:
		return "Hire agent request"
	case models.ApprovalTypeBudgetIncrease:
		return "Budget increase request"
	case models.ApprovalTypeTaskReview:
		return "Task review request"
	case models.ApprovalTypeSkillCreation:
		return "Skill creation request"
	case models.ApprovalTypeBoardApproval:
		return "Board approval request"
	default:
		return "Approval request"
	}
}

// sortInboxItemsByTime sorts items by created_at descending (newest first).
func sortInboxItemsByTime(items []*models.InboxItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].CreatedAt.After(items[j-1].CreatedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
