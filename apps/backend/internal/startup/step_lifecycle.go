package startup

import (
	"context"
	"math"
	"time"

	"go.uber.org/zap"
)

const (
	stallThresholdMS = 120000
	rateWindow       = 30 * time.Second
	maxETAMs         = 86400000
	maxRateSamples   = 64
)

type rateSample struct {
	at    time.Time
	delta int64
}

// stepActivation is the mutable state of one BeginStep..EndStep lifetime. A
// fresh one is created on every BeginStep, so nothing here survives past the
// next activation of the same step.
type stepActivation struct {
	spec      StepSpec
	startedAt time.Time

	measure          Measure
	promotionBlocked bool

	hasTotal   bool
	total      int64
	done       int64
	anchorDone int64

	hasAdvancedOnce bool
	lastAdvanceAt   time.Time
	samples         []rateSample

	warned map[string]bool
}

// BeginStep ends the currently active step, if any, and opens id as the new
// active step, as a single sequence-number transition. A call naming the
// already-active step is a no-op. A call naming an unregistered identifier
// opens no step and records one warning.
func (r *Reporter) BeginStep(id StepID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	spec, ok := lookupStep(id)
	if !ok {
		r.warnUnregisteredLocked(id)
		return
	}
	if r.active != nil && r.active.spec.ID == id {
		return
	}
	r.endActiveLocked()

	a := &stepActivation{spec: spec, startedAt: time.Now(), measure: MeasureOpaque, warned: map[string]bool{}}
	if spec.Measure == MeasureCounting {
		a.measure = MeasureCounting
	}
	r.active = a
	r.seq++
	if spec.Phase != r.phase {
		r.warnActivationLocked(a, "phase_mismatch")
	}
	if r.log != nil {
		r.log.Info("Startup step started", zap.String("step", string(id)), zap.String("measure", string(a.measure)))
	}
}

// EndStep closes the active step if it is the one named. A call naming a
// step that is not active changes nothing.
func (r *Reporter) EndStep(id StepID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active == nil || r.active.spec.ID != id {
		return
	}
	r.endActiveLocked()
	r.seq++
}

func (r *Reporter) endActiveLocked() {
	a := r.active
	if a == nil {
		return
	}
	if r.log != nil {
		r.log.Info("Startup step completed",
			zap.String("step", string(a.spec.ID)), zap.String("measure", string(a.measure)),
			zap.Int64("done", a.done), zap.Int64("total", a.total))
		if a.measure == MeasureCounted && a.hasTotal && a.done < a.total {
			r.log.Warn("Startup step ended below its total",
				zap.String("step", string(a.spec.ID)), zap.Int64("done", a.done), zap.Int64("total", a.total))
		}
	}
	r.active = nil
}

// SeedDone assigns the active step's done count outright from durably
// applied work, without appending a rate sample or touching the
// last-advance clock. Rejected once the total is set or the step has
// advanced.
func (r *Reporter) SeedDone(id StepID, done int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.active
	if a == nil || a.spec.ID != id {
		r.warnMismatchLocked(id, "seed_done")
		return
	}
	if done < 0 {
		r.warnActivationLocked(a, "seed_done_negative")
		return
	}
	if a.hasTotal || a.hasAdvancedOnce {
		r.warnActivationLocked(a, "seed_done_rejected")
		return
	}
	if done == a.done {
		return
	}
	a.done = done
	a.anchorDone = done
}

// SetTotal accepts the active step's total, the single promotion point for a
// step whose registry entry declares counted. A total of zero seals the
// activation opaque; any other accepted total promotes it.
func (r *Reporter) SetTotal(id StepID, total int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.active
	if a == nil || a.spec.ID != id {
		r.warnMismatchLocked(id, "set_total")
		return
	}
	if total < 0 {
		r.warnActivationLocked(a, "set_total_negative")
		return
	}
	if a.hasTotal {
		if total != a.total {
			r.warnActivationLocked(a, "set_total_rejected")
		}
		return
	}
	a.hasTotal = true
	a.total = total
	if a.done > total {
		a.done = total
	}
	if total == 0 {
		a.promotionBlocked = true
		return
	}
	if !a.promotionBlocked && a.spec.Measure == MeasureCounted {
		a.measure = MeasureCounted
	}
}

// Advance adds delta to the active step's done count, saturating at the
// total when one is known. Only the actual increase - never the requested
// delta - resets the stall clock and feeds the rate sample.
func (r *Reporter) Advance(id StepID, delta int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.active
	if a == nil || a.spec.ID != id {
		r.warnMismatchLocked(id, "advance")
		return
	}
	if delta <= 0 {
		return
	}
	newDone := saturatingAdd(a.done, delta)
	if a.hasTotal && newDone > a.total {
		newDone = a.total
		r.warnActivationLocked(a, "advance_exceeds_total")
	}
	increase := newDone - a.done
	a.done = newDone
	if increase <= 0 {
		return
	}
	now := time.Now()
	a.hasAdvancedOnce = true
	a.lastAdvanceAt = now
	samples, droppedDelta := appendSample(a.samples, rateSample{at: now, delta: increase})
	a.samples = samples
	a.anchorDone += droppedDelta
}

// Degrade moves the active step's measure down one rung (counted->counting,
// counting->opaque) and permanently blocks re-promotion for the rest of this
// activation, including when it is called while already opaque.
func (r *Reporter) Degrade(id StepID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a := r.active
	if a == nil || a.spec.ID != id {
		r.warnMismatchLocked(id, "degrade")
		return
	}
	switch a.measure {
	case MeasureCounted:
		a.measure = MeasureCounting
	case MeasureCounting:
		a.measure = MeasureOpaque
	case MeasureOpaque:
		// already at the floor; only the latch below changes.
	}
	a.promotionBlocked = true
	r.warnActivationLocked(a, "degrade")
}

func (r *Reporter) warnActivationLocked(a *stepActivation, condition string) {
	if a.warned[condition] {
		return
	}
	a.warned[condition] = true
	if r.log != nil {
		r.log.Warn("Startup step warning", zap.String("step", string(a.spec.ID)), zap.String("condition", condition))
	}
}

func (r *Reporter) warnUnregisteredLocked(id StepID) {
	if r.unregisteredWarned == nil {
		r.unregisteredWarned = map[StepID]bool{}
	}
	if r.unregisteredWarned[id] {
		return
	}
	r.unregisteredWarned[id] = true
	if r.log != nil {
		r.log.Warn("Startup step begin for unregistered identifier", zap.String("step", string(id)))
	}
}

// mismatchKey dedupes a "called for a step that isn't active" warning per
// (target step, operation): a caller bug of this shape typically repeats on
// every record in a loop, and only the first occurrence is informative.
type mismatchKey struct {
	id StepID
	op string
}

func (r *Reporter) warnMismatchLocked(id StepID, op string) {
	if r.mismatchWarned == nil {
		r.mismatchWarned = map[mismatchKey]bool{}
	}
	key := mismatchKey{id: id, op: op}
	if r.mismatchWarned[key] {
		return
	}
	r.mismatchWarned[key] = true
	if r.log != nil {
		var active StepID
		if r.active != nil {
			active = r.active.spec.ID
		}
		r.log.Warn("Startup step call for a step that is not active",
			zap.String("step", string(id)), zap.String("op", op), zap.String("active_step", string(active)))
	}
}

func saturatingAdd(a, b int64) int64 {
	sum := a + b
	if sum < a {
		return math.MaxInt64
	}
	return sum
}

// appendSample appends s to samples, trimming from the front once the ring
// exceeds maxRateSamples. It also returns the sum of the trimmed samples'
// deltas so the caller can fold it into anchorDone: rateWindowStart's
// cumulative-sum walk assumes anchorDone is the done value immediately
// before the first sample still in the slice, and silently dropping old
// samples without that adjustment understates the window's baseline,
// overstating the rate it produces.
func appendSample(samples []rateSample, s rateSample) ([]rateSample, int64) {
	samples = append(samples, s)
	var droppedDelta int64
	if len(samples) > maxRateSamples {
		trimCount := len(samples) - maxRateSamples
		for _, dropped := range samples[:trimCount] {
			droppedDelta += dropped.delta
		}
		samples = samples[trimCount:]
	}
	return samples, droppedDelta
}

// Package-level helpers follow the existing SetPhase pattern: a no-op when
// ctx carries no reporter, which keeps every call site safe in tests and
// embedded use.

func BeginStep(ctx context.Context, id StepID) {
	if r := FromContext(ctx); r != nil {
		r.BeginStep(id)
	}
}

func EndStep(ctx context.Context, id StepID) {
	if r := FromContext(ctx); r != nil {
		r.EndStep(id)
	}
}

func SeedDone(ctx context.Context, id StepID, done int64) {
	if r := FromContext(ctx); r != nil {
		r.SeedDone(id, done)
	}
}

func SetTotal(ctx context.Context, id StepID, total int64) {
	if r := FromContext(ctx); r != nil {
		r.SetTotal(id, total)
	}
}

func Advance(ctx context.Context, id StepID, delta int64) {
	if r := FromContext(ctx); r != nil {
		r.Advance(id, delta)
	}
}

func Degrade(ctx context.Context, id StepID) {
	if r := FromContext(ctx); r != nil {
		r.Degrade(id)
	}
}
