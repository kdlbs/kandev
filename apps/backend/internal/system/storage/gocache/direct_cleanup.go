package gocache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/system/metrics"
)

const (
	cleanupDeadline    = 30 * time.Second
	cleanupBatchSize   = 1000
	cleanupEntryLimit  = 100_000
	cleanupIssueLimit  = 8
	cleanupMarkerLimit = len(markerContent) + 1
	cleanupMaxDepth    = 128
	fuzzDirectoryName  = "fuzz"
)

type cleanupSnapshot struct {
	entries      []cleanupEntry
	directories  map[string]os.FileInfo
	bytes        int64
	examined     int
	issues       []string
	limitReached bool
}

type cleanupEntry struct {
	relative string
	info     os.FileInfo
	bytes    int64
}

func (p *Provider) cleanupContents(
	ctx context.Context,
	cachePath string,
	adopted bool,
	maxBytes int64,
) (CleanupResult, error) {
	return p.cleanupContentsWithLimit(ctx, cachePath, adopted, maxBytes, cleanupEntryLimit, cacheFilesystemIdentity)
}

func openOwnedCacheRoot(cachePath string, adopted bool) (*os.Root, os.FileInfo, string, error) {
	return openOwnedCacheRootWithIdentity(cachePath, adopted, cacheFilesystemIdentity)
}

func openOwnedCacheRootWithIdentity(
	cachePath string,
	adopted bool,
	identity filesystemIdentityFunc,
) (*os.Root, os.FileInfo, string, error) {
	pathInfo, err := os.Lstat(cachePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("inspect Go-cache root: %w", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.IsDir() {
		return nil, nil, "", errors.New("go-cache root must be a directory, not a symlink")
	}
	if !adopted && !hasValidMarker(cachePath) {
		return nil, nil, "", ErrNotOwned
	}
	root, err := os.OpenRoot(cachePath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open Go-cache root: %w", err)
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, nil, "", fmt.Errorf("inspect opened Go-cache root: %w", err)
	}
	if !os.SameFile(pathInfo, rootInfo) {
		_ = root.Close()
		return nil, nil, "", errors.New("go-cache root changed while opening")
	}
	if err := validateOwnedMarker(root, adopted); err != nil {
		_ = root.Close()
		return nil, nil, "", err
	}
	filesystem, err := identity(cachePath)
	if err != nil {
		_ = root.Close()
		return nil, nil, "", fmt.Errorf("identify Go-cache filesystem: %w", err)
	}
	return root, rootInfo, filesystem, nil
}

func validateOwnedMarker(root *os.Root, adopted bool) error {
	info, err := root.Lstat(markerName)
	if adopted && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotOwned
		}
		return fmt.Errorf("inspect Go-cache ownership marker: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrNotOwned
	}
	marker, err := root.Open(markerName)
	if err != nil {
		return fmt.Errorf("open Go-cache ownership marker: %w", err)
	}
	defer func() { _ = marker.Close() }()
	contents, err := io.ReadAll(io.LimitReader(marker, int64(cleanupMarkerLimit)))
	if err != nil {
		return fmt.Errorf("read Go-cache ownership marker: %w", err)
	}
	if !adopted && string(contents) != markerContent {
		return ErrNotOwned
	}
	return nil
}

func scanCache(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	entryLimit int,
) cleanupSnapshot {
	return scanCacheWithIdentity(ctx, root, cachePath, filesystem, entryLimit, cacheFilesystemIdentity)
}

func scanCacheWithIdentity(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	entryLimit int,
	identity filesystemIdentityFunc,
) cleanupSnapshot {
	snapshot := cleanupSnapshot{directories: make(map[string]os.FileInfo)}
	scanCacheDirectory(ctx, root, cachePath, filesystem, "", entryLimit, identity, &snapshot)
	return snapshot
}

func scanCacheDirectory(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	directory string,
	entryLimit int,
	identity filesystemIdentityFunc,
	snapshot *cleanupSnapshot,
) {
	if ctx.Err() != nil {
		addCleanupIssue(snapshot, cancellationIssue(ctx.Err()))
		return
	}
	directoryRoot, closeRoot, ok := openScanDirectory(root, directory, snapshot)
	if !ok {
		return
	}
	if closeRoot != nil {
		defer closeRoot()
	}
	file, err := directoryRoot.Open(".")
	if err != nil {
		addCleanupIssue(snapshot, "directory_read_failed")
		return
	}
	defer func() { _ = file.Close() }()
	for {
		batch, readErr := file.ReadDir(cleanupBatchSize)
		if !scanCacheEntries(ctx, root, cachePath, filesystem, directory, entryLimit, identity, batch, snapshot) {
			return
		}
		if errors.Is(readErr, io.EOF) {
			return
		}
		if readErr != nil {
			addCleanupIssue(snapshot, "directory_read_failed")
			return
		}
	}
}

func openScanDirectory(root *os.Root, directory string, snapshot *cleanupSnapshot) (*os.Root, func(), bool) {
	if directory == "" {
		return root, nil, true
	}
	opened, err := root.OpenRoot(directory)
	if err != nil {
		addCleanupIssue(snapshot, "directory_open_failed")
		return nil, nil, false
	}
	openedInfo, err := opened.Stat(".")
	if err != nil || snapshot.directories[directory] == nil || !os.SameFile(snapshot.directories[directory], openedInfo) {
		_ = opened.Close()
		addCleanupIssue(snapshot, "path_changed_or_unsafe")
		return nil, nil, false
	}
	return opened, func() { _ = opened.Close() }, true
}

func scanCacheEntries(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	directory string,
	entryLimit int,
	identity filesystemIdentityFunc,
	entries []os.DirEntry,
	snapshot *cleanupSnapshot,
) bool {
	for _, entry := range entries {
		if snapshot.limitReached {
			return false
		}
		if err := ctx.Err(); err != nil {
			addCleanupIssue(snapshot, cancellationIssue(err))
			return false
		}
		if snapshot.examined >= entryLimit {
			addCleanupIssue(snapshot, "entry_limit_reached")
			snapshot.limitReached = true
			return false
		}
		snapshot.examined++
		scanCacheEntry(ctx, root, cachePath, filesystem, directory, entryLimit, identity, entry, snapshot)
	}
	return true
}

func scanCacheEntry(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	directory string,
	entryLimit int,
	identity filesystemIdentityFunc,
	entry os.DirEntry,
	snapshot *cleanupSnapshot,
) {
	relative := filepath.Join(directory, entry.Name())
	if relative == markerName || (directory == "" && entry.Name() == fuzzDirectoryName) {
		return
	}
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		addCleanupIssue(snapshot, "entry_read_failed")
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		addCleanupIssue(snapshot, "symlink_skipped")
		return
	}
	entryIdentity, err := identity(filepath.Join(cachePath, relative))
	if err != nil || entryIdentity != filesystem {
		addCleanupIssue(snapshot, "filesystem_boundary_skipped")
		return
	}
	if info.IsDir() {
		snapshot.directories[relative] = info
		scanCacheDirectory(ctx, root, cachePath, filesystem, relative, entryLimit, identity, snapshot)
		snapshot.entries = append(snapshot.entries, cleanupEntry{relative: relative, info: info})
		return
	}
	if !info.Mode().IsRegular() {
		addCleanupIssue(snapshot, "unsupported_entry_skipped")
		return
	}
	snapshot.bytes = saturatingAdd(snapshot.bytes, info.Size())
	snapshot.entries = append(snapshot.entries, cleanupEntry{relative: relative, info: info, bytes: info.Size()})
}

func verifyCacheRoot(root *os.Root, cachePath string, expected os.FileInfo, adopted bool) error {
	return verifyCacheRootWithIdentity(root, cachePath, expected, "", adopted, nil)
}

func verifyCacheRootWithIdentity(
	root *os.Root,
	cachePath string,
	expected os.FileInfo,
	filesystem string,
	adopted bool,
	identity filesystemIdentityFunc,
) error {
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Lstat(cachePath)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(expected, opened) || !os.SameFile(expected, current) {
		return errors.New("go-cache root was replaced")
	}
	if filesystem != "" && identity != nil {
		currentIdentity, err := identity(cachePath)
		if err != nil {
			return err
		}
		if currentIdentity != filesystem {
			return errors.New("go-cache root mount changed")
		}
	}
	return validateOwnedMarker(root, adopted)
}

func verifySnapshotPathWithIdentity(
	root *os.Root,
	cachePath string,
	filesystem string,
	relative string,
	expected os.FileInfo,
	directories map[string]os.FileInfo,
	identity filesystemIdentityFunc,
) error {
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	current := ""
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("go-cache path contains a symlink")
		}
		want := expected
		if index < len(parts)-1 {
			want = directories[current]
		}
		if want == nil || !os.SameFile(want, info) {
			return errors.New("go-cache path changed after scan")
		}
		currentIdentity, err := identity(filepath.Join(cachePath, current))
		if err != nil {
			return err
		}
		if currentIdentity != filesystem {
			return errors.New("go-cache mount boundary changed after scan")
		}
		if index == len(parts)-1 && expected.Mode().IsRegular() && expected.Size() != info.Size() {
			return errors.New("go-cache file changed after scan")
		}
		if index < len(parts)-1 && !info.IsDir() {
			return errors.New("go-cache parent changed from a directory")
		}
	}
	return nil
}

type filesystemIdentityFunc func(string) (string, error)

func cacheFilesystemIdentity(path string) (string, error) {
	filesystem, err := metrics.FilesystemIdentity(path)
	if err != nil {
		return "", err
	}
	mount, err := cacheMountIdentity(path)
	if err != nil {
		return "", err
	}
	return filesystem + "\x00" + mount, nil
}

func addCleanupIssue(snapshot *cleanupSnapshot, issue string) {
	if len(snapshot.issues) < cleanupIssueLimit {
		snapshot.issues = append(snapshot.issues, issue)
	}
}

func boundedCleanupIssues(issues []string) []string {
	if len(issues) > cleanupIssueLimit {
		return issues[:cleanupIssueLimit]
	}
	return issues
}

func cancellationIssue(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "cleanup_deadline_reached"
	}
	return "cleanup_cancelled"
}

func newInt64(value int64) *int64 {
	return &value
}
