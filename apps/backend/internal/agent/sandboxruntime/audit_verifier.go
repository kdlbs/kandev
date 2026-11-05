package sandboxruntime

import (
	"context"
	"path/filepath"
	"runtime"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (m *Manager) verifyCapturedAudit(ctx context.Context, org string, job *policy.AuditVerification) (*policy.AuditVerificationResult, error) {
	attempt, err := m.policies.ReadAttempt(ctx, org, job.AttemptID)
	if err != nil {
		return nil, err
	}
	bindingDigest, err := policy.AuditBindingDigest(attempt)
	if err != nil || bindingDigest != job.BindingDigest || attempt.Owner.ProviderSessionID != job.ProviderSessionID || attempt.Snapshot.Provider != job.Provider {
		return nil, contract.ErrInvalidPolicy
	}
	binding, err := m.auditProvider(ctx, attempt)
	if err != nil {
		return nil, err
	}
	key, err := m.policies.ReadAuditVerificationKey(ctx, org, job.ID, m.audit.epoch, job.Version)
	if err != nil {
		return nil, err
	}
	var trust *pluginsdk.SandboxAuditTrustV2
	if len(key) > 0 {
		trust = &pluginsdk.SandboxAuditTrustV2{ExpectedPublicKeyPem: key, ExpectedKeyFingerprint: job.Trust.Fingerprint, TrustRevision: job.Trust.RevisionID}
	}
	base := filepath.Join(m.base, "audit-archives")
	archive, err := contract.LoadAuditArchive(ctx, base, base, contract.AuditArchiveKey(job.BindingDigest, job.ArtifactSetDigest), job.BindingDigest, job.ArtifactSetDigest)
	if err != nil {
		return nil, err
	}
	defer func() { _ = archive.Close() }()
	return m.verifyAuditChunks(ctx, binding, attempt, archive, trust)
}

func (m *Manager) auditProvider(ctx context.Context, attempt *policy.Attempt) (plugins.SandboxProviderBinding, error) {
	captured := attempt.Snapshot.Provider
	binding, err := m.selectProvider(ctx, captured.InstallationID, captured.Key)
	if err != nil {
		return binding, err
	}
	if !sameCapturedProvider(captured, binding) || binding.ProtocolVersion() != 2 {
		return binding, plugins.ErrSandboxProviderUnavailable
	}
	description, err := m.providers.DescribeSandboxProvider(ctx, binding, &pluginsdk.DescribeSandboxProviderRequest{Context: auditRequestContext(attempt), Runtime: &pluginsdk.SandboxRuntimeFacts{Os: runtime.GOOS, Architecture: runtime.GOARCH, Transport: sandboxLocalTransport, RuntimeVersion: captured.RuntimeVersion, RuntimeDigest: captured.RuntimeDigest}})
	if err != nil {
		return binding, err
	}
	if len(description.Description.GetCapabilitiesV2().GetAuditVerificationFormats()) == 0 {
		return binding, plugins.ErrSandboxReportUnsupported
	}
	binary, err := m.providers.ReadSandboxRuntime(ctx, binding, description.Description.RuntimeArtifact)
	if err != nil {
		return binding, err
	}
	if binary.Digest != captured.RuntimeDigest || description.Description.RuntimeVersion != captured.RuntimeVersion {
		return binding, plugins.ErrSandboxProviderUnavailable
	}
	return binding, nil
}

func auditRequestContext(attempt *policy.Attempt) *pluginsdk.SandboxRequestContext {
	provider := attempt.Snapshot.Provider
	return &pluginsdk.SandboxRequestContext{ProtocolMajor: provider.ProtocolVersion(), InstallationId: provider.InstallationID, ProviderKey: provider.Key, OperationId: uuid.NewString(), AttemptId: attempt.ID, ExecutionId: attempt.Snapshot.ExecutionID, Generation: attempt.Snapshot.Launch.Generation}
}
