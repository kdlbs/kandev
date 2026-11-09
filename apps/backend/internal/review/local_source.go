package review

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Source names a backend-local Git worktree and the comparison it supplies.
type Source struct {
	WorktreePath string `json:"worktree_path"`
	BaseRef      string `json:"base_ref"`
	HeadRef      string `json:"head_ref,omitempty"`
}

// ValidateSource verifies an existing local worktree and resolvable commit refs.
func ValidateSource(ctx context.Context, source Source) error {
	_, _, _, err := prepareLocalSource(ctx, source)
	return err
}

func localPath(path string) bool {
	return filepath.IsAbs(path) && !strings.HasPrefix(path, `\\`) && !strings.HasPrefix(path, "//")
}

// hasTraversalParent reports whether any path segment is "..", so a path cannot
// climb out of the directory it names before it is resolved.
func hasTraversalParent(path string) bool {
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

func prepareLocalSource(ctx context.Context, source Source) (string, string, string, error) {
	if !localPath(source.WorktreePath) || hasTraversalParent(source.WorktreePath) {
		return "", "", "", fmt.Errorf("%w: worktree_path must be an absolute local path", ErrWorkspaceUnavailable)
	}
	path, err := filepath.EvalSymlinks(source.WorktreePath)
	if err != nil || !localPath(path) {
		return "", "", "", fmt.Errorf("%w: worktree_path must exist on local disk", ErrWorkspaceUnavailable)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", "", "", fmt.Errorf("%w: worktree_path must be a directory", ErrWorkspaceUnavailable)
	}
	inside, err := localGit(ctx, path, localMaxTotalDiffBytes, "rev-parse", "--is-inside-work-tree")
	if err != nil || strings.TrimSpace(inside) != "true" {
		return "", "", "", fmt.Errorf("%w: worktree_path is not a git worktree", ErrWorkspaceUnavailable)
	}
	base, err := localCommit(ctx, path, source.BaseRef)
	if err != nil {
		return "", "", "", err
	}
	head := ""
	if source.HeadRef != "" {
		head, err = localCommit(ctx, path, source.HeadRef)
		if err != nil {
			return "", "", "", err
		}
		base, err = localGit(ctx, path, localMaxTotalDiffBytes, "merge-base", base, head)
		if err != nil {
			return "", "", "", fmt.Errorf("%w: refs have no merge base", ErrWorkspaceUnavailable)
		}
		base = strings.TrimSpace(base)
	}
	return path, base, head, nil
}

func localCommit(ctx context.Context, path, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", fmt.Errorf("%w: base_ref is required", ErrWorkspaceUnavailable)
	}
	commit, err := localGit(ctx, path, localMaxTotalDiffBytes, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%w: invalid git ref %q", ErrWorkspaceUnavailable, ref)
	}
	return strings.TrimSpace(commit), nil
}

// ResolveLocalComparisonBase picks the base a local cumulative diff compares
// against, mirroring agentctl: the live target branch (origin/<target> first,
// then <target>) so changes already merged from the target are excluded, and
// the stored base only when no target ref can be compared with HEAD. It never
// fetches.
func ResolveLocalComparisonBase(ctx context.Context, worktreePath, targetBranch, storedBase string) (string, bool) {
	for _, ref := range localComparisonRefs(targetBranch) {
		if mergeBase, err := localGit(ctx, worktreePath, localMaxTotalDiffBytes, "merge-base", "HEAD", ref); err == nil && strings.TrimSpace(mergeBase) != "" {
			return ref, true
		}
	}
	if stored := strings.TrimSpace(storedBase); stored != "" {
		return stored, true
	}
	return "", false
}

// ResolveLocalComparisonRef compares HEAD with one authoritative ref. It does
// not try a same-named origin branch or a stored base when the ref is missing.
func ResolveLocalComparisonRef(ctx context.Context, worktreePath, comparisonRef string) (string, bool) {
	commit, err := localCommit(ctx, worktreePath, comparisonRef)
	if err != nil {
		return "", false
	}
	mergeBase, err := localGit(ctx, worktreePath, localMaxTotalDiffBytes, "merge-base", "HEAD", commit)
	if err != nil || strings.TrimSpace(mergeBase) == "" {
		return "", false
	}
	return strings.TrimSpace(mergeBase), true
}

func localComparisonRefs(targetBranch string) []string {
	target := strings.TrimSpace(targetBranch)
	if target == "" {
		return nil
	}
	return []string{"origin/" + target, target}
}

// CollectLocalChanges reads a committed three-dot range when HeadRef is set;
// otherwise it reads the tracked index and working tree (including untracked
// files) against BaseRef.
func CollectLocalChanges(ctx context.Context, source Source) ([]ChangedFile, error) {
	path, base, head, err := prepareLocalSource(ctx, source)
	if err != nil {
		return nil, err
	}
	args := localDiffArgs(base, head)
	statusArgs := append(append([]string{}, args...), "--name-status", "-z", "--")
	status, err := localGit(ctx, path, localMaxTotalDiffBytes, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("%w: read local git changes: %v", ErrWorkspaceUnavailable, err)
	}
	entries := strings.Split(strings.TrimSuffix(status, "\x00"), "\x00")
	files := make([]ChangedFile, 0)
	covered := make(map[string]struct{})
	used := 0
	for i := 0; i+1 < len(entries); i += 2 {
		file, err := localChangedFile(ctx, path, base, head, entries[i+1], entries[i], &used)
		if err != nil {
			return nil, err
		}
		covered[entries[i+1]] = struct{}{}
		if file.Diff != "" {
			files = append(files, file)
		}
	}
	if head == "" {
		staged, err := localStagedChanges(ctx, path, base, covered, &used)
		if err != nil {
			return nil, err
		}
		files = append(files, staged...)
		untracked, err := localUntrackedFiles(ctx, path, &used)
		if err != nil {
			return nil, err
		}
		files = append(files, untracked...)
	}
	sortChangedFiles(files)
	return files, nil
}

// localStagedChanges reads index-only changes (BaseRef to the index) that the
// working-tree diff did not already cover. `git diff BaseRef` compares BaseRef
// to the working tree, so a staged change the working tree reverted to the base
// content would otherwise be dropped; agentctl's collection keeps it.
func localStagedChanges(ctx context.Context, path, base string, covered map[string]struct{}, used *int) ([]ChangedFile, error) {
	statusArgs := append(localCachedDiffArgs(base), "--name-status", "-z", "--")
	status, err := localGit(ctx, path, localMaxTotalDiffBytes, statusArgs...)
	if err != nil {
		return nil, fmt.Errorf("%w: read staged changes: %v", ErrWorkspaceUnavailable, err)
	}
	entries := strings.Split(strings.TrimSuffix(status, "\x00"), "\x00")
	files := make([]ChangedFile, 0)
	for i := 0; i+1 < len(entries); i += 2 {
		name := entries[i+1]
		if _, ok := covered[name]; ok {
			continue
		}
		file, err := localStagedFile(ctx, path, base, name, entries[i], used)
		if err != nil {
			return nil, err
		}
		if file.Diff != "" {
			files = append(files, file)
		}
	}
	return files, nil
}

func localStagedFile(ctx context.Context, path, base, file, status string, used *int) (ChangedFile, error) {
	if *used >= localMaxTotalDiffBytes {
		return ChangedFile{}, nil
	}
	if localFileTooLarge(ctx, path, base, "", file) {
		return ChangedFile{}, nil
	}
	tooLarge, err := localIndexFileTooLarge(ctx, path, file)
	if err != nil {
		return ChangedFile{}, fmt.Errorf("%w: inspect staged file %q: %v", ErrWorkspaceUnavailable, file, err)
	}
	if tooLarge {
		return ChangedFile{}, nil
	}
	args := append(localCachedDiffArgs(base), "--", file)
	diff, err := localGit(ctx, path, localMaxDiffOutputSize, args...)
	if err != nil && !errors.Is(err, errLocalGitOutputLimit) {
		return ChangedFile{}, fmt.Errorf("%w: read staged diff for %q: %v", ErrWorkspaceUnavailable, file, err)
	}
	return localFile(file, localChangeStatus(status), diff, false, used), nil
}

func localIndexFileTooLarge(ctx context.Context, path, file string) (bool, error) {
	entries, err := localGit(ctx, path, localMaxDiffOutputSize, "ls-files", "--stage", "-z", "--", file)
	if err != nil {
		return false, err
	}
	for _, entry := range strings.Split(strings.TrimSuffix(entries, "\x00"), "\x00") {
		metadata, name, ok := strings.Cut(entry, "\t")
		if !ok || name != file {
			continue
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 {
			return false, fmt.Errorf("unexpected index entry for %q", file)
		}
		sizeText, err := localGit(ctx, path, 128, "cat-file", "-s", fields[1])
		if err != nil {
			return false, err
		}
		size, err := strconv.ParseInt(strings.TrimSpace(sizeText), 10, 64)
		if err != nil {
			return false, fmt.Errorf("invalid index blob size %q", sizeText)
		}
		if size > localMaxDiffFileSize {
			return true, nil
		}
	}
	return false, nil
}

func localDiffArgs(base, head string) []string {
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=all", base}
	if head != "" {
		args = append(args, head)
	}
	return args
}

func localCachedDiffArgs(base string) []string {
	return []string{"diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--ignore-submodules=all", base}
}
