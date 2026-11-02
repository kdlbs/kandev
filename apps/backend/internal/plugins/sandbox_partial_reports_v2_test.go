package plugins

import (
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestIncompleteAuditIdentityCanBeReadButCannotAuthorizeAccess(t *testing.T) {
	req := &pluginsdk.ReadSandboxReportRequest{Context: &pluginsdk.SandboxRequestContext{AttemptId: "attempt", Generation: 2}, Policy: &pluginsdk.SandboxPolicySnapshot{Digest: "policy"}}
	command := &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Command{Command: &pluginsdk.SandboxCommandInvocationV2{CommandId: "git", Caller: "shell"}}}
	event := &pluginsdk.SandboxAuditEventV2{RecordId: "record", SourceId: "source", Operation: command, Decision: "denied", OccurredAtUnixMs: 1, PolicyDigest: "policy", AttemptId: "attempt", Generation: 2}
	require.True(t, validSandboxEventV2(req, event), "a redacted report remains useful for inspection")
	require.False(t, validSandboxOperationV2(command), "approval and policy validation still require the complete identity")
	event.ExactTarget = true
	require.False(t, validSandboxEventV2(req, event))
	event.ExactTarget = false
	command.GetCommand().Argv = []string{"secret-private-argument"}
	require.False(t, validSandboxEventV2(req, event), "incomplete reports cannot carry raw argv")
}

func TestAuditEventsCannotChangeTheChunkSource(t *testing.T) {
	req := &pluginsdk.ReadSandboxReportRequest{Context: &pluginsdk.SandboxRequestContext{AttemptId: "attempt", Generation: 2}, Policy: &pluginsdk.SandboxPolicySnapshot{Digest: "policy"}, ChunkV2: &pluginsdk.SandboxAuditChunkV2{SourceId: "owned-source"}}
	event := &pluginsdk.SandboxAuditEventV2{RecordId: "record", SourceId: "foreign-source", Operation: &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Filesystem{Filesystem: &pluginsdk.SandboxFilesystemOperationV2{Path: "/tmp/target", Access: "read"}}}, Decision: "denied", OccurredAtUnixMs: 1, PolicyDigest: "policy", AttemptId: "attempt", Generation: 2, ExactTarget: true}
	require.False(t, validSandboxEventV2(req, event))
	event.SourceId = "owned-source"
	require.True(t, validSandboxEventV2(req, event))
}

func TestAuditReasonsCannotExposePrivateErrors(t *testing.T) {
	require.True(t, validSandboxAuditReasonV2("upstream_unavailable"))
	require.False(t, validSandboxAuditReasonV2("/private/runtime/error?token=redacted-fixture"))
}

func TestPartialEndpointReportsDoNotInventAnOriginOrAuthorizePlainHTTP(t *testing.T) {
	req := &pluginsdk.ReadSandboxReportRequest{Context: &pluginsdk.SandboxRequestContext{AttemptId: "attempt", Generation: 2}, Policy: &pluginsdk.SandboxPolicySnapshot{Digest: "policy"}}
	operation := &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_HttpEndpoint{HttpEndpoint: &pluginsdk.SandboxHTTPEndpointV2{Method: "POST", Path: "/v1/items"}}}
	event := &pluginsdk.SandboxAuditEventV2{RecordId: "record", SourceId: "source", Operation: operation, Decision: "denied", OccurredAtUnixMs: 1, PolicyDigest: "policy", AttemptId: "attempt", Generation: 2}
	require.True(t, validSandboxEventV2(req, event), "missing route/origin remains a visibly partial endpoint report")
	require.False(t, validSandboxOperationV2(operation))
	operation.GetHttpEndpoint().Origin = "http://localhost:8080"
	require.True(t, validSandboxEventV2(req, event), "an observed plain-HTTP request is inspection-only")
	event.ExactTarget = true
	require.False(t, validSandboxEventV2(req, event), "the policy contract still refuses that incomplete/plain-HTTP grant")
	event.ExactTarget = false
	operation.GetHttpEndpoint().Query = "token=synthetic-private"
	require.False(t, validSandboxEventV2(req, event))
	operation.GetHttpEndpoint().Query = ""
	operation.GetHttpEndpoint().Origin = "https://user:synthetic-private@api.example.test"
	require.False(t, validSandboxEventV2(req, event))
}

func TestRedactedNetworkReportDoesNotInventAHostname(t *testing.T) {
	req := &pluginsdk.ReadSandboxReportRequest{Context: &pluginsdk.SandboxRequestContext{AttemptId: "attempt", Generation: 2}, Policy: &pluginsdk.SandboxPolicySnapshot{Digest: "policy"}}
	operation := &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Tcp{Tcp: &pluginsdk.SandboxTCPDestinationV2{Port: 8443, Transport: "tcp"}}}
	event := &pluginsdk.SandboxAuditEventV2{RecordId: "record", SourceId: "source", Operation: operation, Decision: "denied", OccurredAtUnixMs: 1, PolicyDigest: "policy", AttemptId: "attempt", Generation: 2}
	require.True(t, validSandboxEventV2(req, event))
	require.False(t, validSandboxOperationV2(operation))
	event.ExactTarget = true
	require.False(t, validSandboxEventV2(req, event))
}
