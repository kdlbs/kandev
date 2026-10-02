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
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()

	session, result, err := p.prepareCleanupSession(result, cachePath, adopted, maxBytes, identityKey, identity)
	if err != nil {
		return result, err
	}
	result, handled, passError, err := p.discoverCleanup(ctx, session, entryLimit, result)
	if handled {
		return result, err
	}
	return p.deleteCleanupPass(ctx, session, cachePath, entryLimit, result, passError)
}

func (p *Provider) prepareCleanupSession(
	result CleanupResult,
	cachePath string,
	adopted bool,
	maxBytes int64,
	identityKey string,
	identity filesystemIdentityFunc,
) (*cleanupSession, CleanupResult, error) {
	session := p.cleanupState
	if session != nil && !session.matches(cachePath, adopted, maxBytes, identityKey) {
		session.close()
		p.cleanupState = nil
		session = nil
	}
	if session != nil {
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
		session, err = newCleanupSession(cachePath, adopted, maxBytes, identityKey, identity)
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
	entryLimit int,
	result CleanupResult,
	passError error,
) (CleanupResult, error) {
	removed, deleteErr := session.deleteCandidates(ctx, entryLimit)
	result.ReclaimedBytes += removed
	if deleteErr != nil {
		passError = errors.Join(passError, deleteErr)
	}
	if ctx.Err() == nil && session.nextDelete == len(session.entries) && !session.traversalDone {
		removed, passError = session.deleteRemainder(ctx, entryLimit)
		result.ReclaimedBytes += removed
	}
	result.BytesBefore = session.bytes
	result.BytesBeforeComplete = session.discoveryComplete
	result.Errors = append([]string(nil), session.issues...)
	if session.traversalDone && session.nextDelete == len(session.entries) {
		remaining := scanCacheWithIdentity(ctx, session.root, cachePath, session.filesystem, entryLimit, session.identity)
		if len(remaining.issues) == 0 {
			result.BytesAfter = newInt64(remaining.bytes)
		} else {
			result.Errors = append(result.Errors, remaining.issues...)
		}
		result.Errors = boundedCleanupIssues(result.Errors)
		result.Partial = len(result.Errors) > 0 || passError != nil || ctx.Err() != nil
		p.finishCleanupSession(session)
		if result.Partial {
			return result, errors.Join(session.failure(), passError, issueError(result.Errors))
		}
		return result, nil
	}

	result.Partial = true
	result.Reason = "cleanup_in_progress"
	result.Errors = boundedCleanupIssues(append(result.Errors, session.errorsForPass(passError, "entry_limit_reached")...))
	return result, session.failureForPass(passError, "entry_limit_reached")
}

func (p *Provider) discoverCleanup(
	ctx context.Context,
	session *cleanupSession,
	entryLimit int,
	result CleanupResult,
) (CleanupResult, bool, error, error) {
	passError := ctx.Err()
	if passError == nil && session.phase == cleanupDiscover {
		_, passError = session.discover(ctx, entryLimit)
	}
	result.BytesBefore = session.bytes
	result.BytesBeforeComplete = session.discoveryComplete
	if session.phase != cleanupDiscover {
		return result, false, passError, nil
	}
	if !session.traversalDone {
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

func newCleanupSession(
	cachePath string,
	adopted bool,
	maxBytes int64,
	identityKey string,
	identity filesystemIdentityFunc,
) (*cleanupSession, error) {
	root, rootInfo, filesystem, err := openOwnedCacheRootWithIdentity(cachePath, adopted, identity)
	if err != nil {
		return nil, err
	}
	reader, err := root.Open(".")
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("open Go-cache root for cleanup: %w", err)
	}
	session := &cleanupSession{
		cachePath: cachePath, adopted: adopted, maxBytes: maxBytes, identityKey: identityKey,
		identity: identity, root: root, rootInfo: rootInfo, filesystem: filesystem,
		directories: make(map[string]os.FileInfo),
		stack:       []*cleanupFrame{{relative: "", info: rootInfo, reader: reader}},
	}
	return session, nil
}

func (s *cleanupSession) matches(cachePath string, adopted bool, maxBytes int64, identityKey string) bool {
	return s.cachePath == cachePath && s.adopted == adopted && s.maxBytes == maxBytes && s.identityKey == identityKey
}

func (s *cleanupSession) verifyRoot() error {
	return verifyCacheRootWithIdentity(s.root, s.cachePath, s.rootInfo, s.filesystem, s.adopted, s.identity)
}

func (s *cleanupSession) close() {
	for _, frame := range s.stack {
		_ = frame.reader.Close()
	}
	if s.root != nil {
		_ = s.root.Close()
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

func (s *cleanupSession) deleteRemainder(ctx context.Context, entryLimit int) (int64, error) {
	removed := int64(0)
	processed := 0
	for processed < entryLimit && len(s.stack) > 0 {
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
		processed++
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
	identity, err := s.identity(filepath.Join(s.cachePath, relative))
	if err != nil || identity != s.filesystem {
		if err == nil {
			err = errors.New("go-cache mount boundary was preserved")
		}
		s.addIssue("filesystem_boundary_skipped", err)
		return nil, err
	}
	return info, nil
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
	if err != nil || identity != s.filesystem {
		_ = reader.Close()
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

func (s *cleanupSession) deleteCandidates(ctx context.Context, entryLimit int) (int64, error) {
	removed := int64(0)
	processed := 0
	for s.nextDelete < len(s.entries) && processed < entryLimit {
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
		bytes, err := s.removeEntry(entry)
		removed += bytes
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			s.addIssue("path_changed_or_unsafe", err)
		}
	}
	return removed, nil
}

func (s *cleanupSession) removeEntry(entry cleanupEntry) (int64, error) {
	if err := verifySnapshotPathWithIdentity(
		s.root, s.cachePath, s.filesystem, entry.relative, entry.info, s.directories, s.identity,
	); err != nil {
		return 0, err
	}
	if err := s.root.Remove(entry.relative); err != nil {
		return 0, err
	}
	return entry.bytes, nil
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
