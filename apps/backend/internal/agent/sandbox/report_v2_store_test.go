package sandbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTypedReportStoreSQLite(t *testing.T) { runTypedReportStore(t, sqlitePolicyDB(t)) }
func TestTypedReportStorePostgres(t *testing.T) {
	runTypedReportStore(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func runTypedReportStore(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database, 2)
	ctx := operatorContext(t)
	var data ReportData
	require.NoError(t, json.Unmarshal(typedReportFixtureJSON(t), &data))
	succeeded := time.Now().UTC().Truncate(time.Millisecond)
	data.LastSuccessfulReadAt = &succeeded
	first, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, data)
	require.NoError(t, err)
	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	reloaded.SetSessionAuthorizer(func(context.Context, string) error { return nil })
	read, err := reloaded.ReadReport(ctx, "session", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, first, read)
	data.Status = "unavailable"
	data.RefreshError = "unavailable"
	stale, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, first.Version, data)
	require.NoError(t, err)
	require.Equal(t, &succeeded, stale.Data.LastSuccessfulReadAt)
	require.Equal(t, first.Data.V2.Records, stale.Data.V2.Records)
	require.Equal(t, "valid", stale.Data.V2.Integrity, "the last valid evidence remains historical")
	_, err = service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, first.Version, data)
	require.ErrorIs(t, err, ErrConflict)
}

func TestTypedReportCannotReinterpretAMajorOneAttempt(t *testing.T) {
	service, attempt, _ := approvalStoreFixture(t, sqlitePolicyDB(t))
	var data ReportData
	require.NoError(t, json.Unmarshal(typedReportFixtureJSON(t), &data))
	_, err := service.CaptureReport(operatorContext(t), "org", attempt.ID, attempt.Version, 0, data)
	require.ErrorIs(t, err, contract.ErrInvalidPolicy)
}
