package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresCreateMessageAndQueueOnce(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := seedPostgresPlanCommentMessageFixture(t, context.Background(), repo, "once")
	runQueuedMessageOnceScenarios(t, repo, base)
}
