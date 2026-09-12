//go:build windows

package tempartifacts

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isCrossDeviceError(err error) bool {
	return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE)
}
