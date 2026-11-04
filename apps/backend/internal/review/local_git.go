package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/common/unidiff"
	"github.com/kandev/kandev/internal/utility/hash"
)

// These match agentctl's workspace_git_diff.go collection limits.
const (
	localStatusAdded       = "added"
	localMaxDiffFileSize   = 10 * 1024 * 1024
	localMaxDiffOutputSize = 256 * 1024
	localMaxTotalDiffBytes = 2 * 1024 * 1024
	localBinaryCheckSize   = 8 * 1024
)

type localGitBuffer struct {
	data     []byte
	limit    int
	exceeded bool
}

var errLocalGitOutputLimit = errors.New("git output exceeds collection limit")

func (b *localGitBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - len(b.data)
	if n > remaining {
		b.exceeded = true
		p = p[:remaining]
	}
	b.data = append(b.data, p...)
	return n, nil
}

func localGit(ctx context.Context, path string, limit int, args ...string) (string, error) {
	var output localGitBuffer
	output.limit = limit
	cmdErr, ctxErr := subproc.RunGitAfterAcquire(ctx, subproc.GitInteractive, 30*time.Second,
		func(execCtx context.Context) *exec.Cmd {
			cmd := subproc.NewGitCommand(execCtx, append([]string{"--literal-pathspecs", "-C", path}, args...)...)
			cmd.Stdout = &output
			return cmd
		})
	if ctxErr != nil {
		return "", ctxErr
	}
	if output.exceeded {
		return string(output.data), fmt.Errorf("%w: %d bytes", errLocalGitOutputLimit, limit)
	}
	return string(output.data), cmdErr
}

func localChangedFile(ctx context.Context, path, base, head, file, status string, used *int) (ChangedFile, error) {
	if *used >= localMaxTotalDiffBytes {
		return ChangedFile{}, nil
	}
	if localFileTooLarge(ctx, path, base, head, file) {
		return ChangedFile{}, nil
	}
	args := append(localDiffArgs(base, head), "--", file)
	diff, err := localGit(ctx, path, localMaxDiffOutputSize, args...)
	if err != nil && !errors.Is(err, errLocalGitOutputLimit) {
		return ChangedFile{}, fmt.Errorf("%w: read local diff for %q: %v", ErrWorkspaceUnavailable, file, err)
	}
	return localFile(file, localChangeStatus(status), diff, head != "", used), nil
}

func localFileTooLarge(ctx context.Context, path, base, head, file string) bool {
	if head == "" {
		if info, err := os.Stat(filepath.Join(path, filepath.FromSlash(file))); err == nil && info.Size() > localMaxDiffFileSize {
			return true
		}
	}
	for _, ref := range []string{base, head} {
		if ref == "" {
			continue
		}
		size, err := localGit(ctx, path, 128, "cat-file", "-s", ref+":"+file)
		if err == nil {
			n, _ := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
			if n > localMaxDiffFileSize {
				return true
			}
		}
	}
	return false
}

func localFile(path, status, diff string, committed bool, used *int) ChangedFile {
	diff = NormalizeDiff(diff)
	if diff == "" || !strings.Contains(diff, "\n@@ ") || len(diff)+*used > localMaxTotalDiffBytes {
		return ChangedFile{}
	}
	*used += len(diff)
	file := ChangedFile{Path: path, Status: status, Diff: diff, DiffHash: hash.DJB2(diff), Source: sourceUncommitted}
	if committed {
		file.Source = sourceCommitted
	}
	file.Additions, file.Deletions = localHunkLineCounts(diff)
	return file
}

// localHunkLineCounts counts added and removed lines inside hunks only. Diff
// header lines (`+++`/`---`) sit before the first `@@ `, so a code line whose
// content begins with `+` or `-` is still counted correctly.
func localHunkLineCounts(diff string) (additions, deletions int) {
	inHunk := false
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			inHunk = false
			continue
		}
		if strings.HasPrefix(line, "@@ ") {
			inHunk = true
			continue
		}
		if !inHunk {
			continue
		}
		if strings.HasPrefix(line, "+") {
			additions++
		} else if strings.HasPrefix(line, "-") {
			deletions++
		}
	}
	return additions, deletions
}

// localChangeStatus maps a `git --name-status` code to the file status
// vocabulary agentctl stamps (added, modified, deleted, renamed). Unknown
// codes, including a type change, read as modified.
func localChangeStatus(code string) string {
	switch {
	case strings.HasPrefix(code, "A"):
		return localStatusAdded
	case strings.HasPrefix(code, "D"):
		return "deleted"
	case strings.HasPrefix(code, "R"):
		return "renamed"
	default:
		return "modified"
	}
}

func localUntrackedFiles(ctx context.Context, path string, used *int) ([]ChangedFile, error) {
	output, err := localGit(ctx, path, localMaxTotalDiffBytes, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("%w: read untracked files: %v", ErrWorkspaceUnavailable, err)
	}
	var files []ChangedFile
	for _, name := range strings.Split(output, "\x00") {
		if name == "" || *used >= localMaxTotalDiffBytes {
			continue
		}
		info, err := os.Lstat(filepath.Join(path, filepath.FromSlash(name)))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// The file can disappear between `ls-files` and this stat.
				continue
			}
			return nil, fmt.Errorf("%w: read untracked file %q", ErrWorkspaceUnavailable, name)
		}
		if !info.Mode().IsRegular() || info.Size() > localMaxDiffFileSize {
			continue
		}
		content, binary, err := localUntrackedContent(ctx, path, name)
		if err != nil {
			return nil, fmt.Errorf("%w: read untracked file %q: %v", ErrWorkspaceUnavailable, name, err)
		}
		if binary {
			continue
		}
		diff, additions, _, err := unidiff.BuildUntrackedDiff(ctx, name, content, localMaxDiffOutputSize)
		if err != nil {
			return nil, fmt.Errorf("%w: format untracked diff %q: %v", ErrWorkspaceUnavailable, name, err)
		}
		file := localFile(name, "untracked", diff, false, used)
		if file.Diff != "" {
			file.Additions = additions
			files = append(files, file)
		}
	}
	return files, nil
}

func localUntrackedContent(ctx context.Context, path, name string) ([]byte, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	file, err := os.Open(filepath.Join(path, filepath.FromSlash(name)))
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > localMaxDiffFileSize {
		return nil, true, nil
	}
	content, err := io.ReadAll(io.LimitReader(file, localMaxDiffOutputSize))
	if err != nil {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	header := content
	if len(header) > localBinaryCheckSize {
		header = header[:localBinaryCheckSize]
	}
	return content, bytes.IndexByte(header, 0) >= 0, nil
}
