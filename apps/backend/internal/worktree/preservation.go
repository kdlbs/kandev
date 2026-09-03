package worktree

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrPreservedCheckoutUnproven = errors.New("preserved checkout identity is not proven")

type PreservationRequest struct {
	RepositoryPath string
	WorktreePath   string
	ExpectedBranch string
	WorktreeID     string
}

type PreservationEvidence struct {
	ObservedBranch string
	RefName        string
	HeadOID        string
	WorktreeID     string
	PathHash       string
	StatusHash     string
	ContentHash    string
	IndexHash      string
	DirtyCount     int
	UntrackedCount int
}

// InspectPreservedCheckout proves reciprocal Git identity without modifying
// the checkout, index, refs, or object database.
func InspectPreservedCheckout(ctx context.Context, req PreservationRequest) (*PreservationEvidence, error) {
	repositoryPath, err := canonicalDirectory(req.RepositoryPath)
	if err != nil {
		return nil, fmt.Errorf("%w: repository path", ErrPreservedCheckoutUnproven)
	}
	worktreePath, err := canonicalDirectory(req.WorktreePath)
	if err != nil || req.ExpectedBranch == "" || req.WorktreeID == "" {
		return nil, fmt.Errorf("%w: checkout path", ErrPreservedCheckoutUnproven)
	}
	identity, err := inspectPreservedGitIdentity(ctx, repositoryPath, worktreePath, req.ExpectedBranch)
	if err != nil {
		return nil, err
	}
	// --no-optional-locks tells Git to skip any action that would require
	// taking a lock — in particular, status's normal opportunistic index
	// refresh (updating cached stat info) — so this inspection can never
	// write to .git/index even incidentally.
	status, err := gitBytes(ctx, worktreePath, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	contentHash, err := checkoutContentHash(ctx, worktreePath)
	if err != nil {
		return nil, err
	}
	indexHash, err := stagedIndexHash(ctx, worktreePath)
	if err != nil {
		return nil, err
	}
	dirty, untracked := statusCounts(status)
	return &PreservationEvidence{
		ObservedBranch: identity.branch, RefName: identity.refName, HeadOID: identity.headOID,
		WorktreeID: req.WorktreeID, PathHash: hashBytes([]byte(worktreePath)),
		StatusHash: hashBytes(status), ContentHash: contentHash, IndexHash: indexHash,
		DirtyCount: dirty, UntrackedCount: untracked,
	}, nil
}

type preservedGitIdentity struct {
	branch  string
	refName string
	headOID string
}

func inspectPreservedGitIdentity(
	ctx context.Context,
	repositoryPath, worktreePath, expectedBranch string,
) (*preservedGitIdentity, error) {
	commonDir, err := gitAbsolutePath(ctx, worktreePath, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	repositoryCommonDir, err := gitAbsolutePath(ctx, repositoryPath, "--git-common-dir")
	if err != nil || commonDir != repositoryCommonDir {
		return nil, fmt.Errorf("%w: common git directory", ErrPreservedCheckoutUnproven)
	}
	topLevel, err := gitAbsolutePath(ctx, worktreePath, "--show-toplevel")
	if err != nil || topLevel != worktreePath {
		return nil, fmt.Errorf("%w: checkout root", ErrPreservedCheckoutUnproven)
	}
	branch, err := gitText(ctx, worktreePath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || branch != expectedBranch {
		return nil, fmt.Errorf("%w: branch", ErrPreservedCheckoutUnproven)
	}
	refName, err := gitText(ctx, worktreePath, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("%w: ref", ErrPreservedCheckoutUnproven)
	}
	headOID, err := gitText(ctx, worktreePath, "rev-parse", "HEAD")
	if err != nil || !worktreeRegistrationMatches(ctx, repositoryPath, worktreePath, refName, headOID) {
		return nil, fmt.Errorf("%w: git worktree registration", ErrPreservedCheckoutUnproven)
	}
	return &preservedGitIdentity{branch: branch, refName: refName, headOID: headOID}, nil
}

func canonicalDirectory(path string) (string, error) {
	return CanonicalDirectory(path)
}

// CanonicalDirectory resolves path to its canonical, symlink-free absolute
// form: it rejects a path whose final component is itself a symlink and
// resolves every parent-directory symlink via filepath.EvalSymlinks, so a
// symlink planted in a parent directory cannot be used to make a path lexically
// compare as scoped to a root it does not actually resolve into. Callers that
// need to prove canonical ownership of a directory (not just compare strings)
// should use this instead of filepath.Abs/Clean.
func CanonicalDirectory(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", ErrPreservedCheckoutUnproven
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func gitAbsolutePath(ctx context.Context, directory, argument string) (string, error) {
	value, err := gitText(ctx, directory, "rev-parse", "--path-format=absolute", argument)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(value)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func gitText(ctx context.Context, directory string, args ...string) (string, error) {
	output, err := gitBytes(ctx, directory, args...)
	return strings.TrimSpace(string(output)), err
}

func gitBytes(ctx context.Context, directory string, args ...string) ([]byte, error) {
	cmd := newGitCommand(ctx, append([]string{"-C", directory}, args...)...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: git inspection failed", ErrPreservedCheckoutUnproven)
	}
	return output, nil
}

func worktreeRegistrationMatches(ctx context.Context, repositoryPath, worktreePath, refName, headOID string) bool {
	output, err := gitText(ctx, repositoryPath, "worktree", "list", "--porcelain")
	if err != nil {
		return false
	}
	for _, block := range strings.Split(output, "\n\n") {
		fields := strings.Split(block, "\n")
		if len(fields) < 3 || strings.TrimPrefix(fields[0], "worktree ") != worktreePath {
			continue
		}
		return strings.TrimPrefix(fields[1], "HEAD ") == headOID && strings.TrimPrefix(fields[2], "branch ") == refName
	}
	return false
}

func statusCounts(status []byte) (int, int) {
	dirty, untracked := 0, 0
	for _, entry := range bytes.Split(status, []byte{0}) {
		if len(entry) < 3 {
			continue
		}
		if bytes.HasPrefix(entry, []byte("?? ")) {
			untracked++
		} else {
			dirty++
		}
	}
	return dirty, untracked
}

func checkoutContentHash(ctx context.Context, worktreePath string) (string, error) {
	output, err := gitBytes(ctx, worktreePath, "ls-files", "-co", "-z")
	if err != nil {
		return "", err
	}
	paths := bytes.Split(output, []byte{0})
	sort.Slice(paths, func(i, j int) bool { return bytes.Compare(paths[i], paths[j]) < 0 })
	hash := sha256.New()
	for _, rawPath := range paths {
		if len(rawPath) == 0 {
			continue
		}
		path := filepath.Join(worktreePath, filepath.FromSlash(string(rawPath)))
		info, statErr := os.Lstat(path)
		if statErr != nil || info.IsDir() {
			return "", fmt.Errorf("%w: checkout content", ErrPreservedCheckoutUnproven)
		}
		_, _ = hash.Write(rawPath)
		_, _ = hash.Write([]byte{0})
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return "", readErr
			}
			_, _ = hash.Write([]byte(target))
			continue
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", readErr
		}
		_, _ = hash.Write(contents)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// stagedIndexHash proves the exact staged blob identity of every path in
// the index — mode, object SHA, and path — without reading a single working
// tree file. checkoutContentHash alone hashes on-disk bytes, so it cannot
// see a change that only touches the index: for example, a low-level
// `git update-index --cacheinfo` write that repoints a path's staged blob
// SHA while the working tree file, and therefore its bytes, its `git status`
// classification, and its dirty/untracked counts, are all left untouched.
// `git ls-files --stage` output is already sorted by path, and each path's
// SHA already commits to that blob's content, so hashing it directly proves
// staged identity without ever writing to .git/index or the object
// database.
func stagedIndexHash(ctx context.Context, worktreePath string) (string, error) {
	output, err := gitBytes(ctx, worktreePath, "--no-optional-locks", "ls-files", "--stage", "-z")
	if err != nil {
		return "", err
	}
	return hashBytes(output), nil
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
