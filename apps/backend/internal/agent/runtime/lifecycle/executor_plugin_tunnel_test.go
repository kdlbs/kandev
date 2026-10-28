package lifecycle

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

type tunnelOperationsFake struct {
	pluginExecutorOperationsFake
	opened []tunnelOpen
	closed [][2]string
}

type tunnelOpen struct {
	port    uint32
	allowed []uint32
	env     string
}

func (f *tunnelOperationsFake) OpenExecutorTunnel(_ context.Context, rc *pluginsdk.ExecutorProviderRequestContext, _ *pluginsdk.ExecutorResourceDescriptor, port uint32, allowed []uint32) (net.Conn, error) {
	f.opened = append(f.opened, tunnelOpen{port: port, allowed: allowed, env: rc.GetEnvironmentId()})
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (f *tunnelOperationsFake) CloseExecutorTunnels(installationID, environmentID string) {
	f.closed = append(f.closed, [2]string{installationID, environmentID})
}

func tunnelLeaseResponse() *pluginsdk.ResolveExecutorConnectionResponse {
	return &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
		Transport: "host_tunnel", Generation: "g1", ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
	}}
}

func TestHostTunnelLeaseDialsTheResolvedPortWithTheControlPortAllowed(t *testing.T) {
	operations := &tunnelOperationsFake{pluginExecutorOperationsFake: pluginExecutorOperationsFake{connectionResponse: tunnelLeaseResponse()}}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	const instancePort = 40123
	resolver := runtime.connectionResolver(&pluginsdk.ExecutorProviderRequestContext{EnvironmentId: "env-1"}, &pluginsdk.ExecutorResourceDescriptor{}, instancePort)

	lease, err := resolver(context.Background())
	if err != nil {
		t.Fatalf("resolver() error = %v", err)
	}
	if lease.Dial == nil || lease.BaseURL != "" {
		t.Fatalf("lease = %+v, want a dialer and no base URL", lease)
	}
	conn, err := lease.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	_ = conn.Close()
	if len(operations.opened) != 1 || operations.opened[0].port != instancePort || operations.opened[0].env != "env-1" {
		t.Fatalf("opened = %+v, want one stream to the instance port for env-1", operations.opened)
	}
	got := operations.opened[0].allowed
	if len(got) != 2 || got[0] != pluginExecutorRuntimePort || got[1] != instancePort {
		t.Fatalf("allowed ports = %v, want [control instance]", got)
	}
}

func TestHostTunnelLeaseWithoutTunnelOperationsFailsClosed(t *testing.T) {
	operations := &pluginExecutorOperationsFake{connectionResponse: tunnelLeaseResponse()}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	resolver := runtime.connectionResolver(&pluginsdk.ExecutorProviderRequestContext{}, &pluginsdk.ExecutorResourceDescriptor{}, pluginExecutorRuntimePort)

	if _, err := resolver(context.Background()); err == nil || !strings.Contains(err.Error(), "host tunnel") {
		t.Fatalf("resolver() error = %v, want a host tunnel refusal", err)
	}
}

func TestClosePluginExecutorTunnelsNamesTheInstallationAndEnvironment(t *testing.T) {
	operations := &tunnelOperationsFake{}
	runtime := NewPluginRemoteExecutor(operations, newTestLogger())
	runtime.closePluginExecutorTunnels(&pluginsdk.ExecutorProviderRequestContext{InstallationId: "inst-1", EnvironmentId: "env-1"})
	runtime.closePluginExecutorTunnels(nil)
	if len(operations.closed) != 1 || operations.closed[0] != [2]string{"inst-1", "env-1"} {
		t.Fatalf("closed = %v, want one close of inst-1/env-1", operations.closed)
	}
}
