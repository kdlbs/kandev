package runtime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

type fakeBuildClient struct {
	gotDockerfile string
	gotTag        string
	err           error
}

func (f *fakeBuildClient) BuildImage(_ context.Context, dockerfile, tag string, _ map[string]*string) (io.ReadCloser, error) {
	f.gotDockerfile, f.gotTag = dockerfile, tag
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader("built")), nil
}

// validConfig names the SSH user explicitly. Target resolution falls back to
// $USER when the config omits it, which passes on a developer machine and
// fails on a CI runner that sets no $USER; the fixture must not depend on the
// ambient environment either way.
func validConfig() map[string]string {
	return map[string]string{
		"ssh_host":             "build-box",
		"ssh_user":             "builder",
		"ssh_host_fingerprint": "SHA256:abc",
	}
}

func builderWith(client *fakeBuildClient, released *bool) *RemoteDockerBuilder {
	return &RemoteDockerBuilder{
		Connect: func(context.Context, *lifecycle.SSHTarget) (RemoteDockerBuildClient, func(), error) {
			return client, func() { *released = true }, nil
		},
	}
}

// TestBuildRunsOnTheExecutorsDaemon is the point of this path: the image must
// exist on the daemon that will run the container, not on the backend's own.
func TestBuildRunsOnTheExecutorsDaemon(t *testing.T) {
	released := false
	client := &fakeBuildClient{}
	stream, err := builderWith(client, &released).Build(
		context.Background(), validConfig(), "FROM alpine", "kandev/agent:latest", nil,
	)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if client.gotTag != "kandev/agent:latest" || client.gotDockerfile != "FROM alpine" {
		t.Fatalf("daemon got dockerfile=%q tag=%q", client.gotDockerfile, client.gotTag)
	}
	if released {
		t.Fatal("connection released before the build stream was read")
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !released {
		t.Fatal("closing the build stream did not release the connection")
	}
}

// TestBuildRequiresATrustedFingerprint refuses to run Dockerfile instructions,
// which carry the daemon's authority, against an unverified host.
func TestBuildRequiresATrustedFingerprint(t *testing.T) {
	cfg := validConfig()
	delete(cfg, "ssh_host_fingerprint")

	released := false
	_, err := builderWith(&fakeBuildClient{}, &released).Build(
		context.Background(), cfg, "FROM alpine", "t", nil,
	)
	if err == nil {
		t.Fatal("Build against an untrusted host = nil error, want error")
	}
}

// TestBuildRejectsDaemonURL keeps a Docker host URL out of the saved config.
func TestBuildRejectsDaemonURL(t *testing.T) {
	cfg := validConfig()
	cfg["ssh_host"] = "tcp://build-box:2375"

	released := false
	if _, err := builderWith(&fakeBuildClient{}, &released).Build(
		context.Background(), cfg, "FROM alpine", "t", nil,
	); err == nil {
		t.Fatal("Build with a daemon URL = nil error, want error")
	}
}

// TestBuildReleasesOnFailure keeps a failed build from leaking its connection.
func TestBuildReleasesOnFailure(t *testing.T) {
	released := false
	client := &fakeBuildClient{err: errors.New("daemon refused")}

	if _, err := builderWith(client, &released).Build(
		context.Background(), validConfig(), "FROM alpine", "t", nil,
	); err == nil {
		t.Fatal("Build = nil error despite a daemon failure")
	}
	if !released {
		t.Fatal("failed build leaked its connection")
	}
}
