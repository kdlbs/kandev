//go:build !windows

package service

func listVirtualDirectoryRoot(_ string) (DirectoryListing, bool, error) {
	return DirectoryListing{}, false, nil
}
