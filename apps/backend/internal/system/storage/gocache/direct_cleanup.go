package gocache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/system/metrics"
	"github.com/kandev/kandev/internal/system/storage"
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

type cacheRootHandle struct {
	anchor               *os.Root
	anchorPath           string
	relative             string
	path                 string
	root                 *os.Root
	rootInfo             os.FileInfo
	filesystem           string
	descriptorFilesystem string
	descriptorIdentity   func(*os.File) (string, error)
}

func (h *cacheRootHandle) close() {
	if h == nil {
		return
	}
	if h.root != nil {
		_ = h.root.Close()
	}
	if h.anchor != nil {
		_ = h.anchor.Close()
	}
}

func (p *Provider) cleanupContents(
	ctx context.Context,
	cachePath string,
	adopted bool,
	maxBytes int64,
) (CleanupResult, error) {
	return p.cleanupContentsWithLimit(ctx, cachePath, adopted, maxBytes, cleanupEntryLimit, cacheFilesystemIdentity)
}

func (p *Provider) openValidatedCacheRoot(
	cachePath string,
	adopted bool,
	identity filesystemIdentityFunc,
) (*cacheRootHandle, error) {
	anchorPath, err := storage.CommonPath(p.config.HomeDir, cachePath)
	if err != nil {
		return nil, err
	}
	return openOwnedCacheRootAtAnchor(anchorPath, cachePath, adopted, identity)
}

func openOwnedCacheRoot(cachePath string, adopted bool) (*os.Root, os.FileInfo, string, error) {
	handle, err := openOwnedCacheRootAtAnchor(filepath.Dir(cachePath), cachePath, adopted, cacheFilesystemIdentity)
	if err != nil {
		return nil, nil, "", err
	}
	_ = handle.anchor.Close()
	return handle.root, handle.rootInfo, handle.filesystem, nil
}

func openOwnedCacheRootAtAnchor(
	anchorPath string,
	cachePath string,
	adopted bool,
	identity filesystemIdentityFunc,
) (handle *cacheRootHandle, err error) {
	if identity == nil {
		identity = cacheFilesystemIdentity
	}
	anchorPath = filepath.Clean(anchorPath)
	cachePath = filepath.Clean(cachePath)
	relative, err := cacheRootRelativePath(anchorPath, cachePath)
	if err != nil {
		return nil, err
	}
	anchor, err := openVerifiedSafetyAnchor(anchorPath)
	if err != nil {
		return nil, err
	}
	var root *os.Root
	defer func() {
		if err != nil {
			_ = anchor.Close()
			if root != nil {
				_ = root.Close()
			}
		}
	}()
	root, rootInfo, err := openRootBelowAnchor(anchor, relative)
	if err != nil {
		return nil, fmt.Errorf("open Go-cache root below safety anchor: %w", err)
	}
	if err := validateOwnedMarker(root, adopted); err != nil {
		return nil, err
	}
	filesystem, err := identity(cachePath)
	if err != nil {
		return nil, fmt.Errorf("identify Go-cache filesystem: %w", err)
	}
	descriptorFilesystem, err := cacheFilesystemIdentityFromRoot(root)
	if err != nil {
		return nil, fmt.Errorf("identify opened Go-cache filesystem: %w", err)
	}
	opened, openedInfo, err := openRootBelowAnchor(anchor, relative)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(rootInfo, openedInfo) {
		_ = opened.Close()
		return nil, errors.New("go-cache path changed while opening")
	}
	_ = opened.Close()
	return &cacheRootHandle{
		anchor: anchor, anchorPath: anchorPath, relative: relative, path: cachePath,
		root: root, rootInfo: rootInfo, filesystem: filesystem,
		descriptorFilesystem: descriptorFilesystem, descriptorIdentity: cacheFilesystemIdentityFromFile,
	}, nil
}

func cacheRootRelativePath(anchorPath, cachePath string) (string, error) {
	relative, err := filepath.Rel(anchorPath, cachePath)
	if err != nil {
		return "", fmt.Errorf("relate Go-cache root to safety anchor: %w", err)
	}
	if relative == "." || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("go-cache root must be below its safety anchor")
	}
	return relative, nil
}

func openVerifiedSafetyAnchor(anchorPath string) (*os.Root, error) {
	anchor, err := os.OpenRoot(anchorPath)
	if err != nil {
		return nil, fmt.Errorf("open Go-cache safety anchor: %w", err)
	}
	anchorInfo, err := anchor.Stat(".")
	if err != nil {
		_ = anchor.Close()
		return nil, fmt.Errorf("inspect opened Go-cache safety anchor: %w", err)
	}
	anchorPathInfo, err := os.Lstat(anchorPath)
	if err == nil && anchorPathInfo.Mode()&os.ModeSymlink == 0 && os.SameFile(anchorInfo, anchorPathInfo) {
		return anchor, nil
	}
	_ = anchor.Close()
	if err == nil {
		err = errors.New("go-cache safety anchor changed while opening")
	}
	return nil, err
}

func verifyAnchoredCacheRoot(handle *cacheRootHandle, adopted bool, identity filesystemIdentityFunc) error {
	if handle == nil || handle.anchor == nil || handle.root == nil {
		return errors.New("go-cache root handle is unavailable")
	}
	if err := verifySafetyAnchor(handle); err != nil {
		return err
	}
	currentRoot, currentInfo, err := openRootBelowAnchor(handle.anchor, handle.relative)
	if err != nil {
		return err
	}
	defer func() { _ = currentRoot.Close() }()
	if err := verifyAnchoredRootIdentity(handle, currentInfo); err != nil {
		return err
	}
	if err := verifyAnchoredRootFilesystem(handle, currentRoot, identity); err != nil {
		return err
	}
	return validateOwnedMarker(handle.root, adopted)
}

func verifySafetyAnchor(handle *cacheRootHandle) error {
	anchorInfo, err := handle.anchor.Stat(".")
	if err != nil {
		return err
	}
	currentAnchor, err := os.Lstat(handle.anchorPath)
	if err != nil {
		return err
	}
	if currentAnchor.Mode()&os.ModeSymlink != 0 || !os.SameFile(anchorInfo, currentAnchor) {
		return errors.New("go-cache safety anchor was replaced")
	}
	openedInfo, err := handle.root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(handle.rootInfo, openedInfo) {
		return errors.New("go-cache root was replaced")
	}
	return nil
}

func verifyAnchoredRootIdentity(handle *cacheRootHandle, currentInfo os.FileInfo) error {
	if !os.SameFile(handle.rootInfo, currentInfo) {
		return errors.New("go-cache root was replaced")
	}
	return nil
}

func verifyAnchoredRootFilesystem(
	handle *cacheRootHandle,
	currentRoot *os.Root,
	identity filesystemIdentityFunc,
) error {
	currentFilesystem, err := cacheFilesystemIdentityFromRootWith(currentRoot, handle.descriptorIdentity)
	if err != nil || currentFilesystem != handle.descriptorFilesystem {
		if err == nil {
			err = errors.New("go-cache root mount changed")
		}
		return err
	}
	if identity != nil {
		currentIdentity, err := identity(handle.path)
		if err != nil {
			return err
		}
		if currentIdentity != handle.filesystem {
			return errors.New("go-cache root mount changed")
		}
	}
	return nil
}

func cacheFilesystemIdentityFromRoot(root *os.Root) (string, error) {
	return cacheFilesystemIdentityFromRootWith(root, cacheFilesystemIdentityFromFile)
}

func cacheFilesystemIdentityFromRootWith(
	root *os.Root,
	identity func(*os.File) (string, error),
) (string, error) {
	if identity == nil {
		return "", errors.New("go-cache descriptor identity is unavailable")
	}
	file, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	return identity(file)
}

func cacheFilesystemIdentityFromFile(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	mount, err := cacheMountIdentityFromFile(file)
	if err != nil {
		return "", err
	}
	value := reflect.ValueOf(info.Sys())
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	device := value.FieldByName("Dev")
	if !device.IsValid() {
		return mount, nil
	}
	switch device.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return "device:" + strconv.FormatUint(device.Uint(), 10) + "\x00" + mount, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "device:" + strconv.FormatInt(device.Int(), 10) + "\x00" + mount, nil
	default:
		return "", errors.New("go-cache filesystem identity is unavailable for opened path")
	}
}

func openRootBelowAnchor(anchor *os.Root, relative string) (*os.Root, os.FileInfo, error) {
	components, err := cachePathComponents(relative)
	if err != nil {
		return nil, nil, err
	}
	var current *os.Root
	parent := anchor
	for _, component := range components {
		next, err := openCachePathComponent(parent, component)
		if err != nil {
			closeOpenedRoot(current)
			return nil, nil, err
		}
		if current != nil {
			_ = current.Close()
		}
		current = next
		parent = current
	}
	if current == nil {
		return nil, nil, errors.New("go-cache root is the safety anchor")
	}
	info, err := current.Stat(".")
	if err != nil {
		_ = current.Close()
		return nil, nil, err
	}
	return current, info, nil
}

func cachePathComponents(relative string) ([]string, error) {
	components := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, errors.New("invalid Go-cache path component")
		}
	}
	return components, nil
}

func openCachePathComponent(parent *os.Root, component string) (*os.Root, error) {
	before, err := parent.Lstat(component)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 || !before.IsDir() {
		return nil, errors.New("go-cache path component must be a real directory")
	}
	next, err := parent.OpenRoot(component)
	if err != nil {
		return nil, err
	}
	if err := verifyOpenedPathComponent(next, before); err != nil {
		_ = next.Close()
		return nil, err
	}
	return next, nil
}

func verifyOpenedPathComponent(opened *os.Root, expected os.FileInfo) error {
	actual, err := opened.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(expected, actual) {
		return errors.New("go-cache path component changed while opening")
	}
	return nil
}

func closeOpenedRoot(root *os.Root) {
	if root != nil {
		_ = root.Close()
	}
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
	rootDescriptor, err := cacheFilesystemIdentityFromRoot(root)
	if err != nil {
		return cleanupSnapshot{issues: []string{"filesystem_boundary_skipped"}}
	}
	return scanCacheWithDescriptorIdentity(
		ctx, root, cachePath, filesystem, entryLimit, identity,
		rootDescriptor, cacheFilesystemIdentityFromFile,
	)
}

func scanCacheWithDescriptorIdentity(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	entryLimit int,
	identity filesystemIdentityFunc,
	rootDescriptor string,
	descriptorIdentity func(*os.File) (string, error),
) cleanupSnapshot {
	if entryLimit <= 0 {
		entryLimit = cleanupEntryLimit
	}
	snapshot := cleanupSnapshot{directories: make(map[string]os.FileInfo)}
	scanCacheDirectory(
		ctx, root, cachePath, filesystem, "", 0, entryLimit,
		identity, rootDescriptor, descriptorIdentity, &snapshot,
	)
	return snapshot
}

func scanCacheDirectory(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	directory string,
	depth int,
	entryLimit int,
	identity filesystemIdentityFunc,
	rootDescriptor string,
	descriptorIdentity func(*os.File) (string, error),
	snapshot *cleanupSnapshot,
) {
	if ctx.Err() != nil {
		addCleanupIssue(snapshot, cancellationIssue(ctx.Err()))
		return
	}
	if depth >= cleanupMaxDepth {
		addCleanupIssue(snapshot, "directory_depth_limit_reached")
		return
	}
	directoryRoot, closeRoot, ok := openScanDirectory(root, directory, rootDescriptor, descriptorIdentity, snapshot)
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
		if !scanCacheEntries(
			ctx, root, cachePath, filesystem, directory, depth, entryLimit,
			identity, rootDescriptor, descriptorIdentity, batch, snapshot,
		) {
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

func openScanDirectory(
	root *os.Root,
	directory string,
	rootDescriptor string,
	descriptorIdentity func(*os.File) (string, error),
	snapshot *cleanupSnapshot,
) (*os.Root, func(), bool) {
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
	openedFile, err := opened.Open(".")
	if err != nil {
		_ = opened.Close()
		addCleanupIssue(snapshot, "directory_open_failed")
		return nil, nil, false
	}
	openedDescriptor, err := descriptorIdentity(openedFile)
	_ = openedFile.Close()
	if err != nil || openedDescriptor != rootDescriptor {
		_ = opened.Close()
		addCleanupIssue(snapshot, "filesystem_boundary_skipped")
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
	depth int,
	entryLimit int,
	identity filesystemIdentityFunc,
	rootDescriptor string,
	descriptorIdentity func(*os.File) (string, error),
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
		if isProtectedCacheEntry(directory, entry.Name()) {
			continue
		}
		if snapshot.examined >= entryLimit {
			addCleanupIssue(snapshot, "entry_limit_reached")
			snapshot.limitReached = true
			return false
		}
		snapshot.examined++
		scanCacheEntry(
			ctx, root, cachePath, filesystem, directory, depth, entryLimit,
			identity, rootDescriptor, descriptorIdentity, entry, snapshot,
		)
	}
	return true
}

func scanCacheEntry(
	ctx context.Context,
	root *os.Root,
	cachePath string,
	filesystem string,
	directory string,
	depth int,
	entryLimit int,
	identity filesystemIdentityFunc,
	rootDescriptor string,
	descriptorIdentity func(*os.File) (string, error),
	entry os.DirEntry,
	snapshot *cleanupSnapshot,
) {
	relative := filepath.Join(directory, entry.Name())
	if isProtectedCacheEntry(directory, entry.Name()) {
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
		scanCacheDirectory(
			ctx, root, cachePath, filesystem, relative, depth+1, entryLimit,
			identity, rootDescriptor, descriptorIdentity, snapshot,
		)
		snapshot.entries = append(snapshot.entries, cleanupEntry{relative: relative, info: info})
		return
	}
	if !info.Mode().IsRegular() {
		addCleanupIssue(snapshot, "unsupported_entry_skipped")
		return
	}
	opened, err := root.Open(relative)
	if err != nil {
		addCleanupIssue(snapshot, "entry_read_failed")
		return
	}
	openedInfo, statErr := opened.Stat()
	openedDescriptor, descriptorErr := descriptorIdentity(opened)
	_ = opened.Close()
	if statErr != nil || !os.SameFile(info, openedInfo) {
		addCleanupIssue(snapshot, "path_changed_or_unsafe")
		return
	}
	if descriptorErr != nil || openedDescriptor != rootDescriptor {
		addCleanupIssue(snapshot, "filesystem_boundary_skipped")
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
