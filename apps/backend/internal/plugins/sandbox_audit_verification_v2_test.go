package plugins

import (
	"strings"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestExplicitAuditVerificationRequiresFrozenSessionSources(t *testing.T) {
	request := &pluginsdk.ReadSandboxReportRequest{VerifyAuditV2: true}
	require.False(t, validSandboxReportRequestV2(request))
	request.ChunkV2 = auditVerificationChunk()
	require.True(t, validSandboxReportRequestV2(request), "verification without an expected key must remain possible")
	require.False(t, sandboxMajorFields(request, 1), "major 1 must never receive the new action")
	request.ChunkV2.ProviderSessionId = ""
	require.False(t, validSandboxReportRequestV2(request))
}

func TestExplicitAuditVerificationBindsTheObservedSessionAndSigner(t *testing.T) {
	request, report := auditVerificationReply()
	require.NoError(t, validateSandboxReportV2(request, report))
	report.Verification.SessionId = "foreign-session"
	require.Error(t, validateSandboxReportV2(request, report))
	report.Verification.SessionId = "captured-session"
	report.Verification.SignerKeyFingerprint = "private provider text"
	require.Error(t, validateSandboxReportV2(request, report))
	report.Verification.SignerKeyFingerprint = strings.Repeat("b", 64)
	report.Verification.SignatureState = "verified"
	report.Verification.ExpectedKeyFingerprint = strings.Repeat("a", 64)
	report.Verification.TrustRevision = "trust-1"
	request.TrustV2 = &pluginsdk.SandboxAuditTrustV2{ExpectedPublicKeyPem: []byte("fixture public key"), ExpectedKeyFingerprint: strings.Repeat("a", 64), TrustRevision: "trust-1"}
	require.Error(t, validateSandboxReportV2(request, report), "a different observed signer cannot be trusted")
	report.Verification.SignerKeyFingerprint = strings.Repeat("a", 64)
	require.NoError(t, validateSandboxReportV2(request, report))
}

func TestExplicitAuditVerificationKeepsUntrustedAndIncompleteResultsBound(t *testing.T) {
	request, report := auditVerificationReply()
	report.Integrity, report.Finalization = "unchecked", "interrupted"
	report.Verification.ChainState, report.Verification.Finalization = "unchecked", "interrupted"
	report.Verification.SignatureState = "unchecked"
	require.NoError(t, validateSandboxReportV2(request, report))
	report.Verification.TrustRevision = "unrequested-trust"
	require.Error(t, validateSandboxReportV2(request, report))
}

func auditVerificationChunk() *pluginsdk.SandboxAuditChunkV2 {
	digest := strings.Repeat("a", 64)
	return &pluginsdk.SandboxAuditChunkV2{OperationHandle: "verify-1", ArtifactSetDigest: digest, SourceId: digest, SourceDigest: digest, Content: []byte("sample"), TotalBytes: 6, Final: true, SourceSchema: "fixture.audit.v1", SourceRelativePath: "audit/captured-session/events.ndjson", ProviderSessionId: "captured-session", SourceCount: 1}
}

func auditVerificationReply() (*pluginsdk.ReadSandboxReportRequest, *pluginsdk.SandboxReportV2) {
	chunk := auditVerificationChunk()
	request := &pluginsdk.ReadSandboxReportRequest{VerifyAuditV2: true, ChunkV2: chunk}
	result := &pluginsdk.SandboxAuditVerificationV2{ArtifactSetDigest: chunk.ArtifactSetDigest, VerifierVersion: "fixture-1", SessionId: "captured-session", EventRoot: chunk.SourceDigest, ChainState: "valid", Finalization: "final", SignatureState: "unsigned", LedgerState: "unavailable"}
	return request, &pluginsdk.SandboxReportV2{OperationHandle: chunk.OperationHandle, ArtifactSetDigest: chunk.ArtifactSetDigest, NextSequence: 1, Integrity: "valid", Finalization: "final", Verification: result}
}
