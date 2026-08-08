//go:build windows

package netutil

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, windows.WSAEADDRINUSE)
}
