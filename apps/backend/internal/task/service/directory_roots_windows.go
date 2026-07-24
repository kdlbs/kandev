//go:build windows

package service

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func listVirtualDirectoryRoot(path string) (DirectoryListing, bool, error) {
	if path != "/" {
		return DirectoryListing{}, false, nil
	}
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return DirectoryListing{}, true, fmt.Errorf("list logical drives: %w", err)
	}
	return DirectoryListing{
		Path:      "/",
		Entries:   driveRootsFromMask(mask),
		Choosable: false,
	}, true, nil
}
