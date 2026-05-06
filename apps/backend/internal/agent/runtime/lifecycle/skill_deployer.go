package lifecycle

import (
	"context"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// SkillDeployer materialises an agent profile's skills + custom prompt into
// the workspace before the agent process starts. ADR 0005 Wave A introduced
// this seam so the runtime owns "make the agent's skills available"; Wave E
// moved the concrete implementation (manifest builder + per-executor
// strategies) into runtime/lifecycle/skill so every launch — kanban or
// office — flows through the same path.
//
// Implementations are responsible for short-circuiting when the profile has
// nothing to deploy. The runtime does NOT pre-filter beyond a basic
// empty-profile check, so each implementation can decide what "empty"
// means for its delivery strategy.
type SkillDeployer interface {
	DeploySkills(ctx context.Context, req SkillDeployRequest) (SkillDeployResult, error)
}

// SkillDeployRequest carries everything a SkillDeployer needs to materialise
// per-agent skills + prompt before launch. The Profile is the resolved row
// from the agent_profiles table; WorkspacePath is the host filesystem path
// the agent will run inside; ExecutorType is the executor backend
// (local_pc / local_docker / sprites) so the deployer can pick a strategy.
type SkillDeployRequest struct {
	Profile       *settingsmodels.AgentProfile
	WorkspacePath string
	ExecutorType  string
	WorkspaceID   string
	SessionID     string
}

// SkillDeployResult carries the side-effects a successful deploy produced
// that the runtime needs to wire onto the in-flight LaunchRequest:
//   - Metadata: per-executor metadata keys (e.g. kandev_runtime_dir for
//     Docker bind-mounts, skill_manifest_json for Sprites uploads).
//   - InstructionsDir: the on-host or sprite-side path where instruction
//     files were materialised. Office's prompt builder reads from this
//     path; kanban launches that don't consume instructions can ignore it.
type SkillDeployResult struct {
	Metadata        map[string]any
	InstructionsDir string
}

// AgentProfileReader is the minimal surface SkillDeployer implementations and
// the runtime need to fetch a full profile row by id. It mirrors the
// agent settings store's GetAgentProfile method without forcing a full
// import of the store package on every call site.
type AgentProfileReader interface {
	GetAgentProfile(ctx context.Context, id string) (*settingsmodels.AgentProfile, error)
}

// noopSkillDeployer is the default. When skill_ids and desired_skills are
// empty (or no deployer has been wired), this is what runs — a fast-path
// no-op.
type noopSkillDeployer struct{}

// DeploySkills satisfies SkillDeployer with a zero-effort implementation.
func (noopSkillDeployer) DeploySkills(_ context.Context, _ SkillDeployRequest) (SkillDeployResult, error) {
	return SkillDeployResult{}, nil
}

// NoopSkillDeployer returns the default deployer used when no concrete
// strategy has been wired. Kanban-only setups without office wiring get
// this; office plugs its own deployer in via Manager.SetSkillDeployer.
func NoopSkillDeployer() SkillDeployer { return noopSkillDeployer{} }
