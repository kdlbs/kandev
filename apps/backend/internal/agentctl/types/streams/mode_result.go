package streams

// ModeResult reports what a mode change actually achieved.
//
// It exists because the previous event was built from the requested mode: a
// clamped, refused or ignored mode was indistinguishable from an applied one,
// both in the UI and in the log.
type ModeResult struct {
	// Requested is the mode the caller asked for.
	Requested string
	// Effective is the mode the agent reports as current. It equals Requested
	// on a clean apply and differs when the agent clamped the request.
	Effective string
	// Confirmed is true only when the agent reported Effective within the
	// settle window. False means the mode may have applied but was not observed.
	Confirmed bool
}

// Applied reports whether the agent confirmed the exact requested mode.
func (r ModeResult) Applied() bool {
	return r.Confirmed && r.Effective == r.Requested
}
