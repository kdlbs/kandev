package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/kandev/kandev/internal/task/models"
)

// Crash barrier names. The set is closed and provider-neutral: a barrier marks
// a point in host launch, recovery or cleanup where a test pauses or ends the
// backend process.
const (
	BarrierAfterSideEffect       = "after_side_effect"
	BarrierAfterDrain            = "after_drain"
	BarrierAfterHandshake        = "after_handshake"
	BarrierBeforePublish         = "before_publish"
	BarrierAfterDestroyConfirm   = "after_destroy_confirm"
	BarrierAfterPlacementPersist = "after_placement_persist"
	BarrierBeforePlacementCommit = "before_placement_commit"
	BarrierAfterCancelMark       = "after_cancel_mark"
)

// Crash barrier modes: block until released, or terminate the backend process.
const (
	CrashBarrierModeBlock     = "block"
	CrashBarrierModeTerminate = "terminate"
)

// crashBarrierExitCode is the status a terminated backend exits with, matching
// a process killed by SIGKILL.
const crashBarrierExitCode = 137

var (
	// ErrCrashBarrierDisabled is returned when arming is attempted outside the
	// e2e mock profile.
	ErrCrashBarrierDisabled = errors.New("crash barriers are available only when KANDEV_E2E_MOCK is true")
	// ErrCrashBarrierUnknown is returned for a name outside the closed set.
	ErrCrashBarrierUnknown = errors.New("unknown crash barrier")
	// ErrCrashBarrierMode is returned for a mode other than block or terminate.
	ErrCrashBarrierMode = errors.New("unknown crash barrier mode")
)

var crashBarrierNames = []string{
	BarrierAfterSideEffect, BarrierAfterDrain, BarrierAfterHandshake, BarrierBeforePublish,
	BarrierAfterDestroyConfirm, BarrierAfterPlacementPersist, BarrierBeforePlacementCommit, BarrierAfterCancelMark,
}

// CrashBarrierNames returns the closed barrier names in a stable order.
func CrashBarrierNames() []string { return append([]string(nil), crashBarrierNames...) }

type armedCrashBarrier struct {
	mode     string
	fired    bool
	released chan struct{}
}

// crashBarrierSet holds the armed barriers. A barrier point is one atomic load
// while nothing is armed.
type crashBarrierSet struct {
	enabled func() bool
	exit    func(int)

	armedCount atomic.Int32
	mu         sync.Mutex
	armed      map[string]*armedCrashBarrier
}

func newCrashBarrierSet(enabled func() bool, exit func(int)) *crashBarrierSet {
	return &crashBarrierSet{enabled: enabled, exit: exit, armed: map[string]*armedCrashBarrier{}}
}

var defaultCrashBarriers = newCrashBarrierSet(
	func() bool { return os.Getenv("KANDEV_E2E_MOCK") == "true" },
	os.Exit,
)

// ArmCrashBarrier arms a barrier on the process-wide set. It is refused unless
// KANDEV_E2E_MOCK is true, and with `conflict` when the name is already armed.
func ArmCrashBarrier(name, mode string) error { return defaultCrashBarriers.arm(name, mode) }

// ReleaseCrashBarrier releases or disarms a barrier; an unarmed name is a no-op.
func ReleaseCrashBarrier(name string) { defaultCrashBarriers.release(name) }

func (s *crashBarrierSet) arm(name, mode string) error {
	if !s.enabled() {
		return ErrCrashBarrierDisabled
	}
	if !knownCrashBarrier(name) {
		return fmt.Errorf("%w: %q", ErrCrashBarrierUnknown, name)
	}
	if mode != CrashBarrierModeBlock && mode != CrashBarrierModeTerminate {
		return fmt.Errorf("%w: %q", ErrCrashBarrierMode, mode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.armed[name]; exists {
		return fmt.Errorf("%w: crash barrier %q is already armed", models.ErrExecutorConflict, name)
	}
	s.armed[name] = &armedCrashBarrier{mode: mode, released: make(chan struct{})}
	s.armedCount.Add(1)
	return nil
}

func (s *crashBarrierSet) release(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	barrier, exists := s.armed[name]
	if !exists {
		return
	}
	delete(s.armed, name)
	s.armedCount.Add(-1)
	close(barrier.released)
}

// reach is a barrier point. The first caller to reach an armed name fires it:
// onFire is told the mode, then the caller blocks until release (or until ctx
// ends) or the process exits. Every other caller, and every call while nothing
// is armed, returns at once.
func (s *crashBarrierSet) reach(ctx context.Context, name string, onFire func(mode string)) {
	if s.armedCount.Load() == 0 {
		return
	}
	s.mu.Lock()
	barrier, exists := s.armed[name]
	if !exists || barrier.fired {
		s.mu.Unlock()
		return
	}
	barrier.fired = true
	s.mu.Unlock()
	if onFire != nil {
		onFire(barrier.mode)
	}
	if barrier.mode == CrashBarrierModeTerminate {
		s.exit(crashBarrierExitCode)
		return
	}
	select {
	case <-barrier.released:
	case <-ctx.Done():
	}
}

func knownCrashBarrier(name string) bool {
	for _, known := range crashBarrierNames {
		if known == name {
			return true
		}
	}
	return false
}

// reachCrashBarrier is the production barrier point; it logs the firing with
// the barrier name.
func (r *PluginRemoteExecutor) reachCrashBarrier(ctx context.Context, name string) {
	defaultCrashBarriers.reach(ctx, name, func(mode string) {
		r.logger.Warn("crash barrier fired: " + name + " (" + mode + ")")
	})
}
