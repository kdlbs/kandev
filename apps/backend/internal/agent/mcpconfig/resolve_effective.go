package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrMCPEffectiveNameCollision = errors.New("effective MCP runtime name collision")
	ErrMCPSecretResolution       = errors.New("MCP secret resolution failed")
)

// SelectionOrigin identifies the scope that contributed one definition to an
// effective set. It is safe to expose because it contains no configuration or
// secret value.
type SelectionOrigin struct {
	Scope       SelectionScope `json:"scope"`
	WorkspaceID string         `json:"workspace_id"`
	OwnerID     string         `json:"owner_id"`
}

// ResolutionContext is the task-owned identity needed to compose MCP
// selections. RepositoryIDs are ordered only for reproducible reads; the
// effective set itself has no scope precedence.
type ResolutionContext struct {
	WorkspaceID   string
	RepositoryIDs []string
	ProfileID     string
	TaskID        string
	SessionID     string
}

// MCPResolutionDecision explains why a selected definition was not delivered.
// The decision is bounded and never includes resolved configuration values.
type MCPResolutionDecision struct {
	DefinitionID       string            `json:"definition_id,omitempty"`
	DefinitionRevision int64             `json:"definition_revision,omitempty"`
	RuntimeName        string            `json:"runtime_name,omitempty"`
	ReasonCode         string            `json:"reason_code"`
	Summary            string            `json:"summary,omitempty"`
	Origins            []SelectionOrigin `json:"origins,omitempty"`
}

// EffectiveMCPResolution contains the delivery servers and safe filtered
// decisions. Resolved secrets exist only in Servers.
type EffectiveMCPResolution struct {
	Servers   []ResolvedServer        `json:"-"`
	Decisions []MCPResolutionDecision `json:"decisions,omitempty"`
}

// MCPSecretResolver reveals one secret at the last responsible moment.
// Implementations must enforce workspace visibility before returning a value.
type MCPSecretResolver func(context.Context, string, string) (string, error)

// LegacyImportStateReader lets the resolver decide whether the compatibility
// profile JSON fallback is still required for one workspace/profile pair.
type LegacyImportStateReader interface {
	GetMCPImportState(context.Context, string, string) (LegacyImportState, error)
}

// Resolver composes catalog definitions selected by all supported scopes.
// Selection order is deterministic, but no scope overrides another scope.
type Resolver struct {
	catalog       CatalogRepository
	selections    SelectionRepository
	legacy        LegacyMCPConfigReader
	legacyStates  LegacyImportStateReader
	secretResolve MCPSecretResolver
}

func NewResolver(catalog CatalogRepository, selections SelectionRepository) *Resolver {
	return &Resolver{catalog: catalog, selections: selections}
}

func (r *Resolver) SetLegacyProvider(reader LegacyMCPConfigReader, states LegacyImportStateReader) {
	r.legacy = reader
	r.legacyStates = states
}

func (r *Resolver) SetSecretResolver(resolver MCPSecretResolver) {
	r.secretResolve = resolver
}

// Resolve builds the effective set and applies the existing executor/provider
// transport policy. It does not mutate selections or execute a server.
func (r *Resolver) Resolve(ctx context.Context, resolutionContext ResolutionContext, policy Policy) (*EffectiveMCPResolution, error) {
	if r == nil || r.catalog == nil || r.selections == nil {
		return &EffectiveMCPResolution{}, nil
	}
	if strings.TrimSpace(resolutionContext.WorkspaceID) == "" {
		return nil, fmt.Errorf("%w: workspace id is required", ErrMCPInvalidSelection)
	}
	origins, legacyServers, err := r.loadOrigins(ctx, resolutionContext)
	if err != nil {
		return nil, err
	}
	r.addLegacyFallback(ctx, resolutionContext, origins, legacyServers)
	ids := make([]string, 0, len(origins))
	for id := range origins {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := &EffectiveMCPResolution{}
	names := make(map[string]string, len(ids))
	for _, id := range ids {
		resolved, decision, err := r.resolveDefinition(ctx, resolutionContext, policy, id, origins[id], legacyServers, names)
		if err != nil {
			return nil, err
		}
		if decision != nil {
			result.Decisions = append(result.Decisions, *decision)
			continue
		}
		result.Servers = append(result.Servers, *resolved)
	}
	sort.SliceStable(result.Servers, func(i, j int) bool {
		left := strings.ToLower(result.Servers[i].Name)
		right := strings.ToLower(result.Servers[j].Name)
		if left == right {
			return result.Servers[i].DefinitionID < result.Servers[j].DefinitionID
		}
		return left < right
	})
	return result, nil
}

func (r *Resolver) resolveDefinition(
	ctx context.Context,
	resolutionContext ResolutionContext,
	policy Policy,
	id string,
	origins []SelectionOrigin,
	legacyServers map[string]ServerDef,
	names map[string]string,
) (*ResolvedServer, *MCPResolutionDecision, error) {
	definition, getErr := r.catalog.GetMCPServerDefinition(ctx, resolutionContext.WorkspaceID, id)
	if legacyServer, ok := legacyServers[id]; ok {
		definition = legacyDefinition(id, resolutionContext, legacyServer)
		getErr = nil
	}
	if errors.Is(getErr, ErrMCPServerDefinitionNotFound) || definition == nil {
		return nil, &MCPResolutionDecision{
			DefinitionID: id, ReasonCode: "definition_missing",
			Summary: "MCP definition is no longer available", Origins: origins,
		}, nil
	}
	if getErr != nil {
		return nil, nil, getErr
	}
	if !definition.Enabled {
		return nil, decisionPointer(definitionDecision(definition, "definition_disabled", "MCP definition is disabled", origins)), nil
	}
	server, secretErr := r.deliveryServer(ctx, resolutionContext.WorkspaceID, definition)
	if secretErr != nil {
		return nil, decisionPointer(definitionDecision(definition, "secret_resolution_failed", "MCP secret could not be resolved", origins)), nil
	}
	resolved, warnings, resolveErr := resolveServer(definition.RuntimeName, server, policy)
	if resolveErr != nil {
		return nil, nil, resolveErr
	}
	if resolved == nil {
		return nil, decisionPointer(definitionDecision(definition, resolutionWarningCode(warnings), firstWarning(warnings), origins)), nil
	}
	resolved.DefinitionID = definition.ID
	resolved.DefinitionRevision = definition.Revision
	resolved.Origins = cloneOrigins(origins)
	if previous, exists := names[definition.NormalizedRuntimeName]; exists && previous != definition.ID {
		return nil, nil, fmt.Errorf("%w: %s", ErrMCPEffectiveNameCollision, definition.RuntimeName)
	}
	names[definition.NormalizedRuntimeName] = definition.ID
	return resolved, nil, nil
}

func decisionPointer(decision MCPResolutionDecision) *MCPResolutionDecision {
	return &decision
}

// ResolveEffective is a descriptive alias for callers outside mcpconfig.
func (r *Resolver) ResolveEffective(ctx context.Context, resolutionContext ResolutionContext, policy Policy) (*EffectiveMCPResolution, error) {
	return r.Resolve(ctx, resolutionContext, policy)
}

func (r *Resolver) loadOrigins(ctx context.Context, rc ResolutionContext) (map[string][]SelectionOrigin, map[string]ServerDef, error) {
	origins := make(map[string][]SelectionOrigin)
	legacyServers := make(map[string]ServerDef)
	repositories := uniqueSorted(rc.RepositoryIDs)
	for _, repositoryID := range repositories {
		ids, err := r.selections.ListMCPSelections(ctx, SelectionScopeRepository, rc.WorkspaceID, repositoryID)
		if err != nil {
			return nil, nil, err
		}
		addOrigins(origins, ids, SelectionOrigin{Scope: SelectionScopeRepository, WorkspaceID: rc.WorkspaceID, OwnerID: repositoryID})
	}
	if rc.ProfileID != "" {
		ids, err := r.selections.ListMCPSelections(ctx, SelectionScopeProfile, rc.WorkspaceID, rc.ProfileID)
		if err != nil {
			return nil, nil, err
		}
		addOrigins(origins, ids, SelectionOrigin{Scope: SelectionScopeProfile, WorkspaceID: rc.WorkspaceID, OwnerID: rc.ProfileID})
	}
	if rc.TaskID != "" {
		ids, err := r.selections.ListMCPSelections(ctx, SelectionScopeTask, rc.WorkspaceID, rc.TaskID)
		if err != nil {
			return nil, nil, err
		}
		addOrigins(origins, ids, SelectionOrigin{Scope: SelectionScopeTask, WorkspaceID: rc.WorkspaceID, OwnerID: rc.TaskID})
	}
	if rc.SessionID != "" {
		ids, err := r.selections.ListMCPSelections(ctx, SelectionScopeTaskSession, rc.WorkspaceID, rc.SessionID)
		if err != nil {
			return nil, nil, err
		}
		addOrigins(origins, ids, SelectionOrigin{Scope: SelectionScopeTaskSession, WorkspaceID: rc.WorkspaceID, OwnerID: rc.SessionID})
	}
	return origins, legacyServers, nil
}

func (r *Resolver) addLegacyFallback(ctx context.Context, rc ResolutionContext, origins map[string][]SelectionOrigin, legacyServers map[string]ServerDef) {
	if r.legacy == nil || rc.ProfileID == "" || r.legacyImportComplete(ctx, rc.WorkspaceID, rc.ProfileID) {
		return
	}
	config, err := r.legacy.GetConfigByProfileID(ctx, rc.ProfileID)
	if err != nil || config == nil || !config.Enabled {
		return
	}
	selectedNames := r.selectedRuntimeNames(ctx, rc.WorkspaceID, origins)
	profileOrigin := SelectionOrigin{Scope: SelectionScopeProfile, WorkspaceID: rc.WorkspaceID, OwnerID: rc.ProfileID}
	for name, definition := range config.Servers {
		if _, selected := selectedNames[normalizeRuntimeName(legacyRuntimeName(name))]; selected {
			continue
		}
		id := "legacy:" + rc.WorkspaceID + ":" + rc.ProfileID + ":" + name
		if existing, selected := origins[id]; selected {
			origins[id] = appendOrigin(existing, profileOrigin)
			continue
		}
		origins[id] = []SelectionOrigin{{Scope: SelectionScopeProfile, WorkspaceID: rc.WorkspaceID, OwnerID: rc.ProfileID}}
		// Store a synthetic definition only in the resolver's transient map. The
		// delivery path below recognizes it through legacyServers instead of
		// exposing raw legacy values to the catalog.
		legacyServers[id] = definition
	}
}

func (r *Resolver) selectedRuntimeNames(
	ctx context.Context,
	workspaceID string,
	origins map[string][]SelectionOrigin,
) map[string]struct{} {
	names := make(map[string]struct{}, len(origins))
	for definitionID := range origins {
		definition, err := r.catalog.GetMCPServerDefinition(ctx, workspaceID, definitionID)
		if err == nil && definition != nil {
			names[definition.NormalizedRuntimeName] = struct{}{}
		}
	}
	return names
}

func (r *Resolver) legacyImportComplete(ctx context.Context, workspaceID, profileID string) bool {
	if r.legacyStates == nil {
		return false
	}
	state, err := r.legacyStates.GetMCPImportState(ctx, workspaceID, profileID)
	return err == nil && state.Status == LegacyImportStatusComplete
}

func (r *Resolver) deliveryServer(ctx context.Context, workspaceID string, definition *MCPServerDefinition) (ServerDef, error) {
	server := ServerDef{
		Type: definition.Transport, Command: definition.Configuration.Command,
		Args: append([]string(nil), definition.Configuration.Args...),
		Env:  cloneStringMap(definition.Configuration.Env),
		URL:  definition.Configuration.URL, Headers: cloneStringMap(definition.Configuration.Headers),
	}
	if definition.ExecutionMode == ExecutionModeManagedPackage {
		command, args, err := ManagedPackageCommand(definition.Configuration)
		if err != nil {
			return ServerDef{}, err
		}
		server.Command = command
		server.Args = append(args, server.Args...)
	}
	for _, binding := range definition.SecretBindings {
		if r.secretResolve == nil {
			return ServerDef{}, ErrMCPSecretResolution
		}
		value, err := r.secretResolve(ctx, binding.SecretID, workspaceID)
		if err != nil {
			return ServerDef{}, fmt.Errorf("%w: %w", ErrMCPSecretResolution, err)
		}
		if definition.Transport == ServerTypeStdio {
			if server.Env == nil {
				server.Env = make(map[string]string)
			}
			server.Env[binding.InputName] = value
		} else {
			if server.Headers == nil {
				server.Headers = make(map[string]string)
			}
			server.Headers[binding.InputName] = value
		}
	}
	return server, nil
}

func legacyDefinition(id string, rc ResolutionContext, server ServerDef) *MCPServerDefinition {
	name := strings.TrimSpace(id)
	if index := strings.LastIndex(name, ":"); index >= 0 {
		name = name[index+1:]
	}
	transport := normalizeServerType(server)
	return &MCPServerDefinition{
		ID: id, WorkspaceID: rc.WorkspaceID, RuntimeName: name,
		NormalizedRuntimeName: strings.ToLower(name), DisplayName: name,
		Enabled: true, ExecutionMode: executionModeForLegacy(server),
		Transport: transport, Configuration: MCPServerConfiguration{
			Command: server.Command, Args: server.Args, Env: server.Env,
			URL: server.URL, Headers: server.Headers,
		}, Revision: 1,
	}
}

func executionModeForLegacy(server ServerDef) ExecutionMode {
	if normalizeServerType(server) == ServerTypeStdio {
		return ExecutionModeExistingExecutable
	}
	return ExecutionModeRemote
}

func definitionDecision(definition *MCPServerDefinition, code, summary string, origins []SelectionOrigin) MCPResolutionDecision {
	return MCPResolutionDecision{DefinitionID: definition.ID, DefinitionRevision: definition.Revision, RuntimeName: definition.RuntimeName, ReasonCode: code, Summary: summary, Origins: cloneOrigins(origins)}
}

func resolutionWarningCode(warnings []string) string {
	if len(warnings) == 0 {
		return "filtered"
	}
	if strings.Contains(warnings[0], "transport") {
		return "transport_policy"
	}
	if strings.Contains(warnings[0], "allow") {
		return "server_policy"
	}
	return "invalid_transport"
}

func firstWarning(warnings []string) string {
	if len(warnings) == 0 {
		return "MCP definition was filtered by runtime policy"
	}
	return warnings[0]
}

func addOrigins(target map[string][]SelectionOrigin, ids []string, origin SelectionOrigin) {
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		target[id] = appendOrigin(target[id], origin)
	}
}

func appendOrigin(origins []SelectionOrigin, origin SelectionOrigin) []SelectionOrigin {
	for _, existing := range origins {
		if existing == origin {
			return origins
		}
	}
	return append(origins, origin)
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func cloneOrigins(origins []SelectionOrigin) []SelectionOrigin {
	return append([]SelectionOrigin(nil), origins...)
}
