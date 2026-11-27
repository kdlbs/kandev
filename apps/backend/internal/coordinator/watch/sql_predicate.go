package watch

import "strings"

// TaskSQL is the SQL form of InProjects, for queries whose task alias is t.
// Empty repository links do not establish a repository binding. Membership
// comes from Resolver, including its workspace and live-repository checks.
func TaskSQL(projects Resolved) (string, []any) {
	if !projects.selected {
		return "TRUE", nil
	}
	var clauses []string
	if projects.includeNoRepo {
		clauses = append(clauses, `NOT EXISTS (SELECT 1 FROM task_repositories tr WHERE tr.task_id = t.id AND tr.repository_id <> '')`)
	}
	ids := projects.RepositoryIDs()
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if len(ids) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		clauses = append(clauses, `EXISTS (SELECT 1 FROM task_repositories tr WHERE tr.task_id = t.id AND tr.repository_id IN (`+marks+`))`)
	}
	if len(clauses) == 0 {
		return "FALSE", nil
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}
