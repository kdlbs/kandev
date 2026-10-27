//go:build unix

package launchwrapper

import (
	"os"
	"syscall"
)

func nodePolicyOwned(info os.FileInfo, owner int) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && uint64(stat.Uid) == uint64(owner)
}
