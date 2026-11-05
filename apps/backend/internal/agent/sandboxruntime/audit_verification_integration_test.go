//go:build linux

package sandboxruntime

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/stretchr/testify/require"
)

func assertRuntimeAuditVerification(t *testing.T, ctx context.Context, prepared *Prepared, report *policy.Report) {
	t.Helper()
	manager, policies := prepared.manager, prepared.manager.policies
	require.NoError(t, manager.InitializeAuditVerifications(ctx))
	trust, err := policies.GetAuditTrust(ctx, report.SessionID, report.AttemptID)
	require.NoError(t, err)
	request := policy.AuditVerificationRequest{RequestID: uuid.NewString(), ExpectedReportDigest: report.Digest, ExpectedArtifactDigest: report.Data.FinalAudit.ArtifactSetDigest, ExpectedTrustVersion: trust.Version}
	job, err := manager.EnqueueAuditVerification(ctx, report.SessionID, report.AttemptID, request)
	require.NoError(t, err)
	var view *policy.AuditVerificationView
	require.Eventually(t, func() bool {
		view, err = policies.ReadAuditVerification(ctx, report.SessionID, report.AttemptID, job.ID)
		return err == nil && (view.Verification.State == "completed" || view.Verification.State == "failed" || view.Verification.State == "interrupted")
	}, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, "completed", view.Verification.State, view.Verification.FailureCode)
	require.NotNil(t, view.Verification.Result)
	result := view.Verification.Result
	require.Equal(t, "unsigned", result.SignatureState, "the default managed launch does not provision a signing key")
	require.Equal(t, "valid", result.ChainState)
	require.Equal(t, "final", result.Finalization)
	require.Equal(t, "valid", result.LedgerState)
	require.Contains(t, result.VerifierVersion, "nono-0.81.0")
	require.Equal(t, report.Data.FinalAudit.ArtifactSetDigest, result.ArtifactSetDigest)
	require.False(t, view.SignerTrustedNow)
	completed := view.Verification
	again, err := manager.EnqueueAuditVerification(ctx, report.SessionID, report.AttemptID, request)
	require.NoError(t, err)
	require.Equal(t, completed, again, "a repeated request must return history instead of executing again")
	read, err := policies.ReadCapturedReport(ctx, prepared.orgID, report.AttemptID)
	require.NoError(t, err)
	require.Equal(t, report, read, "offline verification cannot replace normalized rows or their read time")
	assertAuditWorkerCapacity(t, ctx, manager, report, request)
	request.RequestID = uuid.NewString()
	pending, err := policies.BeginAuditVerification(ctx, report.SessionID, report.AttemptID, request, uuid.NewString())
	require.NoError(t, err)
	restarted := NewManager(policies, manager.providers, manager.base)
	t.Cleanup(func() { require.NoError(t, restarted.Close()) })
	require.NoError(t, restarted.InitializeAuditVerifications(ctx))
	interrupted, err := policies.ReadAuditVerification(ctx, report.SessionID, report.AttemptID, pending.ID)
	require.NoError(t, err)
	require.Equal(t, "interrupted", interrupted.Verification.State)
	require.Equal(t, "restarted", interrupted.Verification.FailureCode)
	require.Equal(t, pending.RequestedAt, interrupted.Verification.RequestedAt)
	history, err := policies.ReadAuditVerification(ctx, report.SessionID, report.AttemptID, job.ID)
	require.NoError(t, err)
	require.Equal(t, completed, history.Verification)
	observed, err := restarted.Observe(ctx, prepared.orgID, report.AttemptID)
	require.NoError(t, err)
	require.Equal(t, "valid", observed.Facts.Audit.Integrity)
	require.Equal(t, "final", observed.Facts.Audit.Finalization)
	require.Equal(t, "unsigned", observed.Facts.Audit.Signer)
	require.NotNil(t, observed.Facts.Audit.Verification)
	require.Equal(t, *completed.CompletedAt, observed.Facts.Audit.Verification.CompletedAt)
	require.Equal(t, completed.ID, observed.Facts.Audit.Verification.ID)
	require.Equal(t, "stopped", observed.Status, "audit validity cannot activate confinement")
	require.NoError(t, manager.providers.Disable(prepared.binding.PluginID))
	defer func() { require.NoError(t, manager.providers.Enable(prepared.binding.PluginID)) }()
	withoutProvider, err := restarted.Observe(ctx, prepared.orgID, report.AttemptID)
	require.NoError(t, err)
	require.Equal(t, observed.Facts.Audit, withoutProvider.Facts.Audit, "captured verification must not need a provider RPC")
}

func assertAuditWorkerCapacity(t *testing.T, ctx context.Context, manager *Manager, report *policy.Report, request policy.AuditVerificationRequest) {
	t.Helper()
	manager.mu.Lock()
	var ids []string
	for range maximumAuditJobs {
		id := uuid.NewString()
		ids = append(ids, id)
		manager.audit.jobs[id] = &auditJob{}
	}
	manager.mu.Unlock()
	defer func() {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		for _, id := range ids {
			delete(manager.audit.jobs, id)
		}
	}()
	request.RequestID = uuid.NewString()
	job, err := manager.EnqueueAuditVerification(ctx, report.SessionID, report.AttemptID, request)
	require.NoError(t, err)
	require.Equal(t, "failed", job.State)
	require.Equal(t, "limit_exceeded", job.FailureCode)
	require.Nil(t, job.StartedAt, "a refused job must not start a verifier")
	again, err := manager.EnqueueAuditVerification(ctx, report.SessionID, report.AttemptID, request)
	require.NoError(t, err)
	require.Equal(t, job, again)
}
