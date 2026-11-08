package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/db"
)

// pluginChangeRequestTimeLayout matches the fixed-width UTC text the plugin
// change-request ledger stores, so window bounds compare lexically.
const pluginChangeRequestTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

type controlMergeAvailability struct{ github, plugin bool }

func (r *Repository) controlMergeAvailability(ctx context.Context) (controlMergeAvailability, error) {
	var available controlMergeAvailability
	for _, source := range []struct {
		table string
		dest  *bool
	}{{"github_task_prs", &available.github}, {"plugin_task_change_requests", &available.plugin}} {
		rows, err := r.ro.QueryContext(ctx, "SELECT 1 FROM "+source.table+" LIMIT 0")
		if err != nil {
			if db.IsMissingTableError(err) {
				continue
			}
			return available, err
		}
		_ = rows.Close()
		*source.dest = true
	}
	return available, nil
}

// controlMergeKey uses URL identity only when the URL itself names the
// reported number. Unknown repository aliases keep their
// provider/host/repository tuple.
func controlMergeKey(url, number, provider, fallback string) string {
	trimmed := "RTRIM(" + url + ",'/')"
	plain := "CASE WHEN " + trimmed + " LIKE 'https://%' THEN SUBSTR(" + trimmed + ",9) WHEN " + trimmed + " LIKE 'http://%' THEN SUBSTR(" + trimmed + ",8) ELSE " + trimmed + " END"
	plain = "CASE WHEN " + provider + "='github' THEN LOWER(" + plain + ") ELSE " + plain + " END"
	return "CASE WHEN " + trimmed + " LIKE '%/'||CAST(" + number + " AS TEXT) THEN 'url:'||" + plain + " ELSE " + fallback + " END"
}

func controlBuiltinMergeKey(alias string) string {
	fallback := "'github:github.com:'||LOWER(" + alias + ".owner||'/'||" + alias + ".repo)||':'||CAST(" + alias + ".pr_number AS TEXT)"
	return controlMergeKey("COALESCE("+alias+".pr_url,'')", alias+".pr_number", "'github'", fallback)
}

// controlPluginMergeGroup collapses repeated reports of one forge object by
// different visible tasks of the workspace into one merged observation.
const controlPluginMergeGroup = `SELECT p.workspace_id,p.provider_id,p.provider_host,p.repository_id,p.number,MAX(p.url) AS url
 FROM plugin_task_change_requests p JOIN tasks t ON t.id=p.task_id AND t.workspace_id=p.workspace_id
 WHERE ` + controlTaskVisibility + ` AND p.workspace_id=? AND p.state='merged' AND p.merged_at>=? AND p.merged_at<=?
 GROUP BY p.workspace_id,p.provider_id,p.provider_host,p.repository_id,p.number`

// controlMergeKeysQuery returns one merge_key row per merged observation in
// the window. A plugin report of a GitHub pull request the built-in
// integration also tracks takes the built-in identity and merge time, so the
// same merge is counted once.
func controlMergeKeysQuery() string {
	github := `SELECT ` + controlBuiltinMergeKey("g") + ` AS merge_key
 FROM github_task_prs g JOIN tasks t ON t.id=g.task_id AND t.workspace_id=g.workspace_id
 WHERE ` + controlTaskVisibility + ` AND g.workspace_id=? AND g.merged_at>=? AND g.merged_at<=?`
	pluginKey := controlMergeKey("p.url", "p.number", "p.provider_id", "p.provider_id||':'||p.provider_host||':'||p.repository_id||':'||CAST(p.number AS TEXT)")
	plugin := `SELECT CASE WHEN b.id IS NOT NULL THEN ` + controlBuiltinMergeKey("b") + ` ELSE ` + pluginKey + ` END AS merge_key
 FROM (` + controlPluginMergeGroup + `) p
 LEFT JOIN github_task_prs b ON b.workspace_id=p.workspace_id AND b.pr_number=p.number AND b.merged_at IS NOT NULL AND (
 (` + controlBuiltinMergeKey("b") + `=` + pluginKey + `) OR
 (p.provider_id='github' AND (p.repository_id=b.repository_id OR LOWER(p.repository_id)=LOWER(b.owner||'/'||b.repo)) AND (
 (p.provider_host IN ('','github.com') AND (LOWER(b.pr_url) LIKE 'https://github.com/%' OR COALESCE(b.pr_url,'')='')) OR
 (p.provider_host<>'' AND (LOWER(b.pr_url) LIKE 'https://'||p.provider_host||'/%' OR LOWER(b.pr_url) LIKE 'http://'||p.provider_host||'/%')))))
 WHERE b.id IS NULL OR (b.merged_at>=? AND b.merged_at<=?)`
	return strings.Join([]string{github, plugin}, " UNION ALL ")
}

// controlMergedCount counts change requests merged in the window across the
// built-in GitHub pull requests and plugin-reported change requests. Nil means
// either source is unavailable or unreadable, so the figure is unknown rather
// than zero.
func (r *Repository) controlMergedCount(ctx context.Context, ws string, since, now time.Time) *int {
	available, err := r.controlMergeAvailability(ctx)
	if err != nil || !available.github || !available.plugin {
		return nil
	}
	query := "SELECT COUNT(DISTINCT merge_key) FROM (" + controlMergeKeysQuery() + ") merges"
	sinceText := since.UTC().Format(pluginChangeRequestTimeLayout)
	nowText := now.UTC().Format(pluginChangeRequestTimeLayout)
	var count int
	if r.ro.GetContext(ctx, &count, r.ro.Rebind(query), ws, since, now, ws, sinceText, nowText, since, now) != nil {
		return nil
	}
	return &count
}
