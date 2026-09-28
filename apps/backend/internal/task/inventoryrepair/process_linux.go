//go:build linux

package inventoryrepair

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func processRoots(p Plan) []string {
	var roots []string
	for _, r := range p.Repairs {
		roots = append(roots, filepath.Dir(r.SourcePath), filepath.Dir(r.Path), p.ExpectedGit[r.WorktreeID].GitDir)
	}
	return roots
}

func affectedProcessPath(path string, roots []string) bool {
	path = strings.TrimSuffix(path, " (deleted)")
	for _, root := range roots {
		if root != "" && (path == root || strings.HasPrefix(path, root+"/")) {
			return true
		}
	}
	return false
}

// Host worktree agents run as the backend user. Inspect every such process,
// including inherited shells and processes absent from runtime inventory.
func checkProcesses(ctx context.Context, p Plan) error {
	owner, err := os.Stat(p.Home)
	if err != nil {
		return err
	}
	stat, ok := owner.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot determine installation owner")
	}
	return inspectHostProcesses(ctx, p, stat.Uid)
}

func inspectHostProcesses(ctx context.Context, p Plan, uid uint32) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	roots := processRoots(p)
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return err
		}
		pid, parseErr := strconv.Atoi(entry.Name())
		if parseErr != nil || pid == os.Getpid() {
			continue
		}
		path := filepath.Join("/proc", entry.Name())
		info, statErr := os.Stat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.New("cannot determine host process owner")
		}
		if stat.Uid != uid {
			continue
		}
		if err = inspectProcess(path, roots, p); err != nil {
			if _, statErr = os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("host process %d prevents repair: %w", pid, err)
		}
	}
	return nil
}

func inspectProcess(path string, roots []string, p Plan) error {
	status, err := os.ReadFile(filepath.Join(path, "status"))
	if err != nil {
		return err
	}
	if strings.Contains(string(status), "State:\tZ") {
		return nil
	}
	cwd, err := os.Readlink(filepath.Join(path, "cwd"))
	if err != nil {
		return err
	}
	if affectedProcessPath(cwd, roots) {
		return errors.New("working directory is inside a selected task root")
	}
	args, err := os.ReadFile(filepath.Join(path, "cmdline"))
	if err != nil {
		return err
	}
	for _, r := range p.Repairs {
		for _, needle := range []string{r.SourcePath, r.Path, r.EnvironmentID, r.WorktreeID} {
			if len(needle) >= 16 && strings.Contains(string(args), needle) {
				return errors.New("command references selected inventory")
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(path, "fd"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(path, "fd", entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if affectedProcessPath(target, roots) {
			return errors.New("open file is inside a selected task root")
		}
	}
	return nil
}
