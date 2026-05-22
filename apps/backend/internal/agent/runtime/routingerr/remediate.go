package routingerr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// RemediateNpxCache wipes a single npx package cache directory (the
// `_npx/<hash>` subtree) so the next `npx -y <pkg>` invocation extracts
// cleanly. It is a no-op-safe operation: if the directory is already
// gone, the call succeeds.
//
// Safety: the path MUST resolve (with symlinks evaluated) to a child of
// `$HOME/.npm/_npx/`. Any other path is rejected without modification,
// so a malformed log line can never trick the caller into removing an
// unrelated tree.
func RemediateNpxCache(path string, log *zap.Logger) error {
	if path == "" {
		return fmt.Errorf("RemediateNpxCache: empty path")
	}
	if log == nil {
		log = zap.NewNop()
	}
	resolved, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("RemediateNpxCache: resolve %q: %w", path, err)
	}
	evaluated, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("npx cache already absent", zap.String("path", resolved))
			return nil
		}
		return fmt.Errorf("RemediateNpxCache: eval symlinks %q: %w", resolved, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("RemediateNpxCache: user home: %w", err)
	}
	expectedRoot := filepath.Join(home, ".npm", "_npx")
	evaluatedRoot, err := filepath.EvalSymlinks(expectedRoot)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("RemediateNpxCache: eval symlinks %q: %w", expectedRoot, err)
		}
		evaluatedRoot = expectedRoot
	}
	rootWithSep := evaluatedRoot + string(os.PathSeparator)
	if !strings.HasPrefix(evaluated+string(os.PathSeparator), rootWithSep) {
		return fmt.Errorf("RemediateNpxCache: refusing path %q outside %q", evaluated, rootWithSep)
	}
	if evaluated == evaluatedRoot {
		return fmt.Errorf("RemediateNpxCache: refusing to delete _npx root %q", evaluated)
	}
	if err := os.RemoveAll(evaluated); err != nil {
		return fmt.Errorf("RemediateNpxCache: remove %q: %w", evaluated, err)
	}
	log.Info("removed corrupted npx cache directory", zap.String("path", evaluated))
	return nil
}
