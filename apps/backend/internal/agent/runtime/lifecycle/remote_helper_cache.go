package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	remoteHelperCacheActiveDir     = ".active"
	remoteHelperCacheLockFile      = ".cache.lock"
	remoteHelperCacheLeaseGrace    = 15 * time.Second
	remoteHelperCacheLeaseFallback = time.Hour
)

type remoteHelperCacheLeaseRecord struct {
	Path      string    `json:"path"`
	ExpiresAt time.Time `json:"expires_at"`
}

type remoteHelperCacheLease struct {
	cacheRoot string
	marker    string
	once      sync.Once
	err       error
}

func pinRemoteHelperCachePath(cacheRoot, helperPath string, ctx context.Context) (*remoteHelperCacheLease, error) {
	cacheRoot, err := filepath.Abs(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve remote helper cache root: %w", err)
	}
	helperPath, err = filepath.Abs(helperPath)
	if err != nil {
		return nil, fmt.Errorf("resolve pinned remote helper path: %w", err)
	}
	if !remoteHelperCachePathWithin(cacheRoot, helperPath) {
		return nil, errors.New("pinned remote helper path is outside the cache")
	}

	lock, err := lockRemoteHelperCache(cacheRoot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unlockRemoteHelperCache(lock) }()

	activeDir := filepath.Join(cacheRoot, remoteHelperCacheActiveDir)
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create remote helper cache lease directory: %w", err)
	}
	temporary, err := os.CreateTemp(activeDir, "lease-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create remote helper cache lease: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	expiresAt := time.Now().Add(remoteHelperCacheLeaseFallback)
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			expiresAt = deadline.Add(remoteHelperCacheLeaseGrace)
		}
	}
	data, err := json.Marshal(remoteHelperCacheLeaseRecord{Path: helperPath, ExpiresAt: expiresAt})
	if err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("encode remote helper cache lease: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("write remote helper cache lease: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("sync remote helper cache lease: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close remote helper cache lease: %w", err)
	}
	markerPath := strings.TrimSuffix(temporaryPath, ".tmp") + ".json"
	if err := os.Rename(temporaryPath, markerPath); err != nil {
		return nil, fmt.Errorf("publish remote helper cache lease: %w", err)
	}
	return &remoteHelperCacheLease{cacheRoot: cacheRoot, marker: markerPath}, nil
}

func (lease *remoteHelperCacheLease) Release() error {
	lease.once.Do(func() {
		lock, err := lockRemoteHelperCache(lease.cacheRoot)
		if err != nil {
			lease.err = err
			return
		}
		if err := os.Remove(lease.marker); err != nil && !errors.Is(err, os.ErrNotExist) {
			lease.err = fmt.Errorf("remove remote helper cache lease: %w", err)
		}
		if err := unlockRemoteHelperCache(lock); err != nil {
			lease.err = errors.Join(lease.err, err)
		}
	})
	return lease.err
}

func remoteHelperCachePathWithin(cacheRoot, path string) bool {
	rel, err := filepath.Rel(cacheRoot, path)
	return err == nil && rel != "." && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// PruneRemoteHelperCache retains the running release and its two previous
// versions. Cleanup is skipped unless the caller has positively inventoried
// every managed container that can still reference a helper mount.
func PruneRemoteHelperCache(cacheRoot, currentVersion string, pinnedPaths []string, inventoryComplete bool) ([]string, error) {
	if !inventoryComplete {
		return nil, nil
	}
	if _, err := parseRemoteHelperVersion(currentVersion); err != nil {
		return nil, fmt.Errorf("invalid current remote helper version: %w", err)
	}
	cacheRoot, err := filepath.Abs(cacheRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve remote helper cache root: %w", err)
	}
	lock, err := lockRemoteHelperCache(cacheRoot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unlockRemoteHelperCache(lock) }()
	activePaths, err := readActiveRemoteHelperCacheLeases(cacheRoot, time.Now())
	if err != nil {
		return nil, err
	}
	pinnedPaths = append(pinnedPaths, activePaths...)
	return pruneRemoteHelperCacheUnlocked(cacheRoot, currentVersion, pinnedPaths)
}

func pruneRemoteHelperCacheUnlocked(cacheRoot, currentVersion string, pinnedPaths []string) ([]string, error) {
	versions, err := readRemoteHelperCacheVersions(cacheRoot)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	keep := remoteHelperVersionsToKeep(versions, currentVersion)
	protected, err := absolutePinnedHelperPaths(pinnedPaths)
	if err != nil {
		return nil, err
	}
	return removeUnusedRemoteHelperVersions(cacheRoot, versions, keep, protected)
}

func readActiveRemoteHelperCacheLeases(cacheRoot string, now time.Time) ([]string, error) {
	activeDir := filepath.Join(cacheRoot, remoteHelperCacheActiveDir)
	entries, err := os.ReadDir(activeDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read remote helper cache leases: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path, active, err := readRemoteHelperCacheLease(cacheRoot, activeDir, entry, now)
		if err != nil {
			return nil, err
		}
		if active {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func readRemoteHelperCacheLease(cacheRoot, activeDir string, entry os.DirEntry, now time.Time) (string, bool, error) {
	name := entry.Name()
	if strings.HasSuffix(name, ".tmp") {
		return "", false, nil
	}
	info, err := entry.Info()
	if err != nil {
		return "", false, fmt.Errorf("inspect remote helper cache lease %s: %w", name, err)
	}
	if !strings.HasSuffix(name, ".json") || !info.Mode().IsRegular() {
		return "", false, fmt.Errorf("unexpected remote helper cache lease entry %q", name)
	}
	marker := filepath.Join(activeDir, name)
	data, err := os.ReadFile(marker)
	if err != nil {
		return "", false, fmt.Errorf("read remote helper cache lease %s: %w", name, err)
	}
	var record remoteHelperCacheLeaseRecord
	if err := json.Unmarshal(data, &record); err != nil || record.Path == "" || record.ExpiresAt.IsZero() {
		return "", false, fmt.Errorf("decode remote helper cache lease %s: invalid lease record", name)
	}
	path, err := filepath.Abs(record.Path)
	if err != nil || !remoteHelperCachePathWithin(cacheRoot, path) {
		return "", false, fmt.Errorf("decode remote helper cache lease %s: path is outside the cache", name)
	}
	if now.Before(record.ExpiresAt) {
		return path, true, nil
	}
	if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, fmt.Errorf("remove expired remote helper cache lease %s: %w", name, err)
	}
	return "", false, nil
}

func readRemoteHelperCacheVersions(cacheRoot string) ([]string, error) {
	entries, err := os.ReadDir(cacheRoot)
	if os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("read remote helper cache: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			if _, err := parseRemoteHelperVersion(entry.Name()); err == nil {
				versions = append(versions, entry.Name())
			}
		}
	}
	sort.Slice(versions, func(i, j int) bool { return compareRemoteHelperVersions(versions[i], versions[j]) > 0 })
	return versions, nil
}

func remoteHelperVersionsToKeep(versions []string, currentVersion string) map[string]struct{} {
	keep := map[string]struct{}{currentVersion: {}}
	previous := 0
	for _, version := range versions {
		if compareRemoteHelperVersions(version, currentVersion) >= 0 {
			continue
		}
		if previous == 2 {
			break
		}
		keep[version] = struct{}{}
		previous++
	}
	return keep
}

func absolutePinnedHelperPaths(pinnedPaths []string) ([]string, error) {
	protected := make([]string, 0, len(pinnedPaths))
	for _, path := range pinnedPaths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve pinned helper path: %w", err)
		}
		protected = append(protected, filepath.Clean(absolute))
	}
	return protected, nil
}

func removeUnusedRemoteHelperVersions(cacheRoot string, versions []string, keep map[string]struct{}, protected []string) ([]string, error) {
	removed := make([]string, 0)
	for _, version := range versions {
		if _, ok := keep[version]; ok {
			continue
		}
		versionPath := filepath.Join(cacheRoot, version)
		if cacheVersionContainsPinnedPath(versionPath, protected) {
			continue
		}
		if err := os.RemoveAll(versionPath); err != nil {
			return removed, fmt.Errorf("remove unused remote helper cache version %s: %w", version, err)
		}
		removed = append(removed, version)
	}
	return removed, nil
}

func cacheVersionContainsPinnedPath(versionPath string, pinned []string) bool {
	absoluteVersionPath, err := filepath.Abs(versionPath)
	if err != nil {
		return true
	}
	for _, path := range pinned {
		rel, err := filepath.Rel(absoluteVersionPath, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

type remoteHelperVersion struct {
	major int
	minor int
	patch int
	pre   string
}

func parseRemoteHelperVersion(version string) (remoteHelperVersion, error) {
	version, _ = strings.CutPrefix(version, "v")
	parts := strings.SplitN(version, "-", 2)
	numbers := strings.Split(parts[0], ".")
	if len(numbers) != 3 {
		return remoteHelperVersion{}, fmt.Errorf("version %q is not semantic", version)
	}
	values := [3]int{}
	for i, number := range numbers {
		value, err := strconv.Atoi(number)
		if err != nil || value < 0 {
			return remoteHelperVersion{}, fmt.Errorf("version %q is not semantic", version)
		}
		values[i] = value
	}
	pre := ""
	if len(parts) == 2 {
		pre = parts[1]
		if pre == "" || strings.ContainsAny(pre, `/\\ `) {
			return remoteHelperVersion{}, fmt.Errorf("version %q has invalid prerelease", version)
		}
	}
	return remoteHelperVersion{major: values[0], minor: values[1], patch: values[2], pre: pre}, nil
}

func compareRemoteHelperVersions(left, right string) int {
	l, lerr := parseRemoteHelperVersion(left)
	r, rerr := parseRemoteHelperVersion(right)
	if lerr != nil || rerr != nil {
		return strings.Compare(left, right)
	}
	for _, pair := range [][2]int{{l.major, r.major}, {l.minor, r.minor}, {l.patch, r.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if l.pre == r.pre {
		return 0
	}
	if l.pre == "" {
		return 1
	}
	if r.pre == "" {
		return -1
	}
	return strings.Compare(l.pre, r.pre)
}
