package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

// AgentTypeResolver resolves an agent profile ID to its agent type ID.
type AgentTypeResolver func(profileID string) string

// SetAgentTypeResolver sets the callback used to map agent profile IDs to agent type IDs.
func (s *Service) SetAgentTypeResolver(resolver AgentTypeResolver) {
	s.agentTypeResolver = resolver
}

// agentSkillDirs returns all skill discovery directories for a given agent type.
// Multiple dirs because some agents read from several locations.
func agentSkillDirs(agentTypeID string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	agents := filepath.Join(home, ".agents", "skills")
	switch agentTypeID {
	case "claude-acp":
		return []string{filepath.Join(home, ".claude", "skills"), agents}
	case "codex-acp":
		return []string{filepath.Join(home, ".codex", "skills"), agents}
	case "opencode-acp":
		return []string{filepath.Join(home, ".config", "opencode", "skills"), filepath.Join(home, ".claude", "skills"), agents}
	case "gemini":
		return []string{filepath.Join(home, ".gemini", "skills")}
	case "copilot-acp":
		return []string{filepath.Join(home, ".copilot", "skills"), filepath.Join(home, ".claude", "skills"), agents}
	case "auggie":
		return []string{filepath.Join(home, ".augment", "skills"), filepath.Join(home, ".claude", "skills"), agents}
	case "amp-acp":
		return []string{agents}
	default:
		return nil
	}
}

// allKnownSkillDirs returns every skill directory path we might write to.
func allKnownSkillDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"),
		filepath.Join(home, ".codex", "skills"),
		filepath.Join(home, ".gemini", "skills"),
		filepath.Join(home, ".copilot", "skills"),
		filepath.Join(home, ".augment", "skills"),
		filepath.Join(home, ".cursor", "skills"),
		filepath.Join(home, ".config", "opencode", "skills"),
	}
}

// InjectSkillsForAgent symlinks desired skills into the agent's skill dirs.
func (s *Service) InjectSkillsForAgent(ctx context.Context, agentInstanceID, workspaceName string) error {
	agent, err := s.repo.GetAgentInstance(ctx, agentInstanceID)
	if err != nil {
		return fmt.Errorf("get agent instance: %w", err)
	}
	slugs := ParseDesiredSlugs(agent.DesiredSkills)
	if len(slugs) == 0 {
		return nil
	}
	agentTypeID := s.resolveAgentType(agent.AgentProfileID)
	targetDirs := agentSkillDirs(agentTypeID)
	if len(targetDirs) == 0 {
		s.logger.Debug("agent type does not support skills", zap.String("type", agentTypeID))
		return nil
	}
	skillDirs := s.resolveSkillPaths(slugs, workspaceName)
	if len(skillDirs) == 0 {
		s.logger.Warn("no skills resolved", zap.Strings("slugs", slugs))
		return nil
	}
	kandevBase := s.kandevBasePath()
	for _, dir := range targetDirs {
		if err := symlinkSkillsSafe(dir, skillDirs, kandevBase); err != nil {
			s.logger.Warn("symlink skills failed", zap.String("dir", dir), zap.Error(err))
		}
	}
	return nil
}

// kandevBasePath returns the kandev base path for ownership checks.
func (s *Service) kandevBasePath() string {
	if s.cfgLoader != nil {
		return s.cfgLoader.BasePath()
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kandev")
}

// symlinkSkillsSafe creates symlinks with conflict handling.
// Only manages symlinks that point into kandevBase. Skips conflicts.
func symlinkSkillsSafe(targetDir string, skillDirs []SkillDir, kandevBase string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", targetDir, err)
	}
	for _, sd := range skillDirs {
		if sd.Path == "" {
			continue
		}
		link := filepath.Join(targetDir, sd.Slug)
		if skipConflict(link, sd.Path, kandevBase) {
			continue
		}
		_ = os.Remove(link)
		if err := os.Symlink(sd.Path, link); err != nil {
			return fmt.Errorf("symlink %s: %w", sd.Slug, err)
		}
	}
	return nil
}

// skipConflict returns true if the link path should NOT be touched.
func skipConflict(link, targetPath, kandevBase string) bool {
	info, err := os.Lstat(link)
	if os.IsNotExist(err) {
		return false // doesn't exist, safe to create
	}
	if err != nil {
		return true // can't stat, skip to be safe
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return true // real file/dir, not ours, skip
	}
	existing, err := os.Readlink(link)
	if err != nil {
		return true
	}
	if existing == targetPath {
		return true // already correct, skip (no-op)
	}
	// Symlink exists but points elsewhere -- only replace if it's ours
	if strings.HasPrefix(existing, kandevBase) {
		return false // ours, safe to replace
	}
	return true // someone else's symlink, don't touch
}

// CleanupOwnedSymlinks removes all symlinks in agent skill dirs that point
// into kandevBase. Called on kandev shutdown.
func CleanupOwnedSymlinks(kandevBase string) {
	for _, dir := range allKnownSkillDirs() {
		removeOwnedSymlinksInDir(dir, kandevBase)
	}
}

func removeOwnedSymlinksInDir(dir, kandevBase string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		link := filepath.Join(dir, e.Name())
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(link)
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, kandevBase) {
			_ = os.Remove(link)
		}
	}
}

// CleanDanglingSymlinks removes symlinks in agent skill dirs that point to
// nonexistent targets within kandevBase. Called on startup.
func CleanDanglingSymlinks(kandevBase string) {
	for _, dir := range allKnownSkillDirs() {
		cleanDanglingInDir(dir, kandevBase)
	}
}

func cleanDanglingInDir(dir, kandevBase string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		link := filepath.Join(dir, e.Name())
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(link)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(target, kandevBase) {
			continue
		}
		if _, err := os.Stat(target); os.IsNotExist(err) {
			_ = os.Remove(link) // dangling, target was deleted
		}
	}
}

// resolveAgentType maps an agent profile ID to an agent type ID.
func (s *Service) resolveAgentType(profileID string) string {
	if s.agentTypeResolver == nil || profileID == "" {
		return ""
	}
	return s.agentTypeResolver(profileID)
}

// ParseDesiredSlugs parses a DesiredSkills string into a list of slugs.
func ParseDesiredSlugs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	if strings.HasPrefix(raw, "[") {
		var slugs []string
		if err := json.Unmarshal([]byte(raw), &slugs); err == nil {
			return filterEmpty(slugs)
		}
	}
	return filterEmpty(strings.Split(raw, ","))
}

func filterEmpty(ss []string) []string {
	var out []string
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// resolveSkillPaths maps desired slugs to on-disk paths.
func (s *Service) resolveSkillPaths(desiredSlugs []string, workspaceName string) []SkillDir {
	var result []SkillDir
	resolved := make(map[string]bool)
	if s.cfgLoader != nil {
		for _, info := range s.cfgLoader.GetSkills(workspaceName) {
			for _, slug := range desiredSlugs {
				if info.Slug == slug && !resolved[slug] {
					result = append(result, SkillDir{Slug: slug, Path: info.DirPath})
					resolved[slug] = true
				}
			}
		}
	}
	if s.cfgLoader != nil {
		bundledDir := filepath.Join(s.cfgLoader.BasePath(), "skills")
		for _, slug := range desiredSlugs {
			if resolved[slug] {
				continue
			}
			dirPath := filepath.Join(bundledDir, slug)
			if _, err := os.Stat(filepath.Join(dirPath, "SKILL.md")); err == nil {
				result = append(result, SkillDir{Slug: slug, Path: dirPath})
				resolved[slug] = true
			}
		}
	}
	return result
}
