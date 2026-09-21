package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
)

// resolveContainerNetwork owns the precedence a container's primary network is
// chosen by. The install-wide default reaches a local daemon only.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.1
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.2
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.4
func TestResolveContainerNetworkPrecedence(t *testing.T) {
	tests := []struct {
		name           string
		executorType   string
		profileNetwork string
		installDefault string
		want           string
	}{
		{
			name:           "profile value wins over the install default",
			executorType:   "local_docker",
			profileNetwork: "lab-bridge",
			installDefault: "install-bridge",
			want:           "lab-bridge",
		},
		{
			name:           "local falls back to the install default",
			executorType:   "local_docker",
			installDefault: "install-bridge",
			want:           "install-bridge",
		},
		{
			name:         "local with nothing configured leaves the daemon default",
			executorType: "local_docker",
			want:         "",
		},
		{
			name:           "remote uses its own profile value",
			executorType:   "remote_docker",
			profileNetwork: "remote-bridge",
			installDefault: "install-bridge",
			want:           "remote-bridge",
		},
		{
			name:           "remote never inherits the install default",
			executorType:   "remote_docker",
			installDefault: "install-bridge",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]interface{}{}
			if tt.profileNetwork != "" {
				metadata[MetadataKeyDockerNetwork] = tt.profileNetwork
			}

			got, err := resolveContainerNetwork(metadata, tt.executorType, tt.installDefault)
			if err != nil {
				t.Fatalf("resolveContainerNetwork() error = %v, want nil", err)
			}
			if got.Name != tt.want {
				t.Errorf("Name = %q, want %q", got.Name, tt.want)
			}
		})
	}
}

// A gateway priority is optional. Absent means Docker's own default-route
// selection is left alone, which is not the same as a configured zero.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.4
func TestResolveContainerNetworkGatewayPriority(t *testing.T) {
	t.Run("absent leaves the priority unset", func(t *testing.T) {
		got, err := resolveContainerNetwork(
			map[string]interface{}{MetadataKeyDockerNetwork: "lab-bridge"},
			"local_docker", "",
		)
		if err != nil {
			t.Fatalf("resolveContainerNetwork() error = %v, want nil", err)
		}
		if got.GwPriority != nil {
			t.Errorf("GwPriority = %v, want nil", *got.GwPriority)
		}
	})

	t.Run("a configured zero is distinct from absent", func(t *testing.T) {
		got, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerNetwork:           "lab-bridge",
			MetadataKeyDockerNetworkGwPriority: "0",
		}, "local_docker", "")
		if err != nil {
			t.Fatalf("resolveContainerNetwork() error = %v, want nil", err)
		}
		if got.GwPriority == nil || *got.GwPriority != 0 {
			t.Errorf("GwPriority = %v, want 0", got.GwPriority)
		}
	})

	t.Run("a negative priority is carried through", func(t *testing.T) {
		got, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerNetwork:           "lab-bridge",
			MetadataKeyDockerNetworkGwPriority: "-10",
		}, "local_docker", "")
		if err != nil {
			t.Fatalf("resolveContainerNetwork() error = %v, want nil", err)
		}
		if got.GwPriority == nil || *got.GwPriority != -10 {
			t.Errorf("GwPriority = %v, want -10", got.GwPriority)
		}
	})

	t.Run("an unparsable priority fails the launch", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerNetwork:           "lab-bridge",
			MetadataKeyDockerNetworkGwPriority: "highest",
		}, "local_docker", "")
		if err == nil {
			t.Fatal("resolveContainerNetwork() error = nil, want an error")
		}
		if !strings.Contains(err.Error(), MetadataKeyDockerNetworkGwPriority) {
			t.Errorf("error = %q, want it to name the field", err)
		}
	})

	t.Run("a priority without a network is rejected", func(t *testing.T) {
		_, err := resolveContainerNetwork(map[string]interface{}{
			MetadataKeyDockerNetworkGwPriority: "10",
		}, "local_docker", "")
		if err == nil {
			t.Fatal("resolveContainerNetwork() error = nil, want an error")
		}
	})
}

// A value that names a Docker network *mode* is rejected on the string alone,
// before any daemon call, because none of them is a network the backend can
// reach a published agentctl port through.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.6
func TestResolveContainerNetworkRejectsNetworkModes(t *testing.T) {
	for _, name := range []string{"host", "none", "default", "container:other", "HOST"} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveContainerNetwork(
				map[string]interface{}{MetadataKeyDockerNetwork: name},
				"local_docker", "",
			)
			if err == nil {
				t.Fatalf("resolveContainerNetwork(%q) error = nil, want an error", name)
			}
			if !strings.Contains(err.Error(), MetadataKeyDockerNetwork) {
				t.Errorf("error = %q, want it to name the profile field", err)
			}
		})
	}

	t.Run("bridge is a real network and stays allowed", func(t *testing.T) {
		got, err := resolveContainerNetwork(
			map[string]interface{}{MetadataKeyDockerNetwork: "bridge"},
			"local_docker", "",
		)
		if err != nil {
			t.Fatalf("resolveContainerNetwork() error = %v, want nil", err)
		}
		if got.Name != "bridge" {
			t.Errorf("Name = %q, want %q", got.Name, "bridge")
		}
	})
}

// The install-wide default is operator-supplied too, so it gets the same
// name check rather than bypassing it by arriving from a different source.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.6
func TestResolveContainerNetworkValidatesInstallDefault(t *testing.T) {
	_, err := resolveContainerNetwork(map[string]interface{}{}, "local_docker", "host")
	if err == nil {
		t.Fatal("resolveContainerNetwork() error = nil, want an error")
	}
}

// The resolved network reaches the Docker container configuration, and a
// container with no configured network is created with exactly the arguments
// it was created with before this capability existed.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.1
// @covers AC-EXECUTORS-DOCKER-NETWORKS-002.4
func TestBuildContainerConfigCarriesNetwork(t *testing.T) {
	t.Run("named network becomes the network mode", func(t *testing.T) {
		cm := newCMTest(t)
		got, err := cm.buildContainerConfig(ContainerConfig{
			AgentConfig: newConfigStubAgent(),
			InstanceID:  "0123456789abcdef",
			TaskID:      "task-1",
			Network:     containerNetwork{Name: "lab-bridge"},
		})
		if err != nil {
			t.Fatalf("buildContainerConfig() error = %v", err)
		}
		if got.NetworkMode != "lab-bridge" {
			t.Errorf("NetworkMode = %q, want %q", got.NetworkMode, "lab-bridge")
		}
		if got.NetworkEndpoint != nil {
			t.Errorf("NetworkEndpoint = %+v, want nil without a configured priority", got.NetworkEndpoint)
		}
	})

	t.Run("a configured priority produces an endpoint", func(t *testing.T) {
		cm := newCMTest(t)
		priority := -10
		got, err := cm.buildContainerConfig(ContainerConfig{
			AgentConfig: newConfigStubAgent(),
			InstanceID:  "0123456789abcdef",
			TaskID:      "task-1",
			Network:     containerNetwork{Name: "lab-bridge", GwPriority: &priority},
		})
		if err != nil {
			t.Fatalf("buildContainerConfig() error = %v", err)
		}
		if got.NetworkEndpoint == nil {
			t.Fatal("NetworkEndpoint = nil, want an endpoint")
		}
		if got.NetworkEndpoint.Network != "lab-bridge" {
			t.Errorf("NetworkEndpoint.Network = %q, want %q", got.NetworkEndpoint.Network, "lab-bridge")
		}
		if got.NetworkEndpoint.GwPriority == nil || *got.NetworkEndpoint.GwPriority != -10 {
			t.Errorf("NetworkEndpoint.GwPriority = %v, want -10", got.NetworkEndpoint.GwPriority)
		}
	})

	t.Run("no configured network leaves both unset", func(t *testing.T) {
		cm := newCMTest(t)
		got, err := cm.buildContainerConfig(ContainerConfig{
			AgentConfig: newConfigStubAgent(),
			InstanceID:  "0123456789abcdef",
			TaskID:      "task-1",
		})
		if err != nil {
			t.Fatalf("buildContainerConfig() error = %v", err)
		}
		if got.NetworkMode != "" {
			t.Errorf("NetworkMode = %q, want empty", got.NetworkMode)
		}
		if got.NetworkEndpoint != nil {
			t.Errorf("NetworkEndpoint = %+v, want nil", got.NetworkEndpoint)
		}
	})
}

// buildDockerContainerConfig is where the launch path resolves the network, so
// the install-wide default reaches a local launch and not a remote one.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.2
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.4
func TestBuildDockerContainerConfigResolvesNetwork(t *testing.T) {
	req := &ExecutorCreateRequest{
		InstanceID: "instance-1",
		TaskID:     "task-1",
		Metadata:   map[string]interface{}{},
	}

	local, err := buildDockerContainerConfig(req, "local_docker", "install-bridge")
	if err != nil {
		t.Fatalf("buildDockerContainerConfig(local) error = %v", err)
	}
	if local.Network.Name != "install-bridge" {
		t.Errorf("local Network.Name = %q, want %q", local.Network.Name, "install-bridge")
	}

	remote, err := buildDockerContainerConfig(req, "remote_docker", "install-bridge")
	if err != nil {
		t.Fatalf("buildDockerContainerConfig(remote) error = %v", err)
	}
	if remote.Network.Name != "" {
		t.Errorf("remote Network.Name = %q, want empty", remote.Network.Name)
	}
}

// fakeNetworkInspector answers the driver lookup without a daemon.
type fakeNetworkInspector struct {
	drivers  map[string]string
	inspects []string
	err      error
}

func (f *fakeNetworkInspector) InspectNetwork(_ context.Context, name string) (docker.NetworkInfo, error) {
	f.inspects = append(f.inspects, name)
	if f.err != nil {
		return docker.NetworkInfo{}, f.err
	}
	driver, ok := f.drivers[name]
	if !ok {
		return docker.NetworkInfo{}, fmt.Errorf("network %s not found", name)
	}
	return docker.NetworkInfo{Name: name, Driver: driver}, nil
}

// The primary network carries the published agentctl port. A driver that
// ignores published ports produces a container that starts and cannot be
// reached, so it is refused before the container is created.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.6
func TestVerifyPrimaryNetworkDriver(t *testing.T) {
	inspector := &fakeNetworkInspector{drivers: map[string]string{
		"lab-bridge":  "bridge",
		"swarm-net":   "overlay",
		"lan-macvlan": "macvlan",
		"lan-ipvlan":  "ipvlan",
		"null-net":    "null",
	}}

	t.Run("a port-publishing driver is accepted", func(t *testing.T) {
		for _, name := range []string{"lab-bridge", "swarm-net"} {
			if err := verifyPrimaryNetwork(context.Background(), inspector, name); err != nil {
				t.Errorf("verifyPrimaryNetwork(%q) error = %v, want nil", name, err)
			}
		}
	})

	t.Run("a driver that ignores published ports is refused", func(t *testing.T) {
		for _, name := range []string{"lan-macvlan", "lan-ipvlan", "null-net"} {
			err := verifyPrimaryNetwork(context.Background(), inspector, name)
			if err == nil {
				t.Fatalf("verifyPrimaryNetwork(%q) error = nil, want an error", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error = %q, want it to name the network", err)
			}
			if !strings.Contains(err.Error(), "publish") {
				t.Errorf("error = %q, want it to state the port-publishing requirement", err)
			}
			if !strings.Contains(err.Error(), MetadataKeyDockerAdditionalNetworks) {
				t.Errorf("error = %q, want it to point at the additional-networks field", err)
			}
		}
	})

	t.Run("a missing network is refused with the daemon's reason", func(t *testing.T) {
		err := verifyPrimaryNetwork(context.Background(), inspector, "absent")
		if err == nil {
			t.Fatal("verifyPrimaryNetwork() error = nil, want an error")
		}
		if !strings.Contains(err.Error(), "absent") {
			t.Errorf("error = %q, want it to name the network", err)
		}
	})

	t.Run("no configured network makes no daemon call", func(t *testing.T) {
		quiet := &fakeNetworkInspector{drivers: map[string]string{}}
		if err := verifyPrimaryNetwork(context.Background(), quiet, ""); err != nil {
			t.Fatalf("verifyPrimaryNetwork(\"\") error = %v, want nil", err)
		}
		if len(quiet.inspects) != 0 {
			t.Errorf("inspects = %v, want no daemon call", quiet.inspects)
		}
	})
}

// The network check runs before anything touches the daemon's container API.
// A ContainerManager with no Docker client would panic the moment it tried to
// create one, so reaching a clean error proves nothing was created first.
//
// @covers AC-EXECUTORS-DOCKER-NETWORKS-001.6
func TestCreateAndStartContainerVerifiesNetworkBeforeCreate(t *testing.T) {
	cm := newCMTest(t)
	cm.networkInspector = &fakeNetworkInspector{drivers: map[string]string{"lan-macvlan": "macvlan"}}

	_, _, _, _, err := cm.createAndStartContainer(context.Background(), ContainerConfig{
		AgentConfig: newConfigStubAgent(),
		InstanceID:  "0123456789abcdef",
		TaskID:      "task-1",
		Network:     containerNetwork{Name: "lan-macvlan"},
	})
	if err == nil {
		t.Fatal("createAndStartContainer() error = nil, want the network rejection")
	}
	if !strings.Contains(err.Error(), "lan-macvlan") {
		t.Errorf("error = %q, want it to name the network", err)
	}
}
