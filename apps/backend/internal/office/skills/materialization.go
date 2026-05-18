package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/office/models"
)

// skillSourceTypeGit is the source type for skills fetched from a git repository.
const skillSourceTypeGit = "git"

// SkillDir represents a materialized skill directory on disk.
type SkillDir struct {
	Slug string
	Path string
}

// MaterializeSkills resolves skill IDs to on-disk paths.
// For inline skills, it writes SKILL.md to a temp directory.
// For local_path skills, it validates the path exists.
// For git skills, it returns a placeholder (clone logic deferred).
func (s *SkillService) MaterializeSkills(ctx context.Context, skillIDs []string, cacheDir string) ([]SkillDir, error) {
	var dirs []SkillDir
	var allowedRoot string
	if s.cfgLoader != nil {
		allowedRoot = s.cfgLoader.BasePath()
	}
	for _, id := range skillIDs {
		skill, err := s.GetSkillFromConfig(ctx, id)
		if err != nil {
			s.logger.Warn("skipping skill: " + err.Error())
			continue
		}
		sd, err := materializeSkill(skill, cacheDir, allowedRoot)
		if err != nil {
			s.logger.Warn("failed to materialize skill " + skill.Slug + ": " + err.Error())
			continue
		}
		dirs = append(dirs, sd)
	}
	return dirs, nil
}

func materializeSkill(skill *models.Skill, cacheDir, allowedRoot string) (SkillDir, error) {
	switch skill.SourceType {
	case SkillSourceTypeInline, "filesystem":
		return materializeInline(skill, cacheDir)
	case "local_path":
		return materializeLocalPath(skill, allowedRoot)
	case skillSourceTypeGit:
		return materializeGit(skill, cacheDir)
	default:
		return SkillDir{}, fmt.Errorf("unknown source type: %s", skill.SourceType)
	}
}

func materializeInline(skill *models.Skill, cacheDir string) (SkillDir, error) {
	dir := filepath.Join(cacheDir, skill.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SkillDir{}, fmt.Errorf("creating skill dir: %w", err)
	}
	skillFile := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte(skill.Content), 0o644); err != nil {
		return SkillDir{}, fmt.Errorf("writing SKILL.md: %w", err)
	}
	return SkillDir{Slug: skill.Slug, Path: dir}, nil
}

func materializeLocalPath(skill *models.Skill, allowedRoot string) (SkillDir, error) {
	if skill.SourceLocator == "" {
		return SkillDir{}, fmt.Errorf("local_path skill %q has no source_locator", skill.Slug)
	}
	if err := validateLocalPathUnderRoot(skill.SourceLocator, allowedRoot); err != nil {
		return SkillDir{}, err
	}
	info, err := os.Stat(skill.SourceLocator)
	if err != nil {
		return SkillDir{}, fmt.Errorf("local path %q: %w", skill.SourceLocator, err)
	}
	if !info.IsDir() {
		return SkillDir{}, fmt.Errorf("local path %q is not a directory", skill.SourceLocator)
	}
	return SkillDir{Slug: skill.Slug, Path: skill.SourceLocator}, nil
}

// validateLocalPathUnderRoot rejects local_path skill source locators that
// escape the kandev config base path. Without this guard a workspace could
// symlink an arbitrary directory (e.g. /etc/ssh, /root/.ssh) into an
// agent's home. If allowedRoot is empty the service has no config loader
// (test wiring), in which case the guard is skipped — production wiring
// always supplies a base path.
func validateLocalPathUnderRoot(locator, allowedRoot string) error {
	if allowedRoot == "" {
		return nil
	}
	abs, err := filepath.Abs(locator)
	if err != nil {
		return fmt.Errorf("invalid local_path source_locator: %w", err)
	}
	rootAbs, err := filepath.Abs(allowedRoot)
	if err != nil {
		return fmt.Errorf("invalid allowed root: %w", err)
	}
	if abs != rootAbs && !strings.HasPrefix(abs, rootAbs+string(os.PathSeparator)) {
		return fmt.Errorf("local_path source_locator %q is outside allowed root", locator)
	}
	return nil
}

func materializeGit(skill *models.Skill, cacheDir string) (SkillDir, error) {
	if err := validateGitLocator(skill.SourceLocator); err != nil {
		return SkillDir{}, err
	}
	repoDir := filepath.Join(cacheDir, skillSourceTypeGit, hashLocator(skill.SourceLocator))
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		return SkillDir{}, fmt.Errorf("creating git cache: %w", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		if err := runGit(repoDir, "pull", "--ff-only"); err != nil {
			return SkillDir{}, err
		}
	} else if err := runGit("", "clone", "--depth=1", skill.SourceLocator, repoDir); err != nil {
		return SkillDir{}, err
	}
	if _, err := os.Stat(filepath.Join(repoDir, "SKILL.md")); err != nil {
		return SkillDir{}, fmt.Errorf("git skill %q has no SKILL.md at repository root", skill.Slug)
	}
	return SkillDir{Slug: skill.Slug, Path: repoDir}, nil
}

func validateGitLocator(locator string) error {
	trimmed := strings.TrimSpace(locator)
	if trimmed == "" {
		return fmt.Errorf("git skill has no source_locator")
	}
	if strings.Contains(trimmed, "..") {
		return fmt.Errorf("git source_locator must not contain path traversal")
	}
	// Accept SCP-style SSH: git@host:path — url.Parse won't assign a scheme for these.
	if strings.HasPrefix(trimmed, "git@") && strings.Contains(trimmed, ":") {
		return nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid git source_locator: %w", err)
	}
	switch u.Scheme {
	case "https", "ssh", skillSourceTypeGit:
		// allowed
	default:
		return fmt.Errorf("git source_locator scheme %q is not allowed (use https, ssh, or git)", u.Scheme)
	}
	return nil
}

func hashLocator(locator string) string {
	sum := sha256.Sum256([]byte(locator))
	return hex.EncodeToString(sum[:])
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SymlinkSkills creates symlinks from the agent's skill directory to skill paths.
func SymlinkSkills(agentHome string, skillDirs []SkillDir) error {
	skillsDir := filepath.Join(agentHome, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("creating skills dir: %w", err)
	}
	for _, sd := range skillDirs {
		if sd.Path == "" {
			continue
		}
		link := filepath.Join(skillsDir, sd.Slug)
		_ = os.Remove(link)
		if err := os.Symlink(sd.Path, link); err != nil {
			return fmt.Errorf("symlinking skill %s: %w", sd.Slug, err)
		}
	}
	return nil
}

// CleanupSymlinks removes skill symlinks from the agent's home directory.
func CleanupSymlinks(agentHome string, slugs []string) error {
	skillsDir := filepath.Join(agentHome, ".claude", "skills")
	for _, slug := range slugs {
		link := filepath.Join(skillsDir, slug)
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing symlink %s: %w", slug, err)
		}
	}
	return nil
}
