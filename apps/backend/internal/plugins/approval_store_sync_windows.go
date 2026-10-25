//go:build windows

package plugins

// Windows does not provide a usable directory fsync handle through Go's os
// package. The ledger file is flushed before rename, so the replacement is
// complete without attempting the read-only directory Sync that fails on
// Windows.
func syncApprovalLedgerDirectory(string) error {
	return nil
}
