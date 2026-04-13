package worktree

import (
	"context"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// getSubmodulePaths returns the paths of all submodules registered in HEAD.
// It reads from the git object store (git ls-tree), so it works in --no-checkout worktrees.
// Returns nil (not an error) if there are no submodules.
func getSubmodulePaths(ctx context.Context, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "HEAD")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		// Format: "<mode> <type> <hash>\t<path>"
		// Submodules have mode 160000 and type "commit".
		if strings.HasPrefix(line, "160000 ") {
			if _, path, ok := strings.Cut(line, "\t"); ok {
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

// initSubmodules runs "git submodule update --init" in the given directory.
// Failures are non-fatal: submodule URLs may be unreachable (private repos,
// missing credentials), but the worktree is still usable for non-submodule files.
func (m *Manager) initSubmodules(ctx context.Context, dir string) {
	cmd := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Warn("git submodule update --init failed (non-fatal)",
			zap.String("dir", dir),
			zap.String("output", string(output)),
			zap.Error(err))
		return
	}
	m.logger.Debug("initialized submodules in worktree", zap.String("dir", dir))
}
