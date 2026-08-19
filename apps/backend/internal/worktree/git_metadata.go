package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitMetadataProjectionVersion changes whenever the shape of the metadata
// permission contract changes. Hashes are freshness diagnostics only; callers
// must always resolve the projection again before installing permissions.
const GitMetadataProjectionVersion = 1

// ErrGitMetadataProjectionInvalid is returned when checkout metadata cannot
// prove that it belongs to the checkout being authorized.
var ErrGitMetadataProjectionInvalid = errors.New("git metadata projection invalid")

// GitMetadataProjection describes exactly the Git metadata a task checkout
// needs for ordinary add and commit operations. CommonDir is intentionally not
// included in WritablePaths: callers must grant only its listed children.
type GitMetadataProjection struct {
	Version        int
	CheckoutPath   string
	GitDir         string
	CommonDir      string
	WorktreesDir   string
	ObjectDir      string
	CurrentRef     string
	CurrentRefPath string
	ReflogPath     string
	WritablePaths  []string
	Hash           string
}

// ResolveGitMetadata validates checkout Git metadata and returns a narrow
// projection. The checkout path is server-owned input; .git contents are
// treated solely as evidence that must reciprocally validate.
func ResolveGitMetadata(checkoutPath string) (*GitMetadataProjection, error) {
	checkout, err := canonicalDirectory(checkoutPath)
	if err != nil {
		return nil, invalidGitMetadata(err)
	}
	gitEntry := filepath.Join(checkout, ".git")
	entryInfo, err := os.Lstat(gitEntry)
	if err != nil {
		return nil, invalidGitMetadata(err)
	}
	if entryInfo.Mode()&os.ModeSymlink != 0 {
		return nil, invalidGitMetadata(errors.New(".git is symbolic link"))
	}
	if entryInfo.IsDir() {
		return resolveRegularGitMetadata(checkout, gitEntry)
	}
	if !entryInfo.Mode().IsRegular() {
		return nil, invalidGitMetadata(errors.New(".git is not directory or pointer"))
	}
	return resolveLinkedGitMetadata(checkout, gitEntry)
}

// Revalidate detects replacement or redirection of any metadata used by this
// projection immediately before a policy or mount is installed.
func (p *GitMetadataProjection) Revalidate() error {
	if p == nil {
		return invalidGitMetadata(errors.New("projection is nil"))
	}
	current, err := ResolveGitMetadata(p.CheckoutPath)
	if err != nil {
		return err
	}
	if current.Hash != p.Hash {
		return invalidGitMetadata(errors.New("projection changed after resolution"))
	}
	return nil
}

func resolveRegularGitMetadata(checkout, gitDir string) (*GitMetadataProjection, error) {
	if err := rejectSymlinkComponents(gitDir); err != nil {
		return nil, invalidGitMetadata(err)
	}
	if _, err := os.Lstat(filepath.Join(gitDir, "commondir")); err == nil {
		return nil, invalidGitMetadata(errors.New("regular repository redirects common directory"))
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, invalidGitMetadata(err)
	}
	return buildGitMetadataProjection(checkout, gitDir, gitDir, "")
}

func resolveLinkedGitMetadata(checkout, gitEntry string) (*GitMetadataProjection, error) {
	gitDir, err := readGitdirPointer(gitEntry, checkout)
	if err != nil {
		return nil, invalidGitMetadata(err)
	}
	if err := rejectSymlinkComponents(gitDir); err != nil {
		return nil, invalidGitMetadata(err)
	}
	backPointer, err := readMetadataPathStrict(filepath.Join(gitDir, "gitdir"), gitDir)
	if err != nil || backPointer != gitEntry {
		return nil, invalidGitMetadata(errors.New("linked worktree reciprocal pointer mismatch"))
	}
	commonDir, err := readMetadataPathStrict(filepath.Join(gitDir, "commondir"), gitDir)
	if err != nil {
		return nil, invalidGitMetadata(err)
	}
	if err := rejectSymlinkComponents(commonDir); err != nil {
		return nil, invalidGitMetadata(err)
	}
	worktreesDir := filepath.Join(commonDir, "worktrees")
	name, err := directChildName(worktreesDir, gitDir)
	if err != nil {
		return nil, invalidGitMetadata(err)
	}
	return buildGitMetadataProjection(checkout, gitDir, commonDir, name)
}

func buildGitMetadataProjection(checkout, gitDir, commonDir, worktreeName string) (*GitMetadataProjection, error) {
	ref, err := readCurrentBranchRef(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return nil, invalidGitMetadata(err)
	}
	paths := []string{gitDir, filepath.Join(commonDir, "objects")}
	refPath, reflogPath := "", ""
	if ref != "" {
		refPath = filepath.Join(commonDir, filepath.FromSlash(ref))
		reflogPath = filepath.Join(commonDir, "logs", filepath.FromSlash(ref))
		if err := rejectSymlinkComponents(refPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, invalidGitMetadata(err)
		}
		paths = append(paths, refPath, reflogPath)
	}
	projection := &GitMetadataProjection{
		Version:        GitMetadataProjectionVersion,
		CheckoutPath:   checkout,
		GitDir:         gitDir,
		CommonDir:      commonDir,
		WorktreesDir:   filepath.Join(commonDir, "worktrees"),
		ObjectDir:      filepath.Join(commonDir, "objects"),
		CurrentRef:     ref,
		CurrentRefPath: refPath,
		ReflogPath:     reflogPath,
		WritablePaths:  uniqueGitMetadataPaths(paths),
	}
	if worktreeName == "" {
		projection.WorktreesDir = ""
	}
	projection.Hash = projectionHash(projection)
	return projection, nil
}

func canonicalDirectory(path string) (string, error) {
	if path == "" {
		return "", errors.New("checkout path is empty")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("checkout is not directory")
	}
	return filepath.Clean(canonical), nil
}

func readGitdirPointer(gitEntry, checkout string) (string, error) {
	content, err := os.ReadFile(gitEntry)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", errors.New("invalid gitdir pointer")
	}
	path := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if path == "" {
		return "", errors.New("empty gitdir pointer")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(checkout, path)
	}
	return canonicalExistingPath(path)
}

func readMetadataPathStrict(path, relativeTo string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", errors.New("metadata pointer is not regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", errors.New("metadata pointer is empty")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(relativeTo, value)
	}
	return canonicalExistingPath(value)
}

func canonicalExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(canonical), nil
}

func directChildName(parent, child string) (string, error) {
	rel, err := filepath.Rel(parent, child)
	if err != nil || rel == "." || rel == ".." || filepath.Dir(rel) != "." {
		return "", errors.New("linked worktree gitdir is outside common worktrees directory")
	}
	return rel, nil
}

func readCurrentBranchRef(headPath string) (string, error) {
	content, err := os.ReadFile(headPath)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(content))
	if !strings.HasPrefix(value, "ref: ") {
		return "", nil // detached HEAD: commits update HEAD, which is in GitDir.
	}
	ref := strings.TrimSpace(strings.TrimPrefix(value, "ref: "))
	if !validBranchRef(ref) {
		return "", errors.New("invalid HEAD ref")
	}
	return ref, nil
}

func validBranchRef(ref string) bool {
	if !strings.HasPrefix(ref, "refs/heads/") || strings.Contains(ref, "\\") {
		return false
	}
	for _, part := range strings.Split(ref, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func rejectSymlinkComponents(path string) error {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(abs)
	current := volume + string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link in metadata path")
		}
	}
	return nil
}

func uniqueGitMetadataPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func projectionHash(p *GitMetadataProjection) string {
	parts := append([]string{fmt.Sprintf("v%d", p.Version), p.CheckoutPath, p.GitDir, p.CommonDir, p.CurrentRef}, p.WritablePaths...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func invalidGitMetadata(err error) error {
	return fmt.Errorf("%w: %v", ErrGitMetadataProjectionInvalid, err)
}
