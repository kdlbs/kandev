package sqlite

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/db/dialect"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

func TestTaskListOrderBy_UsesDialectTitleOrdering(t *testing.T) {
	sqliteOrder := taskListOrderBy("sqlite3", "t", usermodels.TasksListSortTitleAsc)
	if !strings.Contains(sqliteOrder, "t.title COLLATE NOCASE ASC") {
		t.Fatalf("sqlite order = %q, want NOCASE title ordering", sqliteOrder)
	}

	postgresOrder := taskListOrderBy(dialect.PGX, "t", usermodels.TasksListSortTitleAsc)
	if strings.Contains(postgresOrder, "COLLATE NOCASE") {
		t.Fatalf("postgres order = %q, must not use SQLite-only COLLATE NOCASE", postgresOrder)
	}
	if !strings.Contains(postgresOrder, "LOWER(t.title) ASC") {
		t.Fatalf("postgres order = %q, want LOWER(title) ordering", postgresOrder)
	}
}

func TestTaskSearchSelectQuery_OrdersOutsideDistinctForPostgres(t *testing.T) {
	query := taskSearchSelectQuery(dialect.PGX, "", "ILIKE", usermodels.TasksListSortTitleAsc)
	distinctIndex := strings.Index(query, "SELECT DISTINCT")
	orderIndex := strings.LastIndex(query, "ORDER BY")
	if distinctIndex == -1 {
		t.Fatalf("query = %q, want SELECT DISTINCT", query)
	}
	if orderIndex == -1 {
		t.Fatalf("query = %q, want ORDER BY", query)
	}
	if orderIndex < distinctIndex {
		t.Fatalf("query = %q, ORDER BY must be outside SELECT DISTINCT subquery", query)
	}
	if strings.Contains(query[distinctIndex:orderIndex], "LOWER(") {
		t.Fatalf("query = %q, distinct subquery must not order by LOWER(title)", query)
	}
	if !strings.Contains(query, "ORDER BY LOWER(task_search.title) ASC") {
		t.Fatalf("query = %q, want outer Postgres title ordering", query)
	}
}
