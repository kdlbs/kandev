package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/stretchr/testify/require"
)

// Additional networks are the L2 half of the capability: the primary network
// keeps carrying the published agentctl port while these provide LAN presence.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.1
func TestResolveContainerNetworkAdditional(t *testing.T) {
	t.Run("entries keep their order and optional priority", func(t *testing.T) {
		got, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerNetwork: "lab-bridge",
			MetadataKeyDockerAdditionalNetworks: `[
				{"name":"lan-macvlan","gw_priority":-10},
				{"name":"metrics-internal"}
			]`,
		}, "local_docker", "")
		require.NoError(t, err)
		require.Len(t, got.Additional, 2)

		require.Equal(t, "lan-macvlan", got.Additional[0].Name)
		require.NotNil(t, got.Additional[0].GwPriority)
		require.Equal(t, -10, *got.Additional[0].GwPriority)

		require.Equal(t, "metrics-internal", got.Additional[1].Name)
		require.Nil(t, got.Additional[1].GwPriority)
	})

	t.Run("an absent value attaches nothing", func(t *testing.T) {
		got, err := resolveContainerNetwork(
			map[string]interface{}{MetadataKeyDockerNetwork: "lab-bridge"}, "local_docker", "")
		require.NoError(t, err)
		require.Empty(t, got.Additional)
	})

	t.Run("malformed JSON fails the launch and names the field", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerAdditionalNetworks: `[{"name":`,
		}, "local_docker", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), MetadataKeyDockerAdditionalNetworks)
	})

	t.Run("an entry with no name is refused", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerAdditionalNetworks: `[{"name":"  "}]`,
		}, "local_docker", "")
		require.Error(t, err)
	})

	t.Run("an additional network may name a mode-like value only if it is a network", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerAdditionalNetworks: `[{"name":"host"}]`,
		}, "local_docker", "")
		require.Error(t, err, "a network mode is not attachable as a secondary network either")
	})
}

// A name repeated across the primary and the additional list, or within the
// list, is an operator mistake the daemon reports as an opaque conflict.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.5
func TestResolveContainerNetworkRejectsDuplicates(t *testing.T) {
	t.Run("duplicate of the primary network", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerNetwork:            "lab-bridge",
			MetadataKeyDockerAdditionalNetworks: `[{"name":"lab-bridge"}]`,
		}, "local_docker", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "lab-bridge")
	})

	t.Run("duplicate within the list", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerAdditionalNetworks: `[{"name":"lan"},{"name":"lan"}]`,
		}, "local_docker", "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "lan")
	})
}

// fakeNetworkConnector records attachments without a daemon.
type fakeNetworkConnector struct {
	attached []docker.NetworkEndpointConfig
	err      error
}

func (f *fakeNetworkConnector) ConnectNetwork(
	_ context.Context, containerID string, endpoint docker.NetworkEndpointConfig,
) error {
	_ = containerID
	f.attached = append(f.attached, endpoint)
	return f.err
}

// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.4
func TestAttachAdditionalNetworks(t *testing.T) {
	priority := -10
	connector := &fakeNetworkConnector{}

	err := attachAdditionalNetworks(context.Background(), connector, "cid-1", []additionalNetwork{
		{Name: "lan-macvlan", GwPriority: &priority},
		{Name: "metrics-internal"},
	})

	require.NoError(t, err)
	require.Equal(t, []docker.NetworkEndpointConfig{
		{Network: "lan-macvlan", GwPriority: &priority},
		{Network: "metrics-internal"},
	}, connector.attached)
}

// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.6
func TestAttachAdditionalNetworksNamesTheFailingNetwork(t *testing.T) {
	daemonErr := errors.New("network lan-macvlan not found")
	connector := &fakeNetworkConnector{err: daemonErr}

	err := attachAdditionalNetworks(context.Background(), connector, "cid-1",
		[]additionalNetwork{{Name: "lan-macvlan"}})

	require.ErrorIs(t, err, daemonErr)
	require.Contains(t, err.Error(), "lan-macvlan")
}

// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.7
func TestAttachAdditionalNetworksMakesNoCallWhenEmpty(t *testing.T) {
	connector := &fakeNetworkConnector{}
	require.NoError(t, attachAdditionalNetworks(context.Background(), connector, "cid-1", nil))
	require.Empty(t, connector.attached)
}

// The agent must observe every interface for the whole of its life, so every
// attachment lands before the container starts. Attachment also precedes the
// seed, which extracts the agent's credentials: a network failure after that
// point would have put them in a container about to be deleted.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.2
func TestCreateSeedAndStartAttachesBeforeSeedAndStart(t *testing.T) {
	api := &fakeContainerStarter{createID: "cid-1"}

	id, err := createSeedAndStart(context.Background(), api, docker.ContainerConfig{},
		containerCreateHooks{
			attachNetworks: func(_ context.Context, containerID string) error {
				api.calls = append(api.calls, "attach:"+containerID)
				return nil
			},
			seed: func(_ context.Context, containerID string) error {
				api.calls = append(api.calls, "seed:"+containerID)
				return nil
			},
		}, newTestLogger())

	require.NoError(t, err)
	require.Equal(t, "cid-1", id)
	require.Equal(t, []string{"create", "attach:cid-1", "seed:cid-1", "start:cid-1"}, api.calls)
}

// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.6
func TestCreateSeedAndStartRemovesContainerWhenAttachFails(t *testing.T) {
	api := &fakeContainerStarter{createID: "cid-2"}
	attachErr := errors.New("attach network lan-macvlan: not found")

	id, err := createSeedAndStart(context.Background(), api, docker.ContainerConfig{},
		containerCreateHooks{
			attachNetworks: func(_ context.Context, _ string) error { return attachErr },
			seed: func(_ context.Context, _ string) error {
				t.Fatal("seed must not run after a failed attachment")
				return nil
			},
		}, newTestLogger())

	require.Empty(t, id)
	require.ErrorIs(t, err, attachErr)
	require.Equal(t, []string{"create", "remove:cid-2"}, api.calls)
	require.True(t, strings.Contains(err.Error(), "lan-macvlan"))
}

// Additional networks are profile-supplied, not install-supplied, so unlike
// the primary network's install-wide fallback they apply on a remote daemon
// exactly as they do on a local one.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.3
func TestBuildDockerContainerConfigCarriesAdditionalNetworksOnRemote(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		Metadata: map[string]interface{}{
			MetadataKeyDockerNetwork:            "remote-bridge",
			MetadataKeyDockerAdditionalNetworks: `[{"name":"lan-macvlan","gw_priority":-10}]`,
		},
	}

	cfg, err := buildDockerContainerConfig(req, "remote_docker", "install-bridge")
	require.NoError(t, err)
	require.Equal(t, "remote-bridge", cfg.Network.Name)
	require.Len(t, cfg.Network.Additional, 1)
	require.Equal(t, "lan-macvlan", cfg.Network.Additional[0].Name)
	require.NotNil(t, cfg.Network.Additional[0].GwPriority)
	require.Equal(t, -10, *cfg.Network.Additional[0].GwPriority)
}
