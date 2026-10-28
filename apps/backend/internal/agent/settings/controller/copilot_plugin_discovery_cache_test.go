package controller

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestCopilotPluginDiscoveryRefreshBypassesUnchangedCacheWithinTTL(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	runner := &recordingCopilotRunner{stdout: []byte(`[{"name":"first","version":"1.0.0","enabled":true,"source":"builtin"}]`)}
	ctrl.copilotPluginRunnerImpl = runner
	req := dto.CopilotPluginDiscoveryRequest{ProfileID: profileID, AuthorizationScope: "user-1"}

	initial, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("initial discovery: %v", err)
	}
	if initial.Plugins[0].Name != "first" {
		t.Fatalf("initial plugin = %q", initial.Plugins[0].Name)
	}

	runner.stdout = []byte(`[{"name":"second","version":"1.0.0","enabled":true,"source":"builtin"}]`)
	cached, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("cached discovery: %v", err)
	}
	if cached.Plugins[0].Name != "first" || len(runner.calls) != 1 {
		t.Fatalf("cache was not used inside the TTL: plugin=%q calls=%d", cached.Plugins[0].Name, len(runner.calls))
	}

	refresh := req
	refresh.Refresh = true
	reloaded, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", refresh)
	if err != nil {
		t.Fatalf("refresh discovery: %v", err)
	}
	if reloaded.Plugins[0].Name != "second" {
		t.Fatalf("refresh plugin = %q, want the changed inventory", reloaded.Plugins[0].Name)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("refresh ran %d native commands, want 2", len(runner.calls))
	}

	// The refreshed result replaces the cache entry for later non-refresh reads.
	settled, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("post-refresh cached discovery: %v", err)
	}
	if settled.Plugins[0].Name != "second" || len(runner.calls) != 2 {
		t.Fatalf("post-refresh cache = %q calls=%d", settled.Plugins[0].Name, len(runner.calls))
	}
}

func TestCopilotPluginDiscoveryOldCompletionBeforeNewerPublicationIsRejected(t *testing.T) {
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
	requireCall(t, runner.entered, 1)

	refresh := req
	refresh.Refresh = true
	secondResult := make(chan discoveryResult, 1)
	go func() {
		response, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", refresh)
		secondResult <- discoveryResult{response: response, err: err}
	}()
	requireCall(t, runner.entered, 2)

	// The older request finishes while the newer refresh is still running.
	close(runner.releases[0])
	first := waitDiscoveryResult(t, firstResult)
	if first.err == nil || first.response != nil {
		t.Fatalf("stale completion = (%#v, %v), want superseded", first.response, first.err)
	}
	if got := first.err.Error(); got != ErrCopilotPluginDiscoverySuperseded.Error() {
		t.Fatalf("stale completion error = %q", got)
	}

	close(runner.releases[1])
	second := waitDiscoveryResult(t, secondResult)
	if second.err != nil {
		t.Fatalf("newer refresh: %v", second.err)
	}
	if second.response.Plugins[0].Name != "second" {
		t.Fatalf("newer refresh plugin = %q", second.response.Plugins[0].Name)
	}

	cached, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
	if err != nil {
		t.Fatalf("cached read after supersede: %v", err)
	}
	if cached.Plugins[0].Name != "second" {
		t.Fatalf("cache retained stale inventory %q", cached.Plugins[0].Name)
	}
}

func requireCall(t *testing.T, calls <-chan int, want int) {
	t.Helper()
	select {
	case got := <-calls:
		if got != want {
			t.Fatalf("runner call = %d, want %d", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for runner call %d", want)
	}
}

func waitDiscoveryResult(t *testing.T, results <-chan discoveryResult) discoveryResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for discovery result")
		return discoveryResult{}
	}
}
