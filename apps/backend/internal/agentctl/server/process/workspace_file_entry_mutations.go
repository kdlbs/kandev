package process

import (
	"fmt"
	"path/filepath"
)

// resolveEntryMutationPath resolves parent routing while retaining the selected
// leaf name under the workspace or registered source's authority-root handle.
func (wt *WorkspaceTracker) resolveEntryMutationPath(reqPath string) (*rootedMutationPath, error) {
	entryPath := filepath.Clean(reqPath)
	if !filepath.IsAbs(entryPath) {
		entryPath = filepath.Join(wt.resolvedWorkDir(), entryPath)
	}
	if entryPath == wt.resolvedWorkDir() {
		return nil, fmt.Errorf("cannot mutate workspace root")
	}

	parent, err := wt.resolveMutationPath(filepath.Dir(entryPath))
	if err != nil {
		return nil, err
	}
	parent.rel = filepath.Join(parent.rel, filepath.Base(entryPath))
	parent.safe = filepath.Join(parent.rootPath, parent.rel)
	return parent, nil
}

// resolveEntryMutationSource retains full-target admission and root protection
// before selecting the link entry. Leaf identity does not grant another root.
func (wt *WorkspaceTracker) resolveEntryMutationSource(reqPath string) (*rootedMutationPath, error) {
	target, err := wt.resolveMutationPath(reqPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = target.root.Close() }()
	if target.rel == "." {
		return nil, fmt.Errorf("cannot mutate workspace or registered source root")
	}
	if err := validateSourceExistsRooted(target.root, target.rel, reqPath); err != nil {
		return nil, err
	}

	entry, err := wt.resolveEntryMutationPath(reqPath)
	if err != nil {
		return nil, err
	}
	if target.rootPath != entry.rootPath {
		_ = entry.root.Close()
		return nil, fmt.Errorf("cannot mutate leaf across workspace roots")
	}
	return entry, nil
}
