package plugins

import (
	"encoding/json"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-SANDBOX-002.1
func TestSandboxV2ContextPinsDeclaredMajor(t *testing.T) {
	binding := SandboxProviderBinding{InstallationID: "installation", ProviderKey: "fixture"}
	require.NoError(t, json.Unmarshal([]byte(`{"protocol_major":2}`), &binding))
	context := &pluginsdk.SandboxRequestContext{ProtocolMajor: 2, InstallationId: "installation", ProviderKey: "fixture", OperationId: "operation", AttemptId: "attempt", ExecutionId: "execution"}
	require.True(t, validSandboxRequestContext(context, binding))
	context.ProtocolMajor = 1
	require.False(t, validSandboxRequestContext(context, binding))
	context.ProtocolMajor = 3
	require.False(t, validSandboxRequestContext(context, binding))
}

func TestSandboxV2ResponseCannotDowngrade(t *testing.T) {
	request := &pluginsdk.SandboxRequestContext{ProtocolMajor: 2, OperationId: "operation"}
	require.True(t, validSandboxResponseContext(&pluginsdk.SandboxResponseContext{SchemaVersion: 2, OperationId: "operation"}, request))
	require.False(t, validSandboxResponseContext(&pluginsdk.SandboxResponseContext{SchemaVersion: 1, OperationId: "operation"}, request))
}

func TestSandboxV2FieldsCannotEnterMajorOne(t *testing.T) {
	service, provider, binding, request := sandboxWireService(t)
	request.Policy.PolicyV2 = &pluginsdk.SandboxPolicyV2{SchemaVersion: 2}
	_, err := service.CompileSandboxPolicy(t.Context(), binding, request)
	require.Error(t, err)
	require.Zero(t, provider.calls.Load())
}

func TestSandboxV2ServiceIdentityMustMatchAuthenticatedBinding(t *testing.T) {
	request := &pluginsdk.NormalizeSandboxApprovalRequest{
		Context:          &pluginsdk.SandboxRequestContext{ProtocolMajor: 2},
		ServiceBindingV2: &pluginsdk.SandboxServiceBindingV2{ChannelId: "channel-1", ServiceKey: "proxy", RouteId: "route-1"},
	}
	result := &pluginsdk.NormalizeSandboxApprovalResponse{ApprovalV2: &pluginsdk.SandboxApprovalV2{
		RequestId:          "request-1",
		Operation:          &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Tcp{Tcp: &pluginsdk.SandboxTCPDestinationV2{Host: "api.example.test", Port: 443, Transport: "tcp"}}},
		Actor:              &pluginsdk.SandboxApprovalActorV2{Value: &pluginsdk.SandboxApprovalActorV2_RuntimeService{RuntimeService: &pluginsdk.SandboxServiceBindingV2{ChannelId: "channel-1", ServiceKey: "proxy", RouteId: "route-1"}}},
		SupportedDecisions: []string{"deny", "allow_once"},
	}}
	require.NoError(t, validateSandboxReplyPayload(request, result))
	result.ApprovalV2.Actor.GetRuntimeService().ChannelId = "foreign"
	require.Error(t, validateSandboxReplyPayload(request, result))
}

func TestSandboxV2RuntimeObservationsDoNotRequireConfinementEvidence(t *testing.T) {
	request := &pluginsdk.NormalizeSandboxStatusRequest{Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}, EvidenceSchema: "runtime.v2"}
	result := &pluginsdk.NormalizeSandboxStatusResponse{ObservationsV2: []*pluginsdk.SandboxRuntimeObservationV2{{SourceId: "runtime.v2", ObservedAtUnixMs: 123, Platform: "linux"}}}
	require.NoError(t, validateSandboxReplyPayload(request, result))
	require.Nil(t, result.Evidence)
	result.ObservationsV2[0].SourceId = "foreign-source"
	require.Error(t, validateSandboxReplyPayload(request, result))
}

func TestSandboxV2ChunkReplyCannotChangeArtifactIdentity(t *testing.T) {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := &pluginsdk.ReadSandboxReportRequest{Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}, ChunkV2: &pluginsdk.SandboxAuditChunkV2{OperationHandle: "operation-1", ArtifactSetDigest: digest, SourceId: "audit.v2", Sequence: 2, Content: []byte("sample"), TotalBytes: 100, SourceDigest: digest}}
	result := &pluginsdk.ReadSandboxReportResponse{ReportV2: &pluginsdk.SandboxReportV2{OperationHandle: "operation-1", ArtifactSetDigest: digest, NextSequence: 3, Integrity: "unchecked", Finalization: "live", Coverage: []*pluginsdk.SandboxReportCoverageV2{{OperationKind: "network", Coverage: "partial"}}}}
	require.NoError(t, validateSandboxReplyPayload(request, result))
	result.ReportV2.ArtifactSetDigest = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	require.Error(t, validateSandboxReplyPayload(request, result))
}

func TestSandboxV2StatusSourceCannotEscapeOrLoseItsSessionBinding(t *testing.T) {
	request := &pluginsdk.NormalizeSandboxStatusRequest{Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}, EvidenceSchema: "audit.v2", SourceRelativePathV2: "nono/audit/session-1/audit-events.ndjson", ProviderSessionIdV2: "session-1"}
	require.True(t, validSandboxRequestPayload(request))
	request.SourceRelativePathV2 = "../foreign/audit-events.ndjson"
	require.False(t, validSandboxRequestPayload(request))
	request.SourceRelativePathV2 = "nono/audit/session-1/audit-events.ndjson"
	request.ProviderSessionIdV2 = ""
	require.False(t, validSandboxRequestPayload(request))
}

func TestSandboxV2AuditVerificationRequiresExplicitTrust(t *testing.T) {
	digest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	request := &pluginsdk.ReadSandboxReportRequest{Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}, ChunkV2: &pluginsdk.SandboxAuditChunkV2{OperationHandle: "handle-1", ArtifactSetDigest: digest, SourceId: "audit.v2", Content: []byte("sample"), TotalBytes: 6, SourceDigest: digest, Final: true}}
	report := &pluginsdk.SandboxReportV2{OperationHandle: "handle-1", ArtifactSetDigest: digest, NextSequence: 1, Integrity: "valid", Finalization: "final", Verification: &pluginsdk.SandboxAuditVerificationV2{ArtifactSetDigest: digest, VerifierVersion: "fixture-1", SessionId: "session-1", EventRoot: digest, ChainState: "valid", Finalization: "final", SignatureState: "verified", ExpectedKeyFingerprint: digest, TrustRevision: "trust-1", LedgerState: "unavailable"}}
	require.Error(t, validateSandboxReportV2(request, report))
	request.TrustV2 = &pluginsdk.SandboxAuditTrustV2{ExpectedPublicKeyPem: []byte("fixture public key"), ExpectedKeyFingerprint: digest, TrustRevision: "trust-1"}
	require.NoError(t, validateSandboxReportV2(request, report))
	report.Verification.ExpectedKeyFingerprint = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	require.Error(t, validateSandboxReportV2(request, report))
}

func TestSandboxV2MissingRuntimeEventIsNotAnRPCFailure(t *testing.T) {
	request := &pluginsdk.NormalizeSandboxStatusRequest{Context: &pluginsdk.SandboxRequestContext{ProtocolMajor: 2}, EvidenceSchema: "runtime.v2"}
	require.NoError(t, validateSandboxReplyPayload(request, &pluginsdk.NormalizeSandboxStatusResponse{}), "the runtime may not emit a conditional event")
	request.Context.ProtocolMajor = 1
	require.Error(t, validateSandboxReplyPayload(request, &pluginsdk.NormalizeSandboxStatusResponse{}))
}
