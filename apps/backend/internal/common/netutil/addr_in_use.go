// Package netutil contains small helpers for network operations.
package netutil

// IsAddrInUse reports whether err indicates that a requested network address
// is already bound by another listener.
func IsAddrInUse(err error) bool {
	return isAddrInUse(err)
}
