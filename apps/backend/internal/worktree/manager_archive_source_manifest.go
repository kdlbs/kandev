package worktree

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ArchiveSourceManifest is the content-identity record captured before an
// archived worktree is removed. It deliberately stores digests, never file
// contents, so durable cleanup evidence cannot retain source secrets.
type ArchiveSourceManifest struct {
	TaskID            string                       `json:"task_id"`
	TaskEnvironmentID string                       `json:"task_environment_id,omitempty"`
	WorktreeID        string                       `json:"worktree_id"`
	RepositoryID      string                       `json:"repository_id"`
	HeadOID           string                       `json:"head_oid"`
	IndexTreeOID      string                       `json:"index_tree_oid"`
	Entries           []ArchiveSourceManifestEntry `json:"entries,omitempty"`
}

// ArchiveSourceManifestEntry identifies one changed path without retaining its
// source bytes. A missing digest represents a deleted path.
type ArchiveSourceManifestEntry struct {
	Path          string `json:"path"`
	Status        string `json:"status"`
	ContentSHA256 string `json:"content_sha256,omitempty"`
}

// CaptureArchiveSourceManifests reads the git state of exact registered
// worktrees before archive cleanup. Any uncertain ownership, git metadata, or
// filesystem read fails the caller closed.
func (m *Manager) CaptureArchiveSourceManifests(
	ctx context.Context, worktrees []*Worktree,
) (map[string]ArchiveSourceManifest, error) {
	manifests := make(map[string]ArchiveSourceManifest, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		if wt.ID == "" || wt.TaskID == "" || wt.RepositoryID == "" || wt.Path == "" || wt.RepositoryPath == "" {
			return nil, fmt.Errorf("archive source manifest has incomplete worktree identity")
		}
		if err := m.validateExistingWorktreePathOwner(wt.Path, wt); err != nil {
			return nil, fmt.Errorf("validate archive source manifest owner for %s: %w", wt.ID, err)
		}
		head, err := m.runBoundedGitInspect(ctx, wt.Path, "rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			return nil, fmt.Errorf("capture archive source HEAD for %s: %w", wt.ID, err)
		}
		indexTree, err := m.runBoundedGitInspect(ctx, wt.Path, "write-tree")
		if err != nil {
			return nil, fmt.Errorf("capture archive source index for %s: %w", wt.ID, err)
		}
		status, err := m.runBoundedGitInspect(ctx, wt.Path, "status", "--porcelain=v1", "-z", "--untracked-files=all")
		if err != nil {
			return nil, fmt.Errorf("capture archive source status for %s: %w", wt.ID, err)
		}
		entries, err := archiveSourceManifestEntries(wt.Path, status)
		if err != nil {
			return nil, fmt.Errorf("capture archive source entries for %s: %w", wt.ID, err)
		}
		manifests[wt.ID] = ArchiveSourceManifest{
			TaskID: wt.TaskID, TaskEnvironmentID: wt.TaskEnvironmentID,
			WorktreeID: wt.ID, RepositoryID: wt.RepositoryID,
			HeadOID: strings.TrimSpace(head), IndexTreeOID: strings.TrimSpace(indexTree), Entries: entries,
		}
	}
	return manifests, nil
}

func archiveSourceManifestEntries(root, output string) ([]ArchiveSourceManifestEntry, error) {
	fields := strings.Split(output, "\x00")
	entries := make([]ArchiveSourceManifestEntry, 0, len(fields))
	for index := 0; index < len(fields); index++ {
		field := fields[index]
		if field == "" {
			continue
		}
		if len(field) < 4 || field[2] != ' ' {
			return nil, fmt.Errorf("invalid git status record")
		}
		path := field[3:]
		if err := archiveSourceManifestPath(root, path); err != nil {
			return nil, err
		}
		entry := ArchiveSourceManifestEntry{Path: path, Status: field[:2]}
		digest, err := archiveSourceManifestDigest(root, path)
		if err != nil {
			if os.IsNotExist(err) {
				entries = append(entries, entry)
				continue
			}
			return nil, err
		}
		entry.ContentSHA256 = digest
		entries = append(entries, entry)
		// In porcelain v1 -z, rename and copy records carry the original
		// pathname as the following NUL-delimited field without a status prefix.
		if (field[0] == 'R' || field[0] == 'C' || field[1] == 'R' || field[1] == 'C') && index+1 < len(fields) {
			index++
		}
	}
	return entries, nil
}

func archiveSourceManifestPath(root, path string) error {
	if path == "" || filepath.IsAbs(path) {
		return fmt.Errorf("invalid source path")
	}
	rel, err := filepath.Rel(root, filepath.Join(root, path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("source path escapes worktree")
	}
	return nil
}

func archiveSourceManifestDigest(root, path string) (string, error) {
	fullPath := filepath.Join(root, path)
	info, err := os.Lstat(fullPath)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(fullPath)
		if readErr != nil {
			return "", readErr
		}
		sum := sha256.Sum256([]byte("symlink:\x00" + target))
		return fmt.Sprintf("%x", sum), nil
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("source path is not a regular file")
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}
