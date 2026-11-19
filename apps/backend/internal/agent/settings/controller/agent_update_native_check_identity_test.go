package controller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestNativeCheckSymlinkRetargetChangesIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	dir := filepath.Join(h.home, "install", "versions")
	first, second := filepath.Join(dir, "18.13.3"), filepath.Join(dir, "18.13.4")
	writeNativeCheckFile(t, first, "first binary")
	writeNativeCheckFile(t, second, "second binary")
	link := filepath.Join(h.home, "omp")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	h.runner.setPath(link)
	spec := agents.NewOmpACP().NativeUpdateCheck()
	before := h.ctrl.prepareNativeUpdateCheck(ompAgentID, spec)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	after := h.ctrl.prepareNativeUpdateCheck(ompAgentID, spec)
	if before.key == after.key {
		t.Fatal("retargeted executable to a different real path but cache identity did not change")
	}
}

func TestNativeCheckFoldedXDGBunfigChangesIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	xdg := filepath.Join(h.home, "xdg")
	h.runner.setEnv(append(h.baseEnv(), "xdg_config_home="+xdg)...)
	config := filepath.Join(xdg, ".bunfig.toml")
	writeNativeCheckFile(t, config, "[install]\nregistry = 'https://old.invalid'\n")
	spec := agents.NewOmpACP().NativeUpdateCheck()
	before := h.ctrl.prepareNativeUpdateCheck(ompAgentID, spec)
	writeNativeCheckFile(t, config, "[install]\nregistry = 'https://new-and-longer.invalid'\n")
	after := h.ctrl.prepareNativeUpdateCheck(ompAgentID, spec)
	if before.key == after.key {
		t.Fatal("changed upstream-selected XDG bunfig size but cache identity did not change")
	}
}

// A result produced after the executable changed during the wait or the run
// describes another installation, so it must not be cached under the key that
// was prepared earlier.
func TestNativeCheckIdentityChangeDuringRunIsNotCached(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	first := filepath.Join(h.home, "install", "a", "omp")
	second := filepath.Join(h.home, "install", "b", "omp")
	writeNativeCheckFile(t, first, "a")
	writeNativeCheckFile(t, second, "b")
	h.runner.setPath(first)
	h.runner.queue(ompUpToDate, ompCanaryAvailable)
	started, release := h.runner.gated()
	done := make(chan dto.AgentUpdateStatusDTO, 1)
	go func() { done <- h.status(t, ompAgentID) }()
	<-started
	h.runner.setPath(second)
	release()
	<-done
	h.runner.setPath(first)
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("status after identity change = %+v; the result from another executable was cached", status)
	}
	requireRuns(t, h.runner, 2, "after the executable changed during a run")
}

// Removing a configuration file is a change even when it restores an
// identity that was cached earlier (AC-AGENTS-RUNTIME-NATIVE-CHECK-002.2).
func TestNativeCheckReturnToPriorIdentityRechecks(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.queue(ompUpToDate, ompUpToDate, ompCanaryAvailable)
	_ = h.status(t, ompAgentID)
	npmrc := filepath.Join(h.home, ".npmrc")
	writeNativeCheckFile(t, npmrc, "registry=https://registry.invalid/\n")
	_ = h.status(t, ompAgentID)
	if err := os.Remove(npmrc); err != nil {
		t.Fatal(err)
	}
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("status after removing .npmrc = %+v; a superseded identity was reused", status)
	}
	requireRuns(t, h.runner, 3, "after create and remove")
	h.ctrl.runtimeUpdateStatusMu.Lock()
	defer h.ctrl.runtimeUpdateStatusMu.Unlock()
	native := 0
	for key := range h.ctrl.runtimeUpdateStatusCache {
		if strings.HasPrefix(key, nativeUpdateCheckKeyPrefix(ompAgentID)) {
			native++
		}
	}
	if native != 1 {
		t.Errorf("native cache entries = %d, want only the active identity", native)
	}
}

// A request that returns to identity A while the first A check is still in
// flight must start a new check rather than join it, and the stale A result
// must not fill the cache.
func TestNativeCheckReturnToIdentityWhileInFlightStartsNewCheck(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.queue(ompUpToDate, ompUpToDate, ompCanaryAvailable)
	started, release := h.runner.gated()
	results := make(chan dto.AgentUpdateStatusDTO, 3)
	request := func() {
		go func() {
			resp, err := h.ctrl.ListAgentUpdateStatuses(context.Background())
			if err != nil || len(resp.Statuses) != 1 {
				results <- dto.AgentUpdateStatusDTO{}
				return
			}
			results <- resp.Statuses[0]
		}()
	}
	awaitStart := func(step string) {
		t.Helper()
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			release()
			t.Fatalf("%s did not start its own check", step)
		}
	}
	npmrc := filepath.Join(h.home, ".npmrc")
	request()
	awaitStart("first A")
	writeNativeCheckFile(t, npmrc, "registry=https://registry.invalid/\n")
	request()
	awaitStart("B")
	if err := os.Remove(npmrc); err != nil {
		t.Fatal(err)
	}
	request()
	awaitStart("returning A")
	release()
	for range 3 {
		<-results
	}
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpdateAvailable {
		t.Errorf("cached status = %+v; want the returning A check's result, not the stale first A", status)
	}
	requireRuns(t, h.runner, 3, "after A, B, and a returning A")
}

// An observation that started before a newer one committed must not replace
// the newer identity or evict its cached result.
func TestNativeCheckStaleObservationDoesNotEvictNewerIdentity(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	first := filepath.Join(h.home, "install", "a", "omp")
	second := filepath.Join(h.home, "install", "b", "omp")
	writeNativeCheckFile(t, first, "a")
	writeNativeCheckFile(t, second, "b")
	h.runner.setPath(first)
	h.runner.setOutput(ompUpToDate, 0)
	paused, resume := make(chan struct{}), make(chan struct{})
	var pausing atomic.Bool
	pausing.Store(true)
	h.runner.setEnvironmentHook(func() {
		if pausing.CompareAndSwap(true, false) {
			close(paused)
			<-resume
		}
	})
	stale := make(chan dto.AgentUpdateStatusDTO, 1)
	go func() { stale <- listStatus(t, h.ctrl, ompAgentID) }()
	<-paused
	h.runner.setPath(second)
	_ = h.status(t, ompAgentID)
	requireRuns(t, h.runner, 1, "after the newer observation")
	close(resume)
	<-stale
	if status := h.status(t, ompAgentID); status.CheckState != dto.AgentUpdateCheckStateUpToDate {
		t.Errorf("status = %+v", status)
	}
	requireRuns(t, h.runner, 1, "after a stale observation resumed")
}

// The standalone build service origin (PI_BUILD_URL) selects the release
// source in check mode, so changing it reruns the check.
func TestNativeCheckBuildURLChangeRechecks(t *testing.T) {
	h := newNativeCheckHarness(t, "18.13.3", agents.NewOmpACP())
	h.runner.setOutput(ompUpToDate, 0)
	_ = h.status(t, ompAgentID)
	h.runner.setEnv(append(h.baseEnv(), "PI_BUILD_URL=https://builds.invalid")...)
	_ = h.status(t, ompAgentID)
	requireRuns(t, h.runner, 2, "after PI_BUILD_URL changed")
}
