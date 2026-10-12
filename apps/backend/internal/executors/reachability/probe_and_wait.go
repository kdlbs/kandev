package reachability

import (
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// ProbeResult is what ProbeAndWait returns: the durable record when available,
// or the locally projected observation when persistence failed, plus whether
// that specific write actually landed.
type ProbeResult struct {
	Record    *models.ExecutorReachability
	Persisted bool
}

// probeAndWaitOutcome is the value singleflight.Group shares across every
// coalesced caller for one key. It is deliberately independent of any
// caller's ProbeResult — ProbeAndWait copies out of it before returning, so
// no caller can mutate what another caller already received.
type probeAndWaitOutcome struct {
	ran       bool
	persisted bool
	record    *models.ExecutorReachability
}

// ProbeAndWait runs one probe for executor synchronously and returns its
// persisted record, coalescing overlapping callers for the same executor id
// into a single dial via singleflight — the primitive behind the immediate-
// probe HTTP route. Unlike ProbeNow, this does not accept (or run on) any
// caller-supplied context: like every other off-cycle probe it runs on the
// poller's own context/WaitGroup, registered through acquire(), so a
// disconnecting HTTP caller neither cancels it nor affects a caller still
// waiting on the same coalesced result, and Stop still drains it.
//
// ran is false only when the poller itself refused new work (not started, or
// stopping); it does not report whether the probe outcome was a success —
// see ProbeResult.Persisted for whether the result was actually stored.
func (p *Poller) ProbeAndWait(executor *models.Executor) (ProbeResult, bool) {
	v, _, _ := p.inflight.Do(probeKey(executor), func() (any, error) {
		return p.probeAndWaitOnce(executor), nil
	})
	outcome := v.(probeAndWaitOutcome)
	if !outcome.ran {
		return ProbeResult{}, false
	}
	var record *models.ExecutorReachability
	if outcome.record != nil {
		cp := *outcome.record
		record = &cp
	}
	return ProbeResult{Record: record, Persisted: outcome.persisted}, true
}

// probeKey includes the connection identity, not only the executor ID. A
// save can change the host or its pinned key while an older probe is still in
// flight; those observations must not share a singleflight result.
func probeKey(executor *models.Executor) string {
	if executor == nil {
		return "<nil>"
	}
	values := make([]string, 0, len(connectionConfigKeys)+1)
	values = append(values, executor.ID)
	for _, key := range connectionConfigKeys {
		values = append(values, key+"="+executor.Config[key])
	}
	return strings.Join(values, "\x00")
}

// probeAndWaitOnce is the singleflight leader's body: acquire a WaitGroup
// slot exactly like every other off-cycle unit of work, probe, and persist.
func (p *Poller) probeAndWaitOnce(executor *models.Executor) probeAndWaitOutcome {
	ctx, ok := p.acquire()
	if !ok {
		return probeAndWaitOutcome{}
	}
	defer p.wg.Done()
	if !p.acquireProbeSlot(ctx) {
		return probeAndWaitOutcome{ran: true}
	}
	defer p.releaseProbeSlot()

	outcome := p.probe(ctx, executor)
	if outcome.Cancelled {
		probeDiscardedTotal.Add(1)
		return probeAndWaitOutcome{ran: true}
	}
	result := p.observeAndPublish(ctx, executor, outcome)
	record := result.After
	if record == nil {
		record = result.Observed
	}
	return probeAndWaitOutcome{ran: true, persisted: result.Applied, record: record}
}

// EffectiveIntervalSeconds returns the clamped interval this poller is
// running with (0 when scheduled probing is disabled), for a caller (the
// reachability DTO) that needs to report probing_enabled/
// probe_interval_seconds without duplicating ClampInterval's rules.
func (p *Poller) EffectiveIntervalSeconds() int {
	return p.intervalSeconds
}
