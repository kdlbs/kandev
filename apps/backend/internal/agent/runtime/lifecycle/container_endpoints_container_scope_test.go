package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// perContainerPublishedPorts maps (container, container port) to the port the
// remote daemon published, so a test can tell two containers apart.
type perContainerPublishedPorts struct {
	byContainer map[string]map[int]int
}

func (p perContainerPublishedPorts) PublishedPort(
	_ context.Context, containerID string, containerPort int,
) (string, int, error) {
	ports, ok := p.byContainer[containerID]
	if !ok {
		return "", 0, fmt.Errorf("unknown container %q", containerID)
	}
	published, ok := ports[containerPort]
	if !ok {
		return "", 0, errors.New("port not published")
	}
	return "127.0.0.1", published, nil
}

// TestRemoteEndpointResolverScopesForwardsToContainers covers a review finding.
//
// One session resolver can serve two containers: CreateInstance tries to
// reconnect to a preserved container first, and that path resolves the
// agentctl port before it checks the container's health. When the reconnect
// then fails and a fresh container is launched, a cache keyed on the container
// port alone hands back the previous container's forward, and the backend
// dials a port that belongs to a container it has abandoned.
func TestRemoteEndpointResolverScopesForwardsToContainers(t *testing.T) {
	published := perContainerPublishedPorts{byContainer: map[string]map[int]int{
		"stale-container": {8765: 49001},
		"fresh-container": {8765: 49002},
	}}
	fwd := newRecordingForwarder()
	resolver := newRemoteEndpointResolver(published, fwd)
	t.Cleanup(func() { _ = resolver.Close() })

	ctx := context.Background()
	_, stalePort, err := resolver.Resolve(ctx, "stale-container", 8765, "172.17.0.2")
	if err != nil {
		t.Fatalf("resolve for the preserved container: %v", err)
	}
	_, freshPort, err := resolver.Resolve(ctx, "fresh-container", 8765, "172.17.0.3")
	if err != nil {
		t.Fatalf("resolve for the replacement container: %v", err)
	}

	if stalePort == freshPort {
		t.Fatalf("both containers resolved to local port %d; the replacement is unreachable "+
			"because the cache is keyed on the container port alone", freshPort)
	}
	if got := len(fwd.openedPorts()); got != 2 {
		t.Fatalf("opened %d forwards for two containers, want 2 (%v)", got, fwd.openedPorts())
	}

	// The same container still reuses its own forward.
	_, again, err := resolver.Resolve(ctx, "fresh-container", 8765, "172.17.0.3")
	if err != nil {
		t.Fatalf("second resolve for the replacement container: %v", err)
	}
	if again != freshPort {
		t.Errorf("same container resolved to %d then %d; its forward was not reused", freshPort, again)
	}
	if got := len(fwd.openedPorts()); got != 2 {
		t.Errorf("opened %d forwards after a repeat resolve, want 2", got)
	}
}
