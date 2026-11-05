//go:build unix

package journal

import (
	"errors"
	"syscall"
)

func classifyOpenFilesystemCause(err error) OpenReason {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return OpenReasonFilesystemFull
	case errors.Is(err, syscall.EDQUOT):
		return OpenReasonDiskQuotaExceeded
	case errors.Is(err, syscall.EIO):
		return OpenReasonIOFailure
	default:
		return ""
	}
}
