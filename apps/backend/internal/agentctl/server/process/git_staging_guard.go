package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// errAttachedRepositoryStaging protects an attached repository from becoming
// an embedded repository entry in the outer worktree index.
var errAttachedRepositoryStaging = errors.New("attached repository cannot be staged as an outer gitlink")

// attachedRepositoryPaths returns registered repository roots represented below
// the operator's workspace. Source roots can be real nested worktrees or
// directory links to repositories outside the workspace.
func (g *GitOperator) attachedRepositoryPaths() ([]string, error) {
	if g.workspaceTracker == nil || g.workDir == "" {
		return nil, nil
	}
	workspaceRoot, err := filepath.Abs(filepath.Clean(g.workDir))
	if err != nil {
		return nil, fmt.Errorf("resolve staging workspace: %w", err)
	}
	g.workspaceTracker.mu.RLock()
	sourceRoots := append([]string(nil), g.workspaceTracker.allowedSourceRoots...)
	g.workspaceTracker.mu.RUnlock()
	paths := make(map[string]struct{}, len(sourceRoots))
	add := func(candidate string) {
		relative, relErr := filepath.Rel(workspaceRoot, candidate)
		if relErr != nil || relative == "." || pathEscapesRoot(relative) {
			return
		}
		if !isGitRepositoryPath(candidate) {
			return
		}
		paths[filepath.ToSlash(filepath.Clean(relative))] = struct{}{}
	}
	for _, sourceRoot := range sourceRoots {
		resolved, resolveErr := filepath.EvalSymlinks(filepath.Clean(sourceRoot))
		if resolveErr != nil {
			continue
		}
		relative, relErr := filepath.Rel(workspaceRoot, resolved)
		if relErr == nil && relative != "." && !pathEscapesRoot(relative) {
			add(filepath.Join(workspaceRoot, relative))
		}
	}
	for _, child := range scanRepositorySubdirs(workspaceRoot, sourceRoots) {
		add(filepath.Join(workspaceRoot, child.name))
	}
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func isGitRepositoryPath(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	_, err = os.Lstat(filepath.Join(path, ".git"))
	return err == nil
}

func (g *GitOperator) attachedRepositoryGitlinks(ctx context.Context, paths []string) ([]string, error) {
	gitlinks := make([]string, 0, len(paths))
	for _, path := range paths {
		output, err := g.runGitCommand(ctx, "ls-files", "--stage", "--", filepath.FromSlash(path))
		if err != nil {
			return nil, fmt.Errorf("inspect attached repository %q: %w", path, err)
		}
		if hasGitlinkAtPath(output, path) {
			gitlinks = append(gitlinks, path)
		}
	}
	return gitlinks, nil
}

func hasGitlinkAtPath(output, path string) bool {
	for _, line := range strings.Split(output, "\n") {
		metadata, entryPath, ok := strings.Cut(line, "\t")
		if !ok || !strings.HasPrefix(metadata, "160000 ") {
			continue
		}
		if filepath.ToSlash(filepath.Clean(entryPath)) == path {
			return true
		}
	}
	return false
}

func (g *GitOperator) checkAttachedRepositoryStaging(ctx context.Context) error {
	paths, err := g.attachedRepositoryPaths()
	if err != nil || len(paths) == 0 {
		return err
	}
	gitlinks, err := g.attachedRepositoryGitlinks(ctx, paths)
	if err != nil {
		return err
	}
	if len(gitlinks) > 0 {
		return fmt.Errorf("%w: %s is already tracked", errAttachedRepositoryStaging, gitlinks[0])
	}
	for _, path := range paths {
		probe := filepath.ToSlash(filepath.Join(path, ".kandev-stage-probe"))
		if _, err := g.runGitCommand(ctx, "check-ignore", "--no-index", "--quiet", "--", probe); err != nil {
			if isGitExitCode(err, 1) {
				return fmt.Errorf("%w: %s is not protected by Git excludes", errAttachedRepositoryStaging, path)
			}
			return fmt.Errorf("verify attached repository exclusion %q: %w", path, err)
		}
	}
	return nil
}

func (g *GitOperator) finishAttachedRepositoryStaging(ctx context.Context) error {
	paths, err := g.attachedRepositoryPaths()
	if err != nil || len(paths) == 0 {
		return err
	}
	gitlinks, err := g.attachedRepositoryGitlinks(ctx, paths)
	if err != nil || len(gitlinks) == 0 {
		return err
	}
	resetArgs := append([]string{"reset", "HEAD", "--"}, gitlinks...)
	if _, resetErr := g.runGitCommand(ctx, resetArgs...); resetErr != nil {
		return errors.Join(
			fmt.Errorf("%w: %s", errAttachedRepositoryStaging, strings.Join(gitlinks, ", ")),
			fmt.Errorf("remove attached repository gitlink: %w", resetErr),
		)
	}
	return fmt.Errorf("%w: %s was removed from the outer index", errAttachedRepositoryStaging, strings.Join(gitlinks, ", "))
}

func isGitExitCode(err error, code int) bool {
	var exitErr interface{ ExitCode() int }
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}
