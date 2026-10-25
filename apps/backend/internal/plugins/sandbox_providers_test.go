package plugins

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type sandboxWireFixture struct {
	compile func(context.Context, *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error)
	calls   atomic.Int32
}

func TestSandboxProviderFeatureDisabled(t *testing.T) {
	svc, provider, binding, req := sandboxWireService(t)
	svc.SetSandboxProvidersEnabled(false)
	_, err := svc.CompileSandboxPolicy(t.Context(), binding, req)
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	require.Zero(t, provider.calls.Load())
	listed, err := svc.ListSandboxProviders(t.Context())
	require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	require.Empty(t, listed)
}

func (*sandboxWireFixture) OnEvent(context.Context, *pluginsdk.Event) error { return nil }
func (*sandboxWireFixture) HandleWebhook(context.Context, *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unsupported webhook")
}

func (f *sandboxWireFixture) CompileSandboxPolicy(ctx context.Context, req *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
	f.calls.Add(1)
	if f.compile != nil {
		return f.compile(ctx, req)
	}
	_, compiled := sandboxCompilationFixture()
	return &pluginsdk.CompileSandboxPolicyResponse{Context: &pluginsdk.SandboxResponseContext{SchemaVersion: 1, OperationId: req.Context.OperationId}, Compiled: compiled}, nil
}

func (*sandboxWireFixture) DescribeSandboxProvider(context.Context, *pluginsdk.DescribeSandboxProviderRequest) (*pluginsdk.DescribeSandboxProviderResponse, error) {
	return nil, status.Error(codes.Unimplemented, "fixture describes compilation only")
}
func (*sandboxWireFixture) NormalizeSandboxApproval(context.Context, *pluginsdk.NormalizeSandboxApprovalRequest) (*pluginsdk.NormalizeSandboxApprovalResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unsupported approval")
}
func (*sandboxWireFixture) NormalizeSandboxStatus(context.Context, *pluginsdk.NormalizeSandboxStatusRequest) (*pluginsdk.NormalizeSandboxStatusResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unsupported status")
}
func (*sandboxWireFixture) ReadSandboxReport(context.Context, *pluginsdk.ReadSandboxReportRequest) (*pluginsdk.ReadSandboxReportResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unsupported report")
}
func (*sandboxWireFixture) ProposeSandboxRevision(context.Context, *pluginsdk.ProposeSandboxRevisionRequest) (*pluginsdk.ProposeSandboxRevisionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unsupported learning")
}

func sandboxWireService(t *testing.T) (*Service, *sandboxWireFixture, SandboxProviderBinding, *pluginsdk.CompileSandboxPolicyRequest) {
	t.Helper()
	svc, _, runtime := newTestService(t)
	svc.SetSandboxProvidersEnabled(true)
	provider := &sandboxWireFixture{}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Impl: provider},
	})
	t.Cleanup(func() { server.Stop(); _ = client.Close() })
	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	require.NoError(t, err)
	runtime.setRemote(raw.(*pluginsdk.RemotePlugin))
	record := &store.Record{Manifest: manifest.Manifest{
		ID: "fixture-policy", Version: "1.0.0", APIVersion: 2,
		Capabilities:     manifest.Capabilities{SandboxProvider: true},
		SandboxProviders: []manifest.SandboxProvider{{Key: "compiler", ProtocolMajor: 1, DisplayName: "Compiler"}},
	}, InstallationID: "installation-1", InstalledAt: time.Now().UTC(), Status: StatusActive}
	require.NoError(t, svc.store.Save(record))
	svc.registry.Add(record)
	runtime.running[record.ID] = true
	binding := SandboxProviderBinding{PluginID: record.ID, InstallationID: record.InstallationID, Version: record.Version, ProviderKey: "compiler", InstalledAt: record.InstalledAt}
	req, _ := sandboxCompilationFixture()
	req.Context = &pluginsdk.SandboxRequestContext{
		ProtocolMajor: 1, InstallationId: record.InstallationID, ProviderKey: "compiler",
		OperationId: "operation-1", AttemptId: "attempt-1", ExecutionId: "execution-1", Generation: 4,
	}
	return svc, provider, binding, req
}

// @covers AC-PLUGINS-SANDBOX-001.2
func TestSandboxProviderHostDispatchPinsInstallation(t *testing.T) {
	svc, provider, binding, req := sandboxWireService(t)
	response, err := svc.CompileSandboxPolicy(context.Background(), binding, req)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, int32(1), provider.calls.Load())
	for _, mutate := range []func(*SandboxProviderBinding){
		func(b *SandboxProviderBinding) { b.InstallationID = "foreign" },
		func(b *SandboxProviderBinding) { b.Version = "2.0.0" },
		func(b *SandboxProviderBinding) { b.InstalledAt = b.InstalledAt.Add(time.Second) },
		func(b *SandboxProviderBinding) { b.ProviderKey = "undeclared" },
		func(b *SandboxProviderBinding) { b.PackageDigest = "different-package" },
	} {
		changed := binding
		mutate(&changed)
		response, err = svc.CompileSandboxPolicy(context.Background(), changed, req)
		require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
		require.Nil(t, response)
	}
	require.Equal(t, int32(1), provider.calls.Load())
}

func TestSandboxProviderCatalogRetainsUnavailableSelections(t *testing.T) {
	svc, _, binding, _ := sandboxWireService(t)
	record, err := svc.Get(binding.PluginID)
	require.NoError(t, err)
	record.SandboxProviders = append(record.SandboxProviders, manifest.SandboxProvider{Key: "second", DisplayName: "Other compiler", ProtocolMajor: 1})
	svc.registry.Add(record)
	providers, err := svc.ListSandboxProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 2)
	require.Equal(t, "Compiler", providers[0].DisplayName)
	require.Equal(t, "Other compiler", providers[1].DisplayName)
	require.True(t, providers[0].PluginRunning)
	record.Status = StatusDisabled
	svc.registry.Add(record)
	providers, err = svc.ListSandboxProviders(context.Background())
	require.NoError(t, err)
	require.Len(t, providers, 2)
	require.False(t, providers[0].PluginRunning)
	require.Equal(t, binding.InstallationID, providers[0].Binding.InstallationID)
}

func TestSandboxProviderHostRejectsMalformedReply(t *testing.T) {
	svc, provider, binding, req := sandboxWireService(t)
	provider.compile = func(context.Context, *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
		_, compiled := sandboxCompilationFixture()
		return &pluginsdk.CompileSandboxPolicyResponse{Context: &pluginsdk.SandboxResponseContext{SchemaVersion: 1, OperationId: "wrong-operation"}, Compiled: compiled}, nil
	}
	response, err := svc.CompileSandboxPolicy(context.Background(), binding, req)
	require.Error(t, err)
	require.Nil(t, response)
}

func TestSandboxProviderHostRejectsUntrustedContextAndDisabledProvider(t *testing.T) {
	for _, mutation := range []string{"protocol", "installation", "operation", "attempt", "oversized", "disabled", "permission", "process"} {
		t.Run(mutation, func(t *testing.T) {
			svc, provider, binding, req := sandboxWireService(t)
			switch mutation {
			case "protocol":
				req.Context.ProtocolMajor = 2
			case "installation":
				req.Context.InstallationId = "foreign"
			case "operation":
				req.Context.OperationId = ""
			case "attempt":
				req.Context.AttemptId = ""
			case "oversized":
				req.Policy.PolicyId = strings.Repeat("x", (1<<20)+1)
			case "disabled", "permission":
				record, err := svc.Get(binding.PluginID)
				require.NoError(t, err)
				if mutation == "disabled" {
					record.Status = StatusDisabled
				} else {
					record.Capabilities.SandboxProvider = false
				}
				svc.registry.Add(record)
			case "process":
				svc.runtime.Stop(binding.PluginID)
			}
			response, err := svc.CompileSandboxPolicy(context.Background(), binding, req)
			require.Error(t, err)
			require.Nil(t, response)
			require.Zero(t, provider.calls.Load())
		})
	}
}

func TestSandboxProviderHostBoundsDeadlineAndSanitizesFailure(t *testing.T) {
	svc, provider, binding, req := sandboxWireService(t)
	provider.compile = func(ctx context.Context, received *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
		if received.Context.DeadlineUnixMs == 0 {
			return nil, status.Error(codes.Internal, "private diagnostic")
		}
		<-ctx.Done()
		return nil, status.Error(codes.Internal, "private diagnostic")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	response, err := svc.CompileSandboxPolicy(ctx, binding, req)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "private diagnostic")
	require.Nil(t, response)
	require.Zero(t, req.Context.DeadlineUnixMs, "the frozen caller request must remain unchanged")
}

func TestSandboxProviderHostRejectsUnknownProtocolFields(t *testing.T) {
	for _, reply := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "reply"}[reply], func(t *testing.T) {
			svc, provider, binding, req := sandboxWireService(t)
			unknown := []byte{0x80, 0x06, 0x01}
			if reply {
				provider.compile = func(_ context.Context, received *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
					_, compiled := sandboxCompilationFixture()
					compiled.Prefix[0].ProtoReflect().SetUnknown(unknown)
					return &pluginsdk.CompileSandboxPolicyResponse{Context: &pluginsdk.SandboxResponseContext{SchemaVersion: 1, OperationId: received.Context.OperationId}, Compiled: compiled}, nil
				}
			} else {
				req.Context.ProtoReflect().SetUnknown(unknown)
			}
			response, err := svc.CompileSandboxPolicy(context.Background(), binding, req)
			require.Error(t, err)
			require.Nil(t, response)
		})
	}
}

func TestSandboxProviderDisableCancelsInflightCompilation(t *testing.T) {
	assertSandboxCancellation(t, func(s *Service, id string) error { return s.Disable(id) }, true)
}

func TestSandboxProviderConfigurationRestartCancelsInflightCompilation(t *testing.T) {
	assertSandboxCancellation(t, func(s *Service, id string) error { return s.UpdateConfig(context.Background(), id, map[string]any{}) }, false)
}

func TestSandboxProviderUninstallCancelsInflightCompilation(t *testing.T) {
	assertSandboxCancellation(t, func(s *Service, id string) error { return s.Uninstall(context.Background(), id) }, true)
}

func assertSandboxCancellation(t *testing.T, change func(*Service, string) error, unavailable bool) {
	t.Helper()
	svc, provider, binding, req := sandboxWireService(t)
	entered := make(chan struct{})
	finished := make(chan struct{})
	provider.compile = func(ctx context.Context, _ *pluginsdk.CompileSandboxPolicyRequest) (*pluginsdk.CompileSandboxPolicyResponse, error) {
		close(entered)
		defer close(finished)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	callDone := make(chan error, 1)
	go func() { _, err := svc.CompileSandboxPolicy(ctx, binding, req); callDone <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("compilation did not start")
	}
	disableDone := make(chan error, 1)
	go func() { disableDone <- change(svc, binding.PluginID) }()
	select {
	case err := <-callDone:
		require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
	case <-time.After(time.Second):
		cancel()
		<-callDone
		<-disableDone
		t.Fatal("lifecycle change did not cancel the admitted compilation")
	}
	require.NoError(t, <-disableDone)
	<-finished
	provider.compile = nil
	response, err := svc.CompileSandboxPolicy(context.Background(), binding, req)
	if unavailable {
		require.ErrorIs(t, err, ErrSandboxProviderUnavailable)
		require.Nil(t, response)
	} else {
		require.NoError(t, err)
		require.NotNil(t, response)
	}
}

func TestSandboxProviderDoesNotReceiveCallbackSecretReferences(t *testing.T) {
	for _, key := range []string{sandboxApprovalEndpoint, sandboxApprovalToken} {
		svc, provider, binding, req := sandboxWireService(t)
		req.References = []*pluginsdk.SandboxRuntimeReference{{Key: key, Value: "private-value"}}
		_, err := svc.CompileSandboxPolicy(t.Context(), binding, req)
		require.Error(t, err)
		require.Zero(t, provider.calls.Load())
	}
}
