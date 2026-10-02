package gocache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type cleanupPhase uint8

const (
	cleanupDiscover cleanupPhase = iota
	cleanupDelete
)

type cleanupSession struct {
	cachePath         string
	adopted           bool
	maxBytes          int64
	identityKey       string
	identity          filesystemIdentityFunc
	root              *os.Root
	rootInfo          os.FileInfo
	rootHandle        *cacheRootHandle
	filesystem        string
	stack             []*cleanupFrame
	directories       map[string]os.FileInfo
	entries           []cleanupEntry
	nextDelete        int
	bytes             int64
	phase             cleanupPhase
	traversalDone     bool
	discoveryComplete bool
	issues            []string
	cause             error
}

type cleanupFrame struct {
	relative   string
	info       os.FileInfo
	reader     *os.File
	pending    []os.DirEntry
	pendingAt  int
	pendingErr error
}

func (p *Provider) cleanupContentsWithLimit(
	ctx context.Context,
	cachePath string,
	adopted bool,
	maxBytes int64,
	entryLimit int,
	identity filesystemIdentityFunc,
) (CleanupResult, error) {
	return p.cleanupContentsWithOptions(ctx, cachePath, adopted, maxBytes, entryLimit, "filesystem", identity)
}

func (p *Provider) cleanupContentsWithOptions(
	ctx context.Context,
	cachePath string,
	adopted bool,
	maxBytes int64,
	entryLimit int,
	identityKey string,
	identity filesystemIdentityFunc,
) (CleanupResult, error) {
	result := CleanupResult{Path: cachePath}
	if entryLimit <= 0 {
		entryLimit = cleanupEntryLimit
	}
	if identity == nil {
		identity = cacheFilesystemIdentity
	}
	cachePath = filepath.Clean(cachePath)
	rootHandle, err := p.openValidatedCacheRoot(cachePath, adopted, identity)
	if err != nil {
		return result, err
	}
	return p.cleanupContentsWithPreparedRoot(ctx, cachePath, adopted, maxBytes, entryLimit, identityKey, identity, rootHandle)
}

func (p *Provider) cleanupContentsWithPreparedRoot(
	ctx context.Context,
	cachePath string,
	adopted bool,
	maxBytes int64,
	entryLimit int,
	identityKey string,
	identity filesystemIdentityFunc,
	rootHandle *cacheRootHandle,
) (CleanupResult, error) {
	result := CleanupResult{Path: cachePath}
	if entryLimit <= 0 {
		entryLimit = cleanupEntryLimit
	}
	if identity == nil {
		identity = cacheFilesystemIdentity
	}
	cachePath = filepath.Clean(cachePath)
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()

	session, result, err := p.prepareCleanupSession(
		result, cachePath, adopted, maxBytes, identityKey, identity, rootHandle,
	)
	if err != nil {
		return result, err
	}
	budget := &cleanupEntryBudget{remaining: entryLimit}
	result, handled, passError, err := p.discoverCleanup(ctx, session, budget, result)
	if handled {
		return result, err
	}
	return p.deleteCleanupPass(ctx, session, cachePath, budget, result, passError)
}

type cleanupEntryBudget struct {
	remaining int
}

func (b *cleanupEntryBudget) consume(count int) {
	b.remaining -= count
	if b.remaining < 0 {
		b.remaining = 0
	}
}

func (p *Provider) prepareCleanupSession(
	result CleanupResult,
	cachePath string,
	adopted bool,
	maxBytes int64,
	identityKey string,
	identity filesystemIdentityFunc,
	rootHandle *cacheRootHandle,
) (*cleanupSession, CleanupResult, error) {
	session := p.cleanupState
	if session != nil && !session.matches(cachePath, adopted, maxBytes, identityKey) {
		session.close()
		p.cleanupState = nil
		session = nil
	}
	if session != nil {
		rootHandle.close()
		if err := session.verifyRoot(); err != nil {
			session.close()
			p.cleanupState = nil
			result.Partial = true
			result.Errors = []string{"cache_root_changed"}
			return nil, result, err
		}
	}
	if session == nil {
		var err error
		session, err = newCleanupSessionWithRoot(
			cachePath, adopted, maxBytes, identityKey, identity, rootHandle,
		)
		if err != nil {
			return nil, result, err
		}
		p.cleanupState = session
	}
	return session, result, nil
}

func (p *Provider) deleteCleanupPass(
	ctx context.Context,
	session *cleanupSession,
	cachePath string,
	budget *cleanupEntryBudget,
	result CleanupResult,
	passError error,
) (CleanupResult, error) {
	removed, deleteErr := session.deleteCandidates(ctx, budget)
	result.ReclaimedBytes += removed
	if deleteErr != nil {
		passError = errors.Join(passError, deleteErr)
	}
	if ctx.Err() == nil && session.nextDelete == len(session.entries) && !session.traversalDone {
		removed, passError = session.deleteRemainder(ctx, budget)
		result.ReclaimedBytes += removed
	}
	result.BytesBefore = session.bytes
	result.BytesBeforeComplete = session.discoveryComplete
	result.Errors = append([]string(nil), session.issues...)
	if !session.traversalDone || session.nextDelete != len(session.entries) {
		result.Partial = true
		result.Reason = "cleanup_in_progress"
		result.Errors = boundedCleanupIssues(append(result.Errors, session.errorsForPass(passError, "entry_limit_reached")...))
		return result, session.failureForPass(passError, "entry_limit_reached")
	}
	return p.finishCompletedCleanup(ctx, session, cachePath, budget, result, passError)
}

func (p *Provider) finishCompletedCleanup(
	ctx context.Context,
	session *cleanupSession,
	cachePath string,
	budget *cleanupEntryBudget,
	result CleanupResult,
	passError error,
) (CleanupResult, error) {
	remaining := p.measureRemainingCleanup(ctx, session, cachePath, budget)
	if len(remaining.issues) == 0 {
		result.BytesAfter = newInt64(remaining.bytes)
	} else {
		result.Errors = append(result.Errors, remaining.issues...)
	}
	result.Errors = boundedCleanupIssues(result.Errors)
	result.Partial = len(result.Errors) > 0 || passError != nil || ctx.Err() != nil
	if result.Partial && result.BytesAfter == nil {
		result.Reason = "measurement_incomplete"
	}
	p.finishCleanupSession(session)
	if result.Partial {
		return result, errors.Join(session.failure(), passError, issueError(result.Errors))
	}
	return result, nil
}

func (p *Provider) measureRemainingCleanup(
	ctx context.Context,
	session *cleanupSession,
	cachePath string,
	budget *cleanupEntryBudget,
) cleanupSnapshot {
	if budget.remaining == 0 {
		return cleanupSnapshot{issues: []string{"entry_limit_reached"}}
	}
	remaining := scanCacheWithDescriptorIdentity(
		ctx, session.root, cachePath, session.filesystem, budget.remaining, session.identity,
		session.rootHandle.descriptorFilesystem, session.rootHandle.descriptorIdentity,
	)
	budget.consume(remaining.examined)
	return remaining
}

func (p *Provider) discoverCleanup(
	ctx context.Context,
	session *cleanupSession,
	budget *cleanupEntryBudget,
	result CleanupResult,
) (CleanupResult, bool, error, error) {
	passError := ctx.Err()
	if passError == nil && session.phase == cleanupDiscover && budget.remaining > 0 {
		processed, discoverErr := session.discover(ctx, budget.remaining)
		budget.consume(processed)
		passError = discoverErr
	}
	result.BytesBefore = session.bytes
	result.BytesBeforeComplete = session.discoveryComplete
	if session.phase != cleanupDiscover {
		return result, false, passError, nil
	}
	if !session.traversalDone && session.phase == cleanupDiscover {
		result.Skipped = true
		result.Partial = true
		result.Reason = "incomplete_scan"
		result.Errors = session.errorsForPass(passError, "entry_limit_reached")
		return result, true, nil, session.failureForPass(passError, "entry_limit_reached")
	}
	result.Partial = len(session.issues) > 0
	if !result.Partial {
		result.BytesAfter = newInt64(session.bytes)
	}
	p.finishCleanupSession(session)
	if result.Partial {
		result.Skipped = true
		result.Reason = "incomplete_scan"
		result.Errors = append([]string(nil), session.issues...)
		return result, true, nil, session.failure()
	}
	return result, true, nil, nil
}

func newCleanupSessionWithRoot(
	cachePath string,
	adopted bool,
	maxBytes int64,
	identityKey string,
	identity filesystemIdentityFunc,
	rootHandle *cacheRootHandle,
) (*cleanupSession, error) {
	reader, err := rootHandle.root.Open(".")
	if err != nil {
		rootHandle.close()
		return nil, fmt.Errorf("open Go-cache root for cleanup: %w", err)
	}
	session := &cleanupSession{
		cachePath: cachePath, adopted: adopted, maxBytes: maxBytes, identityKey: identityKey,
		identity: identity, root: rootHandle.root, rootInfo: rootHandle.rootInfo,
		rootHandle: rootHandle, filesystem: rootHandle.filesystem,
		directories: make(map[string]os.FileInfo),
		stack:       []*cleanupFrame{{relative: "", info: rootHandle.rootInfo, reader: reader}},
	}
	return session, nil
}

func (s *cleanupSession) matches(cachePath string, adopted bool, maxBytes int64, identityKey string) bool {
	return s.cachePath == cachePath && s.adopted == adopted && s.maxBytes == maxBytes && s.identityKey == identityKey
}

func (s *cleanupSession) verifyRoot() error {
	return verifyAnchoredCacheRoot(s.rootHandle, s.adopted, s.identity)
}

func (s *cleanupSession) close() {
	for _, frame := range s.stack {
		_ = frame.reader.Close()
	}
	if s.rootHandle != nil {
		s.rootHandle.close()
	}
}

func (p *Provider) finishCleanupSession(session *cleanupSession) {
	session.close()
	if p.cleanupState == session {
		p.cleanupState = nil
	}
}

func (s *cleanupSession) discover(ctx context.Context, entryLimit int) (int, error) {
	processed := 0
	for processed < entryLimit && len(s.stack) > 0 {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		frame := s.stack[len(s.stack)-1]
		entry, err := frame.nextEntry()
		if errors.Is(err, io.EOF) {
			s.popFrame(frame, false, ctx)
			continue
		}
		if err != nil {
			s.addIssue("directory_read_failed", err)
			s.popFrame(frame, false, ctx)
			continue
		}
		if isProtectedCacheEntry(frame.relative, entry.Name()) {
			continue
		}
		processed++
		if s.discoverEntry(frame, entry) {
			return processed, nil
		}
	}
	if len(s.stack) == 0 {
		s.markTraversalDone()
	}
	return processed, nil
}

func (s *cleanupSession) discoverEntry(frame *cleanupFrame, entry os.DirEntry) bool {
	relative, info, ok := s.inspectFrameEntry(frame, entry)
	if !ok {
		return false
	}
	if info.IsDir() {
		s.pushDirectory(relative, info)
		return false
	}
	if !info.Mode().IsRegular() {
		s.addIssue("unsupported_entry_skipped", errors.New("unsupported Go-cache entry was preserved"))
		return false
	}
	s.bytes = saturatingAdd(s.bytes, info.Size())
	s.entries = append(s.entries, cleanupEntry{relative: relative, info: info, bytes: info.Size()})
	if s.bytes <= s.maxBytes {
		return false
	}
	s.phase = cleanupDelete
	return true
}

func (s *cleanupSession) inspectFrameEntry(frame *cleanupFrame, entry os.DirEntry) (string, os.FileInfo, bool) {
	relative := entry.Name()
	if frame.relative != "" {
		relative = filepath.Join(frame.relative, relative)
	}
	if relative == markerName || (frame.relative == "" && entry.Name() == fuzzDirectoryName) {
		return relative, nil, false
	}
	info, err := s.inspectEntry(relative)
	return relative, info, err == nil && info != nil
}

func (s *cleanupSession) deleteRemainder(ctx context.Context, budget *cleanupEntryBudget) (int64, error) {
	removed := int64(0)
	processed := 0
	for budget.remaining > 0 && len(s.stack) > 0 {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if processed%cleanupBatchSize == 0 {
			if err := s.verifyRoot(); err != nil {
				s.addIssue("cache_root_changed", err)
				return removed, err
			}
		}
		frame := s.stack[len(s.stack)-1]
		entry, err := frame.nextEntry()
		if errors.Is(err, io.EOF) {
			removed += s.popFrame(frame, true, ctx)
			continue
		}
		if err != nil {
			s.addIssue("directory_read_failed", err)
			_ = s.popFrame(frame, false, ctx)
			continue
		}
		if isProtectedCacheEntry(frame.relative, entry.Name()) {
			continue
		}
		processed++
		budget.consume(1)
		removed += s.deleteRemainderEntry(frame, entry)
	}
	if len(s.stack) == 0 {
		s.traversalDone = true
	}
	return removed, nil
}

func (s *cleanupSession) deleteRemainderEntry(frame *cleanupFrame, entry os.DirEntry) int64 {
	relative, info, ok := s.inspectFrameEntry(frame, entry)
	if !ok {
		return 0
	}
	if info.IsDir() {
		s.pushDirectory(relative, info)
		return 0
	}
	if !info.Mode().IsRegular() {
		s.addIssue("unsupported_entry_skipped", errors.New("unsupported Go-cache entry was preserved"))
		return 0
	}
	removed, err := s.removeEntry(cleanupEntry{relative: relative, info: info, bytes: info.Size()})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.addIssue("entry_delete_failed", err)
	}
	return removed
}

func (s *cleanupSession) inspectEntry(relative string) (os.FileInfo, error) {
	info, err := s.root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		s.addIssue("entry_read_failed", err)
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		s.addIssue("symlink_skipped", errors.New("go-cache symlink was preserved"))
		return nil, nil
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return info, nil
	}
	opened, err := s.root.Open(relative)
	if err != nil {
		s.addIssue("entry_read_failed", err)
		return nil, err
	}
	defer func() { _ = opened.Close() }()
	openedInfo, err := opened.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		if err == nil {
			err = errors.New("go-cache entry changed while opening")
		}
		s.addIssue("path_changed_or_unsafe", err)
		return nil, err
	}
	descriptorIdentity, err := s.rootHandle.descriptorIdentity(opened)
	if err != nil || descriptorIdentity != s.rootHandle.descriptorFilesystem {
		if err == nil {
			err = errors.New("go-cache entry crosses a mount boundary")
		}
		s.addIssue("filesystem_boundary_skipped", err)
		return nil, err
	}
	if err := s.verifyEntryPathMount(relative); err != nil {
		s.addIssue("filesystem_boundary_skipped", err)
		return nil, err
	}
	return info, nil
}

func (s *cleanupSession) verifyEntryPathMount(relative string) error {
	pathIdentity, err := s.identity(filepath.Join(s.cachePath, relative))
	if err != nil {
		return err
	}
	if pathIdentity != s.filesystem {
		return errors.New("go-cache mount boundary was preserved")
	}
	return nil
}

func (s *cleanupSession) pushDirectory(relative string, expected os.FileInfo) bool {
	if len(s.stack) >= cleanupMaxDepth {
		s.addIssue("directory_depth_limit_reached", errors.New("go-cache directory depth limit reached"))
		return false
	}
	reader, err := s.root.Open(relative)
	if err != nil {
		s.addIssue("directory_open_failed", err)
		return false
	}
	openedInfo, err := reader.Stat()
	if err != nil || !os.SameFile(expected, openedInfo) {
		_ = reader.Close()
		if err == nil {
			err = errors.New("go-cache directory changed while opening")
		}
		s.addIssue("path_changed_or_unsafe", err)
		return false
	}
	identity, err := s.identity(filepath.Join(s.cachePath, relative))
	descriptorIdentity, descriptorErr := s.rootHandle.descriptorIdentity(reader)
	if err != nil || identity != s.filesystem || descriptorErr != nil || descriptorIdentity != s.rootHandle.descriptorFilesystem {
		_ = reader.Close()
		if err == nil {
			err = descriptorErr
		}
		if err == nil {
			err = errors.New("go-cache mount boundary changed while opening")
		}
		s.addIssue("filesystem_boundary_skipped", err)
		return false
	}
	s.directories[relative] = expected
	s.stack = append(s.stack, &cleanupFrame{relative: relative, info: expected, reader: reader})
	return true
}

func (s *cleanupSession) popFrame(frame *cleanupFrame, removeDirectory bool, ctx context.Context) int64 {
	_ = frame.reader.Close()
	s.stack = s.stack[:len(s.stack)-1]
	if frame.relative == "" {
		s.markTraversalDone()
		return 0
	}
	entry := cleanupEntry{relative: frame.relative, info: frame.info}
	if !strings.ContainsRune(frame.relative, filepath.Separator) {
		s.markTraversalDone()
		return 0
	}
	if removeDirectory {
		s.removeDirectory(entry, ctx)
	} else {
		s.entries = append(s.entries, entry)
	}
	s.markTraversalDone()
	return 0
}

func (s *cleanupSession) removeDirectory(entry cleanupEntry, ctx context.Context) {
	if err := ctx.Err(); err != nil {
		s.addIssue(cancellationIssue(err), err)
		return
	}
	if err := s.verifyRoot(); err != nil {
		s.addIssue("cache_root_changed", err)
		return
	}
	if _, err := s.removeEntry(entry); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.addIssue("entry_delete_failed", err)
	}
}

func (s *cleanupSession) markTraversalDone() {
	if len(s.stack) != 0 {
		return
	}
	s.traversalDone = true
	if s.phase == cleanupDiscover {
		s.discoveryComplete = len(s.issues) == 0
	}
}

func (s *cleanupSession) deleteCandidates(ctx context.Context, budget *cleanupEntryBudget) (int64, error) {
	removed := int64(0)
	processed := 0
	for s.nextDelete < len(s.entries) && budget.remaining > 0 {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if processed%cleanupBatchSize == 0 {
			if err := s.verifyRoot(); err != nil {
				s.addIssue("cache_root_changed", err)
				return removed, err
			}
		}
		entry := s.entries[s.nextDelete]
		s.nextDelete++
		processed++
		budget.consume(1)
		bytes, err := s.removeEntry(entry)
		removed += bytes
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.addIssue("path_changed_or_unsafe", err)
		}
	}
	return removed, nil
}

func isProtectedCacheEntry(directory, name string) bool {
	return directory == "" && (name == markerName || name == fuzzDirectoryName)
}

func (s *cleanupSession) removeEntry(entry cleanupEntry) (int64, error) {
	parent, closeParent, err := s.openEntryParent(entry.relative)
	if err != nil {
		return 0, err
	}
	if closeParent {
		defer func() { _ = parent.Close() }()
	}
	name := filepath.Base(entry.relative)
	if err := s.verifyRemovalTarget(parent, name, entry); err != nil {
		return 0, err
	}
	if err := parent.Remove(name); err != nil {
		return 0, err
	}
	return entry.bytes, nil
}

func (s *cleanupSession) verifyRemovalTarget(parent *os.Root, name string, entry cleanupEntry) error {
	current, err := parent.Lstat(name)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !os.SameFile(entry.info, current) {
		return errors.New("go-cache entry changed after scan")
	}
	opened, err := parent.Open(name)
	if err != nil {
		return err
	}
	openedInfo, statErr := opened.Stat()
	if statErr == nil && !os.SameFile(entry.info, openedInfo) {
		statErr = errors.New("go-cache entry changed while opening for removal")
	}
	if statErr == nil {
		filesystem, identityErr := s.rootHandle.descriptorIdentity(opened)
		if identityErr != nil {
			statErr = identityErr
		} else if filesystem != s.rootHandle.descriptorFilesystem {
			statErr = errors.New("go-cache entry crossed a mount boundary before removal")
		}
	}
	_ = opened.Close()
	if statErr != nil {
		return statErr
	}
	pathIdentity, err := s.identity(filepath.Join(s.cachePath, entry.relative))
	if err != nil || pathIdentity != s.filesystem {
		if err == nil {
			err = errors.New("go-cache entry mount changed before removal")
		}
		return err
	}
	return nil
}

func (s *cleanupSession) openEntryParent(relative string) (*os.Root, bool, error) {
	parentPath := filepath.Dir(relative)
	if parentPath == "." || parentPath == "" {
		return s.root, false, nil
	}
	current := s.root
	owned := false
	currentRelative := ""
	for _, component := range strings.Split(filepath.Clean(parentPath), string(filepath.Separator)) {
		parentRelative := component
		if currentRelative != "" {
			parentRelative = filepath.Join(currentRelative, component)
		}
		next, err := s.openEntryDirectory(current, component, parentRelative)
		if err != nil {
			if owned {
				_ = current.Close()
			}
			return nil, false, err
		}
		if owned {
			_ = current.Close()
		}
		current = next
		owned = true
		currentRelative = parentRelative
	}
	return current, owned, nil
}

func (s *cleanupSession) openEntryDirectory(parent *os.Root, component, relative string) (*os.Root, error) {
	if component == "" || component == "." || component == ".." {
		return nil, errors.New("invalid Go-cache parent path")
	}
	before, err := parent.Lstat(component)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("go-cache parent contains a symlink")
	}
	expected := s.directories[relative]
	if expected == nil || !os.SameFile(expected, before) {
		return nil, errors.New("go-cache parent changed after scan")
	}
	next, err := parent.OpenRoot(component)
	if err != nil {
		return nil, err
	}
	if err := s.verifyEntryDirectory(next, expected, relative); err != nil {
		_ = next.Close()
		return nil, err
	}
	return next, nil
}

func (s *cleanupSession) verifyEntryDirectory(next *os.Root, expected os.FileInfo, relative string) error {
	actual, err := next.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(expected, actual) {
		return errors.New("go-cache parent changed while opening")
	}
	descriptorIdentity, err := cacheFilesystemIdentityFromRootWith(next, s.rootHandle.descriptorIdentity)
	if err != nil {
		return err
	}
	if descriptorIdentity != s.rootHandle.descriptorFilesystem {
		return errors.New("go-cache parent crosses a mount boundary")
	}
	pathIdentity, err := s.identity(filepath.Join(s.cachePath, relative))
	if err != nil {
		return err
	}
	if pathIdentity != s.filesystem {
		return errors.New("go-cache parent mount changed")
	}
	return nil
}

func (s *cleanupSession) addIssue(issue string, cause error) {
	if len(s.issues) < cleanupIssueLimit {
		s.issues = append(s.issues, issue)
		s.cause = errors.Join(s.cause, cause)
	}
}

func (s *cleanupSession) errorsForPass(passError error, fallback string) []string {
	issues := append([]string(nil), s.issues...)
	if passError != nil {
		issues = append(issues, cancellationIssue(passError))
	} else if fallback != "" {
		issues = append(issues, fallback)
	}
	return boundedCleanupIssues(issues)
}

func (s *cleanupSession) failure() error {
	if s.cause != nil {
		return s.cause
	}
	if len(s.issues) > 0 {
		return issueError(s.issues)
	}
	return nil
}

func (s *cleanupSession) failureForPass(passError error, fallback string) error {
	return errors.Join(s.failure(), passError, issueError(s.errorsForPass(passError, fallback)))
}

func (f *cleanupFrame) nextEntry() (os.DirEntry, error) {
	if f.pendingAt < len(f.pending) {
		entry := f.pending[f.pendingAt]
		f.pendingAt++
		return entry, nil
	}
	if f.pendingErr != nil {
		err := f.pendingErr
		f.pendingErr = nil
		f.pending = nil
		f.pendingAt = 0
		return nil, err
	}
	f.pending, f.pendingErr = f.reader.ReadDir(cleanupBatchSize)
	f.pendingAt = 0
	return f.nextEntry()
}

func issueError(issues []string) error {
	if len(issues) == 0 {
		return nil
	}
	return errors.New(strings.Join(issues, ", "))
}

func saturatingAdd(left, right int64) int64 {
	if right > 0 && left > int64(^uint64(0)>>1)-right {
		return int64(^uint64(0) >> 1)
	}
	return left + right
}
