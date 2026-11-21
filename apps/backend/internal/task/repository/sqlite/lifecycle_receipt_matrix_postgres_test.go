package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresLifecycleReceiptAdmissionRefusalLeavesTaskAndReceiptUntouched(t *testing.T) {
	testLifecycleReceiptAdmissionRefusalLeavesTaskAndReceiptUntouched(t, newPostgresLifecycleReceiptMatrixRepo(t))
}

func TestPostgresLifecycleReceiptPromotionRefusalAndReplayDoNotAdvance(t *testing.T) {
	testLifecycleReceiptPromotionRefusalAndReplayDoNotAdvance(t, newPostgresLifecycleReceiptMatrixRepo(t))
}

func newPostgresLifecycleReceiptMatrixRepo(t *testing.T) *Repository {
	t.Helper()
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	return repo
}
