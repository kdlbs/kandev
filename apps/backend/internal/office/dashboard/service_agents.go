package dashboard

import (
	"context"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// LiveRun holds the enriched data for a single agent run.
type LiveRun struct {
	AgentID        string
	AgentName      string
	TaskID         string
	TaskTitle      string
	TaskIdentifier string
	Status         string
	DurationMs     int64
	StartedAt      string
	FinishedAt     string
}

// GetLiveRuns returns recent agent runs (active first, then finished) enriched with agent and task context.
func (s *DashboardService) GetLiveRuns(ctx context.Context, wsID string, limit int) ([]LiveRun, error) {
	if limit <= 0 {
		limit = 4
	}
	sessions, err := s.repo.QueryRecentSessions(ctx, wsID, limit*4)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return []LiveRun{}, nil
	}

	// Collect unique task IDs for batch lookup.
	taskIDSet := make(map[string]struct{}, len(sessions))
	for _, s := range sessions {
		taskIDSet[s.TaskID] = struct{}{}
	}
	taskIDs := make([]string, 0, len(taskIDSet))
	for id := range taskIDSet {
		taskIDs = append(taskIDs, id)
	}

	taskRows, _ := s.repo.GetTasksByIDs(ctx, taskIDs)
	taskMap := make(map[string]sqlite.TaskTitleRow, len(taskRows))
	for _, t := range taskRows {
		taskMap[t.ID] = t
	}

	// Collect unique agent_execution_ids referenced by sessions and fetch
	// only those agent rows (batched), instead of loading every agent in
	// the workspace.
	agentIDSet := make(map[string]struct{}, len(sessions))
	for _, s := range sessions {
		if s.AgentExecID != "" {
			agentIDSet[s.AgentExecID] = struct{}{}
		}
	}
	agentIDs := make([]string, 0, len(agentIDSet))
	for id := range agentIDSet {
		agentIDs = append(agentIDs, id)
	}
	agentInstances, _ := s.agents.ListAgentInstancesByIDs(ctx, agentIDs)
	agentByExecID := buildAgentByExecID(agentInstances)

	now := time.Now().UTC()
	runs := make([]LiveRun, 0, len(sessions))
	for _, sess := range sessions {
		if len(runs) >= limit {
			break
		}
		task := taskMap[sess.TaskID]
		agent := agentByExecID[sess.AgentExecID]
		run := LiveRun{
			AgentID:        sess.AgentExecID,
			AgentName:      agent,
			TaskID:         sess.TaskID,
			TaskTitle:      task.Title,
			TaskIdentifier: task.Identifier,
			Status:         liveRunStatus(sess.State),
			StartedAt:      sess.StartedAt,
		}
		run.DurationMs = computeDuration(sess.StartedAt, sess.CompletedAt, now)
		if sess.CompletedAt != nil {
			run.FinishedAt = *sess.CompletedAt
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// buildAgentByExecID builds a map from agent_execution_id to agent name using
// the AgentInstance.ID as the execution ID key (matching task_sessions.agent_execution_id).
func buildAgentByExecID(agents []*models.AgentInstance) map[string]string {
	m := make(map[string]string, len(agents))
	for _, a := range agents {
		m[a.ID] = a.Name
	}
	return m
}

// liveRunStatus maps a task_sessions.state value to a UI-friendly status string.
func liveRunStatus(state string) string {
	switch state {
	case stateCompleted:
		return "completed"
	case "FAILED":
		return "failed"
	case "RUNNING", "STARTING", "CREATED":
		return "running"
	default:
		return statusCancelledLowercase
	}
}

// AgentSummary is a per-agent dashboard card payload (B1 of
// office-dashboard-agent-cards). Holds the agent identity, current
// status, and up to 5 most-recent sessions.
type AgentSummary struct {
	AgentID        string           `json:"agent_id"`
	AgentName      string           `json:"agent_name"`
	AgentRole      string           `json:"agent_role"`
	Status         string           `json:"status"` // "live" | "finished" | "never_run"
	LiveSession    *SessionSummary  `json:"live_session,omitempty"`
	LastSession    *SessionSummary  `json:"last_session,omitempty"`
	RecentSessions []SessionSummary `json:"recent_sessions"`
	// LastRunStatus is "ok" | "failed" — drives the dashboard card's
	// "Last run failed" subtitle branch.
	LastRunStatus string `json:"last_run_status,omitempty"`
	// PauseReason mirrors the agent row when set; the card switches
	// to "Paused — N consecutive failures" when this starts with
	// "Auto-paused:".
	PauseReason string `json:"pause_reason,omitempty"`
	// ConsecutiveFailures is rendered into the auto-pause subtitle.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`
}

// SessionSummary is a thin per-session view for the agent card. Includes
// the resolved task identifier/title and a tool-call command count for
// the row's "ran N commands" label.
//
// DurationSeconds is computed once at serialization so the frontend
// renders a stable value across refetches:
//   - RUNNING: now - started_at (intentionally grows on each refetch
//     so live cards reflect current elapsed time)
//   - non-RUNNING with completed_at: completed_at - started_at
//   - non-RUNNING without completed_at (e.g. office IDLE — fire-and-
//     forget sessions stay non-terminal but the agent is gone):
//     updated_at - started_at, since updated_at marks the moment the
//     state flipped to IDLE.
type SessionSummary struct {
	SessionID       string     `json:"session_id"`
	TaskID          string     `json:"task_id"`
	TaskIdentifier  string     `json:"task_identifier"`
	TaskTitle       string     `json:"task_title"`
	State           string     `json:"state"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationSeconds int        `json:"duration_seconds"`
	CommandCount    int        `json:"command_count"`
}

// agentSummaryStateRunning marks a session as the agent's live row.
const agentSummaryStateRunning = "RUNNING"

// agentSummaryStatusLive / Finished / NeverRun are the canonical Status
// values returned to the client.
const (
	agentSummaryStatusLive     = "live"
	agentSummaryStatusFinished = "finished"
	agentSummaryStatusNeverRun = "never_run"
)

// GetAgentSummaries returns one AgentSummary per workspace agent, sorted
// live-first. Each summary carries up to 5 most recent sessions enriched
// with task identifier/title and tool-call counts.
func (s *DashboardService) GetAgentSummaries(ctx context.Context, wsID string) ([]AgentSummary, error) {
	agents, err := s.agents.ListAgentInstances(ctx, wsID)
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return []AgentSummary{}, nil
	}

	sessionsByAgent, allSessions := s.collectAgentSessions(ctx, agents)
	taskMap := s.resolveTaskTitles(ctx, allSessions)
	cmdCounts := s.resolveCommandCounts(ctx, allSessions)

	summaries := make([]AgentSummary, 0, len(agents))
	for _, agent := range agents {
		summaries = append(summaries, buildAgentSummary(agent, sessionsByAgent[agent.ID], taskMap, cmdCounts))
	}
	sortAgentSummaries(summaries)
	return summaries, nil
}

// collectAgentSessions fetches up to 5 recent sessions for each agent in a
// single batched query, returning both a per-agent map and a flat slice of
// every collected session. The batch query keeps query count constant
// regardless of agent count (was N+1).
func (s *DashboardService) collectAgentSessions(
	ctx context.Context, agents []*models.AgentInstance,
) (map[string][]sqlite.AgentSessionRow, []sqlite.AgentSessionRow) {
	byAgent := make(map[string][]sqlite.AgentSessionRow, len(agents))
	if len(agents) == 0 {
		return byAgent, nil
	}
	ids := make([]string, len(agents))
	for i, agent := range agents {
		ids[i] = agent.ID
	}
	rowsByAgent, err := s.repo.ListRecentSessionsByAgentBatch(ctx, ids, 5)
	if err != nil {
		s.logger.Warn("ListRecentSessionsByAgentBatch failed", zap.Error(err))
		return byAgent, nil
	}
	all := make([]sqlite.AgentSessionRow, 0, len(agents)*5)
	for _, agent := range agents {
		rows := rowsByAgent[agent.ID]
		if rows == nil {
			continue
		}
		byAgent[agent.ID] = rows
		all = append(all, rows...)
	}
	return byAgent, all
}

// resolveTaskTitles batches a single GetTasksByIDs call for every unique
// task referenced across all sessions.
func (s *DashboardService) resolveTaskTitles(ctx context.Context, sessions []sqlite.AgentSessionRow) map[string]sqlite.TaskTitleRow {
	if len(sessions) == 0 {
		return map[string]sqlite.TaskTitleRow{}
	}
	idSet := make(map[string]struct{}, len(sessions))
	for _, sess := range sessions {
		if sess.TaskID != "" {
			idSet[sess.TaskID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return map[string]sqlite.TaskTitleRow{}
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	rows, err := s.repo.GetTasksByIDs(ctx, ids)
	if err != nil {
		s.logger.Warn("GetTasksByIDs failed", zap.Error(err))
		return map[string]sqlite.TaskTitleRow{}
	}
	out := make(map[string]sqlite.TaskTitleRow, len(rows))
	for _, t := range rows {
		out[t.ID] = t
	}
	return out
}

// resolveCommandCounts batches a single tool-call COUNT query for every
// session ID across all agents.
func (s *DashboardService) resolveCommandCounts(ctx context.Context, sessions []sqlite.AgentSessionRow) map[string]int {
	if len(sessions) == 0 {
		return map[string]int{}
	}
	ids := make([]string, 0, len(sessions))
	for _, sess := range sessions {
		ids = append(ids, sess.ID)
	}
	counts, err := s.repo.CountToolCallMessagesBySession(ctx, ids)
	if err != nil {
		s.logger.Warn("CountToolCallMessagesBySession failed", zap.Error(err))
		return map[string]int{}
	}
	return counts
}

// buildAgentSummary assembles the final per-agent summary from a slice of
// already-resolved sessions plus the shared task/command-count maps.
func buildAgentSummary(
	agent *models.AgentInstance,
	sessions []sqlite.AgentSessionRow,
	taskMap map[string]sqlite.TaskTitleRow,
	cmdCounts map[string]int,
) AgentSummary {
	recent := make([]SessionSummary, 0, len(sessions))
	var live, last *SessionSummary
	for i := range sessions {
		row := sessions[i]
		summary := buildSessionSummary(row, taskMap, cmdCounts)
		recent = append(recent, summary)
		if live == nil && row.State == agentSummaryStateRunning {
			cp := summary
			live = &cp
		}
	}
	if len(recent) > 0 {
		cp := recent[0]
		last = &cp
	}

	status := agentSummaryStatusNeverRun
	if live != nil {
		status = agentSummaryStatusLive
	} else if last != nil {
		status = agentSummaryStatusFinished
	}

	lastRunStatus := ""
	if last != nil {
		// FAILED office sessions surface here as last_run_status="failed"
		// so the card subtitle flips to "Last run failed". Anything else
		// is "ok" (the absence of a failure for the most recent run).
		if last.State == "FAILED" {
			lastRunStatus = "failed"
		} else {
			lastRunStatus = "ok"
		}
	}

	return AgentSummary{
		AgentID:             agent.ID,
		AgentName:           agent.Name,
		AgentRole:           string(agent.Role),
		Status:              status,
		LiveSession:         live,
		LastSession:         last,
		RecentSessions:      recent,
		LastRunStatus:       lastRunStatus,
		PauseReason:         agent.PauseReason,
		ConsecutiveFailures: agent.ConsecutiveFailures,
	}
}

// buildSessionSummary projects an AgentSessionRow into a SessionSummary,
// filling in task identifier/title and command count from the shared maps.
func buildSessionSummary(
	row sqlite.AgentSessionRow,
	taskMap map[string]sqlite.TaskTitleRow,
	cmdCounts map[string]int,
) SessionSummary {
	task := taskMap[row.TaskID]
	startedAt, _ := parseSessionTime(row.StartedAt)
	var completedAt *time.Time
	if row.CompletedAt != nil && *row.CompletedAt != "" {
		if t, ok := parseSessionTime(*row.CompletedAt); ok {
			completedAt = &t
		}
	}
	updatedAt, _ := parseSessionTime(row.UpdatedAt)
	return SessionSummary{
		SessionID:       row.ID,
		TaskID:          row.TaskID,
		TaskIdentifier:  task.Identifier,
		TaskTitle:       task.Title,
		State:           row.State,
		StartedAt:       startedAt,
		CompletedAt:     completedAt,
		DurationSeconds: computeDurationSeconds(row.State, startedAt, completedAt, updatedAt),
		CommandCount:    cmdCounts[row.ID],
	}
}

// computeDurationSeconds returns the displayed duration for a session row.
// See SessionSummary doc for the per-state policy.
func computeDurationSeconds(
	state string, startedAt time.Time, completedAt *time.Time, updatedAt time.Time,
) int {
	if startedAt.IsZero() {
		return 0
	}
	var end time.Time
	switch {
	case state == "RUNNING":
		end = time.Now().UTC()
	case completedAt != nil && !completedAt.IsZero():
		end = *completedAt
	case !updatedAt.IsZero():
		end = updatedAt
	default:
		end = time.Now().UTC()
	}
	delta := int(end.Sub(startedAt).Seconds())
	if delta < 0 {
		return 0
	}
	return delta
}

// parseSessionTime parses any of the time formats SQLite may return for
// task_sessions timestamps.
func parseSessionTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// sortAgentSummaries orders agent cards: live agents first, then by most
// recent session start, then alphabetical. Mutates in place.
func sortAgentSummaries(summaries []AgentSummary) {
	sort.SliceStable(summaries, func(i, j int) bool {
		a, b := summaries[i], summaries[j]
		if (a.Status == agentSummaryStatusLive) != (b.Status == agentSummaryStatusLive) {
			return a.Status == agentSummaryStatusLive
		}
		ai := agentSortRecencyKey(a)
		bi := agentSortRecencyKey(b)
		if !ai.Equal(bi) {
			return ai.After(bi)
		}
		return a.AgentName < b.AgentName
	})
}

// agentSortRecencyKey returns the timestamp used to rank two agents with
// the same liveness bucket. Live session start beats last session start;
// agents with no history sort last (zero time).
func agentSortRecencyKey(a AgentSummary) time.Time {
	if a.LiveSession != nil {
		return a.LiveSession.StartedAt
	}
	if a.LastSession != nil {
		return a.LastSession.StartedAt
	}
	return time.Time{}
}

// computeDuration returns the elapsed milliseconds for a session.
// If completed, uses finished - started. If still running, uses now - started.
func computeDuration(startedAt string, completedAt *string, now time.Time) int64 {
	startFormats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
	}
	var start time.Time
	for _, f := range startFormats {
		if t, err := time.Parse(f, startedAt); err == nil {
			start = t
			break
		}
	}
	if start.IsZero() {
		return 0
	}
	end := now
	if completedAt != nil {
		for _, f := range startFormats {
			if t, err := time.Parse(f, *completedAt); err == nil {
				end = t
				break
			}
		}
	}
	return end.Sub(start).Milliseconds()
}

// dashboardQueryBundle holds the results of all sub-queries that feed into
// GetDashboardData. Hard fields fail the request; soft fields fall back to
// zero/empty so the dashboard still renders.
type dashboardQueryBundle struct {
	agents           []*models.AgentInstance
	pendingApprovals int
	monthSpend       int64
	activity         []*models.ActivityEntry
	taskCount        int
	skillCount       int
	routineCount     int
	runActivity      []models.RunActivityDay
	taskBreakdown    models.TaskBreakdown
	recentTasks      []models.RecentTask
}

func (s *DashboardService) runHardQueries(
	g *errgroup.Group, gctx context.Context, wsID string, b *dashboardQueryBundle,
) {
	g.Go(func() error {
		a, err := s.agents.ListAgentInstances(gctx, wsID)
		if err != nil {
			return err
		}
		b.agents = a
		return nil
	})
	g.Go(func() error {
		p, err := s.repo.CountPendingApprovals(gctx, wsID)
		if err != nil {
			return err
		}
		b.pendingApprovals = p
		return nil
	})
	g.Go(func() error {
		m, err := s.costs.GetCostSummary(gctx, wsID)
		if err != nil {
			return err
		}
		b.monthSpend = m
		return nil
	})
	g.Go(func() error {
		a, err := s.repo.ListActivityEntries(gctx, wsID, 10)
		if err != nil {
			return err
		}
		b.activity = a
		return nil
	})
}

func (s *DashboardService) runSoftQueries(
	g *errgroup.Group, gctx context.Context, wsID string, b *dashboardQueryBundle,
) {
	g.Go(func() error {
		b.taskCount, _ = s.repo.CountTasksByWorkspace(gctx, wsID)
		return nil
	})
	g.Go(func() error {
		if s.skillLister == nil {
			return nil
		}
		if skills, sErr := s.skillLister.ListSkills(gctx, wsID); sErr == nil {
			b.skillCount = len(skills)
		}
		return nil
	})
	g.Go(func() error {
		if s.routineLister == nil {
			return nil
		}
		if routines, rErr := s.routineLister.ListRoutines(gctx, wsID); rErr == nil {
			for _, r := range routines {
				if r.Status == "active" {
					b.routineCount++
				}
			}
		}
		return nil
	})
	g.Go(func() error { b.runActivity = s.getRunActivity(gctx, wsID); return nil })
	g.Go(func() error { b.taskBreakdown = s.getTaskBreakdown(gctx, wsID); return nil })
	g.Go(func() error { b.recentTasks = s.getRecentTasks(gctx, wsID); return nil })
}

// GetDashboardData returns aggregated dashboard data for a workspace.
// All independent sub-queries run concurrently via errgroup so total wall
// time tracks the slowest query rather than the sum (Stream C of office
// optimization). The first hard error cancels remaining queries; soft
// failures (counts, time series) fall back to zero/empty without aborting.
func (s *DashboardService) GetDashboardData(ctx context.Context, wsID string) (*models.DashboardData, error) {
	var b dashboardQueryBundle
	g, gctx := errgroup.WithContext(ctx)
	s.runHardQueries(g, gctx, wsID, &b)
	s.runSoftQueries(g, gctx, wsID, &b)
	if err := g.Wait(); err != nil {
		return nil, err
	}

	agentCounts := countAgentsByStatus(b.agents)
	return &models.DashboardData{
		AgentCount:         len(b.agents),
		RunningCount:       agentCounts.running,
		PausedCount:        agentCounts.paused,
		ErrorCount:         agentCounts.errors,
		MonthSpendSubcents: b.monthSpend,
		PendingApprovals:   b.pendingApprovals,
		RecentActivity:     b.activity,
		TaskCount:          b.taskCount,
		SkillCount:         b.skillCount,
		RoutineCount:       b.routineCount,
		RunActivity:        b.runActivity,
		TaskBreakdown:      b.taskBreakdown,
		RecentTasks:        b.recentTasks,
	}, nil
}

// getRunActivity queries a 14-day run time-series and pads missing dates with zeros.
func (s *DashboardService) getRunActivity(ctx context.Context, wsID string) []models.RunActivityDay {
	const days = 14
	rows, err := s.repo.QueryRunActivity(ctx, wsID, days)
	if err != nil {
		return buildEmptyRunActivity(days)
	}
	return padRunActivity(rows, days)
}

// buildEmptyRunActivity returns a slice of n zero-filled RunActivityDay entries.
func buildEmptyRunActivity(days int) []models.RunActivityDay {
	result := make([]models.RunActivityDay, days)
	for i := range result {
		result[i].Date = time.Now().UTC().AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
	}
	return result
}

// padRunActivity fills missing dates so the returned slice always has exactly `days` entries.
func padRunActivity(rows []sqlite.RunActivityRow, days int) []models.RunActivityDay {
	byDate := make(map[string]models.RunActivityDay, len(rows))
	for _, r := range rows {
		byDate[r.Date] = models.RunActivityDay{
			Date:      r.Date,
			Succeeded: r.Succeeded,
			Failed:    r.Failed,
			Other:     r.Other,
		}
	}
	result := make([]models.RunActivityDay, days)
	for i := range result {
		d := time.Now().UTC().AddDate(0, 0, -(days - 1 - i)).Format("2006-01-02")
		if entry, ok := byDate[d]; ok {
			result[i] = entry
		} else {
			result[i] = models.RunActivityDay{Date: d}
		}
	}
	return result
}

// getTaskBreakdown returns bucketed task counts for the workspace.
func (s *DashboardService) getTaskBreakdown(ctx context.Context, wsID string) models.TaskBreakdown {
	rows, err := s.repo.QueryTaskBreakdown(ctx, wsID)
	if err != nil {
		return models.TaskBreakdown{}
	}
	return sqlite.BucketTaskBreakdown(rows)
}

// getRecentTasks returns the 10 most recently updated tasks.
func (s *DashboardService) getRecentTasks(ctx context.Context, wsID string) []models.RecentTask {
	rows, err := s.repo.QueryRecentTasks(ctx, wsID, 10)
	if err != nil {
		return nil
	}
	result := make([]models.RecentTask, len(rows))
	for i, r := range rows {
		result[i] = models.RecentTask{
			ID:                     r.ID,
			Identifier:             r.Identifier,
			Title:                  r.Title,
			Status:                 r.State,
			AssigneeAgentProfileID: r.AssigneeAgentProfileID,
			UpdatedAt:              r.UpdatedAt,
		}
	}
	return result
}

// TimelineEvent is an internal representation of a status-change event on a task.
type TimelineEvent struct {
	From string
	To   string
	At   string
}

type agentStatusCounts struct {
	running int
	paused  int
	errors  int
}

func countAgentsByStatus(agents []*models.AgentInstance) agentStatusCounts {
	var c agentStatusCounts
	for _, a := range agents {
		switch a.Status {
		case models.AgentStatusWorking:
			c.running++
		case models.AgentStatusPaused:
			c.paused++
		case models.AgentStatusStopped:
			if a.PauseReason != "" {
				c.errors++
			}
		}
	}
	return c
}
