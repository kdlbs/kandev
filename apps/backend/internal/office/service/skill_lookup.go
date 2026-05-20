// Package service hosts the office orchestration service. After ADR
// 0005 Wave E the per-launch skill + instruction file delivery moved
// into internal/agent/runtime/lifecycle/skill; this file keeps the
// in-memory manifest lookup the prompt builder needs (AGENTS.md
// content + deterministic instructionsDir path).
package service

import (
	"fmt"
	"path/filepath"

	"github.com/kandev/kandev/internal/common/instructionrefs"
)

// runtimeSubdir is the on-host runtime tree root where the
// runtime-tier skill deployer materialises files. Mirrored here so
// the office prompt builder can reference the path without importing
// runtime/lifecycle/skill (which would risk an import loop in tests).
const runtimeSubdir = "runtime"

// spritesRuntimeBase is the on-sprite runtime tree root, mirroring
// internal/agent/runtime/lifecycle/skill.SpritesRuntimeBase. Same
// rationale as runtimeSubdir.
const spritesRuntimeBase = "/root/.kandev/runtime"

// resolveInstructionsForPrompt returns the path the agent will see at
// instructionsDir (host or sprite-side, depending on executor) and
// the AGENTS.md content for prompt embedding. Pure data — no disk I/O.
// The runtime writes the actual files during launch.
func (si *SchedulerIntegration) resolveInstructionsForPrompt(manifest *SkillManifest, executorType string) (string, string) {
	if manifest == nil {
		return "", ""
	}
	dir := instructionsDirForExecutor(si.svc.kandevBasePath(), manifest.WorkspaceSlug, manifest.AgentID, executorType)
	agentsMD := ""
	for _, instr := range manifest.Instructions {
		if instr.Filename == "AGENTS.md" {
			// Rewrite ./HEARTBEAT.md style sibling refs to absolute paths so
			// the agent can act on them without resolving manually. Mirrors
			// the rewrite the runtime applies when materialising the file
			// to disk — the prompt copy and the on-disk copy stay aligned.
			agentsMD = instructionrefs.Rewrite(instr.Content, dir)
			break
		}
	}
	return dir, agentsMD
}

// instructionsDirForExecutor returns the path the runtime deployer
// will materialise instruction files at, given an executor type.
//
//   - sprites          → /root/.kandev/runtime/<ws>/instructions/<agent>
//   - local_pc/docker  → <basePath>/runtime/<ws>/instructions/<agent>
//
// The returned path is used both by office's prompt builder (embeds
// it as a hint string) and matches what skill.Deployer writes to.
func instructionsDirForExecutor(basePath, workspaceSlug, agentID, executorType string) string {
	if executorType == "sprites" {
		return fmt.Sprintf("%s/%s/instructions/%s", spritesRuntimeBase, workspaceSlug, agentID)
	}
	return filepath.Join(basePath, runtimeSubdir, workspaceSlug, "instructions", agentID)
}
