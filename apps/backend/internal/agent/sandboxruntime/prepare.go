package sandboxruntime

import (
	"context"
	"runtime"
	"slices"

	"github.com/google/uuid"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (m *Manager) Prepare(ctx context.Context, req PrepareRequest) (prepared *Prepared, err error) {
	if req.ExecutionID == "" || req.Generation == 0 {
		return nil, contract.ErrInvalidPolicy
	}
	resolved, previous, err := m.resolve(ctx, req)
	if err != nil || resolved == nil {
		return nil, err
	}
	binding, err := m.selectProvider(ctx, resolved.Revision.Policy.InstallationID, resolved.Revision.Policy.ProviderKey)
	if err != nil {
		return nil, err
	}
	if previous != nil && (previous.Snapshot.Provider.Version != binding.Version || previous.Snapshot.Provider.PackageDigest != binding.PackageDigest) {
		return nil, policy.ErrIneligible
	}
	p := &Prepared{manager: m, orgID: req.Selection.OrgID, binding: binding, protectedPorts: slices.Clone(req.ProtectedPorts)}
	p.selectionDigest, err = selectionDigest(resolved, binding)
	if err != nil {
		return nil, err
	}
	p.request = &pluginsdk.SandboxRequestContext{ProtocolMajor: 1, InstallationId: binding.InstallationID, ProviderKey: binding.ProviderKey, OperationId: uuid.NewString(), AttemptId: uuid.NewString(), ExecutionId: req.ExecutionID, Generation: req.Generation}
	defer func() {
		if err != nil {
			p.discard()
		}
	}()
	compiled, binary, grants, err := p.compile(ctx, req, resolved)
	if err != nil {
		return nil, err
	}
	if err = p.materialize(ctx, binary, compiled.Compiled, grants); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, policy.ErrConflict
	}
	_, err = m.policies.CaptureAttempt(ctx, policy.CaptureRequest{RestartOf: req.RestartOf, Selection: req.Selection, Resolved: resolved, SessionID: req.SessionID, ExecutionID: req.ExecutionID, WorkDir: req.WorkDir, AgentState: p.agentState, Launch: p.launch, Provider: policy.AttemptProvider{PluginID: binding.PluginID, InstallationID: binding.InstallationID, Key: binding.ProviderKey, Version: binding.Version, PackageDigest: binding.PackageDigest, RuntimeVersion: p.description.RuntimeVersion, RuntimeDigest: binary.Digest}})
	if err != nil {
		return nil, err
	}
	m.attempts[p.launch.AttemptID] = p
	return p, nil
}

func (m *Manager) resolve(ctx context.Context, req PrepareRequest) (*policy.Resolved, *policy.Attempt, error) {
	if req.Selection.RestartReview != nil {
		return m.policies.ResolveReviewedRestart(ctx, req.Selection, req.SessionID, req.WorkDir, req.RestartOf)
	}
	if req.RestartOf != "" {
		previous, err := m.policies.ResolveRestart(ctx, req.Selection, req.SessionID, req.WorkDir, req.RestartOf)
		if err != nil {
			return nil, nil, err
		}
		return previous.Snapshot.Resolved, previous, nil
	}
	if req.Selection.TaskID != "" {
		resolved, err := m.policies.AdmitTask(ctx, req.Selection, req.Selection.ParentTaskID)
		return resolved, nil, err
	}
	resolved, err := m.policies.Resolve(ctx, req.Selection)
	return resolved, nil, err
}

func (m *Manager) selectProvider(ctx context.Context, installationID, key string) (plugins.SandboxProviderBinding, error) {
	providers, err := m.providers.ListSandboxProviders(ctx)
	if err != nil {
		return plugins.SandboxProviderBinding{}, err
	}
	for _, provider := range providers {
		if provider.Binding.InstallationID == installationID && provider.Binding.ProviderKey == key && provider.PluginRunning {
			return provider.Binding, nil
		}
	}
	return plugins.SandboxProviderBinding{}, plugins.ErrSandboxProviderUnavailable
}

func validateCompiledOverrides(req PrepareRequest, compiled *pluginsdk.SandboxCompiledPolicy) error {
	for _, capability := range compiled.RequiredCapabilities {
		if !slices.Contains([]string{"private_artifacts", "foreground_process", "approval_webhook", "isolated_environment", "egress_proxy"}, capability) {
			return policy.ErrIneligible
		}
	}
	constraints := compiled.GetConstraints()
	return contract.ValidateOverrides(req.Arguments, req.EnvironmentOverrides, contract.OverrideConstraints{ForbiddenArguments: constraints.GetForbiddenArguments(), ForbiddenEnvironment: constraints.GetForbiddenEnvironmentKeys()})
}

func (p *Prepared) compile(ctx context.Context, req PrepareRequest, resolved *policy.Resolved) (*pluginsdk.CompileSandboxPolicyResponse, *plugins.SandboxRuntimeBinary, contract.Grants, error) {
	facts := &pluginsdk.SandboxRuntimeFacts{Os: runtime.GOOS, Architecture: runtime.GOARCH, Transport: "local"}
	description, err := p.manager.providers.DescribeSandboxProvider(ctx, p.binding, &pluginsdk.DescribeSandboxProviderRequest{Context: p.request, Runtime: facts})
	if err != nil {
		return nil, nil, contract.Grants{}, err
	}
	p.description = description.Description
	if !slices.Contains(p.description.SupportedTransports, "local") {
		return nil, nil, contract.Grants{}, policy.ErrIneligible
	}
	binary, err := p.manager.providers.ReadSandboxRuntime(ctx, p.binding, p.description.RuntimeArtifact)
	if err != nil {
		return nil, nil, contract.Grants{}, err
	}
	facts.RuntimeVersion, facts.RuntimeDigest = p.description.RuntimeVersion, binary.Digest
	if err = p.prepareAgentState(req, resolved); err != nil {
		return nil, nil, contract.Grants{}, err
	}
	grants, err := contract.ResolveAttemptGrants(resolved.Revision.Policy.Grants, req.WorkDir, p.agentState, []string{p.manager.base}, req.SessionID)
	if err != nil {
		return nil, nil, contract.Grants{}, err
	}
	p.policy = wirePolicy(resolved, grants)
	endpoints, err := p.prepareMCP(req, grants)
	if err != nil {
		return nil, nil, contract.Grants{}, err
	}
	compiled, err := p.manager.providers.CompileSandboxPolicy(ctx, p.binding, &pluginsdk.CompileSandboxPolicyRequest{Context: p.request, Policy: p.policy, Runtime: facts, Endpoints: endpoints, References: p.childDirectoryReferences()})
	if err != nil {
		return nil, nil, contract.Grants{}, err
	}
	if err = validateCompiledOverrides(req, compiled.Compiled); err != nil {
		return nil, nil, contract.Grants{}, err
	}
	p.constraints = contract.OverrideConstraints{ForbiddenArguments: slices.Clone(compiled.Compiled.GetConstraints().GetForbiddenArguments()), ForbiddenEnvironment: slices.Clone(compiled.Compiled.GetConstraints().GetForbiddenEnvironmentKeys())}
	return compiled, binary, grants, nil
}
