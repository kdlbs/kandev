package subproc

import (
	"expvar"
	"strconv"
	"time"
)

// expvar maps published at package init, exposed via stdlib's
// /debug/vars handler. Each map is keyed by Throttle.name ("gh", "git",
// ...) so a single backend process surfaces both pools side-by-side
// without an extra metrics pipeline. Unnamed throttles (tests, ad-hoc
// pools) bypass these maps entirely so /debug/vars stays clean.
//
// Counters and gauges only; the wait-millis sum lets you compute a
// rolling average via two scrapes (delta(sum)/delta(acquires)). p95 /
// histograms would need a more invasive change — kept out for now since
// the host-freeze investigation only needs "is there a queue, and how
// long is the wait?"
//
// Naming follows the existing routing_* / watcher_dispatch_* precedent
// in apps/backend/internal/office/scheduler/metrics_vars.go and
// internal/orchestrator/watcher_throttle.go.
var (
	subprocCap               = expvar.NewMap("subproc_cap")
	subprocInflight          = expvar.NewMap("subproc_inflight")
	subprocWaiters           = expvar.NewMap("subproc_waiters")
	subprocAcquireTotal      = expvar.NewMap("subproc_acquire_total")
	subprocAcquireWaitMillis = expvar.NewMap("subproc_acquire_wait_millis_total")
)

// publishCap sets the gauge for the configured cap. Called once at
// NewNamedThrottle and again from SetCapForTest. The cap is published
// even when name == "" is a no-op so test pools don't drop a "0" into
// the production map.
func (t *Throttle) publishCap(c int) {
	if t.name == "" {
		return
	}
	v := new(expvar.Int)
	v.Set(int64(c))
	subprocCap.Set(t.name, v)
}

func (t *Throttle) incInflight(delta int64) {
	if t.name == "" {
		return
	}
	subprocInflight.Add(t.name, delta)
}

func (t *Throttle) incWaiters(delta int64) {
	if t.name == "" {
		return
	}
	subprocWaiters.Add(t.name, delta)
}

// incAcquire bumps the acquire counter and, when waited > 0, the
// cumulative wait gauge. Wait time is recorded in whole milliseconds —
// the host-freeze investigation cared about second-level deltas, not
// sub-ms precision, and integer math keeps the expvar.Map type stable.
func (t *Throttle) incAcquire(waited time.Duration) {
	if t.name == "" {
		return
	}
	subprocAcquireTotal.Add(t.name, 1)
	if waited > 0 {
		ms := waited.Milliseconds()
		if ms > 0 {
			subprocAcquireWaitMillis.Add(t.name, ms)
		}
	}
}

// metricInt reads a Throttle metric for tests. The expvar.Map exposes
// Get(string) but returns expvar.Var; this unwraps to int64 with a
// sane default for missing keys.
func metricInt(m *expvar.Map, key string) int64 {
	v := m.Get(key)
	if v == nil {
		return 0
	}
	parsed, err := strconv.ParseInt(v.String(), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
