package sandboxruntime

import (
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kdlbs/kandev/pluginsdk"
)

func wirePermissionsV2(permissions contract.PermissionsV2) *pluginsdk.SandboxPermissionsV2 {
	result := &pluginsdk.SandboxPermissionsV2{KandevMcp: permissions.KandevMCP, EnvironmentKeys: permissions.EnvironmentKeys}
	for _, grant := range permissions.Filesystem {
		result.Filesystem = append(result.Filesystem, &pluginsdk.SandboxFilesystemPermissionV2{Path: grant.Path, Reference: grant.Reference, Writable: grant.Writable})
	}
	for _, destination := range permissions.TCP {
		result.Tcp = append(result.Tcp, &pluginsdk.SandboxTCPDestinationV2{Host: destination.Host, Port: uint32(destination.Port), Transport: destination.Transport})
	}
	return result
}

func (p *Prepared) resolveRuntimePolicy(req PrepareRequest, resolved *policy.Resolved) (contract.Grants, *contract.PolicyV2, error) {
	if typed := resolved.Revision.Policy.PolicyV2; typed != nil {
		value, err := contract.ResolveAttemptPolicyV2(*typed, req.WorkDir, p.agentState, []string{p.manager.base}, req.SessionID)
		if err != nil {
			return contract.Grants{}, nil, err
		}
		static, err := value.StaticAccess()
		return static, &value, err
	}
	static, err := contract.ResolveAttemptGrants(resolved.Revision.Policy.Grants, req.WorkDir, p.agentState, []string{p.manager.base}, req.SessionID)
	return static, nil, err
}
