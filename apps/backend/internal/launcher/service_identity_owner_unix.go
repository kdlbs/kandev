//go:build linux || darwin

package launcher

import (
	"fmt"
	"os"
	"syscall"
)

func nativePathOwnerUID(path string) (int, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported filesystem metadata type %T", info.Sys())
	}
	return int(stat.Uid), nil
}
