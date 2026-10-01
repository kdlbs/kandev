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

	"github.com/kandev/kandev/internal/coordinator/watch"
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
	// Projects is the stored project scope; its zero value is every project.
	Projects watch.Scope
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

// Project entry kinds of coordinator_watch_projects.
const (
	projectKindSet        = "repository_set"
	projectKindRepository = "repository"
)

// LoadWatchSet reads the coordinator's watch scope, selected workflows and
// project scope through exec. An unknown stored scope reads as selected; an
// unknown stored project scope reads as selected too, so it never widens.
func (s *Store) LoadWatchSet(ctx context.Context, exec coordinatorExec, coordinatorID string) (WatchSet, error) {
	var (
		scope, projectScope string
		includeNoRepo       bool
	)
	err := exec.QueryRowContext(ctx, s.db.Rebind(`SELECT watch_scope, project_scope, include_no_repository FROM coordinators WHERE id = ?`), coordinatorID).
		Scan(&scope, &projectScope, &includeNoRepo)
	if errors.Is(err, sql.ErrNoRows) {
		return WatchSet{}, ErrNotFound
	}
	if err != nil {
		return WatchSet{}, fmt.Errorf("read watch scope: %w", err)
	}
	set := WatchSet{All: normalizeWatchScope(scope) == watchScopeAll, WorkflowIDs: []string{}}
	if normalizeWatchScope(projectScope) == watchScopeSelected {
		if set.Projects, err = s.loadProjectScope(ctx, exec, coordinatorID, includeNoRepo); err != nil {
			return WatchSet{}, err
		}
	}
	if set.All {
		return set, nil
	}
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

// ProjectEntry is one stored project entry.
type ProjectEntry struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// storedProjectEntries returns the coordinator's entries ordered by kind
// (repository_set before repository) then id.
func (s *Store) storedProjectEntries(ctx context.Context, exec coordinatorExec, coordinatorID string) ([]ProjectEntry, error) {
	rows, err := exec.QueryContext(ctx, s.db.Rebind(`SELECT entry_kind, entry_id FROM coordinator_watch_projects WHERE coordinator_id = ?
		ORDER BY CASE entry_kind WHEN 'repository_set' THEN 0 ELSE 1 END, entry_id`), coordinatorID)
	if err != nil {
		return nil, fmt.Errorf("read watch projects: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ProjectEntry{}
	for rows.Next() {
		var e ProjectEntry
		if err := rows.Scan(&e.Kind, &e.ID); err != nil {
			return nil, fmt.Errorf("scan watch project: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) loadProjectScope(ctx context.Context, exec coordinatorExec, coordinatorID string, includeNoRepo bool) (watch.Scope, error) {
	entries, err := s.storedProjectEntries(ctx, exec, coordinatorID)
	if err != nil {
		return watch.Scope{}, err
	}
	scope := watch.Scope{Selected: true, IncludeNoRepo: includeNoRepo}
	for _, e := range entries {
		if e.Kind == projectKindSet {
			scope.SetIDs = append(scope.SetIDs, e.ID)
		} else {
			scope.RepoIDs = append(scope.RepoIDs, e.ID)
		}
	}
	return scope, nil
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

// ActiveStandingOrders returns the coordinator's active orders, oldest first,
// read through the reader pool. The result is never nil.
func (s *Store) ActiveStandingOrders(ctx context.Context, coordinatorID string) ([]StandingOrder, error) {
	return s.activeStandingOrdersOn(ctx, s.ro, coordinatorID)
}

func (s *Store) activeStandingOrdersOn(ctx context.Context, exec coordinatorExec, coordinatorID string) ([]StandingOrder, error) {
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
func (s *Store) ActiveGoal(ctx context.Context, coordinatorID string) (*Goal, error) {
	return s.readGoal(ctx, s.ro, `SELECT `+goalColumns+` FROM coordinator_goals WHERE coordinator_id = ? AND status = '`+goalStatusActive+`' LIMIT 1`, coordinatorID)
}

// LastMetGoal returns the most recently met goal, or nil when none.
func (s *Store) LastMetGoal(ctx context.Context, coordinatorID string) (*Goal, error) {
	return s.readGoal(ctx, s.ro, `SELECT `+goalColumns+` FROM coordinator_goals WHERE coordinator_id = ? AND status = '`+goalStatusMet+`' ORDER BY met_at DESC, id DESC LIMIT 1`, coordinatorID)
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
	CoordinatorID  string             `json:"coordinator_id"`
	WorkspaceID    string             `json:"workspace_id"`
	PolicyRevision int                `json:"policy_revision"`
	Actions        map[Action]Setting `json:"actions"`
	WatchScope     string             `json:"watch_scope"`
	WorkflowIDs    []string           `json:"workflow_ids"`
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
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
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

// Phase2 reports whether the phase-2 control surface is on.
func (s *Service) Phase2() bool { return s.phase2 }

// ActionAllowed reports whether the coordinator's live policy permits the
// action to be proposed. An unreadable policy allows nothing.
func (s *Service) ActionAllowed(ctx context.Context, coordinatorID string, action Action) (bool, error) {
	c, err := s.store.GetCoordinatorByID(ctx, coordinatorID)
	if err != nil {
		return false, err
	}
	return s.policyFor(c).Allows(action), nil
}
