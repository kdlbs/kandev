package coordinator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
)

const (
	watchScopeAll      = "all"
	watchScopeSelected = "selected"
	goalStatusActive   = "active"
	goalStatusMet      = "met"
)

// WatchSet is the set of workflows a coordinator watches.
type WatchSet struct {
	All         bool
	WorkflowIDs []string
}

// Contains reports whether the workflow is watched. The empty id is never
// watched.
func (w WatchSet) Contains(workflowID string) bool {
	if workflowID == "" {
		return false
	}
	if w.All {
		return true
	}
	for _, id := range w.WorkflowIDs {
		if id == workflowID {
			return true
		}
	}
	return false
}

func normalizeWatchScope(scope string) string {
	if scope == watchScopeAll {
		return watchScopeAll
	}
	return watchScopeSelected
}

// LoadWatchSet reads the coordinator's watch scope and selected workflows
// through exec. An unknown stored scope reads as selected.
func (s *Store) LoadWatchSet(ctx context.Context, exec coordinatorExec, coordinatorID string) (WatchSet, error) {
	var scope string
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT watch_scope FROM coordinators WHERE id = ?`), coordinatorID).Scan(&scope)
	if errors.Is(err, sql.ErrNoRows) {
		return WatchSet{}, ErrNotFound
	}
	if err != nil {
		return WatchSet{}, fmt.Errorf("read watch scope: %w", err)
	}
	set := WatchSet{All: normalizeWatchScope(scope) == watchScopeAll, WorkflowIDs: []string{}}
	rows, err := exec.QueryContext(ctx, s.db.Rebind(`SELECT workflow_id FROM coordinator_watches WHERE coordinator_id = ?`), coordinatorID)
	if err != nil {
		return WatchSet{}, fmt.Errorf("read watches: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return WatchSet{}, fmt.Errorf("scan watch: %w", err)
		}
		set.WorkflowIDs = append(set.WorkflowIDs, id)
	}
	if err := rows.Err(); err != nil {
		return WatchSet{}, fmt.Errorf("read watches: %w", err)
	}
	sort.Strings(set.WorkflowIDs)
	return set, nil
}

// StandingOrder is one stored standing order.
type StandingOrder struct {
	ID               string     `db:"id" json:"id"`
	CoordinatorID    string     `db:"coordinator_id" json:"coordinator_id"`
	Text             string     `db:"text" json:"text"`
	CreatedBy        string     `db:"created_by" json:"created_by"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	RetiredAt        *time.Time `db:"retired_at" json:"retired_at"`
	RetiredBy        *string    `db:"retired_by" json:"retired_by"`
	SourceProposalID *string    `db:"source_proposal_id" json:"source_proposal_id"`
	LastAppliedAt    *time.Time `db:"last_applied_at" json:"last_applied_at"`
}

// ActiveStandingOrders returns the coordinator's active orders, oldest first.
// The result is never nil.
func (s *Store) ActiveStandingOrders(ctx context.Context, exec coordinatorExec, coordinatorID string) ([]StandingOrder, error) {
	rows, err := exec.QueryContext(ctx, s.db.Rebind(`SELECT id, coordinator_id, text, created_by, created_at, retired_at, retired_by, source_proposal_id, last_applied_at
		FROM coordinator_standing_orders WHERE coordinator_id = ? AND retired_at IS NULL ORDER BY created_at ASC, id ASC`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("list standing orders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StandingOrder{}
	for rows.Next() {
		var o StandingOrder
		if err := rows.Scan(&o.ID, &o.CoordinatorID, &o.Text, &o.CreatedBy, &o.CreatedAt, &o.RetiredAt, &o.RetiredBy, &o.SourceProposalID, &o.LastAppliedAt); err != nil {
			return nil, fmt.Errorf("scan standing order: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list standing orders: %w", err)
	}
	return out, nil
}

// GoalCriterion is one checklist item of a goal.
type GoalCriterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// Goal is a coordinator goal.
type Goal struct {
	ID            string          `json:"id"`
	CoordinatorID string          `json:"coordinator_id"`
	Name          string          `json:"name"`
	DueOn         *string         `json:"due_on"`
	Status        string          `json:"status"`
	Criteria      []GoalCriterion `json:"criteria"`
	Baseline      json.RawMessage `json:"baseline"`
	SetAt         time.Time       `json:"set_at"`
	MetAt         *time.Time      `json:"met_at"`
	MetBy         *string         `json:"met_by"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

const goalColumns = `id, coordinator_id, name, due_on, status, criteria_json, baseline_json, set_at, met_at, met_by, created_at, updated_at`

// ActiveGoal returns the coordinator's active goal, or nil when none.
func (s *Store) ActiveGoal(ctx context.Context, exec coordinatorExec, coordinatorID string) (*Goal, error) {
	return s.readGoal(ctx, exec, `SELECT `+goalColumns+` FROM coordinator_goals WHERE coordinator_id = ? AND status = '`+goalStatusActive+`' LIMIT 1`, coordinatorID)
}

// LastMetGoal returns the most recently met goal, or nil when none.
func (s *Store) LastMetGoal(ctx context.Context, exec coordinatorExec, coordinatorID string) (*Goal, error) {
	return s.readGoal(ctx, exec, `SELECT `+goalColumns+` FROM coordinator_goals WHERE coordinator_id = ? AND status = '`+goalStatusMet+`' ORDER BY met_at DESC, id DESC LIMIT 1`, coordinatorID)
}

func (s *Store) readGoal(ctx context.Context, exec coordinatorExec, query, coordinatorID string) (*Goal, error) {
	var (
		g        Goal
		criteria string
		baseline string
	)
	err := exec.QueryRowContext(ctx, s.db.Rebind(query), coordinatorID).Scan(&g.ID, &g.CoordinatorID, &g.Name, &g.DueOn, &g.Status,
		&criteria, &baseline, &g.SetAt, &g.MetAt, &g.MetBy, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read goal: %w", err)
	}
	g.Criteria = []GoalCriterion{}
	if err := json.Unmarshal([]byte(criteria), &g.Criteria); err != nil {
		return nil, fmt.Errorf("decode goal criteria: %w", err)
	}
	g.Baseline = json.RawMessage(baseline)
	return &g, nil
}

// PolicyView is the effective, read-only policy of one coordinator.
type PolicyView struct {
	CoordinatorID  string
	WorkspaceID    string
	PolicyRevision int
	Actions        map[Action]Setting
	WatchScope     string
	WorkflowIDs    []string
}

// policyFor parses the stored policy. An unreadable policy denies every
// action, and the failure is logged once per coordinator revision.
func (s *Service) policyFor(c *Coordinator) Policy {
	p, err := ParsePolicy(c.PolicyJSON)
	if err == nil {
		return p
	}
	key := fmt.Sprintf("%s:%d", c.ID, c.PolicyRevision)
	if _, seen := s.policyErrLogged.LoadOrStore(key, struct{}{}); !seen {
		s.logger.Error("coordinator policy unreadable; denying every action",
			zap.String("coordinator_id", c.ID), zap.Int("policy_revision", c.PolicyRevision), zap.Error(err))
	}
	return Policy{Version: policyVersion, Actions: deniedActions()}
}

// Policy returns the coordinator's effective policy and watch set. With
// phase 2 off it is the fixed phase-1 policy.
func (s *Service) Policy(ctx context.Context, coordinatorID string) (PolicyView, error) {
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return PolicyView{}, err
	}
	if !s.phase2 {
		return PolicyView{
			CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID,
			Actions: PhaseOnePolicy().Actions, WatchScope: watchScopeAll, WorkflowIDs: []string{},
		}, nil
	}
	set, err := s.store.LoadWatchSet(ctx, s.store.ro, c.ID)
	if err != nil {
		return PolicyView{}, err
	}
	scope := watchScopeSelected
	if set.All {
		scope = watchScopeAll
	}
	return PolicyView{
		CoordinatorID: c.ID, WorkspaceID: c.WorkspaceID, PolicyRevision: c.PolicyRevision,
		Actions: s.policyFor(c).Actions, WatchScope: scope, WorkflowIDs: set.WorkflowIDs,
	}, nil
}
