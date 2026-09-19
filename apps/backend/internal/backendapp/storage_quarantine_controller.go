package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	storagepkg "github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/gocache"
	"github.com/kandev/kandev/internal/system/storage/tempartifacts"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
)

type workspaceQuarantineController struct {
	settings  *storagepkg.SettingsStore
	store     quarantineEntryStore
	factory   workspaceFactory
	homeDir   string
	activity  *activity.Coordinator
	temporary *tempartifacts.Provider
	rename    func(string, string) error
}

type quarantineEntryStore interface {
	GetQuarantineEntry(context.Context, string) (storagepkg.QuarantineEntry, error)
	ListQuarantineEntries(context.Context, bool) ([]storagepkg.QuarantineEntry, error)
	TransitionQuarantineEntry(
		context.Context, string, storagepkg.QuarantineState, string,
	) (storagepkg.QuarantineEntry, error)
}

func (c *workspaceQuarantineController) Purge(
	ctx context.Context,
	scope storagepkg.QuarantinePurgeScope,
	confirmation string,
) (storagepkg.QuarantinePurgeResult, error) {
	result := storagepkg.QuarantinePurgeResult{Scope: scope}
	force := false
	switch scope {
	case storagepkg.QuarantinePurgeScopeEligible:
		if confirmation != storagepkg.QuarantineConfirmationEligible {
			return result, fmt.Errorf("%w: eligible quarantine purge requires %s confirmation", storagepkg.ErrValidation, storagepkg.QuarantineConfirmationEligible)
		}
	case storagepkg.QuarantinePurgeScopeAll:
		if confirmation != storagepkg.QuarantineConfirmationForce {
			return result, storagepkg.ErrForceDeleteConfirmation
		}
		force = true
	default:
		return result, fmt.Errorf("%w: unknown quarantine purge scope %q", storagepkg.ErrValidation, scope)
	}
	entries, err := c.store.ListQuarantineEntries(ctx, false)
	if err != nil {
		return result, err
	}
	now := time.Now().UTC()
	var purgeErrs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Considered++
		if !force && now.Before(entry.DeleteAfter) {
			result.Protected++
			result.ProtectedBytes += entry.SizeBytes
			continue
		}
		var deleted storagepkg.QuarantineEntry
		var payloadRemoved bool
		if force {
			deleted, payloadRemoved, err = c.permanentDeleteWithPayload(ctx, entry.ID, confirmation, true)
		} else {
			deleted, payloadRemoved, err = c.permanentDeleteWithPayload(ctx, entry.ID, storagepkg.QuarantineConfirmationDelete, false)
		}
		if err != nil {
			result.Failed++
			result.FailedBytes += entry.SizeBytes
			result.Failures = append(result.Failures, storagepkg.QuarantinePurgeFailure{ID: entry.ID, Error: err.Error()})
			purgeErrs = append(purgeErrs, fmt.Errorf("%s: %w", entry.ID, err))
			continue
		}
		result.Deleted++
		if payloadRemoved {
			result.DeletedBytes += deleted.SizeBytes
		}
	}
	return result, errors.Join(purgeErrs...)
}

const (
	goCacheOwnershipManaged = "managed"
	goCacheOwnershipAdopted = "adopted"
)

func (c *workspaceQuarantineController) RestoreTask(
	ctx context.Context,
	taskID string,
) workspaces.WorkspaceRecovery {
	current, err := c.settings.GetSettings(ctx)
	if err != nil {
		return workspaces.WorkspaceRecovery{TaskID: taskID, Status: "failed", Message: err.Error()}
	}
	return c.factory(current).RestoreTask(ctx, taskID)
}

func (c *workspaceQuarantineController) Restore(
	ctx context.Context,
	id string,
) (storagepkg.QuarantineEntry, error) {
	entry, err := c.store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if entry.ResourceType == storagepkg.ResourceTypeGoCache {
		return c.restoreGoCache(ctx, entry)
	}
	if entry.ResourceType == storagepkg.ResourceTypeTemporaryArtifact {
		if c.temporary == nil {
			return storagepkg.QuarantineEntry{}, errors.New("temporary artifact provider is unavailable")
		}
		return c.temporary.Restore(ctx, id)
	}
	if entry.ResourceType != storagepkg.ResourceTypeTaskWorkspace {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: unsupported quarantine resource %q", storagepkg.ErrValidation, entry.ResourceType)
	}
	settings, err := c.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	restored, err := c.factory(settings).Restore(ctx, id)
	if errors.Is(err, workspaces.ErrRestoreConflict) {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("%w: %v", storagepkg.ErrConflict, err)
	}
	return restored, err
}

func (c *workspaceQuarantineController) PermanentDelete(
	ctx context.Context,
	id string,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	return c.permanentDelete(ctx, id, confirmation, false)
}

func (c *workspaceQuarantineController) PermanentDeleteForce(
	ctx context.Context,
	id string,
	confirmation string,
) (storagepkg.QuarantineEntry, error) {
	return c.permanentDelete(ctx, id, confirmation, true)
}

func (c *workspaceQuarantineController) permanentDelete(
	ctx context.Context,
	id string,
	confirmation string,
	force bool,
) (storagepkg.QuarantineEntry, error) {
	deleted, _, err := c.permanentDeleteWithPayload(ctx, id, confirmation, force)
	return deleted, err
}

func (c *workspaceQuarantineController) permanentDeleteWithPayload(
	ctx context.Context,
	id string,
	confirmation string,
	force bool,
) (storagepkg.QuarantineEntry, bool, error) {
	entry, err := c.store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storagepkg.QuarantineEntry{}, false, err
	}
	if entry.ResourceType == storagepkg.ResourceTypeGoCache {
		if force {
			return c.deleteGoCacheWithRetention(ctx, entry, confirmation, true)
		}
		return c.deleteGoCacheWithRetention(ctx, entry, confirmation, false)
	}
	if entry.ResourceType == storagepkg.ResourceTypeTemporaryArtifact {
		if c.temporary == nil {
			return storagepkg.QuarantineEntry{}, false, errors.New("temporary artifact provider is unavailable")
		}
		var deleted storagepkg.QuarantineEntry
		if force {
			deleted, err = c.temporary.PermanentDeleteForce(ctx, id, confirmation)
		} else {
			deleted, err = c.temporary.PermanentDelete(ctx, id, confirmation)
		}
		return deleted, true, err
	}
	if entry.ResourceType != storagepkg.ResourceTypeTaskWorkspace {
		return storagepkg.QuarantineEntry{}, false, fmt.Errorf("%w: unsupported quarantine resource %q", storagepkg.ErrValidation, entry.ResourceType)
	}
	settings, err := c.settings.GetSettings(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, false, err
	}
	var deleted storagepkg.QuarantineEntry
	if force {
		deleted, err = c.factory(settings).PermanentDeleteForce(ctx, id, confirmation)
	} else {
		deleted, err = c.factory(settings).PermanentDelete(ctx, id, confirmation)
	}
	return deleted, true, err
}

func (c *workspaceQuarantineController) restoreGoCache(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
) (storagepkg.QuarantineEntry, error) {
	if err := c.validateGoCacheEntry(ctx, entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	lease, err := c.acquireGoCacheMaintenance(ctx)
	if err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if lease != nil {
		defer lease.Release()
	}
	if err := c.rejectAmbiguousMissingGoCachePayload(entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if err := c.prepareGoCacheRestoreDestination(entry); err != nil {
		return storagepkg.QuarantineEntry{}, err
	}
	if err := c.renamePath(entry.QuarantinePath, entry.OriginalPath); err != nil {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("restore Go cache: %w", err)
	}
	return c.persistGoCacheRestore(ctx, entry)
}

func (c *workspaceQuarantineController) prepareGoCacheRestoreDestination(
	entry storagepkg.QuarantineEntry,
) error {
	if err := c.validateGoCacheRestorePath(entry.OriginalPath); err != nil {
		return err
	}
	if _, err := os.Lstat(entry.OriginalPath); err == nil {
		ownership, ownershipErr := goCacheEntryOwnership(entry)
		if ownershipErr != nil {
			return ownershipErr
		}
		removed, removeErr := gocache.RemoveRestorePlaceholder(
			entry.OriginalPath, ownership == goCacheOwnershipAdopted,
		)
		if removeErr != nil {
			return removeErr
		}
		if !removed {
			return fmt.Errorf("%w: Go-cache restore destination already exists", storagepkg.ErrConflict)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Go-cache restore destination: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(entry.OriginalPath), 0o700); err != nil {
		return fmt.Errorf("create Go-cache restore parent: %w", err)
	}
	return nil
}

func (c *workspaceQuarantineController) persistGoCacheRestore(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
) (storagepkg.QuarantineEntry, error) {
	restored, err := c.store.TransitionQuarantineEntry(
		ctx, entry.ID, storagepkg.QuarantineStateRestored, "",
	)
	if err != nil {
		persistErr := fmt.Errorf("persist Go-cache restore: %w", err)
		if rollbackErr := c.renamePath(entry.OriginalPath, entry.QuarantinePath); rollbackErr != nil {
			return storagepkg.QuarantineEntry{}, errors.Join(
				persistErr, fmt.Errorf("rollback Go-cache restore: %w", rollbackErr),
			)
		}
		return storagepkg.QuarantineEntry{}, persistErr
	}
	return restored, nil
}

func (c *workspaceQuarantineController) renamePath(oldPath, newPath string) error {
	if c.rename != nil {
		return c.rename(oldPath, newPath)
	}
	return os.Rename(oldPath, newPath)
}

func (c *workspaceQuarantineController) acquireGoCacheMaintenance(
	ctx context.Context,
) (*activity.MaintenanceLease, error) {
	if c.activity == nil {
		return nil, nil
	}
	lease, busy, err := c.activity.TryAcquireMaintenance(ctx, 0)
	if errors.Is(err, activity.ErrBusy) {
		return nil, &storagepkg.BusyError{
			Resources:      activity.BusyResourcesForKinds(busy),
			ForceAvailable: false,
		}
	}
	return lease, err
}

func (c *workspaceQuarantineController) deleteGoCacheWithRetention(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
	confirmation string,
	force bool,
) (storagepkg.QuarantineEntry, bool, error) {
	if force {
		if confirmation != storagepkg.QuarantineConfirmationForce {
			return storagepkg.QuarantineEntry{}, false, storagepkg.ErrForceDeleteConfirmation
		}
	} else if confirmation != storagepkg.QuarantineConfirmationDelete {
		return storagepkg.QuarantineEntry{}, false, fmt.Errorf("%w: quarantine deletion requires DELETE confirmation", storagepkg.ErrValidation)
	}
	if err := c.validateGoCacheEntry(ctx, entry); err != nil {
		return storagepkg.QuarantineEntry{}, false, err
	}
	if !force && time.Now().UTC().Before(entry.DeleteAfter) {
		return storagepkg.QuarantineEntry{}, false, fmt.Errorf("%w: quarantine retention deadline has not elapsed", storagepkg.ErrConflict)
	}
	payloadPresent, err := goCacheQuarantinePayloadPresent(entry)
	if err != nil {
		return storagepkg.QuarantineEntry{}, false, err
	}
	if !payloadPresent {
		deleted, err := c.persistGoCacheDeletion(ctx, entry)
		return deleted, false, err
	}
	if err := c.rejectAmbiguousMissingGoCachePayload(entry); err != nil {
		return storagepkg.QuarantineEntry{}, false, err
	}
	if err := os.RemoveAll(entry.QuarantinePath); err != nil {
		return storagepkg.QuarantineEntry{}, false, fmt.Errorf("delete quarantined Go cache: %w", err)
	}
	deleted, err := c.persistGoCacheDeletion(ctx, entry)
	return deleted, true, err
}

func goCacheQuarantinePayloadPresent(entry storagepkg.QuarantineEntry) (bool, error) {
	if _, err := os.Lstat(entry.QuarantinePath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect Go-cache quarantine payload: %w", err)
	}
	return true, nil
}

func (c *workspaceQuarantineController) persistGoCacheDeletion(
	ctx context.Context,
	entry storagepkg.QuarantineEntry,
) (storagepkg.QuarantineEntry, error) {
	deleted, err := c.store.TransitionQuarantineEntry(
		context.WithoutCancel(ctx), entry.ID, storagepkg.QuarantineStateDeleted, "",
	)
	if err != nil {
		return storagepkg.QuarantineEntry{}, fmt.Errorf("persist Go-cache deletion: %w", err)
	}
	return deleted, nil
}

func (c *workspaceQuarantineController) validateGoCacheEntry(
	_ context.Context,
	entry storagepkg.QuarantineEntry,
) error {
	if entry.State != storagepkg.QuarantineStateQuarantined &&
		entry.State != storagepkg.QuarantineStateFailed {
		return fmt.Errorf("%w: Go-cache quarantine entry is %q", storagepkg.ErrConflict, entry.State)
	}
	expectedQuarantine := filepath.Join(c.homeDir, "trash", "go-cache", entry.ID)
	if filepath.Clean(entry.QuarantinePath) != filepath.Clean(expectedQuarantine) {
		return fmt.Errorf("%w: Go-cache quarantine paths do not match managed storage", storagepkg.ErrValidation)
	}
	ownership, err := goCacheEntryOwnership(entry)
	if err != nil {
		return err
	}
	switch ownership {
	case goCacheOwnershipManaged:
		expectedOriginal := filepath.Join(c.homeDir, "cache", "go-build")
		if filepath.Clean(entry.OriginalPath) != filepath.Clean(expectedOriginal) {
			return fmt.Errorf("%w: managed Go-cache original path does not match owned storage", storagepkg.ErrValidation)
		}
	case goCacheOwnershipAdopted:
		original := filepath.Clean(entry.OriginalPath)
		if !filepath.IsAbs(original) || original == filepath.VolumeName(original)+string(filepath.Separator) {
			return fmt.Errorf("%w: adopted Go-cache original path is unsafe", storagepkg.ErrValidation)
		}
	default:
		return fmt.Errorf("%w: unknown Go-cache ownership policy %q", storagepkg.ErrValidation, ownership)
	}
	if err := storagepkg.ValidateNoSymlinkPath(c.homeDir, entry.QuarantinePath); err != nil {
		return fmt.Errorf("%w: validate Go-cache quarantine path: %v", storagepkg.ErrValidation, err)
	}
	return nil
}

func goCacheEntryOwnership(entry storagepkg.QuarantineEntry) (string, error) {
	var metadata struct {
		Ownership string `json:"ownership"`
	}
	if err := json.Unmarshal(entry.Metadata, &metadata); err != nil || metadata.Ownership == "" {
		return "", fmt.Errorf("%w: invalid Go-cache quarantine ownership metadata", storagepkg.ErrValidation)
	}
	return metadata.Ownership, nil
}

func (c *workspaceQuarantineController) rejectAmbiguousMissingGoCachePayload(
	entry storagepkg.QuarantineEntry,
) error {
	if entry.State != storagepkg.QuarantineStateFailed &&
		entry.State != storagepkg.QuarantineStateQuarantined {
		return nil
	}
	if _, err := os.Lstat(entry.OriginalPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect failed Go-cache original path: %w", err)
	}
	if _, err := os.Lstat(entry.QuarantinePath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect failed Go-cache quarantine path: %w", err)
	}
	if err := c.validateGoCacheRestorePath(entry.OriginalPath); err != nil {
		return err
	}
	ownership, err := goCacheEntryOwnership(entry)
	if err != nil {
		return err
	}
	placeholder, err := gocache.IsRestorePlaceholder(
		entry.OriginalPath, ownership == goCacheOwnershipAdopted,
	)
	if err != nil {
		return fmt.Errorf("inspect Go-cache restore placeholder: %w", err)
	}
	if placeholder {
		return fmt.Errorf(
			"%w: quarantined Go-cache payload is missing and the original path is only a rotation placeholder",
			storagepkg.ErrConflict,
		)
	}
	return fmt.Errorf(
		"%w: quarantined Go-cache payload is missing and the populated original cannot be proven restored",
		storagepkg.ErrConflict,
	)
}

func (c *workspaceQuarantineController) validateGoCacheRestorePath(path string) error {
	anchor, err := storagepkg.CommonPath(c.homeDir, path)
	if err != nil {
		return fmt.Errorf("%w: resolve Go-cache restore safety anchor: %v", storagepkg.ErrValidation, err)
	}
	if err := storagepkg.ValidateNoSymlinkPath(anchor, path); err != nil {
		return fmt.Errorf("%w: validate Go-cache restore path: %v", storagepkg.ErrValidation, err)
	}
	return nil
}
