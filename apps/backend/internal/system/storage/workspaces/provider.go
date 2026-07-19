package workspaces

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/system/storage"
)

type Provider struct {
	config Config
}

const (
	workspaceRecoveryStatusNotFound = "not_found"
	workspaceRecoveryStatusFailed   = "failed"
	workspaceRecoveryStatusRestored = "restored"
)

func New(config Config) *Provider {
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.NewID == nil {
		config.NewID = uuid.NewString
	}
	if config.GracePeriod <= 0 {
		config.GracePeriod = 7 * 24 * time.Hour
	}
	if config.Retention <= 0 {
		config.Retention = 7 * 24 * time.Hour
	}
	return &Provider{config: config}
}

type candidate struct {
	path     string
	owner    OwnershipMarker
	size     int64
	measured bool
}

func (p *Provider) Analyze(ctx context.Context) (Analysis, error) {
	_, trashTasks, protected, roots, warnings, err := p.classificationInputs(ctx)
	if err != nil {
		return Analysis{}, err
	}
	analysis := Analysis{Warnings: warnings}
	for index := range roots {
		size, sizeErr := directorySizeNoFollow(roots[index].path)
		if sizeErr != nil {
			analysis.Warnings = append(
				analysis.Warnings,
				fmt.Sprintf("measure workspace %s: %v", roots[index].path, sizeErr),
			)
			continue
		}
		roots[index].size = size
		roots[index].measured = true
		analysis.TotalBytes += size
		if _, active := protected[roots[index].path]; active {
			analysis.ActiveBytes += size
		}
	}
	candidates, err := p.eligibleCandidates(roots, protected, trashTasks)
	if err != nil {
		return Analysis{}, err
	}
	for _, item := range candidates {
		analysis.CandidateBytes += item.size
	}
	return analysis, nil
}

func (p *Provider) Cleanup(ctx context.Context) (CleanupResult, error) {
	candidates, _, warnings, err := p.classify(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{Candidates: len(candidates), Warnings: warnings}
	for _, item := range candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		entry, err := p.quarantine(ctx, item)
		if err != nil {
			return result, err
		}
		result.Quarantined++
		result.ReclaimedBytes += entry.SizeBytes
	}
	return result, nil
}

func (p *Provider) classify(ctx context.Context) ([]candidate, map[string]struct{}, []string, error) {
	_, trashTasks, protected, roots, warnings, err := p.classificationInputs(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	candidates, err := p.eligibleCandidates(roots, protected, trashTasks)
	return candidates, protected, warnings, err
}

func (p *Provider) classificationInputs(
	ctx context.Context,
) (string, string, map[string]struct{}, []candidate, []string, error) {
	tasksRoot, trashTasks, err := p.roots()
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	if p.config.Inventory == nil {
		return "", "", nil, nil, nil, ErrInventoryIncomplete
	}
	inventory, err := p.config.Inventory.LoadWorkspaceInventory(ctx)
	if err != nil {
		return "", "", nil, nil, nil, fmt.Errorf("load authoritative workspace inventory: %w", err)
	}
	if !inventory.Complete {
		return "", "", nil, nil, nil, ErrInventoryIncomplete
	}
	protected, err := buildProtectedSet(tasksRoot, inventory)
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	roots, warnings, err := discoverTaskRoots(tasksRoot)
	if err != nil {
		return "", "", nil, nil, nil, err
	}
	return tasksRoot, trashTasks, protected, roots, warnings, nil
}

func (p *Provider) eligibleCandidates(
	roots []candidate,
	protected map[string]struct{},
	trashTasks string,
) ([]candidate, error) {
	cutoff := p.config.Now().Add(-p.config.GracePeriod)
	candidates := make([]candidate, 0)
	for _, root := range roots {
		if _, keep := protected[root.path]; keep {
			continue
		}
		if pathContains(root.path, trashTasks) || pathContains(trashTasks, root.path) {
			return nil, fmt.Errorf("workspace candidate overlaps trash: %s", root.path)
		}
		info, err := os.Lstat(root.path)
		if err != nil {
			return nil, fmt.Errorf("inspect workspace candidate %s: %w", root.path, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("workspace candidate is not a real directory: %s", root.path)
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if !root.measured {
			size, err := directorySizeNoFollow(root.path)
			if err != nil {
				return nil, fmt.Errorf("measure workspace candidate %s: %w", root.path, err)
			}
			root.size = size
			root.measured = true
		}
		candidates = append(candidates, root)
	}
	return candidates, nil
}

func (p *Provider) quarantine(ctx context.Context, item candidate) (storage.QuarantineEntry, error) {
	if p.config.Store == nil {
		return storage.QuarantineEntry{}, errors.New("workspace quarantine store is required")
	}
	_, trashTasks, err := p.roots()
	if err != nil {
		return storage.QuarantineEntry{}, err
	}
	if err := os.MkdirAll(trashTasks, 0o700); err != nil {
		return storage.QuarantineEntry{}, fmt.Errorf("create workspace trash: %w", err)
	}
	if _, err := storage.ReleaseFailedQuarantineIntent(
		ctx, p.config.Store, storage.ResourceTypeTaskWorkspace, item.path,
	); err != nil {
		return storage.QuarantineEntry{}, err
	}
	now := p.config.Now().UTC()
	id := p.config.NewID()
	metadata, _ := json.Marshal(item.owner)
	entry := storage.QuarantineEntry{
		ID: id, ResourceType: storage.ResourceTypeTaskWorkspace,
		TaskID: item.owner.TaskID, WorkspaceID: item.owner.WorkspaceID,
		OriginalPath: item.path, QuarantinePath: filepath.Join(trashTasks, id),
		SizeBytes: item.size, State: storage.QuarantineStateQuarantined,
		QuarantinedAt: now, DeleteAfter: now.Add(p.config.Retention), Metadata: metadata,
	}
	if err := p.config.Store.CreateQuarantineEntry(ctx, &entry); err != nil {
		return storage.QuarantineEntry{}, fmt.Errorf("persist workspace quarantine intent: %w", err)
	}
	manifest := quarantineManifest{Entry: entry, Owner: item.owner}
	if err := writeJSONFile(filepath.Join(item.path, quarantineManifestName), manifest); err != nil {
		_, _ = p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateFailed, err.Error())
		return storage.QuarantineEntry{}, fmt.Errorf("write workspace quarantine manifest: %w", err)
	}
	if err := os.Rename(item.path, entry.QuarantinePath); err != nil {
		_, _ = p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateFailed, err.Error())
		return storage.QuarantineEntry{}, fmt.Errorf("quarantine workspace: %w", err)
	}
	return entry, nil
}

func (p *Provider) Restore(ctx context.Context, id string) (storage.QuarantineEntry, error) {
	if p.config.Store == nil {
		return storage.QuarantineEntry{}, errors.New("workspace quarantine store is required")
	}
	entry, err := p.config.Store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storage.QuarantineEntry{}, err
	}
	if err := p.validateEntryPaths(entry); err != nil {
		return storage.QuarantineEntry{}, err
	}
	if _, err := os.Lstat(entry.OriginalPath); err == nil {
		return entry, ErrRestoreConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return entry, fmt.Errorf("inspect workspace restore destination: %w", err)
	}
	if err := rejectRootSymlink(entry.QuarantinePath); err != nil {
		return entry, err
	}
	if err := ensureSafeParent(p.cleanTasksRoot(), filepath.Dir(entry.OriginalPath)); err != nil {
		return entry, err
	}
	if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o755); err != nil {
		return entry, fmt.Errorf("create workspace restore parent: %w", err)
	}
	if err := os.Rename(entry.QuarantinePath, entry.OriginalPath); err != nil {
		return entry, fmt.Errorf("restore workspace: %w", err)
	}
	restored, err := p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateRestored, "")
	if err != nil {
		_ = os.Rename(entry.OriginalPath, entry.QuarantinePath)
		return entry, fmt.Errorf("persist workspace restore: %w", err)
	}
	_ = os.Remove(filepath.Join(entry.OriginalPath, quarantineManifestName))
	return restored, nil
}

func (p *Provider) RestoreTask(ctx context.Context, taskID string) WorkspaceRecovery {
	recovery := WorkspaceRecovery{TaskID: taskID, Status: workspaceRecoveryStatusNotFound}
	if p.config.Store == nil {
		recovery.Status = workspaceRecoveryStatusFailed
		recovery.Message = "workspace quarantine store is unavailable"
		return recovery
	}
	entries, err := p.config.Store.ListQuarantineEntries(ctx, false)
	if err != nil {
		recovery.Status = workspaceRecoveryStatusFailed
		recovery.Message = err.Error()
		return recovery
	}
	var newest *storage.QuarantineEntry
	for i := range entries {
		entry := &entries[i]
		if entry.ResourceType != storage.ResourceTypeTaskWorkspace || entry.TaskID != taskID ||
			(entry.State != storage.QuarantineStateQuarantined && entry.State != storage.QuarantineStateFailed) {
			continue
		}
		if newest == nil || entry.QuarantinedAt.After(newest.QuarantinedAt) {
			newest = entry
		}
	}
	if newest == nil {
		return recovery
	}
	if newest.State == storage.QuarantineStateFailed {
		resolved, err := p.resolveFailedTaskRestore(ctx, *newest)
		if err != nil {
			recovery.Status = workspaceRecoveryStatusFailed
			recovery.Message = err.Error()
			return recovery
		}
		if resolved {
			recovery.Status = workspaceRecoveryStatusRestored
			return recovery
		}
	}
	if _, err := p.Restore(ctx, newest.ID); err != nil {
		recovery.Status = workspaceRecoveryStatusFailed
		recovery.Message = err.Error()
		return recovery
	}
	recovery.Status = workspaceRecoveryStatusRestored
	return recovery
}

func (p *Provider) resolveFailedTaskRestore(
	ctx context.Context,
	entry storage.QuarantineEntry,
) (bool, error) {
	if err := p.validateEntryPaths(entry); err != nil {
		return false, err
	}
	originalExists, err := workspacePathExists(entry.OriginalPath)
	if err != nil {
		return false, fmt.Errorf("inspect failed workspace original: %w", err)
	}
	quarantineExists, err := workspacePathExists(entry.QuarantinePath)
	if err != nil {
		return false, fmt.Errorf("inspect failed workspace quarantine: %w", err)
	}
	if originalExists && !quarantineExists {
		_, err := p.config.Store.TransitionQuarantineEntry(
			ctx, entry.ID, storage.QuarantineStateRestored, "",
		)
		return err == nil, err
	}
	if !originalExists && quarantineExists {
		return false, nil
	}
	return false, fmt.Errorf("%w: failed workspace quarantine state is ambiguous", storage.ErrConflict)
}

func workspacePathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (p *Provider) PermanentDelete(
	ctx context.Context,
	id string,
	confirmation string,
) (storage.QuarantineEntry, error) {
	if confirmation != "DELETE" {
		return storage.QuarantineEntry{}, ErrDeleteConfirmation
	}
	if p.config.Store == nil {
		return storage.QuarantineEntry{}, errors.New("workspace quarantine store is required")
	}
	entry, err := p.config.Store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storage.QuarantineEntry{}, err
	}
	if err := p.validateEntryPaths(entry); err != nil {
		return entry, err
	}
	if p.config.Now().Before(entry.DeleteAfter) {
		return entry, fmt.Errorf("%w: quarantine retention deadline has not elapsed", storage.ErrConflict)
	}
	if _, err := directorySizeNoFollow(entry.QuarantinePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return entry, fmt.Errorf("validate quarantined workspace: %w", err)
	}
	if p.config.Pruner != nil && !p.config.Now().Before(entry.DeleteAfter) {
		if err := p.config.Pruner.PruneQuarantinedWorkspace(ctx, entry); err != nil {
			_, _ = p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateFailed, err.Error())
			return entry, fmt.Errorf("prune stale Git worktree registration: %w", err)
		}
	}
	if err := os.RemoveAll(entry.QuarantinePath); err != nil {
		_, _ = p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateFailed, err.Error())
		return entry, fmt.Errorf("delete quarantined workspace: %w", err)
	}
	deleted, err := p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateDeleted, "")
	if err != nil {
		return entry, fmt.Errorf("persist workspace deletion: %w", err)
	}
	return deleted, nil
}

func (p *Provider) Reconcile(ctx context.Context) (ReconcileResult, error) {
	result := ReconcileResult{}
	if p.config.Store == nil {
		return result, errors.New("workspace quarantine store is required")
	}
	_, trashTasks, err := p.roots()
	if err != nil {
		return result, err
	}
	persisted, err := p.config.Store.ListQuarantineEntries(ctx, false)
	if err != nil {
		return result, err
	}
	entries, err := readTrashEntries(trashTasks)
	if err != nil {
		return result, err
	}
	seen := make(map[string]struct{}, len(entries))
	for _, dir := range entries {
		path := filepath.Join(trashTasks, dir.Name())
		seen[path] = struct{}{}
		recovered, failed, warning, err := p.reconcileDiskEntry(ctx, path, dir)
		if err != nil {
			return result, err
		}
		result.Recovered += recovered
		result.Failed += failed
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}
	result.Failed += p.reconcileMissingEntries(ctx, persisted, seen)
	return result, nil
}

func readTrashEntries(trashTasks string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(trashTasks)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

func (p *Provider) reconcileDiskEntry(
	ctx context.Context,
	path string,
	dir os.DirEntry,
) (int, int, string, error) {
	if dir.Type()&os.ModeSymlink != 0 || !dir.IsDir() {
		return 0, 0, "unsafe quarantine path kept: " + path, nil
	}
	manifest, err := readQuarantineManifest(path)
	if err != nil {
		return 0, 0, err.Error(), nil
	}
	if filepath.Clean(manifest.Entry.QuarantinePath) != path {
		return 0, 0, "quarantine manifest path conflict: " + path, nil
	}
	if err := p.validateEntryPaths(manifest.Entry); err != nil {
		return 0, 0, err.Error(), nil
	}
	_, err = p.config.Store.GetQuarantineEntry(ctx, manifest.Entry.ID)
	if errors.Is(err, storage.ErrNotFound) {
		entry := manifest.Entry
		if err := p.config.Store.CreateQuarantineEntry(ctx, &entry); err != nil {
			return 0, 0, "", err
		}
		return 1, 0, "", nil
	}
	if err != nil {
		return 0, 0, "", err
	}
	if _, err := os.Lstat(manifest.Entry.OriginalPath); err == nil {
		_, _ = p.config.Store.TransitionQuarantineEntry(
			ctx,
			manifest.Entry.ID,
			storage.QuarantineStateFailed,
			"original and quarantine paths both exist",
		)
		return 0, 1, "", nil
	}
	return 0, 0, "", nil
}

func (p *Provider) reconcileMissingEntries(
	ctx context.Context,
	persisted []storage.QuarantineEntry,
	seen map[string]struct{},
) int {
	failed := 0
	for _, entry := range persisted {
		if entry.ResourceType != storage.ResourceTypeTaskWorkspace {
			continue
		}
		if _, exists := seen[entry.QuarantinePath]; exists {
			continue
		}
		message := "quarantine path is missing"
		if _, err := os.Lstat(entry.OriginalPath); err == nil {
			message = "quarantine move did not complete"
		}
		_, _ = p.config.Store.TransitionQuarantineEntry(ctx, entry.ID, storage.QuarantineStateFailed, message)
		failed++
	}
	return failed
}

func (p *Provider) roots() (string, string, error) {
	tasksRoot := p.cleanTasksRoot()
	trashRoot := filepath.Clean(p.config.TrashRoot)
	if !filepath.IsAbs(tasksRoot) || !filepath.IsAbs(trashRoot) {
		return "", "", errors.New("workspace tasks and trash roots must be absolute")
	}
	if pathContains(tasksRoot, trashRoot) || pathContains(trashRoot, tasksRoot) {
		return "", "", errors.New("workspace tasks and trash roots must not overlap")
	}
	trashTasks := filepath.Join(trashRoot, "tasks")
	anchor, err := storage.CommonPath(tasksRoot, trashRoot)
	if err != nil {
		return "", "", err
	}
	if err := storage.ValidateNoSymlinkPath(anchor, tasksRoot); err != nil {
		return "", "", fmt.Errorf("validate workspace tasks root: %w", err)
	}
	if err := storage.ValidateNoSymlinkPath(anchor, trashRoot); err != nil {
		return "", "", fmt.Errorf("validate workspace trash root: %w", err)
	}
	if err := storage.ValidateNoSymlinkPath(anchor, trashTasks); err != nil {
		return "", "", fmt.Errorf("validate workspace task trash: %w", err)
	}
	if err := rejectRootSymlink(tasksRoot); err != nil {
		return "", "", err
	}
	return tasksRoot, trashTasks, nil
}

func (p *Provider) cleanTasksRoot() string { return filepath.Clean(p.config.TasksRoot) }

func (p *Provider) validateEntryPaths(entry storage.QuarantineEntry) error {
	tasksRoot, trashTasks, err := p.roots()
	if err != nil {
		return err
	}
	original := filepath.Clean(entry.OriginalPath)
	quarantine := filepath.Clean(entry.QuarantinePath)
	if original != entry.OriginalPath || quarantine != entry.QuarantinePath ||
		!pathContains(tasksRoot, original) || original == tasksRoot ||
		!pathContains(trashTasks, quarantine) || quarantine == trashTasks {
		return errors.New("quarantine entry paths are outside owned roots")
	}
	anchor, err := storage.CommonPath(tasksRoot, trashTasks)
	if err != nil {
		return err
	}
	if err := storage.ValidateNoSymlinkPath(anchor, original); err != nil {
		return fmt.Errorf("validate original workspace path: %w", err)
	}
	if err := storage.ValidateNoSymlinkPath(anchor, quarantine); err != nil {
		return fmt.Errorf("validate quarantined workspace path: %w", err)
	}
	return nil
}

func buildProtectedSet(tasksRoot string, inventory Inventory) (map[string]struct{}, error) {
	paths := make([]string, 0, len(inventory.WorktreePaths)+len(inventory.EnvironmentPaths)+len(inventory.ExecutionPaths)+len(inventory.ScratchRoots))
	paths = append(paths, inventory.WorktreePaths...)
	paths = append(paths, inventory.EnvironmentPaths...)
	paths = append(paths, inventory.ExecutionPaths...)
	for _, scratch := range inventory.ScratchRoots {
		if scratch.Path == "" || scratch.TaskID == "" || scratch.WorkspaceID == "" {
			return nil, errors.New("active scratch inventory is incomplete")
		}
		paths = append(paths, scratch.Path)
	}
	protected := make(map[string]struct{})
	for _, raw := range paths {
		if raw == "" || !filepath.IsAbs(raw) {
			return nil, fmt.Errorf("inventory path is not absolute: %q", raw)
		}
		path := filepath.Clean(raw)
		if !pathContains(tasksRoot, path) || path == tasksRoot {
			continue
		}
		for current := path; current != tasksRoot; current = filepath.Dir(current) {
			protected[current] = struct{}{}
			if current == filepath.Dir(current) {
				return nil, fmt.Errorf("inventory path escaped tasks root: %s", path)
			}
		}
	}
	return protected, nil
}

func discoverTaskRoots(tasksRoot string) ([]candidate, []string, error) {
	entries, err := os.ReadDir(tasksRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	roots := make([]candidate, 0)
	warnings := make([]string, 0)
	for _, entry := range entries {
		path := filepath.Join(tasksRoot, entry.Name())
		discovered, classified, err := discoverTaskRoot(path, entry)
		if err != nil {
			return nil, nil, err
		}
		roots = append(roots, discovered...)
		if entry.IsDir() && !classified {
			warnings = append(warnings, "unclassified task directory kept: "+path)
		}
	}
	return roots, warnings, nil
}

func discoverTaskRoot(path string, entry os.DirEntry) ([]candidate, bool, error) {
	if entry.Type()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("symlink beneath tasks root: %s", path)
	}
	if !entry.IsDir() {
		return nil, false, nil
	}
	owner, marked, err := readOwnershipMarker(path)
	if err != nil {
		return nil, false, err
	}
	if marked {
		return []candidate{{path: path, owner: owner}}, true, nil
	}
	if looksSemanticTaskDir(entry.Name()) {
		owner = OwnershipMarker{TaskDirName: entry.Name(), LayoutVersion: LayoutVersionSemantic}
		return []candidate{{path: path, owner: owner}}, true, nil
	}
	return discoverScratchRoots(path, entry.Name())
}

func discoverScratchRoots(workspacePath, workspaceID string) ([]candidate, bool, error) {
	children, err := os.ReadDir(workspacePath)
	if err != nil {
		return nil, false, err
	}
	roots := make([]candidate, 0, len(children))
	for _, child := range children {
		childPath := filepath.Join(workspacePath, child.Name())
		if child.Type()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("symlink beneath tasks root: %s", childPath)
		}
		if !child.IsDir() {
			continue
		}
		owner, marked, err := readOwnershipMarker(childPath)
		if err != nil {
			return nil, false, err
		}
		if marked && owner.LayoutVersion != LayoutVersionScratch {
			return nil, false, fmt.Errorf("unexpected nested semantic workspace marker: %s", childPath)
		}
		if !marked {
			owner = OwnershipMarker{
				TaskID: child.Name(), WorkspaceID: workspaceID,
				TaskDirName: child.Name(), LayoutVersion: LayoutVersionScratch,
			}
		}
		roots = append(roots, candidate{path: childPath, owner: owner})
	}
	return roots, len(roots) > 0, nil
}

func looksSemanticTaskDir(name string) bool {
	index := strings.LastIndexByte(name, '_')
	return index > 0 && len(name[index+1:]) == 3
}

// directorySizeNoFollow counts files in root while treating nested symlinks as opaque entries.
func directorySizeNoFollow(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
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

func rejectRootSymlink(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("owned root is not a real directory: %s", root)
	}
	return nil
}

func ensureSafeParent(root, parent string) error {
	if !pathContains(root, parent) {
		return fmt.Errorf("path escapes owned root: %s", parent)
	}
	rel, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe restore parent: %s", current)
		}
	}
	return nil
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeJSONFile(path string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe JSON control file: %s", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect JSON control file %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".kandev-control-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(encoded); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("install JSON control file %s: %w", path, err)
	}
	return nil
}

func readQuarantineManifest(root string) (quarantineManifest, error) {
	path := filepath.Join(root, quarantineManifestName)
	info, err := os.Lstat(path)
	if err != nil {
		return quarantineManifest{}, fmt.Errorf("inspect quarantine manifest %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return quarantineManifest{}, fmt.Errorf("unsafe quarantine manifest: %s", path)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return quarantineManifest{}, err
	}
	var manifest quarantineManifest
	if err := json.Unmarshal(encoded, &manifest); err != nil {
		return quarantineManifest{}, fmt.Errorf("decode quarantine manifest: %w", err)
	}
	return manifest, nil
}
