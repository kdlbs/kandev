package inventoryrepair

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func fileDigest(path string) (string, error) {
	h, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(filepath.Dir(path)), filepath.Dir(path))
	if err != nil {
		return "", err
	}
	defer func() { _ = h.Close() }()
	f, err := h.OpenFile(filepath.Base(path))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	if _, err = io.Copy(hash, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func checkJournalBackup(j *journal) error {
	hash, err := fileDigest(filepath.Join(filepath.Dir(journalPath(j.Plan)), "before.db"))
	if err != nil {
		return err
	}
	if hash != j.BackupSHA256 {
		return errors.New("repair backup changed")
	}
	return nil
}

func pathPresent(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (j *journal) verifyRoots(r Repair) error {
	source, err := taskRoot(j.Plan.TasksRoot, r.SourcePath)
	if err != nil {
		return err
	}
	dest, err := taskRoot(j.Plan.TasksRoot, r.Path)
	if err != nil {
		return err
	}
	for _, root := range []string{source, dest} {
		marker, err := readMarker(j.Plan.TasksRoot, root)
		if err != nil {
			return err
		}
		if digest(marker) != j.Plan.ExpectedRows["marker:"+root] {
			return errors.New("root ownership changed since preview")
		}
	}
	return nil
}

func (j *journal) verifyCheckout(ctx context.Context, r Repair, path string) error {
	if err := j.verifyRoots(r); err != nil {
		return err
	}
	got, err := inspectGit(ctx, r.RepositoryPath, path)
	if err != nil {
		return err
	}
	if got != j.Plan.ExpectedGit[r.WorktreeID] {
		return errors.New("checkout identity, content, index, or refs changed since preview")
	}
	return nil
}

func (j *journal) verifyLocations(ctx context.Context, forward bool) error {
	for _, r := range j.Plan.Repairs {
		path := r.SourcePath
		if forward {
			path = r.Path
		}
		if err := j.verifyCheckout(ctx, r, path); err != nil {
			return err
		}
		if r.SourcePath != r.Path {
			other := r.Path
			if forward {
				other = r.SourcePath
			}
			present, err := pathPresent(other)
			if err != nil {
				return err
			}
			if present {
				return errors.New("both repair locations are present")
			}
		}
	}
	return nil
}

func (j *journal) moveLocations(ctx context.Context, forward bool) error {
	for _, r := range j.Plan.Repairs {
		if err := j.moveOne(ctx, r, forward); err != nil {
			return err
		}
	}
	return nil
}

func (j *journal) moveOne(ctx context.Context, r Repair, forward bool) error {
	from, to := r.SourcePath, r.Path
	if !forward {
		from, to = to, from
	}
	if from == to {
		return nil
	}
	source, err := pathPresent(from)
	if err != nil {
		return err
	}
	dest, err := pathPresent(to)
	if err != nil {
		return err
	}
	if source == dest {
		return errors.New("relocation requires exactly one existing checkout")
	}
	if dest {
		if err = j.verifyCheckout(ctx, r, to); err != nil {
			return err
		}
		return nil
	}
	if err = j.verifyCheckout(ctx, r, from); err != nil {
		return err
	}
	parent, err := storageworkspaces.OpenDirectoryNoFollow(j.Plan.TasksRoot, filepath.Dir(to))
	if err != nil {
		return err
	}
	_, err = runGit(ctx, r.RepositoryPath, "worktree", "move", "--", from, to)
	verifyErr := parent.VerifyPath(filepath.Dir(to))
	_ = parent.Close()
	if err != nil || verifyErr != nil {
		return errors.Join(err, verifyErr)
	}
	if err = syncDirectory(filepath.Dir(from)); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(to)); err != nil {
		return err
	}
	if err = j.verifyCheckout(ctx, r, to); err != nil {
		return err
	}
	return syncMoveMetadata(to, j.Plan.ExpectedGit[r.WorktreeID].GitDir)
}

func syncMoveMetadata(path, gitDir string) error {
	for _, file := range []string{filepath.Join(path, ".git"), filepath.Join(gitDir, "gitdir")} {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		err = f.Sync()
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			return errors.Join(err, closeErr)
		}
	}
	return errors.Join(syncDirectory(path), syncDirectory(gitDir))
}
