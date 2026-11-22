package passthroughconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func PathEscapesWorkspace(workspaceDir, path string) (bool, error) {
	if workspaceDir == "" {
		return false, nil
	}
	cleanWS := filepath.Clean(workspaceDir)
	cleanPath := filepath.Clean(path)
	if rel, err := filepath.Rel(cleanWS, cleanPath); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil // not a workspace-relative file; not our concern
	}
	canonWS, err := filepath.EvalSymlinks(cleanWS)
	if err != nil {
		return false, fmt.Errorf("resolve workspace dir: %w", err)
	}
	// Resolve the deepest existing ancestor of the target (the leaf and some
	// parents may not exist yet — those get created as real dirs).
	ancestor := filepath.Dir(cleanPath)
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			rel, relErr := filepath.Rel(canonWS, resolved)
			escaped := relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
			return escaped, nil
		}
		if !os.IsNotExist(err) {
			return false, fmt.Errorf("resolve ancestor %q: %w", ancestor, err)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return false, nil // reached filesystem root with nothing existing
		}
		ancestor = parent
	}
}
