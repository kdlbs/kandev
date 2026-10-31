package launchwrapper

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrAllocDir reports an allocation directory that is missing, unsupplied or
// cannot be trusted. Messages name the stage and never the path.
var ErrAllocDir = errors.New("node launch wrapper allocation directory is not usable")

// allocDirOwnerUID names the user that must own the allocation directory.
// Production never changes it. Tests replace it to exercise the refusal.
var allocDirOwnerUID = os.Geteuid

func allocDirError(stage string) error {
	return fmt.Errorf("%w: %s", ErrAllocDir, stage)
}

// UsesAllocDir reports whether a prefix references the allocation directory.
func UsesAllocDir(argv []string) bool {
	for i := 1; i < len(argv); i++ {
		if argv[i] == AllocDir {
			return true
		}
	}
	return false
}

// resolveAllocDir turns the operator-supplied allocation directory into the
// value a launch uses. It refuses a directory inside the workspace, because
// the point of the reference is state the agent cannot rewrite.
func resolveAllocDir(path, workDir string) (string, error) {
	resolved, err := ResolveAllocDir(path)
	if err != nil {
		return "", err
	}
	work, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		return "", ErrContext
	}
	if rel, err := filepath.Rel(work, resolved); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", allocDirError("inside the workspace")
	}
	return resolved, nil
}
