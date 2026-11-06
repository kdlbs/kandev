package sqlite_test

import (
	"context"
	"testing"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// completeExternalUsageMigration finishes the worker's bounded migration,
// including source adoption, before external fixtures discover targets.
func completeExternalUsageMigration(t *testing.T, repo *analyticssqlite.Repository) {
	t.Helper()
	for range 1000 {
		done, err := repo.BackfillUsageHistory(context.Background(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("usage history migration did not finish")
}
