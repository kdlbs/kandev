package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// validRemoteConfig is a saved remote Docker executor's connection, complete
// enough for RemoteDockerTargetFromConfig to resolve it.
func validRemoteConfig() map[string]string {
	return map[string]string{
		"ssh_host":             "10.0.0.5",
		"ssh_user":             "root",
		"ssh_port":             "22",
		"ssh_identity_source":  "file",
		"ssh_identity_file":    "~/.ssh/id_ed25519",
		"ssh_host_fingerprint": "SHA256:abc",
	}
}

type fakeContainerClient struct {
	info        *docker.ContainerInfo
	infoErr     error
	removed     []string
	removeForce []bool
	removeErr   error
}

func (f *fakeContainerClient) GetContainerInfo(_ context.Context, id string) (*docker.ContainerInfo, error) {
	if f.infoErr != nil {
		return nil, f.infoErr
	}
	return f.info, nil
}

func (f *fakeContainerClient) RemoveContainer(_ context.Context, id string, force bool) error {
	f.removed = append(f.removed, id)
	f.removeForce = append(f.removeForce, force)
	return f.removeErr
}

// containersWith wires a RemoteDockerContainers whose Connect returns the
// supplied client, and records whether the connection was released.
func containersWith(client *fakeContainerClient, connectErr error) (*RemoteDockerContainers, *bool, *int) {
	released := false
	connects := 0
	c := &RemoteDockerContainers{
		Connect: func(_ context.Context, _ *lifecycle.SSHTarget) (RemoteDockerContainerClient, func(), error) {
			connects++
			if connectErr != nil {
				return nil, nil, connectErr
			}
			return client, func() { released = true }, nil
		},
	}
	return c, &released, &connects
}

// TestRemoteDockerContainersInspectReportsRunning covers the core of
// AC-EXECUTORS-REMOTE-DOCKER-001.12: a container the remote daemon knows about
// is reported from that daemon, not from the install-wide one.
func TestRemoteDockerContainersInspectReportsRunning(t *testing.T) {
	client := &fakeContainerClient{info: &docker.ContainerInfo{
		ID: "abc123", State: "running", Status: "Up 21 minutes",
	}}
	containers, released, connects := containersWith(client, nil)

	info, err := containers.Inspect(context.Background(), validRemoteConfig(), "abc123")
	if err != nil {
		t.Fatalf("Inspect() error = %v, want nil", err)
	}
	if info.State != "running" || info.Status != "Up 21 minutes" {
		t.Errorf("Inspect() = %+v, want the remote daemon's running state", info)
	}
	if *connects != 1 {
		t.Errorf("connects = %d, want 1", *connects)
	}
	if !*released {
		t.Error("connection was not released; a status poll must not leak an SSH session")
	}
}

// TestRemoteDockerContainersInspectDistinguishesFailures is the behavioral
// core of the fix. An unreachable daemon and a daemon that does not know the
// container are different outcomes: collapsing them is what made a live
// session report "missing" and become unresumable.
func TestRemoteDockerContainersInspectDistinguishesFailures(t *testing.T) {
	t.Run("unreachable daemon is a transport error", func(t *testing.T) {
		containers, _, _ := containersWith(nil, errors.New("dial tcp: connection refused"))

		_, err := containers.Inspect(context.Background(), validRemoteConfig(), "abc123")

		if err == nil {
			t.Fatal("Inspect() error = nil, want a transport error")
		}
		if errors.Is(err, ErrRemoteContainerMissing) {
			t.Error("an unreachable daemon was reported as a missing container")
		}
	})

	t.Run("daemon answers but does not know the container", func(t *testing.T) {
		client := &fakeContainerClient{infoErr: errors.New("Error response from daemon: No such container: abc123")}
		containers, released, _ := containersWith(client, nil)

		_, err := containers.Inspect(context.Background(), validRemoteConfig(), "abc123")

		if !errors.Is(err, ErrRemoteContainerMissing) {
			t.Fatalf("Inspect() error = %v, want it to wrap ErrRemoteContainerMissing", err)
		}
		if !*released {
			t.Error("connection was not released on the missing-container path")
		}
	})
}

// TestRemoteDockerContainersRemove covers AC-EXECUTORS-REMOTE-DOCKER-001.12's
// teardown half: Reset Environment must remove the container on the remote
// daemon, and must not claim success when it could not reach it.
func TestRemoteDockerContainersRemove(t *testing.T) {
	t.Run("removes on the remote daemon", func(t *testing.T) {
		client := &fakeContainerClient{}
		containers, released, _ := containersWith(client, nil)

		if err := containers.Remove(context.Background(), validRemoteConfig(), "abc123"); err != nil {
			t.Fatalf("Remove() error = %v, want nil", err)
		}
		if len(client.removed) != 1 || client.removed[0] != "abc123" {
			t.Errorf("removed = %v, want [abc123]", client.removed)
		}
		if len(client.removeForce) != 1 || !client.removeForce[0] {
			t.Errorf("removeForce = %v, want [true]; reset tears the container down", client.removeForce)
		}
		if !*released {
			t.Error("connection was not released after removal")
		}
	})

	t.Run("an unreachable daemon is an error, not a silent orphan", func(t *testing.T) {
		containers, _, _ := containersWith(nil, errors.New("dial tcp: connection refused"))

		err := containers.Remove(context.Background(), validRemoteConfig(), "abc123")

		if err == nil {
			t.Fatal("Remove() error = nil; reporting success would orphan the remote container")
		}
	})

	t.Run("a container the daemon already forgot is not an error", func(t *testing.T) {
		client := &fakeContainerClient{removeErr: errors.New("Error response from daemon: No such container: abc123")}
		containers, _, _ := containersWith(client, nil)

		if err := containers.Remove(context.Background(), validRemoteConfig(), "abc123"); err != nil {
			t.Fatalf("Remove() error = %v; an already-removed container is the desired end state", err)
		}
	})
}

// TestRemoteDockerContainersRejectUnusableConfig keeps the fingerprint
// requirement that RemoteDockerTargetFromConfig enforces: these operations
// reach a host with effective root, so an unverified target must not be
// dialed, and no connection should be attempted.
func TestRemoteDockerContainersRejectUnusableConfig(t *testing.T) {
	cfg := validRemoteConfig()
	delete(cfg, "ssh_host_fingerprint")
	containers, _, connects := containersWith(&fakeContainerClient{}, nil)

	if _, err := containers.Inspect(context.Background(), cfg, "abc123"); err == nil {
		t.Error("Inspect() error = nil, want a refusal for an executor with no pinned fingerprint")
	}
	if err := containers.Remove(context.Background(), cfg, "abc123"); err == nil {
		t.Error("Remove() error = nil, want a refusal for an executor with no pinned fingerprint")
	}
	if *connects != 0 {
		t.Errorf("connects = %d, want 0; an unverified target must not be dialed", *connects)
	}
}
