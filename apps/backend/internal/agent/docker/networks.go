package docker

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"time"

	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

type networkAPI interface {
	NetworkList(context.Context, client.NetworkListOptions) (client.NetworkListResult, error)
	NetworkInspect(context.Context, string, client.NetworkInspectOptions) (client.NetworkInspectResult, error)
	NetworkRemove(context.Context, string, client.NetworkRemoveOptions) (client.NetworkRemoveResult, error)
	NetworkCreate(context.Context, string, client.NetworkCreateOptions) (client.NetworkCreateResult, error)
}

// NetworkListOptions restricts the daemon network census to ownership evidence.
type NetworkListOptions struct {
	Driver   string
	Labels   []string
	Dangling *bool
}

// NetworkInfo is the network summary needed by maintenance providers.
// IPv4Subnets carries every configured IPAM IPv4 subnet; it is empty for
// networks the daemon never allocated one for. Containers maps connected
// endpoint IDs to endpoint names and is populated only by InspectNetwork;
// list summaries cannot see attachments.
type NetworkInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Driver      string            `json:"driver"`
	Scope       string            `json:"scope"`
	ConfigOnly  bool              `json:"config_only"`
	Labels      map[string]string `json:"labels"`
	IPv4Subnets []netip.Prefix    `json:"ipv4_subnets"`
	Containers  map[string]string `json:"containers,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}

// CreateNetworkOptions describes the throwaway bridge network the capacity
// probe creates. Driver is always bridge: the probe asserts the daemon
// allocates the subnet, so no IPAM override exists here.
type CreateNetworkOptions struct {
	Name   string
	Labels map[string]string
}

// ListNetworks returns network summaries matching the supplied ownership filters.
func (c *Client) ListNetworks(ctx context.Context, options NetworkListOptions) ([]NetworkInfo, error) {
	filters := make(client.Filters)
	if options.Driver != "" {
		filters.Add("driver", options.Driver)
	}
	for _, label := range options.Labels {
		filters.Add("label", label)
	}
	if options.Dangling != nil {
		filters.Add("dangling", strconv.FormatBool(*options.Dangling))
	}

	result, err := c.networkClient().NetworkList(ctx, client.NetworkListOptions{Filters: filters})
	if err != nil {
		return nil, fmt.Errorf("list Docker networks: %w", err)
	}
	return networkSummaries(result.Items), nil
}

// InspectNetwork returns one network with connected-container detail.
func (c *Client) InspectNetwork(ctx context.Context, id string) (NetworkInfo, error) {
	result, err := c.networkClient().NetworkInspect(ctx, id, client.NetworkInspectOptions{})
	if err != nil {
		return NetworkInfo{}, fmt.Errorf("inspect Docker network %s: %w", id, err)
	}
	return networkInfoFromInspect(result.Network), nil
}

// RemoveNetwork removes exactly one network by its daemon ID.
func (c *Client) RemoveNetwork(ctx context.Context, id string) error {
	if _, err := c.networkClient().NetworkRemove(ctx, id, client.NetworkRemoveOptions{}); err != nil {
		return fmt.Errorf("remove Docker network %s: %w", id, err)
	}
	return nil
}

// CreateNetwork creates a throwaway bridge network for the capacity probe and
// returns its inspected detail, including the allocated subnet.
func (c *Client) CreateNetwork(ctx context.Context, options CreateNetworkOptions) (NetworkInfo, error) {
	enableIPv4 := true
	result, err := c.networkClient().NetworkCreate(ctx, options.Name, client.NetworkCreateOptions{
		Driver: "bridge", EnableIPv4: &enableIPv4, Labels: options.Labels,
	})
	if err != nil {
		return NetworkInfo{}, fmt.Errorf("create Docker network %s: %w", options.Name, err)
	}
	return c.InspectNetwork(ctx, result.ID)
}

func (c *Client) networkClient() networkAPI {
	if c.networks != nil {
		return c.networks
	}
	return c.cli
}

func networkSummaries(items []network.Summary) []NetworkInfo {
	result := make([]NetworkInfo, 0, len(items))
	for _, item := range items {
		result = append(result, networkInfoFromNetwork(item.Network))
	}
	return result
}

func networkInfoFromInspect(inspect network.Inspect) NetworkInfo {
	info := networkInfoFromNetwork(inspect.Network)
	if len(inspect.Containers) > 0 {
		info.Containers = make(map[string]string, len(inspect.Containers))
		for id, endpoint := range inspect.Containers {
			info.Containers[id] = endpoint.Name
		}
	}
	return info
}

func networkInfoFromNetwork(net network.Network) NetworkInfo {
	info := NetworkInfo{
		ID: net.ID, Name: net.Name, Driver: net.Driver, Scope: net.Scope,
		ConfigOnly: net.ConfigOnly, Labels: net.Labels, CreatedAt: net.Created,
	}
	if info.Labels == nil {
		info.Labels = map[string]string{}
	}
	for _, config := range net.IPAM.Config {
		if config.Subnet.IsValid() && config.Subnet.Addr().Is4() {
			info.IPv4Subnets = append(info.IPv4Subnets, config.Subnet)
		}
	}
	return info
}
