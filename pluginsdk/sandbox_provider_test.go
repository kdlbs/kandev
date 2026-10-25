package pluginsdk

import (
	"context"
	"strings"
	"testing"

	hcplugin "github.com/hashicorp/go-plugin"
	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-PLUGINS-SANDBOX-001.1
func TestSandboxProviderProtocolIsOptionalAndDataOnly(t *testing.T) {
	service := pluginv1.File_kandev_plugin_v1_plugin_proto.Services().ByName("Plugin")
	assertServiceMethods(t, service, "DescribeSandboxProvider", "CompileSandboxPolicy", "NormalizeSandboxApproval", "NormalizeSandboxStatus", "ReadSandboxReport", "ProposeSandboxRevision")
	assertMessageFields(t, "SandboxRequestContext", "protocol_major", "installation_id", "provider_key", "operation_id", "attempt_id", "execution_id", "generation", "deadline_unix_ms")
	assertMessageFields(t, "SandboxStatusEvidence", "mechanism", "process", "policy_digest", "attempt_id", "generation", "schema_version", "observed_at_unix_ms", "limitations")
	assertMessageOmitsFields(t, "SandboxStatusEvidence", "active", "verified", "shield")
	assertMessageOmitsFields(t, "CompileSandboxPolicyResponse", "child_command", "environment", "operator_token")
}

type sandboxContractFixture struct {
	name string
}

func (*sandboxContractFixture) OnEvent(context.Context, *Event) error { return nil }
func (*sandboxContractFixture) HandleWebhook(context.Context, *WebhookRequest) (*WebhookResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unsupported webhook")
}

func sandboxReply(req *pluginv1.SandboxRequestContext) *pluginv1.SandboxResponseContext {
	return &pluginv1.SandboxResponseContext{OperationId: req.OperationId, SchemaVersion: 1}
}

func (f *sandboxContractFixture) DescribeSandboxProvider(_ context.Context, req *pluginv1.DescribeSandboxProviderRequest) (*pluginv1.DescribeSandboxProviderResponse, error) {
	return &pluginv1.DescribeSandboxProviderResponse{Context: sandboxReply(req.Context), Description: &pluginv1.SandboxProviderDescription{DisplayName: f.name}}, nil
}

func (f *sandboxContractFixture) CompileSandboxPolicy(_ context.Context, req *pluginv1.CompileSandboxPolicyRequest) (*pluginv1.CompileSandboxPolicyResponse, error) {
	return &pluginv1.CompileSandboxPolicyResponse{Context: sandboxReply(req.Context), Compiled: &pluginv1.SandboxCompiledPolicy{
		Artifacts:       []*pluginv1.SandboxArtifact{{Name: "profile.json", ContentUtf8: f.name, Kind: "profile"}},
		EffectiveGrants: req.Policy.Grants,
	}}, nil
}

func (*sandboxContractFixture) NormalizeSandboxApproval(_ context.Context, req *pluginv1.NormalizeSandboxApprovalRequest) (*pluginv1.NormalizeSandboxApprovalResponse, error) {
	return &pluginv1.NormalizeSandboxApprovalResponse{Context: sandboxReply(req.Context), Operation: &pluginv1.SandboxApprovalOperation{RequestId: "request-1"}}, nil
}

func (*sandboxContractFixture) NormalizeSandboxStatus(_ context.Context, req *pluginv1.NormalizeSandboxStatusRequest) (*pluginv1.NormalizeSandboxStatusResponse, error) {
	return &pluginv1.NormalizeSandboxStatusResponse{Context: sandboxReply(req.Context), Evidence: &pluginv1.SandboxStatusEvidence{Process: req.Process, PolicyDigest: req.Policy.Digest}}, nil
}

func (*sandboxContractFixture) ReadSandboxReport(_ context.Context, req *pluginv1.ReadSandboxReportRequest) (*pluginv1.ReadSandboxReportResponse, error) {
	return &pluginv1.ReadSandboxReportResponse{Context: sandboxReply(req.Context), Integrity: "complete", Complete: true}, nil
}

func (*sandboxContractFixture) ProposeSandboxRevision(_ context.Context, req *pluginv1.ProposeSandboxRevisionRequest) (*pluginv1.ProposeSandboxRevisionResponse, error) {
	return &pluginv1.ProposeSandboxRevisionResponse{Context: sandboxReply(req.Context), SourceRecordIds: []string{"record-1"}}, nil
}

func TestSandboxProviderRPCDispatchUsesOptionalInterface(t *testing.T) {
	for _, name := range []string{"First provider", "Different provider"} {
		t.Run(name, func(t *testing.T) {
			gp := &GRPCPlugin{Impl: &sandboxContractFixture{name: name}}
			client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{PluginMapKey: gp})
			defer func() { _ = client.Close() }()
			defer server.Stop()
			raw, err := client.Dispense(PluginMapKey)
			require.NoError(t, err)
			remote := raw.(*RemotePlugin)
			response, err := remote.client.CompileSandboxPolicy(context.Background(), &pluginv1.CompileSandboxPolicyRequest{
				Context: &pluginv1.SandboxRequestContext{OperationId: "operation-1", ProtocolMajor: 1},
				Policy:  &pluginv1.SandboxPolicySnapshot{Grants: &pluginv1.SandboxGrants{KandevMcp: true}},
			})
			require.NoError(t, err)
			require.Equal(t, "operation-1", response.Context.OperationId)
			require.Equal(t, name, response.Compiled.Artifacts[0].ContentUtf8)
			require.True(t, response.Compiled.EffectiveGrants.KandevMcp)
		})
	}
}

func TestSandboxProviderAbsentCapabilityRemainsUnsupported(t *testing.T) {
	server := &grpcPluginServer{impl: &fakeAuthorPlugin{}}
	_, err := server.DescribeSandboxProvider(context.Background(), &pluginv1.DescribeSandboxProviderRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

type nilSandboxFixture struct{ sandboxContractFixture }

func (*nilSandboxFixture) CompileSandboxPolicy(context.Context, *pluginv1.CompileSandboxPolicyRequest) (*pluginv1.CompileSandboxPolicyResponse, error) {
	return nil, nil
}

func TestSandboxProviderRejectsNilReply(t *testing.T) {
	server := &grpcPluginServer{impl: &nilSandboxFixture{}}
	_, err := server.CompileSandboxPolicy(context.Background(), &pluginv1.CompileSandboxPolicyRequest{})
	require.Equal(t, codes.Internal, status.Code(err))
}

type oversizedSandboxFixture struct{ sandboxContractFixture }

func (*oversizedSandboxFixture) CompileSandboxPolicy(context.Context, *pluginv1.CompileSandboxPolicyRequest) (*pluginv1.CompileSandboxPolicyResponse, error) {
	return &pluginv1.CompileSandboxPolicyResponse{Compiled: &pluginv1.SandboxCompiledPolicy{
		Artifacts: []*pluginv1.SandboxArtifact{{ContentUtf8: strings.Repeat("x", (1<<20)+1)}},
	}}, nil
}

func TestSandboxProviderTransportBoundsResponse(t *testing.T) {
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		PluginMapKey: &GRPCPlugin{Impl: &oversizedSandboxFixture{}},
	})
	defer func() { _ = client.Close() }()
	defer server.Stop()
	raw, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	_, err = raw.(*RemotePlugin).CompileSandboxPolicy(context.Background(), &CompileSandboxPolicyRequest{})
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}
