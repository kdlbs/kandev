// Package activity coordinates host-resource task work with destructive
// install-wide maintenance without importing lifecycle or storage packages.
package activity

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrBusy = errors.New("host resources are busy")

type Kind string

const (
	KindExecutionStarting  Kind = "execution_starting"
	KindExecutionPreparing Kind = "execution_preparing"
	KindExecutionRunning   Kind = "execution_running"
	KindExecutionStopping  Kind = "execution_stopping"
	KindShellCommand       Kind = "shell_command"
	KindTestCommand        Kind = "test_command"
	KindSetupScript        Kind = "setup_script"
	KindCleanupScript      Kind = "cleanup_script"
	KindDockerImageBuild   Kind = "docker_image_build"
	KindQuietPeriod        Kind = "quiet_period"
)

type Options struct {
	Now func() time.Time
}

type Coordinator struct {
	mu           sync.Mutex
	now          func() time.Time
	active       map[Kind]int
	lastActivity time.Time
	maintenance  *maintenanceState
}

type maintenanceState struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func NewCoordinator(options Options) *Coordinator {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{
		now: now, active: make(map[Kind]int), lastActivity: now().UTC(),
	}
}

type TaskLease struct {
	coordinator *Coordinator
	mu          sync.Mutex
	kind        Kind
	released    bool
}

func (l *TaskLease) Release() {
	if l == nil || l.coordinator == nil {
		return
	}
	l.mu.Lock()
	if l.released {
		l.mu.Unlock()
		return
	}
	l.released = true
	kind := l.kind
	l.mu.Unlock()
	l.coordinator.releaseTask(kind)
}

func (l *TaskLease) SetKind(kind Kind) {
	if l == nil || l.coordinator == nil || kind == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return
	}
	l.coordinator.mu.Lock()
	defer l.coordinator.mu.Unlock()
	if l.kind == kind {
		return
	}
	l.coordinator.decrementLocked(l.kind)
	l.coordinator.active[kind]++
	l.kind = kind
}

type MaintenanceLease struct {
	coordinator *Coordinator
	state       *maintenanceState
	ctx         context.Context
	once        sync.Once
}

func (l *MaintenanceLease) Context() context.Context { return l.ctx }

func (l *MaintenanceLease) Release() {
	if l == nil || l.coordinator == nil {
		return
	}
	l.once.Do(func() { l.coordinator.releaseMaintenance(l.state) })
}

func (c *Coordinator) AcquireTask(ctx context.Context, kind Kind) (*TaskLease, error) {
	for {
		c.mu.Lock()
		maintenance := c.maintenance
		if maintenance == nil {
			c.active[kind]++
			c.mu.Unlock()
			return &TaskLease{coordinator: c, kind: kind}, nil
		}
		maintenance.cancel()
		done := maintenance.done
		c.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *Coordinator) TryAcquireMaintenance(
	ctx context.Context,
	quietPeriod time.Duration,
) (*MaintenanceLease, []Kind, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maintenance != nil {
		return nil, []Kind{KindQuietPeriod}, ErrBusy
	}
	if busy := c.busyKindsLocked(); len(busy) > 0 {
		return nil, busy, ErrBusy
	}
	if quietPeriod > 0 && c.now().UTC().Sub(c.lastActivity) < quietPeriod {
		return nil, []Kind{KindQuietPeriod}, ErrBusy
	}
	maintenanceCtx, cancel := context.WithCancel(ctx)
	state := &maintenanceState{cancel: cancel, done: make(chan struct{})}
	c.maintenance = state
	return &MaintenanceLease{coordinator: c, state: state, ctx: maintenanceCtx}, nil, nil
}

func (c *Coordinator) BusyKinds() []Kind {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.busyKindsLocked()
}

func (c *Coordinator) releaseTask(kind Kind) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.decrementLocked(kind)
	if len(c.active) == 0 {
		c.lastActivity = c.now().UTC()
	}
}

func (c *Coordinator) decrementLocked(kind Kind) {
	if c.active[kind] <= 1 {
		delete(c.active, kind)
		return
	}
	c.active[kind]--
}

func (c *Coordinator) releaseMaintenance(state *maintenanceState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maintenance != state {
		return
	}
	c.maintenance = nil
	state.cancel()
	close(state.done)
}

func (c *Coordinator) busyKindsLocked() []Kind {
	busy := make([]Kind, 0, len(c.active))
	for kind := range c.active {
		busy = append(busy, kind)
	}
	sort.Slice(busy, func(i, j int) bool { return busy[i] < busy[j] })
	return busy
}
