package reachability

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// probeFunc is the injectable seam over a single probe attempt. defaultProbe
// is the production implementation; tests substitute a fake so poller
// lifecycle/concurrency behavior can be verified under testing/synctest
// without real SSH or network I/O.
type probeFunc func(ctx context.Context, executor *models.Executor) agentruntime.SSHProbeOutcome

// Poller sweeps every eligible SSH executor on intervalSeconds, probing each
// with bounded concurrency and persisting results through store. An
// intervalSeconds of 0 disables the scheduled sweep entirely — the poller
// still starts (so ProbeNow keeps working) but never runs a pass on its own.
//
// Start and Stop are idempotent. Stop cancels every in-flight unit of work
// registered on wg — the scheduled loop, the pass it is running, and any
// off-cycle ProbeNow call — and waits for all of them to drain before
// returning, so a probe cancelled by Stop is guaranteed to never be
// persisted (see probeAndPersist).
type Poller struct {
	store           *store
	log             *logger.Logger
	intervalSeconds int
	probe           probeFunc
	// publisher is optional: nil until SetPublisher wires the event bus, so
	// task 03's own tests and callers that never call it keep working
	// unchanged. Set once at construction time in production.
	publisher *Publisher

	// inflight coalesces concurrent ProbeAndWait callers for the same
	// executor id into a single dial. Zero value is ready to use.
	inflight singleflight.Group

	// mu guards started/stopping/ctx/cancel/wg against concurrent
	// Start/Stop/ProbeNow calls. acquire() registers new work on wg under mu
	// so it can never race a concurrent Stop's wg.Wait() observing the
	// counter transition from zero.
	mu       sync.Mutex
	started  bool
	stopping bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// passMu guards passRunning, the single-flight gate for the scheduled
	// pass: an overlapping tick is dropped and counted, never queued.
	passMu      sync.Mutex
	passRunning bool
}

// New builds a Poller. intervalSeconds is clamped via ClampInterval; a
// clamp is logged once rather than refusing to start.
func New(repo Repository, intervalSeconds int, log *logger.Logger) *Poller {
	effective, clamped := ClampInterval(intervalSeconds)
	if clamped {
		log.Warn("executor ssh reachability: interval clamped",
			zap.Int("configured", intervalSeconds), zap.Int("effective", effective))
	}
	return &Poller{
		store:           &store{repo: repo, log: log},
		log:             log,
		intervalSeconds: effective,
		probe:           defaultProbe,
	}
}

// SetPublisher wires the event publisher used to announce a state/reason
// change. Optional — a Poller with no publisher still probes and persists
// exactly as before, it just never publishes.
func (p *Poller) SetPublisher(publisher *Publisher) {
	p.publisher = publisher
}

// Start launches the background loop. Calling Start more than once without
// an intervening Stop is a no-op.
func (p *Poller) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	p.started = true
	p.stopping = false
	p.ctx = runCtx
	p.cancel = cancel
	if p.intervalSeconds > 0 {
		p.wg.Add(1)
	}
	p.mu.Unlock()

	if p.intervalSeconds > 0 {
		go p.loop(runCtx)
	}
}

// Stop cancels the loop and every in-flight probe, then waits for all of
// them to drain. Calling Stop before Start, or twice in a row, is a no-op.
func (p *Poller) Stop() {
	p.mu.Lock()
	if !p.started || p.stopping {
		p.mu.Unlock()
		return
	}
	p.stopping = true
	cancel := p.cancel
	p.mu.Unlock()

	cancel()
	p.wg.Wait()

	p.mu.Lock()
	p.started = false
	p.stopping = false
	p.ctx = nil
	p.cancel = nil
	p.mu.Unlock()
}

// ProbeNow runs one off-cycle probe for executor, outside the scheduled
// cadence, tracked on the same WaitGroup/context as the scheduled loop so
// Stop drains it too. Returns false without doing anything when the poller
// is not currently accepting new work.
func (p *Poller) ProbeNow(executor *models.Executor) bool {
	ctx, ok := p.acquire()
	if !ok {
		return false
	}
	go func() {
		defer p.wg.Done()
		p.probeAndPersist(ctx, executor)
	}()
	return true
}

// acquire registers one unit of in-flight work on wg, gated by mu so it can
// never race a concurrent Stop's wg.Wait(). It refuses when the poller isn't
// started or a Stop is already underway.
func (p *Poller) acquire() (context.Context, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started || p.stopping {
		return nil, false
	}
	p.wg.Add(1)
	return p.ctx, true
}

func (p *Poller) loop(ctx context.Context) {
	defer p.wg.Done()

	ticker := time.NewTicker(time.Duration(p.intervalSeconds) * time.Second)
	defer ticker.Stop()

	p.dispatchPass(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.dispatchPass(ctx)
		}
	}
}

// dispatchPass starts one pass if none is currently running. An overlapping
// dispatch is dropped and counted rather than queued, so a slow pass never
// builds up a backlog of pending passes. The pass itself runs on its own
// WaitGroup unit so Stop drains it even after loop itself has returned.
func (p *Poller) dispatchPass(ctx context.Context) {
	p.passMu.Lock()
	if p.passRunning {
		p.passMu.Unlock()
		passSkippedTotal.Add(1)
		return
	}
	p.passRunning = true
	p.passMu.Unlock()

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		defer func() {
			p.passMu.Lock()
			p.passRunning = false
			p.passMu.Unlock()
		}()
		p.runPass(ctx)
	}()
}

// runPass lists every eligible executor and probes each with bounded
// concurrency. A listing failure abandons the pass without writing anything.
// Cancellation stops dispatching new probes and waits only for the ones
// already started to finish or self-cancel.
func (p *Poller) runPass(ctx context.Context) {
	start := time.Now()
	executors, err := p.store.repo.ListSSHExecutorsForReachability(ctx)
	if err != nil {
		p.log.Warn("executor ssh reachability: list failed", zap.Error(err))
		return
	}

	sem := make(chan struct{}, passConcurrency)
	var wg sync.WaitGroup
	for _, executor := range executors {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func(executor *models.Executor) {
			defer wg.Done()
			defer func() { <-sem }()
			p.probeAndPersist(ctx, executor)
		}(executor)
	}
	wg.Wait()
	probeDurationMsLastRun.Set(time.Since(start).Milliseconds())
}

// probeAndPersist runs one probe and records it, unless the probe was
// cancelled — a cancelled outcome is not an observation about the host and
// must never be written (see agentruntime.SSHProbeOutcome.Cancelled).
func (p *Poller) probeAndPersist(ctx context.Context, executor *models.Executor) {
	outcome := p.probe(ctx, executor)
	if outcome.Cancelled {
		probeDiscardedTotal.Add(1)
		return
	}
	p.observeAndPublish(ctx, executor, outcome)
}

// observeAndPublish records outcome through store.Observe and, when the
// write actually changed the stored state or reason, announces it through
// the publisher — the one propagation path every probe primitive
// (scheduled pass, ProbeNow, ProbeAndWait) shares.
func (p *Poller) observeAndPublish(ctx context.Context, executor *models.Executor, outcome agentruntime.SSHProbeOutcome) observeResult {
	result := p.store.Observe(ctx, executor, outcome, time.Now().UTC())
	if result.Changed && p.publisher != nil {
		p.publisher.PublishChanged(ctx, result.After, p.EffectiveIntervalSeconds())
	}
	return result
}
