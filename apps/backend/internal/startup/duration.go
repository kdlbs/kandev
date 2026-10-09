package startup

// EstimateComponents converts an ETA in milliseconds into the whole count and
// unit AC-PLATFORM-STARTUP-PROGRESS-003.10 requires a rendering surface to
// use: 60000 ms or less rounds up to whole seconds with a floor of one second
// while work remains; anything longer rounds up to whole minutes. Never
// sub-second. Both the Go-rendered startup page and the web dialog mirror
// this rule so the two surfaces never disagree on the same snapshot.
func EstimateComponents(ms int64) (value int64, minutes bool) {
	if ms <= 60000 {
		seconds := ceilDiv(ms, 1000)
		if seconds < 1 {
			seconds = 1
		}
		return seconds, false
	}
	return ceilDiv(ms, 60000), true
}

func ceilDiv(numerator, denominator int64) int64 {
	if numerator <= 0 {
		return 0
	}
	return (numerator + denominator - 1) / denominator
}
