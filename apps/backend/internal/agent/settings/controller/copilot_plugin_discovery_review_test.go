package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type mutatingRuntimeSelection struct {
	mu      sync.Mutex
	version string
}

func (s *mutatingRuntimeSelection) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return managedruntime.Selection{Package: "@github/copilot", Version: s.version}, true, nil
}

func (s *mutatingRuntimeSelection) Save(_ context.Context, _, _, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.version = version
	return nil
}

func (s *mutatingRuntimeSelection) Delete(context.Context, string, string) error { return nil }

type activatingCopilotRunner struct {
	selection *mutatingRuntimeSelection
	next      string
	calls     int
}

func (r *activatingCopilotRunner) Run(context.Context, []string, map[string]string, string) (copilotplugins.CommandResult, error) {
	r.calls++
	_ = r.selection.Save(context.Background(), "", "", r.next)
	return copilotplugins.CommandResult{Stdout: []byte(`[{"name":"spark","marketplace":"m","version":"1.0.0","enabled":true,"source":"builtin"}]`)}, nil
}

func TestCopilotPluginDiscoveryRejectsRuntimeActivation(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	selection := &mutatingRuntimeSelection{version: "1.0.88"}
	ctrl.managedRuntimeSelections = selection
	runner := &activatingCopilotRunner{selection: selection, next: "1.0.104"}
	ctrl.copilotPluginRunnerImpl = runner

	_, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", dto.CopilotPluginDiscoveryRequest{
		ProfileID: profileID, AuthorizationScope: "user-1",
	})
	if !errors.Is(err, ErrCopilotPluginDiscoverySuperseded) {
		t.Fatalf("runtime activation error = %#v", err)
	}
	cache := ctrl.copilotPluginCacheInstance()
	cache.mu.Lock()
	cachedEntries := len(cache.entries)
	cache.mu.Unlock()
	if cachedEntries != 0 {
		t.Fatalf("runtime activation cached %d entries", cachedEntries)
	}
	if runner.calls != 1 {
		t.Fatalf("activation ran %d native inventories", runner.calls)
	}
}

func TestCopilotPluginDiscoveryFailsWhenRandomKeyUnavailable(t *testing.T) {
	ctrl, _, profileID := newCopilotDiscoveryController(t)
	ctrl.copilotPluginRandom = strings.NewReader("")
	runner := &recordingCopilotRunner{stdout: []byte(`[]`)}
	ctrl.copilotPluginRunnerImpl = runner

	_, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", dto.CopilotPluginDiscoveryRequest{
		ProfileID: profileID, AuthorizationScope: "user-1",
	})
	if err == nil {
		t.Fatal("expected key generation failure")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("key failure ran %d native inventories", len(runner.calls))
	}
}

func TestCopilotPluginDiscoveryConcurrentReadsDoNotSupersede(t *testing.T) {
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
	secondResult := make(chan discoveryResult, 1)
	go func() {
		response, err := ctrl.DiscoverCopilotPlugins(context.Background(), "agent-row-1", req)
		secondResult <- discoveryResult{response: response, err: err}
	}()
	requireCall(t, runner.entered, 2)

	close(runner.releases[0])
	close(runner.releases[1])
	first := waitDiscoveryResult(t, firstResult)
	second := waitDiscoveryResult(t, secondResult)
	if first.err != nil {
		t.Fatalf("first concurrent read: %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second concurrent read: %v", second.err)
	}
}

func TestCopilotPluginDiscoveryGenerationStaysMonotonicAcrossPruning(t *testing.T) {
	ctrl, _, _ := newCopilotDiscoveryController(t)
	first := ctrl.beginCopilotPluginDiscovery("revision-a", true)
	for index := 0; index < 2050; index++ {
		ctrl.beginCopilotPluginDiscovery(fmt.Sprintf("padding-%d", index), true)
	}
	next := ctrl.beginCopilotPluginDiscovery("revision-a", true)
	if next <= first {
		t.Fatalf("generation regressed: first=%d next=%d", first, next)
	}
}

func TestCopilotPluginDiscoveryCacheOrderStaysConsistent(t *testing.T) {
	cache := newCopilotPluginDiscoveryCache()
	now := time.Now()
	response := func() *dto.CopilotPluginDiscoveryResponse {
		return &dto.CopilotPluginDiscoveryResponse{Status: "ready"}
	}
	cache.set("a", response(), now)
	cache.set("b", response(), now)
	cache.delete("a")
	cache.set("a", response(), now)
	if len(cache.order) != 2 || len(cache.entries) != 2 {
		t.Fatalf("order=%#v entries=%d", cache.order, len(cache.entries))
	}
	if cache.order[0] != "b" || cache.order[1] != "a" {
		t.Fatalf("order after delete/set = %#v", cache.order)
	}

	for index := 0; index < copilotPluginCacheMaxEntries+5; index++ {
		cache.set(fmt.Sprintf("extra-%d", index), response(), now)
	}
	if len(cache.order) != copilotPluginCacheMaxEntries {
		t.Fatalf("order length = %d, want %d", len(cache.order), copilotPluginCacheMaxEntries)
	}
	inOrder := make(map[string]bool, len(cache.order))
	for _, key := range cache.order {
		if _, ok := cache.entries[key]; !ok {
			t.Fatalf("order key %q has no entry", key)
		}
		inOrder[key] = true
	}
	for key := range cache.entries {
		if !inOrder[key] {
			t.Fatalf("entry key %q is missing from order", key)
		}
	}
}
