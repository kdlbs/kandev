package sandbox

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAuditVerificationHistorySQLite(t *testing.T) {
	runAuditVerificationHistory(t, sqlitePolicyDB(t))
}
func TestAuditVerificationHistoryPostgres(t *testing.T) {
	runAuditVerificationHistory(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func runAuditVerificationHistory(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database, 2)
	ctx := operatorContext(t)
	attempt, err := service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	binding, err := AuditBindingDigest(attempt)
	require.NoError(t, err)
	report, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, ReportData{Status: "unavailable", Integrity: "unverified", Finalized: true, FinalAudit: &FinalAuditReceipt{BindingDigest: binding, ArtifactSetDigest: strings.Repeat("d", 64), CapturedAt: time.Now().UTC()}})
	require.NoError(t, err)
	for range 23 {
		job, err := service.BeginAuditVerification(ctx, "session", attempt.ID, AuditVerificationRequest{RequestID: uuid.NewString(), ExpectedReportDigest: report.Digest, ExpectedArtifactDigest: report.Data.FinalAudit.ArtifactSetDigest}, auditVerificationTestEpoch)
		require.NoError(t, err)
		_, err = service.FailOwnedAuditVerification(ctx, "org", job.ID, auditVerificationTestEpoch, "unavailable")
		require.NoError(t, err)
	}
	first, err := service.ListAuditVerifications(ctx, "session", attempt.ID, "")
	require.NoError(t, err)
	require.Len(t, first.Verifications, 20)
	require.NotEmpty(t, first.NextCursor)
	second, err := service.ListAuditVerifications(ctx, "session", attempt.ID, first.NextCursor)
	require.NoError(t, err)
	require.Len(t, second.Verifications, 3)
	require.Empty(t, second.NextCursor)
	seen := map[string]bool{}
	previous := time.Now().UTC()
	for _, view := range append(first.Verifications, second.Verifications...) {
		job := view.Verification
		require.False(t, seen[job.ID], "history pages must not repeat a result")
		require.LessOrEqual(t, job.RequestedAt.UnixMilli(), previous.UnixMilli(), "history must preserve its indexed request order")
		require.Equal(t, "failed", job.State)
		require.Nil(t, job.StartedAt)
		seen[job.ID], previous = true, job.RequestedAt
	}
	_, err = service.ListAuditVerifications(ctx, "session", attempt.ID, "invalid-cursor")
	require.ErrorIs(t, err, contract.ErrInvalidPolicy)
	_, err = service.ListAuditVerifications(ctx, "session", attempt.ID, uuid.NewString())
	require.ErrorIs(t, err, ErrNotFound)
	_, err = service.ListAuditVerifications(ctx, "foreign-session", attempt.ID, first.NextCursor)
	require.ErrorIs(t, err, ErrNotFound)
}
