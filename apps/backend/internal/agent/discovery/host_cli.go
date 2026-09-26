package discovery

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostcli"
)

// hostCLICacheTTL bounds how long a detected version is reused. A rescan and
// a successful install both invalidate it explicitly; the TTL only covers a
// CLI updated outside Kandev on a backend nobody rescans.
const hostCLICacheTTL = 10 * time.Minute

// hostCLIMaxConcurrent bounds the version subprocesses one sweep may spawn.
const hostCLIMaxConcurrent = 4

// HostCLIState is the detected state of one agent type's vendor CLI.
type HostCLIState struct {
	Version      string
	VersionError string
}

type hostCLICacheEntry struct {
	state     HostCLIState
	expiresAt time.Time
}

// hostCLIResolver detects and caches vendor CLI state per executable path.
type hostCLIResolver struct {
	runner hostcli.Runner
	logger *zap.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]hostCLICacheEntry
}

func newHostCLIResolver(log *zap.Logger) *hostCLIResolver {
	return &hostCLIResolver{
		runner: hostcli.ExecRunner{},
		logger: log,
		now:    time.Now,
		cache:  make(map[string]hostCLICacheEntry),
	}
}

// SetHostCLIRunner replaces the process boundary used for version detection.
// Tests inject a deterministic runner; passing nil restores the real one.
func (r *Registry) SetHostCLIRunner(runner hostcli.Runner) {
	if r == nil || r.hostCLI == nil {
		return
	}
	r.hostCLI.mu.Lock()
	defer r.hostCLI.mu.Unlock()
	if runner == nil {
		r.hostCLI.runner = hostcli.ExecRunner{}
	} else {
		r.hostCLI.runner = runner
	}
	r.hostCLI.cache = make(map[string]hostCLICacheEntry)
}

// InvalidateHostCLICache drops every cached version so the next sweep
// re-detects. Called by a rescan and after a successful agent install.
func (r *Registry) InvalidateHostCLICache() {
	if r == nil || r.hostCLI == nil {
		return
	}
	r.hostCLI.mu.Lock()
	defer r.hostCLI.mu.Unlock()
	r.hostCLI.cache = make(map[string]hostCLICacheEntry)
}

// ApplyHostCLI fills the host-CLI fields of every available result whose agent
// declares a spec. Results for agents without one are left untouched, so agent
// types with no vendor CLI keep their current payload exactly.
func (r *Registry) ApplyHostCLI(
	ctx context.Context,
	lookup func(name string) (agents.Agent, bool),
	results []Availability,
) {
	if r == nil || r.hostCLI == nil || lookup == nil {
		return
	}
	slots := make(chan struct{}, hostCLIMaxConcurrent)
	var wg sync.WaitGroup
	for i := range results {
		if !results[i].Available {
			continue
		}
		ag, ok := lookup(results[i].Name)
		if !ok {
			continue
		}
		cliAgent, ok := ag.(agents.HostCLIAgent)
		if !ok {
			continue
		}
		spec := cliAgent.HostCLI()
		if !spec.Valid() {
			continue
		}
		wg.Add(1)
		go func(idx int, spec hostcli.Spec) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			state := r.hostCLI.resolve(ctx, spec, results[idx].MatchedPath)
			results[idx].CLIVersion = state.Version
			results[idx].CLIVersionError = state.VersionError
		}(i, spec)
	}
	wg.Wait()
}

// resolve returns the cached state for a CLI path or detects it once.
func (h *hostCLIResolver) resolve(ctx context.Context, spec hostcli.Spec, matchedPath string) HostCLIState {
	path := strings.TrimSpace(matchedPath)
	if path == "" {
		path = spec.Executable
	}
	key := spec.Executable + "\x00" + path

	h.mu.Lock()
	entry, cached := h.cache[key]
	now := h.now()
	h.mu.Unlock()
	if cached && now.Before(entry.expiresAt) {
		return entry.state
	}

	state := h.detect(ctx, spec, path)

	h.mu.Lock()
	h.cache[key] = hostCLICacheEntry{state: state, expiresAt: h.now().Add(hostCLICacheTTL)}
	h.mu.Unlock()
	return state
}

func (h *hostCLIResolver) detect(ctx context.Context, spec hostcli.Spec, path string) HostCLIState {
	h.mu.Lock()
	runner := h.runner
	h.mu.Unlock()
	version, err := hostcli.DetectVersion(ctx, runner, path, spec.VersionArgs)
	if err != nil {
		if h.logger != nil {
			h.logger.Debug("host cli version detection failed",
				zap.String("executable", spec.Executable),
				zap.String("path", path),
				zap.Error(err))
		}
		return HostCLIState{VersionError: versionErrorMessage(err)}
	}
	return HostCLIState{Version: version}
}

func versionErrorMessage(err error) string {
	if errors.Is(err, hostcli.ErrNotInstalled) {
		return "executable is not available to the Kandev process"
	}
	return err.Error()
}
