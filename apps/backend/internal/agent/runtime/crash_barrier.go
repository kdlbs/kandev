package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// Crash barrier controls for the e2e mock profile; arming is refused outside it.
var (
	// ErrCrashBarrierDisabled is returned when arming is attempted outside the e2e mock profile.
	ErrCrashBarrierDisabled = lifecycle.ErrCrashBarrierDisabled
	// ArmCrashBarrier arms a named barrier in block or terminate mode.
	ArmCrashBarrier = lifecycle.ArmCrashBarrier
	// ReleaseCrashBarrier releases or disarms a barrier; an unarmed name is a no-op.
	ReleaseCrashBarrier = lifecycle.ReleaseCrashBarrier
)

// BarrierAfterDrain is a crash barrier name used by tests of the route.
const BarrierAfterDrain = lifecycle.BarrierAfterDrain
