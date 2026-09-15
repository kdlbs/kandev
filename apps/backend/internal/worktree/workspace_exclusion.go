package worktree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/kandev/kandev/internal/common/gitref"
)

const (
	managedWorkspaceExclusionPrefix = "# kandev managed nested worktree: "
	managedWorkspaceIncludeHeader   = "# kandev managed nested workspace include\n"
	managedWorkspaceBaseHashPrefix  = "# kandev base excludes sha256: "
)

// ErrNestedWorkspaceExclusionNotEffective means outer Git rules still expose a
// nested worktree to ordinary staging.
var ErrNestedWorkspaceExclusionNotEffective = errors.New("nested workspace exclusion is not effective")

type nestedWorkspaceExclusionTarget struct {
	outerRoot        string
	commonConfigPath string
	configKey        string
	includePath      string
	excludePath      string
	pattern          string
}

// addNestedWorkspaceExclusion installs a worktree-scoped Git exclude file for
// the outer worktree that contains worktreePath. The conditional include is
// written to the shared repository config, but its gitdir condition matches
// only that outer worktree. The source repository's tracked ignore files,
// common info/exclude, and global Git configuration remain unchanged.
func (m *Manager) addNestedWorkspaceExclusion(ctx context.Context, _, worktreePath string) (bool, error) {
	target, err := m.nestedWorkspaceExcludeTarget(worktreePath)
	if err != nil || target.includePath == "" {
		return false, err
	}

	release := m.lockWorkspaceExclusion(target.commonConfigPath)
	defer release()

	includePaths, err := m.workspaceExclusionIncludes(ctx, target)
	if err != nil {
		return false, err
	}
	includeExists := containsPath(includePaths, target.includePath)
	changed := false
	if !includeExists {
		if err := m.createNestedWorkspaceExclusion(ctx, target); err != nil {
			return false, err
		}
		changed = true
	} else if _, err := os.Stat(target.excludePath); err != nil {
		return false, fmt.Errorf("inspect managed Git exclude file: %w", err)
	}

	content, err := os.ReadFile(target.excludePath)
	if err != nil {
		return false, fmt.Errorf("read managed Git exclude file: %w", err)
	}
	previousContent := content
	updated, entryChanged := appendManagedWorkspaceExclusion(string(content), target.pattern)
	if entryChanged {
		if err := os.WriteFile(target.excludePath, []byte(updated), 0o644); err != nil {
			return false, fmt.Errorf("update managed Git exclude file: %w", err)
		}
		changed = true
	}
	if err := m.verifyNestedWorkspaceExclusion(ctx, target); err != nil {
		rollbackErr := m.rollbackProvisionalNestedWorkspaceExclusion(ctx, target, includeExists, previousContent, entryChanged)
		return false, errors.Join(err, rollbackErr)
	}
	return changed, nil
}

func (m *Manager) rollbackProvisionalNestedWorkspaceExclusion(ctx context.Context, target nestedWorkspaceExclusionTarget, includeExists bool, previousContent []byte, entryChanged bool) error {
	var rollbackErr error
	if entryChanged {
		if err := os.WriteFile(target.excludePath, previousContent, 0o644); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore managed Git exclude file: %w", err))
		}
	}
	if includeExists {
		return rollbackErr
	}
	if err := m.removeWorkspaceExclusionInclude(ctx, target); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if err := os.Remove(target.includePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove managed Git include file: %w", err))
	}
	if err := os.Remove(target.excludePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove managed Git exclude file: %w", err))
	}
	return rollbackErr
}

// removeNestedWorkspaceExclusion removes only the managed entry for one
// nested worktree. A generated exclude file remains in place when its content
// no longer matches the captured baseline, preserving user edits made to it.
func (m *Manager) removeNestedWorkspaceExclusion(ctx context.Context, _, worktreePath string) error {
	target, err := m.nestedWorkspaceExcludeTarget(worktreePath)
	if err != nil || target.includePath == "" {
		return err
	}

	release := m.lockWorkspaceExclusion(target.commonConfigPath)
	defer release()
	includePaths, err := m.workspaceExclusionIncludes(ctx, target)
	if err != nil {
		return err
	}
	if !containsPath(includePaths, target.includePath) {
		return nil
	}
	content, err := os.ReadFile(target.excludePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read managed Git exclude file: %w", err)
	}
	updated, removed := removeManagedWorkspaceExclusion(string(content), target.pattern)
	if !removed {
		return nil
	}
	return m.finishRemovingNestedWorkspaceExclusion(ctx, target, updated)
}

func (m *Manager) createNestedWorkspaceExclusion(ctx context.Context, target nestedWorkspaceExclusionTarget) error {
	base, err := m.readConfiguredExcludes(ctx, target.outerRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target.excludePath), 0o755); err != nil {
		return fmt.Errorf("create Git exclude directory: %w", err)
	}
	if err := os.WriteFile(target.excludePath, base, 0o644); err != nil {
		return fmt.Errorf("write managed Git exclude file: %w", err)
	}
	if err := writeWorkspaceExclusionInclude(target.includePath, target.excludePath, base); err != nil {
		return err
	}
	if err := m.addWorkspaceExclusionInclude(ctx, target); err != nil {
		_ = os.Remove(target.includePath)
		_ = os.Remove(target.excludePath)
		return err
	}
	return nil
}

func (m *Manager) finishRemovingNestedWorkspaceExclusion(ctx context.Context, target nestedWorkspaceExclusionTarget, updated string) error {
	if hasManagedWorkspaceExclusions(updated) {
		if err := os.WriteFile(target.excludePath, []byte(updated), 0o644); err != nil {
			return fmt.Errorf("update managed Git exclude file: %w", err)
		}
		return nil
	}

	baseline, baselineOK := workspaceExclusionBaseline(target.includePath)
	if baselineOK && sha256.Sum256([]byte(updated)) == baseline {
		if err := m.removeWorkspaceExclusionInclude(ctx, target); err != nil {
			return err
		}
		if err := os.Remove(target.includePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove managed Git include file: %w", err)
		}
		if err := os.Remove(target.excludePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove managed Git exclude file: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(target.excludePath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("preserve managed Git exclude file: %w", err)
	}
	return nil
}

func (m *Manager) nestedWorkspaceExcludeTarget(worktreePath string) (nestedWorkspaceExclusionTarget, error) {
	outerRoot, err := findContainingGitRoot(worktreePath)
	if err != nil || outerRoot == "" {
		return nestedWorkspaceExclusionTarget{}, err
	}
	worktreePath, err = filepath.Abs(filepath.Clean(worktreePath))
	if err != nil {
		return nestedWorkspaceExclusionTarget{}, fmt.Errorf("resolve worktree path: %w", err)
	}
	relative, err := filepath.Rel(outerRoot, worktreePath)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nestedWorkspaceExclusionTarget{}, nil
	}
	gitDir, err := gitref.ResolveGitDir(outerRoot)
	if err != nil {
		return nestedWorkspaceExclusionTarget{}, fmt.Errorf("resolve outer worktree Git directory: %w", err)
	}
	gitDir, err = filepath.Abs(filepath.Clean(gitDir))
	if err != nil {
		return nestedWorkspaceExclusionTarget{}, fmt.Errorf("resolve outer worktree Git directory: %w", err)
	}
	commonDir := gitref.ResolveCommonGitDir(gitDir)
	commonDir, err = filepath.Abs(filepath.Clean(commonDir))
	if err != nil {
		return nestedWorkspaceExclusionTarget{}, fmt.Errorf("resolve common Git directory: %w", err)
	}
	conditionPrefix := "gitdir:"
	if runtime.GOOS == windowsGOOS {
		conditionPrefix = "gitdir/i:"
	}
	condition := conditionPrefix + filepath.ToSlash(gitDir)
	pattern := "/" + strings.Trim(filepath.ToSlash(relative), "/") + "/"
	includePath := filepath.Join(gitDir, "kandev-nested-workspace.include")
	return nestedWorkspaceExclusionTarget{
		outerRoot:        outerRoot,
		commonConfigPath: filepath.Join(commonDir, "config"),
		configKey:        "includeIf." + condition + ".path",
		includePath:      includePath,
		excludePath:      filepath.Join(gitDir, "kandev-nested-workspace.exclude"),
		pattern:          pattern,
	}, nil
}

func findContainingGitRoot(worktreePath string) (string, error) {
	path, err := filepath.Abs(filepath.Clean(worktreePath))
	if err != nil {
		return "", fmt.Errorf("resolve worktree path: %w", err)
	}
	for candidate := filepath.Dir(path); ; candidate = filepath.Dir(candidate) {
		gitPath := filepath.Join(candidate, ".git")
		if _, err := os.Lstat(gitPath); err == nil {
			if gitRootUsable(candidate) {
				return candidate, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect containing Git directory: %w", err)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", nil
		}
	}
}

func gitRootUsable(root string) bool {
	gitDir, err := gitref.ResolveGitDir(root)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(gitDir, "HEAD"))
	return err == nil
}

func (m *Manager) lockWorkspaceExclusion(configPath string) func() {
	lock := m.getRepoLock(configPath)
	lock.Lock()
	return func() {
		lock.Unlock()
		m.releaseRepoLock(configPath)
	}
}

func (m *Manager) workspaceExclusionIncludes(ctx context.Context, target nestedWorkspaceExclusionTarget) ([]string, error) {
	output, err := runGitCmdOutput(ctx, m.newNonInteractiveGitCmd(ctx, filepath.Dir(target.commonConfigPath), "config", "--file", target.commonConfigPath, "--get-all", target.configKey))
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("read Git workspace include: %w", err)
		}
	}
	var paths []string
	for _, line := range strings.Split(string(output), "\n") {
		if path := strings.TrimSpace(line); path != "" {
			paths = append(paths, filepath.Clean(path))
		}
	}
	return paths, nil
}

func (m *Manager) addWorkspaceExclusionInclude(ctx context.Context, target nestedWorkspaceExclusionTarget) error {
	cmd := m.newNonInteractiveGitCmd(ctx, filepath.Dir(target.commonConfigPath), "config", "--file", target.commonConfigPath, "--add", target.configKey, target.includePath)
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("register Git workspace include: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) removeWorkspaceExclusionInclude(ctx context.Context, target nestedWorkspaceExclusionTarget) error {
	cmd := m.newNonInteractiveGitCmd(ctx, filepath.Dir(target.commonConfigPath), "config", "--file", target.commonConfigPath, "--unset", target.configKey, regexp.QuoteMeta(target.includePath))
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("remove Git workspace include: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) readConfiguredExcludes(ctx context.Context, worktreePath string) ([]byte, error) {
	output, err := runGitCmdOutput(ctx, m.newNonInteractiveGitCmd(ctx, worktreePath, "config", "--path", "--get", "core.excludesFile"))
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("read configured Git excludes: %w", err)
		}
		output = nil
	}
	path := strings.TrimSpace(string(output))
	if path == "" {
		path = implicitGlobalGitIgnorePath()
		if path == "" {
			return []byte{}, nil
		}
	}
	if !filepath.IsAbs(path) {
		path, err = filepath.Abs(filepath.Join(worktreePath, path))
		if err != nil {
			return nil, fmt.Errorf("resolve configured Git excludes: %w", err)
		}
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read configured Git excludes: %w", err)
	}
	return content, nil
}

func implicitGlobalGitIgnorePath() string {
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "git", "ignore")
}

func (m *Manager) verifyNestedWorkspaceExclusion(ctx context.Context, target nestedWorkspaceExclusionTarget) error {
	relative := strings.Trim(target.pattern, "/")
	if relative == "" {
		return fmt.Errorf("%w: empty nested workspace path", ErrNestedWorkspaceExclusionNotEffective)
	}
	trackedOutput, trackedErr := runGitCmdOutput(ctx, m.newNonInteractiveGitCmd(ctx, target.outerRoot, "ls-files", "--error-unmatch", "--", relative))
	if trackedErr == nil && strings.TrimSpace(string(trackedOutput)) != "" {
		return fmt.Errorf("%w: destination %q is already tracked by the outer worktree", ErrNestedWorkspaceExclusionNotEffective, relative)
	}
	if trackedErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(trackedErr, &exitErr) || exitErr.ExitCode() != 1 {
			return fmt.Errorf("verify outer worktree destination %q: %w", relative, trackedErr)
		}
	}
	probe := filepath.ToSlash(filepath.Join(relative, ".kandev-ignore-probe"))
	_, err := runGitCmdCombinedOutput(ctx, m.newNonInteractiveGitCmd(ctx, target.outerRoot, "check-ignore", "--no-index", "--quiet", "--", probe))
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return fmt.Errorf("%w: tracked ignore rules do not protect %q", ErrNestedWorkspaceExclusionNotEffective, relative)
	}
	return fmt.Errorf("verify nested workspace exclusion for %q: %w", relative, err)
}

func writeWorkspaceExclusionInclude(includePath, excludePath string, baseline []byte) error {
	if err := os.MkdirAll(filepath.Dir(includePath), 0o755); err != nil {
		return fmt.Errorf("create Git include directory: %w", err)
	}
	hash := sha256.Sum256(baseline)
	content := managedWorkspaceIncludeHeader + managedWorkspaceBaseHashPrefix + hex.EncodeToString(hash[:]) + "\n[core]\n\texcludesFile = " + quoteGitConfigValue(excludePath) + "\n"
	if err := os.WriteFile(includePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write managed Git include file: %w", err)
	}
	return nil
}

func quoteGitConfigValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func workspaceExclusionBaseline(includePath string) ([32]byte, bool) {
	content, err := os.ReadFile(includePath)
	if err != nil {
		return [32]byte{}, false
	}
	for _, line := range strings.Split(string(content), "\n") {
		encoded, found := strings.CutPrefix(line, managedWorkspaceBaseHashPrefix)
		if !found {
			continue
		}
		decoded, err := hex.DecodeString(strings.TrimSpace(encoded))
		if err != nil || len(decoded) != sha256.Size {
			return [32]byte{}, false
		}
		var hash [32]byte
		copy(hash[:], decoded)
		return hash, true
	}
	return [32]byte{}, false
}

func appendManagedWorkspaceExclusion(content, pattern string) (string, bool) {
	marker := managedWorkspaceExclusionPrefix + pattern
	entry := marker + "\n" + pattern
	if strings.Contains(content, entry) {
		return content, false
	}
	prefix := ""
	if content != "" && !strings.HasSuffix(content, "\n") {
		prefix = "\n"
	}
	return content + prefix + entry + "\n", true
}

func removeManagedWorkspaceExclusion(content, pattern string) (string, bool) {
	entry := managedWorkspaceExclusionPrefix + pattern + "\n" + pattern
	for _, candidate := range []string{entry + "\n", "\n" + entry, entry} {
		if strings.Contains(content, candidate) {
			return strings.Replace(content, candidate, "", 1), true
		}
	}
	return content, false
}

func hasManagedWorkspaceExclusions(content string) bool {
	return strings.Contains(content, managedWorkspaceExclusionPrefix)
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}
