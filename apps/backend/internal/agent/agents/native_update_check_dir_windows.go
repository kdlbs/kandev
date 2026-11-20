//go:build windows

package agents

import (
	"errors"
	"os"
)

var errFingerprintNotDirectory = errors.New("not a directory")

// openFingerprintDir opens path only if it is a directory. Windows has no
// FIFOs in the filesystem namespace, so opening after a type check cannot
// block.
func openFingerprintDir(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errFingerprintNotDirectory
	}
	return os.Open(path)
}
