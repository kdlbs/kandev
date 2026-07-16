// Package runtime spawns and supervises kandev plugin backends as
// hashicorp/go-plugin subprocesses, per §2/§3 of
// docs/plans/plugins/GRPC-CONTRACT.md. Manager owns one *process (see
// process.go) per running plugin: Start resolves the host-platform
// executable declared in the plugin's manifest, spawns it, and performs the
// go-plugin gRPC handshake; the returned *process then supervises the
// subprocess in the background (periodic health pings, crash detection,
// restart with backoff) until Stop is called.
package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// pluginDataDirEnv is the environment variable kandev injects into every
// spawned plugin subprocess, per §2 of the frozen contract.
const pluginDataDirEnv = "KANDEV_PLUGIN_DATA_DIR"

// errNotRunning is returned by Manager.Ping (via process.ping) when no
// process is currently tracked/live for the given plugin id.
func errNotRunning(id string) error {
	return fmt.Errorf("plugins/runtime: plugin %q is not running", id)
}

// Option configures a Manager at construction time.
type Option func(*Manager)

// WithPingInterval overrides the supervision loop's health-check cadence
// (default 30s). Tests use a short interval instead of real sleeps.
func WithPingInterval(d time.Duration) Option {
	return func(m *Manager) { m.pingInterval = d }
}

// WithMaxConsecutiveFailures overrides how many consecutive failed pings
// trigger a restart (default 3).
func WithMaxConsecutiveFailures(n int) Option {
	return func(m *Manager) { m.maxConsecutiveFailures = n }
}

// WithRestartBackoff overrides the delay schedule between restart attempts
// (default 1s/2s/4s/8s/16s). len(delays) also bounds maxRestartAttempts
// unless WithMaxRestartAttempts is set separately.
func WithRestartBackoff(delays []time.Duration) Option {
	return func(m *Manager) { m.restartBackoff = delays }
}

// WithMaxRestartAttempts overrides how many restart attempts are made
// before giving up permanently (default 5).
func WithMaxRestartAttempts(n int) Option {
	return func(m *Manager) { m.maxRestartAttempts = n }
}

// Manager owns the set of currently-spawned plugin subprocesses. One
// Manager is constructed for the whole backend process (see
// internal/plugins.Provide); Start/Stop are called per plugin as it is
// enabled/disabled/installed/uninstalled.
type Manager struct {
	pluginsDir     string
	log            *logger.Logger
	onStatusChange func(id string, healthy bool)

	pingInterval           time.Duration
	maxConsecutiveFailures int
	restartBackoff         []time.Duration
	maxRestartAttempts     int

	mu        sync.Mutex
	processes map[string]*process
}

// NewManager returns a Manager rooted at pluginsDir (the same directory
// store.FSStore persists records under — KANDEV_PLUGIN_DATA_DIR for plugin
// id "x" is pluginsDir/x/data). onStatusChange is invoked from the
// supervision loop's own goroutine whenever a running plugin's health
// transitions (degraded -> unhealthy, or a restart recovers it); it must
// not block. May be nil (e.g. tests that don't care about transitions).
func NewManager(pluginsDir string, onStatusChange func(id string, healthy bool), log *logger.Logger, opts ...Option) *Manager {
	m := &Manager{
		pluginsDir:             pluginsDir,
		log:                    log,
		onStatusChange:         onStatusChange,
		pingInterval:           defaultPingInterval,
		maxConsecutiveFailures: defaultMaxConsecutiveFailures,
		restartBackoff:         defaultRestartBackoff(),
		maxRestartAttempts:     defaultMaxRestartAttempts,
		processes:              make(map[string]*process),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start resolves the host-platform executable declared in rec's manifest
// under rec.InstallPath, spawns it as a go-plugin gRPC subprocess (AutoMTLS,
// KANDEV_PLUGIN_DATA_DIR set, cwd = rec.InstallPath), performs the
// handshake, and — on success — begins background supervision. hostFactory
// builds the Go-native Host implementation bound to this plugin's record;
// it is called once, synchronously, before the subprocess is spawned.
//
// Returns an error without registering anything if rec is not
// runtime-managed, a process is already tracked for rec.ID, the host
// platform has no declared executable, or the initial spawn/handshake
// fails. ctx is checked for early cancellation before spawning; the
// subprocess handshake itself is bounded by go-plugin's own StartTimeout
// (not ctx), since hashicorp/go-plugin's Client() call is not
// context-aware.
func (m *Manager) Start(ctx context.Context, rec *store.Record, hostFactory func(pluginID string) pluginsdk.Host) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !rec.IsManaged() {
		return fmt.Errorf("plugins/runtime: plugin %q is not runtime-managed", rec.ID)
	}

	m.mu.Lock()
	if _, exists := m.processes[rec.ID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("plugins/runtime: plugin %q is already running", rec.ID)
	}
	m.mu.Unlock()

	recCopy := rec.Manifest
	spawnFn, err := m.spawnFuncFor(rec.ID, &recCopy, rec.InstallPath, hostFactory)
	if err != nil {
		return err
	}

	p := newProcess(rec.ID, m.log, spawnFn, m.onStatusChange)
	p.pingInterval = m.pingInterval
	p.maxConsecutiveFails = m.maxConsecutiveFailures
	p.restartBackoff = m.restartBackoff
	p.maxRestartAttempts = m.maxRestartAttempts

	if err := p.start(); err != nil {
		return fmt.Errorf("plugins/runtime: start plugin %q: %w", rec.ID, err)
	}

	m.mu.Lock()
	m.processes[rec.ID] = p
	m.mu.Unlock()
	return nil
}

// Stop kills and stops supervising the process for id, if any. A no-op if
// id is not currently running.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	p, ok := m.processes[id]
	if ok {
		delete(m.processes, id)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	p.stop()
}

// StopAll stops every currently-running plugin. Intended for graceful
// backend shutdown (addCleanup).
func (m *Manager) StopAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Stop(id)
	}
}

// Get returns the live RemotePlugin for id, for calling DeliverEvent/
// InvokeTool/HandleWebhook, and whether one is currently available (false
// if the plugin was never started, was stopped, or is mid-restart).
func (m *Manager) Get(id string) (*pluginsdk.RemotePlugin, bool) {
	m.mu.Lock()
	p, ok := m.processes[id]
	m.mu.Unlock()
	if !ok {
		return nil, false
	}
	return p.remote()
}

// Ping issues an on-demand health check against id's current process.
func (m *Manager) Ping(id string) error {
	m.mu.Lock()
	p, ok := m.processes[id]
	m.mu.Unlock()
	if !ok {
		return errNotRunning(id)
	}
	return p.ping()
}

// Running reports whether id currently has a live process.
func (m *Manager) Running(id string) bool {
	_, ok := m.Get(id)
	return ok
}

// RestartCount returns how many times id's process has been automatically
// restarted since it was started, for Service to persist best-effort onto
// store.Record.RestartCount. Returns 0 if id is not tracked.
func (m *Manager) RestartCount(id string) int {
	m.mu.Lock()
	p, ok := m.processes[id]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	return p.restartCount()
}

// spawnFuncFor builds the production spawnFn for a plugin: resolve the host
// platform's executable, prepare its data dir, and construct a fresh
// hcplugin.Client wired to hostFactory(id)'s Host implementation on every
// call (so a restart gets a fresh client/broker rather than reusing a dead
// one).
func (m *Manager) spawnFuncFor(id string, mf *manifest.Manifest, installPath string, hostFactory func(string) pluginsdk.Host) (func() (spawnedProcess, error), error) {
	execRelPath, ok := mf.ExecutableFor(goruntime.GOOS, goruntime.GOARCH)
	if !ok {
		return nil, fmt.Errorf("plugins/runtime: plugin %q has no executable for %s-%s", id, goruntime.GOOS, goruntime.GOARCH)
	}
	execPath := filepath.Join(installPath, execRelPath)
	dataDir := filepath.Join(m.pluginsDir, id, "data")

	return func() (spawnedProcess, error) {
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return nil, fmt.Errorf("plugins/runtime: create data dir for %q: %w", id, err)
		}

		cmd := exec.Command(execPath) //nolint:gosec // execPath is resolved from a verified, installed package manifest
		cmd.Dir = installPath
		cmd.Env = append(os.Environ(), pluginDataDirEnv+"="+dataDir)

		client := hcplugin.NewClient(&hcplugin.ClientConfig{
			HandshakeConfig:  pluginsdk.Handshake,
			Plugins:          map[string]hcplugin.Plugin{pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Host: hostFactory(id)}},
			AllowedProtocols: []hcplugin.Protocol{hcplugin.ProtocolGRPC},
			AutoMTLS:         true,
			Cmd:              cmd,
		})

		rpcClient, err := client.Client()
		if err != nil {
			client.Kill()
			return nil, fmt.Errorf("plugins/runtime: connect to plugin %q: %w", id, err)
		}
		raw, err := rpcClient.Dispense(pluginsdk.PluginMapKey)
		if err != nil {
			client.Kill()
			return nil, fmt.Errorf("plugins/runtime: dispense plugin %q: %w", id, err)
		}
		remote, ok := raw.(*pluginsdk.RemotePlugin)
		if !ok {
			client.Kill()
			return nil, fmt.Errorf("plugins/runtime: plugin %q dispensed %T, want *pluginsdk.RemotePlugin", id, raw)
		}
		return &hcProcess{client: client, proto: rpcClient, remote: remote}, nil
	}, nil
}

// hcProcess adapts a real *hcplugin.Client (plus its dispensed
// ClientProtocol/RemotePlugin) to the spawnedProcess interface process.go's
// supervision loop depends on.
type hcProcess struct {
	client *hcplugin.Client
	proto  hcplugin.ClientProtocol
	remote *pluginsdk.RemotePlugin
}

func (h *hcProcess) Ping() error                     { return h.proto.Ping() }
func (h *hcProcess) Exited() bool                    { return h.client.Exited() }
func (h *hcProcess) Kill()                           { h.client.Kill() }
func (h *hcProcess) Remote() *pluginsdk.RemotePlugin { return h.remote }

var _ spawnedProcess = (*hcProcess)(nil)
