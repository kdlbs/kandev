package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle/skill"
)

// concreteDeployer is the actual deployer interface implemented by
// runtime/lifecycle/skill.Deployer. We accept it via an adapter here
// so the lifecycle package doesn't depend on the skill package's
// concrete struct.
type concreteDeployer interface {
	Deploy(ctx context.Context, req skill.Request) (skill.DeployResult, error)
}

// skillDeployerAdapter bridges the skill package's Deployer to the
// lifecycle SkillDeployer interface. It's the seam Wave E uses to plug
// the runtime-tier deployer in via Manager.SetSkillDeployer.
type skillDeployerAdapter struct {
	inner concreteDeployer
}

// NewSkillDeployerAdapter wraps a runtime-tier skill.Deployer as a
// lifecycle.SkillDeployer. cmd/kandev's wiring uses this — office no
// longer ships its own deployer.
func NewSkillDeployerAdapter(inner concreteDeployer) SkillDeployer {
	return &skillDeployerAdapter{inner: inner}
}

// DeploySkills satisfies SkillDeployer by delegating to the inner
// runtime-tier deployer and mapping its result type.
func (a *skillDeployerAdapter) DeploySkills(ctx context.Context, req SkillDeployRequest) (SkillDeployResult, error) {
	if a.inner == nil {
		return SkillDeployResult{}, nil
	}
	res, err := a.inner.Deploy(ctx, skill.Request{
		Profile:       req.Profile,
		WorkspacePath: req.WorkspacePath,
		ExecutorType:  req.ExecutorType,
		WorkspaceID:   req.WorkspaceID,
		SessionID:     req.SessionID,
	})
	if err != nil {
		return SkillDeployResult{}, err
	}
	return SkillDeployResult{
		Metadata:        res.Metadata,
		InstructionsDir: res.InstructionsDir,
	}, nil
}
