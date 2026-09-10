// Package databasestore measures the local filesystem footprint of Kandev's
// configured database and its sibling backup directory.
package databasestore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/kandev/kandev/internal/system/storage/filescan"
)

type Status string

const (
	StatusMeasured      Status = "measured"
	StatusUnavailable   Status = "unavailable"
	StatusNotApplicable Status = "not_applicable"
)

const (
	ReasonUnsupportedDriver               = "unsupported_driver"
	ReasonMeasurementFailed               = "measurement_failed"
	ReasonOverlapsExistingSource          = "overlaps_existing_source"
	ReasonPartiallyOverlapsExistingSource = "partially_overlaps_existing_source"
)

// Measurement is one independently attributed local storage measurement.
// SizeBytes is a pointer so a measured zero is distinct from an unknown value.
type Measurement struct {
	Status           Status `json:"status"`
	SizeBytes        *int64 `json:"size_bytes,omitempty"`
	CountedSizeBytes *int64 `json:"counted_size_bytes,omitempty"`
	Path             string `json:"path,omitempty"`
	IncludedInTotal  bool   `json:"included_in_total"`
	Reason           string `json:"reason,omitempty"`
	Warning          string `json:"warning,omitempty"`
}

type Config struct {
	Driver        string
	DatabasePath  string
	Scanner       *filescan.Limiter
	ExistingRoots []string
}

type Provider struct {
	driver        string
	databasePath  string
	limiter       *filescan.Limiter
	existingRoots []string
}

func New(config Config) *Provider {
	return &Provider{
		driver:        strings.ToLower(strings.TrimSpace(config.Driver)),
		databasePath:  absoluteCleanPath(config.DatabasePath),
		limiter:       config.Scanner,
		existingRoots: normalizePaths(config.ExistingRoots),
	}
}

func (p *Provider) AnalyzeDatabase(
	ctx context.Context,
	notify func(filescan.Progress),
	additionalRoots ...string,
) (Measurement, error) {
	if !p.localSQLite() {
		return notApplicableMeasurement(), nil
	}
	if err := ctx.Err(); err != nil {
		return unavailableMeasurement("", err), err
	}
	databasePath, err := p.resolvedDatabasePath()
	if err != nil {
		return unavailableMeasurement(p.databasePath, err), err
	}
	if err := validateRequiredFile(databasePath); err != nil {
		return unavailableMeasurement(databasePath, err), err
	}
	overlapRoots := p.existingRootsFor(additionalRoots)

	roots := []filescan.Root{{
		Path: databasePath, SymlinkPolicy: filescan.RejectSymlinks, OverlapRoots: overlapRoots,
	}}
	for _, sidecar := range sidecarPaths(databasePath) {
		info, statErr := os.Lstat(sidecar)
		switch {
		case errors.Is(statErr, os.ErrNotExist):
			continue
		case statErr != nil:
			return unavailableMeasurement(databasePath, statErr), statErr
		case info.Mode()&os.ModeSymlink != 0:
			err := fmt.Errorf("database sidecar is a symlink: %s", sidecar)
			return unavailableMeasurement(databasePath, err), err
		case !info.Mode().IsRegular():
			err := fmt.Errorf("database sidecar is not a regular file: %s", sidecar)
			return unavailableMeasurement(databasePath, err), err
		default:
			roots = append(roots, filescan.Root{
				Path: sidecar, MissingOK: true, SymlinkPolicy: filescan.RejectSymlinks,
				OverlapRoots: overlapRoots,
			})
		}
	}

	measurements := p.scanner().Measure(ctx, roots, notify)
	var total int64
	var overlapped int64
	for index, result := range measurements {
		if result.Err != nil {
			if isMissingOptionalSidecar(index, result.Err) {
				continue
			}
			return unavailableMeasurement(databasePath, result.Err), result.Err
		}
		total += result.Bytes
		overlapped += result.OverlappedBytes
	}
	return measuredMeasurement(databasePath, total, total-overlapped), nil
}

func (p *Provider) AnalyzeBackups(
	ctx context.Context,
	notify func(filescan.Progress),
	additionalRoots ...string,
) (Measurement, error) {
	if !p.localSQLite() {
		return notApplicableMeasurement(), nil
	}
	if err := ctx.Err(); err != nil {
		return unavailableMeasurement("", err), err
	}
	databasePath, err := p.resolvedDatabasePath()
	if err != nil {
		return unavailableMeasurement(p.databasePath, err), err
	}
	backupPath, err := resolvePathAllowMissing(filepath.Join(filepath.Dir(databasePath), "backups"))
	if err != nil {
		return unavailableMeasurement(filepath.Join(filepath.Dir(databasePath), "backups"), err), err
	}
	info, statErr := os.Lstat(backupPath)
	if errors.Is(statErr, os.ErrNotExist) {
		return measuredMeasurement(backupPath, 0, 0), nil
	}
	if statErr != nil {
		return unavailableMeasurement(backupPath, statErr), statErr
	}
	if info.Mode()&os.ModeSymlink != 0 {
		err := fmt.Errorf("database backup directory is a symlink: %s", backupPath)
		return unavailableMeasurement(backupPath, err), err
	}
	if !info.IsDir() {
		err := fmt.Errorf("database backup path is not a directory: %s", backupPath)
		return unavailableMeasurement(backupPath, err), err
	}

	var warningPaths []string
	var warningMu sync.Mutex
	databasePaths := make(map[string]struct{}, len(sidecarPaths(databasePath))+1)
	databasePaths[databasePath] = struct{}{}
	for _, sidecar := range sidecarPaths(databasePath) {
		databasePaths[sidecar] = struct{}{}
	}
	root := filescan.Root{
		Path: backupPath, MissingOK: true, SymlinkPolicy: filescan.SkipSymlinks,
		OverlapRoots: p.existingRootsFor(additionalRoots),
		Exclude: func(path string, entry fs.DirEntry) bool {
			cleanPath := filepath.Clean(path)
			if _, overlaps := databasePaths[cleanPath]; overlaps {
				return true
			}
			if entry.Type()&os.ModeSymlink != 0 {
				warningMu.Lock()
				warningPaths = append(warningPaths, cleanPath)
				warningMu.Unlock()
				return true
			}
			return false
		},
	}
	results := p.scanner().Measure(ctx, []filescan.Root{root}, notify)
	if len(results) != 1 {
		err := errors.New("database backup scanner returned an invalid result")
		return unavailableMeasurement(backupPath, err), err
	}
	if results[0].Err != nil {
		return unavailableMeasurement(backupPath, results[0].Err), results[0].Err
	}
	measurement := measuredMeasurement(
		backupPath, results[0].Bytes, results[0].Bytes-results[0].OverlappedBytes,
	)
	warningMu.Lock()
	if len(warningPaths) > 0 {
		sort.Strings(warningPaths)
		measurement.Warning = "skipped symlink entries: " + strings.Join(warningPaths, ", ")
	}
	warningMu.Unlock()
	return measurement, nil
}

func (p *Provider) localSQLite() bool {
	return p != nil && p.driver == "sqlite"
}

func (p *Provider) scanner() *filescan.Limiter {
	if p != nil && p.limiter != nil {
		return p.limiter
	}
	return filescan.NewLimiter(4)
}

func (p *Provider) resolvedDatabasePath() (string, error) {
	if p == nil || p.databasePath == "" {
		return "", errors.New("database path is empty")
	}
	return resolvePathAllowMissing(p.databasePath)
}

func measuredMeasurement(path string, bytes int64, countedBytes int64) Measurement {
	if bytes < 0 {
		bytes = 0
	}
	if countedBytes < 0 {
		countedBytes = 0
	}
	if countedBytes > bytes {
		countedBytes = bytes
	}
	measurement := Measurement{
		Status:           StatusMeasured,
		SizeBytes:        int64Pointer(bytes),
		CountedSizeBytes: int64Pointer(countedBytes),
		Path:             filepath.Clean(path),
		IncludedInTotal:  true,
	}
	if bytes > 0 && countedBytes == 0 {
		measurement.IncludedInTotal = false
		measurement.Reason = ReasonOverlapsExistingSource
	} else if countedBytes < bytes {
		measurement.Reason = ReasonPartiallyOverlapsExistingSource
	}
	return measurement
}

func (p *Provider) existingRootsFor(additionalRoots []string) []string {
	if p == nil {
		return normalizePaths(additionalRoots)
	}
	roots := append([]string(nil), p.existingRoots...)
	return append(roots, normalizePaths(additionalRoots)...)
}

func isMissingOptionalSidecar(index int, err error) bool {
	return index > 0 && errors.Is(err, os.ErrNotExist)
}

func notApplicableMeasurement() Measurement {
	return Measurement{Status: StatusNotApplicable, Reason: ReasonUnsupportedDriver}
}

func unavailableMeasurement(path string, err error) Measurement {
	measurement := Measurement{
		Status: StatusUnavailable, Reason: ReasonMeasurementFailed,
	}
	if path != "" {
		measurement.Path = filepath.Clean(path)
	}
	if err != nil {
		measurement.Warning = err.Error()
	}
	return measurement
}

func sidecarPaths(databasePath string) []string {
	return []string{databasePath + "-wal", databasePath + "-shm", databasePath + "-journal"}
}

func validateRequiredFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("database is a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("database is not a regular file: %s", path)
	}
	return nil
}

func resolvePathAllowMissing(path string) (string, error) {
	path = absoluteCleanPath(path)
	if path == "" {
		return "", errors.New("database path is empty")
	}
	parent, parentErr := filepath.EvalSymlinks(filepath.Dir(path))
	if parentErr == nil {
		return filepath.Join(parent, filepath.Base(path)), nil
	}
	if errors.Is(parentErr, os.ErrNotExist) {
		return path, nil
	}
	return "", parentErr
}

func absoluteCleanPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func normalizePaths(paths []string) []string {
	normalized := make([]string, 0, len(paths))
	for _, path := range paths {
		if clean := absoluteCleanPath(path); clean != "" {
			if resolved, err := resolvePathAllowMissing(clean); err == nil {
				clean = resolved
			}
			normalized = append(normalized, clean)
		}
	}
	return normalized
}

func int64Pointer(value int64) *int64 {
	return &value
}
