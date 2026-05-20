package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeCommandOutputRunner struct {
	output []byte
	err    error
}

func (f *fakeCommandOutputRunner) RunCommandOutput(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return f.output, f.err
}

func TestResolveRemoteAuthHomeDir_UsesOverride(t *testing.T) {
	exec := &SpritesExecutor{logger: newTestLogger()}
	req := &ExecutorCreateRequest{
		Metadata: map[string]interface{}{
			MetadataKeyRemoteAuthHome: "  /home/sprite-user  ",
		},
	}

	home, err := exec.resolveRemoteAuthHomeDir(context.Background(), req, nil)
	require.NoError(t, err)
	require.Equal(t, "/home/sprite-user", home)
}

func TestResolveRemoteAuthHomeDir_UsesProbedHome(t *testing.T) {
	exec := &SpritesExecutor{logger: newTestLogger()}
	runner := &fakeCommandOutputRunner{output: []byte("/home/sprite\n")}

	home, err := exec.resolveRemoteAuthHomeDir(context.Background(), &ExecutorCreateRequest{}, runner)
	require.NoError(t, err)
	require.Equal(t, "/home/sprite", home)
}

func TestResolveRemoteAuthHomeDir_FailsWhenProbeIsEmpty(t *testing.T) {
	exec := &SpritesExecutor{logger: newTestLogger()}
	runner := &fakeCommandOutputRunner{output: []byte(" \n\t")}

	_, err := exec.resolveRemoteAuthHomeDir(context.Background(), &ExecutorCreateRequest{}, runner)
	require.Error(t, err)
	require.ErrorContains(t, err, "empty HOME")
}

func TestResolveRemoteAuthHomeDir_FailsWhenProbeCommandFails(t *testing.T) {
	exec := &SpritesExecutor{logger: newTestLogger()}
	runner := &fakeCommandOutputRunner{err: errors.New("sh failed")}

	_, err := exec.resolveRemoteAuthHomeDir(context.Background(), &ExecutorCreateRequest{}, runner)
	require.Error(t, err)
	require.ErrorContains(t, err, "failed to resolve remote user home")
}
