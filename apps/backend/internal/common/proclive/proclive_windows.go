//go:build windows

package proclive

// Alive cannot portably determine process liveness on Windows.
func Alive(int64) (alive, known bool) {
	return false, false
}
