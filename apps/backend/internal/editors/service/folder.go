package service

import (
	"errors"
	"os/exec"
	"runtime"
)

var ErrFolderUnavailable = errors.New("folder opener not available")

func folderOpenCommand(platform string) string {
	switch platform {
	case "darwin":
		return "open"
	case "linux":
		return "xdg-open"
	case "windows":
		return "explorer"
	default:
		return ""
	}
}

// FolderOpeningAvailable checks the same host executable used by OpenFolder.
func FolderOpeningAvailable() bool {
	command := folderOpenCommand(runtime.GOOS)
	if command == "" {
		return false
	}
	_, err := exec.LookPath(command)
	return err == nil
}
