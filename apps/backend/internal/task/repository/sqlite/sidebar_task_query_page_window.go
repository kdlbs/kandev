package sqlite

import (
	"strconv"

	"github.com/kandev/kandev/internal/task/models"
)

func sidebarRootMembershipCTEs() string {
	return `, display_roots AS MATERIALIZED (
		SELECT v.* FROM filtered v LEFT JOIN filtered parent ON parent.id = v.parent_id
		LEFT JOIN cycle_roots cycle_root ON cycle_root.root_key = v.id
		WHERE parent.id IS NULL OR cycle_root.root_key IS NOT NULL
	), root_members(id, root_id) AS (
		SELECT id, id FROM display_roots
		UNION ALL
		SELECT child.id, member.root_id FROM root_members member
		JOIN filtered child ON child.parent_id = member.id
		LEFT JOIN cycle_roots cycle_root ON cycle_root.root_key = child.id
		WHERE cycle_root.root_key IS NULL
	)`
}

func sidebarRootWindowCTEs(driver string, query models.SidebarTaskViewQuery, treeRootSQL string, page *sidebarPageBuildContext) string {
	visible := ""
	if len(query.CollapsedTaskIDs) > 0 {
		visible = ` WHERE id NOT IN (SELECT id FROM hidden_tasks)`
	}
	groups := ""
	if len(query.CollapsedGroupKeys) > 0 {
		groupsSQL, groupsArgs := sidebarStringListSQL(driver, query.CollapsedGroupKeys)
		groups = ` AND root.root_group_key NOT IN (` + groupsSQL + `)`
		page.args = append(page.args, groupsArgs...)
	}
	// Counts use the complete forest; only intersecting trees need child ordering.
	return `, root_visible_counts AS (
		SELECT root_id, COUNT(*) AS visible_count FROM root_members` + visible + ` GROUP BY root_id
	), root_display_rows AS (` + treeRootSQL + `
	), group_task_counts AS (
		SELECT root.root_group_key AS group_key, root.root_group_label AS group_label,
			CAST(SUM(COALESCE(counts.visible_count, 0)) AS BIGINT) AS task_count
		FROM root_display_rows root LEFT JOIN root_visible_counts counts ON counts.root_id = root.id
		GROUP BY root.root_group_key, root.root_group_label
	), root_windows AS (
		SELECT root.*, counts.visible_count,
			CAST(SUM(counts.visible_count) OVER (ORDER BY root.group_order, root.order_path) AS BIGINT) AS end_offset
		FROM root_display_rows root JOIN root_visible_counts counts ON counts.root_id = root.id
		WHERE counts.visible_count > 0` + groups + `
	), page_summary AS (
		SELECT (SELECT COUNT(*) FROM filtered) AS total_tasks,
			COALESCE((SELECT MAX(end_offset) FROM root_windows), 0) AS total_visible_tasks,
			` + sidebarTotalGroupCountSQL(query.Group) + ` AS total_groups
	), page_options AS (
		SELECT request.page_size,
			CASE WHEN summary.total_visible_tasks = 0 THEN 1
				WHEN request.requested_page <= ((summary.total_visible_tasks + request.page_size - 1) / request.page_size)
					THEN request.requested_page
				ELSE ((summary.total_visible_tasks + request.page_size - 1) / request.page_size)
			END AS page
		FROM (SELECT CAST(? AS BIGINT) AS page_size, CAST(? AS BIGINT) AS requested_page) request
		CROSS JOIN page_summary summary
	), selected_roots AS MATERIALIZED (
		SELECT root.* FROM root_windows root CROSS JOIN page_options options
		WHERE root.end_offset > (options.page - 1) * options.page_size
			AND root.end_offset - root.visible_count < options.page * options.page_size
	)`
}

func sidebarSelectedTreeCTEs(query models.SidebarTaskViewQuery, page sidebarPageBuildContext) string {
	visible := ""
	if len(query.CollapsedTaskIDs) > 0 {
		visible = ` WHERE tree.id NOT IN (SELECT id FROM hidden_tasks)`
	}
	return `, selected_members AS (
		SELECT member.id FROM root_members member JOIN selected_roots root ON root.id = member.root_id
	), ranked_children AS (
		SELECT v.id, v.workflow_id, v.workflow_step_id, v.parent_id,
			ROW_NUMBER() OVER (PARTITION BY v.parent_id ORDER BY ` + page.order + `) AS sibling_order
		FROM filtered v JOIN selected_members member ON member.id = v.id
		JOIN filtered parent ON parent.id = v.parent_id
		LEFT JOIN cycle_roots cycle_root ON cycle_root.root_key = v.id
		` + page.activityJoin + page.stateJoin + `
		WHERE cycle_root.root_key IS NULL
	), tree AS (
		SELECT root.id, root.root_id, root.root_group_key, root.root_group_label, root.group_order,
			root.workflow_id, root.workflow_step_id, root.parent_id, root.depth, root.order_path
		FROM selected_roots root
		UNION ALL
		SELECT child.id, tree.root_id, tree.root_group_key, tree.root_group_label,
			tree.group_order, child.workflow_id, child.workflow_step_id,
			child.parent_id, tree.depth + 1, tree.order_path || '.' || ` + page.childPathPart + `
		FROM tree JOIN ranked_children child ON child.parent_id = tree.id
	), page_window AS MATERIALIZED (
		SELECT tree.* FROM tree` + visible + `
		ORDER BY group_order ASC, order_path ASC
		LIMIT ` + strconv.Itoa(query.PageSize) + `
		OFFSET (SELECT (page - 1) * page_size FROM page_options)
			- COALESCE((SELECT MIN(end_offset - visible_count) FROM selected_roots), 0)
	)`
}

func sidebarPageResultCTEs(group string) string {
	groupStarts := `SELECT group_order, MIN(order_path) AS first_order_path FROM root_windows GROUP BY group_order`
	if group == sidebarGroupNone {
		groupStarts = `SELECT 1 AS group_order, MIN(order_path) AS first_order_path FROM root_windows`
	}
	return `, page_group_starts AS (` + groupStarts + `
	), page_subtask_walk(ancestor_id, descendant_id) AS (
		SELECT page.id, page.id FROM page_window page
		UNION ALL
		SELECT walk.ancestor_id, child.id FROM page_subtask_walk walk
		JOIN filtered child ON child.parent_id = walk.descendant_id
		LEFT JOIN cycle_roots cycle_root ON cycle_root.root_key = child.id
		WHERE cycle_root.root_key IS NULL
	), page_subtask_counts AS (
		SELECT ancestor_id, COUNT(*) - 1 AS subtask_count FROM page_subtask_walk GROUP BY ancestor_id
	)`
}
