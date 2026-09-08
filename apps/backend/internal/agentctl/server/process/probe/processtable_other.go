//go:build !linux && !darwin

package probe

// platformProcessTableReader returns nil on every platform without a
// process-table reader (Windows). ProbeBackgroundWorkloads treats a nil
// reader as always ResultUnknown.
func platformProcessTableReader() processTableReader {
	return nil
}
