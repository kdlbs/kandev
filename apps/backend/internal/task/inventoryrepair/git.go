package inventoryrepair

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/subproc"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
)

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	out, err, ctxErr := subproc.RunGitOutputAfterAcquire(ctx, subproc.GitLifecycle, 30*time.Second, func(execCtx context.Context) *exec.Cmd {
		cmd := subproc.NewGitCommand(execCtx, args...)
		cmd.Dir = dir
		cmd.Env = repairGitEnvironment()
		return cmd
	})
	if err != nil || ctxErr != nil {
		return "", fmt.Errorf("git inspection %s: %w", args[0], errors.Join(err, ctxErr))
	}
	return strings.TrimSpace(string(out)), nil
}

func repairGitEnvironment() []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		// Repair uses only local Git operations bound to explicitly inspected paths.
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			env = append(env, entry)
		}
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

func inspectGit(ctx context.Context, repository, path string) (GitState, error) {
	var result GitState
	handle, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(path), path)
	if err != nil {
		return result, err
	}
	defer func() { _ = handle.Close() }()
	repoHandle, err := storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(repository), repository)
	if err != nil {
		return result, err
	}
	defer func() { _ = repoHandle.Close() }()
	result, admin, err := inspectGitRegistration(ctx, repository, path)
	if err != nil {
		return result, err
	}
	defer func() { _ = admin.Close() }()
	index, err := admin.ReadFile("index")
	if err != nil {
		return result, err
	}
	result.IndexSHA256 = fmt.Sprintf("%x", sha256.Sum256(index))
	refs, err := runGit(ctx, repository, "for-each-ref", "--format=%(refname) %(objectname)")
	if err != nil {
		return result, err
	}
	result.RefsSHA256 = digest(refs)
	result.ContentSHA256, err = contentDigest(ctx, handle)
	if err != nil {
		return result, err
	}
	return result, errors.Join(handle.VerifyPath(path), repoHandle.VerifyPath(repository), admin.VerifyPath(result.GitDir))
}

func verifyRegistration(ctx context.Context, repo, path, branch, head string) error {
	listing, err := runGit(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	matches := 0
	for _, record := range strings.Split(listing, "\x00\x00") {
		fields := strings.Split(record, "\x00")
		if len(fields) > 0 && fields[0] == "worktree "+path {
			matches++
			if !contains(fields, "HEAD "+head) || !contains(fields, "branch "+branch) {
				return errors.New("registered branch/HEAD differs from checkout")
			}
		}
	}
	if matches != 1 {
		return errors.New("checkout has no unique Git worktree registration")
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

type contentEntry struct {
	Path   string
	Mode   uint32
	Digest string
}

func contentDigest(ctx context.Context, root storageworkspaces.DirectoryHandle) (string, error) {
	entries := []contentEntry{}
	if err := collectContent(ctx, root, "", &entries); err != nil {
		return "", err
	}
	return digest(entries), nil
}

func collectContent(ctx context.Context, dir storageworkspaces.DirectoryHandle, prefix string, result *[]contentEntry) error {
	entries, err := dir.ReadDir()
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if prefix == "" && entry.Name() == ".git" {
			continue
		}
		if len(*result) >= 100000 {
			return errors.New("checkout content exceeds repair entry limit")
		}
		mode, err := dir.LstatEntry(entry.Name())
		if err != nil {
			return err
		}
		record := contentEntry{Path: prefix + entry.Name(), Mode: uint32(mode)}
		if mode.IsDir() {
			*result = append(*result, record)
			child, err := dir.OpenSubdirectory(entry.Name())
			if err != nil {
				return err
			}
			err = collectContent(ctx, child, record.Path+"/", result)
			closeErr := child.Close()
			if err != nil || closeErr != nil {
				return errors.Join(err, closeErr)
			}
			continue
		}
		record.Digest, err = entryDigest(dir, entry.Name(), mode)
		if err != nil {
			return err
		}
		*result = append(*result, record)
	}
	return nil
}

func entryDigest(dir storageworkspaces.DirectoryHandle, name string, mode os.FileMode) (string, error) {
	if mode&os.ModeSymlink != 0 {
		link, err := dir.ReadLink(name)
		return digest(link), err
	}
	if !mode.IsRegular() {
		return "", errors.New("checkout contains an unsupported special file")
	}
	f, err := dir.OpenFile(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 1<<30+1))
	if err != nil {
		return "", err
	}
	if n > 1<<30 {
		return "", errors.New("checkout file exceeds repair size limit")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func inspectGitRegistration(ctx context.Context, repository, path string) (GitState, storageworkspaces.DirectoryHandle, error) {
	var result GitState
	var err error
	result.CommonDir, err = runGit(ctx, path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return result, nil, err
	}
	common, err := runGit(ctx, repository, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return result, nil, err
	}
	if common != result.CommonDir {
		return result, nil, errors.New("checkout belongs to another Git common directory")
	}
	result.GitDir, err = runGit(ctx, path, "rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return result, nil, err
	}
	if filepath.Dir(filepath.Dir(result.GitDir)) != common || filepath.Base(filepath.Dir(result.GitDir)) != "worktrees" {
		return result, nil, errors.New("checkout is not a registered linked worktree")
	}
	admin, err := storageworkspaces.OpenDirectoryNoFollow(common, result.GitDir)
	if err != nil {
		return result, nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = admin.Close()
		}
	}()
	backlink, err := admin.ReadFile("gitdir")
	if err != nil {
		return result, nil, err
	}
	if strings.TrimSpace(string(backlink)) != filepath.Join(path, ".git") {
		return result, nil, errors.New("worktree administrative backlink differs from checkout")
	}
	result.Head, err = runGit(ctx, path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return result, nil, err
	}
	branch, err := runGit(ctx, path, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		return result, nil, errors.New("repair requires an attached checkout")
	}
	if !strings.HasPrefix(branch, "refs/heads/") {
		return result, nil, errors.New("checkout HEAD does not name a local branch")
	}
	result.Branch = strings.TrimPrefix(branch, "refs/heads/")
	if err := verifyRegistration(ctx, repository, path, branch, result.Head); err != nil {
		return result, nil, err
	}
	keep = true
	return result, admin, nil
}
