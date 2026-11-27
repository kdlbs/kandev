package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-TASKS-WORKSPACE-SEARCH-001.5
func TestPostgresWorkspaceTaskSearchLiteral(t *testing.T) {
	_ = testutil.PostgresDSNFromEnv(t)
	t.Run("fields", func(t *testing.T) { testLiteralSearchFields(t, openPostgresRepo) })
	t.Run("pagination", func(t *testing.T) { testLiteralSearchPagination(t, openPostgresRepo) })
	t.Run("filters", func(t *testing.T) { testLiteralSearchFilters(t, openPostgresRepo(t)) })
}
