package process

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// ActivateProviderReplacement configures and starts one successor inside the
// retained instance. The API persists starting before invoking this primitive.
func (m *Manager) ActivateProviderReplacement(ctx context.Context, req types.ProviderReplacementRequest) (uint64, error) {
	release, err := m.BeginProviderOperation(req.OperationID)
	if err != nil {
		return 0, err
	}
	defer release()
	if err := m.verifyReplacementDelivery(ctx, req.Source); err != nil {
		return 0, err
	}
	profile, servers, err := m.replacementAttachment(req)
	if err != nil {
		return 0, err
	}
	args, env, err := m.replacementCommand(req)
	if err != nil {
		return 0, err
	}
	source := providerReplacementSource{managerID: req.Source.HelperID, processGeneration: req.Source.ProcessGeneration, configRevision: req.Source.ConfigurationRevision, stopGeneration: req.Source.StopGeneration}
	source, err = m.configureProviderForReplacement(source, args, env)
	if err != nil {
		return 0, err
	}
	if err := m.publishReplacementAttachment(source, req, profile, servers); err != nil {
		return 0, err
	}
	generation, err := m.startProviderForReplacement(ctx, source)
	if err != nil {
		return generation, err
	}
	if generation != req.Scope.ProcessGeneration {
		_, _ = m.stopProviderForReplacement(ctx, generation)
		return generation, errors.New("provider successor generation differs from its binding")
	}
	return generation, nil
}

func (m *Manager) replacementAttachment(req types.ProviderReplacementRequest) (p.Context, []config.McpServerConfig, error) {
	encoded, err := p.MarshalBuiltinBinding(req.Profile)
	if err != nil {
		return p.Context{}, nil, err
	}
	frozen, err := p.ParseBuiltinBindingMetadata(encoded)
	if err != nil {
		return p.Context{}, nil, err
	}
	servers, err := config.ProviderBuiltinMCPServersForInstance(*frozen, config.ProviderMCPConfig{Scope: req.Scope, Credential: req.ScopeCredential}, m.cfg.MCPHost, m.cfg.Port, m.cfg.InstanceID, m.cfg.ScopedMCPRoutes)
	if err != nil {
		return p.Context{}, nil, err
	}
	if !m.validReplacementOwner(req) {
		return p.Context{}, nil, errors.New("provider successor owner is invalid")
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.cfg.McpProfile == nil || m.cfg.McpProfile.BuiltinPackages == nil || frozen.BuiltinPackages.WorkspaceID != m.cfg.McpProfile.BuiltinPackages.WorkspaceID || frozen.Surface != m.cfg.McpProfile.Surface {
		return p.Context{}, nil, errors.New("provider successor workspace changed")
	}
	for _, existing := range m.cfg.McpServers {
		if p.ReservedBuiltinRuntimeName(existing.Name) {
			continue
		}
		existing.Args = slices.Clone(existing.Args)
		existing.Env = maps.Clone(existing.Env)
		existing.Headers = maps.Clone(existing.Headers)
		servers = append(servers, existing)
	}
	return *frozen, servers, nil
}

func (m *Manager) replacementCommand(req types.ProviderReplacementRequest) ([]string, map[string]string, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	current, err := m.providerReplacementSourceLocked()
	if err != nil {
		return nil, nil, err
	}
	current.NativeSessionID = req.Source.NativeSessionID
	if current != req.Source || m.Status() != StatusStopped {
		return nil, nil, errors.New("provider source is not retired")
	}
	args := slices.Clone(req.Args)
	if len(args) == 0 {
		args = slices.Clone(m.cfg.AgentArgs)
	}
	env := maps.Clone(req.Env)
	if env == nil {
		env = make(map[string]string, len(m.cfg.AgentEnv))
		for _, entry := range m.cfg.AgentEnv {
			if key, value, ok := strings.Cut(entry, "="); ok {
				env[key] = value
			}
		}
	}
	return args, env, nil
}

func (m *Manager) publishReplacementAttachment(source providerReplacementSource, req types.ProviderReplacementRequest, profile p.Context, servers []config.McpServerConfig) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if source.configRevision != m.configRevision || source.stopGeneration != m.stopGeneration || source.processGeneration != m.ProcessGeneration() || m.Status() != StatusStopped {
		return errors.New("provider replacement configuration changed")
	}
	m.requireProviderMCPProtocol()
	m.cfg.McpProfile = &profile
	m.cfg.McpServers = servers
	m.cfg.ProviderMCP = &config.ProviderMCPConfig{Scope: req.Scope, Credential: req.ScopeCredential}
	return nil
}

func (m *Manager) validReplacementOwner(req types.ProviderReplacementRequest) bool {
	return req.Scope.ExecutionID == m.cfg.InstanceID && req.Scope.HelperID == m.ProcessManagerID() && req.Scope.ProcessGeneration == req.Source.ProcessGeneration+1 && req.Scope.HarnessGeneration == req.Source.HarnessGeneration && req.Profile.BuiltinPackages.TaskID == m.cfg.TaskID && req.TargetStreamID == req.Source.StreamID && req.TargetStreamID != ""
}

func (m *Manager) verifyReplacementDelivery(ctx context.Context, source types.ProviderReplacementSource) error {
	if m.DeliveryStreamID() != source.StreamID || m.DeliveryHarnessGeneration() != source.HarnessGeneration || m.DeliveryIncarnationID() != source.IncarnationID {
		return errors.New("provider replacement cannot change conversation delivery ownership")
	}
	descriptor, err := m.DeliveryRecoveryDescriptor(ctx, source.StreamID)
	if err != nil {
		return err
	}
	if !descriptor.Durable || descriptor.Unresolved || descriptor.SubmissionsTruncated {
		return errors.New("source delivery has not settled")
	}
	return nil
}
