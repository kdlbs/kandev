package launchwrapper

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/common/managednpm"
)

// Context contains the final execution-host process context.
type Context struct {
	WorkDir string
	Env     []string
}

// Prepare resolves a wrapper and returns a new complete argv. It never starts a process.
func Prepare(spec Spec, inner []string, ctx Context) ([]string, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	if len(inner) == 0 || strings.TrimSpace(inner[0]) == "" {
		return nil, ErrInvalid
	}
	for _, arg := range inner {
		if strings.ContainsRune(arg, 0) {
			return nil, ErrInvalid
		}
	}
	if !filepath.IsAbs(ctx.WorkDir) {
		return nil, ErrContext
	}
	info, err := os.Stat(ctx.WorkDir)
	if err != nil || !info.IsDir() {
		return nil, ErrContext
	}
	wrapper, err := resolveExecutable(spec.Argv[0], ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	child := slices.Clone(inner)
	child[0], err = resolveExecutable(child[0], ctx)
	if err != nil {
		return nil, ErrContext
	}
	if err := managednpm.PrepareNPMProjectPrefix(child); err != nil {
		return nil, ErrContext
	}
	prefix, err := resolveReferences(spec.Argv, ctx.WorkDir)
	if err != nil {
		return nil, err
	}
	prefix[0] = wrapper
	return append(prefix, child...), nil
}

func resolveReferences(tokens []string, workDir string) ([]string, error) {
	prefix := slices.Clone(tokens)
	npmDir := ""
	for i := 1; i < len(prefix); i++ {
		switch prefix[i] {
		case WorkDir:
			prefix[i] = workDir
		case ManagedNPMDir:
			if npmDir == "" {
				var err error
				npmDir, err = managednpm.PrepareNPMProjectDirectory()
				if err != nil {
					return nil, ErrContext
				}
			}
			prefix[i] = npmDir
		default:
			if escaped := escapedReference(prefix[i]); escaped != "" {
				prefix[i] = escaped
			}
		}
	}
	return prefix, nil
}
