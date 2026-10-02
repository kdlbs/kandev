// Package gocache owns Kandev's opt-in local Go build cache.
package gocache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/filescan"
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

// Config contains the provider's install-owned paths and persistence dependencies.
type Config struct {
	HomeDir    string
	TrashDir   string
	Settings   SettingsSource
	Mutations  *storage.MutationGate
	Scanner    *filescan.Limiter
	OnProgress func(filescan.Progress)
}

// Provider manages the single Go cache selected by persisted settings.
type Provider struct {
	config       Config
	cleanupMu    sync.Mutex
	cleanupState *cleanupSession
}

// Analysis describes the configured cache without changing it.
type Analysis struct {
	Path          string `json:"path"`
	SizeBytes     int64  `json:"size_bytes"`
	Owned         bool   `json:"owned"`
	Enabled       bool   `json:"enabled"`
	UnmanagedPath string `json:"unmanaged_path,omitempty"`
	// A nil size means that the distinct user cache was not measured. A pointer
	// preserves an explicitly measured zero in the successful response.
	UnmanagedSizeBytes *int64 `json:"unmanaged_size_bytes,omitempty"`
}

// CleanupResult describes bounded deletion of Go build-cache contents.
type CleanupResult struct {
	Path                string                   `json:"path"`
	Skipped             bool                     `json:"skipped"`
	Reason              string                   `json:"reason,omitempty"`
	BytesBefore         int64                    `json:"bytes_before"`
	BytesBeforeComplete bool                     `json:"bytes_before_complete"`
	BytesAfter          *int64                   `json:"bytes_after"`
	ReclaimedBytes      int64                    `json:"reclaimed_bytes"`
	Partial             bool                     `json:"partial,omitempty"`
	Errors              []string                 `json:"errors,omitempty"`
	QuarantineEntry     *storage.QuarantineEntry `json:"quarantine_entry"`
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
	scanner := p.config.Scanner
	if scanner == nil {
		scanner = filescan.NewLimiter(4)
	}
	measurementRoots := []filescan.Root{
		{
			Path: cachePath, MissingOK: true, SymlinkPolicy: filescan.RejectSymlinks,
			Exclude: func(path string, _ fs.DirEntry) bool { return path == markerPath(cachePath) },
		},
	}
	unmanagedPath, hasUnmanagedPath := defaultGoCachePath()
	if hasUnmanagedPath && unmanagedPath != cachePath {
		measurementRoots = append(measurementRoots, filescan.Root{
			Path: unmanagedPath, MissingOK: true, SymlinkPolicy: filescan.SkipSymlinks,
		})
	}
	measurements := scanner.Measure(ctx, measurementRoots, p.config.OnProgress)
	if len(measurements) != len(measurementRoots) {
		return Analysis{}, errors.New("go-cache scanner returned an invalid result")
	}
	if err := measurements[0].Err; err != nil {
		return Analysis{}, fmt.Errorf("measure Go cache: %w", err)
	}
	analysis := Analysis{
		Path: cachePath, SizeBytes: measurements[0].Bytes, Owned: owned, Enabled: settings.GoCache.Enabled,
	}
	if !hasUnmanagedPath || unmanagedPath == cachePath {
		return analysis, nil
	}
	if err := measurements[1].Err; err != nil {
		return Analysis{}, fmt.Errorf("measure Go cache: %w", err)
	}
	analysis.UnmanagedPath = unmanagedPath
	unmanagedSizeBytes := measurements[1].Bytes
	analysis.UnmanagedSizeBytes = &unmanagedSizeBytes
	return analysis, nil
}

func (p *Provider) AnalyzeWithProgress(
	ctx context.Context,
	onProgress func(filescan.Progress),
) (Analysis, error) {
	copy := &Provider{config: p.config}
	copy.config.OnProgress = onProgress
	return copy.Analyze(ctx)
}

// MeasurementRoots returns every filesystem root included by Analyze for the
// supplied settings. Callers use these roots to avoid attributing one file to
// multiple storage categories.
func (p *Provider) MeasurementRoots(settings storage.StorageMaintenanceSettings) ([]string, error) {
	cachePath, _, err := p.cachePath(settings)
	if err != nil {
		return nil, err
	}
	roots := []string{cachePath}
	unmanagedPath, hasUnmanagedPath := defaultGoCachePath()
	if hasUnmanagedPath && unmanagedPath != cachePath {
		roots = append(roots, unmanagedPath)
	}
	return roots, nil
}

func defaultGoCachePath() (string, bool) {
	if configured := os.Getenv("GOCACHE"); configured != "" {
		if configured == "off" || !filepath.IsAbs(configured) {
			return "", false
		}
		return filepath.Clean(configured), true
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil || !filepath.IsAbs(cacheDir) {
		return "", false
	}
	return filepath.Join(cacheDir, "go-build"), true
}

// Cleanup deletes an above-threshold cache's build data in place.
func (p *Provider) Cleanup(ctx context.Context) (CleanupResult, error) {
	return p.cleanup(ctx, false)
}

func (p *Provider) CleanupWithSettings(
	ctx context.Context,
	settings storage.StorageMaintenanceSettings,
) (CleanupResult, error) {
	return p.cleanupWithSettings(ctx, settings, false)
}

// CleanupExplicit deletes above-threshold cache data even when scheduling is disabled.
func (p *Provider) CleanupExplicit(ctx context.Context) (CleanupResult, error) {
	return p.cleanup(ctx, true)
}

func (p *Provider) CleanupExplicitWithSettings(
	ctx context.Context,
	settings storage.StorageMaintenanceSettings,
) (CleanupResult, error) {
	return p.cleanupWithSettings(ctx, settings, true)
}

func (p *Provider) cleanup(ctx context.Context, explicit bool) (CleanupResult, error) {
	cleanupCtx, cancel := context.WithTimeout(ctx, cleanupDeadline)
	defer cancel()
	release, err := p.config.Mutations.Acquire(cleanupCtx)
	if err != nil {
		return CleanupResult{Partial: true, Errors: []string{cancellationIssue(err)}}, err
	}
	defer release()
	settings, err := p.loadSettings(cleanupCtx)
	if err != nil {
		return CleanupResult{}, err
	}
	return p.cleanupWithSettingsLocked(cleanupCtx, settings, explicit)
}

func (p *Provider) cleanupWithSettings(
	ctx context.Context,
	settings storage.StorageMaintenanceSettings,
	explicit bool,
) (CleanupResult, error) {
	cleanupCtx, cancel := context.WithTimeout(ctx, cleanupDeadline)
	defer cancel()
	release, err := p.config.Mutations.Acquire(cleanupCtx)
	if err != nil {
		return CleanupResult{Partial: true, Errors: []string{cancellationIssue(err)}}, err
	}
	defer release()
	return p.cleanupWithSettingsLocked(cleanupCtx, settings, explicit)
}

func (p *Provider) cleanupWithSettingsLocked(
	ctx context.Context,
	settings storage.StorageMaintenanceSettings,
	explicit bool,
) (CleanupResult, error) {
	cachePath, adopted, err := p.cachePath(settings)
	if err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{Path: cachePath}
	if !settings.GoCache.Enabled && !explicit {
		return result, nil
	}
	if err := p.validateCachePath(cachePath); err != nil {
		return result, err
	}
	trashRoot := filepath.Clean(p.config.TrashDir)
	if !filepath.IsAbs(trashRoot) {
		return result, fmt.Errorf("go-cache trash path must be absolute: %q", trashRoot)
	}
	if pathsOverlap(cachePath, trashRoot) {
		return result, errors.New("go cache and Kandev trash must not contain each other")
	}
	if !adopted && !hasValidMarker(cachePath) {
		return result, ErrNotOwned
	}
	return p.cleanupContents(ctx, cachePath, adopted, settings.GoCache.MaxBytes)
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
	return nil
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
