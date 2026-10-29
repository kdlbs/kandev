package lifecycle

import (
	"context"
	"net"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
)

// orderedTunnelOperations records tunnel closes in the same event list as the
// provider destroy.
type orderedTunnelOperations struct {
	*pluginExecutorOperationsFake
	events *[]string
}

func (o orderedTunnelOperations) OpenExecutorTunnel(context.Context, *pluginsdk.ExecutorProviderRequestContext, *pluginsdk.ExecutorResourceDescriptor, uint32, []uint32, *pluginsdk.ExecutorProfileSnapshot) (net.Conn, error) {
	return nil, net.ErrClosed
}

func (o orderedTunnelOperations) CloseExecutorTunnels(string, string) {
	*o.events = append(*o.events, "close-tunnels")
}

func TestDestroyClosesTheEnvironmentTunnelsBeforeTheProviderDestroy(t *testing.T) {
	runtime, operations, _, _ := drainRuntimeFixture(t, []string{"drain_receipt@1"}, nil)
	var events []string
	operations.beforeDestroy = func() { events = append(events, "destroy") }
	runtime.operations = orderedTunnelOperations{pluginExecutorOperationsFake: operations, events: &events}

	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if len(events) != 2 || events[0] != "close-tunnels" || events[1] != "destroy" {
		t.Fatalf("event order = %v, want the tunnels closed before the provider destroy", events)
	}
}
