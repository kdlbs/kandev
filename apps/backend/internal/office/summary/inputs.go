package summary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// Repo is the minimal repository surface LoadInputs needs. Mirrored
// from the dashboard package's RunDetailRepo pattern — keeps the
// summary package out of the larger Repository interface and lets
// tests stub the dependencies.
type Repo interface {
	GetContinuationSummary(
		ctx context.Context, agentProfileID, scope string,
	) (*sqlite.AgentContinuationSummary, error)
}

// LoadInputs assembles a BuildInputs from the database for a freshly
// completed taskless run. Caller passes the run row (we read its
// result_json + workspace via the agent profile join), the agent
// profile id, and the scope key (e.g. "heartbeat" or "routine:<id>").
//
// Best-effort: any sub-query that fails is logged-by-virtue-of being
// silently dropped, so a partial summary always wins over no summary.
// The caller is responsible for treating BuildInputs.PreviousSummary
// as the safety net.
//
// The repo field is the writer/reader db handle pair; we pass them
// in directly because a few queries (workspace_id resolution, the
// blocked-tasks list) don't have first-class repo methods today.
//
// V1 LIMITATION: blockers are scoped to the agent's workspace as a
// whole, not "agents this CEO manages". Office today has no first-
// class CEO ↔ direct-report relation; widening to workspace is the
// intentional v1 fallback. When that relation lands we tighten the
// query.
func LoadInputs(
	ctx context.Context,
	repo Repo,
	reader *sqlx.DB,
	run RunSnapshot,
	agentProfileID, scope string,
) (BuildInputs, error) {
	in := BuildInputs{
		AgentProfileID: agentProfileID,
		Scope:          scope,
		LastRunStatus:  run.Status,
	}

	parsed := parseResultJSON(run.ResultJSON)
	in.ActiveFocus = parsed.ActiveFocus
	in.NextAction = parsed.NextAction
	in.Decisions = parsed.Decisions

	prior := loadPriorSummary(ctx, repo, agentProfileID, scope)
	if prior != nil {
		in.PreviousSummary = prior.Content
	}

	workspaceID := loadWorkspaceID(ctx, reader, agentProfileID)
	if workspaceID != "" {
		in.Activity = loadActivityStats(ctx, reader, workspaceID, prior)
		in.Blockers = loadBlockers(ctx, reader, workspaceID)
	}

	return in, nil
}

// RunSnapshot is the slim subset of models.Run that LoadInputs needs.
// Callers usually already hold the full row; passing only the fields
// we read keeps this package's API surface small.
type RunSnapshot struct {
	ID         string
	Status     string
	ResultJSON string
}

// parsedResult is the structured shape we expect inside
// runs.result_json. Adapters populate (or don't populate) these
// fields based on the agent CLI's final-result schema. We look at
// .summary first, then fall back to .result, .message, .error
// for a synthetic ActiveFocus.
type parsedResult struct {
	ActiveFocus string
	NextAction  string
	Decisions   []DecisionInput
}

func parseResultJSON(raw string) parsedResult {
	out := parsedResult{}
	if raw == "" || raw == "{}" {
		return out
	}
	var probe struct {
		Summary    string `json:"summary"`
		Result     string `json:"result"`
		Message    string `json:"message"`
		Error      string `json:"error"`
		NextAction string `json:"next_action"`
		Decisions  []struct {
			Text string    `json:"text"`
			At   time.Time `json:"at"`
		} `json:"decisions"`
	}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return out
	}
	out.ActiveFocus = firstNonEmpty(probe.Summary, probe.Result, probe.Message, probe.Error)
	out.NextAction = probe.NextAction
	for _, d := range probe.Decisions {
		if d.Text == "" {
			continue
		}
		out.Decisions = append(out.Decisions, DecisionInput{Text: d.Text, At: d.At})
	}
	return out
}

func loadPriorSummary(
	ctx context.Context, repo Repo, agentProfileID, scope string,
) *sqlite.AgentContinuationSummary {
	prior, err := repo.GetContinuationSummary(ctx, agentProfileID, scope)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return nil
	}
	return prior
}

func loadWorkspaceID(ctx context.Context, reader *sqlx.DB, agentProfileID string) string {
	if reader == nil || agentProfileID == "" {
		return ""
	}
	var workspaceID string
	err := reader.QueryRowxContext(ctx, reader.Rebind(`
		SELECT COALESCE(workspace_id, '') FROM agent_profiles WHERE id = ?
	`), agentProfileID).Scan(&workspaceID)
	if err != nil {
		return ""
	}
	return workspaceID
}

// loadActivityStats counts run + task outcomes for the workspace
// since the prior summary's updated_at. When no prior summary
// exists the cutoff falls back to "last 24h" so we still produce
// useful activity counts on first fire.
func loadActivityStats(
	ctx context.Context,
	reader *sqlx.DB,
	workspaceID string,
	prior *sqlite.AgentContinuationSummary,
) ActivityStats {
	stats := ActivityStats{}
	if reader == nil {
		return stats
	}
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	if prior != nil && !prior.UpdatedAt.IsZero() {
		cutoff = prior.UpdatedAt.UTC()
	}

	_ = reader.QueryRowxContext(ctx, reader.Rebind(`
		SELECT
			SUM(CASE WHEN r.status = 'finished' THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN r.status = 'failed'   THEN 1 ELSE 0 END) AS failed
		FROM runs r
		JOIN agent_profiles a ON a.id = r.agent_profile_id
		WHERE a.workspace_id = ?
		  AND r.finished_at IS NOT NULL
		  AND r.finished_at >= ?
	`), workspaceID, cutoff).Scan(&stats.CompletedRuns, &stats.FailedRuns)

	_ = reader.QueryRowxContext(ctx, reader.Rebind(`
		SELECT
			SUM(CASE WHEN state = 'IN_PROGRESS' THEN 1 ELSE 0 END) AS in_progress,
			SUM(CASE WHEN state IN ('TODO','BACKLOG','REVIEW') THEN 1 ELSE 0 END) AS open_tasks
		FROM tasks
		WHERE workspace_id = ?
		  AND archived_at IS NULL
		  AND is_ephemeral = 0
	`), workspaceID).Scan(&stats.InProgress, &stats.OpenTasks)

	return stats
}

// loadBlockers returns up to 10 non-archived BLOCKED tasks in the
// agent's workspace. Today we surface every blocker in the workspace
// (see V1 LIMITATION on LoadInputs); tightening to "agents this CEO
// manages" is a follow-up once that relation exists.
func loadBlockers(ctx context.Context, reader *sqlx.DB, workspaceID string) []BlockerInput {
	if reader == nil {
		return nil
	}
	rows, err := reader.QueryxContext(ctx, reader.Rebind(`
		SELECT title, COALESCE(description, '') AS description, created_at
		FROM tasks
		WHERE workspace_id = ?
		  AND state = 'BLOCKED'
		  AND archived_at IS NULL
		  AND is_ephemeral = 0
		ORDER BY updated_at DESC
		LIMIT 10
	`), workspaceID)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var out []BlockerInput
	for rows.Next() {
		var (
			title, desc string
			createdAt   time.Time
		)
		if err := rows.Scan(&title, &desc, &createdAt); err != nil {
			continue
		}
		out = append(out, BlockerInput{
			Title:      title,
			Reason:     truncateLine(desc, 120),
			SurfacedAt: createdAt,
		})
	}
	return out
}

func truncateLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
