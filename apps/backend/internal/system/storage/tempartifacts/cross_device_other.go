//go:build !aix && !android && !darwin && !dragonfly && !freebsd && !hurd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package tempartifacts

func isCrossDeviceError(error) bool { return false }
