//go:build windows

package tempartifacts

func processAlive(pid int64) (alive, known bool) {
	// Windows does not expose a portable, permission-safe signal-zero probe.
	// Unknown liveness is treated as protected by reconciliation.
	return false, false
}
