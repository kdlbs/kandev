package plugins

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kandev/kandev/internal/plugins/executortunnel"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	pluginv1 "github.com/kdlbs/kandev/pluginsdk/proto/kandev/plugin/v1"
)

const (
	tunnelPluginID       = "kandev-plugin-tunnel-test"
	tunnelInstallationID = "installation-tunnel"
	tunnelControlPort    = 39100
)

// tunnelProviderPlugin acknowledges every binding and echoes data back. It
// records whether the host's end of each stream was canceled.
type tunnelProviderPlugin struct {
	lifecycleExecutorProviderPlugin
	opened   chan *pluginsdk.ExecutorTunnelHeader
	canceled chan struct{}
}

func (p *tunnelProviderPlugin) OpenExecutorTunnel(ctx context.Context, stream pluginsdk.ExecutorTunnelStream) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	p.opened <- frame.GetHeader()
	if err := stream.Send(&pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Ack{
		Ack: &pluginsdk.ExecutorTunnelAck{BindingNonce: frame.GetHeader().GetBindingNonce()},
	}}); err != nil {
		return err
	}
	for {
		data, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				close(p.canceled)
			}
			return err
		}
		if err := stream.Send(&pluginsdk.ExecutorTunnelFrame{Kind: &pluginv1.ExecutorTunnelFrame_Data{Data: data.GetData()}}); err != nil {
			return err
		}
	}
}

func (p *tunnelProviderPlugin) ResolveExecutorConnection(context.Context, *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	return &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{Transport: "host_tunnel", Generation: "g1"}}, nil
}

func TestResolveExecutorConnectionRejectsAHostTunnelLeaseFromAnUngatedProvider(t *testing.T) {
	gated, _ := tunnelService(t, true)
	response, err := gated.ResolveExecutorConnection(context.Background(), &pluginsdk.ResolveExecutorConnectionRequest{Context: tunnelRequest(1, tunnelControlPort).Context})
	if err != nil || response.GetLease().GetTransport() != "host_tunnel" {
		t.Fatalf("gated provider lease = %v, %v; want the host_tunnel lease", response, err)
	}
	ungated, _ := tunnelService(t, false)
	if _, err := ungated.ResolveExecutorConnection(context.Background(), &pluginsdk.ResolveExecutorConnectionRequest{Context: tunnelRequest(1, tunnelControlPort).Context}); !errors.Is(err, models.ErrExecutorConflict) {
		t.Fatalf("ungated provider error = %v, want conflict", err)
	}
}

func tunnelRecord(requireCapability bool) *store.Record {
	provider := manifest.ExecutorProvider{Key: "tunnel", ContractVersion: manifest.CurrentExecutorProviderContractVersion}
	if requireCapability {
		provider.RequiresHostCapabilities = []manifest.HostCapabilityRequirement{{Name: "tunnel_transport", Version: 1}}
	}
	return &store.Record{
		Manifest: manifest.Manifest{
			ID: tunnelPluginID, Capabilities: manifest.Capabilities{ExecutorProvider: true},
			ExecutorProviders: []manifest.ExecutorProvider{provider},
		},
		InstallationID: tunnelInstallationID, Status: StatusActive,
	}
}

func tunnelService(t *testing.T, requireCapability bool) (*Service, *tunnelProviderPlugin) {
	t.Helper()
	service, _, runtime := newTestService(t)
	impl := &tunnelProviderPlugin{opened: make(chan *pluginsdk.ExecutorTunnelHeader, 4), canceled: make(chan struct{})}
	client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Impl: impl},
	})
	t.Cleanup(func() { _ = client.Close() })
	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	if err != nil {
		t.Fatalf("Dispense() error = %v", err)
	}
	runtime.setRemote(raw.(*pluginsdk.RemotePlugin))
	record := tunnelRecord(requireCapability)
	service.registry.Add(record)
	if err := runtime.Start(context.Background(), record, func(string) pluginsdk.Host { return nil }); err != nil {
		t.Fatalf("runtime.Start() error = %v", err)
	}
	return service, impl
}

func tunnelRequest(generation uint64, port uint32) ExecutorTunnelRequest {
	return ExecutorTunnelRequest{
		Context: &pluginsdk.ExecutorProviderRequestContext{
			PluginId: tunnelPluginID, InstallationId: tunnelInstallationID, ProviderKey: "tunnel",
			ContractVersion: int32(manifest.CurrentExecutorProviderContractVersion),
			EnvironmentId:   "environment-1", EnvironmentGeneration: generation,
		},
		Resource: &pluginsdk.ExecutorResourceDescriptor{},
		Port:     port, AllowedPorts: []uint32{tunnelControlPort},
	}
}

func TestOpenExecutorTunnelCarriesBytesAfterTheProviderAcknowledges(t *testing.T) {
	service, impl := tunnelService(t, true)
	conn, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, tunnelControlPort))
	if err != nil {
		t.Fatalf("OpenExecutorTunnel() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	header := <-impl.opened
	if header.GetPort() != tunnelControlPort || header.GetContext().GetEnvironmentId() != "environment-1" || header.GetBindingNonce() == "" {
		t.Fatalf("provider saw header %+v", header)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 5)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "hello" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}

func TestOpenExecutorTunnelRefusesAProviderWithoutTheCapability(t *testing.T) {
	service, impl := tunnelService(t, false)
	_, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, tunnelControlPort))
	if !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
		t.Fatalf("error = %v, want capability_mismatch", err)
	}
	select {
	case <-impl.opened:
		t.Fatal("the provider saw a tunnel header without declaring the capability")
	default:
	}
}

func TestOpenExecutorTunnelRefusesADisallowedPortWithoutCallingTheProvider(t *testing.T) {
	service, impl := tunnelService(t, true)
	if _, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, 22)); !errors.Is(err, executortunnel.ErrPortNotAllowed) {
		t.Fatalf("error = %v, want ErrPortNotAllowed", err)
	}
	select {
	case <-impl.opened:
		t.Fatal("the provider was called for a disallowed port")
	default:
	}
}

func TestClosingTheTunnelNotifiesTheProvider(t *testing.T) {
	service, impl := tunnelService(t, true)
	conn, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, tunnelControlPort))
	if err != nil {
		t.Fatalf("OpenExecutorTunnel() error = %v", err)
	}
	_ = conn.Close()
	select {
	case <-impl.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the provider was not told the host closed the stream")
	}
	waitForTunnels(t, service, 0)
}

func TestLifecycleChangeClosesEveryTunnelOfThePlugin(t *testing.T) {
	service, _ := tunnelService(t, true)
	conn, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, tunnelControlPort))
	if err != nil {
		t.Fatalf("OpenExecutorTunnel() error = %v", err)
	}
	service.closeExecutorProviderAdmission(tunnelPluginID)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("the stream stayed open across a lifecycle change")
	}
	waitForTunnels(t, service, 0)
	if _, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, tunnelControlPort)); !errors.Is(err, ErrExecutorProviderUnavailable) {
		t.Fatalf("open while admission is closed = %v, want unavailable", err)
	}
}

func TestCloseExecutorTunnelsClosesAReleasedEnvironment(t *testing.T) {
	service, _ := tunnelService(t, true)
	conn, err := service.openExecutorTunnel(context.Background(), tunnelRequest(1, tunnelControlPort))
	if err != nil {
		t.Fatalf("OpenExecutorTunnel() error = %v", err)
	}
	service.CloseExecutorTunnels(tunnelInstallationID, "environment-1")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("the stream stayed open after the environment was released")
	}
}

func TestLeaseTransportRules(t *testing.T) {
	gated := &manifest.ExecutorProvider{RequiresHostCapabilities: []manifest.HostCapabilityRequirement{{Name: "tunnel_transport", Version: 1}}}
	plain := &manifest.ExecutorProvider{}
	cases := []struct {
		name     string
		provider *manifest.ExecutorProvider
		lease    *pluginsdk.ExecutorConnectionLease
		want     error
	}{
		{"default https", plain, &pluginsdk.ExecutorConnectionLease{BaseUrl: "https://x.example"}, nil},
		{"explicit https", plain, &pluginsdk.ExecutorConnectionLease{Transport: "https", BaseUrl: "https://x.example"}, nil},
		{"tunnel with capability", gated, &pluginsdk.ExecutorConnectionLease{Transport: "host_tunnel"}, nil},
		{"tunnel without capability", plain, &pluginsdk.ExecutorConnectionLease{Transport: "host_tunnel"}, models.ErrExecutorConflict},
		{"tunnel with base_url", gated, &pluginsdk.ExecutorConnectionLease{Transport: "host_tunnel", BaseUrl: "https://x.example"}, models.ErrExecutorConflict},
		{"unknown transport", gated, &pluginsdk.ExecutorConnectionLease{Transport: "carrier_pigeon"}, models.ErrExecutorInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExecutorLeaseTransport(tc.provider, tc.lease)
			if !errors.Is(err, tc.want) && (tc.want != nil || err != nil) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}

func waitForTunnels(t *testing.T, service *Service, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if service.tunnels.Active(tunnelInstallationID, "environment-1") == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("active tunnels = %d, want %d", service.tunnels.Active(tunnelInstallationID, "environment-1"), want)
}

func TestOpenExecutorTunnelHeaderCarriesTheRecordedProfile(t *testing.T) {
	service, impl := tunnelService(t, true)
	request := tunnelRequest(1, tunnelControlPort)
	request.Profile = &pluginsdk.ExecutorProfileSnapshot{
		ProfileId: "profile-1", Config: map[string]string{"region": "eu-west-1"},
		SecretValues: map[string]string{"credential": "recorded-secret-value-0123"},
	}
	conn, err := service.openExecutorTunnel(context.Background(), request)
	if err != nil {
		t.Fatalf("OpenExecutorTunnel() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	profile := (<-impl.opened).GetProfile()
	if profile.GetProfileId() != "profile-1" || profile.GetConfig()["region"] != "eu-west-1" ||
		profile.GetSecretValues()["credential"] != "recorded-secret-value-0123" {
		t.Fatalf("provider saw tunnel profile %+v, want the recorded snapshot", profile)
	}
}
