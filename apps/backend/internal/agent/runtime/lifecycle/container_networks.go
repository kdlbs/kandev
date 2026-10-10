package lifecycle

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agent/docker"
)

// containerNetwork is the resolved primary network a task container is created
// on. It is the attachment Docker publishes the agentctl port on.
type containerNetwork struct {
	// Name is the network the container is created on. Empty leaves
	// NetworkMode unset, which is the daemon's own default network.
	Name string
	// GwPriority is the endpoint's gateway priority. Nil leaves Docker's own
	// default-route selection unchanged, which a configured zero does not.
	GwPriority *int
	// Additional are the networks attached after the container is created, in
	// configured order. They do not publish ports, so this is where an L2
	// attachment belongs.
	Additional []additionalNetwork
}

// additionalNetwork is one attachment beyond the primary network.
type additionalNetwork struct {
	Name       string
	GwPriority *int
}

// additionalNetworkSpec is the persisted form of one additional network.
type additionalNetworkSpec struct {
	Name       string `json:"name"`
	GwPriority *int   `json:"gw_priority,omitempty"`
}

// reservedContainerNetworkNames are Docker network *modes*. None of them names
// a network the backend can reach a published agentctl port through, so each
// is rejected on the string alone before any daemon call.
//
// "bridge" is absent deliberately: it names the daemon's real default bridge
// network, which publishes ports.
var reservedContainerNetworkNames = map[string]bool{
	"host":    true,
	"none":    true,
	"default": true,
}

// containerNetworkModePrefix is the "join this container's namespace" form.
const containerNetworkModePrefix = "container:"

// resolveContainerNetwork chooses the primary network for one launch.
//
// The executor profile is the only source. A network name is scoped to the
// daemon that owns it, so there is no install-wide default to fall back to:
// an empty name leaves the daemon's own default, which is what Kandev did
// before the network was configurable.
func resolveContainerNetwork(metadata map[string]interface{}) (containerNetwork, error) {
	name := strings.TrimSpace(getMetadataString(metadata, MetadataKeyDockerNetwork))
	if err := validateContainerNetworkName(name); err != nil {
		return containerNetwork{}, err
	}

	priority, err := parseNetworkGwPriority(
		getMetadataString(metadata, MetadataKeyDockerNetworkGwPriority),
		MetadataKeyDockerNetworkGwPriority,
	)
	if err != nil {
		return containerNetwork{}, err
	}
	if priority != nil && name == "" {
		return containerNetwork{}, fmt.Errorf(
			"%s is set without %s; a gateway priority belongs to a named network",
			MetadataKeyDockerNetworkGwPriority, MetadataKeyDockerNetwork)
	}

	additional, err := parseAdditionalNetworks(
		getMetadataString(metadata, MetadataKeyDockerAdditionalNetworks), name)
	if err != nil {
		return containerNetwork{}, err
	}

	return containerNetwork{Name: name, GwPriority: priority, Additional: additional}, nil
}

// parseAdditionalNetworks reads the configured attachment list.
//
// A duplicate is rejected here rather than left to the daemon, which reports a
// repeated attachment as a conflict naming neither the field nor the intent.
func parseAdditionalNetworks(raw, primary string) ([]additionalNetwork, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	var specs []additionalNetworkSpec
	if err := json.Unmarshal([]byte(trimmed), &specs); err != nil {
		return nil, fmt.Errorf("%s is not a list of networks: %w", MetadataKeyDockerAdditionalNetworks, err)
	}

	seen := map[string]bool{}
	if primary != "" {
		seen[primary] = true
	}
	networks := make([]additionalNetwork, 0, len(specs))
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			return nil, fmt.Errorf("%s contains an entry with no network name", MetadataKeyDockerAdditionalNetworks)
		}
		if err := validateAdditionalNetworkName(name); err != nil {
			return nil, err
		}
		if seen[name] {
			return nil, fmt.Errorf(
				"%s lists %q more than once, or repeats the primary network",
				MetadataKeyDockerAdditionalNetworks, name)
		}
		seen[name] = true
		networks = append(networks, additionalNetwork{Name: name, GwPriority: spec.GwPriority})
	}
	return networks, nil
}

// validateAdditionalNetworkName refuses a network mode here too. A mode is not
// something a container can hold alongside another attachment.
func validateAdditionalNetworkName(name string) error {
	lowered := strings.ToLower(name)
	if reservedContainerNetworkNames[lowered] || strings.HasPrefix(lowered, containerNetworkModePrefix) {
		return fmt.Errorf(
			"%s = %q names a Docker network mode, not a network",
			MetadataKeyDockerAdditionalNetworks, name)
	}
	return nil
}

// validateContainerNetworkName rejects the values that name a network mode
// rather than a network. An empty name is the "daemon default" case.
func validateContainerNetworkName(name string) error {
	if name == "" {
		return nil
	}
	lowered := strings.ToLower(name)
	if reservedContainerNetworkNames[lowered] || strings.HasPrefix(lowered, containerNetworkModePrefix) {
		return fmt.Errorf(
			"%s = %q names a Docker network mode, not a network; "+
				"the primary network must publish the agentctl port",
			MetadataKeyDockerNetwork, name)
	}
	return nil
}

// parseNetworkGwPriority converts a configured gateway priority. An empty
// value means unset, which Docker treats differently from zero.
func parseNetworkGwPriority(raw, field string) (*int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%s = %q is not a whole number", field, raw)
	}
	return &value, nil
}

// endpointConfig returns the Docker create-time endpoint for this network.
//
// It is nil unless a gateway priority is configured, because NetworkMode
// already places the container on the network. Returning an endpoint
// unconditionally would send an EndpointsConfig on every Docker launch, where
// today there is none.
func (n containerNetwork) endpointConfig() *docker.NetworkEndpointConfig {
	if n.Name == "" || n.GwPriority == nil {
		return nil
	}
	return &docker.NetworkEndpointConfig{Network: n.Name, GwPriority: n.GwPriority}
}

// networkInspector reads a network's state from the daemon that will host the
// container. Narrowed to the one call the check needs so it can be tested
// without a daemon.
type networkInspector interface {
	InspectNetwork(ctx context.Context, name string) (docker.NetworkInfo, error)
}

// nonPublishingNetworkDrivers ignore a container's published ports, or publish
// nothing at all. The backend reaches agentctl through a published port, so a
// container whose primary network uses one of these starts and is unreachable.
var nonPublishingNetworkDrivers = map[string]bool{
	"macvlan": true,
	"ipvlan":  true,
	"null":    true,
}

// verifyPrimaryNetwork refuses a primary network the backend could not reach
// agentctl through, before the container is created.
//
// The check runs against the daemon that will host the container rather than
// at profile save time: a remote profile's daemon is reachable only over the
// connection the launch establishes, and a network can be removed between a
// save and a launch.
func verifyPrimaryNetwork(ctx context.Context, inspector networkInspector, name string) error {
	if name == "" {
		return nil
	}
	info, err := inspector.InspectNetwork(ctx, name)
	if err != nil {
		return fmt.Errorf("%s = %q: %w", MetadataKeyDockerNetwork, name, err)
	}
	if nonPublishingNetworkDrivers[strings.ToLower(info.Driver)] {
		return fmt.Errorf(
			"%s = %q uses the %s driver, which does not publish container ports; "+
				"the primary network must publish the agentctl port, so configure "+
				"this network under %s instead",
			MetadataKeyDockerNetwork, name, info.Driver, MetadataKeyDockerAdditionalNetworks)
	}
	return nil
}

// networkConnector attaches a created container to a further network.
type networkConnector interface {
	ConnectNetwork(ctx context.Context, containerID string, endpoint docker.NetworkEndpointConfig) error
}

// attachAdditionalNetworks connects every configured additional network, in
// configured order.
//
// An empty list makes no daemon call at all, so a launch that configures no
// additional network is indistinguishable from one made before this existed.
func attachAdditionalNetworks(
	ctx context.Context, connector networkConnector, containerID string, networks []additionalNetwork,
) error {
	for _, net := range networks {
		endpoint := docker.NetworkEndpointConfig{Network: net.Name, GwPriority: net.GwPriority}
		if err := connector.ConnectNetwork(ctx, containerID, endpoint); err != nil {
			return fmt.Errorf("attach network %s: %w", net.Name, err)
		}
	}
	return nil
}
