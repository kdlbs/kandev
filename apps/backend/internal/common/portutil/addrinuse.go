package portutil

import (
	"errors"
	"syscall"
)

// IsAddrInUse reports whether err is the operating system's "address already
// in use" bind failure, on every platform kandev builds for.
//
// Testing errors.Is(err, syscall.EADDRINUSE) alone is not enough. On Windows
// the runtime returns the winsock code WSAEADDRINUSE (10048), while
// syscall.EADDRINUSE in the Windows syscall package is a distinct synthetic
// value (536870914) that no bind ever produces. The comparison is therefore
// always false there, and callers silently lose the retry path they wrote —
// see isPlatformAddrInUse in addrinuse_windows.go.
//
// Matching on the error text is not a workaround either: Windows words the
// failure as "Only one usage of each socket address ... is normally
// permitted", which shares no substring with the Unix "address already in
// use", and both strings are localized by the OS.
func IsAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	return isPlatformAddrInUse(err)
}
