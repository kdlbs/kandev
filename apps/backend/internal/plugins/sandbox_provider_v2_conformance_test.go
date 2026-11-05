package plugins

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pluginruntime "github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// @covers AC-PLUGINS-SANDBOX-002.1, AC-PLUGINS-SANDBOX-002.3, AC-PLUGINS-SANDBOX-002.5
func TestSandboxV2PackagedContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	binary := filepath.Join(t.TempDir(), "fixture")
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "../../cmd/plugin-fixture")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	content, err := os.ReadFile(binary)
	require.NoError(t, err)
	service, dir, _, _ := newTestServiceWithDir(t)
	service.SetSandboxProvidersEnabled(true)
	service.SetRuntime(pluginruntime.NewManager(dir, service.handleStatusChange, testLogger(t)))
	installed, err := service.Install(ctx, sandboxFixturePackage(t, content, "1.0.0", 2))
	require.NoError(t, err)
	descriptors, err := service.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, descriptors, 2)
	for _, descriptor := range descriptors {
		t.Run(descriptor.Binding.ProviderKey, func(t *testing.T) {
			require.Equal(t, uint32(2), descriptor.Binding.ProtocolVersion())
			request := sandboxFixtureCompileRequest(descriptor.Binding)
			request.Context.ProtocolMajor = 2
			request.Policy.Grants = nil
			request.Policy.PolicyV2 = &pluginsdk.SandboxPolicyV2{SchemaVersion: 2, StaticGrants: &pluginsdk.SandboxPermissionsV2{}, ApprovalEnvelope: &pluginsdk.SandboxPermissionsV2{}}
			assertSandboxV2Calls(t, service, descriptor.Binding, request)
		})
	}
	require.NoError(t, service.Uninstall(ctx, installed.ID))
}

func assertSandboxV2Calls(t *testing.T, service *Service, binding SandboxProviderBinding, request *pluginsdk.CompileSandboxPolicyRequest) {
	t.Helper()
	described, err := service.DescribeSandboxProvider(t.Context(), binding, &pluginsdk.DescribeSandboxProviderRequest{Context: request.Context})
	require.NoError(t, err)
	require.Equal(t, []uint32{2}, described.Description.CapabilitiesV2.PolicySchemas)
	compiled, err := service.CompileSandboxPolicy(t.Context(), binding, request)
	require.NoError(t, err)
	require.True(t, proto.Equal(request.Policy.PolicyV2, compiled.Compiled.EffectivePolicyV2))
	operation := &pluginsdk.SandboxOperationV2{Value: &pluginsdk.SandboxOperationV2_Tcp{Tcp: &pluginsdk.SandboxTCPDestinationV2{Host: "api.example.test", Port: 443, Transport: "tcp"}}}
	channel := &pluginsdk.SandboxServiceBindingV2{ChannelId: "channel-1", ServiceKey: "proxy", RouteId: "route-1"}
	body, err := protojson.Marshal(&pluginsdk.SandboxApprovalV2{RequestId: "request-1", Operation: operation, Actor: &pluginsdk.SandboxApprovalActorV2{Value: &pluginsdk.SandboxApprovalActorV2_RuntimeService{RuntimeService: channel}}, SupportedDecisions: []string{"deny", "allow_once"}})
	require.NoError(t, err)
	approval, err := service.NormalizeSandboxApproval(t.Context(), binding, &pluginsdk.NormalizeSandboxApprovalRequest{Context: request.Context, Policy: request.Policy, VerifiedEvent: body, ServiceBindingV2: channel})
	require.NoError(t, err)
	require.Equal(t, "channel-1", approval.ApprovalV2.Actor.GetRuntimeService().ChannelId)
	observed, err := service.NormalizeSandboxStatus(t.Context(), binding, &pluginsdk.NormalizeSandboxStatusRequest{Context: request.Context, Policy: request.Policy, EvidenceSchema: "fixture.runtime.v2"})
	require.NoError(t, err)
	require.Nil(t, observed.Evidence)
	require.NotEmpty(t, observed.ObservationsV2)
	assertSandboxV2ReportLearning(t, service, binding, request, operation)
}

func assertSandboxV2ReportLearning(t *testing.T, service *Service, binding SandboxProviderBinding, request *pluginsdk.CompileSandboxPolicyRequest, operation *pluginsdk.SandboxOperationV2) {
	t.Helper()
	event := &pluginsdk.SandboxAuditEventV2{RecordId: "denial-1", Operation: operation, Decision: "denied", SourceId: "audit.v2", OccurredAtUnixMs: 123, PolicyDigest: request.Policy.Digest, AttemptId: request.Context.AttemptId, Generation: request.Context.Generation, ExactTarget: true}
	body, err := protojson.Marshal(&pluginsdk.SandboxReportV2{Events: []*pluginsdk.SandboxAuditEventV2{event}, Integrity: "valid", Finalization: "final", Coverage: []*pluginsdk.SandboxReportCoverageV2{{OperationKind: "network", Coverage: "partial"}}})
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)
	report, err := service.ReadSandboxReport(t.Context(), binding, &pluginsdk.ReadSandboxReportRequest{Context: request.Context, Policy: request.Policy, ChunkV2: &pluginsdk.SandboxAuditChunkV2{OperationHandle: "handle-1", ArtifactSetDigest: digest, SourceId: "audit.v2", Content: body, Final: true, TotalBytes: uint64(len(body)), SourceDigest: digest}})
	require.NoError(t, err)
	require.Len(t, report.ReportV2.Events, 1)
	proposed, err := service.ProposeSandboxRevision(t.Context(), binding, &pluginsdk.ProposeSandboxRevisionRequest{Context: request.Context, Policy: request.Policy, SelectedEventsV2: report.ReportV2.Events})
	require.NoError(t, err)
	require.Len(t, proposed.AdditionsV2.StaticGrants.Tcp, 1)
	require.Empty(t, request.Policy.PolicyV2.StaticGrants.Tcp)
}
