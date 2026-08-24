package process

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	tools "github.com/kandev/kandev/internal/tools/installer"
)

// RepairManagedRuntimeCache resolves npm's cache in the instance environment
// and removes only the execution tree for one exact managed package spec.
func (m *Manager) RepairManagedRuntimeCache(ctx context.Context, packageSpec string) error {
	if err := managedruntime.ValidateExactPackageSpec(packageSpec); err != nil {
		return err
	}
	env, err := m.CommandEnvironment()
	if err != nil {
		return errors.New("resolve agent environment for managed runtime repair")
	}
	output, err := m.CombinedOutput(ctx, tools.CommandSpec{
		Path: "npm",
		Args: []string{"config", "get", "cache"},
		Dir:  m.cfg.WorkDir,
		Env:  env,
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("resolve npm cache for managed runtime repair")
	}
	cacheRoot, err := npmCacheRootFromOutput(output)
	if err != nil {
		return err
	}
	if err := managedruntime.RemoveNpxExecutionTree(cacheRoot, packageSpec); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("remove managed runtime npm execution tree")
	}
	return nil
}

func npmCacheRootFromOutput(output []byte) (string, error) {
	lines := strings.Split(string(output), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		candidate := strings.TrimSpace(lines[index])
		if candidate != "" && filepath.IsAbs(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("npm cache path was not returned")
}
