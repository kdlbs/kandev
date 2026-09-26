package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

type sidebarBaseNeeds struct {
	state, activity, repository, executor      bool
	diff, pullRequest, reviewWatch, issueWatch bool
	workflowNames, summary                     bool
}

func sidebarBaseNeedsFor(query models.SidebarTaskViewQuery) sidebarBaseNeeds {
	needs := sidebarBaseNeeds{
		state:         query.Group == sidebarStateKey || query.Sort.Key == sidebarStateKey || sidebarQueryHasFilter(query, sidebarStateKey),
		activity:      query.Sort.Key == sidebarActivitySortField,
		repository:    query.Group == sidebarRepositoryKey || sidebarQueryHasFilter(query, sidebarRepositoryKey),
		executor:      query.Group == "executorType" || sidebarQueryHasFilter(query, "executorType"),
		diff:          sidebarQueryHasFilter(query, "hasDiff"),
		pullRequest:   sidebarQueryHasFilter(query, "hasPR"),
		reviewWatch:   sidebarQueryHasFilter(query, "isPRReview"),
		issueWatch:    sidebarQueryHasFilter(query, "isIssueWatch"),
		workflowNames: query.Group == sidebarWorkflowKey || query.Group == "workflowStep",
	}
	needs.summary = needs.state || needs.activity || needs.diff || needs.pullRequest
	return needs
}

func sidebarBaseCTE(driver, groupExpr, groupLabelExpr string, query models.SidebarTaskViewQuery) string {
	needs := sidebarBaseNeedsFor(query)
	summaryJoin, workflowJoins := sidebarBaseJoins(needs)
	candidateFields := sidebarBaseCandidateFields(driver, needs)
	return `WITH RECURSIVE candidate_raw AS (
		SELECT ` + strings.Join(candidateFields, ",\n\t\t\t") + `
		FROM tasks t
		` + workflowJoins + summaryJoin + `
		WHERE t.workspace_id = ? AND COALESCE(t.is_ephemeral, 0) = 0
			AND COALESCE(t.origin, '') <> 'automation_run'
			AND ` + excludeConfigModePredicate(driver, "t.metadata") + `
	), candidate AS (
		SELECT candidate_raw.*, ` + groupExpr + ` AS group_key, ` + groupLabelExpr + ` AS group_label
		FROM candidate_raw
	)`
}

func sidebarBaseJoins(needs sidebarBaseNeeds) (string, string) {
	summaryJoin, workflowJoins := "", ""
	if needs.summary {
		summaryJoin = ` LEFT JOIN task_status_summaries summary ON summary.task_id = t.id AND summary.workspace_id = t.workspace_id`
	}
	if needs.workflowNames {
		workflowJoins = ` LEFT JOIN workflows w ON w.id = t.workflow_id LEFT JOIN workflow_steps ws ON ws.id = t.workflow_step_id`
	}
	return summaryJoin, workflowJoins
}

func sidebarBaseCandidateFields(driver string, needs sidebarBaseNeeds) []string {
	fields := []string{"t.id", "t.workspace_id", "t.workflow_id", "t.workflow_step_id", "t.title", "t.parent_id", "t.archived_at", "t.created_at", "t.updated_at"}
	fields = append(fields, sidebarStateFields(driver, needs)...)
	fields = append(fields, sidebarActivityFields(driver, needs)...)
	fields = append(fields, sidebarWorkflowFields(needs)...)
	fields = append(fields, sidebarRepositoryFields(driver, needs)...)
	fields = append(fields, sidebarExecutorFields(needs)...)
	fields = append(fields, sidebarDiffFields(driver, needs)...)
	fields = append(fields, sidebarPullRequestFields(driver, needs)...)
	fields = append(fields, sidebarWatchFields(driver, needs)...)
	return fields
}

func sidebarStateFields(driver string, needs sidebarBaseNeeds) []string {
	if !needs.state {
		return nil
	}
	primary := `COALESCE(NULLIF(` + dialect.JSONExtractPath(driver, "summary.summary", "primary_session", sidebarStateKey) + `, ''), '')`
	bucket := `CASE
		WHEN COALESCE(` + primary + `, '') IN ('WAITING_FOR_INPUT', 'COMPLETED', 'FAILED', 'CANCELLED') THEN 'review'
		WHEN COALESCE(` + primary + `, '') = 'RUNNING' THEN 'in_progress'
		WHEN COALESCE(` + primary + `, '') IN ('CREATED', 'STARTING') THEN CASE
			WHEN t.state IN ('REVIEW', 'COMPLETED') THEN 'review'
			WHEN t.state IN ('IN_PROGRESS', 'SCHEDULING') THEN 'in_progress'
			ELSE 'backlog' END
		WHEN t.state IN ('REVIEW', 'COMPLETED') THEN 'review'
		WHEN t.state IN ('IN_PROGRESS', 'SCHEDULING') THEN 'in_progress'
		ELSE 'backlog' END`
	return []string{"t.state", primary + " AS primary_session_state", bucket + " AS state_bucket"}
}

func sidebarActivityFields(driver string, needs sidebarBaseNeeds) []string {
	if !needs.activity {
		return nil
	}
	value := `COALESCE(NULLIF(` + dialect.JSONExtract(driver, "summary.summary", "last_activity_at") + `, ''), CAST(t.updated_at AS TEXT), CAST(t.created_at AS TEXT))`
	return []string{sidebarActivitySortKey(driver, value) + " AS activity_at"}
}

func sidebarWorkflowFields(needs sidebarBaseNeeds) []string {
	if !needs.workflowNames {
		return nil
	}
	return []string{
		`COALESCE(NULLIF(w.name, ''), 'undefined') AS workflow_name`,
		`COALESCE(NULLIF(ws.name, ''), 'undefined') AS workflow_step_name`,
	}
}

func sidebarRepositoryFields(driver string, needs sidebarBaseNeeds) []string {
	if !needs.repository {
		return nil
	}
	repoName := `COALESCE((SELECT CASE
		WHEN COALESCE(r.provider_owner, '') <> '' AND COALESCE(r.provider_name, '') <> ''
		THEN r.provider_owner || '/' || r.provider_name
		ELSE r.name END
		FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = t.id ORDER BY tr.position ASC, tr.id ASC LIMIT 1), 'undefined')`
	repositoryCount := `(SELECT COUNT(DISTINCT tr.repository_id) FROM task_repositories tr WHERE tr.task_id = t.id)`
	resolvedRepositoryCount := `(SELECT COUNT(DISTINCT tr.repository_id) FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id WHERE tr.task_id = t.id)`
	repositorySlug := `CASE
		WHEN COALESCE(r.provider_owner, '') <> '' AND COALESCE(r.provider_name, '') <> ''
		THEN r.provider_owner || '/' || r.provider_name
		ELSE r.name END`
	repositorySlugs := `(SELECT json_group_array(repo_slug) FROM (
		SELECT ` + repositorySlug + ` AS repo_slug, MIN(tr.position) AS position, MIN(tr.id) AS id
		FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = t.id GROUP BY tr.repository_id, repo_slug ORDER BY position, id))`
	repositoryLabels := `(SELECT group_concat(repo_slug, ', ') FROM (
		SELECT ` + repositorySlug + ` AS repo_slug, MIN(tr.position) AS position, MIN(tr.id) AS id
		FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = t.id GROUP BY tr.repository_id, repo_slug ORDER BY position, id))`
	if dialect.IsPostgres(driver) {
		repositorySlugs = `(SELECT COALESCE(array_to_json(array_agg(repo_slug ORDER BY position, id))::text, '[]') FROM (
			SELECT ` + repositorySlug + ` AS repo_slug, MIN(tr.position) AS position, MIN(tr.id) AS id
			FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
			WHERE tr.task_id = t.id GROUP BY tr.repository_id, repo_slug) ordered_repositories)`
		repositoryLabels = `(SELECT string_agg(repo_slug, ', ' ORDER BY position, id) FROM (
			SELECT ` + repositorySlug + ` AS repo_slug, MIN(tr.position) AS position, MIN(tr.id) AS id
			FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
			WHERE tr.task_id = t.id GROUP BY tr.repository_id, repo_slug) ordered_repositories)`
	}
	return []string{repoName + " AS repository_name", repositoryCount + " AS repository_count",
		resolvedRepositoryCount + " AS resolved_repository_count", "COALESCE(" + repositorySlugs + ", '[]') AS repository_slugs",
		"COALESCE(" + repositoryLabels + ", 'undefined') AS repository_labels"}
}

func sidebarExecutorFields(needs sidebarBaseNeeds) []string {
	if !needs.executor {
		return nil
	}
	return []string{`COALESCE((SELECT e.type FROM task_sessions s JOIN executors e ON e.id = s.executor_id
		WHERE s.task_id = t.id ORDER BY s.is_primary DESC, s.updated_at DESC, s.id ASC LIMIT 1), 'undefined') AS executor_type`}
}

func sidebarDiffFields(driver string, needs sidebarBaseNeeds) []string {
	if !needs.diff {
		return nil
	}
	return []string{`CASE WHEN CAST(COALESCE(` + dialect.JSONExtractPath(driver, "summary.summary", "git", "additions") + `, '0') AS INTEGER) > 0
		OR CAST(COALESCE(` + dialect.JSONExtractPath(driver, "summary.summary", "git", "deletions") + `, '0') AS INTEGER) > 0 THEN 'true' ELSE 'false' END AS has_diff`}
}

func sidebarPullRequestFields(driver string, needs sidebarBaseNeeds) []string {
	if !needs.pullRequest {
		return nil
	}
	return []string{`CASE WHEN CAST(COALESCE(` + dialect.JSONExtractPath(driver, "summary.summary", "pull_request", "count") + `, '0') AS INTEGER) > 0
		OR COALESCE(` + dialect.JSONExtractPath(driver, "summary.summary", "pull_request", "url") + `, '') <> '' THEN 'true' ELSE 'false' END AS has_pr`}
}

func sidebarWatchFields(driver string, needs sidebarBaseNeeds) []string {
	fields := make([]string, 0, 2)
	if needs.reviewWatch {
		fields = append(fields, sidebarWatchField(driver, "review_watch_id", "is_pr_review"))
	}
	if needs.issueWatch {
		fields = append(fields, sidebarWatchField(driver, "issue_watch_id", "is_issue_watch"))
	}
	return fields
}

func sidebarWatchField(driver, metadataKey, alias string) string {
	value := dialect.JSONExtract(driver, "t.metadata", metadataKey)
	return "CASE WHEN COALESCE(NULLIF(" + value + ", ''), 'undefined') <> 'undefined' THEN 'true' ELSE 'false' END AS " + alias
}

// Fixed-width UTC keys make lexical MAX and ORDER BY compare valid instants consistently.
func sidebarActivitySortKey(driver, value string) string {
	if dialect.IsPostgres(driver) {
		validRFC3339 := `^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])([T]([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]([.][0-9]{1,9}){0,1}(Z|([+-]([01][0-9]|2[0-3]):[0-5][0-9]))| ([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9]([.][0-9]{1,9}){0,1}(Z|([+-]([01][0-9]|2[0-3])(:[0-5][0-9]){0,1})))$`
		utcSecond := `TO_CHAR(DATE_TRUNC('second', CAST((` + value + `) AS TIMESTAMPTZ)) AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS')`
		fraction := `RPAD(COALESCE(SUBSTRING((` + value + `) FROM '[.]([0-9]{1,9})'), ''), 9, '0')`
		year := `CAST(SUBSTRING((` + value + `), 1, 4) AS INTEGER)`
		month := `CAST(SUBSTRING((` + value + `), 6, 2) AS INTEGER)`
		day := `CAST(SUBSTRING((` + value + `), 9, 2) AS INTEGER)`
		maxDay := `CASE ` + month + ` WHEN 2 THEN CASE WHEN (` + year + ` % 4 = 0 AND ` + year + ` % 100 <> 0) OR ` + year + ` % 400 = 0 THEN 29 ELSE 28 END
			WHEN 4 THEN 30 WHEN 6 THEN 30 WHEN 9 THEN 30 WHEN 11 THEN 30 ELSE 31 END`
		return `CASE WHEN (` + value + `) ~ '` + validRFC3339 + `' AND SUBSTRING((` + value + `), 1, 4) <> '0000' THEN
			CASE WHEN ` + day + ` <= ` + maxDay + `
				THEN ` + utcSecond + ` || '.' || ` + fraction + ` ELSE (` + value + `) END
			ELSE (` + value + `) END`
	}
	tail := `SUBSTR((` + value + `), INSTR((` + value + `), '.') + 1)`
	fractionEnd := `CASE WHEN INSTR(` + tail + `, 'Z') > 0 THEN INSTR(` + tail + `, 'Z') - 1
		WHEN INSTR(` + tail + `, '+') > 0 THEN INSTR(` + tail + `, '+') - 1
		WHEN INSTR(` + tail + `, '-') > 0 THEN INSTR(` + tail + `, '-') - 1 ELSE LENGTH(` + tail + `) END`
	rawFraction := `SUBSTR(` + tail + `, 1, ` + fractionEnd + `)`
	fraction := `CASE WHEN INSTR((` + value + `), '.') > 0 THEN ` + rawFraction + ` ELSE '' END`
	normalizedFraction := `SUBSTR((` + fraction + `) || '000000000', 1, 9)`
	zoneStart := `CASE WHEN INSTR((` + value + `), '.') > 0 THEN INSTR((` + value + `), '.') + LENGTH(` + fraction + `) + 1 ELSE 20 END`
	zone := `SUBSTR((` + value + `), ` + zoneStart + `)`
	utcSecond := `STRFTIME('%Y-%m-%dT%H:%M:%S', (` + value + `))`
	validCalendarDate := `DATE(SUBSTR((` + value + `), 1, 10), '+0 days') = SUBSTR((` + value + `), 1, 10)`
	validClock := `SUBSTR((` + value + `), 1, 4) GLOB '[0-9][0-9][0-9][0-9]'
		AND SUBSTR((` + value + `), 6, 2) BETWEEN '01' AND '12'
		AND SUBSTR((` + value + `), 9, 2) BETWEEN '01' AND '31'
		AND SUBSTR((` + value + `), 11, 1) IN ('T', ' ')
		AND SUBSTR((` + value + `), 12, 2) BETWEEN '00' AND '23'
		AND SUBSTR((` + value + `), 14, 1) = ':'
		AND SUBSTR((` + value + `), 15, 2) BETWEEN '00' AND '59'
		AND SUBSTR((` + value + `), 17, 1) = ':'
		AND SUBSTR((` + value + `), 18, 2) BETWEEN '00' AND '59'`
	validFraction := `(INSTR((` + value + `), '.') = 0 OR (LENGTH(` + fraction + `) BETWEEN 1 AND 9 AND (` + fraction + `) NOT GLOB '*[^0-9]*'))`
	validZone := `( (` + zone + `) = 'Z'
		OR (LENGTH(` + zone + `) = 6 AND SUBSTR(` + zone + `, 1, 1) IN ('+', '-')
			AND SUBSTR(` + zone + `, 2, 2) BETWEEN '00' AND '23' AND SUBSTR(` + zone + `, 4, 1) = ':'
			AND SUBSTR(` + zone + `, 5, 2) BETWEEN '00' AND '59')
		OR (SUBSTR((` + value + `), 11, 1) = ' ' AND LENGTH(` + zone + `) = 3 AND SUBSTR(` + zone + `, 1, 1) IN ('+', '-')
			AND SUBSTR(` + zone + `, 2, 2) BETWEEN '00' AND '23'))`
	return `CASE WHEN ` + utcSecond + ` IS NOT NULL AND ` + validCalendarDate + ` AND ` + validClock + `
		AND ` + validFraction + ` AND ` + validZone + `
		THEN ` + utcSecond + ` || '.' || ` + normalizedFraction + ` ELSE (` + value + `) END`
}

func sidebarQueryHasFilter(query models.SidebarTaskViewQuery, dimension string) bool {
	for _, filter := range query.Filters {
		if filter.Dimension == dimension {
			return true
		}
	}
	return false
}

func sidebarGroupExpressions(group string) (string, string) {
	switch group {
	case sidebarWorkflowKey:
		return `COALESCE(NULLIF(workflow_id, ''), '__unassigned__')`, `workflow_name`
	case "workflowStep":
		return `COALESCE(NULLIF(workflow_step_id, ''), '__unassigned__')`, `workflow_step_name`
	case "executorType":
		return `COALESCE(NULLIF(executor_type, 'undefined'), '__unassigned__')`, `executor_type`
	case sidebarStateKey:
		return `COALESCE(NULLIF(state, ''), '__not_started__')`, `COALESCE(NULLIF(state, ''), '__not_started__')`
	case sidebarRepositoryKey:
		return `CASE
			WHEN repository_count = 0 THEN '__unassigned__'
			WHEN repository_count = 1 THEN COALESCE(NULLIF(repository_name, 'undefined'), '__unassigned__')
			WHEN repository_count = resolved_repository_count THEN '__repo_combination__:' || repository_slugs
			ELSE '__multi__' END`, `CASE
			WHEN repository_count = 0 THEN '__unassigned__'
			WHEN repository_count = 1 THEN COALESCE(NULLIF(repository_name, 'undefined'), '__unassigned__')
			WHEN repository_count = resolved_repository_count THEN repository_labels
			ELSE '__multi__' END`
	default:
		return `'__all__'`, `'__all__'`
	}
}

func sidebarVisibleCTE(query models.SidebarTaskViewQuery) (string, []any) {
	var ctes []string
	var args []any
	if len(query.CollapsedTaskIDs) > 0 {
		roots := make([]string, len(query.CollapsedTaskIDs))
		for i, id := range query.CollapsedTaskIDs {
			roots[i] = "SELECT ? AS id"
			args = append(args, id)
		}
		ctes = append(ctes, "collapsed_requested(id) AS ("+strings.Join(roots, " UNION ALL ")+")")
		ctes = append(ctes, `collapsed_roots(id) AS (
			SELECT filtered.id FROM filtered JOIN collapsed_requested requested ON requested.id = filtered.id
		)`)
		ctes = append(ctes, `hidden_tasks(id) AS (
			SELECT child.id FROM filtered child JOIN collapsed_roots root ON child.parent_id = root.id
			UNION
			SELECT child.id FROM filtered child JOIN hidden_tasks hidden ON child.parent_id = hidden.id
		)`)
	}
	visible := `visible AS MATERIALIZED (SELECT * FROM filtered WHERE 1=1`
	if len(query.CollapsedTaskIDs) > 0 {
		visible += ` AND id NOT IN (SELECT id FROM hidden_tasks)`
	}
	visible += ")"
	ctes = append(ctes, visible)
	return ", " + strings.Join(ctes, ", "), args
}

func sidebarFilterSQL(driver string, filters []models.SidebarTaskViewClause) (string, []any, error) {
	parts := []string{"1=1"}
	args := make([]any, 0, len(filters)*2)
	hasArchivedFilter := false
	for _, filter := range filters {
		if filter.Dimension == sidebarArchivedKey {
			hasArchivedFilter = true
		}
		actual, ok := sidebarFilterColumn(driver, filter.Dimension)
		if !ok {
			return "", nil, fmt.Errorf("unsupported sidebar filter dimension %q", filter.Dimension)
		}
		condition, values, err := sidebarClauseCondition(actual, filter)
		if err != nil {
			return "", nil, err
		}
		parts = append(parts, condition)
		args = append(args, values...)
	}
	if !hasArchivedFilter {
		parts = append(parts, "archived_at IS NULL")
	}
	return strings.Join(parts, " AND "), args, nil
}

func sidebarFilterColumn(driver, dimension string) (string, bool) {
	columns := map[string]string{
		sidebarArchivedKey:   `CASE WHEN archived_at IS NULL THEN 'false' ELSE 'true' END`,
		sidebarStateKey:      `state_bucket`,
		sidebarWorkflowKey:   `COALESCE(NULLIF(workflow_id, ''), 'undefined')`,
		"workflowStep":       `COALESCE(NULLIF(workflow_step_id, ''), 'undefined')`,
		"executorType":       `executor_type`,
		sidebarRepositoryKey: `repository_name`,
		"hasDiff":            `has_diff`,
		"hasPR":              `has_pr`,
		"isPRReview":         `is_pr_review`,
		"isIssueWatch":       `is_issue_watch`,
		"titleMatch":         `COALESCE(title, '')`,
	}
	value, ok := columns[dimension]
	return value, ok
}

func sidebarClauseCondition(actual string, clause models.SidebarTaskViewClause) (string, []any, error) {
	if clause.Op == "matches" || clause.Op == sidebarNotMatchesOp {
		var value string
		if err := json.Unmarshal(clause.Value, &value); err != nil {
			return "", nil, err
		}
		if value == "" && clause.Op == sidebarNotMatchesOp {
			return "0=1", nil, nil
		}
		pattern := "%" + escapeLike(value) + "%"
		expr := "LOWER(" + actual + ") LIKE LOWER(?) ESCAPE '\\'"
		if clause.Op == sidebarNotMatchesOp {
			expr = "LOWER(" + actual + ") NOT LIKE LOWER(?) ESCAPE '\\'"
		}
		return expr, []any{pattern}, nil
	}
	values, err := sidebarClauseValues(clause)
	if err != nil {
		return "", nil, err
	}
	if clause.Op == "in" || clause.Op == sidebarNotInOp {
		if len(values) == 0 {
			if clause.Op == "in" {
				return "0=1", nil, nil
			}
			return "1=1", nil, nil
		}
		operator := "IN"
		if clause.Op == sidebarNotInOp {
			operator = "NOT IN"
		}
		return actual + " " + operator + " (" + placeholders(len(values)) + ")", values, nil
	}
	operator := "="
	if clause.Op == "is_not" {
		operator = "<>"
	}
	return actual + " " + operator + " ?", values, nil
}

func sidebarClauseValues(clause models.SidebarTaskViewClause) ([]any, error) {
	var values []json.RawMessage
	if clause.Op == "in" || clause.Op == sidebarNotInOp {
		if err := json.Unmarshal(clause.Value, &values); err != nil {
			return nil, err
		}
	} else {
		values = []json.RawMessage{clause.Value}
	}
	result := make([]any, 0, len(values))
	for _, raw := range values {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			result = append(result, text)
			continue
		}
		var boolean bool
		if err := json.Unmarshal(raw, &boolean); err == nil {
			result = append(result, fmt.Sprintf("%t", boolean))
			continue
		}
		return nil, errors.New("unsupported sidebar filter value")
	}
	return result, nil
}

func sidebarTotalGroupCountSQL(group string) string {
	if group == sidebarGroupNone {
		return `(SELECT CASE WHEN COUNT(*) > 0 THEN 1 ELSE 0 END FROM filtered)`
	}
	return `(SELECT COUNT(*) FROM ordered_groups)`
}

func sidebarPageSelectSQL(groupNone bool) string {
	groupCountExpr := `COALESCE(group_count.task_count, 0)`
	groupCountJoin := `LEFT JOIN group_task_counts group_count ON group_count.group_key = tree.root_group_key
		AND group_count.group_label = tree.root_group_label`
	if groupNone {
		groupCountExpr = `(SELECT COUNT(*) FROM visible)`
		groupCountJoin = ""
	}
	return ` SELECT tree.id, tree.root_group_key, tree.root_group_label,
		COALESCE(NULLIF(w.name, ''), 'undefined'), COALESCE(NULLIF(ws.name, ''), 'undefined'), COALESCE(ws.color, ''),
		COALESCE(tree.parent_id, ''), COALESCE(parent.title, ''),
		` + groupCountExpr + `, tree.depth, tree.group_position,
		COALESCE(queue_status.queue_position, 0), COALESCE(queue_status.queue_total, 0),
		page_summary.total_tasks, page_summary.total_visible_tasks, page_summary.total_groups, page_options.page
	FROM page_window page
	JOIN page_ordered_tree tree ON tree.id = page.id
	CROSS JOIN page_summary
	CROSS JOIN page_options
	LEFT JOIN tasks parent ON parent.id = tree.parent_id
	LEFT JOIN workflows w ON w.id = tree.workflow_id
	LEFT JOIN workflow_steps ws ON ws.id = tree.workflow_step_id
	` + groupCountJoin + `
	LEFT JOIN wip_queue_ranked queue_status ON queue_status.id = tree.id
	ORDER BY tree.group_order ASC, tree.order_path ASC`
}

func sidebarGroupHeaderSelectSQL(groupKeys []string) string {
	return ` SELECT groups.group_key, groups.group_label, COALESCE(counts.task_count, 0)
	FROM ordered_groups groups
	LEFT JOIN group_task_counts counts ON counts.group_key = groups.group_key
		AND counts.group_label = groups.group_label
	WHERE groups.group_key IN (` + placeholders(len(groupKeys)) + `)
	ORDER BY groups.group_order ASC`
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	return strings.ReplaceAll(value, `_`, `\_`)
}
