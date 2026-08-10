package workspaces

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/kandev/kandev/internal/system/storage"
)

var dependencyDirectoryAllowlist = []string{
	"node_modules",
	"bower_components",
	".pnpm-store",
	".yarn/cache",
	".yarn/unplugged",
	".venv",
	"venv",
	".tox",
	".nox",
	"__pypackages__",
	"Pods",
	".gradle",
}

var dependencyDirectoryNames = map[string]struct{}{
	"node_modules":     {},
	"bower_components": {},
	".pnpm-store":      {},
	".venv":            {},
	"venv":             {},
	".tox":             {},
	".nox":             {},
	"__pypackages__":   {},
	"Pods":             {},
	".gradle":          {},
}

var dependencyTraversalExclusions = map[string]struct{}{
	"vendor": {},
	"target": {},
	"build":  {},
	"dist":   {},
	".cache": {},
	".git":   {},
}

// DependencyDirectoryAllowlist returns the exact directory patterns that dependency cleanup
// may remove. The returned slice is independent of the provider's internal list.
func DependencyDirectoryAllowlist() []string {
	return append([]string(nil), dependencyDirectoryAllowlist...)
}

func (p *Provider) CleanupDependencies(ctx context.Context) (DependencyCleanupResult, error) {
	result := DependencyCleanupResult{}
	tasksRoot, trashTasks, err := p.roots()
	if err != nil {
		return result, err
	}
	inventory, err := p.loadInventory(ctx)
	if err != nil {
		return result, err
	}
	if inventory.KnownTaskIDs == nil || inventory.ArchivedTaskIDs == nil {
		return result, ErrInventoryIncomplete
	}
	protected, err := buildProtectedSet(tasksRoot, inventory)
	if err != nil {
		return result, err
	}
	roots, warnings, err := discoverTaskRoots(tasksRoot)
	if err != nil {
		return result, err
	}
	result.Warnings = append(result.Warnings, warnings...)
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if _, keep := protected[root.path]; keep || !dependencyEligible(root.owner, inventory) {
			continue
		}
		marked, err := dependencyWorkspaceMarker(root.path, root.owner)
		if err != nil {
			result.Warnings = append(result.Warnings, err.Error())
			continue
		}
		if !marked {
			continue
		}
		workspaceResult, err := p.cleanupDependencyWorkspace(ctx, tasksRoot, trashTasks, root)
		if err != nil {
			return result, err
		}
		result.Directories += workspaceResult.Directories
		result.ReclaimedBytes += workspaceResult.ReclaimedBytes
		result.Workspaces += workspaceResult.Workspaces
		result.Warnings = append(result.Warnings, workspaceResult.Warnings...)
	}
	return result, nil
}

func (p *Provider) cleanupDependencyWorkspace(
	ctx context.Context,
	tasksRoot, trashTasks string,
	root candidate,
) (DependencyCleanupResult, error) {
	result := DependencyCleanupResult{}
	targets, err := findDependencyTargets(ctx, root.path)
	if err != nil {
		if isDependencyCleanupContextError(err) {
			return result, err
		}
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("inspect workspace dependencies %s: %v", root.path, err))
		return result, nil
	}
	inventory, err := p.loadInventory(ctx)
	if err != nil {
		return result, err
	}
	protected, err := buildProtectedSet(tasksRoot, inventory)
	if err != nil {
		return result, err
	}
	for _, target := range targets {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		size, warning, err := p.cleanupDependencyTarget(
			ctx, tasksRoot, trashTasks, root, target, inventory, protected,
		)
		if err != nil {
			if isDependencyCleanupContextError(err) {
				return result, err
			}
			result.Warnings = append(result.Warnings, err.Error())
			continue
		}
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
			continue
		}
		result.Directories++
		result.ReclaimedBytes += size
	}
	if result.Directories > 0 {
		result.Workspaces = 1
	}
	return result, nil
}

func (p *Provider) cleanupDependencyTarget(
	ctx context.Context,
	tasksRoot, trashTasks string,
	root candidate,
	target string,
	inventory Inventory,
	protected map[string]struct{},
) (int64, string, error) {
	valid, warning, err := revalidateDependencyTarget(
		ctx, tasksRoot, trashTasks, root, target, inventory, protected,
	)
	if err != nil || !valid {
		return 0, warning, err
	}
	size, err := dependencyDirectorySize(ctx, target)
	if err != nil {
		if isDependencyCleanupContextError(err) {
			return 0, "", err
		}
		return 0, "", fmt.Errorf("measure dependency directory %s: %w", target, err)
	}
	if err := removeDependencyDirectory(ctx, root.path, target); err != nil {
		return 0, "", fmt.Errorf("remove dependency directory %s: %w", target, err)
	}
	return size, "", nil
}

func isDependencyCleanupContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (p *Provider) loadInventory(ctx context.Context) (Inventory, error) {
	if p.config.Inventory == nil {
		return Inventory{}, ErrInventoryIncomplete
	}
	inventory, err := p.config.Inventory.LoadWorkspaceInventory(ctx)
	if err != nil {
		return Inventory{}, fmt.Errorf("load authoritative workspace inventory: %w", err)
	}
	if !inventory.Complete {
		return Inventory{}, ErrInventoryIncomplete
	}
	return inventory, nil
}

func dependencyEligible(owner OwnershipMarker, inventory Inventory) bool {
	if owner.TaskID == "" {
		return false
	}
	if _, archived := inventory.ArchivedTaskIDs[owner.TaskID]; archived {
		return true
	}
	_, known := inventory.KnownTaskIDs[owner.TaskID]
	return !known
}

func dependencyWorkspaceMarker(path string, expected OwnershipMarker) (bool, error) {
	owner, marked, err := readOwnershipMarker(path)
	if err != nil {
		return false, fmt.Errorf("inspect workspace ownership marker %s: %w", path, err)
	}
	if !marked {
		return false, nil
	}
	if owner.TaskID != expected.TaskID || owner.TaskDirName != expected.TaskDirName ||
		owner.LayoutVersion != expected.LayoutVersion {
		return false, fmt.Errorf("workspace ownership changed while preparing dependency cleanup: %s", path)
	}
	return true, nil
}

func findDependencyTargets(ctx context.Context, root string) ([]string, error) {
	targets := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if path != root && entry.IsDir() {
			if _, excluded := dependencyTraversalExclusions[entry.Name()]; excluded {
				return filepath.SkipDir
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if isDependencyTarget(relative, entry.Name()) {
				targets = append(targets, path)
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(targets)
	return targets, nil
}

func isDependencyTarget(relative, name string) bool {
	if _, allowed := dependencyDirectoryNames[name]; allowed {
		return true
	}
	parent := filepath.Base(filepath.Dir(relative))
	return parent == ".yarn" && (name == "cache" || name == "unplugged")
}

func revalidateDependencyTarget(
	ctx context.Context,
	tasksRoot, trashTasks string,
	root candidate,
	target string,
	inventory Inventory,
	protected map[string]struct{},
) (bool, string, error) {
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	valid, warning, err := validateDependencyTargetPath(tasksRoot, trashTasks, root, target)
	if err != nil || !valid {
		return valid, warning, err
	}
	valid, warning, err = validateDependencyDirectories(root.path, target)
	if err != nil || !valid {
		return valid, warning, err
	}
	owner, err := validateDependencyWorkspaceMarker(root)
	if err != nil {
		return false, "", err
	}
	if owner == nil {
		return false, fmt.Sprintf("skip workspace whose ownership changed: %s", root.path), nil
	}
	if _, isProtected := protected[root.path]; isProtected || !dependencyEligible(*owner, inventory) {
		return false, fmt.Sprintf("skip protected or active workspace: %s", root.path), nil
	}
	return true, "", nil
}

func validateDependencyTargetPath(
	tasksRoot, trashTasks string,
	root candidate,
	target string,
) (bool, string, error) {
	if !pathContains(root.path, target) || target == root.path {
		return false, fmt.Sprintf("skip dependency path outside workspace: %s", target), nil
	}
	anchor, err := storage.CommonPath(tasksRoot, trashTasks)
	if err != nil {
		return false, "", err
	}
	if err := storage.ValidateNoSymlinkPath(anchor, root.path); err != nil {
		return false, fmt.Sprintf("skip workspace with unsafe path %s: %v", root.path, err), nil
	}
	if err := storage.ValidateNoSymlinkPath(anchor, target); err != nil {
		return false, fmt.Sprintf("skip dependency path with unsafe path %s: %v", target, err), nil
	}
	return true, "", nil
}

func validateDependencyDirectories(root, target string) (bool, string, error) {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return false, "", err
	}
	targetInfo, err := os.Lstat(target)
	if err != nil {
		return false, "", err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() ||
		targetInfo.Mode()&os.ModeSymlink != 0 || !targetInfo.IsDir() {
		return false, fmt.Sprintf("skip dependency path that is not a real directory: %s", target), nil
	}
	return true, "", nil
}

func validateDependencyWorkspaceMarker(root candidate) (*OwnershipMarker, error) {
	owner, marked, err := readOwnershipMarker(root.path)
	if err != nil {
		return nil, err
	}
	if !marked || owner.TaskID != root.owner.TaskID || owner.TaskDirName != root.owner.TaskDirName ||
		owner.LayoutVersion != root.owner.LayoutVersion {
		return nil, nil
	}
	return &owner, nil
}

func dependencyDirectorySize(ctx context.Context, root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}
