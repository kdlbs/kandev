package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresWorkflowAllocationConcurrentTransactions(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	verifyWorkflowAllocationConcurrentTransactions(t, repo)
}
