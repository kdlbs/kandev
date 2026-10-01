package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// measureDirection is the API value of a measure's direction.
type measureDirection string

const (
	directionUp         measureDirection = "up"
	directionDown       measureDirection = "down"
	directionNoBaseline measureDirection = "none_no_baseline"
	directionNoneSmall  measureDirection = "none_small"

	// directionThreshold is the smallest change from the baseline that shows
	// a direction.
	directionThreshold = 2

	// measureWindowDays is the window of the two proposal measures.
	measureWindowDays = 7
)

// goalBaseline is the frozen baseline stored with a goal. A nil proposal
// count means the coordinator was too young to have one.
type goalBaseline struct {
	OpenTasks  int64  `json:"open_tasks"`
	Approved7d *int64 `json:"approved_7d"`
	Rejected7d *int64 `json:"rejected_7d"`
}

// Measure is one measure at read time next to its baseline.
type Measure struct {
	Current   int64            `json:"current"`
	Baseline  *int64           `json:"baseline"`
	Direction measureDirection `json:"direction"`
}

// GoalMeasures are the three measures of an active goal.
type GoalMeasures struct {
	OpenTasks  Measure `json:"open_tasks"`
	Approved7d Measure `json:"approved_7d"`
	Rejected7d Measure `json:"rejected_7d"`
}

func newMeasure(current int64, baseline *int64) Measure {
	m := Measure{Current: current, Baseline: baseline, Direction: directionNoBaseline}
	if baseline == nil {
		return m
	}
	switch delta := current - *baseline; {
	case delta >= directionThreshold:
		m.Direction = directionUp
	case -delta >= directionThreshold:
		m.Direction = directionDown
	default:
		m.Direction = directionNoneSmall
	}
	return m
}

// CountOpenWatchedTasks counts the workspace's open tasks in the watch set
// through exec: not archived, not COMPLETED, not ephemeral and not created by
// a coordinator or an automation run. It requires the tasks table. An empty
// selected set counts nothing. Under a project scope the workflow-matching
// tasks are filtered by their repositories through watch.Task.
func (s *Store) CountOpenWatchedTasks(ctx context.Context, exec coordinatorExec, workspaceID string, watch WatchSet) (int64, error) {
	query := `SELECT id, workflow_id FROM tasks WHERE workspace_id = ? AND archived_at IS NULL AND state != 'COMPLETED'
		AND is_ephemeral = 0 AND COALESCE(origin, '') NOT IN ('coordinator', 'automation_run')`
	args := []any{workspaceID}
	if !watch.All {
		if len(watch.WorkflowIDs) == 0 {
			return 0, nil
		}
		query += ` AND workflow_id IN (?` + strings.Repeat(", ?", len(watch.WorkflowIDs)-1) + `)`
		for _, id := range watch.WorkflowIDs {
			args = append(args, id)
		}
	}
	var reader ProjectReader
	if r := s.projects.Load(); r != nil {
		reader = *r
	}
	g, err := newWatchGateFrom(ctx, reader, watch, workspaceID)
	if err != nil {
		return 0, fmt.Errorf("count open watched tasks: %w", err)
	}
	if !g.needsRepositories() {
		return s.countOpenTasks(ctx, exec, query, args)
	}
	return s.countOpenTasksInProjects(ctx, exec, query, args, g, reader)
}

func (s *Store) countOpenTasks(ctx context.Context, exec coordinatorExec, selectQuery string, args []any) (int64, error) {
	query := strings.Replace(selectQuery, "SELECT id, workflow_id FROM", "SELECT COUNT(*) FROM", 1)
	var n int64
	if err := exec.QueryRowContext(ctx, s.db.Rebind(query), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("count open watched tasks: %w", err)
	}
	return n, nil
}

func (s *Store) countOpenTasksInProjects(ctx context.Context, exec coordinatorExec, query string, args []any, g *watchGate, reader ProjectReader) (int64, error) {
	type openTask struct{ id, workflowID string }
	r, err := exec.QueryContext(ctx, s.db.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("count open watched tasks: %w", err)
	}
	defer func() { _ = r.Close() }()
	var rows []openTask
	for r.Next() {
		var t openTask
		if err := r.Scan(&t.id, &t.workflowID); err != nil {
			return 0, fmt.Errorf("count open watched tasks: %w", err)
		}
		rows = append(rows, t)
	}
	if err := r.Err(); err != nil {
		return 0, fmt.Errorf("count open watched tasks: %w", err)
	}
	_ = r.Close()
	if len(rows) == 0 {
		return 0, nil
	}
	if reader == nil {
		return 0, errors.New("count open watched tasks: project reads are not wired")
	}
	ids := make([]string, 0, len(rows))
	for _, t := range rows {
		ids = append(ids, t.id)
	}
	repos, err := reader.ListTaskRepositoryIDsByTaskIDs(ctx, ids)
	if err != nil {
		return 0, fmt.Errorf("count open watched tasks: %w", err)
	}
	var n int64
	for _, t := range rows {
		if g.task(t.workflowID, repos[t.id]) {
			n++
		}
	}
	return n, nil
}

// proposalCounts are the approved and rejected rows of every class.
type proposalCounts struct {
	approved, rejected int64
}

func sumProposalCounts(counts []ActivityCount) proposalCounts {
	var out proposalCounts
	for _, c := range counts {
		switch c.Outcome {
		case ActivityApproved:
			out.approved += c.Rows
		case ActivityRejected:
			out.rejected += c.Rows
		}
	}
	return out
}

// computeBaseline reads the baseline of a goal being set at setAt, through
// the locked handle.
func (s *Store) computeBaseline(ctx context.Context, exec coordinatorExec, c *Coordinator, setAt time.Time) (goalBaseline, error) {
	watch, err := s.LoadWatchSet(ctx, exec, c.ID)
	if err != nil {
		return goalBaseline{}, err
	}
	open, err := s.CountOpenWatchedTasks(ctx, exec, c.WorkspaceID, watch)
	if err != nil {
		return goalBaseline{}, err
	}
	base := goalBaseline{OpenTasks: open}
	windowStart := setAt.AddDate(0, 0, -measureWindowDays)
	if c.CreatedAt.After(windowStart) {
		return base, nil
	}
	counts, err := s.ActivityCountsIn(ctx, exec, c.ID, windowStart, setAt)
	if err != nil {
		return goalBaseline{}, err
	}
	sums := sumProposalCounts(counts)
	base.Approved7d, base.Rejected7d = &sums.approved, &sums.rejected
	return base, nil
}

// goalMeasures computes the measures of an active goal at read time.
func (s *Service) goalMeasures(ctx context.Context, c *Coordinator, g *Goal) (*GoalMeasures, error) {
	var base goalBaseline
	if err := json.Unmarshal(g.Baseline, &base); err != nil {
		return nil, fmt.Errorf("decode goal baseline: %w", err)
	}
	watch, err := s.store.LoadWatchSet(ctx, s.store.ro, c.ID)
	if err != nil {
		return nil, err
	}
	open, err := s.store.CountOpenWatchedTasks(ctx, s.store.ro, c.WorkspaceID, watch)
	if err != nil {
		return nil, err
	}
	summary, err := s.ActivitySummary(ctx, c.ID, measureWindowDays)
	if err != nil {
		return nil, err
	}
	var approved, rejected int64
	for _, oc := range summary.Classes {
		approved += oc.Approved
		rejected += oc.Rejected
	}
	return &GoalMeasures{
		OpenTasks:  newMeasure(open, &base.OpenTasks),
		Approved7d: newMeasure(approved, base.Approved7d),
		Rejected7d: newMeasure(rejected, base.Rejected7d),
	}, nil
}
