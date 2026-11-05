//go:build !windows

package launchwrapper

import (
	"os"
	"path/filepath"
	"syscall"
)

// ResolveAllocDir checks an operator-supplied allocation directory and returns
// it with symlinks resolved. It must be absolute, an existing directory owned
// by the agentctl process owner, and not writable by group or others. The
// checks apply to the resolved target.
func ResolveAllocDir(path string) (string, error) {
	if path == "" {
		return "", allocDirError("not supplied")
	}
	if !filepath.IsAbs(path) {
		return "", allocDirError("not an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", allocDirError("cannot be resolved")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", allocDirError("not a directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", allocDirError("ownership is unavailable")
	}
	if int(stat.Uid) != allocDirOwnerUID() {
		return "", allocDirError("unexpected owner")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "", allocDirError("writable by group or others")
	}
	return resolved, nil
}
