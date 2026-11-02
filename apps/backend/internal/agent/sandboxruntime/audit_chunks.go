package sandboxruntime

import (
	"context"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (m *Manager) verifyAuditChunks(ctx context.Context, binding plugins.SandboxProviderBinding, attempt *policy.Attempt, archive *contract.AuditArchive, trust *pluginsdk.SandboxAuditTrustV2) (*policy.AuditVerificationResult, error) {
	handle := uuid.NewString()
	var sequence uint64
	var verified *pluginsdk.SandboxAuditVerificationV2
	wire := wirePolicy(attempt.Snapshot.Resolved, attempt.Snapshot.Grants)
	for index, source := range archive.Sources {
		var offset int64
		for {
			content, final, err := archive.ReadChunk(ctx, index, offset, 256<<10)
			if err != nil {
				return nil, err
			}
			chunk := &pluginsdk.SandboxAuditChunkV2{OperationHandle: handle, ArtifactSetDigest: archive.Digest, SourceId: source.ID, Sequence: sequence, Content: content, Final: final, TotalBytes: uint64(source.Size), SourceDigest: source.Digest, SourceSchema: source.Schema, SourceRelativePath: source.Name, ProviderSessionId: attempt.Owner.ProviderSessionID, SourceIndex: uint32(index), SourceCount: uint32(len(archive.Sources))}
			response, err := m.providers.ReadSandboxReport(ctx, binding, &pluginsdk.ReadSandboxReportRequest{Context: auditRequestContext(attempt), Policy: wire, ChunkV2: chunk, TrustV2: trust, VerifyAuditV2: true})
			if err != nil {
				return nil, err
			}
			if response.ReportV2 == nil {
				return nil, contract.ErrInvalidPolicy
			}
			if response.ReportV2.Verification != nil {
				verified = response.ReportV2.Verification
			}
			offset += int64(len(content))
			sequence++
			if final {
				break
			}
		}
	}
	if verified == nil {
		return nil, contract.ErrInvalidPolicy
	}
	return &policy.AuditVerificationResult{ArtifactSetDigest: verified.ArtifactSetDigest, VerifierVersion: verified.VerifierVersion, SessionID: verified.SessionId, EventRoot: verified.EventRoot, EventCount: verified.EventCount, ChainState: verified.ChainState, Finalization: verified.Finalization, SignatureState: verified.SignatureState, ExpectedKeyFingerprint: verified.ExpectedKeyFingerprint, SignerKeyFingerprint: verified.SignerKeyFingerprint, LedgerState: verified.LedgerState, TrustRevision: verified.TrustRevision}, nil
}
