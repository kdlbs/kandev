package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// completeUsageSourceAdoptionForTest finishes the bounded adoption cursor so
// target discovery is ready in fixtures that do not run the background worker.
func completeUsageSourceAdoptionForTest(t *testing.T, repo *Repository) {
	t.Helper()
	for range 1000 {
		done, err := repo.BackfillUsageSourceAdoption(t.Context(), usageHistoryBatchLimit)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("usage source adoption did not finish")
}
