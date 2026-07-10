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

// gitDirName is the reserved git admin entry that must never be materialized.
const gitDirName = ".git"

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
// mode. Blank entries and files missing from the source are skipped silently.
// Any other per-file failure (path traversal, reserved .git, source resolving
// outside the repo, permission errors, etc.) is collected as a warning and the
// remaining files are still processed — mirroring copyConfiguredFiles so partial
// failures stay observable rather than silently aborting the rest. There is no
// silent fallback from symlink to copy. Returns the collected warnings.
func MaterializeWorktreeFiles(srcRoot, destRoot string, files []FileSpec) []string {
	var warnings []string
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" {
			continue
		}
		mode := NormalizeFileMaterializeMode(string(file.Mode))
		if err := materializeFile(mode, srcRoot, destRoot, file.Path); err != nil {
			if errors.Is(err, errSourceMissing) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("%s: %v", file.Path, err))
		}
	}
	return warnings
}

// materializeFile places a single repo-relative file into destRoot.
// ErrInvalidWorktreeFilePath is returned by CleanWorktreeFilePath for paths that
// are absolute, escape the repository root, or target the reserved .git entry.
var ErrInvalidWorktreeFilePath = errors.New("invalid worktree file path")

// CleanWorktreeFilePath validates a configured worktree-file path and returns
// its cleaned, repository-relative form. Empty/"." inputs return ("", nil) so
// callers can skip them. Absolute paths, parent-directory traversal, and the
// reserved .git admin path are rejected with ErrInvalidWorktreeFilePath so the
// same rules apply at save time (service validation) and at materialization.
func CleanWorktreeFilePath(relPath string) (string, error) {
	cleanRel := filepath.Clean(strings.TrimSpace(relPath))
	if cleanRel == "" || cleanRel == "." {
		return "", nil
	}
	if filepath.IsAbs(cleanRel) || cleanRel == ".." ||
		strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q escapes repository root", ErrInvalidWorktreeFilePath, relPath)
	}
	if cleanRel == gitDirName || strings.HasPrefix(cleanRel, gitDirName+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q targets the reserved .git path", ErrInvalidWorktreeFilePath, relPath)
	}
	return cleanRel, nil
}

func materializeFile(mode FileMaterializeMode, srcRoot, destRoot, relPath string) error {
	cleanRel, err := CleanWorktreeFilePath(relPath)
	if err != nil {
		return err
	}
	if cleanRel == "" {
		return nil
	}

	src := filepath.Join(srcRoot, cleanRel)
	dest := filepath.Join(destRoot, cleanRel)

	if _, err := os.Lstat(src); err != nil {
		if os.IsNotExist(err) {
			return errSourceMissing
		}
		return fmt.Errorf("stat worktree file source %q: %w", src, err)
	}

	// Reject symlinked destination ancestors before writing: os.MkdirAll follows
	// symlinks, so a symlinked parent (e.g. a checked-out repo symlink, or an
	// earlier symlink-mode entry) could otherwise let RemoveAll/copy/symlink
	// escape the worktree.
	if err := rejectSymlinkedDestAncestor(destRoot, dest); err != nil {
		return err
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
	return copyPath(srcRoot, src, dest)
}

// rejectSymlinkedDestAncestor returns an error when any existing directory
// between destRoot (exclusive) and dest is a symlink. Since os.MkdirAll follows
// symlinked ancestors, this prevents a configured file from being materialized
// outside the worktree through a symlinked parent. Non-existent ancestors are
// fine — os.MkdirAll will create them as real directories.
func rejectSymlinkedDestAncestor(destRoot, dest string) error {
	rel, err := filepath.Rel(destRoot, filepath.Dir(dest))
	if err != nil {
		return err
	}
	current := destRoot
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // deeper ancestors don't exist yet; MkdirAll creates them
			}
			return fmt.Errorf("stat worktree dest ancestor %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: destination parent %q is a symlink", ErrInvalidWorktreeFilePath, current)
		}
	}
	return nil
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
// It first resolves symlinks and rejects sources that escape srcRoot so a
// repo-controlled symlink (e.g. .env -> /home/user/.ssh/id_rsa) can't exfiltrate
// files from outside the repository, and it skips non-regular sources (FIFOs,
// devices, sockets) which would otherwise block or error on open.
func copyPath(srcRoot, src, dest string) error {
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		return fmt.Errorf("resolve source %q: %w", src, err)
	}
	if err := ensureWithinRoot(srcRoot, resolved); err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %q: %w", src, err)
	}
	if info.IsDir() {
		return copyDir(src, dest)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("worktree file %q is not a regular file (mode %s)", src, info.Mode())
	}
	return copyFile(src, dest, info.Mode())
}

// ensureWithinRoot returns ErrInvalidWorktreeFilePath when path (already
// symlink-resolved) is not contained within root.
func ensureWithinRoot(root, path string) error {
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootResolved = root
	}
	rel, err := filepath.Rel(rootResolved, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: source %q resolves outside the repository", ErrInvalidWorktreeFilePath, path)
	}
	return nil
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
		// WalkDir does not follow symlinks: it reports them as non-dir entries
		// with the symlink bit set. Recreate the link rather than opening it
		// (which would EISDIR on symlinked dirs or copy with wrong perms).
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, readErr := os.Readlink(path)
			if readErr != nil {
				return fmt.Errorf("read symlink %q: %w", path, readErr)
			}
			return os.Symlink(linkTarget, target)
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		// Skip non-regular entries (FIFOs, devices, sockets) rather than
		// blocking/erroring on open.
		if !info.Mode().IsRegular() {
			return nil
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
