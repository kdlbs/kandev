package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const quarantineRetryReleaseReason = "quarantine move did not occur; intent released for retry"

type quarantineRetryStore interface {
	ListQuarantineEntries(context.Context, bool) ([]QuarantineEntry, error)
	TransitionQuarantineEntry(
		context.Context,
		string,
		QuarantineState,
		string,
	) (QuarantineEntry, error)
}

// ReleaseFailedQuarantineIntent releases a failed intent only when filesystem
// state proves its move did not occur. Ambiguous states remain active for
// recovery or reconciliation.
func ReleaseFailedQuarantineIntent(
	ctx context.Context,
	store quarantineRetryStore,
	resourceType ResourceType,
	originalPath string,
) (bool, error) {
	entries, err := store.ListQuarantineEntries(ctx, false)
	if err != nil {
		return false, fmt.Errorf("list active quarantine intents: %w", err)
	}
	originalPath = filepath.Clean(originalPath)
	for _, entry := range entries {
		if entry.ResourceType != resourceType || filepath.Clean(entry.OriginalPath) != originalPath {
			continue
		}
		if entry.State == QuarantineStateQuarantined {
			return false, fmt.Errorf("%w: quarantine intent for %s is still active", ErrConflict, originalPath)
		}
		if entry.State == QuarantineStateFailed {
			return releaseFailedIntentIfUnmoved(ctx, store, entry)
		}
	}
	return false, nil
}

func releaseFailedIntentIfUnmoved(
	ctx context.Context,
	store quarantineRetryStore,
	entry QuarantineEntry,
) (bool, error) {
	originalExists, err := quarantinePathExists(entry.OriginalPath)
	if err != nil {
		return false, fmt.Errorf("inspect failed quarantine original: %w", err)
	}
	quarantineExists, err := quarantinePathExists(entry.QuarantinePath)
	if err != nil {
		return false, fmt.Errorf("inspect failed quarantine destination: %w", err)
	}
	if !originalExists || quarantineExists {
		return false, fmt.Errorf("%w: failed quarantine intent has ambiguous filesystem state", ErrConflict)
	}
	if _, err := store.TransitionQuarantineEntry(
		ctx,
		entry.ID,
		QuarantineStateRestored,
		quarantineRetryReleaseReason,
	); err != nil {
		return false, fmt.Errorf("release failed quarantine intent: %w", err)
	}
	return true, nil
}

func quarantinePathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
