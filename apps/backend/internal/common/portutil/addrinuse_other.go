//go:build !windows

package portutil

// isPlatformAddrInUse has nothing to add outside Windows: syscall.EADDRINUSE
// is the code the kernel actually reports, and IsAddrInUse already matched it.
func isPlatformAddrInUse(error) bool { return false }
