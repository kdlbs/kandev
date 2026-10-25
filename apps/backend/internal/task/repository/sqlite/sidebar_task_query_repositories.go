package sqlite

import (
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
)

const sidebarRepositoryLabel = `CASE
	WHEN COALESCE(r.provider_owner, '') <> '' AND COALESCE(r.provider_name, '') <> ''
	THEN r.provider_owner || '/' || r.provider_name ELSE r.name END`

func sidebarRepositoryFields(needs sidebarBaseNeeds) []string {
	if !needs.repositoryGroup && !needs.repositoryFilter {
		return nil
	}
	name := "COALESCE(repository_projection.repository_name, 'undefined')"
	if needs.repositoryFilter {
		// Filters use the first resolved link, including its exact branch-link order.
		name = `COALESCE((SELECT ` + sidebarRepositoryLabel + `
			FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
			WHERE tr.task_id = t.id ORDER BY tr.position ASC, tr.id ASC LIMIT 1), 'undefined')`
	}
	fields := []string{name + " AS repository_name"}
	if needs.repositoryGroup {
		fields = append(fields,
			"COALESCE(repository_projection.repository_count, 0) AS repository_count",
			"COALESCE(repository_projection.resolved_repository_count, 0) AS resolved_repository_count",
			"COALESCE(repository_projection.repository_slugs, '[]') AS repository_slugs",
			"COALESCE(repository_projection.repository_labels, 'undefined') AS repository_labels",
		)
	}
	return fields
}

func sidebarRepositoryCTEs(driver string) string {
	slugs := `json_group_array(repo_slug ORDER BY position, link_id) FILTER (WHERE resolved = 1)`
	labels := `group_concat(repo_slug, ', ' ORDER BY position, link_id) FILTER (WHERE resolved = 1)`
	if dialect.IsPostgres(driver) {
		slugs = `array_to_json(array_agg(repo_slug ORDER BY position, link_id) FILTER (WHERE resolved = 1))::text`
		labels = `string_agg(repo_slug, ', ' ORDER BY position, link_id) FILTER (WHERE resolved = 1)`
	}
	groupKey, groupLabel := sidebarGroupExpressions(sidebarRepositoryKey)
	fields := sidebarRepositoryFields(sidebarBaseNeeds{repositoryGroup: true})
	return `, sidebar_repository_members AS MATERIALIZED (
		SELECT tr.task_id, tr.repository_id, ` + sidebarRepositoryLabel + ` AS repo_slug,
			MIN(tr.position) AS position, MIN(tr.id) AS link_id,
			MAX(CASE WHEN r.id IS NOT NULL THEN 1 ELSE 0 END) AS resolved
		FROM display_roots t JOIN task_repositories tr ON tr.task_id = t.id
		LEFT JOIN repositories r ON r.id = tr.repository_id
		GROUP BY tr.task_id, tr.repository_id, repo_slug
	), sidebar_repository_projection AS NOT MATERIALIZED (
		SELECT task_id, COUNT(*) AS repository_count, SUM(resolved) AS resolved_repository_count,
			MAX(repo_slug) AS repository_name,
			` + slugs + ` AS repository_slugs, ` + labels + ` AS repository_labels
		FROM sidebar_repository_members GROUP BY task_id
	), sidebar_repository_groups AS (
		SELECT task_id, ` + groupKey + ` AS group_key, ` + groupLabel + ` AS group_label
		FROM (SELECT task_id, ` + strings.Join(fields, ", ") + ` FROM sidebar_repository_projection repository_projection) projected
	)`
}
