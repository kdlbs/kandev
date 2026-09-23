package worktree

import (
	"bytes"
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

type nestedWorkspaceExclusionState struct {
	originalIncludeExists     bool
	previousInclude           []byte
	previousIncludeFileExists bool
	previousExclude           []byte
	previousExcludeFileExists bool
	changed                   bool
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

	state, err := m.prepareNestedWorkspaceExclusion(ctx, target)
	if err != nil {
		return false, err
	}
	changed, err := m.reconcileNestedWorkspaceExclusionFiles(ctx, target)
	if err != nil {
		return false, err
	}
	state.changed = state.changed || changed
	if err := m.verifyNestedWorkspaceExclusion(ctx, target); err != nil {
		rollbackErr := m.rollbackProvisionalNestedWorkspaceExclusion(ctx, target, state.originalIncludeExists, state.previousInclude, state.previousIncludeFileExists, state.previousExclude, state.previousExcludeFileExists)
		return false, errors.Join(err, rollbackErr)
	}
	return state.changed, nil
}

func (m *Manager) prepareNestedWorkspaceExclusion(ctx context.Context, target nestedWorkspaceExclusionTarget) (nestedWorkspaceExclusionState, error) {
	includePaths, err := m.workspaceExclusionIncludes(ctx, target)
	if err != nil {
		return nestedWorkspaceExclusionState{}, err
	}
	includeExists := containsPath(includePaths, target.includePath)
	previousInclude, previousIncludeFileExists, err := readOptionalWorkspaceExclusionFile(target.includePath)
	if err != nil {
		return nestedWorkspaceExclusionState{}, fmt.Errorf("inspect managed Git include file: %w", err)
	}
	previousExclude, previousExcludeFileExists, err := readOptionalWorkspaceExclusionFile(target.excludePath)
	if err != nil {
		return nestedWorkspaceExclusionState{}, fmt.Errorf("inspect managed Git exclude file: %w", err)
	}
	state := nestedWorkspaceExclusionState{
		originalIncludeExists:     includeExists,
		previousInclude:           previousInclude,
		previousIncludeFileExists: previousIncludeFileExists,
		previousExclude:           previousExclude,
		previousExcludeFileExists: previousExcludeFileExists,
	}
	if includeExists && !previousExcludeFileExists {
		// A stale conditional include must not keep shadowing the effective
		// excludes file. Remove it before rebuilding the managed pair.
		if err := m.removeWorkspaceExclusionInclude(ctx, target); err != nil {
			return nestedWorkspaceExclusionState{}, err
		}
		if err := os.Remove(target.includePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nestedWorkspaceExclusionState{}, fmt.Errorf("remove stale managed Git include file: %w", err)
		}
		includeExists = false
		state.changed = true
	}
	if !includeExists {
		if err := m.createNestedWorkspaceExclusion(ctx, target); err != nil {
			return nestedWorkspaceExclusionState{}, err
		}
		state.changed = true
	}
	return state, nil
}

func (m *Manager) reconcileNestedWorkspaceExclusionFiles(ctx context.Context, target nestedWorkspaceExclusionTarget) (bool, error) {
	content, err := os.ReadFile(target.excludePath)
	if err != nil {
		return false, fmt.Errorf("read managed Git exclude file: %w", err)
	}
	baseline, err := m.readConfiguredExcludesExcept(ctx, target.outerRoot, target.excludePath)
	if err != nil {
		return false, err
	}
	updated := refreshManagedWorkspaceExclusion(string(content), baseline, target.includePath)
	changed := false
	if currentInclude, readErr := os.ReadFile(target.includePath); readErr != nil || !bytes.Equal(currentInclude, managedWorkspaceIncludeContent(target.excludePath, baseline)) {
		if err := writeWorkspaceExclusionInclude(target.includePath, target.excludePath, baseline); err != nil {
			return false, err
		}
		changed = true
	}
	updated, _ = appendManagedWorkspaceExclusion(updated, target.pattern)
	if updated != string(content) {
		if err := os.WriteFile(target.excludePath, []byte(updated), 0o644); err != nil {
			return false, fmt.Errorf("update managed Git exclude file: %w", err)
		}
		changed = true
	}
	return changed, nil
}

func (m *Manager) rollbackProvisionalNestedWorkspaceExclusion(
	ctx context.Context,
	target nestedWorkspaceExclusionTarget,
	includeExists bool,
	previousInclude []byte,
	previousIncludeFileExists bool,
	previousExclude []byte,
	previousExcludeFileExists bool,
) error {
	var rollbackErr error
	if previousExcludeFileExists {
		if err := os.WriteFile(target.excludePath, previousExclude, 0o644); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore managed Git exclude file: %w", err))
		}
	} else if err := os.Remove(target.excludePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove managed Git exclude file: %w", err))
	}
	if includeExists {
		if err := m.ensureWorkspaceExclusionInclude(ctx, target); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	} else if err := m.removeWorkspaceExclusionInclude(ctx, target); err != nil {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if previousIncludeFileExists {
		if err := os.WriteFile(target.includePath, previousInclude, 0o644); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore managed Git include file: %w", err))
		}
	} else if err := os.Remove(target.includePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove managed Git include file: %w", err))
	}
	return rollbackErr
}

func (m *Manager) ensureWorkspaceExclusionInclude(ctx context.Context, target nestedWorkspaceExclusionTarget) error {
	paths, err := m.workspaceExclusionIncludes(ctx, target)
	if err != nil {
		return err
	}
	if containsPath(paths, target.includePath) {
		return nil
	}
	return m.addWorkspaceExclusionInclude(ctx, target)
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
	base, err := m.readConfiguredExcludesExcept(ctx, target.outerRoot, target.excludePath)
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
	paths, err := m.workspaceExclusionIncludes(ctx, target)
	if err != nil {
		return err
	}
	if !containsPath(paths, target.includePath) {
		return nil
	}
	cmd := m.newNonInteractiveGitCmd(ctx, filepath.Dir(target.commonConfigPath), "config", "--file", target.commonConfigPath, "--unset", target.configKey, regexp.QuoteMeta(target.includePath))
	if output, err := runGitCmdCombinedOutput(ctx, cmd); err != nil {
		return fmt.Errorf("remove Git workspace include: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) readConfiguredExcludesExcept(ctx context.Context, worktreePath, excludedPath string) ([]byte, error) {
	output, err := runGitCmdOutput(ctx, m.newNonInteractiveGitCmd(ctx, worktreePath, "config", "--path", "--get-all", "core.excludesFile"))
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("read configured Git excludes: %w", err)
		}
		output = nil
	}
	path := ""
	for _, line := range strings.Split(string(output), "\n") {
		candidate := strings.TrimSpace(line)
		if candidate == "" {
			continue
		}
		resolved, resolveErr := resolveConfiguredExcludesPath(worktreePath, candidate)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if excludedPath != "" {
			excluded, excludeErr := filepath.Abs(filepath.Clean(excludedPath))
			if excludeErr != nil {
				return nil, fmt.Errorf("resolve managed Git excludes: %w", excludeErr)
			}
			if filepath.Clean(resolved) == filepath.Clean(excluded) {
				continue
			}
		}
		path = resolved
	}
	if path == "" {
		path = implicitGlobalGitIgnorePath()
		if path == "" {
			return []byte{}, nil
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

func resolveConfiguredExcludesPath(worktreePath, path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Abs(filepath.Clean(path))
	}
	resolved, err := filepath.Abs(filepath.Join(worktreePath, path))
	if err != nil {
		return "", fmt.Errorf("resolve configured Git excludes: %w", err)
	}
	return resolved, nil
}

func readOptionalWorkspaceExclusionFile(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return content, true, nil
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
	if err := os.WriteFile(includePath, managedWorkspaceIncludeContent(excludePath, baseline), 0o644); err != nil {
		return fmt.Errorf("write managed Git include file: %w", err)
	}
	return nil
}

func managedWorkspaceIncludeContent(excludePath string, baseline []byte) []byte {
	hash := sha256.Sum256(baseline)
	content := managedWorkspaceIncludeHeader + managedWorkspaceBaseHashPrefix + hex.EncodeToString(hash[:]) + "\n[core]\n\texcludesFile = " + quoteGitConfigValue(excludePath) + "\n"
	return []byte(content)
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

func refreshManagedWorkspaceExclusion(content string, baseline []byte, includePath string) string {
	inherited := stripManagedWorkspaceExclusions(content)
	refreshed := ""
	if baseHash, ok := workspaceExclusionBaseline(includePath); ok && equivalentWorkspaceExclusionContent([]byte(inherited), baseline, baseHash) {
		refreshed = string(baseline)
	} else {
		refreshed = mergeWorkspaceExclusionContent(string(baseline), inherited)
	}
	for _, pattern := range managedWorkspaceExclusionPatterns(content) {
		refreshed, _ = appendManagedWorkspaceExclusion(refreshed, pattern)
	}
	return refreshed
}

func stripManagedWorkspaceExclusions(content string) string {
	lines := strings.SplitAfter(content, "\n")
	var builder strings.Builder
	skipNext := false
	for _, line := range lines {
		trimmed := strings.TrimSuffix(line, "\n")
		if strings.HasPrefix(trimmed, managedWorkspaceExclusionPrefix) {
			skipNext = true
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		builder.WriteString(line)
	}
	return builder.String()
}

func managedWorkspaceExclusionPatterns(content string) []string {
	lines := strings.SplitAfter(content, "\n")
	patterns := make([]string, 0)
	for index, line := range lines {
		marker := strings.TrimSuffix(line, "\n")
		pattern, found := strings.CutPrefix(marker, managedWorkspaceExclusionPrefix)
		if !found || pattern == "" || index+1 >= len(lines) {
			continue
		}
		entryPattern := strings.TrimSuffix(lines[index+1], "\n")
		if entryPattern == pattern {
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}

func equivalentWorkspaceExclusionContent(content, baseline []byte, baselineHash [32]byte) bool {
	if bytes.Equal(content, baseline) || bytes.Equal(bytes.TrimSuffix(content, []byte("\n")), bytes.TrimSuffix(baseline, []byte("\n"))) {
		return true
	}
	return sha256.Sum256(content) == baselineHash || sha256.Sum256(bytes.TrimSuffix(content, []byte("\n"))) == baselineHash
}

func mergeWorkspaceExclusionContent(baseline, inherited string) string {
	if baseline == "" && inherited == "" {
		return ""
	}
	seen := make(map[string]struct{})
	lines := make([]string, 0)
	appendLines := func(content string) {
		for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
			if line == "" {
				if len(lines) == 0 || lines[len(lines)-1] == "" {
					continue
				}
			}
			if _, exists := seen[line]; exists {
				continue
			}
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	appendLines(baseline)
	appendLines(inherited)
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
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
