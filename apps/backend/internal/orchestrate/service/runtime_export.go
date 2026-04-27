package service

import (
	"context"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// prepareRuntime exports instruction files and skills to the runtime directory
// before launching an agent session. Returns the instructions directory path.
func (si *SchedulerIntegration) prepareRuntime(
	ctx context.Context, agent *models.AgentInstance, workspaceSlug string,
) (string, error) {
	basePath := si.svc.kandevBasePath()

	// Export instructions from DB to disk.
	instructionsDir := filepath.Join(
		basePath, "runtime", workspaceSlug, "instructions", agent.ID,
	)
	if err := si.svc.ExportInstructionsToDir(ctx, agent.ID, instructionsDir); err != nil {
		return "", err
	}

	// Export desired skills to runtime dir (best-effort).
	si.exportSkillsToRuntime(ctx, agent, basePath, workspaceSlug)

	return instructionsDir, nil
}

// exportSkillsToRuntime writes each desired skill's SKILL.md to the runtime
// skills directory. Errors are logged but do not fail the session.
func (si *SchedulerIntegration) exportSkillsToRuntime(
	ctx context.Context, agent *models.AgentInstance,
	basePath, workspaceSlug string,
) {
	slugs := ParseDesiredSlugs(agent.DesiredSkills)
	if len(slugs) == 0 {
		return
	}
	skillsDir := filepath.Join(basePath, "runtime", workspaceSlug, "skills")
	for _, slug := range slugs {
		skill, err := si.svc.GetSkillFromConfig(ctx, slug)
		if err != nil {
			si.logger.Debug("skip skill export",
				zap.String("slug", slug), zap.Error(err))
			continue
		}
		targetDir := filepath.Join(skillsDir, slug)
		if mkErr := os.MkdirAll(targetDir, 0o755); mkErr != nil {
			si.logger.Warn("failed to create skill dir",
				zap.String("slug", slug), zap.Error(mkErr))
			continue
		}
		if wErr := os.WriteFile(
			filepath.Join(targetDir, "SKILL.md"),
			[]byte(skill.Content), 0o644,
		); wErr != nil {
			si.logger.Warn("failed to write skill",
				zap.String("slug", slug), zap.Error(wErr))
		}
	}
}

// ExportInstructionsToDir writes all instruction files for an agent to the
// given directory. Creates the directory if it does not exist.
// This is a stub that will be fully implemented by the instructions agent.
// If instruction files exist in the DB they are written; otherwise it no-ops.
func (s *Service) ExportInstructionsToDir(
	ctx context.Context, agentInstanceID, targetDir string,
) error {
	files, err := s.repo.ListInstructions(ctx, agentInstanceID)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(targetDir, f.Filename)
		if wErr := os.WriteFile(path, []byte(f.Content), 0o644); wErr != nil {
			return wErr
		}
	}
	return nil
}
