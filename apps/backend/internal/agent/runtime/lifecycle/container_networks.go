package lifecycle

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/task/models"
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
// The install-wide default applies to a local daemon only. A network name is
// scoped to the daemon that owns it, so a name configured for the backend
// host's daemon is not a default for a remote one, where it may not exist or
// may name something else entirely.
func resolveContainerNetwork(
	metadata map[string]interface{}, executorType, installDefault string,
) (containerNetwork, error) {
	name := strings.TrimSpace(getMetadataString(metadata, MetadataKeyDockerNetwork))
	if name == "" && models.ExecutorType(executorType) == models.ExecutorTypeLocalDocker {
		name = strings.TrimSpace(installDefault)
	}
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

	return containerNetwork{Name: name, GwPriority: priority}, nil
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
				"the primary network must publish the agentctl port",
			MetadataKeyDockerNetwork, name, info.Driver)
	}
	return nil
}
