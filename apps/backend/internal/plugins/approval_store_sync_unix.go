//go:build !windows

package plugins

import "os"

// syncApprovalLedgerDirectory flushes the rename metadata on platforms that
// expose directory handles to fsync. The ledger file is already synced before
// the rename.
func syncApprovalLedgerDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
