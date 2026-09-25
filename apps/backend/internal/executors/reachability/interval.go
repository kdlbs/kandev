// Package reachability sweeps every eligible SSH executor on a configurable
// interval, probing each with bounded concurrency and persisting the result
// through a hysteresis-owning write path. See
// docs/specs/executors/system-design/ssh-reachability.md.
package reachability

import "time"

const (
	// probeTimeout bounds a single probe's TCP dial and SSH handshake.
	// sshDialTimeout is 30s; a shorter bound here keeps one hung host from
	// holding a semaphore slot for half the interval.
	probeTimeout = 10 * time.Second
	// passConcurrency bounds simultaneous outbound connections and file
	// descriptors while keeping a pass short.
	passConcurrency = 4
	// failureThreshold is the smallest value that suppresses a single-probe
	// blip; worst-case detection is two intervals.
	failureThreshold = 2

	// DefaultIntervalSeconds is the effective interval used when the
	// configured value is absent, negative, fractional, or unparsable.
	DefaultIntervalSeconds = 60
	// MinIntervalSeconds and MaxIntervalSeconds bound the poller's cadence.
	// 0 is a separate "disabled" sentinel and is never clamped into this
	// range.
	MinIntervalSeconds = 15
	MaxIntervalSeconds = 3600
)

// ClampInterval applies this package's bounds to a raw configured interval
// (in seconds) and reports whether the value was adjusted — worth one
// startup log line rather than a refused boot. The environment path in
// internal/common/config already normalizes an absent, empty, fractional, or
// unparsable value to DefaultIntervalSeconds; a YAML value reaches here
// unnormalized; either way this function still owns the below-minimum,
// above-maximum, and negative cases directly.
func ClampInterval(seconds int) (effective int, clamped bool) {
	switch {
	case seconds == 0:
		return 0, false
	case seconds < 0:
		return DefaultIntervalSeconds, true
	case seconds < MinIntervalSeconds:
		return MinIntervalSeconds, true
	case seconds > MaxIntervalSeconds:
		return MaxIntervalSeconds, true
	default:
		return seconds, false
	}
}
