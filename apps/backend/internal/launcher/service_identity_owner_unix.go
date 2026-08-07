//go:build linux || darwin

package launcher

import (
	"fmt"
	"os"
	"syscall"
)

func nativeFileOwnerUID(info os.FileInfo) (int, error) {
	if info == nil {
		return 0, fmt.Errorf("filesystem metadata is nil")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported filesystem metadata type %T", info.Sys())
	}
	return int(stat.Uid), nil
}
