package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// GetInboxItems returns a computed view of all items needing user attention.
func (s *DashboardService) GetInboxItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
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

	runFailedItems, err := s.inboxAgentRunFailedItems(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agent run failed items: %w", err)
	}
	items = append(items, runFailedItems...)

	pausedItems, err := s.inboxAgentPausedItems(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agent paused items: %w", err)
	}
	items = append(items, pausedItems...)

	reviewItems, err := s.inboxTaskReviewRequests(ctx, wsID, "")
	if err != nil {
		return nil, fmt.Errorf("review request items: %w", err)
	}
	items = append(items, reviewItems...)

	providerItems, err := s.inboxProviderDegradedItems(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("provider degraded items: %w", err)
	}
	items = append(items, providerItems...)

	permItems := s.inboxPermissionItems()
	items = append(items, permItems...)

	if items == nil {
		items = []*models.InboxItem{}
	}
	sortInboxItemsByTime(items)
	return items, nil
}

// GetAgentInboxItems returns the inbox items addressed to a specific
// agent. Currently returns only that agent's pending review requests
// since other inbox sources (approvals, budget alerts) are workspace-
// scoped rather than agent-scoped.
func (s *DashboardService) GetAgentInboxItems(
	ctx context.Context, wsID, agentInstanceID string,
) ([]*models.InboxItem, error) {
	if agentInstanceID == "" {
		return s.GetInboxItems(ctx, wsID)
	}
	items, err := s.inboxTaskReviewRequests(ctx, wsID, agentInstanceID)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []*models.InboxItem{}
	}
	sortInboxItemsByTime(items)
	return items, nil
}

// GetInboxCount returns the total count of unresolved inbox items.
func (s *DashboardService) GetInboxCount(ctx context.Context, wsID string) (int, error) {
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
	permCount := 0
	if s.permissions != nil {
		permCount = len(s.permissions.ListPendingPermissions())
	}
	reviewCount := 0
	if items, rerr := s.inboxTaskReviewRequests(ctx, wsID, ""); rerr == nil {
		reviewCount = len(items)
	}
	return pending + len(alerts) + len(errors) + permCount + reviewCount, nil
}

func (s *DashboardService) inboxApprovalItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
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

func (s *DashboardService) inboxBudgetAlertItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
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

func (s *DashboardService) inboxAgentErrorItems(ctx context.Context, wsID string) ([]*models.InboxItem, error) {
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

// inboxAgentRunFailedItem inbox type — one entry per failed run
// while the agent is below the auto-pause threshold.
const inboxAgentRunFailedType = "agent_run_failed"

// inboxAgentPausedAfterFailuresType — one entry per auto-paused
// agent. Replaces the per-task entries when the threshold trips.
const inboxAgentPausedAfterFailuresType = "agent_paused_after_failures"

// dashboardUserID is the user_id stored in inbox_dismissals when a
// human dismisses a row from the UI. Single-user kandev — multi-user
// support can swap to the authenticated user's id later.
const dashboardUserID = "default"

func (s *DashboardService) inboxAgentRunFailedItems(
	ctx context.Context, wsID string,
) ([]*models.InboxItem, error) {
	if s.failureInbox == nil {
		return nil, nil
	}
	rows, err := s.failureInbox.ListFailedRunInboxRows(ctx, wsID, dashboardUserID)
	if err != nil {
		return nil, err
	}
	items := make([]*models.InboxItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, &models.InboxItem{
			ID:          r.ItemID,
			Type:        inboxAgentRunFailedType,
			Title:       fmt.Sprintf("%s failed on a task", agentNameOrFallback(r.AgentName)),
			Description: truncateInboxDescription(r.ErrorMessage),
			Status:      "active",
			EntityID:    r.TaskID,
			EntityType:  "task",
			Payload: map[string]interface{}{
				"agent_profile_id": r.AgentProfileID,
				"agent_name":       r.AgentName,
				"run_id":           r.ItemID,
				"task_id":          r.TaskID,
				"error_message":    r.ErrorMessage,
			},
			CreatedAt: r.FailedAt,
		})
	}
	return items, nil
}

func (s *DashboardService) inboxAgentPausedItems(
	ctx context.Context, wsID string,
) ([]*models.InboxItem, error) {
	if s.failureInbox == nil {
		return nil, nil
	}
	rows, err := s.failureInbox.ListPausedAgentInboxRows(ctx, wsID, dashboardUserID)
	if err != nil {
		return nil, err
	}
	items := make([]*models.InboxItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, &models.InboxItem{
			ID:    r.ItemID,
			Type:  inboxAgentPausedAfterFailuresType,
			Title: fmt.Sprintf("%s auto-paused", agentNameOrFallback(r.AgentName)),
			Description: fmt.Sprintf("%d consecutive failures. %s",
				r.ConsecutiveFailures, truncateInboxDescription(r.PauseReason)),
			Status:     "active",
			EntityID:   r.AgentProfileID,
			EntityType: "agent_instance",
			Payload: map[string]interface{}{
				"agent_profile_id":     r.AgentProfileID,
				"agent_name":           r.AgentName,
				"consecutive_failures": r.ConsecutiveFailures,
				"pause_reason":         r.PauseReason,
			},
			CreatedAt: r.FailedAt,
		})
	}
	return items, nil
}

func agentNameOrFallback(name string) string {
	if name == "" {
		return "Agent"
	}
	return name
}

func truncateInboxDescription(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// reviewRequestInboxType is the InboxItem.Type for a pending
// reviewer/approver decision request. Mirrored on the frontend.
const reviewRequestInboxType = "task_review_request"

// inboxTaskReviewRequests returns one InboxItem per task awaiting the
// viewer's review/approval decision. When viewerAgentID is empty the
// viewer is treated as the singleton human user and every active
// task with at least one participant lacking a current decision is
// returned. When viewerAgentID is set, only tasks where that specific
// agent is a participant without a current decision are returned.
func (s *DashboardService) inboxTaskReviewRequests(
	ctx context.Context, wsID, viewerAgentID string,
) ([]*models.InboxItem, error) {
	// Inbox surfaces review requests across every task — include system
	// tasks too in case a coordination/routine task ever needs review.
	tasks, err := s.repo.ListTasksByWorkspace(ctx, wsID, true)
	if err != nil {
		return nil, err
	}
	out := make([]*models.InboxItem, 0)
	for _, t := range tasks {
		item := s.buildReviewRequestItem(ctx, t, viewerAgentID)
		if item != nil {
			out = append(out, item)
		}
	}
	return out, nil
}

// buildReviewRequestItem returns an InboxItem for a task when the
// viewer has a pending review/approval decision on it; nil otherwise.
func (s *DashboardService) buildReviewRequestItem(
	ctx context.Context, t *sqlite.TaskRow, viewerAgentID string,
) *models.InboxItem {
	parts, err := s.repo.ListAllTaskParticipants(ctx, t.ID)
	if err != nil || len(parts) == 0 {
		return nil
	}
	roles := s.viewerRoles(parts, viewerAgentID)
	if len(roles) == 0 {
		return nil
	}
	if !s.viewerNeedsDecision(ctx, t.ID, viewerAgentID) {
		return nil
	}
	title := buildReviewRequestTitle(t, roles)
	return &models.InboxItem{
		ID:         "review:" + t.ID + ":" + viewerAgentID,
		Type:       reviewRequestInboxType,
		Title:      title,
		Status:     "pending",
		EntityID:   t.ID,
		EntityType: activityTaskTargetType,
		Payload: map[string]interface{}{
			"task_id":    t.ID,
			"identifier": t.Identifier,
			"task_title": t.Title,
			"roles":      roles,
			"viewer_id":  viewerAgentID,
		},
		CreatedAt: parsedTaskUpdatedAt(t),
	}
}

// viewerRoles returns the participant roles the viewer holds on the
// task. For the singleton human user (viewerAgentID == "") every role
// listed on the task is returned (the human is implicitly all-roles).
// For an agent, only roles where the agent appears in participants.
func (s *DashboardService) viewerRoles(
	parts []sqlite.Participant, viewerAgentID string,
) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, p := range parts {
		if viewerAgentID != "" && p.AgentProfileID != viewerAgentID {
			continue
		}
		if _, dup := seen[p.Role]; dup {
			continue
		}
		seen[p.Role] = struct{}{}
		out = append(out, p.Role)
	}
	return out
}

// viewerNeedsDecision returns true when the viewer has no current
// (non-superseded) decision on the task. For the human user the
// decider_id sentinel is "user".
func (s *DashboardService) viewerNeedsDecision(
	ctx context.Context, taskID, viewerAgentID string,
) bool {
	if s.decisions == nil {
		return true
	}
	decisions, err := s.decisions.ListActiveTaskDecisions(ctx, taskID)
	if err != nil {
		return false
	}
	deciderID := viewerAgentID
	if deciderID == "" {
		deciderID = userSentinel
	}
	for _, d := range decisions {
		if d.DeciderID == deciderID {
			return false
		}
	}
	return true
}

// buildReviewRequestTitle composes the inbox row title.
// Format: "TES-42: present yourself — your review requested".
func buildReviewRequestTitle(t *sqlite.TaskRow, roles []string) string {
	prefix := t.Title
	if t.Identifier != "" {
		prefix = t.Identifier + ": " + t.Title
	}
	suffix := "your review requested"
	for _, r := range roles {
		if r == models.ParticipantRoleApprover {
			suffix = "your approval requested"
			break
		}
	}
	return prefix + " — " + suffix
}

// parsedTaskUpdatedAt best-effort parses the task's updated_at column
// into a time.Time. Falls back to time.Now on parse failure so the
// inbox sort still has a stable ordering key.
func parsedTaskUpdatedAt(t *sqlite.TaskRow) time.Time {
	formats := []string{
		time.RFC3339Nano, time.RFC3339,
		"2006-01-02T15:04:05Z", "2006-01-02 15:04:05",
	}
	for _, f := range formats {
		if v, err := time.Parse(f, t.UpdatedAt); err == nil {
			return v
		}
	}
	return time.Now().UTC()
}

// inboxPermissionItems returns inbox items for pending tool permission requests.
// Returns an empty slice (not nil) when no permission lister is configured or
// when there are no pending requests.
func (s *DashboardService) inboxPermissionItems() []*models.InboxItem {
	if s.permissions == nil {
		return nil
	}
	pending := s.permissions.ListPendingPermissions()
	items := make([]*models.InboxItem, 0, len(pending))
	for _, p := range pending {
		desc := p.Prompt
		if p.Context != "" {
			desc = p.Context + ": " + p.Prompt
		}
		item := &models.InboxItem{
			ID:          p.PendingID,
			Type:        "permission_request",
			Title:       "Tool permission request",
			Description: desc,
			Status:      "pending",
			EntityID:    p.PendingID,
			EntityType:  "clarification",
			Payload: map[string]interface{}{
				"pending_id": p.PendingID,
				"session_id": p.SessionID,
				"task_id":    p.TaskID,
				"prompt":     p.Prompt,
			},
			CreatedAt: p.CreatedAt,
		}
		items = append(items, item)
	}
	return items
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
