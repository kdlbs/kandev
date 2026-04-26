package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
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
func (s *Service) ListSkillsWithUsage(ctx context.Context, wsID string) ([]*SkillWithUsage, error) {
	skills, err := s.repo.ListSkills(ctx, wsID)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.CountSkillUsage(ctx, wsID)
	if err != nil {
		s.logger.Warn("failed to count skill usage", zap.Error(err))
		counts = make(map[string]int)
	}
	result := make([]*SkillWithUsage, len(skills))
	for i, sk := range skills {
		result[i] = &SkillWithUsage{Skill: *sk, UsedByCount: counts[sk.ID]}
	}
	return result, nil
}

// ValidateAndPrepareSkill validates and prepares a skill for creation.
// Call this before CreateSkill to auto-generate slug and validate uniqueness.
func (s *Service) ValidateAndPrepareSkill(ctx context.Context, skill *models.Skill) error {
	if skill.Name == "" {
		return fmt.Errorf("skill name is required")
	}
	if skill.Slug == "" {
		skill.Slug = GenerateSlug(skill.Name)
	}
	if err := s.validateSlugUnique(ctx, skill.WorkspaceID, skill.Slug, ""); err != nil {
		return err
	}
	return s.validateSourceType(skill.SourceType)
}

// ValidateSkillUpdate validates a skill update for slug uniqueness.
func (s *Service) ValidateSkillUpdate(ctx context.Context, skill *models.Skill) error {
	if skill.Slug != "" {
		return s.validateSlugUnique(ctx, skill.WorkspaceID, skill.Slug, skill.ID)
	}
	return nil
}

func (s *Service) validateSlugUnique(ctx context.Context, wsID, slug, excludeID string) error {
	existing, err := s.repo.GetSkillBySlug(ctx, wsID, slug)
	if err != nil {
		return fmt.Errorf("checking slug uniqueness: %w", err)
	}
	if existing != nil && existing.ID != excludeID {
		return fmt.Errorf("skill slug %q already exists in this workspace", slug)
	}
	return nil
}

func (s *Service) validateSourceType(sourceType string) error {
	switch sourceType {
	case "inline", "local_path", "git", "":
		return nil
	default:
		return fmt.Errorf("invalid source type: %q", sourceType)
	}
}
