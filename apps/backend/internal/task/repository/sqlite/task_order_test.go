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
