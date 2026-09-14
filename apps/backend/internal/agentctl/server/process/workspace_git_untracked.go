package process

import (
	"bytes"
	"context"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// gitUntrackedFilesArgs is the shared query for eligible untracked paths.
// Git applies the dependency-tree exclusion while walking the worktree, and
// NUL termination keeps valid paths independent from quoting or newlines.
var gitUntrackedFilesArgs = []string{
	"ls-files",
	"--others",
	"--exclude-standard",
	"--exclude=node_modules/",
	"-z",
}

func parseGitUntrackedOutput(ctx context.Context, output []byte) ([]string, error) {
	paths := make([]string, 0)
	for len(output) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		end := bytes.IndexByte(output, 0)
		if end < 0 {
			end = len(output)
		}
		if end > 0 {
			paths = append(paths, string(output[:end]))
		}
		if end == len(output) {
			break
		}
		output = output[end+1:]
	}
	return paths, nil
}

func (wt *WorkspaceTracker) applyUntrackedOutput(
	ctx context.Context,
	output []byte,
	update *types.GitStatusUpdate,
) error {
	paths, err := parseGitUntrackedOutput(ctx, output)
	if err != nil {
		return err
	}
	for _, filePath := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		update.Untracked = append(update.Untracked, filePath)
		update.Files[filePath] = types.FileInfo{
			Path:   filePath,
			Status: fileStatusUntracked,
		}
	}
	return nil
}
