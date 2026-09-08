package process

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type gitIndexFileContextKey struct{}

func withGitIndexFile(ctx context.Context, indexPath string) context.Context {
	return context.WithValue(ctx, gitIndexFileContextKey{}, indexPath)
}

func gitIndexFile(ctx context.Context) string {
	indexPath, _ := ctx.Value(gitIndexFileContextKey{}).(string)
	return indexPath
}

// snapshotGitIndex creates a stable, read-only view of the Git index for a
// group of related status queries. Git replaces the real index atomically for
// writes, so a hard link keeps the observed index at one point in time. The
// copy fallback supports filesystems that do not allow hard links.
func snapshotGitIndex(ctx context.Context, indexPath string) (string, func(), error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}

	temp, err := os.CreateTemp(filepath.Dir(indexPath), ".kandev-index-snapshot-*")
	if err != nil {
		return "", nil, fmt.Errorf("create git index snapshot: %w", err)
	}
	snapshotPath := temp.Name()
	cleanup := func() { _ = os.Remove(snapshotPath) }
	if err := temp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close git index snapshot: %w", err)
	}
	if err := os.Remove(snapshotPath); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("prepare git index snapshot: %w", err)
	}

	if err := ctx.Err(); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := os.Link(indexPath, snapshotPath); err == nil {
		if err := ctx.Err(); err != nil {
			cleanup()
			return "", nil, err
		}
		return snapshotPath, cleanup, nil
	}

	source, err := os.Open(indexPath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open git index fallback: %w", err)
	}
	defer func() { _ = source.Close() }()

	// The source descriptor remains attached to one index inode even if Git
	// replaces the original path while the fallback copy is prepared.
	destination, err := os.OpenFile(snapshotPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("open git index snapshot fallback: %w", err)
	}
	_, copyErr := io.Copy(destination, source)
	closeErr := destination.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("copy git index snapshot: %w", copyErr)
	}
	if closeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("close git index snapshot fallback: %w", closeErr)
	}
	if err := ctx.Err(); err != nil {
		cleanup()
		return "", nil, err
	}
	return snapshotPath, cleanup, nil
}
