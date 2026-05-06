package lifecycle

import (
	"context"

	"go.uber.org/zap"
)

// MetadataKeyInstructionsDir carries the per-launch path where the
// SkillDeployer wrote the agent's instruction files. Office's prompt
// builder reads from this path; kanban launches don't consume it.
const MetadataKeyInstructionsDir = "kandev_instructions_dir"

// runSkillDeploy resolves the agent profile (now including the office
// enrichment fields from ADR 0005 Wave A), short-circuits when there's
// nothing to deploy, and otherwise hands off to the configured
// SkillDeployer. Any metadata patches the deployer returns are merged
// onto the prepared LaunchRequest so executor backends see them
// (e.g. Docker bind-mount, Sprites manifest upload). Errors are logged
// and swallowed: an in-flight launch must not be aborted because skill
// materialisation hiccupped.
func (m *Manager) runSkillDeploy(ctx context.Context, original, prepared *LaunchRequest) {
	if m.skillDeployer == nil || m.agentProfileReader == nil {
		return
	}
	if original.AgentProfileID == "" {
		return
	}
	profile, err := m.agentProfileReader.GetAgentProfile(ctx, original.AgentProfileID)
	if err != nil || profile == nil {
		m.logger.Debug("skill deploy skipped: profile lookup failed",
			zap.String("profile_id", original.AgentProfileID),
			zap.Error(err))
		return
	}
	if len(profile.SkillIDs) == 0 && len(profile.DesiredSkills) == 0 {
		// Fast path: shallow / kanban-flavour profiles with no enrichment to
		// deploy. Most launches today land here.
		return
	}
	req := SkillDeployRequest{
		Profile:       profile,
		WorkspacePath: prepared.WorkspacePath,
		ExecutorType:  prepared.ExecutorType,
		WorkspaceID:   profile.WorkspaceID,
		SessionID:     original.SessionID,
	}
	result, err := m.skillDeployer.DeploySkills(ctx, req)
	if err != nil {
		m.logger.Warn("skill deploy failed; launch continues",
			zap.String("profile_id", profile.ID),
			zap.String("workspace_path", prepared.WorkspacePath),
			zap.Error(err))
		return
	}
	mergeSkillMetadata(prepared, result)
}

// mergeSkillMetadata applies the deployer's output to the prepared
// launch request: every metadata patch is written through, and the
// instructions directory is stamped under MetadataKeyInstructionsDir
// so downstream consumers (office prompt builder, run detail UI) can
// recover the path.
func mergeSkillMetadata(prepared *LaunchRequest, result SkillDeployResult) {
	if len(result.Metadata) == 0 && result.InstructionsDir == "" {
		return
	}
	if prepared.Metadata == nil {
		prepared.Metadata = make(map[string]interface{})
	}
	for k, v := range result.Metadata {
		prepared.Metadata[k] = v
	}
	if result.InstructionsDir != "" {
		prepared.Metadata[MetadataKeyInstructionsDir] = result.InstructionsDir
	}
}
