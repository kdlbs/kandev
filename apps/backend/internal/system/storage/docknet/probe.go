package docknet

import (
	"context"
	"fmt"
	"net/netip"

	agentdocker "github.com/kandev/kandev/internal/agent/docker"
)

// ProbeNamePrefix is the throwaway probe network's name prefix; the run ID
// suffix keeps every probe network uniquely named and identifiable in census.
const ProbeNamePrefix = "kd-probe-"

// ProbeResult is persisted with the run record (AC11).
type ProbeResult struct {
	// Passed is true iff the throwaway network was created, received an IPv4
	// subnet non-overlapping with every active network, and was removed.
	Passed bool `json:"passed"`
	// Subnet is the subnet the daemon allocated to the probe network.
	Subnet string `json:"subnet,omitempty"`
	// Overlaps lists the active networks whose subnet overlaps the probe's.
	Overlaps []string `json:"overlaps,omitempty"`
	// Error is the failure reason when the probe did not pass.
	Error string `json:"error,omitempty"`
}

// DockerNetworkCreateRemover is the client surface the probe needs; it is the
// same interface the provider uses.
type DockerNetworkCreateRemover interface {
	CreateNetwork(context.Context, agentdocker.CreateNetworkOptions) (agentdocker.NetworkInfo, error)
	RemoveNetwork(context.Context, string) error
	ListNetworks(context.Context, agentdocker.NetworkListOptions) ([]agentdocker.NetworkInfo, error)
	InspectNetwork(context.Context, string) (agentdocker.NetworkInfo, error)
}

// RunProbe creates a uniquely-named throwaway bridge network, asserts the
// daemon allocated an IPv4 subnet for it that does not overlap any active
// census network's subnet, removes the probe network, and returns the result
// (AC11, AC12). It must be called with a fresh active-set census so the
// overlap assertion can run; activeNet is that census's inspected detail.
func RunProbe(
	ctx context.Context,
	client DockerNetworkCreateRemover,
	runID string,
	activeNets []agentdocker.NetworkInfo,
) ProbeResult {
	name := ProbeNamePrefix + runID
	created, err := client.CreateNetwork(ctx, agentdocker.CreateNetworkOptions{
		Name: name,
		Labels: map[string]string{
			"kandev.managed": "true", "kandev.role": "probe",
		},
	})
	if err != nil {
		return ProbeResult{Passed: false, Error: fmt.Sprintf("create probe network: %v", err)}
	}
	// The probe always removes its own network, even on assertion failure.
	defer func() {
		_ = client.RemoveNetwork(ctx, created.ID)
	}()

	subnet, err := allocatedIPv4Subnet(created)
	if err != nil {
		return ProbeResult{Passed: false, Error: err.Error()}
	}
	overlaps := overlappingActiveNets(subnet, activeNets)
	if len(overlaps) > 0 {
		return ProbeResult{
			Passed: false, Subnet: subnet.String(),
			Overlaps: overlaps,
			Error:    "allocated subnet overlaps an active network's subnet",
		}
	}
	return ProbeResult{Passed: true, Subnet: subnet.String()}
}

func allocatedIPv4Subnet(network agentdocker.NetworkInfo) (netip.Prefix, error) {
	for _, subnet := range network.IPv4Subnets {
		return subnet, nil
	}
	return netip.Prefix{}, fmt.Errorf(
		"probe network %s received no IPv4 subnet from the daemon address pool", network.ID,
	)
}

func overlappingActiveNets(subnet netip.Prefix, activeNets []agentdocker.NetworkInfo) []string {
	var overlaps []string
	for _, net := range activeNets {
		for _, candidate := range net.IPv4Subnets {
			if subnet.Overlaps(candidate) {
				overlaps = append(overlaps, net.Name)
				break
			}
		}
	}
	return overlaps
}
