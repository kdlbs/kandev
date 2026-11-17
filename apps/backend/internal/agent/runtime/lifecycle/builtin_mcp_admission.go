package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/types"

	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
)

// verifyBuiltinExecution does not resolve desired policy. A retained process
// must agree with its original binding before it can accept work.
func (m *Manager) verifyBuiltinExecution(ctx context.Context, execution *AgentExecution) error {
	err := m.verifyBuiltinExecutionBinding(ctx, execution)
	if err != nil {
		return &BuiltinMCPRecoveryError{Cause: err}
	}
	return nil
}

// BuiltinMCPRecoveryError prevents a retained binding mismatch from becoming a
// broader fresh-session retry.
type BuiltinMCPRecoveryError struct{ Cause error }

func (e *BuiltinMCPRecoveryError) Error() string          { return "built-in MCP binding requires recovery" }
func (e *BuiltinMCPRecoveryError) Unwrap() error          { return e.Cause }
func (e *BuiltinMCPRecoveryError) RecoveryReason() string { return "builtin_mcp_binding" }

func (m *Manager) verifyBuiltinExecutionBinding(ctx context.Context, execution *AgentExecution) error {
	execution.builtinMCPMu.Lock()
	defer execution.builtinMCPMu.Unlock()
	if execution.providerMCPEffectsClosed() {
		return errors.New("provider MCP application requires recovery")
	}
	if value, present := execution.metadataValue(p.ProviderMCPBindingMetadataKey); present {
		return m.verifyAcceptedProviderBinding(ctx, execution, value)
	}
	value, bound := execution.metadataValue(p.BuiltinBindingMetadataKey)
	if !bound {
		return nil
	}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil {
		return fmt.Errorf("invalid built-in MCP execution binding: %w", err)
	}
	if err := m.validateBuiltinOwner(ctx, execution.TaskID, execution.SessionID, execution.WorkspaceID, profile); err != nil {
		return err
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return errors.New("built-in MCP helper is unavailable")
	}
	if err := client.VerifyBuiltinMCP(ctx, *profile); err != nil {
		return fmt.Errorf("verify built-in MCP execution binding: %w", err)
	}
	return nil
}

func (m *Manager) validateBuiltinOwner(ctx context.Context, taskID, sessionID, workspaceID string, profile *p.Context) error {
	if _, err := p.MarshalBuiltinBinding(*profile); err != nil {
		return fmt.Errorf("invalid built-in MCP profile: %w", err)
	}
	manifest := profile.BuiltinPackages
	if taskID == "" || sessionID == "" || manifest.TaskID != taskID || manifest.WorkspaceID != workspaceID {
		return errors.New("built-in MCP binding does not match the execution owner")
	}
	_, _, scope := m.streamManager.mcpDispatchState()
	if scope == nil {
		return errors.New("built-in MCP principal scope is unavailable")
	}
	scoped, err := scope(ctx, taskID, sessionID)
	if err != nil {
		return fmt.Errorf("resolve built-in MCP principal: %w", err)
	}
	if !mcpscope.BuiltinPrincipalMatches(scoped, manifest) {
		return errors.New("built-in MCP binding does not match the current principal")
	}
	if m.streamManager.builtinResolver() == nil {
		return errors.New("built-in MCP action registry is unavailable")
	}
	return nil
}

func (m *Manager) verifyAcceptedProviderBinding(ctx context.Context, execution *AgentExecution, value any) error {
	binding, _, err := p.ParseProviderMCPBinding(value)
	if err != nil {
		return err
	}
	live, present := execution.metadataValue(p.ProviderMCPRuntimeProfileMetadataKey)
	profile, err := p.ProviderRuntimeProfile(binding, live, present)
	if err != nil {
		return err
	}
	if binding.Scope.ExecutionID != execution.ID {
		return errors.New("provider binding execution changed")
	}
	if err := m.validateBuiltinOwner(ctx, execution.TaskID, execution.SessionID, execution.WorkspaceID, profile); err != nil {
		return err
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return errors.New("provider helper is unavailable")
	}
	status, err := client.GetProviderReplacementStatus(ctx, "")
	if err != nil {
		return err
	}
	source := status.Source
	if !providerSourceMatchesBinding(source, execution, binding) {
		return errors.New("provider helper scope changed")
	}
	if execution.DeliveryHarnessGeneration != 0 && source.HarnessGeneration != execution.DeliveryHarnessGeneration {
		return errors.New("provider delivery owner changed")
	}
	if execution.DeliveryHarnessGeneration == 0 {
		if err := m.verifyProviderRecoveryHarness(ctx, execution, source); err != nil {
			return err
		}
	}
	return client.VerifyBuiltinMCP(ctx, *profile)
}

// Binding verification precedes full journal adoption during recovery. Read the
// SQL owner without publishing a delivery cursor or opening prompt admission.
func (m *Manager) verifyProviderRecoveryHarness(ctx context.Context, execution *AgentExecution, source types.ProviderReplacementSource) error {
	generation, err := loadRecoveredHarnessGeneration(ctx, m.streamManager.deliveryRepository(), execution.SessionID, source.IncarnationID)
	if err != nil {
		return err
	}
	if generation == nil || generation.SessionID != execution.SessionID || generation.IncarnationID != source.IncarnationID || generation.Generation <= 0 || uint64(generation.Generation) != source.HarnessGeneration || source.NativeSessionID == "" || generation.NativeSessionID != source.NativeSessionID {
		return errors.New("provider binding journal owner changed")
	}
	return nil
}

func providerSourceMatchesBinding(source types.ProviderReplacementSource, execution *AgentExecution, binding p.ProviderMCPBinding) bool {
	return source.ExecutionID == execution.ID && source.TaskID == execution.TaskID && source.SessionID == execution.SessionID && source.BindingID == binding.Scope.BindingID && source.ProfileDigest == binding.Scope.ProfileDigest && source.HelperID == binding.Scope.HelperID && source.ProcessGeneration == binding.Scope.ProcessGeneration
}
