//go:build windows

package portutil

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isPlatformAddrInUse covers the winsock code a real bind failure carries.
// net.Listen wraps it as *net.OpError -> *os.SyscallError -> syscall.Errno,
// and errors.Is unwraps the chain for us.
func isPlatformAddrInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE)
}
