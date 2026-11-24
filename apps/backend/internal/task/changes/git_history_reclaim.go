package changes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Reclaim removes unreachable backend-owned objects only after every ref in
// the archive has been released. Shared user repositories are never touched.
func (s *GitHistoryStore) Reclaim(ctx context.Context, sourceIDs []string) (int64, error) {
	if !s.Enabled() {
		return 0, nil
	}
	var freed int64
	for _, sourceID := range sourceIDs {
		if !historySourceKeyPattern.MatchString(sourceID) {
			continue
		}
		unlock := s.lockSource(sourceID)
		repoPath := s.repositoryPath(sourceID)
		refs, err := s.git(ctx, "", repoPath, historyGitOperationTimeout, "for-each-ref", "--format=%(refname)")
		if err != nil {
			if errors.Is(err, ErrHistoryContentUnavailable) {
				unlock()
				continue
			}
			unlock()
			return freed, err
		}
		if strings.TrimSpace(string(refs)) != "" {
			unlock()
			continue
		}
		before, err := directorySize(filepath.Join(repoPath, "objects"))
		if err != nil {
			unlock()
			return freed, err
		}
		if err := removeArchiveObjects(repoPath); err != nil {
			unlock()
			return freed, err
		}
		after, err := directorySize(filepath.Join(repoPath, "objects"))
		if err != nil {
			unlock()
			return freed, err
		}
		if before > after {
			freed += before - after
		}
		unlock()
	}
	return freed, nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func removeArchiveObjects(repoPath string) error {
	_ = os.Remove(filepath.Join(repoPath, "objects", "info", "packs"))
	packDir := filepath.Join(repoPath, "objects", "pack")
	entries, err := os.ReadDir(packDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(packDir, entry.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	objectDirs, err := os.ReadDir(filepath.Join(repoPath, "objects"))
	if err != nil {
		return err
	}
	for _, entry := range objectDirs {
		if !entry.IsDir() || len(entry.Name()) != 2 {
			continue
		}
		if err := os.RemoveAll(filepath.Join(repoPath, "objects", entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// HasCapacity enforces the installation-wide archive bound without evicting
// protected history.
func (s *GitHistoryStore) HasCapacity(ctx context.Context, extra int64) bool {
	if !s.Enabled() || extra < 0 {
		return s.Enabled() && extra == 0
	}
	if _, err := os.Stat(s.root); os.IsNotExist(err) {
		return extra <= maxHistoryArchiveInstallationBytes
	}
	size, err := directorySize(s.root)
	if err != nil {
		return false
	}
	limit := s.maxBytes
	if limit <= 0 {
		limit = maxHistoryArchiveInstallationBytes
	}
	return size+extra <= limit
}
