package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/task/models"
)

type sandboxUtilityAdmission struct {
	launch  sandboxLaunchAdmission
	targets sandboxTargetResolver
}

func (a sandboxUtilityAdmission) admit(ctx context.Context, probe *hostutility.ProfileProbeContext) error {
	if probe != nil {
		probe.SandboxSelection = nil
		probe.SandboxDigest = ""
	}
	req := &runtime.LaunchRequest{}
	if probe == nil || probe.ExecutorID == "" {
		return a.launch.admit(ctx, req)
	}
	if _, err := a.targets.resolve(ctx, sandbox.ScopeExecutor, probe.ExecutorID); err != nil {
		return err
	}
	if probe.ProfileID != "" {
		target, err := a.targets.resolve(ctx, sandbox.ScopeAgent, probe.ProfileID)
		if err != nil {
			return err
		}
		req.WorkspaceID = target.WorkspaceID
	}
	req.AgentProfileID = probe.ProfileID
	req.Metadata = map[string]any{models.MetaKeyExecutorID: probe.ExecutorID}
	if err := a.launch.admit(ctx, req); err != nil {
		return err
	}
	if req.SandboxSelection != nil {
		digest, err := a.launch.runtime.SelectionDigest(ctx, req.SandboxSelection.Prior)
		if err != nil {
			return sandbox.ErrIneligible
		}
		probe.SandboxSelection = req.SandboxSelection
		probe.SandboxDigest = digest
	}
	return nil
}
