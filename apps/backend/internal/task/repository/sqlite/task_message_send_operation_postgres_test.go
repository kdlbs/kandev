package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresSendOperationStoreBehavior is the PostgreSQL counterpart to
// TestSendOperationStoreBehavior: the identity index uses COLLATE "C" and the
// unique violation is classified by constraint name on this dialect.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSendOperationStoreBehavior(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	exerciseSendOperationStore(t, repo)
}
