package skill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// cleanKandevSkills removes every kandev-* directory from the agent's
// project skill directory inside a worktree. User-managed skill dirs
// (anything not prefixed with "kandev-") are left untouched. A missing
// skills directory is not an error.
func cleanKandevSkills(worktreePath, projectSkillDir string) error {
	skillsDir := filepath.Join(worktreePath, projectSkillDir)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "kandev-") {
			if rmErr := os.RemoveAll(filepath.Join(skillsDir, entry.Name())); rmErr != nil {
				return rmErr
			}
		}
	}
	return nil
}

// injectSkills performs a clean-slate injection of skills into a
// worktree:
//  1. Removes every kandev-* directory in the project skill dir.
//  2. Writes each skill's SKILL.md under DirName(slug)/.
//  3. Best-effort appends the kandev-* pattern to .git/info/exclude.
//
// Skills with invalid slugs are skipped silently — caller upstream
// already validates user-facing input.
//
// DirName is not injective: an already-prefixed slug ("kandev-protocol")
// and its unprefixed counterpart ("protocol") resolve to the same
// directory. When two skills in this manifest collide that way, the
// first one (in manifest order) claims the directory; every later
// colliding skill is skipped and logged rather than silently
// overwriting or mixing support files into the first skill's directory.
func injectSkills(worktreePath, projectSkillDir string, skills []Skill, log *logger.Logger) error {
	if err := cleanKandevSkills(worktreePath, projectSkillDir); err != nil {
		return fmt.Errorf("clean kandev skills: %w", err)
	}
	skillsDir := filepath.Join(worktreePath, projectSkillDir)
	claimed := make(map[string]string, len(skills))
	for _, sk := range skills {
		if !isValidSlug(sk.Slug) {
			continue
		}
		dirName := DirName(sk.Slug)
		if owner, ok := claimed[dirName]; ok {
			if log != nil {
				log.Warn("skipping skill: directory name collides with an already-injected skill",
					zap.String("slug", sk.Slug),
					zap.String("collides_with_slug", owner),
					zap.String("dir", dirName))
			}
			continue
		}
		claimed[dirName] = sk.Slug
		dir := filepath.Join(skillsDir, dirName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir skill %s: %w", sk.Slug, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(renderSkillMarkdown(sk)), 0o644); err != nil {
			return fmt.Errorf("write SKILL.md for %s: %w", sk.Slug, err)
		}
		if err := writeSkillFiles(dir, sk.Files); err != nil {
			return fmt.Errorf("write support files for %s: %w", sk.Slug, err)
		}
	}
	// git-exclude is best-effort: a worktree without .git (tests, fresh
	// dirs) just means the file won't be created. Never fail injection
	// over it.
	_ = ensureGitExclude(worktreePath, projectSkillDir)
	return nil
}

func writeSkillFiles(skillDir string, files []SkillFile) error {
	absSkillDir, err := filepath.Abs(skillDir)
	if err != nil {
		return err
	}
	absSkillDir, err = filepath.EvalSymlinks(absSkillDir)
	if err != nil {
		return err
	}
	for _, file := range files {
		rel, ok := cleanRelativeSkillFilePath(file.Path)
		if !ok {
			continue
		}
		target, ok, err := secureSkillFileDestination(absSkillDir, filepath.FromSlash(rel))
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func secureSkillFileDestination(absSkillDir, rel string) (string, bool, error) {
	target, err := filepath.Abs(filepath.Join(absSkillDir, rel))
	if err != nil {
		return "", false, err
	}
	if !pathStaysInside(absSkillDir, target) {
		return "", false, nil
	}
	parent := filepath.Dir(target)
	if !parentChainAllowsSkillWrite(absSkillDir, parent) {
		return "", false, nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", false, err
	}
	canonParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", false, nil
	}
	final := filepath.Join(canonParent, filepath.Base(target))
	if !pathStaysInside(absSkillDir, final) {
		return "", false, nil
	}
	return final, true, nil
}

func pathStaysInside(absBase, target string) bool {
	rel, err := filepath.Rel(absBase, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func parentChainAllowsSkillWrite(absSkillDir, parent string) bool {
	rel, err := filepath.Rel(absSkillDir, parent)
	if err != nil {
		return false
	}
	if rel == "." || rel == "" {
		return true
	}
	cur := absSkillDir
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	return true
}

// ensureGitExclude appends a "kandev-*" glob for the project skill
// directory to the repository's info/exclude file so injected skill
// directories never appear as dirty files in git status. Idempotent.
//
// The pattern is derived from the path git actually sees post-symlink
// resolution (see gitExcludePattern), and the file it's written to is
// resolved to the shared common gitdir (see resolveGitDir) so linked
// worktrees don't write to a location git never reads.
func ensureGitExclude(worktreePath, projectSkillDir string) error {
	gitDir, err := resolveGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve git dir: %w", err)
	}
	excludeFile := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludeFile), 0o755); err != nil {
		return fmt.Errorf("mkdir info dir: %w", err)
	}

	pattern, ok := gitExcludePattern(worktreePath, projectSkillDir)
	if !ok {
		return nil
	}

	if data, err := os.ReadFile(excludeFile); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == pattern {
				return nil
			}
		}
	}

	f, err := os.OpenFile(excludeFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open exclude file: %w", err)
	}
	defer func() { _ = f.Close() }()
	_, err = fmt.Fprintf(f, "%s\n", pattern)
	return err
}

// gitExcludePattern derives the info/exclude glob for the project skill
// directory as git actually sees it: relative to the worktree root,
// after resolving symlinks (a symlinked projectSkillDir, e.g. this
// repo's own ".claude/skills" -> "../.agents/skills", would otherwise
// make the literal pattern never match, since writes through it land on
// the resolved path).
//
// Falls back to the literal "<projectSkillDir>/kandev-*" when resolution
// fails — e.g. the skill dir doesn't exist yet, which happens whenever
// injectSkills is called with an empty skill list — so non-symlink
// behaviour stays byte-identical to before this path existed. Returns
// ok=false when the resolved target falls outside the worktree: nothing
// there for git to exclude.
func gitExcludePattern(worktreePath, projectSkillDir string) (string, bool) {
	fallback := projectSkillDir + "/kandev-*"

	resolvedWorktree, err := filepath.EvalSymlinks(worktreePath)
	if err != nil {
		return fallback, true
	}
	resolvedSkillDir, err := filepath.EvalSymlinks(filepath.Join(worktreePath, projectSkillDir))
	if err != nil {
		return fallback, true
	}

	rel, err := filepath.Rel(resolvedWorktree, resolvedSkillDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	return filepath.ToSlash(rel) + "/kandev-*", true
}

// resolveGitDir returns the git directory whose info/exclude git will
// actually read for a worktree path.
//
// For a normal repository, .git is a directory and is returned as-is.
// For a linked worktree, .git is a file of the form
// "gitdir: /path/to/repo/.git/worktrees/<name>". That per-worktree
// gitdir has its own info/ subdirectory, but git does not read
// info/exclude from it — it reads $GIT_COMMON_DIR/info/exclude, found
// via the gitdir's "commondir" file. Resolve through that file so the
// exclude pattern lands where git looks; without a commondir file (e.g.
// a non-standard layout), fall back to the gitdir itself.
func resolveGitDir(worktreePath string) (string, error) {
	gitPath := filepath.Join(worktreePath, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		return "", fmt.Errorf(".git not found in %s: %w", worktreePath, err)
	}
	if info.IsDir() {
		return gitPath, nil
	}
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unexpected .git file content: %q", line)
	}
	return resolveCommonDir(strings.TrimPrefix(line, prefix)), nil
}

// resolveCommonDir returns the shared git common directory for a linked
// worktree's per-worktree gitdir, read from its "commondir" file (its
// contents are typically a relative path like "../.."). Returns gitDir
// unchanged when no commondir file exists.
func resolveCommonDir(gitDir string) string {
	data, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir
	}
	commonDir := strings.TrimSpace(string(data))
	if commonDir == "" {
		return gitDir
	}
	if filepath.IsAbs(commonDir) {
		return commonDir
	}
	return filepath.Join(gitDir, commonDir)
}

// SpritesProjectSkillPath returns the on-sprite path where a single
// skill's SKILL.md must be uploaded for the given agent's project
// skill dir. The sprite's CWD is always /workspace, so this is just
// /workspace/<projectSkillDir>/<DirName(slug)>/SKILL.md.
func SpritesProjectSkillPath(projectSkillDir, slug string) string {
	return "/workspace/" + projectSkillDir + "/" + DirName(slug) + "/SKILL.md"
}

// DirName returns the on-disk directory name for a skill slug within a
// project skill directory. The "kandev-" prefix marks Kandev-owned
// directories so cleanup can safely remove them without touching a
// user's own skills (see cleanKandevSkills). Applying it is idempotent:
// a slug that already carries the prefix (bundled system skills are
// slugged "kandev-*" at source) is not prefixed a second time, which
// would otherwise leave the skill unloadable under its declared name.
func DirName(slug string) string {
	if strings.HasPrefix(slug, "kandev-") {
		return slug
	}
	return "kandev-" + slug
}

func renderSkillMarkdown(sk Skill) string {
	content := strings.TrimLeft(sk.Content, "\r\n")
	if hasYAMLFrontmatter(content) {
		return content
	}
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n%s", sk.Slug, sk.Slug, content)
}

func hasYAMLFrontmatter(content string) bool {
	return strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n")
}
