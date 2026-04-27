package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// SkillManifest holds the resolved skills and instructions for an agent session.
// It is a pure data structure built before executor selection, so the delivery
// strategy can adapt to the target executor type.
type SkillManifest struct {
	Skills        []ManifestSkill
	Instructions  []ManifestInstruction
	AgentTypeID   string // e.g. "claude-acp", "codex-acp"
	WorkspaceSlug string
	AgentID       string
}

// ManifestSkill represents a single skill's content.
type ManifestSkill struct {
	Slug    string
	Content string // SKILL.md content
}

// ManifestInstruction represents a single instruction file.
type ManifestInstruction struct {
	Filename string // "AGENTS.md", "HEARTBEAT.md", etc.
	Content  string
	IsEntry  bool
}

// buildSkillManifest loads desired skills and instruction files for an agent,
// returning a manifest that can be delivered to any executor type.
func (si *SchedulerIntegration) buildSkillManifest(
	ctx context.Context, agent *models.AgentInstance, workspaceSlug string,
) *SkillManifest {
	manifest := &SkillManifest{
		AgentTypeID:   si.svc.resolveAgentType(agent.AgentProfileID),
		WorkspaceSlug: workspaceSlug,
		AgentID:       agent.ID,
	}

	// Load desired skills.
	slugs := ParseDesiredSlugs(agent.DesiredSkills)
	for _, slug := range slugs {
		skill, err := si.svc.GetSkillFromConfig(ctx, slug)
		if err != nil {
			si.logger.Debug("skip skill in manifest",
				zap.String("slug", slug), zap.Error(err))
			continue
		}
		manifest.Skills = append(manifest.Skills, ManifestSkill{
			Slug:    skill.Slug,
			Content: skill.Content,
		})
	}

	// Load instruction files from DB.
	files, err := si.svc.repo.ListInstructions(ctx, agent.ID)
	if err != nil {
		si.logger.Warn("failed to load instructions for manifest",
			zap.String("agent_id", agent.ID), zap.Error(err))
		return manifest
	}
	for _, f := range files {
		manifest.Instructions = append(manifest.Instructions, ManifestInstruction{
			Filename: f.Filename,
			Content:  f.Content,
			IsEntry:  f.IsEntry,
		})
	}

	return manifest
}
