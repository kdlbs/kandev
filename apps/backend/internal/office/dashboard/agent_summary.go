package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// AgentDashboardSummary is the JSON shape served at
// GET /api/v1/office/agents/:id/summary?days=14. It is a precomputed
// composition of the four daily charts, the latest-run card, the
// recent-tasks list and a costs section, so the dashboard renders in a
// single round-trip.
type AgentDashboardSummary struct {
	AgentID         string                 `json:"agent_id"`
	Days            int                    `json:"days"`
	LatestRun       *AgentLatestRunDTO     `json:"latest_run"`
	RunActivity     []AgentRunActivityDay  `json:"run_activity"`
	TasksByPriority []AgentTaskPriorityDay `json:"tasks_by_priority"`
	TasksByStatus   []AgentTaskStatusDay   `json:"tasks_by_status"`
	SuccessRate     []AgentSuccessRateDay  `json:"success_rate"`
	RecentTasks     []AgentRecentTaskDTO   `json:"recent_tasks"`
	CostAggregate   AgentCostAggregateDTO  `json:"cost_aggregate"`
	RecentRunCosts  []AgentRunCostDTO      `json:"recent_run_costs"`
}

// AgentLatestRunDTO is the "Latest Run" card payload.
type AgentLatestRunDTO struct {
	RunID       string  `json:"run_id"`
	RunIDShort  string  `json:"run_id_short"`
	Status      string  `json:"status"`
	Reason      string  `json:"reason"`
	TaskID      string  `json:"task_id,omitempty"`
	Summary     string  `json:"summary,omitempty"`
	RequestedAt string  `json:"requested_at"`
	FinishedAt  *string `json:"finished_at,omitempty"`
}

// AgentRunActivityDay is one row of the Run Activity chart.
// `total = succeeded + failed + other`.
type AgentRunActivityDay struct {
	Date      string `json:"date"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Other     int    `json:"other"`
	Total     int    `json:"total"`
}

// AgentTaskPriorityDay is one row of the Tasks by Priority chart.
type AgentTaskPriorityDay struct {
	Date     string `json:"date"`
	Critical int    `json:"critical"`
	High     int    `json:"high"`
	Medium   int    `json:"medium"`
	Low      int    `json:"low"`
}

// AgentTaskStatusDay is one row of the Tasks by Status chart.
type AgentTaskStatusDay struct {
	Date       string `json:"date"`
	Todo       int    `json:"todo"`
	InProgress int    `json:"in_progress"`
	InReview   int    `json:"in_review"`
	Done       int    `json:"done"`
	Blocked    int    `json:"blocked"`
	Cancelled  int    `json:"cancelled"`
	Backlog    int    `json:"backlog"`
}

// AgentSuccessRateDay is one row of the Success Rate chart.
type AgentSuccessRateDay struct {
	Date      string `json:"date"`
	Succeeded int    `json:"succeeded"`
	Total     int    `json:"total"`
}

// AgentRecentTaskDTO is one row of the Recent Tasks list on the agent
// dashboard.
type AgentRecentTaskDTO struct {
	TaskID       string `json:"task_id"`
	Identifier   string `json:"identifier"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	LastActiveAt string `json:"last_active_at"`
}

// AgentCostAggregateDTO is the all-time cost rollup for the agent.
// TotalCostSubcents stores hundredths of a cent (UI divides by 10000).
type AgentCostAggregateDTO struct {
	InputTokens       int64 `json:"input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
	CachedTokens      int64 `json:"cached_tokens"`
	TotalCostSubcents int64 `json:"total_cost_subcents"`
}

// AgentRunCostDTO is one row of the per-run costs table.
//
// CostSubcents is stored as int64 hundredths of a cent (matches
// CostEvent.CostSubcents and BudgetPolicy.LimitSubcents). The UI
// divides by 10000 to render dollars; the unit boundary lives in
// apps/web/lib/utils.ts:formatDollars. Never multiply or divide the
// raw value at call sites — call formatDollars(subcents) so the
// conversion stays in one place.
type AgentRunCostDTO struct {
	RunID        string `json:"run_id"`
	RunIDShort   string `json:"run_id_short"`
	Date         string `json:"date"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostSubcents int64  `json:"cost_subcents"`
}

// AgentSummaryRepository is the slim repository surface the
// agent-dashboard summary needs. It is a strict subset of the office
// repository, declared here so handler tests can provide a fake.
type AgentSummaryRepository interface {
	RunCountsByDayForAgent(ctx context.Context, agentID string, days int) ([]sqlite.AgentRunDayRow, error)
	TasksByPriorityByDayForAgent(ctx context.Context, agentID string, days int) ([]sqlite.AgentTaskPriorityDayRow, error)
	TasksByStatusByDayForAgent(ctx context.Context, agentID string, days int) ([]sqlite.AgentTaskStatusDayRow, error)
	RecentTasksForAgent(ctx context.Context, agentID string, limit int) ([]sqlite.AgentRecentTaskRow, error)
	CostAggregateForAgent(ctx context.Context, agentID string) (sqlite.AgentCostAggregate, error)
	RecentRunCostsForAgent(ctx context.Context, agentID string, limit int) ([]sqlite.AgentRunCostRow, error)
	LatestRunForAgent(ctx context.Context, agentID string) (*sqlite.RunSummaryRow, error)
}

// agentSummaryClampDays clamps the requested window into [1, 90]. The
// dashboard charts default to 14 days; values outside the safe range
// are silently clamped instead of erroring so a malformed query string
// still produces a useful response.
func agentSummaryClampDays(days int) int {
	if days <= 0 {
		return 14
	}
	if days > 90 {
		return 90
	}
	return days
}

// shortRunID returns the first 8 chars of a run id, or the whole
// string if shorter. The frontend uses this for compact display.
func shortRunID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// recentTaskCostsLimit is the number of rows for the per-run costs
// table on the agent dashboard. Frontend renders these in a fixed
// table; pagination is a follow-up.
const recentTaskCostsLimit = 10

// recentTasksLimit is the size of the "Recent Tasks" list.
const recentTasksLimit = 10

// GetAgentSummary builds the AgentDashboardSummary for an agent over
// the given window. Days outside [1, 90] are clamped; 0 maps to the
// default of 14.
//
// The implementation issues several short queries in sequence (latest
// run + four chart aggregates + recent tasks + cost aggregate +
// per-run cost rollup). Each query is independently indexed so the
// total cost stays under a couple of milliseconds for a typical
// agent.
func GetAgentSummary(
	ctx context.Context, repo AgentSummaryRepository, agentID string, days int,
) (*AgentDashboardSummary, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent id required")
	}
	days = agentSummaryClampDays(days)

	latest, err := repo.LatestRunForAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("latest run: %w", err)
	}

	runDays, err := repo.RunCountsByDayForAgent(ctx, agentID, days)
	if err != nil {
		return nil, fmt.Errorf("run counts: %w", err)
	}

	priorityDays, err := repo.TasksByPriorityByDayForAgent(ctx, agentID, days)
	if err != nil {
		return nil, fmt.Errorf("priority counts: %w", err)
	}

	statusDays, err := repo.TasksByStatusByDayForAgent(ctx, agentID, days)
	if err != nil {
		return nil, fmt.Errorf("status counts: %w", err)
	}

	recentTasks, err := repo.RecentTasksForAgent(ctx, agentID, recentTasksLimit)
	if err != nil {
		return nil, fmt.Errorf("recent tasks: %w", err)
	}

	cost, err := repo.CostAggregateForAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("cost aggregate: %w", err)
	}

	runCosts, err := repo.RecentRunCostsForAgent(ctx, agentID, recentTaskCostsLimit)
	if err != nil {
		return nil, fmt.Errorf("run costs: %w", err)
	}

	return assembleAgentSummary(agentID, days, latest,
		runDays, priorityDays, statusDays, recentTasks, cost, runCosts), nil
}

// assembleAgentSummary stitches the raw repository rows into the
// response shape, padding the chart series so every day in the window
// is represented (frontend renders zero bars rather than gaps).
func assembleAgentSummary(
	agentID string,
	days int,
	latest *sqlite.RunSummaryRow,
	runDays []sqlite.AgentRunDayRow,
	priorityDays []sqlite.AgentTaskPriorityDayRow,
	statusDays []sqlite.AgentTaskStatusDayRow,
	recentTasks []sqlite.AgentRecentTaskRow,
	cost sqlite.AgentCostAggregate,
	runCosts []sqlite.AgentRunCostRow,
) *AgentDashboardSummary {
	dates := buildDateWindow(days)
	return &AgentDashboardSummary{
		AgentID:         agentID,
		Days:            days,
		LatestRun:       buildLatestRunDTO(latest),
		RunActivity:     padAgentRunActivity(dates, runDays),
		TasksByPriority: padTasksByPriority(dates, priorityDays),
		TasksByStatus:   padTasksByStatus(dates, statusDays),
		SuccessRate:     buildSuccessRate(dates, runDays),
		RecentTasks:     buildRecentTasks(recentTasks),
		CostAggregate: AgentCostAggregateDTO{
			InputTokens:       cost.InputTokens,
			OutputTokens:      cost.OutputTokens,
			CachedTokens:      cost.CachedTokens,
			TotalCostSubcents: cost.TotalCostSubcents,
		},
		RecentRunCosts: buildRecentRunCosts(runCosts),
	}
}

// buildDateWindow returns the list of YYYY-MM-DD strings for the
// last `days` calendar days, oldest first. The series is computed in
// UTC so it matches the SQLite strftime('%Y-%m-%d', ts) buckets the
// repo helpers produce.
func buildDateWindow(days int) []string {
	dates := make([]string, days)
	now := time.Now().UTC()
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i))
		dates[i] = d.Format("2006-01-02")
	}
	return dates
}

func buildLatestRunDTO(run *sqlite.RunSummaryRow) *AgentLatestRunDTO {
	if run == nil {
		return nil
	}
	dto := &AgentLatestRunDTO{
		RunID:       run.ID,
		RunIDShort:  shortRunID(run.ID),
		Status:      run.Status,
		Reason:      run.Reason,
		RequestedAt: run.RequestedAt.UTC().Format(time.RFC3339),
	}
	if run.FinishedAt != nil {
		s := run.FinishedAt.UTC().Format(time.RFC3339)
		dto.FinishedAt = &s
	}
	dto.TaskID, dto.Summary = parseRunPayload(run.Payload)
	return dto
}

// parseRunPayload extracts a couple of well-known fields from the run
// payload JSON for display in the latest-run card. It is intentionally
// permissive: an unparseable payload returns empty strings so the UI
// just hides those fields.
func parseRunPayload(raw string) (taskID, summary string) {
	if raw == "" {
		return "", ""
	}
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return "", ""
	}
	if v, ok := data["task_id"].(string); ok {
		taskID = v
	}
	if v, ok := data["summary"].(string); ok {
		summary = v
	} else if v, ok := data["reply_to"].(string); ok {
		summary = v
	}
	return taskID, summary
}

func padAgentRunActivity(dates []string, rows []sqlite.AgentRunDayRow) []AgentRunActivityDay {
	byDate := make(map[string]sqlite.AgentRunDayRow, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r
	}
	out := make([]AgentRunActivityDay, len(dates))
	for i, date := range dates {
		row := byDate[date]
		out[i] = AgentRunActivityDay{
			Date:      date,
			Succeeded: row.Succeeded,
			Failed:    row.Failed,
			Other:     row.Other,
			Total:     row.Succeeded + row.Failed + row.Other,
		}
	}
	return out
}

func padTasksByPriority(dates []string, rows []sqlite.AgentTaskPriorityDayRow) []AgentTaskPriorityDay {
	byDate := make(map[string]sqlite.AgentTaskPriorityDayRow, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r
	}
	out := make([]AgentTaskPriorityDay, len(dates))
	for i, date := range dates {
		row := byDate[date]
		out[i] = AgentTaskPriorityDay{
			Date:     date,
			Critical: row.Critical,
			High:     row.High,
			Medium:   row.Medium,
			Low:      row.Low,
		}
	}
	return out
}

func padTasksByStatus(dates []string, rows []sqlite.AgentTaskStatusDayRow) []AgentTaskStatusDay {
	byDate := make(map[string]sqlite.AgentTaskStatusDayRow, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r
	}
	out := make([]AgentTaskStatusDay, len(dates))
	for i, date := range dates {
		row := byDate[date]
		out[i] = AgentTaskStatusDay{
			Date:       date,
			Todo:       row.Todo,
			InProgress: row.InProgress,
			InReview:   row.InReview,
			Done:       row.Done,
			Blocked:    row.Blocked,
			Cancelled:  row.Cancelled,
			Backlog:    row.Backlog,
		}
	}
	return out
}

func buildSuccessRate(dates []string, rows []sqlite.AgentRunDayRow) []AgentSuccessRateDay {
	byDate := make(map[string]sqlite.AgentRunDayRow, len(rows))
	for _, r := range rows {
		byDate[r.Date] = r
	}
	out := make([]AgentSuccessRateDay, len(dates))
	for i, date := range dates {
		row := byDate[date]
		out[i] = AgentSuccessRateDay{
			Date:      date,
			Succeeded: row.Succeeded,
			Total:     row.Succeeded + row.Failed + row.Other,
		}
	}
	return out
}

func buildRecentTasks(rows []sqlite.AgentRecentTaskRow) []AgentRecentTaskDTO {
	out := make([]AgentRecentTaskDTO, len(rows))
	for i, r := range rows {
		out[i] = AgentRecentTaskDTO{
			TaskID:       r.TaskID,
			Identifier:   r.Identifier,
			Title:        r.Title,
			Status:       statusFromState(r.State),
			LastActiveAt: r.LastActiveAt,
		}
	}
	return out
}

func buildRecentRunCosts(rows []sqlite.AgentRunCostRow) []AgentRunCostDTO {
	out := make([]AgentRunCostDTO, len(rows))
	for i, r := range rows {
		out[i] = AgentRunCostDTO{
			RunID:        r.RunID,
			RunIDShort:   shortRunID(r.RunID),
			Date:         shortDate(r.RequestedAt),
			InputTokens:  r.InputTokens,
			OutputTokens: r.OutputTokens,
			CostSubcents: r.CostSubcents,
		}
	}
	return out
}

// statusFromState normalises the DB state column (TODO, IN_PROGRESS,
// REVIEW, COMPLETED, BLOCKED, CANCELLED, BACKLOG) into the lowercase
// status the frontend uses. Unknown values fall back to "todo" so the
// UI always has a valid badge to render.
func statusFromState(state string) string {
	switch state {
	case "TODO":
		return statusTODOLowercase
	case "IN_PROGRESS":
		return "in_progress"
	case "REVIEW":
		return "in_review"
	case "COMPLETED":
		return "done"
	case "BLOCKED":
		return "blocked"
	case "CANCELLED":
		return statusCancelledLowercase
	case "BACKLOG":
		return "backlog"
	default:
		return statusTODOLowercase
	}
}

// shortDate truncates an SQLite-formatted timestamp like
// "2026-04-21 12:34:56" to the YYYY-MM-DD prefix. The repo helper
// returns whatever SQLite has stored (datetime / RFC3339 / etc.), so
// we slice the first 10 characters when they look like a date.
func shortDate(s string) string {
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return s
}
