package sqlite

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func sidebarPageCTEs(driver string, query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) (string, []any) {
	page := sidebarPageBuildContextFor(driver, query, prefs)
	groupCTEs, treeRootSQL := sidebarPageGroupExpressions(query, page)
	ctes := sidebarPageTreeCTEs(page) + groupCTEs
	ctes += sidebarRootWindowCTEs(driver, query, treeRootSQL, &page)
	page.args = append(page.args, query.PageSize, query.Page)
	ctes += sidebarSelectedTreeCTEs(query, page)
	page.args = append(page.args, page.childArgs...)
	ctes += sidebarPageResultCTEs(query.Group)
	ctes += sidebarPageQueueCTEs(driver, page)
	return ctes, page.args
}

func sidebarPageQueueCTEs(driver string, page sidebarPageBuildContext) string {
	return `, page_queue_steps AS MATERIALIZED (
		SELECT DISTINCT task.workspace_id, task.queued_for_step_id
		FROM page_window page JOIN tasks task ON task.id = page.id
		WHERE task.queued_for_step_id <> ''
	), wip_queue_ranked AS (
		SELECT queue_task.id,
			ROW_NUMBER() OVER (PARTITION BY queue_task.queued_for_step_id ORDER BY
				queue_task.position ASC,
				CASE LOWER(COALESCE(queue_task.priority, ''))
					WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2
					WHEN 'low' THEN 3 WHEN 'none' THEN 4 ELSE 4 END ASC,
				COALESCE(queue_task.queued_at, queue_task.created_at) ASC,
				queue_task.created_at ASC, queue_task.id ASC) AS queue_position,
			COUNT(*) OVER (PARTITION BY queue_task.queued_for_step_id) AS queue_total
		FROM page_queue_steps
		CROSS JOIN tasks queue_task
		WHERE queue_task.archived_at IS NULL
			AND page_queue_steps.workspace_id = queue_task.workspace_id
			AND COALESCE(queue_task.agent_project_id, '') = ''
			AND page_queue_steps.queued_for_step_id = queue_task.queued_for_step_id
			AND queue_task.workflow_step_id = queue_task.queued_for_step_id
			AND queue_task.queued_for_step_id <> ''
			AND ` + page.wipAdmittedFalse + `
			AND COALESCE(queue_task.is_ephemeral, 0) = 0
			AND COALESCE(queue_task.origin, '') <> 'automation_run'
			AND ` + excludeConfigModePredicate(driver, "queue_task.metadata") + `
	)`
}

func sidebarPageGroupExpressions(query models.SidebarTaskViewQuery, page sidebarPageBuildContext) (string, string) {
	groupCTEs := `, root_groups_raw AS (
			SELECT root.task_group_key AS group_key, root.task_group_label AS group_label, COUNT(*) AS group_count,
				MIN(root.global_root_sort_order) AS first_order
			FROM ranked root WHERE root.display_parent_id IS NULL
			GROUP BY root.task_group_key, root.task_group_label
		), repository_group_meta AS (
			SELECT COUNT(*) AS named_group_count, MIN(group_key) AS named_group_key,
				MIN(group_label) AS named_group_label
			FROM root_groups_raw
			WHERE group_key NOT IN ('__multi__', '__unassigned__')
				AND SUBSTR(group_key, 1, 21) <> '__repo_combination__:'
	), root_group_identity AS (
			SELECT raw.*,
				CASE WHEN '` + query.Group + `' = 'repository' AND raw.group_key = '__unassigned__'
					AND meta.named_group_count = 1 THEN meta.named_group_key ELSE raw.group_key END AS display_group_key,
				CASE WHEN '` + query.Group + `' = 'repository' AND raw.group_key = '__unassigned__'
					AND meta.named_group_count = 1 THEN meta.named_group_label ELSE raw.group_label END AS display_group_label
			FROM root_groups_raw raw CROSS JOIN repository_group_meta meta
		), root_display_order AS (
			SELECT root.*,
				ROW_NUMBER() OVER (PARTITION BY identity.display_group_key, identity.display_group_label ORDER BY
					CASE WHEN root.root_pin_order IS NULL THEN 1 ELSE 0 END,
					root.root_pin_order, root.global_root_sort_order, root.id) AS display_root_order
			FROM ranked root JOIN root_group_identity identity
				ON identity.group_key = root.task_group_key AND identity.group_label = root.task_group_label
			WHERE root.display_parent_id IS NULL
		), root_groups AS (
			SELECT display_group_key AS group_key, display_group_label AS group_label,
				SUM(group_count) AS group_count, MIN(first_order) AS first_order
			FROM root_group_identity
			GROUP BY display_group_key, display_group_label
		), ordered_groups AS (
			SELECT root_groups.*, ROW_NUMBER() OVER (ORDER BY ` + page.groupOrder + `) AS group_order
			FROM root_groups
		)`
	if query.Group == sidebarGroupNone {
		groupCTEs = `, root_group_identity AS (
			SELECT '__all__' AS group_key, '__all__' AS group_label,
				'__all__' AS display_group_key, '__all__' AS display_group_label,
				COUNT(*) AS group_count, MIN(root.root_sort_order) AS first_order
			FROM ranked root WHERE root.display_parent_id IS NULL
			HAVING COUNT(*) > 0
		), root_display_order AS (
			SELECT root.*, root.sibling_order AS display_root_order
			FROM ranked root WHERE root.display_parent_id IS NULL
		), ordered_groups AS (
			SELECT display_group_key AS group_key, display_group_label AS group_label,
				group_count, first_order, 1 AS group_order
			FROM root_group_identity
		)`
	}
	treeRootSQL := `SELECT root.id, root.id AS root_id, identity.display_group_key AS root_group_key,
			identity.display_group_label AS root_group_label, groups.group_order,
			root.workflow_id, root.workflow_step_id,
			root.display_parent_id AS parent_id, 0 AS depth, ` + page.rootPathPart + ` AS order_path
		FROM root_display_order root
		JOIN root_group_identity identity ON identity.group_key = root.task_group_key
			AND identity.group_label = root.task_group_label
		JOIN ordered_groups groups ON groups.group_key = identity.display_group_key
			AND groups.group_label = identity.display_group_label
		WHERE root.display_parent_id IS NULL`
	if query.Group == sidebarGroupNone {
		treeRootSQL = `SELECT root.id, root.id AS root_id, root.task_group_key AS root_group_key,
			root.task_group_label AS root_group_label, 1 AS group_order,
			root.workflow_id, root.workflow_step_id,
			root.display_parent_id AS parent_id, 0 AS depth, ` + page.rootPathPart + ` AS order_path
		FROM root_display_order root
		WHERE root.display_parent_id IS NULL`
	}
	return groupCTEs, treeRootSQL
}

func sidebarPageTreeCTEs(page sidebarPageBuildContext) string {
	//nolint:dupword // SQL CTE keys follow the task identifier schema.
	cycleCTEs := `, cycle_probe(start_key, current_key, parent_key, visited, closed_by_key, min_key) AS (
			SELECT id, id, parent_id, '/' || id || '/', NULL, id
			FROM filtered
			WHERE parent_id >= id
			UNION ALL
			SELECT cycle_probe.start_key, parent.id, parent.parent_id, cycle_probe.visited || parent.id || '/',
				CASE WHEN ` + page.cycleProbeGuard + ` THEN NULL ELSE parent.id END,
				CASE WHEN parent.id < cycle_probe.min_key THEN parent.id ELSE cycle_probe.min_key END
			FROM cycle_probe
			JOIN filtered parent ON parent.id = cycle_probe.parent_key
			WHERE cycle_probe.closed_by_key IS NULL
		), cycle_roots(root_key) AS (
			SELECT DISTINCT min_key FROM cycle_probe WHERE closed_by_key = start_key
		)`
	return cycleCTEs + sidebarRootMembershipCTEs() + page.repositoryCTEs + page.ancestorCTEs + page.stateCTEs + page.activityCTEs + page.runningCTEs + `, ranked_ordered AS (
		SELECT v.id, v.workflow_id, v.workflow_step_id, CAST(NULL AS TEXT) AS display_parent_id,
			` + page.groupKey + ` AS task_group_key, ` + page.groupLabel + ` AS task_group_label,
			ROW_NUMBER() OVER (ORDER BY ` + page.rootOrder + `) AS global_root_sort_order,
			` + page.rootPinExpr + ` AS root_pin_order
		FROM display_roots v
		` + page.activityJoin + page.runningJoin + page.stateJoin + page.repositoryJoin + `
	), ranked AS (
		SELECT ranked_ordered.*, global_root_sort_order AS sibling_order,
			global_root_sort_order AS root_sort_order FROM ranked_ordered
	)`
}

type sidebarPageBuildContext struct {
	groupOrder, rootPathPart, childPathPart                  string
	rootPinExpr                                              string
	cycleProbeGuard                                          string
	wipAdmittedFalse, order, rootOrder, groupKey, groupLabel string
	stateJoin, stateCTEs                                     string
	runningJoin, runningCTEs                                 string
	activityCTEs, activityJoin                               string
	ancestorCTEs                                             string
	repositoryCTEs, repositoryJoin                           string
	args, childArgs                                          []any
}

func sidebarPageBuildContextFor(driver string, query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) sidebarPageBuildContext {

	sortExpressions := make([]string, 0, len(query.Sort.Criteria()))
	var sortArgs []any
	for _, criterion := range query.Sort.Criteria() {
		expression, args := sidebarSortExpression(driver, query, criterion, prefs.OrderedTaskIDs)
		sortExpressions = append(sortExpressions, expression)
		sortArgs = append(sortArgs, args...)
	}
	sortExpr := strings.Join(sortExpressions, ", ")
	groupOrder := sidebarGroupOrderExpression(query.Group)
	pinExpr, pinArgs := sidebarIDOrder(driver, prefs.PinnedTaskIDs)
	subtaskExpr, subtaskArgs := sidebarSubtaskOrder(driver, prefs.SubtaskOrderByParentID)
	cycleProbeGuard := `instr(cycle_probe.visited, '/' || parent.id || '/') = 0`
	rootPathPart := `printf('%010d', root.display_root_order)`
	childPathPart := `printf('%010d', child.sibling_order)`
	if dialect.IsPostgres(driver) {
		cycleProbeGuard = `POSITION('/' || parent.id || '/' IN cycle_probe.visited) = 0`
		rootPathPart = `LPAD(CAST(root.display_root_order AS TEXT), 10, '0')`
		childPathPart = `LPAD(CAST(child.sibling_order AS TEXT), 10, '0')`
	}
	wipAdmittedFalse := `COALESCE(queue_task.wip_admitted, 0) = 0`
	canonicalOrder := sortExpr + `, v.updated_at DESC, ` + taskTitleOrder(driver, "v.", "ASC") + `, v.id ASC`
	rootOrder := canonicalOrder
	rootPinExpr := pinExpr
	args := append([]any(nil), sortArgs...)
	if query.Group == sidebarGroupNone {
		rootOrder = pinExpr + ` ASC, ` + canonicalOrder
		rootPinExpr = sidebarSQLNull
		args = append(append([]any(nil), pinArgs...), sortArgs...)
	} else {
		args = append(args, pinArgs...)
	}
	order := `CASE WHEN (` + subtaskExpr + `) IS NOT NULL THEN 0 ELSE 1 END,
		` + subtaskExpr + ` ASC, ` + canonicalOrder
	childArgs := append(append(append([]any(nil), subtaskArgs...), subtaskArgs...), sortArgs...)

	groupKey := "v.group_key"
	groupLabel := "v.group_label"
	repositoryCTEs, repositoryJoin := "", ""
	if query.Group == sidebarRepositoryKey {
		repositoryCTEs = sidebarRepositoryCTEs(driver)
		repositoryJoin = ` LEFT JOIN sidebar_repository_groups repository_group ON repository_group.task_id = v.id`
		groupKey = `COALESCE(repository_group.group_key, '__unassigned__')`
		groupLabel = `COALESCE(repository_group.group_label, '__unassigned__')`
	}
	stateJoin := ""
	if query.Group == sidebarStateKey {
		groupKey = `COALESCE(effective_state.effective_group_key, '__not_started__')`
		groupLabel = groupKey
	}
	if query.Group == sidebarStateKey || sidebarQueryHasSort(query, sidebarStateKey) {
		stateJoin = ` LEFT JOIN effective_tree_state effective_state ON effective_state.task_id = v.id`
	}
	stateCTEs := sidebarStateCTEs(query)
	activityCTEs, activityJoin := sidebarActivityCTEs(driver, query)
	runningCTEs, runningJoin := sidebarRunningCTEs(driver, query)
	return sidebarPageBuildContext{
		groupOrder: groupOrder, rootPathPart: rootPathPart, childPathPart: childPathPart,
		rootPinExpr:      rootPinExpr,
		cycleProbeGuard:  cycleProbeGuard,
		wipAdmittedFalse: wipAdmittedFalse, order: order, rootOrder: rootOrder, groupKey: groupKey, groupLabel: groupLabel,
		stateJoin: stateJoin, stateCTEs: stateCTEs,
		runningJoin: runningJoin, runningCTEs: runningCTEs,
		repositoryCTEs: repositoryCTEs, repositoryJoin: repositoryJoin,
		ancestorCTEs: sidebarAncestorCTE(driver, query), activityCTEs: activityCTEs, activityJoin: activityJoin, args: args, childArgs: childArgs,
	}
}

func sidebarAncestorCTE(driver string, query models.SidebarTaskViewQuery) string {
	state := sidebarQueryHasSort(query, sidebarStateKey)
	activity := sidebarQueryHasSort(query, sidebarActivitySortField)
	running := sidebarQueryHasSort(query, sidebarRunningSortField)
	if !state && !activity && !running {
		return ""
	}
	walkFields := []string{}
	anchorValues := []string{}
	recursiveValues := []string{}
	if activity {
		walkFields = append(walkFields, "activity_at")
		anchorValues = append(anchorValues, "source.activity_at")
		recursiveValues = append(recursiveValues, "walk.activity_at")
	}
	if state {
		walkFields = append(walkFields, "state", "state_bucket", "primary_session_state")
		anchorValues = append(anchorValues, "source.state", "source.state_bucket", "source.primary_session_state")
		recursiveValues = append(recursiveValues, "walk.state", "walk.state_bucket", "walk.primary_session_state")
	}
	walkColumnProjection, anchorProjection, recursiveProjection := "", "", ""
	if len(walkFields) > 0 {
		walkColumnProjection = ", " + strings.Join(walkFields, ", ")
		anchorProjection = ", " + strings.Join(anchorValues, ", ")
		recursiveProjection = ", " + strings.Join(recursiveValues, ", ")
	}
	//nolint:dupword // A state projection includes the source itself before its ancestors.
	anchorIdentity := `source.id, source.id, source.parent_id, '/' || source.id || '/'`
	anchorSource := `filtered source`
	if !state && !running {
		// Activity aggregation only needs proper descendants; each row keeps its own value.
		anchorIdentity = `source.id, parent.id, parent.parent_id, '/' || source.id || '/' || parent.id || '/'`
		anchorSource += ` JOIN filtered parent ON parent.id = source.parent_id AND parent.id <> source.id`
	}
	guard := `instr(walk.visited, '/' || parent.id || '/') = 0`
	if dialect.IsPostgres(driver) {
		guard = `POSITION('/' || parent.id || '/' IN walk.visited) = 0`
	}
	materialization := "NOT MATERIALIZED"
	if state {
		materialization = "MATERIALIZED"
	}
	return `, ancestor_walk(source_key, ancestor_key, parent_key, visited` + walkColumnProjection + `) AS ` + materialization + ` (
		SELECT ` + anchorIdentity + anchorProjection + ` FROM ` + anchorSource + `
		UNION ALL
		SELECT walk.source_key, parent.id, parent.parent_id, walk.visited || parent.id || '/'` + recursiveProjection + `
		FROM ancestor_walk walk JOIN filtered parent ON parent.id = walk.parent_key
		WHERE ` + guard + `
	)`
}

func sidebarStateCTEs(query models.SidebarTaskViewQuery) string {
	if query.Group != sidebarStateKey && !sidebarQueryHasSort(query, sidebarStateKey) {
		return ""
	}
	members := `, state_members AS NOT MATERIALIZED (
		SELECT source_key, ancestor_key, state, state_bucket, primary_session_state FROM ancestor_walk
	)`
	if !sidebarQueryHasSort(query, sidebarStateKey) {
		// Group identity uses the same complete root memberships as page counts.
		members = `, state_members AS MATERIALIZED (
			SELECT source.id AS source_key, member.root_id AS ancestor_key,
				source.state, source.state_bucket, source.primary_session_state
			FROM root_members member JOIN filtered source ON source.id = member.id
		)`
	}
	return members + `, state_aggregate AS (
		SELECT member.ancestor_key AS task_id,
			MAX(CASE WHEN member.state = 'IN_PROGRESS' OR member.primary_session_state = 'RUNNING' THEN 1 ELSE 0 END) AS has_active,
			MAX(CASE WHEN member.state = 'SCHEDULING' THEN 1 ELSE 0 END) AS has_scheduling,
			CASE WHEN SUM(CASE WHEN member.state = 'COMPLETED' THEN 1 ELSE 0 END) = COUNT(*) THEN 1 ELSE 0 END AS all_completed,
			MIN(CASE WHEN member.state <> 'COMPLETED' THEN member.state END) AS first_state,
			MAX(CASE WHEN member.state <> 'COMPLETED' THEN member.state END) AS last_state,
			MIN(CASE WHEN member.state <> 'COMPLETED' THEN member.state_bucket END) AS first_bucket,
			MAX(CASE WHEN member.state <> 'COMPLETED' THEN member.state_bucket END) AS last_bucket
		FROM state_members member
		GROUP BY member.ancestor_key
	), state_candidates AS (
		SELECT member.ancestor_key AS task_id, member.state, member.state_bucket,
			ROW_NUMBER() OVER (PARTITION BY member.ancestor_key ORDER BY
				CASE member.state_bucket WHEN 'review' THEN 0 WHEN 'in_progress' THEN 1 ELSE 2 END,
				CASE member.state WHEN '__not_started__' THEN 0 WHEN 'CREATED' THEN 1 WHEN 'SCHEDULING' THEN 2
					WHEN 'TODO' THEN 3 WHEN 'IN_PROGRESS' THEN 4 WHEN 'WAITING_FOR_INPUT' THEN 5
					WHEN 'REVIEW' THEN 6 WHEN 'BLOCKED' THEN 7 WHEN 'FAILED' THEN 8
					WHEN 'COMPLETED' THEN 9 WHEN 'CANCELLED' THEN 10 ELSE 99 END,
				CASE WHEN member.source_key = member.ancestor_key THEN 0 ELSE 1 END, member.source_key) AS candidate_order
		FROM state_members member JOIN state_aggregate aggregate ON aggregate.task_id = member.ancestor_key
		WHERE member.state IS NOT NULL AND member.state <> 'COMPLETED'
			AND aggregate.has_active = 0 AND aggregate.has_scheduling = 0 AND aggregate.all_completed = 0
			AND (aggregate.first_state <> aggregate.last_state OR aggregate.first_bucket <> aggregate.last_bucket)
	), effective_tree_state AS (
		SELECT aggregate.task_id,
			CASE WHEN aggregate.has_active = 1 THEN 'IN_PROGRESS'
				WHEN aggregate.has_scheduling = 1 THEN 'SCHEDULING'
				WHEN aggregate.all_completed = 1 THEN 'COMPLETED'
				WHEN aggregate.first_state = aggregate.last_state AND aggregate.first_bucket = aggregate.last_bucket THEN aggregate.first_state
				ELSE COALESCE(candidate.state, '__not_started__') END AS effective_group_key,
			CASE WHEN aggregate.has_active = 1 OR aggregate.has_scheduling = 1 THEN 'in_progress'
				WHEN aggregate.all_completed = 1 THEN 'review'
				WHEN aggregate.first_state = aggregate.last_state AND aggregate.first_bucket = aggregate.last_bucket THEN aggregate.first_bucket
				ELSE COALESCE(candidate.state_bucket, 'backlog') END AS effective_bucket
		FROM state_aggregate aggregate
		LEFT JOIN state_candidates candidate ON candidate.task_id = aggregate.task_id AND candidate.candidate_order = 1
	)`
}

func sidebarActivityCTEs(driver string, query models.SidebarTaskViewQuery) (string, string) {
	if !sidebarQueryHasSort(query, sidebarActivitySortField) {
		return "", ""
	}
	if sidebarQueryHasSort(query, sidebarRunningSortField) {
		return `, tree_activity AS (
			SELECT ancestor_key AS ancestor_id,
				MAX(walk.activity_at) AS tree_activity_at,
				MAX(` + sidebarRunningFlagExpression(driver, "running_summary_walk", "walk.source_key") + `) AS tree_has_running
			FROM ancestor_walk walk
			LEFT JOIN task_status_summaries running_summary_walk ON running_summary_walk.task_id = walk.source_key
			WHERE walk.source_key <> walk.ancestor_key
			GROUP BY walk.ancestor_key
		)`, ` LEFT JOIN tree_activity activity ON activity.ancestor_id = v.id`
	}
	return `, tree_activity AS (
		SELECT ancestor_key AS ancestor_id, MAX(activity_at) AS tree_activity_at
		FROM ancestor_walk WHERE source_key <> ancestor_key
		GROUP BY ancestor_key
	)`, ` LEFT JOIN tree_activity activity ON activity.ancestor_id = v.id`
}

func sidebarRunningCTEs(driver string, query models.SidebarTaskViewQuery) (string, string) {
	if !sidebarQueryHasSort(query, sidebarRunningSortField) {
		return "", ""
	}
	if sidebarQueryHasSort(query, sidebarActivitySortField) {
		return "", ` LEFT JOIN task_status_summaries running_summary ON running_summary.task_id = v.id`
	}
	return `, running_aggregate AS (
		SELECT walk.ancestor_key AS task_id,
			MAX(` + sidebarRunningFlagExpression(driver, "running_summary", "walk.source_key") + `) AS has_running
		FROM ancestor_walk walk
		LEFT JOIN task_status_summaries running_summary ON running_summary.task_id = walk.source_key
		GROUP BY walk.ancestor_key
	)`, ` LEFT JOIN running_aggregate running ON running.task_id = v.id`
}

func sidebarGroupOrderExpression(group string) string {
	switch group {
	case sidebarStateKey:
		return `CASE group_key
			WHEN '__not_started__' THEN 0 WHEN 'CREATED' THEN 1 WHEN 'SCHEDULING' THEN 2
			WHEN 'TODO' THEN 3 WHEN 'IN_PROGRESS' THEN 4 WHEN 'WAITING_FOR_INPUT' THEN 5
			WHEN 'REVIEW' THEN 6 WHEN 'BLOCKED' THEN 7 WHEN 'FAILED' THEN 8
			WHEN 'COMPLETED' THEN 9 WHEN 'CANCELLED' THEN 10 ELSE 99 END, first_order`
	case sidebarRepositoryKey:
		return `CASE WHEN group_key = '__multi__' THEN 0
			WHEN SUBSTR(group_key, 1, 21) = '__repo_combination__:' THEN 1
			WHEN group_key = '__unassigned__' THEN 3 ELSE 2 END,
			LOWER(group_label) ASC, group_label ASC, first_order ASC`
	default:
		return `first_order ASC, group_key ASC`
	}
}

func sidebarSubtaskOrder(driver string, orders map[string][]string) (string, []any) {
	if len(orders) == 0 {
		return sidebarSQLNull, nil
	}
	if !dialect.IsPostgres(driver) {
		for _, ids := range orders {
			if len(ids) > 0 {
				return sidebarPreferenceOrder("child", "v.parent_id"), nil
			}
		}
		return sidebarSQLNull, nil
	}
	parents := make([]string, 0, len(orders))
	for parentID := range orders {
		parents = append(parents, parentID)
	}
	sort.Strings(parents)
	branches := make([]string, 0, len(parents))
	args := make([]any, 0)
	for _, parentID := range parents {
		ids := orders[parentID]
		if len(ids) == 0 {
			continue
		}
		cases := make([]string, 0, len(ids))
		args = append(args, parentID)
		for index, id := range ids {
			cases = append(cases, "WHEN ? THEN CAST(? AS INTEGER)")
			args = append(args, id, index)
		}
		branches = append(branches, "WHEN ? THEN CASE v.id "+strings.Join(cases, " ")+" ELSE NULL END")
	}
	if len(branches) == 0 {
		return sidebarSQLNull, nil
	}
	return "CASE v.parent_id " + strings.Join(branches, " ") + " ELSE NULL END", args
}

func sidebarSortExpression(
	driver string,
	query models.SidebarTaskViewQuery,
	criterion models.SidebarTaskViewSortCriterion,
	orderIDs []string,
) (string, []any) {
	order := strings.ToUpper(criterion.Direction)
	switch criterion.Key {
	case sidebarStateKey:
		return `CASE effective_state.effective_bucket WHEN 'review' THEN 0 WHEN 'in_progress' THEN 1 ELSE 2 END ` + order, nil
	case "updatedAt":
		return "v.updated_at " + order, nil
	case sidebarActivitySortField:
		return "CASE WHEN v.activity_at IS NULL OR activity.tree_activity_at > v.activity_at THEN activity.tree_activity_at ELSE v.activity_at END " + order, nil
	case sidebarRunningSortField:
		if sidebarQueryHasSort(query, sidebarActivitySortField) {
			return `CASE WHEN ` + sidebarRunningFlagExpression(driver, "running_summary", "v.id") + ` = 1
				OR COALESCE(activity.tree_has_running, 0) = 1 THEN 1 ELSE 0 END ` + order, nil
		}
		return `COALESCE(running.has_running, 0) ` + order, nil
	case "color":
		return "CASE WHEN v.effective_color = ? THEN 1 ELSE 0 END " + order, []any{criterion.Color}
	case "createdAt":
		return "v.created_at " + order, nil
	case "title":
		return taskTitleOrder(driver, "v.", order), nil
	case sidebarCustomSortKey:
		if len(orderIDs) == 0 {
			return "v.created_at DESC", nil
		}
		if !dialect.IsPostgres(driver) {
			return sqliteSidebarIDOrder("ordered", len(orderIDs)) + " ASC, v.created_at DESC", nil
		}
		cases := make([]string, 0, len(orderIDs))
		args := make([]any, 0, len(orderIDs)*2)
		for index, id := range orderIDs {
			cases = append(cases, "WHEN ? THEN ?")
			args = append(args, id, index)
		}
		return "CASE v.id " + strings.Join(cases, " ") + " ELSE " + fmt.Sprint(len(orderIDs)) + " END ASC, v.created_at DESC", args
	default:
		return "v.updated_at DESC", nil
	}
}

func sidebarIDOrder(driver string, ids []string) (string, []any) {
	if len(ids) == 0 {
		return "CASE WHEN 1=1 THEN 0 ELSE 0 END", nil
	}
	if !dialect.IsPostgres(driver) {
		return sqliteSidebarIDOrder("pinned", len(ids)), nil
	}
	cases := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)*2)
	for index, id := range ids {
		cases = append(cases, "WHEN ? THEN ?")
		args = append(args, id, index)
	}
	return "CASE v.id " + strings.Join(cases, " ") + " ELSE " + fmt.Sprint(len(ids)) + " END", args
}
