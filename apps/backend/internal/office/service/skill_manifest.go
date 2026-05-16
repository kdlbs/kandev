package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
)

// SkillManifest holds the resolved skills and instructions for an agent session.
// It is a pure data structure built before executor selection, so the delivery
// strategy can adapt to the target executor type.
type SkillManifest struct {
	Skills          []ManifestSkill
	Instructions    []ManifestInstruction
	AgentTypeID     string // e.g. "claude-acp", "codex-acp"
	WorkspaceSlug   string
	AgentID         string
	ProjectSkillDir string // CWD-relative path for project-level skills (e.g. ".claude/skills")
}

// ManifestSkill represents a single skill's content.
type ManifestSkill struct {
	ID          string
	Slug        string
	Content     string // SKILL.md content
	Version     string
	ContentHash string
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
	// Wave G: AgentInstance.ID == agent_profiles.id under the unified model.
	agentTypeID := si.svc.resolveAgentType(agent.ID)
	manifest := &SkillManifest{
		AgentTypeID:     agentTypeID,
		WorkspaceSlug:   workspaceSlug,
		AgentID:         agent.ID,
		ProjectSkillDir: si.svc.resolveProjectSkillDir(agentTypeID),
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
			ID:          skill.ID,
			Slug:        skill.Slug,
			Content:     skill.Content,
			Version:     skill.Version,
			ContentHash: skill.ContentHash,
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
