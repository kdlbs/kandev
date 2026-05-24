// Package copyfiles copies user-specified files (literal paths, directories, or
// globs) from a source directory into a freshly-created target directory while
// preserving relative paths. It is designed for the worktree feature that
// seeds a new worktree with environment / config files that are normally
// gitignored.
//
// Two entry points:
//
//   - Copy: host-side, streams files directly to disk. Used by the worktree
//     preparer when source and target both live on the host filesystem.
//   - Plan + WriteEntries: split for remote executors. Plan reads source
//     bytes into memory (with a per-file size cap); WriteEntries writes a
//     pre-loaded batch into a target directory. The agentctl HTTP path
//     ships Plan output across the wire and calls WriteEntries on the
//     receiving side.
package copyfiles

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// MaxEntryBytes caps the in-memory payload for a single planned entry. A
// stray pattern like `node_modules` would otherwise pull gigabytes into RAM
// and across the agentctl wire. Tuned to comfortably hold reasonable
// .env / config / certificate files while rejecting an accidental
// directory-of-binaries.
const MaxEntryBytes int64 = 5 * 1024 * 1024

// Entry is a single file ready to be written into a target directory.
// RelPath uses forward slashes regardless of host OS — agentctl converts
// per-platform on write.
type Entry struct {
	RelPath string      `json:"rel_path"`
	Mode    os.FileMode `json:"mode"`
	Content []byte      `json:"content"`
}

// Parse splits a comma-separated user spec into trimmed, deduplicated,
// non-empty patterns. Order is preserved (first occurrence wins on dedupe).
func Parse(spec string) []string {
	if spec == "" {
		return nil
	}
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Copy resolves each pattern relative to sourceDir and copies matches into
// targetDir, preserving relative paths. It returns the relative paths of
// files newly written into targetDir (skip-if-exists matches are NOT
// included), one warning per problematic pattern or rejected match, and an
// error only for IO failures that would corrupt the target.
func Copy(ctx context.Context, sourceDir, targetDir string, patterns []string, log *zap.Logger) ([]string, []string, error) {
	if len(patterns) == 0 {
		return nil, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("copyfiles: %w", err)
	}

	canonRoot, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("copyfiles: resolve source: %w", err)
	}

	state := &copyState{
		ctx:       ctx,
		targetDir: targetDir,
		canonRoot: canonRoot,
		log:       log,
		copied:    make(map[string]struct{}),
	}

	for _, pattern := range patterns {
		if err := ctx.Err(); err != nil {
			return state.copiedRel, state.warnings, fmt.Errorf("copyfiles: %w", err)
		}
		if err := state.expandPattern(pattern); err != nil {
			return state.copiedRel, state.warnings, err
		}
	}
	return state.copiedRel, state.warnings, nil
}

type copyState struct {
	ctx       context.Context
	targetDir string
	canonRoot string
	log       *zap.Logger
	copied    map[string]struct{}
	copiedRel []string
	warnings  []string

	// planMode collects file payloads into entries instead of writing to
	// targetDir. Set by Plan(). When true, copyFile reads bytes into memory
	// (respecting MaxEntryBytes) and appends to entries rather than touching
	// the filesystem.
	planMode bool
	entries  []Entry
}

// Plan resolves each pattern relative to sourceDir and reads every match
// into memory as an Entry — without touching any target filesystem. It is
// the read half of a remote copy: the agentctl HTTP client ships the
// returned entries across the wire, and WriteEntries writes them on the
// receiving side. Files larger than MaxEntryBytes emit a warning and are
// skipped to keep launch payloads bounded.
//
// Returned entries preserve forward-slash relative paths; consumers must
// not assume host-native separators.
func Plan(ctx context.Context, sourceDir string, patterns []string, log *zap.Logger) ([]Entry, []string, error) {
	if len(patterns) == 0 {
		return nil, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("copyfiles: %w", err)
	}

	canonRoot, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return nil, nil, fmt.Errorf("copyfiles: resolve source: %w", err)
	}

	state := &copyState{
		ctx:       ctx,
		canonRoot: canonRoot,
		log:       log,
		copied:    make(map[string]struct{}),
		planMode:  true,
	}

	for _, pattern := range patterns {
		if err := ctx.Err(); err != nil {
			return state.entries, state.warnings, fmt.Errorf("copyfiles: %w", err)
		}
		if err := state.expandPattern(pattern); err != nil {
			return state.entries, state.warnings, err
		}
	}
	return state.entries, state.warnings, nil
}

// WriteEntries writes a pre-planned batch into targetDir, preserving
// relative paths and file modes. Idempotent: entries whose destination
// already exists are silently skipped (matches Copy's resume semantics).
// Returns the list of newly-written relative paths plus per-entry
// warnings; an error only for IO failures that would corrupt the target.
//
// Per-entry RelPath is treated as user-controlled — each is rejected
// unless it canonicalizes to a path strictly inside targetDir. This is
// the agentctl-side guard: the planner already validates against the
// source root, but a compromised sender could craft paths that escape
// the target.
func WriteEntries(ctx context.Context, targetDir string, entries []Entry, log *zap.Logger) ([]string, []string, error) {
	if len(entries) == 0 {
		return nil, nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("copyfiles: %w", err)
	}
	// Resolve symlinks AND require the target to exist as a directory before
	// any per-entry write. EvalSymlinks both canonicalizes and validates —
	// it errors on non-existent paths and breaks the taint chain CodeQL
	// would otherwise trace from caller-supplied targetDir into os.Stat /
	// os.WriteFile sinks. Per-entry RelPaths are then re-checked against
	// this canonical root in writeOneEntry.
	absTarget, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		return nil, nil, fmt.Errorf("copyfiles: resolve target: %w", err)
	}
	info, err := os.Stat(absTarget)
	if err != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("copyfiles: target %q is not a directory", absTarget)
	}

	var copied, warnings []string
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return copied, warnings, fmt.Errorf("copyfiles: %w", err)
		}
		written, warn, err := writeOneEntry(absTarget, e, log)
		if warn != "" {
			warnings = append(warnings, warn)
		}
		if err != nil {
			return copied, warnings, err
		}
		if written {
			copied = append(copied, filepath.ToSlash(e.RelPath))
		}
	}
	return copied, warnings, nil
}

// writeOneEntry validates a single Entry, refusing paths that escape
// targetDir, then writes it (skip-if-exists for idempotency). Resolves
// symlinks on the destination's parent directory so a pre-existing
// `config -> /etc` symlink can't smuggle the write outside absTarget
// (flagged as a Critical by CodeRabbit on PR #950).
func writeOneEntry(absTarget string, e Entry, log *zap.Logger) (written bool, warning string, err error) {
	rel := filepath.FromSlash(e.RelPath)
	if rel == "" {
		return false, "rejected empty path", nil
	}
	if filepath.IsAbs(rel) {
		return false, fmt.Sprintf("rejected absolute path %q", e.RelPath), nil
	}
	cleanDst, warn, err := secureDestination(absTarget, rel)
	if err != nil {
		return false, "", err
	}
	if warn != "" {
		return false, warn, nil
	}
	if _, err := os.Lstat(cleanDst); err == nil {
		if log != nil {
			log.Debug("copyfiles: skip existing", zap.String("rel", e.RelPath))
		}
		return false, "", nil
	}
	mode := e.Mode.Perm()
	if mode == 0 {
		mode = 0o644
	}
	if err := os.WriteFile(cleanDst, e.Content, mode); err != nil {
		// Best-effort cleanup so a half-written file doesn't poison the
		// next idempotent run's skip-if-exists branch (the streaming
		// copyFile path does the same). Ignore unlink failure.
		_ = os.Remove(cleanDst)
		return false, "", fmt.Errorf("copyfiles: write %s: %w", cleanDst, err)
	}
	if log != nil {
		log.Debug("copyfiles: wrote entry", zap.String("rel", e.RelPath))
	}
	return true, "", nil
}

// secureDestination resolves the destination path for a relative entry,
// creating parent directories and verifying via EvalSymlinks that the
// final path still lies inside absTarget after any symlinks in parent
// components are followed. Returns:
//
//   - (cleanDst, "", nil) on success — safe to write.
//   - ("", warn, nil)     when the path is rejected (warn surfaced to caller).
//   - ("", "", err)       only on mkdir IO failure — fatal for the batch.
//
// Closes the symlink-target-escape hole CodeRabbit flagged on PR #950:
// without the post-MkdirAll EvalSymlinks, a pre-existing component like
// `config -> /etc` could allow a write at /etc/passwd via a `config/x`
// relative path despite the lexical Rel check passing.
func secureDestination(absTarget, rel string) (cleanDst, warning string, err error) {
	lexical := filepath.Join(absTarget, rel)
	cleanDst, absErr := filepath.Abs(lexical)
	if absErr != nil {
		return "", fmt.Sprintf("rejected %q: %v", rel, absErr), nil
	}
	if relCheck, relErr := filepath.Rel(absTarget, cleanDst); relErr != nil ||
		relCheck == ".." ||
		strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Sprintf("rejected %q: escapes target", rel), nil
	}
	parent := filepath.Dir(cleanDst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", "", fmt.Errorf("copyfiles: mkdir %s: %w", parent, err)
	}
	canonParent, evalErr := filepath.EvalSymlinks(parent)
	if evalErr != nil {
		return "", fmt.Sprintf("rejected %q: parent resolve failed: %v", rel, evalErr), nil
	}
	final := filepath.Join(canonParent, filepath.Base(cleanDst))
	if relCheck, relErr := filepath.Rel(absTarget, final); relErr != nil ||
		relCheck == ".." ||
		strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", fmt.Sprintf("rejected %q: symlink in parent escapes target", rel), nil
	}
	return final, "", nil
}

func (s *copyState) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.warnings = append(s.warnings, msg)
	if s.log != nil {
		s.log.Warn("copyfiles: " + msg)
	}
}

func (s *copyState) debug(msg string, fields ...zap.Field) {
	if s.log != nil {
		s.log.Debug("copyfiles: "+msg, fields...)
	}
}

// expandPattern handles literal files, literal directories, and globs.
func (s *copyState) expandPattern(pattern string) error {
	joined := pattern
	if !filepath.IsAbs(pattern) {
		joined = filepath.Join(s.canonRoot, pattern)
	}

	// Fast path: literal existing entry.
	if _, err := os.Lstat(joined); err == nil {
		return s.handleMatch(joined, pattern)
	}

	matches, err := filepath.Glob(joined)
	if err != nil {
		s.warn("invalid pattern %q: %v", pattern, err)
		return nil
	}
	if len(matches) == 0 {
		s.warn("no matches for pattern %q", pattern)
		return nil
	}
	for _, m := range matches {
		if err := s.ctx.Err(); err != nil {
			return fmt.Errorf("copyfiles: %w", err)
		}
		if _, err := os.Lstat(m); err != nil {
			s.warn("stat %q: %v", m, err)
			continue
		}
		if err := s.handleMatch(m, pattern); err != nil {
			return err
		}
	}
	return nil
}

// handleMatch dispatches a single literal/match path to file or directory copy.
func (s *copyState) handleMatch(matchPath, pattern string) error {
	safe, ok := s.safePath(matchPath)
	if !ok {
		s.warn("path %q is outside source dir (pattern %q)", matchPath, pattern)
		return nil
	}

	// Resolve via EvalSymlinks for the actual final target (follows symlinks).
	resolved, err := filepath.EvalSymlinks(safe)
	if err != nil {
		s.warn("resolve %q: %v", matchPath, err)
		return nil
	}
	if !s.underRoot(resolved) {
		s.warn("symlink %q resolves outside source dir", matchPath)
		return nil
	}

	// Use the original match path to compute the relative dest, NOT the
	// resolved path — a symlink should land at the symlink's location.
	rel, err := filepath.Rel(s.canonRoot, safe)
	if err != nil {
		s.warn("rel %q: %v", matchPath, err)
		return nil
	}

	rInfo, err := os.Stat(resolved)
	if err != nil {
		s.warn("stat resolved %q: %v", resolved, err)
		return nil
	}

	if rInfo.IsDir() {
		return s.copyDir(resolved, rel)
	}
	return s.copyFile(resolved, rel, rInfo)
}

// copyDir walks src recursively and copies every regular file inside.
func (s *copyState) copyDir(src, relRoot string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			s.warn("walk %q: %v", path, walkErr)
			return nil
		}
		if err := s.ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			s.warn("info %q: %v", path, err)
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			s.warn("rel %q: %v", path, err)
			return nil
		}
		// If the entry is itself a symlink, resolve and validate it stays in root.
		resolved := path
		if info.Mode()&os.ModeSymlink != 0 {
			r, rerr := filepath.EvalSymlinks(path)
			if rerr != nil {
				s.warn("resolve %q: %v", path, rerr)
				return nil
			}
			if !s.underRoot(r) {
				s.warn("symlink %q resolves outside source dir", path)
				return nil
			}
			ri, rerr := os.Stat(r)
			if rerr != nil {
				s.warn("stat %q: %v", r, rerr)
				return nil
			}
			if ri.IsDir() {
				s.warn("symlink %q resolves to a directory, skipping", path)
				return nil
			}
			info = ri
			resolved = r
		}
		return s.copyFile(resolved, filepath.Join(relRoot, rel), info)
	})
}

// copyFile copies a single regular file to targetDir/rel, creating parents.
// In planMode it instead reads the file's bytes into an Entry (subject to
// MaxEntryBytes), used by Plan() for the remote-executor path.
func (s *copyState) copyFile(src, rel string, info os.FileInfo) error {
	if err := s.ctx.Err(); err != nil {
		return fmt.Errorf("copyfiles: %w", err)
	}
	if !info.Mode().IsRegular() {
		s.warn("skipping non-regular file %q (mode %s)", rel, info.Mode())
		return nil
	}
	if s.planMode {
		return s.planFile(src, rel, info)
	}
	dst := filepath.Join(s.targetDir, rel)
	if _, dup := s.copied[dst]; dup {
		return nil
	}

	// Skip-if-exists for idempotency on resume.
	if _, err := os.Lstat(dst); err == nil {
		s.copied[dst] = struct{}{}
		s.debug("skip existing", zap.String("rel", rel))
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("copyfiles: mkdir %s: %w", filepath.Dir(dst), err)
	}

	in, err := os.Open(src)
	if err != nil {
		s.warn("open %q: %v", src, err)
		return nil
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copyfiles: create %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("copyfiles: copy %s: %w", dst, err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("copyfiles: close %s: %w", dst, err)
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return fmt.Errorf("copyfiles: chmod %s: %w", dst, err)
	}

	s.copied[dst] = struct{}{}
	s.copiedRel = append(s.copiedRel, filepath.ToSlash(rel))
	s.debug("copied", zap.String("rel", rel))
	return nil
}

// planFile reads a source file into an in-memory Entry instead of writing
// it. Files larger than MaxEntryBytes are skipped with a warning; users
// who hit this are typically pattern-matching too broadly (e.g. catching
// node_modules) and want feedback, not a silent partial copy.
func (s *copyState) planFile(src, rel string, info os.FileInfo) error {
	dedup := filepath.ToSlash(rel)
	if _, dup := s.copied[dedup]; dup {
		return nil
	}
	if info.Size() > MaxEntryBytes {
		s.warn("skipping %q: %d bytes exceeds %d-byte plan cap", rel, info.Size(), MaxEntryBytes)
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		s.warn("open %q: %v", src, err)
		return nil
	}
	defer func() { _ = f.Close() }()
	// LimitReader as belt-and-suspenders against TOCTOU growth between Stat
	// and Read — a file appended-to mid-walk shouldn't bypass MaxEntryBytes.
	content, err := io.ReadAll(io.LimitReader(f, MaxEntryBytes+1))
	if err != nil {
		s.warn("read %q: %v", src, err)
		return nil
	}
	if int64(len(content)) > MaxEntryBytes {
		s.warn("skipping %q: read exceeded %d-byte plan cap", rel, MaxEntryBytes)
		return nil
	}
	s.copied[dedup] = struct{}{}
	s.entries = append(s.entries, Entry{
		RelPath: filepath.ToSlash(rel),
		Mode:    info.Mode().Perm(),
		Content: content,
	})
	s.copiedRel = append(s.copiedRel, filepath.ToSlash(rel))
	s.debug("planned", zap.String("rel", rel))
	return nil
}

// safePath returns the lexically-cleaned absolute path within sourceDir, or
// false if the path escapes the source root before any symlink resolution.
func (s *copyState) safePath(p string) (string, bool) {
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.canonRoot, abs)
	}
	abs = filepath.Clean(abs)
	if !s.underRoot(abs) {
		return "", false
	}
	return abs, true
}

// underRoot reports whether path is canonRoot itself or lies inside canonRoot.
func (s *copyState) underRoot(path string) bool {
	rel, err := filepath.Rel(s.canonRoot, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}
