//go:build windows

package netutil

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func isAddrInUse(err error) bool {
	// Windows can report WSAEACCES when a wildcard bind conflicts with a
	// loopback listener, even though the requested port is already occupied.
	return errors.Is(err, syscall.EADDRINUSE) ||
		errors.Is(err, windows.WSAEADDRINUSE) ||
		errors.Is(err, windows.WSAEACCES)
}
