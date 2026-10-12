package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/toolschema"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"go.uber.org/zap"
)

const maxAgentToolResultBytes = 1 << 20

const agentToolOutcomeError = "error"

type AgentToolInvocationContext struct {
	InvocationID      string
	TaskID            string
	SessionID         string
	WorkspaceID       string
	Surface           string
	ExecutionID       string
	ManagedToolPolicy *mcpprofile.ManagedToolPolicy
}

// AgentToolCatalog returns the current active-plugin tool snapshot. The
// revision changes only when the effective tool set changes; generation is
// stable for the lifetime of the Service instance.
func (s *Service) AgentToolCatalog() (plugintools.Snapshot, error) {
	tools, err := buildAgentToolDefinitions(s.registry.List())
	if err != nil {
		return plugintools.Snapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agentToolGeneration == "" {
		s.agentToolGeneration = fmt.Sprintf("service-%d", time.Now().UnixNano())
	}
	current := plugintools.Snapshot{Generation: s.agentToolGeneration, Tools: tools}
	previous := s.agentToolSnapshot
	previous.Revision = 0
	if !s.agentToolSnapshotReady || !plugintools.Equal(previous, current) {
		s.agentToolRevision++
		current.Revision = s.agentToolRevision
		s.agentToolSnapshot = plugintools.Normalize(current)
		s.agentToolSnapshotReady = true
	}
	return plugintools.Normalize(s.agentToolSnapshot), nil
}

func buildAgentToolDefinitions(records []*store.Record) ([]plugintools.Definition, error) {
	seen := make(map[string]string)
	var definitions []plugintools.Definition
	for _, record := range records {
		if record.Status != StatusActive {
			continue
		}
		for _, tool := range record.AgentTools {
			input, err := json.Marshal(tool.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("plugin %q tool %q input schema: %w", record.ID, tool.Name, err)
			}
			var output json.RawMessage
			if len(tool.OutputSchema) > 0 {
				output, err = json.Marshal(tool.OutputSchema)
				if err != nil {
					return nil, fmt.Errorf("plugin %q tool %q output schema: %w", record.ID, tool.Name, err)
				}
			}
			exposed := plugintools.ExposedName(record.ID, tool.Name)
			if previous, ok := seen[exposed]; ok && previous != record.ID {
				return nil, fmt.Errorf("agent tool name collision: %q from %q and %q", exposed, previous, record.ID)
			}
			seen[exposed] = record.ID
			definitions = append(definitions, plugintools.Definition{
				PluginID:          record.ID,
				PluginDisplayName: record.DisplayName,
				LocalName:         tool.Name,
				ExposedName:       exposed,
				Description:       tool.Description,
				Surfaces:          append([]string(nil), tool.Surfaces...),
				InputSchema:       input,
				OutputSchema:      output,
				ReadOnlyHint:      boolValue(tool.Annotations.ReadOnlyHint, false),
				DestructiveHint:   boolValue(tool.Annotations.DestructiveHint, true),
				IdempotentHint:    boolValue(tool.Annotations.IdempotentHint, false),
				OpenWorldHint:     boolValue(tool.Annotations.OpenWorldHint, true),
			})
		}
	}
	return plugintools.Normalize(plugintools.Snapshot{Tools: definitions}).Tools, nil
}

func (s *Service) validateAgentToolInstall(candidate *manifest.Manifest) error {
	if candidate == nil {
		return nil
	}
	seen := make(map[string]string)
	for _, record := range s.registry.List() {
		if record.ID == candidate.ID {
			continue
		}
		for _, tool := range record.AgentTools {
			seen[plugintools.ExposedName(record.ID, tool.Name)] = record.ID
		}
	}
	for _, tool := range candidate.AgentTools {
		exposed := plugintools.ExposedName(candidate.ID, tool.Name)
		if previous, ok := seen[exposed]; ok && previous != candidate.ID {
			return fmt.Errorf("agent tool name collision: %q from %q and %q", exposed, previous, candidate.ID)
		}
	}
	return nil
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// InvokeAgentTool validates the declaration and arguments, then forwards one
// bounded call to the plugin's gRPC subprocess. It deliberately does not
// retry because tools may have side effects.
func (s *Service) InvokeAgentTool(ctx context.Context, pluginID, localName string, arguments map[string]any, invocation AgentToolInvocationContext) (result *pluginsdk.AgentToolResult, err error) {
	if invocation.ManagedToolPolicy != nil || invocation.Surface == manifest.AgentToolSurfaceManaged {
		return s.invokeManagedAgentTool(ctx, pluginID, localName, arguments, invocation)
	}
	started := time.Now()
	defer func() {
		if s.log == nil {
			return
		}
		outcome := "success"
		if err != nil {
			outcome = agentToolOutcomeError
		} else if result != nil && result.IsError {
			outcome = "tool_error"
		}
		s.log.Info("plugin agent tool invocation",
			zap.String("invocation_id", invocation.InvocationID),
			zap.String("plugin_id", pluginID),
			zap.String("local_name", localName),
			zap.String("task_id", invocation.TaskID),
			zap.String("session_id", invocation.SessionID),
			zap.Duration("duration", time.Since(started)),
			zap.String("outcome", outcome))
	}()

	record, err := s.Get(pluginID)
	if err != nil {
		return nil, err
	}
	if record.Status != StatusActive {
		return nil, fmt.Errorf("plugins: plugin %q is not active", pluginID)
	}
	declaration, err := findAgentTool(record.AgentTools, localName, invocation.Surface)
	if err != nil {
		return nil, err
	}
	arguments, err = validateAgentToolArguments(pluginID, localName, declaration, arguments)
	if err != nil {
		return nil, err
	}
	remote, ok := s.pluginRemote(pluginID)
	if !ok {
		return nil, fmt.Errorf("plugins: plugin %q is not running", pluginID)
	}
	result, err = invokeRemoteAgentTool(ctx, remote, localName, arguments, invocation)
	if err != nil {
		return nil, err
	}
	return validateAgentToolResult(pluginID, localName, declaration, result)
}

func (s *Service) invokeManagedAgentTool(ctx context.Context, pluginID, localName string, arguments map[string]any, invocation AgentToolInvocationContext) (result *pluginsdk.AgentToolResult, err error) {
	return s.invokeManagedAgentToolWithRemote(ctx, pluginID, localName, arguments, invocation, func() (managedAgentToolRemote, error) {
		remote, ok := s.pluginRemote(pluginID)
		if !ok {
			return nil, fmt.Errorf("plugins: plugin %q is not running", pluginID)
		}
		return remote, nil
	})
}

type managedAgentToolRemote interface {
	InvokeAgentTool(context.Context, *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error)
}

//nolint:cyclop,funlen // The approval and provenance snapshot brackets the callback without holding the host lock.
func (s *Service) invokeManagedAgentToolWithRemote(
	ctx context.Context,
	pluginID, localName string,
	arguments map[string]any,
	invocation AgentToolInvocationContext,
	resolveRemote func() (managedAgentToolRemote, error),
) (result *pluginsdk.AgentToolResult, err error) {
	started := time.Now()
	var approvalRevision uint64
	if invocation.ManagedToolPolicy != nil {
		approvalRevision = invocation.ManagedToolPolicy.ApprovalRevision
	}
	defer func() {
		if s.log == nil {
			return
		}
		outcome := "success"
		if err != nil {
			outcome = agentToolOutcomeError
		} else if result != nil && result.IsError {
			outcome = "tool_error"
		}
		s.log.Info("managed plugin agent tool invocation",
			zap.String("audit_id", CanonicalApprovalDigest(pluginID, invocation.InvocationID, invocation.TaskID, invocation.SessionID, localName)),
			zap.String("plugin_id", pluginID),
			zap.String("local_name", localName),
			zap.String("task_id", invocation.TaskID),
			zap.String("session_id", invocation.SessionID),
			zap.Uint64("approval_revision", approvalRevision),
			zap.Duration("duration", time.Since(started)),
			zap.String("outcome", outcome))
	}()
	policy := invocation.ManagedToolPolicy
	if invocation.Surface != manifest.AgentToolSurfaceManaged || policy == nil || policy.Validate() != nil ||
		!policy.Allows(pluginID, localName) || policy.PluginID != pluginID ||
		policy.WorkspaceID != invocation.WorkspaceID || invocation.ExecutionID == "" ||
		invocation.TaskID == "" || invocation.SessionID == "" || invocation.InvocationID == "" {
		return nil, fmt.Errorf("plugins: managed agent tool policy denied the invocation")
	}

	dispatchLock := s.dispatchLocks.lockFor(pluginID)
	dispatchLock.RLock()
	defer dispatchLock.RUnlock()
	s.approvalEffectMu.Lock()

	record, err := s.Get(pluginID)
	if err != nil {
		s.approvalEffectMu.Unlock()
		return nil, err
	}
	if record.Status != StatusActive || record.InstallationID != policy.InstallationID ||
		ManifestCapabilityDigest(record.Manifest) != policy.ManifestDigest {
		s.approvalEffectMu.Unlock()
		return nil, fmt.Errorf("plugins: managed agent tool installation or manifest is stale")
	}
	capability := "host.v2.write:managed_agent_tools"
	declared, err := ManifestCapabilityIDs(record.Manifest)
	if err != nil || !containsString(declared, capability) {
		s.approvalEffectMu.Unlock()
		return nil, fmt.Errorf("plugins: managed agent tools capability is not declared")
	}
	requestDigest := CanonicalApprovalDigest("managed-agent-tool", invocation.InvocationID, invocation.TaskID, invocation.SessionID, localName)
	decision := s.authorizePluginCapability(
		policy.InstallationID, policy.WorkspaceID, capability, policy.ApprovalRevision,
		requestDigest, CanonicalApprovalDigest("host-agent-tool", pluginID, localName, manifest.AgentToolSurfaceManaged, "v2"),
	)
	if !decision.Allowed {
		s.approvalEffectMu.Unlock()
		return nil, fmt.Errorf("plugins: managed agent tool capability denied: %s", decision.Reason)
	}
	declaration, err := findAgentTool(record.AgentTools, localName, manifest.AgentToolSurfaceManaged)
	if err != nil {
		s.approvalEffectMu.Unlock()
		return nil, err
	}
	arguments, err = validateAgentToolArguments(pluginID, localName, declaration, arguments)
	if err != nil {
		s.approvalEffectMu.Unlock()
		return nil, err
	}
	remote, err := resolveRemote()
	if err != nil {
		s.approvalEffectMu.Unlock()
		return nil, err
	}
	// Host callbacks perform their own authorization at the effect boundary.
	// Do not hold approvalEffectMu while plugin code runs: a callback can enter
	// any Host method that also needs this mutex.
	s.approvalEffectMu.Unlock()
	result, err = invokeRemoteAgentTool(ctx, remote, localName, arguments, invocation)
	if err != nil {
		return nil, err
	}
	return validateAgentToolResult(pluginID, localName, declaration, result)
}

func findAgentTool(tools []manifest.AgentTool, localName, surface string) (*manifest.AgentTool, error) {
	for i := range tools {
		if tools[i].Name != localName {
			continue
		}
		if !containsString(tools[i].Surfaces, surface) {
			return nil, fmt.Errorf("plugins: agent tool %q is not available on surface %q", localName, surface)
		}
		return &tools[i], nil
	}
	return nil, fmt.Errorf("plugins: agent tool %q is not declared", localName)
}

func validateAgentToolArguments(pluginID, localName string, declaration *manifest.AgentTool, arguments map[string]any) (map[string]any, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	inputSchema, err := compileInvocationSchema(pluginID+"/"+localName, declaration.InputSchema)
	if err != nil {
		return nil, fmt.Errorf("plugins: invalid input schema: %w", err)
	}
	if err := inputSchema.Validate(arguments); err != nil {
		return nil, fmt.Errorf("plugins: invalid arguments for %q", localName)
	}
	return arguments, nil
}

func invokeRemoteAgentTool(ctx context.Context, remote interface {
	InvokeAgentTool(context.Context, *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error)
}, localName string, arguments map[string]any, invocation AgentToolInvocationContext) (*pluginsdk.AgentToolResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	toolContext := pluginsdk.AgentToolContext{
		TaskID: invocation.TaskID, SessionID: invocation.SessionID,
		WorkspaceID: invocation.WorkspaceID, Surface: invocation.Surface,
		ExecutionID: invocation.ExecutionID,
	}
	if policy := invocation.ManagedToolPolicy; policy != nil {
		toolContext.InstallationID = policy.InstallationID
		toolContext.ConversationRevision = policy.ConversationRevision
		toolContext.ApprovalRevision = policy.ApprovalRevision
		toolContext.ManifestDigest = policy.ManifestDigest
		toolContext.AgentToolNames = append([]string(nil), policy.AgentToolNames...)
	}
	return remote.InvokeAgentTool(callCtx, &pluginsdk.AgentToolRequest{
		InvocationID: invocation.InvocationID,
		Name:         localName,
		Arguments:    arguments,
		Context:      toolContext,
	})
}

func validateAgentToolResult(pluginID, localName string, declaration *manifest.AgentTool, result *pluginsdk.AgentToolResult) (*pluginsdk.AgentToolResult, error) {
	if result == nil || strings.TrimSpace(result.Text) == "" {
		return nil, errors.New("plugins: agent tool returned empty text")
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, fmt.Errorf("plugins: encode agent tool result: %w", err)
	}
	if len(result.Text)+len(encoded) > maxAgentToolResultBytes {
		return nil, errors.New("plugins: agent tool result exceeds 1 MiB")
	}
	if len(declaration.OutputSchema) > 0 && result.StructuredContent != nil {
		outputSchema, err := toolschema.Compile(pluginID+"/"+localName+"/output", declaration.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("plugins: invalid output schema: %w", err)
		}
		if err := outputSchema.Validate(result.StructuredContent); err != nil {
			return nil, errors.New("plugins: agent tool result does not match output schema")
		}
	}
	return result, nil
}

func compileInvocationSchema(name string, document map[string]any) (*jsonschema.Schema, error) {
	copy := make(map[string]any, len(document)+1)
	for key, value := range document {
		copy[key] = value
	}
	copy["additionalProperties"] = false
	return toolschema.Compile(name, copy)
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
