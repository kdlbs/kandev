package sandbox

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

const auditVerificationTestEpoch = "3520d416-b15d-4d26-a65c-d874970852ce"
const auditVerificationTestRequest = "13f030e2-e9db-4ff3-9218-adc610b9b225"

func TestAuditVerificationAdmissionSQLite(t *testing.T) {
	runAuditVerificationAdmission(t, sqlitePolicyDB(t))
}
func TestAuditVerificationAdmissionPostgres(t *testing.T) {
	runAuditVerificationAdmission(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func runAuditVerificationAdmission(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database, 2)
	ctx := operatorContext(t)
	attempt, err := service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	binding, err := AuditBindingDigest(attempt)
	require.NoError(t, err)
	data := ReportData{Status: "unavailable", Integrity: "unverified", Finalized: true, FinalAudit: &FinalAuditReceipt{BindingDigest: binding, ArtifactSetDigest: strings.Repeat("d", 64), CapturedAt: time.Now().UTC()}}
	report, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, data)
	require.NoError(t, err)
	request := AuditVerificationRequest{RequestID: auditVerificationTestRequest, ExpectedReportDigest: report.Digest, ExpectedArtifactDigest: data.FinalAudit.ArtifactSetDigest}
	job, err := service.BeginAuditVerification(ctx, "session", attempt.ID, request, auditVerificationTestEpoch)
	require.NoError(t, err)
	require.Equal(t, "pending", job.State)
	require.EqualValues(t, 1, job.Version)
	require.Equal(t, binding, job.BindingDigest)
	require.True(t, job.Trust.Revoked)
	require.NotEmpty(t, job.RequestedBy)
	again, err := service.BeginAuditVerification(ctx, "session", attempt.ID, request, auditVerificationTestEpoch)
	require.NoError(t, err)
	require.Equal(t, job, again, "a retry of the same request must preserve its original history")
	for _, change := range []string{"report", "artifact", "trust", "nonce", "epoch", "another-active"} {
		invalid := request
		epoch := auditVerificationTestEpoch
		switch change {
		case "report":
			invalid.ExpectedReportDigest = strings.Repeat("e", 64)
		case "artifact":
			invalid.ExpectedArtifactDigest = strings.Repeat("e", 64)
		case "trust":
			invalid.ExpectedTrustVersion = 1
		case "nonce":
			invalid.RequestID = "not a request UUID"
		case "epoch":
			epoch = "not a runtime UUID"
		case "another-active":
			invalid.RequestID = "0198a513-212e-49f7-bb6c-e2de1017192e"
		}
		_, err = service.BeginAuditVerification(ctx, "session", attempt.ID, invalid, epoch)
		require.Error(t, err, change)
	}
	member := authn.WithIdentity(t.Context(), authn.Identity{UserID: "member", OrgID: "org", Role: authn.RoleMember, SessionID: "browser"})
	_, err = service.BeginAuditVerification(member, "session", attempt.ID, request, auditVerificationTestEpoch)
	require.ErrorIs(t, err, ErrAuthority)
	restarted, err := NewService(database, true, nil)
	require.NoError(t, err)
	restarted.SetSessionAuthorizer(service.authorizeSession)
	view, err := restarted.ReadAuditVerification(ctx, "session", attempt.ID, job.ID)
	require.NoError(t, err)
	require.Equal(t, job, view.Verification)
	require.False(t, view.SignerTrustedNow)
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), auditVerificationTestEpoch)
	require.NotContains(t, string(raw), attempt.Snapshot.Launch.Root)
	_, err = restarted.ReadAuditVerification(ctx, "another-session", attempt.ID, job.ID)
	require.Error(t, err)
	_, err = service.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{Revoke: true})
	require.NoError(t, err)
	_, err = service.BeginAuditVerification(ctx, "session", attempt.ID, request, auditVerificationTestEpoch)
	require.ErrorIs(t, err, ErrConflict, "changed trust cannot replay an old verification request")
	_, err = database.Exec(database.Rebind(`UPDATE sandbox_audit_verifications SET content_json=replace(content_json,'"state":"pending"','"state":"corrupt"') WHERE org_id=? AND id=?`), "org", job.ID)
	require.NoError(t, err)
	guard, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.ErrorIs(t, service.InterruptAuditVerifications(guard, uuid.NewString()), ErrConflict, "corrupt active work must fail closed without an unbounded restart loop")
}
