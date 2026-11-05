package sandbox

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAuditVerificationLifecycleSQLite(t *testing.T) {
	runAuditVerificationLifecycle(t, sqlitePolicyDB(t))
}
func TestAuditVerificationLifecyclePostgres(t *testing.T) {
	runAuditVerificationLifecycle(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func runAuditVerificationLifecycle(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, attempt, _ := approvalStoreFixture(t, database, 2)
	changes := 0
	service.SetAuditChangeNotifier(func() { changes++ })
	ctx := operatorContext(t)
	attempt, err := service.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, AttemptStopped, nil)
	require.NoError(t, err)
	public, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(public)
	require.NoError(t, err)
	trust, err := service.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{PublicKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: auditTrustPublicKeyType, Bytes: der}))})
	require.NoError(t, err)
	binding, err := AuditBindingDigest(attempt)
	require.Equal(t, 1, changes, "committed trust changes must wake historical projection")
	require.NoError(t, err)
	report, err := service.CaptureReport(ctx, "org", attempt.ID, attempt.Version, 0, ReportData{Status: "unavailable", Integrity: "unverified", Finalized: true, FinalAudit: &FinalAuditReceipt{BindingDigest: binding, ArtifactSetDigest: strings.Repeat("d", 64), CapturedAt: time.Now().UTC()}})
	require.NoError(t, err)
	request := AuditVerificationRequest{RequestID: uuid.NewString(), ExpectedReportDigest: report.Digest, ExpectedArtifactDigest: report.Data.FinalAudit.ArtifactSetDigest, ExpectedTrustVersion: trust.Version}
	job, err := service.BeginAuditVerification(ctx, "session", attempt.ID, request, auditVerificationTestEpoch)
	require.NoError(t, err)
	_, err = service.ReadAuditVerificationKey(ctx, "org", job.ID, auditVerificationTestEpoch, job.Version)
	require.ErrorIs(t, err, ErrConflict, "a pending job cannot read verification input")
	_, err = service.ClaimAuditVerification(ctx, "org", job.ID, uuid.NewString(), job.Version)
	require.ErrorIs(t, err, ErrConflict)
	running, err := service.ClaimAuditVerification(ctx, "org", job.ID, auditVerificationTestEpoch, job.Version)
	require.NoError(t, err)
	require.Equal(t, "running", running.State)
	require.NotNil(t, running.StartedAt)
	key, err := service.ReadAuditVerificationKey(ctx, "org", job.ID, auditVerificationTestEpoch, running.Version)
	require.NoError(t, err)
	require.Equal(t, trust.PublicKeyPEM, string(key))
	result := &AuditVerificationResult{ArtifactSetDigest: job.ArtifactSetDigest, VerifierVersion: "fixture.audit.v1", SessionID: job.ProviderSessionID, EventRoot: strings.Repeat("e", 64), EventCount: 3, ChainState: "valid", Finalization: "final", SignatureState: "verified", ExpectedKeyFingerprint: trust.Fingerprint, SignerKeyFingerprint: trust.Fingerprint, LedgerState: "valid", TrustRevision: trust.ID}
	for _, change := range []string{"artifact", "session", "signer", "trust", "incomplete"} {
		invalid := *result
		switch change {
		case "artifact":
			invalid.ArtifactSetDigest = strings.Repeat("f", 64)
		case "session":
			invalid.SessionID = "another-session"
		case "signer":
			invalid.SignerKeyFingerprint = strings.Repeat("f", 64)
		case "trust":
			invalid.TrustRevision = uuid.NewString()
		case "incomplete":
			invalid.Finalization = "interrupted"
		}
		_, err = service.FinishAuditVerification(ctx, "org", job.ID, auditVerificationTestEpoch, running.Version, &invalid, "")
		require.Error(t, err, change)
	}
	completed, err := service.FinishAuditVerification(ctx, "org", job.ID, auditVerificationTestEpoch, running.Version, result, "")
	require.NoError(t, err)
	require.Equal(t, "completed", completed.State)
	require.Equal(t, 2, changes, "committed verification must wake historical projection")
	require.NotNil(t, completed.CompletedAt)
	view, err := service.ReadAuditVerification(ctx, "session", attempt.ID, job.ID)
	require.NoError(t, err)
	require.True(t, view.SignerTrustedNow)
	require.Equal(t, result, view.Verification.Result)
	captured, err := service.ReadCapturedAuditVerification(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, view, captured, "trusted observation must read the same immutable result")
	_, err = service.FinishAuditVerification(ctx, "org", job.ID, auditVerificationTestEpoch, running.Version, result, "")
	require.ErrorIs(t, err, ErrConflict)
	request.RequestID = uuid.NewString()
	pending, err := service.BeginAuditVerification(ctx, "session", attempt.ID, request, auditVerificationTestEpoch)
	require.NoError(t, err)
	_, err = service.ClaimAuditVerification(ctx, "org", pending.ID, auditVerificationTestEpoch, pending.Version)
	require.NoError(t, err)
	restarted, err := NewService(database, true, nil)
	require.NoError(t, err)
	restarted.SetSessionAuthorizer(service.authorizeSession)
	restarted.SetAuditChangeNotifier(func() { changes++ })
	newEpoch := uuid.NewString()
	require.NoError(t, restarted.InterruptAuditVerifications(ctx, newEpoch))
	interrupted, err := restarted.ReadAuditVerification(ctx, "session", attempt.ID, pending.ID)
	require.NoError(t, err)
	require.Equal(t, "interrupted", interrupted.Verification.State)
	require.Equal(t, "restarted", interrupted.Verification.FailureCode)
	require.Equal(t, pending.RequestedAt, interrupted.Verification.RequestedAt)
	request.RequestID = uuid.NewString()
	queued, err := restarted.BeginAuditVerification(ctx, "session", attempt.ID, request, newEpoch)
	require.NoError(t, err)
	require.NoError(t, restarted.InterruptAuditVerifications(ctx, uuid.NewString()))
	interrupted, err = restarted.ReadAuditVerification(ctx, "session", attempt.ID, queued.ID)
	require.NoError(t, err)
	require.Equal(t, "interrupted", interrupted.Verification.State)
	require.Nil(t, interrupted.Verification.StartedAt)
	_, err = restarted.SetAuditTrust(ctx, "session", attempt.ID, AuditTrustRequest{ExpectedVersion: trust.Version, Revoke: true})
	require.NoError(t, err)
	history, err := restarted.ReadAuditVerification(ctx, "session", attempt.ID, job.ID)
	require.Equal(t, 3, changes, "revocation must wake projection without rewriting historical verification")
	require.NoError(t, err)
	require.False(t, history.SignerTrustedNow)
	require.Equal(t, completed, history.Verification, "revocation preserves the original mathematical result and time")
	captured, err = restarted.ReadCapturedAuditVerification(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.Equal(t, history, captured, "a newer interrupted job must preserve the last completed audit")
	_, err = restarted.ReadCapturedAuditVerification(ctx, "foreign", attempt.ID)
	require.ErrorIs(t, err, ErrNotFound)
	request.RequestID = uuid.NewString()
	request.ExpectedTrustVersion = trust.Version + 1
	cancelJob, err := restarted.BeginAuditVerification(ctx, "session", attempt.ID, request, newEpoch)
	require.NoError(t, err)
	_, err = restarted.FailOwnedAuditVerification(ctx, "org", cancelJob.ID, uuid.NewString(), "cancelled")
	require.ErrorIs(t, err, ErrConflict)
	cancelled, err := restarted.FailOwnedAuditVerification(ctx, "org", cancelJob.ID, newEpoch, "cancelled")
	require.NoError(t, err)
	require.Equal(t, "interrupted", cancelled.State)
	require.Equal(t, "cancelled", cancelled.FailureCode)
	require.Equal(t, cancelJob.RequestedAt, cancelled.RequestedAt)
	again, err := restarted.FailOwnedAuditVerification(ctx, "org", cancelJob.ID, newEpoch, "cancelled")
	require.NoError(t, err)
	require.Equal(t, cancelled, again)
}
