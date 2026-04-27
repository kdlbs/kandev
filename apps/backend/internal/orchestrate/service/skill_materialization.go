package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// SkillDir represents a materialized skill directory on disk.
type SkillDir struct {
	Slug string
	Path string
}

// MaterializeSkills resolves skill IDs to on-disk paths.
// For inline skills, it writes SKILL.md to a temp directory.
// For local_path skills, it validates the path exists.
// For git skills, it returns a placeholder (clone logic deferred).
func (s *Service) MaterializeSkills(ctx context.Context, skillIDs []string, cacheDir string) ([]SkillDir, error) {
	var dirs []SkillDir
	for _, id := range skillIDs {
		skill, err := s.GetSkillFromConfig(ctx, id)
		if err != nil {
			s.logger.Warn("skipping skill: " + err.Error())
			continue
		}
		sd, err := s.materializeSkill(skill, cacheDir)
		if err != nil {
			s.logger.Warn("failed to materialize skill " + skill.Slug + ": " + err.Error())
			continue
		}
		dirs = append(dirs, sd)
	}
	return dirs, nil
}

func (s *Service) materializeSkill(skill *models.Skill, cacheDir string) (SkillDir, error) {
	switch skill.SourceType {
	case SkillSourceTypeInline, "filesystem":
		return materializeInline(skill, cacheDir)
	case "local_path":
		return materializeLocalPath(skill)
	case "git":
		return SkillDir{Slug: skill.Slug}, fmt.Errorf("git source not yet implemented")
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

func materializeLocalPath(skill *models.Skill) (SkillDir, error) {
	if skill.SourceLocator == "" {
		return SkillDir{}, fmt.Errorf("local_path skill %q has no source_locator", skill.Slug)
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
