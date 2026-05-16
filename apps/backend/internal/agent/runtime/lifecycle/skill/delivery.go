package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/instructionrefs"
)

// deliver dispatches the manifest to the executor-specific strategy.
// Returns the metadata patches and instructions directory the caller
// should attach to the launch request.
func (d *Deployer) deliver(_ context.Context, manifest *Manifest, executorType, worktreePath string) DeployResult {
	if manifest == nil {
		return DeployResult{}
	}
	switch executorType {
	case "sprites":
		return d.deliverSprites(manifest)
	default:
		// local_pc and local_docker share the same delivery: the
		// worktree IS the agent's CWD inside the executor (Docker
		// bind-mounts it; local_pc runs the agent in it directly), so
		// writing skills under <worktree>/<projectSkillDir>/kandev-<slug>
		// gets them in front of the agent's project-skill discovery.
		return d.deliverLocal(manifest, worktreePath)
	}
}

// deliverLocal writes skills directly into the session's worktree
// under the agent's project skill directory and writes instruction
// files to the host runtime tree. Used for local_pc and local_docker
// — Docker's worktree bind-mount makes the worktree path identical
// inside and outside the container, so a single write satisfies both.
func (d *Deployer) deliverLocal(manifest *Manifest, worktreePath string) DeployResult {
	if worktreePath != "" && manifest.ProjectSkillDir != "" {
		if err := injectSkills(worktreePath, manifest.ProjectSkillDir, manifest.Skills); err != nil {
			d.logger.Warn("failed to inject skills into worktree",
				zap.String("worktree", worktreePath),
				zap.String("dir", manifest.ProjectSkillDir),
				zap.Error(err))
		}
	}
	instructionsDir := instructionsDirHost(d.basePath, manifest.WorkspaceSlug, manifest.AgentID)
	d.writeInstructionFiles(manifest, instructionsDir)
	return DeployResult{InstructionsDir: instructionsDir}
}

// deliverSprites serialises the manifest as JSON and stashes it on
// the launch metadata. The Sprites executor reads the JSON during
// post-create setup and uploads files into the sprite. We do NOT
// write files to the host because the sprite runs in a remote sandbox.
func (d *Deployer) deliverSprites(manifest *Manifest) DeployResult {
	dir := spritesInstructionsDir(manifest.WorkspaceSlug, manifest.AgentID)
	rewriteManifestRefs(manifest, dir)
	data, err := json.Marshal(manifest)
	if err != nil {
		d.logger.Warn("failed to marshal skill manifest for sprites", zap.Error(err))
		return DeployResult{}
	}
	return DeployResult{
		InstructionsDir: dir,
		Metadata: map[string]any{
			MetadataKeySkillManifestJSON: string(data),
		},
	}
}

// rewriteManifestRefs canonicalises sibling instruction references
// (./HEARTBEAT.md, ./SOUL.md, ...) inside each instruction file's
// content to absolute paths under instructionsDir. Used by both the
// local writer and the Sprites delivery so the contract matches the
// office prompt builder, which applies the same rewrite.
func rewriteManifestRefs(manifest *Manifest, instructionsDir string) {
	if manifest == nil || instructionsDir == "" {
		return
	}
	for i := range manifest.Instructions {
		manifest.Instructions[i].Content = instructionrefs.Rewrite(
			manifest.Instructions[i].Content, instructionsDir,
		)
	}
}

// writeInstructionFiles writes the manifest's instruction files into
// instructionsDir. Filenames that are not safe single-component
// strings are skipped to avoid path traversal. Sibling refs inside
// the file content are rewritten to absolute paths so the on-disk
// artefact agrees with the prompt the agent receives.
func (d *Deployer) writeInstructionFiles(manifest *Manifest, instructionsDir string) {
	if len(manifest.Instructions) == 0 {
		return
	}
	if err := os.MkdirAll(instructionsDir, 0o755); err != nil {
		d.logger.Warn("failed to create instructions dir", zap.Error(err))
		return
	}
	for _, instr := range manifest.Instructions {
		if !isValidPathComponent(instr.Filename) {
			d.logger.Warn("skipping instruction with invalid filename",
				zap.String("filename", instr.Filename))
			continue
		}
		content := instructionrefs.Rewrite(instr.Content, instructionsDir)
		if err := os.WriteFile(
			filepath.Join(instructionsDir, instr.Filename),
			[]byte(content), 0o644,
		); err != nil {
			d.logger.Warn("failed to write instruction file",
				zap.String("filename", instr.Filename), zap.Error(err))
		}
	}
}
