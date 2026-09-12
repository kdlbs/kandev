package sqlite

import (
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/persistence"
)

// loopLivenessActivationKey is the kandev_meta key published once a
// boot has verified all three causation_id columns exist
// (REQ-OFFICE-LOOP-LIVENESS-002/005). See docs/specs/office/system-design/
// loop-liveness.md, "Activation".
const loopLivenessActivationKey = "telemetry.office_loop_liveness.activated_at"

// causationIDColumn is the column all three activation probes check.
const causationIDColumn = "causation_id"

// activateLoopLiveness writes loopLivenessActivationKey only after a
// positive probe confirms all three causation_id columns exist. Mirrors
// activateRunOutcome: the migration runner swallows failures at WARN,
// so this probe is what stands between a failed migration and a reader
// wrongly believing correlation and terminal-shape classification are
// live. WriteMetaKeyIfAbsent makes the write replay-safe — the instant
// is never overwritten once set, because every run requested before it
// must classify as pre_activation forever, not just until the next
// boot. Any failure here is logged and swallowed — activation is
// best-effort published fact, never a boot-blocking requirement.
func (r *Repository) activateLoopLiveness() {
	if err := persistence.EnsureMetaTable(r.db); err != nil {
		r.logActivationWarn("ensure kandev_meta table failed", err)
		return
	}
	for _, probe := range []struct{ table, column string }{
		{"office_routine_runs", causationIDColumn},
		{"agent_wakeup_requests", causationIDColumn},
		{"runs", causationIDColumn},
	} {
		exists, err := columnExists(r.db, probe.table, probe.column)
		if err != nil {
			r.logActivationWarn("loop liveness schema probe failed", err)
			return
		}
		if !exists {
			return
		}
	}
	if _, err := persistence.WriteMetaKeyIfAbsent(
		r.db, loopLivenessActivationKey, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		r.logActivationWarn("write loop liveness activation key failed", err)
	}
}

// LoopLivenessActivation returns the published activation instant and
// whether it has been published at all. A run requested before the
// instant, or read when activation was never published, must classify
// as pre_activation (AC-005.6) rather than risk a false silent_success.
// Read failures are swallowed here (logged only) because every existing
// caller uses this for a safe-default classification, not a decision
// that must fail loud; the /loop-health read that must distinguish
// "unpublished" from "unreadable" uses LoopLivenessActivationChecked.
func (r *Repository) LoopLivenessActivation() (at time.Time, published bool) {
	at, published, err := r.LoopLivenessActivationChecked()
	if err != nil && r.log != nil {
		r.log.Warn("read loop liveness activation key failed", zap.Error(err))
	}
	return at, published
}

// LoopLivenessActivationChecked is LoopLivenessActivation with the read
// or parse error surfaced instead of swallowed, for a caller that must
// fail loud (REQ-OFFICE-LOOP-LIVENESS-004, AC-004.10) rather than let a
// broken kandev_meta read silently render as "not yet activated".
func (r *Repository) LoopLivenessActivationChecked() (at time.Time, published bool, err error) {
	raw, err := persistence.ReadMetaKey(r.db, loopLivenessActivationKey)
	if err != nil {
		return time.Time{}, false, err
	}
	if raw == "" {
		return time.Time{}, false, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return parsed, true, nil
}
