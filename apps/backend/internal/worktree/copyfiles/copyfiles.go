// Package copyfiles copies user-specified files (literal paths, directories, or
// globs) from a source directory into a freshly-created target directory while
// preserving relative paths. It is designed for the worktree feature that
// seeds a new worktree with environment / config files that are normally
// gitignored.
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
// targetDir, preserving relative paths. It returns one warning per problematic
// pattern or rejected match, and an error only for IO failures that would
// corrupt the target.
func Copy(ctx context.Context, sourceDir, targetDir string, patterns []string, log *zap.Logger) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("copyfiles: %w", err)
	}

	canonRoot, err := filepath.EvalSymlinks(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("copyfiles: resolve source: %w", err)
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
			return state.warnings, fmt.Errorf("copyfiles: %w", err)
		}
		if err := state.expandPattern(pattern); err != nil {
			return state.warnings, err
		}
	}
	return state.warnings, nil
}

type copyState struct {
	ctx       context.Context
	targetDir string
	canonRoot string
	log       *zap.Logger
	copied    map[string]struct{}
	warnings  []string
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
				return nil
			}
			info = ri
			resolved = r
		}
		return s.copyFile(resolved, filepath.Join(relRoot, rel), info)
	})
}

// copyFile copies a single regular file to targetDir/rel, creating parents.
func (s *copyState) copyFile(src, rel string, info os.FileInfo) error {
	if err := s.ctx.Err(); err != nil {
		return fmt.Errorf("copyfiles: %w", err)
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
		return fmt.Errorf("copyfiles: close %s: %w", dst, err)
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return fmt.Errorf("copyfiles: chmod %s: %w", dst, err)
	}

	s.copied[dst] = struct{}{}
	s.debug("copied", zap.String("rel", rel))
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
