package tempartifacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/system/storage"
)

const staleArtifactAge = 24 * time.Hour

type QuarantineStore interface {
	CreateQuarantineEntry(context.Context, *storage.QuarantineEntry) error
	GetQuarantineEntry(context.Context, string) (storage.QuarantineEntry, error)
	ListQuarantineEntries(context.Context, bool) ([]storage.QuarantineEntry, error)
	TransitionQuarantineEntry(context.Context, string, storage.QuarantineState, string) (storage.QuarantineEntry, error)
}

type ProviderConfig struct {
	Registry  *Registry
	Store     QuarantineStore
	HomeDir   string
	TrashDir  string
	Retention time.Duration
	Now       func() time.Time
	NewID     func() string
}

type Provider struct {
	registry  *Registry
	store     QuarantineStore
	homeDir   string
	trashDir  string
	retention time.Duration
	now       func() time.Time
	newID     func() string
}

type Analysis struct {
	Available      bool     `json:"available"`
	TotalCount     int      `json:"total_count"`
	TotalBytes     int64    `json:"total_bytes"`
	ActiveCount    int      `json:"active_count"`
	ActiveBytes    int64    `json:"active_bytes"`
	ProtectedCount int      `json:"protected_count"`
	ProtectedBytes int64    `json:"protected_bytes"`
	StaleCount     int      `json:"stale_count"`
	StaleBytes     int64    `json:"stale_bytes"`
	SkippedCount   int      `json:"skipped_count"`
	Warnings       []string `json:"warnings,omitempty"`
}

type CleanupResult struct {
	Skipped        bool     `json:"skipped"`
	Reason         string   `json:"reason,omitempty"`
	Considered     int      `json:"considered"`
	Quarantined    int      `json:"quarantined"`
	ReclaimedBytes int64    `json:"reclaimed_bytes"`
	Failed         int      `json:"failed"`
	FailedBytes    int64    `json:"failed_bytes"`
	Warnings       []string `json:"warnings,omitempty"`
}

func NewProvider(config ProviderConfig) *Provider {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	retention := config.Retention
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	newID := config.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	trashDir := config.TrashDir
	if trashDir == "" {
		trashDir = filepath.Join(config.HomeDir, "trash")
	}
	return &Provider{
		registry: config.Registry, store: config.Store, homeDir: filepath.Clean(config.HomeDir),
		trashDir:  filepath.Clean(trashDir),
		retention: retention, now: now, newID: newID,
	}
}

func (p *Provider) Name() string { return "temporary_artifacts" }

// Reconcile repairs the durable lifecycle after an interrupted quarantine
// rename. The filesystem is authoritative only when exactly one of the
// original and quarantine paths exists and the quarantine copy still carries
// the matching owner marker. Ambiguous or unowned paths remain untouched.
func (p *Provider) Reconcile(ctx context.Context) error {
	if p.registry == nil || p.store == nil {
		return errors.New("temporary artifact provider is unavailable")
	}
	artifacts, err := p.registry.List(ctx)
	if err != nil {
		return err
	}
	byID := make(map[string]storage.TemporaryArtifact, len(artifacts))
	for _, artifact := range artifacts {
		byID[artifact.ID] = artifact
	}
	entries, err := p.store.ListQuarantineEntries(ctx, false)
	if err != nil {
		return err
	}
	var problems []error
	for _, entry := range entries {
		if entry.ResourceType != storage.ResourceTypeTemporaryArtifact {
			continue
		}
		if err := p.reconcileEntry(ctx, entry, byID); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func (p *Provider) reconcileEntry(
	ctx context.Context,
	entry storage.QuarantineEntry,
	artifacts map[string]storage.TemporaryArtifact,
) error {
	artifactID, err := quarantineArtifactID(entry, artifacts)
	if err != nil {
		return err
	}
	artifact, ok := artifacts[artifactID]
	if !ok {
		return fmt.Errorf("temporary artifact quarantine %s has no lifecycle row", entry.ID)
	}
	if err := p.validateQuarantineEntry(entry, artifact); err != nil {
		return err
	}
	originalExists, err := pathExists(entry.OriginalPath)
	if err != nil {
		return fmt.Errorf("inspect temporary artifact original %s: %w", entry.ID, err)
	}
	quarantineExists, err := pathExists(entry.QuarantinePath)
	if err != nil {
		return fmt.Errorf("inspect temporary artifact quarantine %s: %w", entry.ID, err)
	}

	switch {
	case quarantineExists && !originalExists:
		return p.reconcileMoved(ctx, entry, artifact)
	case originalExists && !quarantineExists:
		return p.reconcileUnmoved(ctx, entry, artifact)
	case originalExists && quarantineExists:
		return fmt.Errorf("temporary artifact quarantine %s has both original and quarantine paths", entry.ID)
	default:
		return fmt.Errorf("temporary artifact quarantine %s has neither original nor quarantine path", entry.ID)
	}
}

func (p *Provider) reconcileMoved(
	ctx context.Context,
	entry storage.QuarantineEntry,
	artifact storage.TemporaryArtifact,
) error {
	if err := p.registry.ValidateMarker(entry.QuarantinePath, artifact); err != nil {
		return fmt.Errorf("validate temporary artifact quarantine %s: %w", entry.ID, err)
	}
	if entry.State == storage.QuarantineStateFailed {
		if _, err := p.store.TransitionQuarantineEntry(
			ctx, entry.ID, storage.QuarantineStateQuarantined, "",
		); err != nil {
			return fmt.Errorf("reconcile temporary artifact quarantine %s: %w", entry.ID, err)
		}
	}
	if artifact.State != storage.TemporaryArtifactStateQuarantined {
		if err := p.registry.MarkQuarantined(ctx, artifact.ID); err != nil {
			return fmt.Errorf("reconcile temporary artifact lifecycle %s: %w", entry.ID, err)
		}
	}
	return nil
}

func (p *Provider) reconcileUnmoved(
	ctx context.Context,
	entry storage.QuarantineEntry,
	artifact storage.TemporaryArtifact,
) error {
	// The rename did not happen. Release a failed or interrupted intent so the
	// original row can be retried without creating a duplicate active-path entry.
	if entry.State == storage.QuarantineStateQuarantined {
		if _, err := p.store.TransitionQuarantineEntry(
			ctx, entry.ID, storage.QuarantineStateFailed,
			"quarantine move did not complete before restart",
		); err != nil {
			return fmt.Errorf("mark temporary artifact quarantine %s failed: %w", entry.ID, err)
		}
	}
	released, err := storage.ReleaseFailedQuarantineIntent(
		ctx, p.store, storage.ResourceTypeTemporaryArtifact, entry.OriginalPath,
	)
	if err != nil {
		return fmt.Errorf("release temporary artifact quarantine %s: %w", entry.ID, err)
	}
	if released && (artifact.State == storage.TemporaryArtifactStateFailed ||
		artifact.State == storage.TemporaryArtifactStateQuarantined) {
		if err := p.registry.MarkClosed(ctx, artifact.ID); err != nil {
			return fmt.Errorf("restore temporary artifact lifecycle %s: %w", entry.ID, err)
		}
	}
	return nil
}

func (p *Provider) Analyze(ctx context.Context) (Analysis, error) {
	if p.registry == nil || p.store == nil {
		return Analysis{Available: false}, errors.New("temporary artifact provider is unavailable")
	}
	artifacts, err := p.registry.List(ctx)
	if err != nil {
		return Analysis{Available: false}, err
	}
	analysis := Analysis{Available: true}
	cutoff := p.now().Add(-staleArtifactAge)
	for _, artifact := range artifacts {
		if artifact.State == storage.TemporaryArtifactStateQuarantined ||
			artifact.State == storage.TemporaryArtifactStateDeleted {
			continue
		}
		if artifact.State == storage.TemporaryArtifactStateFailed {
			analysis.SkippedCount++
			analysis.Warnings = append(analysis.Warnings, temporaryArtifactWarning(artifact))
			continue
		}
		size, inspectErr := p.inspect(artifact)
		if inspectErr != nil {
			analysis.SkippedCount++
			analysis.Warnings = append(analysis.Warnings, inspectErr.Error())
			continue
		}
		analysis.TotalCount++
		analysis.TotalBytes += size
		if artifact.State == storage.TemporaryArtifactStateActive {
			analysis.ActiveCount++
			analysis.ActiveBytes += size
			continue
		}
		if artifactAge(artifact).After(cutoff) {
			analysis.ProtectedCount++
			analysis.ProtectedBytes += size
			continue
		}
		analysis.StaleCount++
		analysis.StaleBytes += size
	}
	return analysis, nil
}

func (p *Provider) Cleanup(context.Context) (map[string]any, error) {
	return toMap(CleanupResult{Skipped: true, Reason: "manual_only"}), nil
}

func (p *Provider) CleanupExplicit(ctx context.Context) (map[string]any, error) {
	result, err := p.cleanupExplicit(ctx)
	return toMap(result), err
}

func (p *Provider) cleanupExplicit(ctx context.Context) (CleanupResult, error) {
	if p.registry == nil || p.store == nil {
		return CleanupResult{}, errors.New("temporary artifact provider is unavailable")
	}
	if err := p.Reconcile(ctx); err != nil {
		return CleanupResult{}, err
	}
	if err := p.releaseRetryableFailures(ctx); err != nil {
		return CleanupResult{}, err
	}
	artifacts, err := p.registry.List(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	if err := p.ensureTrashRoot(); err != nil {
		return CleanupResult{}, err
	}
	cutoff := p.now().Add(-staleArtifactAge)
	result := CleanupResult{}
	var failures []error
	for _, artifact := range artifacts {
		if artifact.State != storage.TemporaryArtifactStateClosed &&
			artifact.State != storage.TemporaryArtifactStateAbandoned {
			continue
		}
		if artifactAge(artifact).After(cutoff) {
			continue
		}
		result.Considered++
		size, inspectErr := p.inspect(artifact)
		if inspectErr != nil {
			result.Failed++
			result.Warnings = append(result.Warnings, inspectErr.Error())
			failures = append(failures, inspectErr)
			continue
		}
		if err := p.quarantine(ctx, artifact, size); err != nil {
			result.Failed++
			result.FailedBytes += size
			result.Warnings = append(result.Warnings, err.Error())
			failures = append(failures, err)
			continue
		}
		result.Quarantined++
		result.ReclaimedBytes += size
	}
	return result, errors.Join(failures...)
}

func (p *Provider) releaseRetryableFailures(ctx context.Context) error {
	artifacts, err := p.registry.List(ctx)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if artifact.State != storage.TemporaryArtifactStateFailed {
			continue
		}
		released, releaseErr := storage.ReleaseFailedQuarantineIntent(
			ctx, p.store, storage.ResourceTypeTemporaryArtifact, artifact.Path,
		)
		if releaseErr != nil || !released {
			continue
		}
		if err := p.registry.MarkClosed(ctx, artifact.ID); err != nil {
			return fmt.Errorf("release temporary artifact %s: %w", artifact.ID, err)
		}
	}
	return nil
}

func (p *Provider) quarantine(
	ctx context.Context,
	artifact storage.TemporaryArtifact,
	size int64,
) error {
	id := p.newID()
	if id == "" {
		return errors.New("temporary artifact quarantine id must not be empty")
	}
	quarantinePath := filepath.Join(p.trashDir, "temporary-artifacts", id)
	if err := storage.ValidateNoSymlinkPath(p.trashDir, filepath.Dir(quarantinePath)); err != nil {
		return fmt.Errorf("validate temporary artifact trash: %w", err)
	}
	if _, err := os.Lstat(quarantinePath); err == nil {
		return fmt.Errorf("temporary artifact quarantine destination already exists: %s", quarantinePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect temporary artifact quarantine destination: %w", err)
	}
	now := p.now().UTC()
	metadata, _ := json.Marshal(map[string]string{
		"artifact_id": artifact.ID, "kind": string(artifact.Kind), "marker_token": artifact.MarkerToken,
	})
	entry := &storage.QuarantineEntry{
		ID: id, ResourceType: storage.ResourceTypeTemporaryArtifact,
		OriginalPath: artifact.Path, QuarantinePath: quarantinePath, SizeBytes: size,
		State: storage.QuarantineStateQuarantined, QuarantinedAt: now,
		DeleteAfter: now.Add(p.retention), Metadata: metadata,
	}
	if err := p.store.CreateQuarantineEntry(ctx, entry); err != nil {
		return fmt.Errorf("persist temporary artifact quarantine intent: %w", err)
	}
	if err := p.registry.Validate(artifact); err != nil {
		_, _ = p.store.TransitionQuarantineEntry(ctx, entry.ID, storage.QuarantineStateFailed, err.Error())
		return err
	}
	if err := os.Rename(artifact.Path, quarantinePath); err != nil {
		_, _ = p.store.TransitionQuarantineEntry(ctx, entry.ID, storage.QuarantineStateFailed, err.Error())
		_ = p.registry.MarkFailed(ctx, artifact.ID, err.Error())
		return fmt.Errorf("quarantine temporary artifact: %w", err)
	}
	if err := p.registry.MarkQuarantined(ctx, artifact.ID); err != nil {
		_, _ = p.store.TransitionQuarantineEntry(ctx, entry.ID, storage.QuarantineStateFailed, err.Error())
		return fmt.Errorf("persist temporary artifact quarantine: %w", err)
	}
	return nil
}

func (p *Provider) Restore(ctx context.Context, id string) (storage.QuarantineEntry, error) {
	entry, artifact, err := p.loadQuarantineArtifact(ctx, id)
	if err != nil {
		return storage.QuarantineEntry{}, err
	}
	if _, err := os.Lstat(artifact.Path); err == nil {
		return entry, fmt.Errorf("%w: temporary artifact restore destination already exists", storage.ErrConflict)
	} else if !errors.Is(err, os.ErrNotExist) {
		return entry, fmt.Errorf("inspect temporary artifact restore destination: %w", err)
	}
	if err := storage.ValidateNoSymlinkPath(p.registry.TempRoot(), artifact.Path); err != nil {
		return entry, err
	}
	if err := os.Rename(entry.QuarantinePath, artifact.Path); err != nil {
		return entry, fmt.Errorf("restore temporary artifact: %w", err)
	}
	if err := p.registry.MarkClosed(ctx, artifact.ID); err != nil {
		_ = os.Rename(artifact.Path, entry.QuarantinePath)
		return entry, fmt.Errorf("persist temporary artifact lifecycle restore: %w", err)
	}
	restored, err := p.store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateRestored, "")
	if err != nil {
		_ = os.Rename(artifact.Path, entry.QuarantinePath)
		return entry, fmt.Errorf("persist temporary artifact restore: %w", err)
	}
	return restored, nil
}

func (p *Provider) PermanentDelete(ctx context.Context, id, confirmation string) (storage.QuarantineEntry, error) {
	return p.permanentDelete(ctx, id, confirmation, false)
}

func (p *Provider) PermanentDeleteForce(ctx context.Context, id, confirmation string) (storage.QuarantineEntry, error) {
	return p.permanentDelete(ctx, id, confirmation, true)
}

func (p *Provider) permanentDelete(
	ctx context.Context,
	id, confirmation string,
	force bool,
) (storage.QuarantineEntry, error) {
	entry, artifact, err := p.loadQuarantineArtifact(ctx, id)
	if err != nil {
		return storage.QuarantineEntry{}, err
	}
	if force {
		if confirmation != storage.QuarantineConfirmationForce {
			return entry, storage.ErrForceDeleteConfirmation
		}
	} else {
		if confirmation != storage.QuarantineConfirmationDelete {
			return entry, fmt.Errorf("%w: quarantine deletion requires DELETE confirmation", storage.ErrValidation)
		}
		if p.now().UTC().Before(entry.DeleteAfter) {
			return entry, fmt.Errorf("%w: quarantine retention deadline has not elapsed", storage.ErrConflict)
		}
	}
	if err := p.registry.ValidateMarker(entry.QuarantinePath, artifact); err != nil {
		return entry, err
	}
	if err := os.RemoveAll(entry.QuarantinePath); err != nil {
		return entry, fmt.Errorf("delete temporary artifact quarantine: %w", err)
	}
	deleted, err := p.store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateDeleted, "")
	if err != nil {
		return entry, fmt.Errorf("persist temporary artifact deletion: %w", err)
	}
	if err := p.registry.MarkDeleted(ctx, artifact.ID); err != nil {
		return deleted, fmt.Errorf("persist temporary artifact lifecycle deletion: %w", err)
	}
	return deleted, nil
}

func (p *Provider) loadQuarantineArtifact(
	ctx context.Context,
	id string,
) (storage.QuarantineEntry, storage.TemporaryArtifact, error) {
	if p.store == nil || p.registry == nil {
		return storage.QuarantineEntry{}, storage.TemporaryArtifact{}, errors.New("temporary artifact provider is unavailable")
	}
	entry, err := p.store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return storage.QuarantineEntry{}, storage.TemporaryArtifact{}, err
	}
	if entry.ResourceType != storage.ResourceTypeTemporaryArtifact {
		return entry, storage.TemporaryArtifact{}, fmt.Errorf("%w: unsupported quarantine resource %q", storage.ErrValidation, entry.ResourceType)
	}
	if entry.State != storage.QuarantineStateQuarantined && entry.State != storage.QuarantineStateFailed {
		return entry, storage.TemporaryArtifact{}, fmt.Errorf("%w: temporary artifact quarantine entry is %q", storage.ErrConflict, entry.State)
	}
	artifactID, artifactIDErr := quarantineArtifactID(entry, nil)
	if artifactIDErr != nil {
		return entry, storage.TemporaryArtifact{}, artifactIDErr
	}
	artifact, err := p.registry.Get(ctx, artifactID)
	if err != nil {
		return entry, storage.TemporaryArtifact{}, err
	}
	expected := filepath.Join(p.trashDir, "temporary-artifacts", entry.ID)
	if filepath.Clean(entry.QuarantinePath) != filepath.Clean(expected) || artifact.Path != entry.OriginalPath {
		return entry, storage.TemporaryArtifact{}, fmt.Errorf("%w: temporary artifact quarantine paths do not match managed storage", storage.ErrValidation)
	}
	if err := p.registry.ValidateMarker(entry.QuarantinePath, artifact); err != nil {
		return entry, storage.TemporaryArtifact{}, err
	}
	return entry, artifact, nil
}

func (p *Provider) inspect(artifact storage.TemporaryArtifact) (int64, error) {
	if err := p.registry.Validate(artifact); err != nil {
		return 0, fmt.Errorf("skip temporary artifact %s: %w", artifact.Path, err)
	}
	size, err := directorySizeNoFollow(artifact.Path)
	if err != nil {
		return 0, fmt.Errorf("measure temporary artifact %s: %w", artifact.Path, err)
	}
	return size, nil
}

func (p *Provider) validateQuarantineEntry(
	entry storage.QuarantineEntry,
	artifact storage.TemporaryArtifact,
) error {
	expected := filepath.Join(p.trashDir, "temporary-artifacts", entry.ID)
	if artifact.Path != entry.OriginalPath ||
		filepath.Clean(entry.QuarantinePath) != filepath.Clean(expected) {
		return fmt.Errorf("temporary artifact quarantine %s paths do not match managed storage", entry.ID)
	}
	if !filepath.IsAbs(entry.OriginalPath) || !filepath.IsAbs(entry.QuarantinePath) {
		return fmt.Errorf("temporary artifact quarantine %s paths must be absolute", entry.ID)
	}
	if _, err := os.Lstat(entry.QuarantinePath); err == nil {
		if err := storage.ValidateNoSymlinkPath(p.trashDir, filepath.Dir(entry.QuarantinePath)); err != nil {
			return fmt.Errorf("validate temporary artifact quarantine path %s: %w", entry.ID, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect temporary artifact quarantine path %s: %w", entry.ID, err)
	}
	return nil
}

func quarantineArtifactID(
	entry storage.QuarantineEntry,
	artifacts map[string]storage.TemporaryArtifact,
) (string, error) {
	var metadata struct {
		ArtifactID string `json:"artifact_id"`
	}
	if len(entry.Metadata) > 0 {
		if err := json.Unmarshal(entry.Metadata, &metadata); err != nil {
			return "", fmt.Errorf("decode temporary artifact quarantine %s metadata: %w", entry.ID, err)
		}
	}
	if metadata.ArtifactID != "" {
		return metadata.ArtifactID, nil
	}
	// Rows written by the initial implementation used the artifact ID as the
	// quarantine ID. New rows carry an explicit artifact_id because one
	// artifact can be restored and quarantined again.
	if artifacts == nil {
		return entry.ID, nil
	}
	if _, ok := artifacts[entry.ID]; ok {
		return entry.ID, nil
	}
	return "", fmt.Errorf("temporary artifact quarantine %s has no artifact ID", entry.ID)
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (p *Provider) ensureTrashRoot() error {
	if err := storage.ValidateNoSymlinkPath(p.homeDir, p.trashDir); err != nil {
		return err
	}
	if err := os.MkdirAll(p.trashDir, 0o700); err != nil {
		return fmt.Errorf("create storage trash: %w", err)
	}
	root := filepath.Join(p.trashDir, "temporary-artifacts")
	if err := storage.ValidateNoSymlinkPath(p.trashDir, root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create temporary artifact trash: %w", err)
	}
	return nil
}

func artifactAge(artifact storage.TemporaryArtifact) time.Time {
	if artifact.ClosedAt != nil {
		return artifact.ClosedAt.UTC()
	}
	if artifact.LastHeartbeatAt != nil {
		return artifact.LastHeartbeatAt.UTC()
	}
	return artifact.CreatedAt.UTC()
}

func temporaryArtifactWarning(artifact storage.TemporaryArtifact) string {
	if artifact.LastError == "" {
		return fmt.Sprintf("skip temporary artifact %s: lifecycle state is %s", artifact.Path, artifact.State)
	}
	return fmt.Sprintf("skip temporary artifact %s: %s", artifact.Path, artifact.LastError)
}

func directorySizeNoFollow(root string) (int64, error) {
	var size int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink found at %s", path)
		}
		if info.Mode().IsRegular() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func toMap(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	result := make(map[string]any)
	_ = json.Unmarshal(encoded, &result)
	return result
}
