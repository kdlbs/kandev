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

// Metadata keys used to pass orchestrate skill data through to executor backends.
const (
	// MetadataKeyRuntimeDir is set on the executor request to indicate
	// that a host-side runtime directory should be bind-mounted into the container.
	MetadataKeyRuntimeDir = "kandev_runtime_dir"

	// MetadataKeySkillManifestJSON is set on the executor request so that
	// the Sprites executor can upload skill files after creating the sprite.
	MetadataKeySkillManifestJSON = "skill_manifest_json"
)

// deliverSkills dispatches skill and instruction file delivery to the
// appropriate executor-specific strategy. Returns the instructions directory
// path that should be passed to the prompt builder.
func (si *SchedulerIntegration) deliverSkills(
	ctx context.Context, manifest *SkillManifest, execCfg *ExecutorConfig,
) string {
	if manifest == nil {
		return ""
	}

	execType := ""
	if execCfg != nil {
		execType = execCfg.Type
	}

	switch execType {
	case "local_docker":
		return si.deliverSkillsDocker(ctx, manifest)
	case "sprites":
		return si.deliverSkillsSprites(ctx, manifest)
	default:
		// local_pc, worktree, or any unknown type: write locally and symlink.
		return si.deliverSkillsLocal(ctx, manifest)
	}
}

// deliverSkillsLocal writes skills and instructions to the host runtime
// directory and creates symlinks into the agent's skill discovery dirs.
// This is the same behavior as the original prepareRuntime + InjectSkillsForAgent
// pipeline, now driven from the manifest.
func (si *SchedulerIntegration) deliverSkillsLocal(
	_ context.Context, manifest *SkillManifest,
) string {
	basePath := si.svc.kandevBasePath()
	runtimeDir := filepath.Join(basePath, "runtime")

	// Write skills to runtime/skills/<slug>/SKILL.md
	si.writeSkillFiles(manifest, runtimeDir)

	// Write instructions to runtime/<ws>/instructions/<agentId>/<filename>
	instructionsDir := filepath.Join(
		runtimeDir, manifest.WorkspaceSlug, "instructions", manifest.AgentID,
	)
	si.writeInstructionFiles(manifest, instructionsDir)

	// Create symlinks into agent type skill dirs (same as InjectSkillsForAgent).
	si.symlinkSkills(manifest, runtimeDir, basePath)

	return instructionsDir
}

// deliverSkillsDocker writes files to the host runtime directory (same as local)
// and stores the runtime dir path in scheduler-level state so the caller can
// pass it to the executor request metadata. The container will bind-mount this
// directory at the same path.
func (si *SchedulerIntegration) deliverSkillsDocker(
	_ context.Context, manifest *SkillManifest,
) string {
	basePath := si.svc.kandevBasePath()
	runtimeDir := filepath.Join(basePath, "runtime")

	// Write files to host.
	si.writeSkillFiles(manifest, runtimeDir)

	instructionsDir := filepath.Join(
		runtimeDir, manifest.WorkspaceSlug, "instructions", manifest.AgentID,
	)
	si.writeInstructionFiles(manifest, instructionsDir)

	// Store the top-level runtime dir for mount propagation.
	// The caller reads si.runtimeDir after deliverSkills returns.
	si.runtimeDir = runtimeDir

	return instructionsDir
}

// deliverSkillsSprites prepares the manifest JSON for upload by the Sprites
// executor. Files are not written locally; instead the manifest is serialized
// and stored in scheduler-level state so the caller can attach it as metadata
// on the executor request.
func (si *SchedulerIntegration) deliverSkillsSprites(
	_ context.Context, manifest *SkillManifest,
) string {
	data, err := json.Marshal(manifest)
	if err != nil {
		si.logger.Warn("failed to marshal skill manifest for sprites", zap.Error(err))
		return ""
	}
	si.skillManifestJSON = string(data)

	// Return the sprite-side path where the files will end up after upload.
	return fmt.Sprintf("/root/.kandev/runtime/%s/instructions/%s",
		manifest.WorkspaceSlug, manifest.AgentID)
}

// writeSkillFiles writes each skill's SKILL.md to runtimeDir/<ws>/skills/<slug>/SKILL.md.
func (si *SchedulerIntegration) writeSkillFiles(manifest *SkillManifest, runtimeDir string) {
	for _, skill := range manifest.Skills {
		targetDir := filepath.Join(runtimeDir, manifest.WorkspaceSlug, "skills", skill.Slug)
		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			si.logger.Warn("failed to create skill dir",
				zap.String("slug", skill.Slug), zap.Error(err))
			continue
		}
		if err := os.WriteFile(
			filepath.Join(targetDir, "SKILL.md"),
			[]byte(skill.Content), 0o644,
		); err != nil {
			si.logger.Warn("failed to write skill",
				zap.String("slug", skill.Slug), zap.Error(err))
		}
	}
}

// writeInstructionFiles writes instruction files to the given directory.
func (si *SchedulerIntegration) writeInstructionFiles(manifest *SkillManifest, instructionsDir string) {
	if len(manifest.Instructions) == 0 {
		return
	}
	if err := os.MkdirAll(instructionsDir, 0o755); err != nil {
		si.logger.Warn("failed to create instructions dir", zap.Error(err))
		return
	}
	for _, instr := range manifest.Instructions {
		if err := os.WriteFile(
			filepath.Join(instructionsDir, instr.Filename),
			[]byte(instr.Content), 0o644,
		); err != nil {
			si.logger.Warn("failed to write instruction file",
				zap.String("filename", instr.Filename), zap.Error(err))
		}
	}
}

// symlinkSkills creates symlinks from the agent type skill directories
// into the runtime skills directory.
func (si *SchedulerIntegration) symlinkSkills(manifest *SkillManifest, runtimeDir, kandevBase string) {
	targetDirs := agentSkillDirs(manifest.AgentTypeID)
	if len(targetDirs) == 0 {
		return
	}

	var skillDirs []SkillDir
	for _, skill := range manifest.Skills {
		skillDirs = append(skillDirs, SkillDir{
			Slug: skill.Slug,
			Path: filepath.Join(runtimeDir, manifest.WorkspaceSlug, "skills", skill.Slug),
		})
	}

	for _, dir := range targetDirs {
		if err := symlinkSkillsSafe(dir, skillDirs, kandevBase); err != nil {
			si.logger.Warn("symlink skills failed",
				zap.String("dir", dir), zap.Error(err))
		}
	}
}

// BuildSymlinkScript generates a shell script that creates skill symlinks
// inside a container or sprite. The script creates the target directories
// and symlinks each skill from the runtime dir into the agent's skill
// discovery directories.
func BuildSymlinkScript(manifest *SkillManifest, runtimeDir string) string {
	if manifest == nil || len(manifest.Skills) == 0 {
		return ""
	}
	targetDirs := agentSkillDirs(manifest.AgentTypeID)
	if len(targetDirs) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, targetDir := range targetDirs {
		fmt.Fprintf(&sb, "mkdir -p %s\n", targetDir)
		for _, skill := range manifest.Skills {
			skillPath := filepath.Join(
				runtimeDir, manifest.WorkspaceSlug, "skills", skill.Slug,
			)
			fmt.Fprintf(&sb, "ln -sf %s %s/%s\n", skillPath, targetDir, skill.Slug)
		}
	}
	return sb.String()
}
