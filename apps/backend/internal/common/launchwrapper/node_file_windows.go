//go:build windows

package launchwrapper

import "fmt"

// LoadNodeFile always refuses: there is no ownership model to verify here.
func LoadNodeFile(string, int) (Spec, error) {
	return Spec{}, fmt.Errorf("%w: unsupported on this platform", ErrNodeFile)
}
