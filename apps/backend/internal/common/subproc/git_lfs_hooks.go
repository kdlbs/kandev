package subproc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// lfsHooks owns the Kandev-written hooks directory whose only hook runs the
// Git LFS pre-push step. A directory that fails verification is replaced by a
// fresh uniquely named one, so a push already using the old path never sees a
// directory change under it.
var lfsHooks struct {
	sync.Mutex
	root    string
	current map[string]string // git-lfs path -> hooks directory
}

func lfsPrePushScript(lfsPath string) string {
	quoted := "'" + strings.ReplaceAll(lfsPath, "'", `'\''`) + "'"
	return "#!/bin/sh\nexec " + quoted + " pre-push \"$@\"\n"
}

func lfsHooksDir(lfsPath string) (string, error) {
	lfsHooks.Lock()
	defer lfsHooks.Unlock()
	want := lfsPrePushScript(lfsPath)
	if dir := lfsHooks.current[lfsPath]; dir != "" && lfsHookValid(dir, want) {
		return dir, nil
	}
	if lfsHooks.root == "" || !isDir(lfsHooks.root) {
		root, err := os.MkdirTemp("", "kandev-git-hooks-")
		if err != nil {
			return "", fmt.Errorf("create Git hooks directory: %w", err)
		}
		lfsHooks.root = root
		lfsHooks.current = map[string]string{}
	}
	dir, err := os.MkdirTemp(lfsHooks.root, "lfs-")
	if err != nil {
		return "", fmt.Errorf("create Git LFS hooks directory: %w", err)
	}
	hook := filepath.Join(dir, "pre-push")
	if err := os.WriteFile(hook, []byte(want), 0o700); err != nil {
		return "", fmt.Errorf("write Git LFS pre-push hook: %w", err)
	}
	if err := os.Chmod(hook, 0o700); err != nil {
		return "", fmt.Errorf("mark Git LFS pre-push hook executable: %w", err)
	}
	lfsHooks.current[lfsPath] = dir
	return dir, nil
}

func lfsHookValid(dir, want string) bool {
	hook := filepath.Join(dir, "pre-push")
	info, err := os.Stat(hook)
	if err != nil || info.IsDir() || info.Mode().Perm()&0o100 == 0 {
		return false
	}
	content, err := os.ReadFile(hook)
	return err == nil && string(content) == want
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// RemoveManagedGitHooksDir deletes the Kandev-written hooks directory. Call it
// at process exit.
func RemoveManagedGitHooksDir() {
	lfsHooks.Lock()
	defer lfsHooks.Unlock()
	if lfsHooks.root != "" {
		_ = os.RemoveAll(lfsHooks.root)
		lfsHooks.root, lfsHooks.current = "", nil
	}
}
