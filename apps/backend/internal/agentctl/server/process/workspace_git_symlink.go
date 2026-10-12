package process

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// enrichSymlinkMetadata observes entry modes independently of patch enrichment.
// Raw diffs retain modes even when patch content is binary or exceeds its budget.
func (wt *WorkspaceTracker) enrichSymlinkMetadata(ctx context.Context, update *types.GitStatusUpdate) {
	if len(update.Files) == 0 {
		return
	}
	staged := wt.readChangedSymlinkModes(ctx, true)
	unstaged := wt.readChangedSymlinkModes(ctx, false)
	for path, file := range update.Files {
		if ctx.Err() != nil {
			return
		}
		if file.StagedChange != nil {
			file.StagedChange.IsSymlink = symlinkModeValue(staged, path)
		}
		if file.UnstagedChange != nil {
			file.UnstagedChange.IsSymlink = symlinkModeValue(unstaged, path)
		}
		switch {
		case file.Status == fileStatusUntracked:
			if filepath.IsLocal(path) {
				if info, err := os.Lstat(filepath.Join(wt.workDir, path)); err == nil {
					value := info.Mode()&os.ModeSymlink != 0
					file.IsSymlink = &value
					if file.UnstagedChange != nil {
						file.UnstagedChange.IsSymlink = &value
					}
				}
			}
		case file.Staged:
			file.IsSymlink = symlinkModeValue(staged, path)
		default:
			file.IsSymlink = symlinkModeValue(unstaged, path)
		}
		update.Files[path] = file
	}
}

func symlinkModeValue(modes map[string]bool, path string) *bool {
	value, ok := modes[path]
	if !ok {
		return nil
	}
	return &value
}

func (wt *WorkspaceTracker) readChangedSymlinkModes(ctx context.Context, staged bool) map[string]bool {
	args := []string{"diff", "--raw", "-z", "--no-renames", "--no-ext-diff"}
	if staged {
		args = append(args, "--cached")
	}
	out, err := wt.runGitOutput(ctx, args...)
	if err != nil {
		return nil
	}
	return parseChangedSymlinkModes(string(out))
}

// --no-renames gives exactly one NUL-delimited path per raw diff record.
func parseChangedSymlinkModes(raw string) map[string]bool {
	modes := make(map[string]bool)
	records := strings.Split(raw, "\x00")
	for i := 0; i+1 < len(records); i += 2 {
		fields := strings.Fields(records[i])
		if len(fields) != 5 || !strings.HasPrefix(fields[0], ":") || records[i+1] == "" {
			continue
		}
		mode := fields[1]
		if fields[4] == "D" {
			mode = strings.TrimPrefix(fields[0], ":")
		}
		if mode != "100644" && mode != "100755" && mode != "120000" && mode != "160000" {
			continue
		}
		modes[records[i+1]] = mode == "120000"
	}
	return modes
}
