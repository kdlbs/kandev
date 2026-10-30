//go:build !windows

package launchwrapper

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

const maxNodeFileBytes = 64 << 10

// nodeTrustBoundary names a directory whose own chain is trusted, so ancestor
// checks stop below it. Production never changes it: the empty default checks
// every ancestor up to the filesystem root. Tests replace it to treat their
// temporary directory as the root of trust.
var nodeTrustBoundary = func() string { return "" }

// LoadNodeFile reads a node wrapper file and refuses anything an unprivileged
// party could have altered. ownerUID < 0 means the process's effective user.
// Root also counts as an owner of directories above the file. No directory may
// be group or world writable; a sticky bit does not excuse it.
func LoadNodeFile(path string, ownerUID int) (Spec, error) {
	if ownerUID < 0 {
		ownerUID = os.Geteuid()
	}
	resolved, err := resolveTrusted(path, ownerUID)
	if err != nil {
		return Spec{}, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return Spec{}, nodeFileError("open", err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return Spec{}, nodeFileError("stat", err)
	}
	if err := checkOwnedNotWritable(info, ownerUID, false); err != nil {
		return Spec{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxNodeFileBytes+1))
	if err != nil || len(data) > maxNodeFileBytes {
		return Spec{}, nodeFileError("read", err)
	}
	spec, err := DecodeNode(data)
	if err != nil {
		return Spec{}, fmt.Errorf("%w: invalid envelope", ErrNodeFile)
	}
	if _, err := resolveTrusted(spec.Argv[0], ownerUID); err != nil {
		return Spec{}, err
	}
	exec, err := os.Stat(spec.Argv[0])
	if err != nil || !exec.Mode().IsRegular() || exec.Mode().Perm()&0o111 == 0 {
		return Spec{}, fmt.Errorf("%w: wrapper executable is not an executable file", ErrNodeFile)
	}
	return spec, nil
}

// resolveTrusted resolves symlinks, then checks the target and every directory
// above both the given and the resolved path.
func resolveTrusted(path string, ownerUID int) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", nodeFileError("resolve", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nodeFileError("resolve", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", nodeFileError("stat", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: not a regular file", ErrNodeFile)
	}
	if err := checkOwnedNotWritable(info, ownerUID, false); err != nil {
		return "", err
	}
	for _, p := range []string{abs, resolved} {
		for dir := filepath.Dir(p); dir != nodeTrustBoundary(); dir = filepath.Dir(dir) {
			dirInfo, err := os.Stat(dir)
			if err != nil {
				return "", nodeFileError("stat directory", err)
			}
			if err := checkOwnedNotWritable(dirInfo, ownerUID, true); err != nil {
				return "", err
			}
			if dir == filepath.Dir(dir) {
				break
			}
		}
	}
	return resolved, nil
}

func checkOwnedNotWritable(info os.FileInfo, ownerUID int, directory bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: ownership is unavailable", ErrNodeFile)
	}
	uid := int(stat.Uid)
	if uid != ownerUID && (!directory || uid != 0) {
		return fmt.Errorf("%w: unexpected owner", ErrNodeFile)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%w: writable by group or others", ErrNodeFile)
	}
	return nil
}

func nodeFileError(stage string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrNodeFile, stage)
	}
	return fmt.Errorf("%w: %s: %v", ErrNodeFile, stage, err)
}
