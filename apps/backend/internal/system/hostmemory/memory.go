// Package hostmemory reports host-wide memory pressure so launch admission can
// hold new sessions back before the machine is driven into swap or an
// out-of-memory reset.
//
// The package is a leaf: it observes the operating system only. It never
// launches, stops, or mutates anything, and the pure threshold decision
// (Pressured) carries no build tag so it is exercised on every platform.
package hostmemory

import (
	"context"
	"os"
	"strconv"
	"strings"
)

const (
	// EnvMinAvailableMB is the available-physical-memory floor in MiB.
	EnvMinAvailableMB = "KANDEV_SESSION_MEMORY_MIN_AVAILABLE_MB"
	// EnvMaxCommitPercent is the commit-charge ceiling in percent.
	EnvMaxCommitPercent = "KANDEV_SESSION_MEMORY_MAX_COMMIT_PERCENT"

	// DefaultMinAvailableBytes is the shipped available-physical-memory floor.
	DefaultMinAvailableBytes uint64 = 8 << 30 // 8 GiB
	// DefaultMaxCommitPercent is the shipped commit-charge ceiling. It leaves
	// headroom below a full commit limit so new launches are held before the
	// host starts failing allocations or a host watchdog restarts the instance.
	DefaultMaxCommitPercent float64 = 75
)

// Reading is one observation of host memory. The counters are installation
// totals, never per-process or per-session data.
type Reading struct {
	TotalBytes       uint64
	AvailableBytes   uint64
	CommitUsedBytes  uint64
	CommitLimitBytes uint64
	// CommitKnown reports whether the commit pair is usable. A platform
	// without a commit reading leaves it false, and the gate judges only the
	// available-memory dimension there.
	CommitKnown bool
	// Complete reports whether the reading is usable at all. False means the
	// platform could not produce a reading and the caller must fail open.
	Complete bool
}

// Gate is the configured memory-pressure threshold pair.
type Gate struct {
	Enabled           bool
	MinAvailableBytes uint64
	MaxCommitPercent  float64
}

// Pressured reports whether r is under pressure under g. unknown is true when
// the reading is incomplete, so the caller can fail open (allow the launch)
// rather than stall every automatic launch on a host it cannot measure. A
// dimension a platform cannot report is simply not judged, which is the same
// fail-open posture one level down.
func (g Gate) Pressured(r Reading) (pressured bool, unknown bool) {
	if !g.Enabled {
		return false, false
	}
	if !r.Complete {
		return false, true
	}
	if g.MinAvailableBytes > 0 && r.AvailableBytes < g.MinAvailableBytes {
		return true, false
	}
	if g.MaxCommitPercent > 0 && r.CommitKnown && r.CommitLimitBytes > 0 {
		percent := float64(r.CommitUsedBytes) / float64(r.CommitLimitBytes) * 100
		if percent > g.MaxCommitPercent {
			return true, false
		}
	}
	return false, false
}

// Reader reads the current host memory state.
type Reader interface {
	Read(ctx context.Context) (Reading, error)
}

// ReadConfig resolves the guard thresholds from the process environment.
// Enablement comes from the features.sessionMemoryGuard runtime flag; a
// missing or unparsable threshold keeps its shipped default.
func ReadConfig(enabled bool) Gate {
	gate := Gate{
		Enabled:           enabled,
		MinAvailableBytes: DefaultMinAvailableBytes,
		MaxCommitPercent:  DefaultMaxCommitPercent,
	}
	if value, ok := os.LookupEnv(EnvMinAvailableMB); ok {
		if mb, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && mb >= 0 {
			gate.MinAvailableBytes = uint64(mb) << 20
		}
	}
	if value, ok := os.LookupEnv(EnvMaxCommitPercent); ok {
		if percent, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && percent >= 0 {
			gate.MaxCommitPercent = percent
		}
	}
	return gate
}
