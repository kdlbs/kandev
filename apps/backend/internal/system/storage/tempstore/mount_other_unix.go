//go:build !windows && !linux && !darwin

package tempstore

import "errors"

type mountReader struct{}

func newMountReader() MountReader { return mountReader{} }

func (mountReader) Identity(string) (string, error) {
	return "", errors.New("temporary mount identity is unsupported on this platform")
}
