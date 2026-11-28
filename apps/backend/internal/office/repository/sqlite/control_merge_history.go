package sqlite

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// controlMergeEventLimit bounds the events returned per merge family; the
// family total still counts every merge in the window.
const controlMergeEventLimit = 50

type controlMergeEventRow struct {
	ID         string `db:"id"`
	TaskID     string `db:"task_id"`
	Title      string `db:"title"`
	Provider   string `db:"provider"`
	Repository string `db:"repository"`
	Number     int64  `db:"number"`
	MergeKey   string `db:"merge_key"`
	MergedAt   any    `db:"merged_at"`
}

// controlGithubMergeEvents mirrors the built-in half of controlMergeKeysQuery.
func controlGithubMergeEvents() string {
	return `SELECT 'github:'||g.id AS id,g.task_id,g.pr_title AS title,'github' AS provider,
 g.owner||'/'||g.repo AS repository,g.pr_number AS number,` + controlBuiltinMergeKey("g") + ` AS merge_key,g.merged_at
 FROM github_task_prs g JOIN tasks t ON t.id=g.task_id AND t.workspace_id=g.workspace_id
 WHERE ` + controlTaskVisibility + ` AND g.workspace_id=? AND g.merged_at>=? AND g.merged_at<=?
 ORDER BY g.merged_at DESC,g.id ASC`
}

// controlPluginMergeEvents mirrors the plugin half of controlMergeKeysQuery,
// keeping only reports the built-in integration does not also track: a
// matched built-in pull request is that merge's identity, so it is either the
// GitHub event already returned or a merge outside the window.
func controlPluginMergeEvents(github bool) string {
	pluginKey := controlMergeKey("p.url", "p.number", "p.provider_id", "p.provider_id||':'||p.provider_host||':'||p.repository_id||':'||CAST(p.number AS TEXT)")
	grouped := `SELECT MIN(p.id) AS id,MIN(p.task_id) AS task_id,MAX(p.title) AS title,MIN(p.merged_at) AS merged_at,
 p.workspace_id,p.provider_id,p.provider_host,p.repository_id,p.number,MAX(p.url) AS url
 FROM plugin_task_change_requests p JOIN tasks t ON t.id=p.task_id AND t.workspace_id=p.workspace_id
 WHERE ` + controlTaskVisibility + ` AND p.workspace_id=? AND p.state='merged' AND p.merged_at>=? AND p.merged_at<=?
 GROUP BY p.workspace_id,p.provider_id,p.provider_host,p.repository_id,p.number`
	query := `SELECT 'plugin:'||p.id AS id,p.task_id,p.title,p.provider_id AS provider,
 CASE WHEN p.provider_host='' THEN p.repository_id ELSE p.provider_host||'/'||p.repository_id END AS repository,
 p.number,` + pluginKey + ` AS merge_key,p.merged_at FROM (` + grouped + `) p`
	if github {
		query += ` LEFT JOIN github_task_prs b ON b.workspace_id=p.workspace_id AND b.pr_number=p.number AND b.merged_at IS NOT NULL AND (
 (` + controlBuiltinMergeKey("b") + `=` + pluginKey + `) OR
 (p.provider_id='github' AND (p.repository_id=b.repository_id OR LOWER(p.repository_id)=LOWER(b.owner||'/'||b.repo)) AND (
 (p.provider_host IN ('','github.com') AND (LOWER(b.pr_url) LIKE 'https://github.com/%' OR COALESCE(b.pr_url,'')='')) OR
 (p.provider_host<>'' AND (LOWER(b.pr_url) LIKE 'https://'||p.provider_host||'/%' OR LOWER(b.pr_url) LIKE 'http://'||p.provider_host||'/%')))))
 WHERE b.id IS NULL`
	}
	return query + ` ORDER BY p.merged_at DESC,p.id ASC`
}

// appendControlMergeHistory adds merged change requests to the 24-hour history.
// A missing ledger stays unavailable rather than reading as no merges.
func (r *Repository) appendControlMergeHistory(ctx context.Context, out *ControlHistory) error {
	unavailable := ControlCoverage{State: "unavailable", Reason: "source_read_failed"}
	out.Sources["github_merge"], out.Sources["plugin_merge"] = unavailable, unavailable
	available, err := r.controlMergeAvailability(ctx)
	if err != nil {
		return err
	}
	since, now := out.Since, out.GeneratedAt
	if available.github {
		var rows []controlMergeEventRow
		if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(controlGithubMergeEvents()), out.WorkspaceID, since, now); err != nil {
			return err
		}
		if err := appendControlMergeFamily(out, "github_merge", rows); err != nil {
			return err
		}
	}
	if available.plugin {
		var rows []controlMergeEventRow
		sinceText := since.UTC().Format(pluginChangeRequestTimeLayout)
		nowText := now.UTC().Format(pluginChangeRequestTimeLayout)
		query := r.ro.Rebind(controlPluginMergeEvents(available.github))
		if err := r.ro.SelectContext(ctx, &rows, query, out.WorkspaceID, sinceText, nowText); err != nil {
			return err
		}
		if err := appendControlMergeFamily(out, "plugin_merge", rows); err != nil {
			return err
		}
	}
	return nil
}

// appendControlMergeFamily counts each merge identity once, keeps the latest
// events up to the family limit, and records the family's coverage.
func appendControlMergeFamily(out *ControlHistory, family string, rows []controlMergeEventRow) error {
	seen := make(map[string]bool, len(rows))
	events := make([]ControlEvent, 0, len(rows))
	for _, row := range rows {
		if seen[row.MergeKey] {
			continue
		}
		seen[row.MergeKey] = true
		at, err := controlMergeTime(row.MergedAt)
		if err != nil {
			return err
		}
		events = append(events, ControlEvent{ID: row.ID, Kind: family + "d", TaskID: row.TaskID, Title: row.Title,
			Provider: row.Provider, Repository: row.Repository, Number: row.Number, OccurredAt: at})
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].ID < events[j].ID
		}
		return events[i].OccurredAt.After(events[j].OccurredAt)
	})
	total := len(events)
	if len(events) > controlMergeEventLimit {
		events = events[:controlMergeEventLimit]
	}
	out.Sources[family] = ControlCoverage{State: "known", Total: &total, Returned: len(events), Truncated: total > len(events)}
	out.Events = append(out.Events, events...)
	return nil
}

// controlMergeTime reads a built-in DATETIME or the plugin ledger's fixed-width
// RFC3339 text. An unreadable value fails the family instead of vanishing.
func controlMergeTime(value any) (time.Time, error) {
	var parsed time.Time
	switch v := value.(type) {
	case time.Time:
		parsed = v
	case string:
		parsed = parseSqliteTime(v)
	case []byte:
		parsed = parseSqliteTime(string(v))
	}
	if parsed.IsZero() {
		return time.Time{}, fmt.Errorf("invalid merged change request timestamp")
	}
	return parsed.UTC(), nil
}
