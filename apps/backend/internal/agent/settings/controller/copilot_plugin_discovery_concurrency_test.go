package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type gatedCopilotRunner struct {
	entered  chan int
	releases []chan struct{}
	outputs  [][]byte
	calls    atomic.Int32
}

func (r *gatedCopilotRunner) Run(context.Context, []string, map[string]string, string) (copilotplugins.CommandResult, error) {
	call := int(r.calls.Add(1))
	r.entered <- call
	<-r.releases[call-1]
	return copilotplugins.CommandResult{Stdout: r.outputs[call-1]}, nil
}

func TestCopilotPluginDiscoverySupersededResultIsNotPublished(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	runner := &gatedCopilotRunner{
		entered:  make(chan int, 2),
		releases: []chan struct{}{make(chan struct{}), make(chan struct{})},
		outputs: [][]byte{
			[]byte(`[{"name":"first","version":"1.0.0","enabled":true,"source":"builtin"}]`),
			[]byte(`[{"name":"second","version":"1.0.0","enabled":true,"source":"builtin"}]`),
		},
	}
	ctrl.copilotPluginRunnerImpl = runner
	req := dto.CopilotPluginDiscoveryRequest{ProfileID: profileID, AuthorizationScope: "user-1"}

	firstResult := make(chan discoveryResult, 1)
	go func() {
		response, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
		firstResult <- discoveryResult{response: response, err: err}
	}()
	require.Equal(t, 1, receiveCall(t, runner.entered))

	refresh := req
	refresh.Refresh = true
	secondResult := make(chan discoveryResult, 1)
	go func() {
		response, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", refresh)
		secondResult <- discoveryResult{response: response, err: err}
	}()
	require.Equal(t, 2, receiveCall(t, runner.entered))

	close(runner.releases[1])
	second := receiveResult(t, secondResult)
	require.NoError(t, second.err)
	require.Equal(t, "second", second.response.Plugins[0].Name)

	close(runner.releases[0])
	first := receiveResult(t, firstResult)
	require.ErrorIs(t, first.err, ErrCopilotPluginDiscoverySuperseded)
	require.Nil(t, first.response)

	// The superseded result must not have overwritten the newer cache entry.
	cached, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	require.NoError(t, err)
	require.Equal(t, "second", cached.Plugins[0].Name)
	require.Equal(t, int32(2), runner.calls.Load(), "the third request should use the published cache")
}

func receiveCall(t *testing.T, calls <-chan int) int {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for native runner")
		return 0
	}
}

type discoveryResult struct {
	response *dto.CopilotPluginDiscoveryResponse
	err      error
}

func receiveResult(t *testing.T, results <-chan discoveryResult) discoveryResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for discovery result")
		return discoveryResult{}
	}
}
