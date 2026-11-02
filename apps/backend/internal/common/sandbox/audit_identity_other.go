//go:build !linux

package sandbox

import "os"

// Persistent birth identity is required before an audit source can be refreshed.
func auditFileIdentity(_ *os.File) (string, error) { return "", ErrInvalidPolicy }
