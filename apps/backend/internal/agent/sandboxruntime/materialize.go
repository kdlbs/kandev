package sandboxruntime

import (
	"context"
	"github.com/kandev/kandev/internal/common/sandboxproxy"
	"slices"
	"time"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/processcontainment"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func wirePolicy(resolved *policy.Resolved, grants contract.Grants) *pluginsdk.SandboxPolicySnapshot {
	result := &pluginsdk.SandboxPolicySnapshot{PolicyId: resolved.Revision.Policy.ID, Revision: uint64(resolved.Revision.Number), Digest: resolved.Revision.Digest, CeilingDigest: resolved.Ceiling.Digest, Grants: &pluginsdk.SandboxGrants{KandevMcp: grants.KandevMCP, EnvironmentKeys: grants.EnvironmentKeys}}
	for _, fs := range grants.Filesystem {
		result.Grants.Filesystem = append(result.Grants.Filesystem, &pluginsdk.SandboxFilesystemGrant{Path: fs.Path, Writable: fs.Writable})
	}
	for _, network := range grants.Network {
		result.Grants.Network = append(result.Grants.Network, &pluginsdk.SandboxNetworkGrant{Host: network.Host, Port: uint32(network.Port), Protocol: network.Protocol})
	}
	for _, source := range resolved.Revision.Policy.Sources {
		result.Sources = append(result.Sources, &pluginsdk.SandboxPolicySource{Scope: source.Kind, SourceId: source.ID, Digest: source.Digest})
	}
	return result
}

func (p *Prepared) materialize(ctx context.Context, binary *plugins.SandboxRuntimeBinary, compiled *pluginsdk.SandboxCompiledPolicy, grants contract.Grants) error {
	wire := p.description.ApprovalWire
	if wire == nil {
		return policy.ErrIneligible
	}
	ports := make([]int, 0, len(p.runtimeNetwork))
	for _, grant := range p.runtimeNetwork {
		ports = append(ports, int(grant.Port))
	}
	channel, err := p.manager.channels.Open(p.manager.ctx, p.orgID, p.request.AttemptId, ports, policy.ApprovalReplies{Allow: wire.AllowOnceBodyUtf8, Deny: wire.DenyBodyUtf8, Timeout: wire.TimeoutBodyUtf8})
	if err != nil {
		return err
	}
	p.channel = channel
	values := map[string]string{"sandbox.approval.endpoint": channel.URL()}
	for _, ref := range p.childDirectoryReferences() {
		values[ref.Key] = ref.Value
	}
	if slices.Contains(compiled.RequiredCapabilities, "egress_proxy") {
		p.egress, err = sandboxproxy.Open(p.manager.ctx, p.runtimeNetwork, p.protectedPorts...)
		if err != nil {
			return err
		}
		values["sandbox.egress.endpoint"] = p.egress.Address()
	}
	artifacts := make([]contract.RuntimeArtifact, 0, len(compiled.Artifacts))
	for _, artifact := range compiled.Artifacts {
		value := contract.RuntimeArtifact{Name: artifact.Name, Content: artifact.ContentUtf8}
		for _, binding := range artifact.Bindings {
			value.Bindings = append(value.Bindings, contract.ArtifactBinding{Path: binding.Path, Reference: binding.RuntimeReference})
		}
		artifacts = append(artifacts, value)
	}
	p.files, err = contract.MaterializeRuntime(ctx, p.manager.base, binary.Content, binary.Digest, artifacts, values)
	if err != nil {
		return err
	}
	prefix, err := materializedPrefix(compiled.Prefix, p.files)
	if err != nil {
		return err
	}
	p.launch = &contract.Launch{Version: 1, AttemptID: p.request.AttemptId, Generation: p.request.Generation, PolicyDigest: p.policy.Digest, PackageDigest: binary.PackageDigest, Root: p.files.Root, Prefix: prefix, Digests: p.files.Digests, EnvironmentKeys: grants.EnvironmentKeys}
	p.launch.ChildHome = p.agentState.Directories().Home
	p.launch.Containment, err = processcontainment.Create()
	if err != nil {
		return err
	}
	if p.mcp != nil {
		p.launch.MCPURL = p.mcp.URL()
	}
	return p.launch.Validate()
}

func materializedPrefix(arguments []*pluginsdk.SandboxArgument, files *contract.RuntimeFiles) ([]string, error) {
	result := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		switch value := argument.Value.(type) {
		case *pluginsdk.SandboxArgument_Literal:
			result = append(result, value.Literal)
		case *pluginsdk.SandboxArgument_RuntimeReference:
			if value.RuntimeReference != pluginsdk.SandboxRuntimeReferenceKey {
				return nil, contract.ErrInvalidPolicy
			}
			result = append(result, files.Executable)
		case *pluginsdk.SandboxArgument_ArtifactReference:
			path, ok := files.Artifacts[value.ArtifactReference]
			if !ok {
				return nil, contract.ErrInvalidPolicy
			}
			result = append(result, path)
		default:
			return nil, contract.ErrInvalidPolicy
		}
	}
	return result, nil
}

func (p *Prepared) operation() *pluginsdk.SandboxRequestContext {
	return &pluginsdk.SandboxRequestContext{ProtocolMajor: 1, InstallationId: p.binding.InstallationID, ProviderKey: p.binding.ProviderKey, OperationId: uuid.NewString(), AttemptId: p.request.AttemptId, ExecutionId: p.request.ExecutionId, Generation: p.request.Generation}
}

func (p *Prepared) discard() {
	if p.launch != nil && p.launch.Containment != nil {
		_ = p.launch.Containment.Remove()
	}
	_ = p.releaseAgentState()
	if p.mcp != nil {
		_ = p.mcp.Close()
	}
	if p.channel != nil {
		_ = p.channel.Close()
	}
	if p.egress != nil {
		_ = p.egress.Close()
	}
	if p.files != nil {
		_ = p.files.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	attempt, err := p.manager.policies.ReadAttempt(ctx, p.orgID, p.request.AttemptId)
	if err == nil {
		_, _ = p.manager.policies.TransitionAttempt(ctx, p.orgID, attempt.ID, attempt.Version, policy.AttemptFailed, nil)
	}
}
