// Package gocache owns Kandev's opt-in local Go build cache.
package gocache

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

var (
	// ErrNotOwned is returned when the managed path lacks Kandev's marker.
	ErrNotOwned             = errors.New("go cache is not owned by Kandev")
	ErrAdoptionConfirmation = errors.New("go cache adoption requires ADOPT confirmation")
)

const (
	markerName    = ".go-build.kandev-owned"
	markerContent = "kandev-managed-go-cache\n"
)

// SettingsSource returns the persisted install-wide storage settings.
type SettingsSource interface {
	GetSettings(ctx context.Context) (storage.StorageMaintenanceSettings, error)
}

// QuarantineStore persists cache rotations before filesystem mutation.
type QuarantineStore interface {
	CreateQuarantineEntry(ctx context.Context, entry *storage.QuarantineEntry) error
	TransitionQuarantineEntry(ctx context.Context, id string, next storage.QuarantineState, lastError string) (storage.QuarantineEntry, error)
	ListQuarantineEntries(ctx context.Context, includeTerminal bool) ([]storage.QuarantineEntry, error)
}

// Config contains the provider's install-owned paths and persistence dependencies.
type Config struct {
	HomeDir  string
	TrashDir string
	Settings SettingsSource
	Store    QuarantineStore
}

// Provider manages the single Go cache selected by persisted settings.
type Provider struct {
	config Config
}

// Analysis describes the configured cache without changing it.
type Analysis struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Owned     bool   `json:"owned"`
	Enabled   bool   `json:"enabled"`
}

// CleanupResult describes one cache rotation.
type CleanupResult struct {
	Path            string                   `json:"path"`
	BytesBefore     int64                    `json:"bytes_before"`
	BytesAfter      int64                    `json:"bytes_after"`
	ReclaimedBytes  int64                    `json:"reclaimed_bytes"`
	QuarantineEntry *storage.QuarantineEntry `json:"quarantine_entry"`
}

// New creates a managed Go-cache provider.
func New(config Config) *Provider {
	return &Provider{config: config}
}

// ExecutionEnvironment returns variables injected into new local executions.
func (p *Provider) ExecutionEnvironment(ctx context.Context) (map[string]string, error) {
	settings, err := p.loadSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.GoCache.Enabled {
		return nil, nil
	}
	cachePath, adopted, err := p.cachePath(settings)
	if err != nil {
		return nil, err
	}
	if err := p.validateCachePath(cachePath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return nil, fmt.Errorf("create managed Go cache: %w", err)
	}
	if !adopted {
		if err := writeMarker(cachePath); err != nil {
			return nil, fmt.Errorf("create Go-cache ownership marker: %w", err)
		}
	}
	return map[string]string{"GOCACHE": cachePath}, nil
}

// Analyze reports the selected cache's current usage.
func (p *Provider) Analyze(ctx context.Context) (Analysis, error) {
	settings, err := p.loadSettings(ctx)
	if err != nil {
		return Analysis{}, err
	}
	cachePath, adopted, err := p.cachePath(settings)
	if err != nil {
		return Analysis{}, err
	}
	if err := p.validateCachePath(cachePath); err != nil {
		return Analysis{}, err
	}
	owned := adopted || hasValidMarker(cachePath)
	size, err := directorySize(cachePath)
	if err != nil {
		return Analysis{}, err
	}
	return Analysis{Path: cachePath, SizeBytes: size, Owned: owned, Enabled: settings.GoCache.Enabled}, nil
}

// Cleanup rotates an above-threshold cache into Kandev trash.
func (p *Provider) Cleanup(ctx context.Context) (CleanupResult, error) {
	return p.cleanup(ctx, false)
}

// CleanupExplicit rotates an above-threshold cache even when scheduled maintenance is disabled.
func (p *Provider) CleanupExplicit(ctx context.Context) (CleanupResult, error) {
	return p.cleanup(ctx, true)
}

func (p *Provider) cleanup(ctx context.Context, explicit bool) (CleanupResult, error) {
	settings, err := p.loadSettings(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	cachePath, adopted, err := p.cachePath(settings)
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{Path: cachePath}
	if !settings.GoCache.Enabled && !explicit {
		return result, nil
	}
	if err := p.validateCacheAndTrash(cachePath); err != nil {
		return result, err
	}
	if !adopted && !hasValidMarker(cachePath) {
		return result, ErrNotOwned
	}
	result.BytesBefore, err = directorySize(cachePath)
	if err != nil {
		return result, err
	}
	if result.BytesBefore <= settings.GoCache.MaxBytes {
		result.BytesAfter = result.BytesBefore
		return result, nil
	}
	entry, err := p.rotate(ctx, cachePath, result.BytesBefore, settings.QuarantineRetentionHours, adopted)
	if err != nil {
		return result, err
	}
	result.QuarantineEntry = entry
	result.ReclaimedBytes = result.BytesBefore
	return result, nil
}

// ValidateAdoption verifies an explicitly confirmed external cache path.
func (p *Provider) ValidateAdoption(_ context.Context, path, confirmation string) error {
	if confirmation != "ADOPT" {
		return ErrAdoptionConfirmation
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("adopted Go-cache path must be absolute: %q", path)
	}
	path = filepath.Clean(path)
	if path == filepath.VolumeName(path)+string(filepath.Separator) {
		return errors.New("filesystem root cannot be adopted as a Go cache")
	}
	trashRoot := filepath.Clean(p.config.TrashDir)
	if !filepath.IsAbs(trashRoot) {
		return fmt.Errorf("go-cache trash path must be absolute: %q", trashRoot)
	}
	if pathsOverlap(path, trashRoot) {
		return errors.New("adopted Go cache and Kandev trash must not contain each other")
	}
	if err := p.validateCacheAndTrash(path); err != nil {
		return err
	}
	return probeAtomicRename(path, trashRoot)
}

func (p *Provider) loadSettings(ctx context.Context) (storage.StorageMaintenanceSettings, error) {
	if p.config.Settings == nil {
		return storage.StorageMaintenanceSettings{}, errors.New("go-cache settings source is required")
	}
	settings, err := p.config.Settings.GetSettings(ctx)
	if err != nil {
		return storage.StorageMaintenanceSettings{}, fmt.Errorf("load Go-cache settings: %w", err)
	}
	return settings, nil
}

func (p *Provider) cachePath(settings storage.StorageMaintenanceSettings) (string, bool, error) {
	path := settings.GoCache.AdoptedPath
	adopted := path != ""
	if !adopted {
		path = filepath.Join(p.config.HomeDir, "cache", "go-build")
	}
	if !filepath.IsAbs(path) {
		return "", false, fmt.Errorf("managed Go-cache path must be absolute: %q", path)
	}
	return filepath.Clean(path), adopted, nil
}

func (p *Provider) rotate(
	ctx context.Context,
	cachePath string,
	sizeBytes int64,
	retentionHours int,
	adopted bool,
) (*storage.QuarantineEntry, error) {
	if p.config.Store == nil {
		return nil, errors.New("go-cache quarantine store is required")
	}
	trashRoot := filepath.Clean(p.config.TrashDir)
	if !filepath.IsAbs(trashRoot) {
		return nil, fmt.Errorf("go-cache trash path must be absolute: %q", trashRoot)
	}
	if err := p.validateCacheAndTrash(cachePath); err != nil {
		return nil, err
	}
	quarantineDir := filepath.Join(trashRoot, "go-cache")
	anchor, err := storage.CommonPath(p.config.HomeDir, cachePath, trashRoot)
	if err != nil {
		return nil, err
	}
	if err := storage.ValidateNoSymlinkPath(anchor, quarantineDir); err != nil {
		return nil, fmt.Errorf("validate Go-cache quarantine directory: %w", err)
	}
	if err := os.MkdirAll(quarantineDir, 0o700); err != nil {
		return nil, fmt.Errorf("create Go-cache trash: %w", err)
	}
	if _, err := storage.ReleaseFailedQuarantineIntent(
		ctx, p.config.Store, storage.ResourceTypeGoCache, cachePath,
	); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	ownership := "managed"
	if adopted {
		ownership = "adopted"
	}
	metadata, _ := json.Marshal(map[string]string{"ownership": ownership})
	entry := &storage.QuarantineEntry{
		ID:             id,
		ResourceType:   storage.ResourceTypeGoCache,
		OriginalPath:   cachePath,
		QuarantinePath: filepath.Join(quarantineDir, id),
		SizeBytes:      sizeBytes,
		State:          storage.QuarantineStateQuarantined,
		QuarantinedAt:  now,
		DeleteAfter:    now.Add(time.Duration(retentionHours) * time.Hour),
		Metadata:       metadata,
	}
	if err := p.config.Store.CreateQuarantineEntry(ctx, entry); err != nil {
		return nil, fmt.Errorf("persist Go-cache quarantine intent: %w", err)
	}
	if err := os.Rename(cachePath, entry.QuarantinePath); err != nil {
		_, _ = p.config.Store.TransitionQuarantineEntry(ctx, id, storage.QuarantineStateFailed, err.Error())
		return nil, fmt.Errorf("quarantine Go cache: %w", err)
	}
	if err := p.validateCachePath(cachePath); err != nil {
		return nil, fmt.Errorf("validate replacement Go cache: %w", err)
	}
	if err := os.MkdirAll(cachePath, 0o755); err != nil {
		return nil, fmt.Errorf("recreate managed Go cache: %w", err)
	}
	if !adopted {
		if err := writeMarker(cachePath); err != nil {
			return nil, fmt.Errorf("restore Go-cache ownership marker: %w", err)
		}
	}
	return entry, nil
}

func writeMarker(cachePath string) error {
	marker := markerPath(cachePath)
	info, err := os.Lstat(marker)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrNotOwned
		}
		existing, readErr := os.ReadFile(marker)
		if readErr != nil || string(existing) != markerContent {
			return ErrNotOwned
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Go-cache ownership marker: %w", err)
	}
	return os.WriteFile(marker, []byte(markerContent), 0o600)
}

func hasValidMarker(cachePath string) bool {
	path := markerPath(cachePath)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false
	}
	marker, err := os.ReadFile(path)
	return err == nil && string(marker) == markerContent
}

func markerPath(cachePath string) string {
	return filepath.Join(cachePath, markerName)
}

// RemoveRestorePlaceholder removes the empty cache directory recreated after
// rotation. Managed caches must contain only their bound ownership marker;
// adopted caches must be completely empty.
func RemoveRestorePlaceholder(cachePath string, adopted bool) (bool, error) {
	placeholder, err := IsRestorePlaceholder(cachePath, adopted)
	if err != nil || !placeholder {
		return placeholder, err
	}
	if !adopted {
		if err := os.Remove(markerPath(cachePath)); err != nil {
			return false, fmt.Errorf("remove Go-cache restore marker: %w", err)
		}
	}
	if err := os.Remove(cachePath); err != nil {
		return false, fmt.Errorf("remove Go-cache restore placeholder: %w", err)
	}
	return true, nil
}

// IsRestorePlaceholder reports whether cachePath is the empty replacement
// created after a cache rotation, without modifying it.
func IsRestorePlaceholder(cachePath string, adopted bool) (bool, error) {
	info, err := os.Lstat(cachePath)
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(cachePath)
	if err != nil {
		return false, fmt.Errorf("inspect Go-cache restore placeholder: %w", err)
	}
	if adopted {
		return len(entries) == 0, nil
	}
	return len(entries) == 1 && entries[0].Name() == markerName && hasValidMarker(cachePath), nil
}

func (p *Provider) validateCachePath(cachePath string) error {
	anchor, err := storage.CommonPath(p.config.HomeDir, cachePath)
	if err != nil {
		return err
	}
	if err := storage.ValidateNoSymlinkPath(anchor, cachePath); err != nil {
		return fmt.Errorf("validate Go-cache path: %w", err)
	}
	return rejectSymlink(cachePath)
}

func (p *Provider) validateCacheAndTrash(cachePath string) error {
	trashRoot := filepath.Clean(p.config.TrashDir)
	anchor, err := storage.CommonPath(p.config.HomeDir, cachePath, trashRoot)
	if err != nil {
		return err
	}
	for name, path := range map[string]string{"cache": cachePath, "trash": trashRoot} {
		if err := storage.ValidateNoSymlinkPath(anchor, path); err != nil {
			return fmt.Errorf("validate Go-cache %s path: %w", name, err)
		}
	}
	return rejectSymlink(cachePath)
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path) // codeql[go/path-injection] path is constrained by ValidateNoSymlinkPath.
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Go-cache path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("go-cache path must not be a symlink: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("go-cache path is not a directory: %s", path)
	}
	return nil
}

func directorySize(root string) (int64, error) {
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("inspect Go-cache path: %w", err)
	}
	var total int64
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink found in Go cache: %s", entry.Name())
		}
		if path == markerPath(root) {
			return nil
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("measure Go cache: %w", err)
	}
	return total, nil
}

func pathsOverlap(first, second string) bool {
	return pathContains(first, second) || pathContains(second, first)
}

func pathContains(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !startsWithParent(rel)
}

func startsWithParent(rel string) bool {
	return len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)
}

func probeAtomicRename(cachePath, trashRoot string) error {
	if err := os.MkdirAll(trashRoot, 0o700); err != nil { // codeql[go/path-injection] validated by validateCacheAndTrash.
		return fmt.Errorf("create Go-cache trash: %w", err)
	}
	probe, err := os.CreateTemp(cachePath, ".kandev-adoption-probe-") // codeql[go/path-injection] cachePath is validated by validateCacheAndTrash.
	if err != nil {
		return fmt.Errorf("create adoption probe: %w", err)
	}
	source := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(source)
		return fmt.Errorf("close adoption probe: %w", err)
	}
	destination := filepath.Join(trashRoot, filepath.Base(source))
	if err := os.Rename(source, destination); err != nil {
		_ = os.Remove(source)
		return fmt.Errorf("adopted Go cache must support atomic rename into Kandev trash: %w", err)
	}
	if err := os.Remove(destination); err != nil {
		return fmt.Errorf("remove adoption probe: %w", err)
	}
	return nil
}
