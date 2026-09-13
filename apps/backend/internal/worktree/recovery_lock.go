package worktree

import "os"

type recoveryLock struct {
	file *os.File
}

func (f *recoveryLock) Close() error {
	if f == nil || f.file == nil {
		return nil
	}
	return releaseRecoveryOperation(f.file)
}
