//go:build windows

package logbundle

import "golang.org/x/sys/windows"

func availableDiskBytes(path string) (uint64, error) {
	directory, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeToCaller, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(directory, &freeToCaller, &totalBytes, &totalFree); err != nil {
		return 0, err
	}
	return freeToCaller, nil
}
