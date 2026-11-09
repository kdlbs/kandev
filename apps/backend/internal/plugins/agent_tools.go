package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/toolschema"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"go.uber.org/zap"
)

const maxAgentToolResultBytes = 1 << 20

const agentToolOutcomeError = "error"
const agentToolOutcomeSuccess = "success"
const agentToolOutcomeToolError = "tool_error"

type AgentToolInvocationContext struct {
	InvocationID                 string
	TaskID                       string
	SessionID                    string
	WorkspaceID                  string
	Surface                      string
	ExecutionID                  string
	ManagedToolPolicy            *mcpprofile.ManagedToolPolicy
	AutomationToolAuthorization  *plugintools.AutomationAuthorization
	CoordinatorToolAuthorization *plugintools.CoordinatorToolAuthorization
}

// AgentToolCatalog returns the current tool snapshot of active plugins whose
// required config is set. The revision changes only when the effective tool
// set changes; generation is stable for the lifetime of the Service instance.
func (s *Service) AgentToolCatalog() (plugintools.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tools, err := buildAgentToolDefinitions(s.configReadyAgentToolRecords())
	if err != nil {
		return plugintools.Snapshot{}, err
	}

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

// configReadyAgentToolRecords returns the active plugin records whose
// required config is set. A plugin that cannot serve its tools until the
// operator configures it contributes none; UpdateConfig recomputes the
// catalog once the config is saved.
func (s *Service) configReadyAgentToolRecords() []*store.Record {
	records := s.registry.List()
	ready := make([]*store.Record, 0, len(records))
	for _, record := range records {
		if record.Status == StatusActive && s.requiredConfigSet(record) {
			ready = append(ready, record)
		}
	}
	return ready
}

// requiredConfigSet reports whether every field in the record's
// config_schema "required" list holds a set value (see
// isRequiredConfigValueSet). A record without required fields needs no
// config. A config that cannot be read leaves only this plugin unready, so
// one broken config file cannot drop the other plugins' tools; the warning
// carries the plugin ID only because the error can quote config content.
func (s *Service) requiredConfigSet(record *store.Record) bool {
	required := requiredConfigFields(record.ConfigSchema)
	if len(required) == 0 {
		return true
	}
	config, err := s.store.GetConfig(record.ID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) && s.log != nil {
			s.log.Warn("plugins: agent tools hidden because the plugin config could not be read",
				zap.String("plugin_id", record.ID))
		}
		return false
	}
	for _, field := range required {
		if !isRequiredConfigValueSet(config[field]) {
			return false
		}
	}
	return true
}

// requiredConfigFields returns the string entries of a config_schema
// "required" list, ignoring malformed entries like checkRequiredKeys does.
func requiredConfigFields(schema map[string]any) []string {
	required, _ := schema["required"].([]any)
	fields := make([]string, 0, len(required))
	for _, raw := range required {
		if name, ok := raw.(string); ok {
			fields = append(fields, name)
		}
	}
	return fields
}

// isRequiredConfigValueSet mirrors the settings "Setup required" badge
// (isConfigured in apps/web/hooks/domains/plugins/use-plugin-setup-status.ts):
// an absent key, null, or blank string is unset; any other value, including
// false and 0, is set. A saved secret holds its non-empty vault reference.
// isZeroConfigValue is not used because it treats false and 0 as unset.
func isRequiredConfigValueSet(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return true
	}
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
				InvocationScope:   tool.NormalizedInvocationScope(),
				InstallationID:    record.InstallationID,
				PolicyVersion:     record.AgentToolPolicyVersion,
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

// CoordinatorPluginBindings returns the exact instance-read references a new
// phase-2 coordinator conversation may bind. Only active, configured plugins
// that explicitly declare the coordinator surface are eligible.
func (s *Service) CoordinatorPluginBindings() ([]mcpprofile.CoordinatorPluginTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	records := s.configReadyAgentToolRecords()
	references := make([]mcpprofile.CoordinatorPluginTool, 0)
	for _, record := range records {
		for _, tool := range record.AgentTools {
			if !tool.IsInstanceRead() || !containsString(tool.Surfaces, manifest.AgentToolSurfaceCoordinator) {
				continue
			}
			if record.InstallationID == "" || record.AgentToolPolicyVersion == "" {
				continue
			}
			references = append(references, mcpprofile.CoordinatorPluginTool{
				PluginID:       record.ID,
				LocalName:      tool.Name,
				InstallationID: record.InstallationID,
				PolicyVersion:  record.AgentToolPolicyVersion,
			})
		}
	}
	sort.Slice(references, func(i, j int) bool {
		if references[i].PluginID == references[j].PluginID {
			return references[i].LocalName < references[j].LocalName
		}
		return references[i].PluginID < references[j].PluginID
	})
	return references, nil
}

// ValidateCoordinatorPluginTool revalidates one bound reference against the
// live installation, policy version, declaration, and required config before
// a coordinator invocation or discovery snapshot is allowed.
func (s *Service) ValidateCoordinatorPluginTool(pluginID, localName, installationID, policyVersion string) error {
	record, err := s.Get(pluginID)
	if err != nil {
		return fmt.Errorf("plugins: coordinator plugin tool installation is unavailable")
	}
	if record.Status != StatusActive {
		return fmt.Errorf("plugins: coordinator plugin tool installation is not active")
	}
	if !s.requiredConfigSet(record) {
		return fmt.Errorf("plugins: coordinator plugin tool installation is not configured")
	}
	if record.InstallationID != installationID || record.AgentToolPolicyVersion != policyVersion {
		return fmt.Errorf("plugins: coordinator plugin tool installation or policy version is stale")
	}
	declaration, err := findAgentTool(record.AgentTools, localName, manifest.AgentToolSurfaceCoordinator)
	if err != nil {
		return err
	}
	if !declaration.IsInstanceRead() {
		return fmt.Errorf("plugins: coordinator plugin tool is not an instance read")
	}
	return nil
}

// InvokeAgentTool validates the declaration and arguments, then forwards one
// bounded call to the plugin's gRPC subprocess. It deliberately does not
// retry because tools may have side effects.
func (s *Service) InvokeAgentTool(ctx context.Context, pluginID, localName string, arguments map[string]any, invocation AgentToolInvocationContext) (result *pluginsdk.AgentToolResult, err error) {
	if invocation.Surface != "" && !supportedAgentToolSurface(invocation.Surface) {
		return nil, fmt.Errorf("plugins: agent tool surface %q is unsupported", invocation.Surface)
	}
	if invocation.ManagedToolPolicy != nil || invocation.Surface == manifest.AgentToolSurfaceManaged {
		return s.invokeManagedAgentTool(ctx, pluginID, localName, arguments, invocation)
	}
	if invocation.CoordinatorToolAuthorization != nil || invocation.Surface == manifest.AgentToolSurfaceCoordinator {
		return s.invokeCoordinatorAgentTool(ctx, pluginID, localName, arguments, invocation)
	}
	if invocation.AutomationToolAuthorization != nil || invocation.Surface == manifest.AgentToolSurfaceAutomation {
		return s.invokeAutomationAgentTool(ctx, pluginID, localName, arguments, invocation)
	}
	started := time.Now()
	defer func() {
		if s.log == nil {
			return
		}
		outcome := agentToolOutcomeSuccess
		if err != nil {
			outcome = agentToolOutcomeError
		} else if result != nil && result.IsError {
			outcome = agentToolOutcomeToolError
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

	// Keep record lookup and the RPC in one dispatch lease across lifecycle changes.
	return s.invokeDeclaredAgentTool(ctx, pluginID, localName, arguments, invocation, invocation.Surface, func(record *store.Record) error {
		if record.Status != StatusActive {
			return fmt.Errorf("plugins: plugin %q is not active", pluginID)
		}
		if !s.requiredConfigSet(record) {
			return fmt.Errorf("plugins: plugin %q configuration is not ready", pluginID)
		}
		return nil
	})
}

// invokeCoordinatorAgentTool dispatches one exact instance-read reference and
// rechecks the pinned installation and policy version against the record inside
// the same dispatch lease as the RPC, closing the handler-to-dispatch gap.
func (s *Service) invokeCoordinatorAgentTool(
	ctx context.Context,
	pluginID, localName string,
	arguments map[string]any,
	invocation AgentToolInvocationContext,
) (result *pluginsdk.AgentToolResult, err error) {
	authorization := invocation.CoordinatorToolAuthorization
	started := time.Now()
	defer func() {
		if s.log == nil {
			return
		}
		outcome := agentToolOutcomeSuccess
		if err != nil {
			outcome = agentToolOutcomeError
		} else if result != nil && result.IsError {
			outcome = agentToolOutcomeToolError
		}
		s.log.Info("coordinator plugin agent tool invocation",
			zap.String("invocation_id", invocation.InvocationID),
			zap.String("plugin_id", pluginID),
			zap.String("local_name", localName),
			zap.String("coordinator_id", coordinatorID(authorization)),
			zap.String("workspace_id", invocation.WorkspaceID),
			zap.String("task_id", invocation.TaskID),
			zap.String("session_id", invocation.SessionID),
			zap.Duration("duration", time.Since(started)),
			zap.String("outcome", outcome))
	}()
	if err := validateCoordinatorInvocation(authorization, invocation, pluginID, localName); err != nil {
		return nil, err
	}
	return s.invokeDeclaredAgentTool(ctx, pluginID, localName, arguments, invocation, manifest.AgentToolSurfaceCoordinator, func(record *store.Record) error {
		if record.Status != StatusActive {
			return fmt.Errorf("plugins: coordinator plugin tool installation is not active")
		}
		if record.InstallationID != authorization.InstallationID ||
			record.AgentToolPolicyVersion != authorization.PolicyVersion {
			return fmt.Errorf("plugins: coordinator plugin tool installation or policy version is stale")
		}
		if !s.requiredConfigSet(record) {
			return fmt.Errorf("plugins: coordinator plugin tool installation is not configured")
		}
		return nil
	})
}

func validateCoordinatorInvocation(
	authorization *plugintools.CoordinatorToolAuthorization,
	invocation AgentToolInvocationContext,
	pluginID, localName string,
) error {
	if invocation.Surface != manifest.AgentToolSurfaceCoordinator || authorization == nil ||
		authorization.Validate() != nil ||
		authorization.WorkspaceID != invocation.WorkspaceID ||
		authorization.ConversationTaskID != invocation.TaskID ||
		authorization.PluginID != pluginID || authorization.LocalName != localName ||
		invocation.TaskID == "" || invocation.SessionID == "" || invocation.WorkspaceID == "" ||
		invocation.InvocationID == "" {
		return fmt.Errorf("plugins: coordinator agent tool authorization denied the invocation")
	}
	return nil
}

func coordinatorID(authorization *plugintools.CoordinatorToolAuthorization) string {
	if authorization == nil {
		return ""
	}
	return authorization.CoordinatorID
}

// invokeDeclaredAgentTool owns the shared dispatch lease and bounded call for
// the ordinary, automation, and coordinator surfaces. The caller supplies the
// surface-specific record checks; argument validation, remote dispatch, and
// result validation stay in one place.
func (s *Service) invokeDeclaredAgentTool(
	ctx context.Context,
	pluginID, localName string,
	arguments map[string]any,
	invocation AgentToolInvocationContext,
	surface string,
	validateRecord func(*store.Record) error,
) (*pluginsdk.AgentToolResult, error) {
	dispatchLock := s.dispatchLocks.lockFor(pluginID)
	release, err := dispatchLock.RLockContext(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	record, err := s.Get(pluginID)
	if err != nil {
		return nil, err
	}
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	declaration, err := findAgentTool(record.AgentTools, localName, surface)
	if err != nil {
		return nil, err
	}
	if err := validateDispatchSurfaceDeclaration(*declaration, invocation); err != nil {
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
	result, err := invokeRemoteAgentTool(ctx, remote, localName, arguments, invocation)
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
		outcome := agentToolOutcomeSuccess
		if err != nil {
			outcome = agentToolOutcomeError
		} else if result != nil && result.IsError {
			outcome = agentToolOutcomeToolError
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

// validateDispatchSurfaceDeclaration re-checks the eligibility that manifest
// validation promises for context-bound surfaces at dispatch time. The caller
// owns the workspace authorization decision; this gate only requires the
// invocation context to match the descriptor scope.
func validateDispatchSurfaceDeclaration(tool manifest.AgentTool, invocation AgentToolInvocationContext) error {
	switch invocation.Surface {
	case manifest.AgentToolSurfaceCoordinator:
		return validateCoordinatorDispatchDeclaration(tool)
	case manifest.AgentToolSurfaceExternal:
		return validateExternalDispatchDeclaration(tool, invocation.WorkspaceID)
	default:
		return nil
	}
}

func validateCoordinatorDispatchDeclaration(tool manifest.AgentTool) error {
	if tool.NormalizedInvocationScope() != manifest.AgentToolInvocationScopeInstance {
		return fmt.Errorf("plugins: agent tool %q requires instance invocation scope on surface %q",
			tool.Name, manifest.AgentToolSurfaceCoordinator)
	}
	if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
		return fmt.Errorf("plugins: agent tool %q requires a true read-only hint on surface %q",
			tool.Name, manifest.AgentToolSurfaceCoordinator)
	}
	return nil
}

func validateExternalDispatchDeclaration(tool manifest.AgentTool, workspaceID string) error {
	switch tool.NormalizedInvocationScope() {
	case manifest.AgentToolInvocationScopeInstance:
		if workspaceID != "" {
			return fmt.Errorf("plugins: agent tool %q does not accept a workspace on surface %q",
				tool.Name, manifest.AgentToolSurfaceExternal)
		}
	case manifest.AgentToolInvocationScopeWorkspace:
		if workspaceID == "" {
			return fmt.Errorf("plugins: agent tool %q requires a verified workspace on surface %q",
				tool.Name, manifest.AgentToolSurfaceExternal)
		}
	default:
		return fmt.Errorf("plugins: agent tool %q requires instance or workspace invocation scope on surface %q",
			tool.Name, manifest.AgentToolSurfaceExternal)
	}
	if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
		return fmt.Errorf("plugins: agent tool %q requires a true read-only hint on surface %q",
			tool.Name, manifest.AgentToolSurfaceExternal)
	}
	return nil
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
	if authorization := invocation.AutomationToolAuthorization; authorization != nil {
		toolContext.InstallationID = authorization.InstallationID
		toolContext.AgentToolNames = append([]string(nil), authorization.ToolNames...)
	}
	if authorization := invocation.CoordinatorToolAuthorization; authorization != nil {
		toolContext.InstallationID = authorization.InstallationID
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

func supportedAgentToolSurface(surface string) bool {
	switch surface {
	case manifest.AgentToolSurfaceKanban, manifest.AgentToolSurfaceOffice,
		manifest.AgentToolSurfaceManaged, manifest.AgentToolSurfaceAutomation,
		manifest.AgentToolSurfaceCoordinator, manifest.AgentToolSurfaceExternal:
		return true
	default:
		return false
	}
}
