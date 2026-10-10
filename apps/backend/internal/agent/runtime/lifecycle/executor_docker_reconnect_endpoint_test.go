package lifecycle

import (
	"context"
	"errors"
	"testing"
)

type failingReconnectEndpointResolver struct {
	err error
}

func (r failingReconnectEndpointResolver) Resolve(
	context.Context, string, int, string,
) (string, int, error) {
	return "", 0, r.err
}

func (failingReconnectEndpointResolver) Close() error { return nil }

func TestDockerReconnectPropagatesRemoteEndpointResolutionFailure(t *testing.T) {
	resolveErr := errors.New("published remote port unavailable")
	exec := &DockerExecutor{
		logger:    newTestDockerLogger(),
		endpoints: failingReconnectEndpointResolver{err: resolveErr},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := exec.bringupAgentctl(ctx, nil, "container-1", "172.17.0.2", &ExecutorCreateRequest{})
	if !errors.Is(err, resolveErr) {
		t.Fatalf("bringupAgentctl() error = %v, want endpoint resolution error", err)
	}
}
