package docker

import (
	"net/netip"
	"testing"

	"github.com/moby/moby/api/types/network"
)

func endpoint(addr string) *network.EndpointSettings {
	return &network.EndpointSettings{IPAddress: netip.MustParseAddr(addr)}
}

// selectContainerIP is the local endpoint resolver's fallback when a published
// port cannot be read. With more than one attachment the map iteration order is
// undefined, so a macvlan address the backend cannot route to can be returned
// instead of the bridge address it needs. Naming the primary network removes
// the ambiguity.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.7
func TestSelectContainerIP(t *testing.T) {
	networks := map[string]*network.EndpointSettings{
		"lab-bridge":  endpoint("172.20.0.5"),
		"lan-macvlan": endpoint("192.168.1.40"),
	}

	t.Run("the named network's address wins", func(t *testing.T) {
		got, ok := selectContainerIP(networks, "lab-bridge")
		if !ok {
			t.Fatal("selectContainerIP() ok = false, want true")
		}
		if got != "172.20.0.5" {
			t.Errorf("ip = %q, want %q", got, "172.20.0.5")
		}
	})

	t.Run("a named network with no address falls back to any attachment", func(t *testing.T) {
		got, ok := selectContainerIP(map[string]*network.EndpointSettings{
			"lab-bridge":  {},
			"lan-macvlan": endpoint("192.168.1.40"),
		}, "lab-bridge")
		if !ok {
			t.Fatal("selectContainerIP() ok = false, want true")
		}
		if got != "192.168.1.40" {
			t.Errorf("ip = %q, want %q", got, "192.168.1.40")
		}
	})

	t.Run("no named network returns the single attachment", func(t *testing.T) {
		got, ok := selectContainerIP(map[string]*network.EndpointSettings{
			"bridge": endpoint("172.17.0.2"),
		}, "")
		if !ok {
			t.Fatal("selectContainerIP() ok = false, want true")
		}
		if got != "172.17.0.2" {
			t.Errorf("ip = %q, want %q", got, "172.17.0.2")
		}
	})

	t.Run("no usable address reports no address", func(t *testing.T) {
		if _, ok := selectContainerIP(map[string]*network.EndpointSettings{
			"bridge": {},
			"other":  nil,
		}, "bridge"); ok {
			t.Error("selectContainerIP() ok = true, want false")
		}
	})
}
