package coordinator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
)

// Activity list and summary bounds.
const (
	DefaultActivityLimit = 50
	MaxActivityLimit     = 50
	DefaultSummaryDays   = 30
	MaxSummaryDays       = 90
)

// ListActivityParams are the list arguments. Zero Limit means the default;
// an empty Class means every class; an empty Before means the first page.
type ListActivityParams struct {
	Class  string
	Before string
	Limit  int
}

// ActivityItem is one list row: the stored row plus fields computed at read
// time. It carries user ids only; display names are the client's concern.
type ActivityItem struct {
	ActivityRow
	Undoable             bool    `json:"undoable"`
	TargetTaskIdentifier *string `json:"target_task_identifier"`
	FromStepID           *string `json:"from_step_id"`
}

// ActivityPage is one page of the list.
type ActivityPage struct {
	Rows       []ActivityItem `json:"rows"`
	NextCursor *string        `json:"next_cursor"`
}

// OutcomeCounts is the per-class summary of one coordinator.
type OutcomeCounts struct {
	Proposed          int64 `json:"proposed"`
	Approved          int64 `json:"approved"`
	ApprovedWithEdits int64 `json:"approved_with_edits"`
	Rejected          int64 `json:"rejected"`
	Failed            int64 `json:"failed"`
	Refused           int64 `json:"refused"`
	Undone            int64 `json:"undone"`
}

// ActivitySummary is the windowed activity summary.
type ActivitySummary struct {
	Days          int                      `json:"days"`
	EarliestRowAt *time.Time               `json:"earliest_row_at"`
	Classes       map[Action]OutcomeCounts `json:"classes"`
}

type activityCursorJSON struct {
	T string `json:"t"`
	I string `json:"i"`
}

func encodeActivityCursor(c ActivityCursor) string {
	raw, _ := json.Marshal(activityCursorJSON{T: c.CreatedAt.UTC().Format(time.RFC3339Nano), I: c.ID})
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeActivityCursor(value string) (*ActivityCursor, error) {
	bad := &FieldError{Field: "before", Message: "before is not a valid cursor"}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, bad
	}
	var c activityCursorJSON
	if err := json.Unmarshal(raw, &c); err != nil || c.I == "" {
		return nil, bad
	}
	at, err := time.Parse(time.RFC3339Nano, c.T)
	if err != nil {
		return nil, bad
	}
	return &ActivityCursor{CreatedAt: at.UTC(), ID: c.I}, nil
}

// moveOutcome is the part of a move proposal's outcome undo needs.
type moveOutcome struct {
	FromStepID string `json:"from_step_id"`
	ToStepID   string `json:"to_step_id"`
	Noop       bool   `json:"noop"`
}

// parseMoveOutcome reports ok only when both step ids are present.
func parseMoveOutcome(raw *string) (moveOutcome, bool) {
	var o moveOutcome
	if raw == nil || json.Unmarshal([]byte(*raw), &o) != nil {
		return moveOutcome{}, false
	}
	return o, o.FromStepID != "" && o.ToStepID != ""
}

// undoableClass reports whether the class can be reversed at all.
func undoableClass(a Action) bool { return a == ActionCreateTask || a == ActionMove }

// undoReadable reports whether everything undo needs from the row is present.
func undoReadable(row *ActivityRow, outcomes map[string]*string) bool {
	if row.Outcome != ActivityApproved || row.UndoneAt != nil || !undoableClass(row.ActionClass) {
		return false
	}
	if row.ActionClass == ActionCreateTask {
		return row.TargetTaskID != nil && *row.TargetTaskID != ""
	}
	if row.ProposalID == nil {
		return false
	}
	raw, present := outcomes[*row.ProposalID]
	if !present {
		return false
	}
	o, ok := parseMoveOutcome(raw)
	return ok && !o.Noop
}

// authorizedCoordinator authorizes the scope and confirms the coordinator
// belongs to the workspace.
func (s *Service) authorizedCoordinator(ctx context.Context, workspaceID, coordinatorID string, scope authz.Scope) (*Coordinator, error) {
	if !s.phase2 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, scope); err != nil {
		return nil, err
	}
	return s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
}

// ListActivity returns one page of the coordinator's activity log.
func (s *Service) ListActivity(ctx context.Context, workspaceID, coordinatorID string, p ListActivityParams) (*ActivityPage, error) {
	if _, err := s.authorizedCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	return s.listActivity(ctx, coordinatorID, p)
}

// ListActivityForCoordinator is ListActivity for a caller that already holds
// the coordinator, such as the coordinator's own read tool.
func (s *Service) ListActivityForCoordinator(ctx context.Context, coordinatorID string, p ListActivityParams) (*ActivityPage, error) {
	if !s.phase2 {
		return nil, ErrNotFound
	}
	if _, err := s.store.GetCoordinatorByID(ctx, coordinatorID); err != nil {
		return nil, err
	}
	return s.listActivity(ctx, coordinatorID, p)
}

func (s *Service) listActivity(ctx context.Context, coordinatorID string, p ListActivityParams) (*ActivityPage, error) {
	limit := p.Limit
	if limit == 0 {
		limit = DefaultActivityLimit
	}
	if limit < 1 || limit > MaxActivityLimit {
		return nil, &FieldError{Field: "limit", Message: fmt.Sprintf("limit must be between 1 and %d", MaxActivityLimit)}
	}
	if p.Class != "" && !validActivityClass(Action(p.Class)) {
		return nil, &FieldError{Field: "class", Message: "class is not an action class"}
	}
	var cursor *ActivityCursor
	if p.Before != "" {
		c, err := decodeActivityCursor(p.Before)
		if err != nil {
			return nil, err
		}
		cursor = c
	}
	rows, err := s.store.ListActivityRows(ctx, coordinatorID, Action(p.Class), cursor, limit+1)
	if err != nil {
		return nil, err
	}
	page := &ActivityPage{Rows: []ActivityItem{}}
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[limit-1]
		next := encodeActivityCursor(ActivityCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		page.NextCursor = &next
	}
	page.Rows = s.enrichActivity(ctx, coordinatorID, rows)
	return page, nil
}

// enrichActivity computes the read-time fields. A failed read leaves its field
// null and never fails the list.
func (s *Service) enrichActivity(ctx context.Context, coordinatorID string, rows []ActivityRow) []ActivityItem {
	var proposalIDs []string
	for i := range rows {
		if rows[i].ActionClass == ActionMove && rows[i].ProposalID != nil {
			proposalIDs = append(proposalIDs, *rows[i].ProposalID)
		}
	}
	outcomes, err := s.store.MoveOutcomes(ctx, coordinatorID, proposalIDs)
	if err != nil {
		s.logger.Warn("failed to read move outcomes for activity list", zap.String("coordinator_id", coordinatorID), zap.Error(err))
		outcomes = map[string]*string{}
	}
	items := make([]ActivityItem, len(rows))
	for i := range rows {
		item := ActivityItem{ActivityRow: rows[i], Undoable: undoReadable(&rows[i], outcomes)}
		if rows[i].ActionClass == ActionMove && rows[i].ProposalID != nil {
			if o, ok := parseMoveOutcome(outcomes[*rows[i].ProposalID]); ok {
				from := o.FromStepID
				item.FromStepID = &from
			}
		}
		item.TargetTaskIdentifier = s.taskIdentifier(ctx, rows[i].TargetTaskID)
		items[i] = item
	}
	return items
}

func (s *Service) taskIdentifier(ctx context.Context, taskID *string) *string {
	if taskID == nil || s.undoTasks == nil {
		return nil
	}
	task, err := s.undoTasks.GetTask(ctx, *taskID)
	if err != nil {
		if !errors.Is(err, ErrTaskNotFound) {
			s.logger.Warn("failed to read task for activity list", zap.String("task_id", *taskID), zap.Error(err))
		}
		return nil
	}
	if task.Identifier == "" {
		return nil
	}
	id := task.Identifier
	return &id
}

// GetActivitySummary is the route form of ActivitySummary: it authorizes the
// read and validates the window.
func (s *Service) GetActivitySummary(ctx context.Context, workspaceID, coordinatorID string, days int) (*ActivitySummary, error) {
	if _, err := s.authorizedCoordinator(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if days < 1 || days > MaxSummaryDays {
		return nil, &FieldError{Field: "days", Message: fmt.Sprintf("days must be between 1 and %d", MaxSummaryDays)}
	}
	return s.ActivitySummary(ctx, coordinatorID, days)
}

// ActivitySummary counts the coordinator's rows created in the last days,
// per class. Phase 3 and the goal baselines read it; days is not validated
// here beyond being positive.
func (s *Service) ActivitySummary(ctx context.Context, coordinatorID string, days int) (*ActivitySummary, error) {
	if days < 1 {
		return nil, &FieldError{Field: "days", Message: "days must be positive"}
	}
	if _, err := s.store.GetCoordinatorByID(ctx, coordinatorID); err != nil {
		return nil, err
	}
	counts, err := s.store.ActivityCounts(ctx, coordinatorID, time.Now().UTC().AddDate(0, 0, -days))
	if err != nil {
		return nil, err
	}
	earliest, err := s.store.EarliestActivityAt(ctx, coordinatorID)
	if err != nil {
		return nil, err
	}
	sum := &ActivitySummary{Days: days, EarliestRowAt: earliest, Classes: map[Action]OutcomeCounts{}}
	for _, a := range AllActions {
		sum.Classes[a] = OutcomeCounts{}
	}
	for _, c := range counts {
		oc := sum.Classes[c.Class]
		switch c.Outcome {
		case ActivityProposed:
			oc.Proposed += c.Rows
		case ActivityApproved:
			oc.Approved += c.Rows
			if c.Edited {
				oc.ApprovedWithEdits += c.Rows
			}
		case ActivityRejected:
			oc.Rejected += c.Rows
		case ActivityFailed:
			oc.Failed += c.Rows
		case ActivityRefused:
			oc.Refused += c.Refusal
		case ActivityUndone:
			oc.Undone += c.Rows
		}
		sum.Classes[c.Class] = oc
	}
	return sum, nil
}

// ActivityToolRow is a list row as the coordinator's own read tool sees it:
// the same fields as the list route without the identities of the managers.
type ActivityToolRow struct {
	ID                   string                `json:"id"`
	ActionClass          Action                `json:"action_class"`
	Outcome              ActivityOutcome       `json:"outcome"`
	Authorization        ActivityAuthorization `json:"authorization"`
	TargetTaskID         *string               `json:"target_task_id"`
	TargetTaskIdentifier *string               `json:"target_task_identifier"`
	ProposalID           *string               `json:"proposal_id"`
	ReasonCode           *string               `json:"reason_code"`
	Detail               string                `json:"detail"`
	Edited               bool                  `json:"edited"`
	RefusalCount         int                   `json:"refusal_count"`
	UndoneAt             *time.Time            `json:"undone_at"`
	UndoOfID             *string               `json:"undo_of_id"`
	FromStepID           *string               `json:"from_step_id"`
	Undoable             bool                  `json:"undoable"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

// ActivityToolPage is one page of the read tool's result.
type ActivityToolPage struct {
	Rows       []ActivityToolRow `json:"rows"`
	NextCursor *string           `json:"next_cursor"`
}

// ForTool strips the manager identities from a page.
func (p *ActivityPage) ForTool() *ActivityToolPage {
	out := &ActivityToolPage{Rows: make([]ActivityToolRow, len(p.Rows)), NextCursor: p.NextCursor}
	for i, r := range p.Rows {
		out.Rows[i] = ActivityToolRow{
			ID: r.ID, ActionClass: r.ActionClass, Outcome: r.Outcome, Authorization: r.Authorization,
			TargetTaskID: r.TargetTaskID, TargetTaskIdentifier: r.TargetTaskIdentifier, ProposalID: r.ProposalID,
			ReasonCode: r.ReasonCode, Detail: r.Detail, Edited: r.Edited, RefusalCount: r.RefusalCount,
			UndoneAt: r.UndoneAt, UndoOfID: r.UndoOfID, FromStepID: r.FromStepID, Undoable: r.Undoable,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
	}
	return out
}
