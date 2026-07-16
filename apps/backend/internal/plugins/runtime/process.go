package runtime

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// Default supervision tuning, per the task's frozen semantics: ping every
// 30s, 3 consecutive failures (or a detected process exit) triggers a
// restart, backed off 1s/2s/4s/8s/16s across up to 5 attempts before giving
// up.
const (
	defaultPingInterval           = 30 * time.Second
	defaultMaxConsecutiveFailures = 3
	defaultMaxRestartAttempts     = 5
)

// defaultRestartBackoff returns a fresh slice (callers may hold onto and
// mutate the one an Option sets, so Manager must not share a single
// package-level slice across instances).
func defaultRestartBackoff() []time.Duration {
	return []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
}

// spawnedProcess is the minimal surface the supervision loop needs from a
// running plugin subprocess. hcProcess (manager.go) implements this over a
// real *hcplugin.Client; tests inject fakes to unit-test supervision
// decisions without spawning a real process.
type spawnedProcess interface {
	// Ping checks that the plugin's control connection is still healthy.
	Ping() error
	// Exited reports whether the subprocess has already exited.
	Exited() bool
	// Kill terminates the subprocess if still running. Idempotent.
	Kill()
	// Remote returns the Go-native client for calling the plugin's RPCs.
	Remote() *pluginsdk.RemotePlugin
}

// process supervises one spawned plugin: periodic health pings, crash
// detection, and restart-with-backoff, up to maxRestartAttempts before
// giving up permanently (until Manager.Start is called again). Lifecycle is
// owned by Manager (start/stop), per apps/backend/AGENTS.md's
// goroutine-ownership convention: start registers the loop on wg, stop
// closes stopCh and waits for it to drain.
type process struct {
	id  string
	log *logger.Logger

	spawnFn        func() (spawnedProcess, error)
	onStatusChange func(id string, healthy bool)

	pingInterval        time.Duration
	maxConsecutiveFails int
	restartBackoff      []time.Duration
	maxRestartAttempts  int

	// sleepFn blocks for d or until stopCh fires, returning false if
	// stopped first. Overridable in tests so supervision-decision tests
	// never perform a real sleep.
	sleepFn func(stopCh <-chan struct{}, d time.Duration) bool

	// onTick, if set, runs once per supervision-loop iteration (after that
	// iteration's outcome has been fully handled), letting tests wait
	// deterministically for N cycles instead of racing the goroutine with
	// a sleep.
	onTick func()

	mu       sync.Mutex
	current  spawnedProcess
	failures int
	restarts int
	gaveUp   bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// newProcess builds a process with default tuning. Callers may override
// pingInterval/maxConsecutiveFails/restartBackoff/maxRestartAttempts/
// sleepFn/onTick before calling start.
func newProcess(id string, log *logger.Logger, spawnFn func() (spawnedProcess, error), onStatusChange func(string, bool)) *process {
	return &process{
		id:                  id,
		log:                 log,
		spawnFn:             spawnFn,
		onStatusChange:      onStatusChange,
		pingInterval:        defaultPingInterval,
		maxConsecutiveFails: defaultMaxConsecutiveFailures,
		restartBackoff:      defaultRestartBackoff(),
		maxRestartAttempts:  defaultMaxRestartAttempts,
		sleepFn:             sleepOrStop,
		stopCh:              make(chan struct{}),
	}
}

// sleepOrStop is the production sleepFn: blocks for d (or returns
// immediately if d<=0) unless stopCh fires first, per the select-based
// backoff convention in apps/backend/AGENTS.md ("never time.Sleep in a
// retry/backoff loop").
func sleepOrStop(stopCh <-chan struct{}, d time.Duration) bool {
	if d <= 0 {
		select {
		case <-stopCh:
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-stopCh:
		return false
	}
}

// start spawns the process for the first time and launches the supervision
// loop. The initial spawn error is returned synchronously so a bad
// executable path/handshake failure fails Manager.Start fast rather than
// being discovered on the first supervision tick.
func (p *process) start() error {
	proc, err := p.spawnFn()
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.current = proc
	p.mu.Unlock()

	p.wg.Add(1)
	go p.run()
	return nil
}

// stop signals the supervision loop to exit, waits for it to drain, and
// kills any still-running process. Idempotent-safe to call at most once per
// process (Manager.Stop removes the process from its map before calling
// this, so a second Stop(id) for the same id is a no-op at the Manager
// level).
func (p *process) stop() {
	close(p.stopCh)
	p.wg.Wait()
	p.mu.Lock()
	if p.current != nil {
		p.current.Kill()
		p.current = nil
	}
	p.mu.Unlock()
}

// remote returns the current live RemotePlugin, if any.
func (p *process) remote() (*pluginsdk.RemotePlugin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return nil, false
	}
	return p.current.Remote(), true
}

// ping issues an on-demand health ping against the current process.
func (p *process) ping() error {
	p.mu.Lock()
	cur := p.current
	p.mu.Unlock()
	if cur == nil {
		return errNotRunning(p.id)
	}
	return cur.Ping()
}

// restartCount returns how many times this process has been successfully
// restarted since it was started.
func (p *process) restartCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restarts
}

// run is the supervision loop: on every tick (pingInterval, or immediately
// if stopped) it health-checks the current process, driving restart on
// failure. It returns either when stopCh fires or the process permanently
// gives up after exhausting restart attempts.
func (p *process) run() {
	defer p.wg.Done()
	for {
		if !p.sleepFn(p.stopCh, p.pingInterval) {
			return
		}
		gaveUp := p.tick()
		if p.onTick != nil {
			p.onTick()
		}
		if gaveUp {
			return
		}
	}
}

// tick performs one supervision check. Returns true if the process just
// permanently gave up (restart attempts exhausted), signalling run to stop
// looping.
func (p *process) tick() bool {
	p.mu.Lock()
	cur := p.current
	p.mu.Unlock()
	if cur == nil {
		return false
	}

	if cur.Exited() {
		p.log.Warn("plugin process exited unexpectedly", zap.String("plugin_id", p.id))
		return p.handleFailureAndRestart()
	}

	if err := cur.Ping(); err != nil {
		p.mu.Lock()
		p.failures++
		reachedThreshold := p.failures >= p.maxConsecutiveFails
		p.mu.Unlock()
		if reachedThreshold {
			p.log.Warn("plugin health check failed repeatedly",
				zap.String("plugin_id", p.id), zap.Error(err))
			return p.handleFailureAndRestart()
		}
		return false
	}

	p.mu.Lock()
	hadFailures := p.failures > 0
	p.failures = 0
	p.mu.Unlock()
	if hadFailures {
		p.notify(true)
	}
	return false
}

// handleFailureAndRestart marks the plugin unhealthy, kills whatever is
// left of the old process, and attempts to respawn it with backoff up to
// maxRestartAttempts times. Returns true if every attempt failed (the
// process has given up permanently).
func (p *process) handleFailureAndRestart() bool {
	p.notify(false)

	p.mu.Lock()
	if p.current != nil {
		p.current.Kill()
		p.current = nil
	}
	p.failures = 0
	p.mu.Unlock()

	for attempt := 0; attempt < p.maxRestartAttempts; attempt++ {
		delay := time.Duration(0)
		if attempt < len(p.restartBackoff) {
			delay = p.restartBackoff[attempt]
		}
		if !p.sleepFn(p.stopCh, delay) {
			return false // stopping, not giving up
		}

		proc, err := p.spawnFn()
		if err != nil {
			p.log.Warn("plugin restart attempt failed",
				zap.String("plugin_id", p.id), zap.Int("attempt", attempt+1), zap.Error(err))
			continue
		}

		p.mu.Lock()
		p.current = proc
		p.restarts++
		p.mu.Unlock()
		p.notify(true)
		return false
	}

	p.log.Error("plugin exhausted restart attempts, giving up",
		zap.String("plugin_id", p.id), zap.Int("max_attempts", p.maxRestartAttempts))
	p.mu.Lock()
	p.gaveUp = true
	p.mu.Unlock()
	return true
}

// notify invokes onStatusChange, if set.
func (p *process) notify(healthy bool) {
	if p.onStatusChange != nil {
		p.onStatusChange(p.id, healthy)
	}
}
