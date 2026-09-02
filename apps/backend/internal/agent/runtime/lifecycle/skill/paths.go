package skill

import (
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/common/skillslug"
)

// isValidSlug reports whether the given slug is safe for use in shell
// commands and on-disk paths. Anything outside this set is dropped during
// delivery to avoid path-traversal or shell-quoting hazards.
func isValidSlug(s string) bool { return skillslug.WellFormed(s) }

// isValidPathComponent reports whether the given filename is a single
// safe path component (no separators, no traversal). Used when writing
// instruction files where the filename comes from upstream data.
func isValidPathComponent(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "/\\") {
		return false
	}
	if strings.Contains(s, "..") {
		return false
	}
	return true
}

func cleanRelativeSkillFilePath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" || strings.Contains(p, "\\") {
		return "", false
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	if strings.HasPrefix(cleaned, "/") || cleaned == "SKILL.md" {
		return "", false
	}
	return cleaned, true
}

const (
	// SpritesRuntimeBase is the on-sprite path where runtime instruction
	// files are uploaded. Skills no longer live under this tree; they go
	// directly into the sprite's worktree (/workspace/<projectSkillDir>).
	SpritesRuntimeBase = "/root/.kandev/runtime"
	// KubernetesRuntimeBase is inside the reserved Pod runtime volume.
	// The Kubernetes lifecycle uploads the serialized manifest there.
	KubernetesRuntimeBase = "/opt/kandev/runtime"
)

// instructionsDirHost returns the on-host directory where a manifest's
// instruction files are written.
func instructionsDirHost(kandevBase, workspaceSlug, agentID string) string {
	return filepath.Join(kandevBase, "runtime", workspaceSlug, "instructions", agentID)
}

// spritesInstructionsDir returns the on-sprite directory where a
// manifest's instruction files are uploaded.
func spritesInstructionsDir(workspaceSlug, agentID string) string {
	return SpritesRuntimeBase + "/" + workspaceSlug + "/instructions/" + agentID
}

func kubernetesInstructionsDir(workspaceSlug, agentID string) string {
	return KubernetesRuntimeBase + "/" + workspaceSlug + "/instructions/" + agentID
}
