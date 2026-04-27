package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9]+`)

// SkillWithUsage extends Skill with the number of agent instances using it.
type SkillWithUsage struct {
	models.Skill
	UsedByCount int `json:"used_by_count"`
}

// GenerateSlug creates a kebab-case slug from a name.
func GenerateSlug(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	slug := nonAlphanumRe.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "skill"
	}
	return slug
}

// ListSkillsWithUsage returns all skills for a workspace with used-by-agent counts.
func (s *Service) ListSkillsWithUsage(ctx context.Context, _ string) ([]*SkillWithUsage, error) {
	skills, err := s.ListSkillsFromConfig(ctx, "")
	if err != nil {
		return nil, err
	}
	// Count usage by scanning agent desired_skills from ConfigLoader.
	counts := s.countSkillUsageFromConfig()
	result := make([]*SkillWithUsage, len(skills))
	for i, sk := range skills {
		result[i] = &SkillWithUsage{Skill: *sk, UsedByCount: counts[sk.Slug]}
	}
	return result, nil
}

// countSkillUsageFromConfig counts how many agents reference each skill slug.
func (s *Service) countSkillUsageFromConfig() map[string]int {
	counts := make(map[string]int)
	if s.cfgLoader == nil {
		return counts
	}
	for _, a := range s.cfgLoader.GetAgents(defaultWorkspaceName) {
		for _, slug := range ParseDesiredSlugs(a.DesiredSkills) {
			counts[slug]++
		}
	}
	return counts
}

// ValidateAndPrepareSkill validates and prepares a skill for creation.
// Call this before CreateSkill to auto-generate slug and validate uniqueness.
func (s *Service) ValidateAndPrepareSkill(_ context.Context, skill *models.Skill) error {
	if skill.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if skill.Slug == "" {
		skill.Slug = GenerateSlug(skill.Name)
	}
	if err := s.validateSlugUnique(skill.Slug, ""); err != nil {
		return err
	}
	return s.validateSourceType(skill.SourceType)
}

// ValidateSkillUpdate validates a skill update for slug uniqueness.
func (s *Service) ValidateSkillUpdate(_ context.Context, skill *models.Skill) error {
	if skill.Slug != "" {
		return s.validateSlugUnique(skill.Slug, skill.ID)
	}
	return nil
}

func (s *Service) validateSlugUnique(slug, excludeID string) error {
	if s.cfgLoader == nil {
		return nil
	}
	for _, si := range s.cfgLoader.GetSkills(defaultWorkspaceName) {
		if si.Slug == slug && si.ID != excludeID {
			return fmt.Errorf("skill slug %q already exists in this workspace", slug)
		}
	}
	return nil
}

func (s *Service) validateSourceType(sourceType string) error {
	switch sourceType {
	case "inline", "local_path", "git", "skills_sh", "":
		return nil
	default:
		return fmt.Errorf("invalid source type: %q", sourceType)
	}
}
