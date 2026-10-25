package sandboxruntime

import (
	"slices"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (p *Prepared) prepareAgentState(req PrepareRequest, resolved *policy.Resolved) error {
	if !slices.ContainsFunc(resolved.Revision.Policy.Grants.Filesystem, func(grant contract.FilesystemGrant) bool { return grant.Reference == contract.ReferenceAgentState }) {
		return nil
	}
	scope := contract.AgentStateScope{OrgID: req.Selection.OrgID, WorkspaceID: req.Selection.WorkspaceID, SessionID: req.SessionID, AgentProfileID: req.Selection.AgentProfileID, InstallationID: p.binding.InstallationID, ProviderKey: p.binding.ProviderKey}
	if scope.SessionID == "" {
		scope.ExecutionID = req.ExecutionID
	}
	state, err := contract.OpenAgentState(p.manager.base, scope)
	if err != nil {
		return err
	}
	p.agentState = state
	return nil
}

func (p *Prepared) childDirectoryReferences() []*pluginsdk.SandboxRuntimeReference {
	if p.agentState == nil {
		return nil
	}
	dirs := p.agentState.Directories()
	return childDirectoryReferences(dirs)
}

func childDirectoryReferences(dirs contract.RuntimeDirectories) []*pluginsdk.SandboxRuntimeReference {
	if dirs.Home == "" {
		return nil
	}
	return []*pluginsdk.SandboxRuntimeReference{
		{Key: "sandbox.child.home", Value: dirs.Home},
		{Key: "sandbox.child.config", Value: dirs.Config},
		{Key: "sandbox.child.state", Value: dirs.State},
		{Key: "sandbox.child.tmp", Value: dirs.Temp},
	}
}

func (p *Prepared) releaseAgentState() error {
	if p.agentState != nil && p.agentState.Snapshot().Scope.SessionID == "" {
		return p.agentState.Remove()
	}
	return p.agentState.Close()
}
