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
	ctes := sidebarPageTreeCTEs(driver, query, page, groupCTEs, treeRootSQL)
	groupCountSQL := `SELECT root_group_key AS group_key, root_group_label AS group_label, COUNT(*) AS task_count
			FROM tree WHERE 1=1`
	if query.Group == sidebarGroupNone {
		groupCountSQL = `SELECT '__all__' AS group_key, '__all__' AS group_label, COUNT(*) AS task_count
			FROM tree WHERE 1=1`
	}
	ctes += groupCountSQL
	if len(query.CollapsedTaskIDs) > 0 {
		ctes += ` AND id NOT IN (SELECT id FROM hidden_tasks)`
	}
	if query.Group != sidebarGroupNone {
		ctes += ` GROUP BY root_group_key, root_group_label`
	}
	ctes += `
		), display_tree AS (
			SELECT tree.* FROM tree WHERE 1=1`
	if len(query.CollapsedTaskIDs) > 0 {
		ctes += ` AND tree.id NOT IN (SELECT id FROM hidden_tasks)`
	}
	if len(query.CollapsedGroupKeys) > 0 {
		ctes += ` AND tree.root_group_key NOT IN (` + placeholders(len(query.CollapsedGroupKeys)) + `)`
		for _, key := range query.CollapsedGroupKeys {
			page.args = append(page.args, key)
		}
	}
	ctes += `
	), page_ordered_tree AS (
		SELECT display_tree.*, ROW_NUMBER() OVER (PARTITION BY group_order ORDER BY order_path) AS group_position
		FROM display_tree
	), page_summary AS (
		SELECT (SELECT COUNT(*) FROM filtered) AS total_tasks,
			(SELECT COUNT(*) FROM display_tree) AS total_visible_tasks,
			` + sidebarTotalGroupCountSQL(query.Group) + ` AS total_groups
	), page_options AS (
		SELECT request.page_size,
			CASE
				WHEN summary.total_visible_tasks = 0 THEN 1
				WHEN request.requested_page <= ((summary.total_visible_tasks + request.page_size - 1) / request.page_size)
					THEN request.requested_page
				ELSE ((summary.total_visible_tasks + request.page_size - 1) / request.page_size)
			END AS page
		FROM (SELECT CAST(? AS BIGINT) AS page_size, CAST(? AS BIGINT) AS requested_page) request
		CROSS JOIN page_summary summary
	), page_window AS (
		SELECT id FROM page_ordered_tree
		ORDER BY group_order ASC, order_path ASC
		LIMIT (SELECT page_size FROM page_options)
		OFFSET (SELECT (page - 1) * page_size FROM page_options)
	), page_queue_steps AS MATERIALIZED (
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
			AND page_queue_steps.queued_for_step_id = queue_task.queued_for_step_id
			AND queue_task.workflow_step_id = queue_task.queued_for_step_id
			AND queue_task.queued_for_step_id <> ''
			AND ` + page.wipAdmittedFalse + `
			AND COALESCE(queue_task.is_ephemeral, 0) = 0
			AND COALESCE(queue_task.origin, '') <> 'automation_run'
			AND ` + excludeConfigModePredicate(driver, "queue_task.metadata") + `
	)`
	page.args = append(page.args, query.PageSize, query.Page)
	return ctes, page.args
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
		FROM ranked root
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
		FROM ranked root WHERE root.display_parent_id IS NULL`
	}
	return groupCTEs, treeRootSQL
}

func sidebarPageTreeCTEs(driver string, query models.SidebarTaskViewQuery, page sidebarPageBuildContext, groupCTEs, treeRootSQL string) string {
	//nolint:dupword // SQL CTE keys follow the task identifier schema.
	cycleCTEs := `, cycle_probe(start_key, current_key, visited, closed_by_key, min_key) AS (
			SELECT id, id, '/' || id || '/', NULL, id
			FROM filtered
			WHERE parent_id >= id
			UNION ALL
			SELECT cycle_probe.start_key, parent.id, cycle_probe.visited || parent.id || '/',
				CASE WHEN ` + page.cycleProbeGuard + ` THEN NULL ELSE parent.id END,
				CASE WHEN parent.id < cycle_probe.min_key THEN parent.id ELSE cycle_probe.min_key END
			FROM cycle_probe
			JOIN filtered current ON current.id = cycle_probe.current_key
			JOIN filtered parent ON parent.id = current.parent_id
			WHERE cycle_probe.closed_by_key IS NULL
		), cycle_roots(root_key) AS (
			SELECT DISTINCT min_key FROM cycle_probe WHERE closed_by_key = start_key
		)`
	globalRootOrder := `CASE WHEN ` + page.rootCondition + ` THEN ROW_NUMBER() OVER (ORDER BY ` + page.sortExpr + `, v.updated_at DESC, ` + taskTitleOrder(driver, "v.", "ASC") + `, v.id ASC) END`
	ctes := cycleCTEs + page.stateCTEs + page.activityCTEs + `, ranked_ordered AS (
			SELECT v.*, CASE WHEN ` + page.rootCondition + ` THEN NULL ELSE parent.id END AS display_parent_id,
				` + page.treeActivityExpr + ` AS tree_activity_at, ` + page.groupKey + ` AS task_group_key, ` + page.groupLabel + ` AS task_group_label,
				ROW_NUMBER() OVER (PARTITION BY CASE
					WHEN ` + page.rootCondition + ` THEN 'root:' || ` + page.groupKey + ` ELSE 'parent:' || parent.id END
					ORDER BY ` + page.order + `) AS sibling_order,
				` + globalRootOrder + ` AS global_root_sort_order
			FROM filtered v LEFT JOIN filtered parent ON parent.id = v.parent_id
			LEFT JOIN cycle_roots cycle_root ON cycle_root.root_key = v.id
			` + page.activityJoin + page.stateJoin + `
		), ranked AS (
			SELECT ranked_ordered.*,
				CASE WHEN display_parent_id IS NULL THEN sibling_order END AS root_sort_order
			FROM ranked_ordered
		)` + groupCTEs + `, tree AS (
			` + treeRootSQL + `
			UNION ALL
			SELECT child.id, tree.root_id, tree.root_group_key, tree.root_group_label,
				tree.group_order, child.workflow_id, child.workflow_step_id,
				child.parent_id, tree.depth + 1, tree.order_path || '.' || ` + page.childPathPart + `
			FROM tree
			JOIN filtered child_source ON child_source.parent_id = tree.id
			JOIN ranked child ON child.id = child_source.id AND child.display_parent_id = tree.id
		), group_task_counts AS (`
	return ctes
}

type sidebarPageBuildContext struct {
	sortExpr, groupOrder, rootPathPart, childPathPart string
	cycleProbeGuard                                   string
	wipAdmittedFalse, order, groupKey, groupLabel     string
	stateJoin, rootCondition, stateCTEs               string
	activityCTEs, activityJoin, treeActivityExpr      string
	args                                              []any
}

func sidebarPageBuildContextFor(driver string, query models.SidebarTaskViewQuery, prefs models.SidebarTaskViewPreferences) sidebarPageBuildContext {

	sortExpr, sortArgs := sidebarSortExpression(driver, query.Sort.Key, query.Sort.Direction, prefs.OrderedTaskIDs)
	groupOrder := sidebarGroupOrderExpression(query.Group)
	pinExpr, pinArgs := sidebarIDOrder(prefs.PinnedTaskIDs)
	subtaskExpr, subtaskArgs := sidebarSubtaskOrder(prefs.SubtaskOrderByParentID)
	cycleProbeGuard := `instr(cycle_probe.visited, '/' || parent.id || '/') = 0`
	activityCycleGuard := `instr(activity_walk.visited, '/' || parent.id || '/') = 0`
	stateCycleGuard := `instr(state_walk.visited, '/' || parent.id || '/') = 0`
	rootPathPart := `printf('%010d', root.sibling_order)`
	childPathPart := `printf('%010d', child.sibling_order)`
	if dialect.IsPostgres(driver) {
		cycleProbeGuard = `POSITION('/' || parent.id || '/' IN cycle_probe.visited) = 0`
		activityCycleGuard = `POSITION('/' || parent.id || '/' IN activity_walk.visited) = 0`
		stateCycleGuard = `POSITION('/' || parent.id || '/' IN state_walk.visited) = 0`
		rootPathPart = `LPAD(CAST(root.sibling_order AS TEXT), 10, '0')`
		childPathPart = `LPAD(CAST(child.sibling_order AS TEXT), 10, '0')`
	}
	wipAdmittedFalse := `COALESCE(queue_task.wip_admitted, 0) = 0`
	rootPinOrder := `CASE WHEN parent.id IS NULL OR cycle_root.root_key IS NOT NULL THEN ` + pinExpr + ` ELSE 0 END`
	manualChildOrder := `CASE WHEN parent.id IS NOT NULL AND cycle_root.root_key IS NULL
			AND (` + subtaskExpr + `) IS NOT NULL THEN 0 ELSE 1 END,
		CASE WHEN parent.id IS NOT NULL AND cycle_root.root_key IS NULL THEN ` + subtaskExpr + ` ELSE 0 END`
	order := rootPinOrder + ` ASC, ` + manualChildOrder + ` ASC,
			` + sortExpr + `, v.updated_at DESC, ` + taskTitleOrder(driver, "v.", "ASC") + `, v.id ASC`
	orderArgs := make([]any, 0, len(pinArgs)+2*len(subtaskArgs)+len(sortArgs))
	orderArgs = append(orderArgs, pinArgs...)
	orderArgs = append(orderArgs, subtaskArgs...)
	orderArgs = append(orderArgs, subtaskArgs...)
	orderArgs = append(orderArgs, sortArgs...)
	args := append([]any(nil), orderArgs...)
	args = append(args, sortArgs...)

	groupKey := "v.group_key"
	groupLabel := "v.group_label"
	stateJoin := ""
	if query.Group == sidebarStateKey {
		groupKey = `COALESCE(effective_state.effective_group_key, '__not_started__')`
		groupLabel = groupKey
	}
	if query.Group == sidebarStateKey || query.Sort.Key == sidebarStateKey {
		stateJoin = ` LEFT JOIN effective_tree_state effective_state ON effective_state.task_id = v.id`
	}
	rootCondition := `parent.id IS NULL OR cycle_root.root_key IS NOT NULL`
	stateCTEs := sidebarStateCTEs(query, stateCycleGuard)
	activityCTEs, activityJoin, treeActivityExpr := sidebarActivityCTEs(query, activityCycleGuard)
	return sidebarPageBuildContext{
		sortExpr: sortExpr, groupOrder: groupOrder, rootPathPart: rootPathPart, childPathPart: childPathPart,
		cycleProbeGuard:  cycleProbeGuard,
		wipAdmittedFalse: wipAdmittedFalse, order: order, groupKey: groupKey, groupLabel: groupLabel,
		stateJoin: stateJoin, rootCondition: rootCondition, stateCTEs: stateCTEs,
		activityCTEs: activityCTEs, activityJoin: activityJoin, treeActivityExpr: treeActivityExpr, args: args,
	}
}

func sidebarStateCTEs(query models.SidebarTaskViewQuery, stateCycleGuard string) string {
	var stateCTEs string
	if query.Group == sidebarStateKey || query.Sort.Key == sidebarStateKey {
		//nolint:dupword // SQL CTE keys follow the task identifier schema.
		stateCTEs = `, state_walk(source_key, ancestor_key, visited) AS (
			SELECT id, id, '/' || id || '/' FROM filtered
			UNION ALL
			SELECT state_walk.source_key, parent.id, state_walk.visited || parent.id || '/'
			FROM state_walk
			JOIN filtered current ON current.id = state_walk.ancestor_key
			JOIN filtered parent ON parent.id = current.parent_id
			WHERE ` + stateCycleGuard + `
		), state_aggregate AS (
			SELECT state_walk.ancestor_key AS task_id,
				MAX(CASE WHEN member.state = 'IN_PROGRESS' OR member.primary_session_state = 'RUNNING' THEN 1 ELSE 0 END) AS has_active,
				MAX(CASE WHEN member.state = 'SCHEDULING' THEN 1 ELSE 0 END) AS has_scheduling,
				CASE WHEN SUM(CASE WHEN member.state = 'COMPLETED' THEN 1 ELSE 0 END) = COUNT(*) THEN 1 ELSE 0 END AS all_completed
			FROM state_walk JOIN filtered member ON member.id = state_walk.source_key
			GROUP BY state_walk.ancestor_key
		), state_candidates AS (
			SELECT state_walk.ancestor_key AS task_id, member.state, member.state_bucket,
				ROW_NUMBER() OVER (PARTITION BY state_walk.ancestor_key ORDER BY
					CASE member.state_bucket WHEN 'review' THEN 0 WHEN 'in_progress' THEN 1 ELSE 2 END,
					CASE member.state WHEN '__not_started__' THEN 0 WHEN 'CREATED' THEN 1 WHEN 'SCHEDULING' THEN 2
						WHEN 'TODO' THEN 3 WHEN 'IN_PROGRESS' THEN 4 WHEN 'WAITING_FOR_INPUT' THEN 5
						WHEN 'REVIEW' THEN 6 WHEN 'BLOCKED' THEN 7 WHEN 'FAILED' THEN 8
						WHEN 'COMPLETED' THEN 9 WHEN 'CANCELLED' THEN 10 ELSE 99 END,
					CASE WHEN member.id = state_walk.ancestor_key THEN 0 ELSE 1 END, member.id) AS candidate_order
			FROM state_walk JOIN filtered member ON member.id = state_walk.source_key
			WHERE member.state IS NOT NULL AND member.state <> 'COMPLETED'
		), effective_tree_state AS (
			SELECT aggregate.task_id,
				CASE WHEN aggregate.has_active = 1 THEN 'IN_PROGRESS'
					WHEN aggregate.has_scheduling = 1 THEN 'SCHEDULING'
					WHEN aggregate.all_completed = 1 THEN 'COMPLETED'
					ELSE COALESCE(candidate.state, '__not_started__') END AS effective_group_key,
				CASE WHEN aggregate.has_active = 1 OR aggregate.has_scheduling = 1 THEN 'in_progress'
					WHEN aggregate.all_completed = 1 THEN 'review'
					ELSE COALESCE(candidate.state_bucket, 'backlog') END AS effective_bucket
			FROM state_aggregate aggregate
			LEFT JOIN state_candidates candidate ON candidate.task_id = aggregate.task_id AND candidate.candidate_order = 1
		)`
	}
	return stateCTEs
}

func sidebarActivityCTEs(query models.SidebarTaskViewQuery, activityCycleGuard string) (string, string, string) {
	activityCTEs := ""
	activityJoin := ""
	treeActivityExpr := sidebarSQLNull
	if query.Sort.Key == sidebarActivitySortField {
		//nolint:dupword // SQL CTE keys follow the task identifier schema.
		activityCTEs = `, activity_walk(source_key, ancestor_key, visited) AS (
			SELECT id, id, '/' || id || '/' FROM filtered
			UNION ALL
			SELECT activity_walk.source_key, parent.id, activity_walk.visited || parent.id || '/'
			FROM activity_walk
			JOIN filtered current ON current.id = activity_walk.ancestor_key
			JOIN filtered parent ON parent.id = current.parent_id
			WHERE ` + activityCycleGuard + `
		), tree_activity AS (
			SELECT activity_walk.ancestor_key AS ancestor_id, MAX(task.activity_at) AS tree_activity_at
			FROM activity_walk JOIN filtered task ON task.id = activity_walk.source_key
			GROUP BY activity_walk.ancestor_key
		)`
		activityJoin = ` LEFT JOIN tree_activity activity ON activity.ancestor_id = v.id`
		treeActivityExpr = "COALESCE(activity.tree_activity_at, v.activity_at)"
	}
	return activityCTEs, activityJoin, treeActivityExpr
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

func sidebarSubtaskOrder(orders map[string][]string) (string, []any) {
	if len(orders) == 0 {
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

func sidebarSortExpression(driver, key, direction string, orderIDs []string) (string, []any) {
	order := strings.ToUpper(direction)
	switch key {
	case sidebarStateKey:
		return `CASE effective_state.effective_bucket WHEN 'review' THEN 0 WHEN 'in_progress' THEN 1 ELSE 2 END ` + order, nil
	case "updatedAt":
		return "v.updated_at " + order, nil
	case sidebarActivitySortField:
		return "COALESCE(activity.tree_activity_at, v.activity_at) " + order, nil
	case "createdAt":
		return "v.created_at " + order, nil
	case "title":
		return taskTitleOrder(driver, "v.", order), nil
	case sidebarCustomSortKey:
		if len(orderIDs) == 0 {
			return "v.created_at DESC", nil
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

func sidebarIDOrder(ids []string) (string, []any) {
	if len(ids) == 0 {
		return "CASE WHEN 1=1 THEN 0 ELSE 0 END", nil
	}
	cases := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)*2)
	for index, id := range ids {
		cases = append(cases, "WHEN ? THEN ?")
		args = append(args, id, index)
	}
	return "CASE v.id " + strings.Join(cases, " ") + " ELSE " + fmt.Sprint(len(ids)) + " END", args
}
