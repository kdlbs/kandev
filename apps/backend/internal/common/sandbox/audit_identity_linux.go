//go:build linux

package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func auditFileIdentity(file *os.File) (string, error) {
	var info unix.Statx_t
	if unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_INO|unix.STATX_BTIME, &info) != nil || info.Mask&unix.STATX_BTIME == 0 {
		return "", ErrInvalidPolicy
	}
	digest := sha256.Sum256(fmt.Appendf(nil, "%d:%d:%d:%d:%d", info.Dev_major, info.Dev_minor, info.Ino, info.Btime.Sec, info.Btime.Nsec))
	return hex.EncodeToString(digest[:]), nil
}
