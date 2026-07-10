package worktree

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileMaterializeMode controls how a repository's configured files are placed
// into a newly created worktree.
type FileMaterializeMode string

const (
	// FileMaterializeCopy copies each configured file into the worktree,
	// producing an isolated per-worktree copy. This is the default and the
	// historical behavior for files a setup script would have `cp`'d.
	FileMaterializeCopy FileMaterializeMode = "copy"
	// FileMaterializeSymlink symlinks each configured file in the worktree back
	// to the file in the main repository, so shared files (e.g. env files) stay
	// centrally managed and reflect updates across every worktree.
	FileMaterializeSymlink FileMaterializeMode = "symlink"
)

// DefaultFileMaterializeMode is applied when a file has no explicit mode.
const DefaultFileMaterializeMode = FileMaterializeCopy

// FileSpec is a single file to materialize into a worktree with its own mode.
type FileSpec struct {
	Path string
	Mode FileMaterializeMode
}

// ErrInvalidFileMaterializeMode is returned by ValidateFileMaterializeMode for
// a non-empty value that is neither "copy" nor "symlink".
var ErrInvalidFileMaterializeMode = errors.New("invalid worktree file mode")

// errSourceMissing is an internal sentinel: a configured file that does not
// exist in the source repository is skipped rather than failing worktree
// creation (a benign, common case for gitignored env files not yet created).
var errSourceMissing = errors.New("worktree file source not found")

// NormalizeFileMaterializeMode maps user input to a valid mode, defaulting any
// empty or unrecognized value to the safe default (copy).
func NormalizeFileMaterializeMode(mode string) FileMaterializeMode {
	switch FileMaterializeMode(strings.ToLower(strings.TrimSpace(mode))) {
	case FileMaterializeSymlink:
		return FileMaterializeSymlink
	default:
		return FileMaterializeCopy
	}
}

// ValidateFileMaterializeMode reports whether a mode value is acceptable. Empty
// is valid (it resolves to the default); any other non-copy/non-symlink value
// is rejected so misconfigurations surface at save time.
func ValidateFileMaterializeMode(mode string) error {
	switch FileMaterializeMode(strings.ToLower(strings.TrimSpace(mode))) {
	case "", FileMaterializeCopy, FileMaterializeSymlink:
		return nil
	default:
		return fmt.Errorf("%w: %q (want %q or %q)", ErrInvalidFileMaterializeMode, mode, FileMaterializeCopy, FileMaterializeSymlink)
	}
}

// MaterializeWorktreeFiles copies or symlinks each configured file from srcRoot
// (the main repository) into destRoot (the worktree), using that file's own
// mode. Blank entries and files missing from the source are skipped; copy/symlink
// failures and paths that escape the repository root are returned as errors (no
// silent fallback).
func MaterializeWorktreeFiles(srcRoot, destRoot string, files []FileSpec) error {
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		mode := NormalizeFileMaterializeMode(string(file.Mode))
		if err := materializeFile(mode, srcRoot, destRoot, file.Path); err != nil {
			if errors.Is(err, errSourceMissing) {
				continue
			}
			return err
		}
	}
	return nil
}

// materializeFile places a single repo-relative file into destRoot.
func materializeFile(mode FileMaterializeMode, srcRoot, destRoot, relPath string) error {
	cleanRel := filepath.Clean(strings.TrimSpace(relPath))
	if cleanRel == "" || cleanRel == "." {
		return nil
	}
	if filepath.IsAbs(cleanRel) || cleanRel == ".." ||
		strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("worktree file %q escapes repository root", relPath)
	}

	src := filepath.Join(srcRoot, cleanRel)
	dest := filepath.Join(destRoot, cleanRel)

	if _, err := os.Lstat(src); err != nil {
		if os.IsNotExist(err) {
			return errSourceMissing
		}
		return fmt.Errorf("stat worktree file source %q: %w", src, err)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %q: %w", dest, err)
	}
	// Remove any pre-existing destination so the configured strategy always wins
	// and materialization stays idempotent across worktree recreation.
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("clear existing worktree file %q: %w", dest, err)
	}

	if mode == FileMaterializeSymlink {
		return symlinkWorktreeFile(src, dest)
	}
	return copyPath(src, dest)
}

// symlinkWorktreeFile creates a relative symlink at dest pointing to src, so the
// link keeps working if the whole ~/.kandev tree is relocated. It falls back to
// an absolute target only when a relative path cannot be computed.
func symlinkWorktreeFile(src, dest string) error {
	target, err := filepath.Rel(filepath.Dir(dest), src)
	if err != nil {
		target = src
	}
	if err := os.Symlink(target, dest); err != nil {
		return fmt.Errorf("create symlink %q -> %q: %w", dest, target, err)
	}
	return nil
}

// copyPath copies a regular file or (recursively) a directory from src to dest.
func copyPath(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %q: %w", src, err)
	}
	if info.IsDir() {
		return copyDir(src, dest)
	}
	return copyFile(src, dest, info.Mode())
}

// copyDir recursively copies the directory tree at src into dest.
func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, info.Mode())
	})
}

// copyFile copies the bytes of src into dest, preserving the source file mode.
func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return fmt.Errorf("create dest %q: %w", dest, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %q -> %q: %w", src, dest, err)
	}
	return out.Close()
}
