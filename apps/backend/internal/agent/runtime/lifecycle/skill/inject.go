package skill

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
//  2. Writes each skill's SKILL.md under kandev-<slug>/.
//  3. Best-effort appends the kandev-* pattern to .git/info/exclude.
//
// Skills with invalid slugs are skipped silently — caller upstream
// already validates user-facing input.
func injectSkills(worktreePath, projectSkillDir string, skills []Skill) error {
	if err := cleanKandevSkills(worktreePath, projectSkillDir); err != nil {
		return fmt.Errorf("clean kandev skills: %w", err)
	}
	skillsDir := filepath.Join(worktreePath, projectSkillDir)
	for _, sk := range skills {
		if !isValidSlug(sk.Slug) {
			continue
		}
		dir := filepath.Join(skillsDir, "kandev-"+sk.Slug)
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

// ensureGitExclude appends "<projectSkillDir>/kandev-*" to the
// worktree's .git/info/exclude file so injected skill directories
// never appear as dirty files in git status. Idempotent.
//
// Linked worktrees use a .git file pointing at the real gitdir; this
// helper resolves both the directory and file forms.
func ensureGitExclude(worktreePath, projectSkillDir string) error {
	gitDir, err := resolveGitDir(worktreePath)
	if err != nil {
		return fmt.Errorf("resolve git dir: %w", err)
	}
	excludeFile := filepath.Join(gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludeFile), 0o755); err != nil {
		return fmt.Errorf("mkdir info dir: %w", err)
	}

	pattern := projectSkillDir + "/kandev-*"
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

// resolveGitDir returns the actual git directory for a worktree path.
// For a normal repository the .git directory is returned as-is. For a
// linked worktree, .git is a file of the form "gitdir: /path/to/gitdir"
// and the target path is returned.
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
	return strings.TrimPrefix(line, prefix), nil
}

// SpritesProjectSkillPath returns the on-sprite path where a single
// skill's SKILL.md must be uploaded for the given agent's project
// skill dir. The sprite's CWD is always /workspace, so this is just
// /workspace/<projectSkillDir>/kandev-<slug>/SKILL.md.
func SpritesProjectSkillPath(projectSkillDir, slug string) string {
	return "/workspace/" + projectSkillDir + "/kandev-" + slug + "/SKILL.md"
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
