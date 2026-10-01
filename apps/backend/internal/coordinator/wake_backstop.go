package coordinator

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// backstopInterval is the cadence of the level-triggered pass.
const backstopInterval = 60 * time.Second

// BackstopDuty is one per-coordinator duty a later task supplies.
type BackstopDuty func(ctx context.Context, coordinatorID string) error

// Hooks carries the duties the backstop calls for each visited coordinator:
// TurnDuties (open-turn recovery, settle and ceiling check), Lowering (the
// undo lowering retry) and Deliver (wake delivery).
type Hooks struct {
	TurnDuties BackstopDuty
	Lowering   BackstopDuty
	Deliver    BackstopDuty
}

// WakeBackstop runs the 60-second pass over the coordinators that need a
// visit. Passes run serially on one goroutine; Stop latches the value closed.
type WakeBackstop struct {
	svc      *Service
	interval time.Duration

	mu      sync.Mutex
	hooks   Hooks
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	closed  bool
}

func newWakeBackstop(svc *Service) *WakeBackstop {
	return &WakeBackstop{svc: svc, interval: backstopInterval}
}

// SetBackstopHooks merges the non-nil fields of h into the backstop's hooks.
// It is honoured only before the backstop starts.
func (s *Service) SetBackstopHooks(h Hooks) { s.backstop.setHooks(h) }

// StartWakeBackstop starts the backstop loop; the first pass runs one interval
// later. It is idempotent and a no-op after StopWakeBackstop.
func (s *Service) StartWakeBackstop(ctx context.Context) { s.backstop.Start(ctx) }

// StopWakeBackstop stops the backstop and waits for a pass in progress. It is
// idempotent and latches the backstop closed.
func (s *Service) StopWakeBackstop() { s.backstop.Stop() }

// WakeBackstopRunning reports whether the backstop loop is running.
func (s *Service) WakeBackstopRunning() bool {
	b := s.backstop
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.started && !b.closed
}

func (b *WakeBackstop) setHooks(h Hooks) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started || b.closed {
		b.svc.logger.Warn("backstop hooks set after start ignored")
		return
	}
	if h.TurnDuties != nil {
		b.hooks.TurnDuties = h.TurnDuties
	}
	if h.Lowering != nil {
		b.hooks.Lowering = h.Lowering
	}
	if h.Deliver != nil {
		b.hooks.Deliver = h.Deliver
	}
}

func (b *WakeBackstop) currentHooks() Hooks {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.hooks
}

// Start launches the loop. Calling it again without Stop, or after Stop, is a
// no-op.
func (b *WakeBackstop) Start(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started || b.closed {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.started = true
	b.wg.Add(1)
	go b.loop(loopCtx)
}

// Stop cancels the loop, waits for it, and latches the value closed.
func (b *WakeBackstop) Stop() {
	b.mu.Lock()
	b.closed = true
	cancel := b.cancel
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	b.wg.Wait()
}

func (b *WakeBackstop) loop(ctx context.Context) {
	defer b.wg.Done()
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.runPassSafely(ctx)
		}
	}
}

func (b *WakeBackstop) runPassSafely(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			b.svc.logger.Error("wake backstop pass panicked", zap.Any("panic", r))
			backstopSkippedTotal.Add(1)
		}
	}()
	b.runPass(ctx)
}

func (b *WakeBackstop) skip(what string, err error) {
	backstopSkippedTotal.Add(1)
	b.svc.logger.Warn("wake backstop skipped", zap.String("what", what), zap.Error(err))
}

// visitSet is the union of the coordinators with autonomy on, with an open or
// recently finished turn, and with an automatic claim, ordered by id. A query
// that fails is counted and the others still contribute.
func (b *WakeBackstop) visitSet(ctx context.Context, now time.Time) []string {
	store := b.svc.store
	sources := []struct {
		name string
		read func() ([]string, error)
	}{
		{"autonomous coordinators", func() ([]string, error) { return store.AutonomousCoordinatorIDs(ctx) }},
		{"coordinators with turns", func() ([]string, error) {
			return store.CoordinatorIDsWithActiveTurns(ctx, now.Add(-recentTurnWindow))
		}},
		{"coordinators with automatic claims", func() ([]string, error) { return store.CoordinatorIDsWithAutomaticClaims(ctx) }},
	}
	seen := map[string]struct{}{}
	for _, src := range sources {
		ids, err := src.read()
		if err != nil {
			if ctx.Err() == nil {
				b.skip(src.name, err)
			}
			continue
		}
		for _, id := range ids {
			seen[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// runPass visits every coordinator of the visit set once.
func (b *WakeBackstop) runPass(ctx context.Context) {
	b.svc.stopPausedCoordinators(ctx, b.skip)
	for _, id := range b.visitSet(ctx, time.Now().UTC()) {
		if ctx.Err() != nil {
			return
		}
		b.visit(ctx, id)
	}
}

func (b *WakeBackstop) visit(ctx context.Context, coordinatorID string) {
	c, err := b.svc.store.GetCoordinatorByID(ctx, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return
	}
	if err != nil {
		if ctx.Err() == nil {
			b.skip("read coordinator", err)
		}
		return
	}
	hooks := b.currentHooks()
	b.runDuty(ctx, "turn duties", hooks.TurnDuties, coordinatorID)
	b.runDuty(ctx, "lowering", hooks.Lowering, coordinatorID)
	if !c.AutonomyEnabled {
		return
	}
	if !b.recordWakes(ctx, c) {
		return
	}
	b.runDuty(ctx, "deliver", hooks.Deliver, coordinatorID)
}

func (b *WakeBackstop) runDuty(ctx context.Context, name string, duty BackstopDuty, coordinatorID string) {
	if duty == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			backstopSkippedTotal.Add(1)
			b.svc.logger.Warn("wake backstop duty panicked",
				zap.String("duty", name), zap.String("coordinator_id", coordinatorID), zap.Any("panic", r))
		}
	}()
	if err := duty(ctx, coordinatorID); err != nil && ctx.Err() == nil {
		b.skip(name, err)
	}
}

type pendingEpisode struct {
	taskID string
	ep     wakeEpisode
}

// recordWakes is the wake duty: it reads every current episode of every
// watched own task first, and only when every read succeeded records the ones
// with no stored wake. It reports false when a read failed, which abandons the
// coordinator's wake duties for the pass.
func (b *WakeBackstop) recordWakes(ctx context.Context, c *Coordinator) bool {
	kept, ok := b.readWakeEpisodes(ctx, c)
	if !ok {
		return false
	}
	for _, p := range kept {
		if ctx.Err() != nil {
			return false
		}
		_, _ = b.svc.RecordWake(ctx, c.ID, p.taskID, p.ep.Kind, p.ep.Key)
	}
	return true
}

func (b *WakeBackstop) readWakeEpisodes(ctx context.Context, c *Coordinator) ([]pendingEpisode, bool) {
	s := b.svc
	s.wakeMu.Lock()
	src := s.wakeSources
	s.wakeMu.Unlock()
	if src == nil {
		return nil, true
	}
	fail := func(err error) ([]pendingEpisode, bool) {
		if ctx.Err() == nil {
			b.skip("wake reads", err)
		}
		return nil, false
	}
	own, err := s.store.ListOwnTasks(ctx, c.ID)
	if err != nil {
		return fail(err)
	}
	set, err := s.store.EffectiveWatchSet(ctx, s.store.ro, c.ID, c.WorkspaceID)
	if err != nil {
		return fail(err)
	}
	watched := make([]OwnTask, 0, len(own))
	ids := make([]string, 0, len(own))
	for _, t := range own {
		if set.Contains(t.WorkflowID) {
			watched = append(watched, t)
			ids = append(ids, t.TaskID)
		}
	}
	existing, err := s.store.ExistingWakeKeys(ctx, c.ID, ids)
	if err != nil {
		return fail(err)
	}
	var kept []pendingEpisode
	for _, t := range watched {
		episodes, err := s.readEpisodes(ctx, src, t.WorkspaceID, t.TaskID, wakeKindOrder)
		if err != nil {
			return fail(err)
		}
		for _, ep := range episodes {
			if _, stored := existing[WakeKey{TaskID: t.TaskID, Kind: ep.Kind, EpisodeKey: ep.Key}]; !stored {
				kept = append(kept, pendingEpisode{taskID: t.TaskID, ep: ep})
			}
		}
	}
	return kept, true
}
