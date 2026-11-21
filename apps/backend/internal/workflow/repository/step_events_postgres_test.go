package repository

import (
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// The PostgreSQL twins of the guarded step-events write. They skip unless
// KANDEV_TEST_POSTGRES_DSN is set.

func TestPostgresUpdateStepEventsIfUnchanged_WritesEventsOnlyAndReturnsNewVersion(t *testing.T) {
	exerciseStepEventsWrite(t, setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), 2))
}

func TestPostgresUpdateStepEventsIfUnchanged_StaleStepWritesNothing(t *testing.T) {
	exerciseStaleStepEventsWrite(t, setupPostgresDecisionTestRepo(t, testutil.PostgresDSNFromEnv(t), 2))
}
