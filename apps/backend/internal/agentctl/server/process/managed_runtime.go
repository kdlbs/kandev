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
	output, err := m.Output(ctx, tools.CommandSpec{
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
	m.ClearStderrBuffer()
	return nil
}

func npmCacheRootFromOutput(output []byte) (string, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 1 {
		return "", errors.New("npm cache path was not returned")
	}
	candidate := strings.TrimSpace(lines[0])
	if candidate == "" || !filepath.IsAbs(candidate) {
		return "", errors.New("npm cache path was not returned")
	}
	return candidate, nil
}
