package automation

import (
	"context"
	"errors"
	"slices"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var errMCPOptionsTooManyRepositories = errors.New("too many repository IDs (maximum 100)")

type MCPDefinitionOption struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}
type MCPInheritedOption struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Origins []string `json:"origins"`
}
type MCPOptions struct {
	TaskDefaults []p.PackageID                `json:"task_defaults"`
	TaskBlocked  []p.PackageID                `json:"task_blocked"`
	Inherited    []MCPInheritedOption         `json:"inherited"`
	Packages     []p.BuiltinPackageDescriptor `json:"packages"`
	Definitions  []MCPDefinitionOption        `json:"definitions"`
}

// MCPOptions returns workspace definitions and normal-task package policy after workspace
// authorization.
func (s *Service) MCPOptions(ctx context.Context, workspaceID string) (*MCPOptions, error) {
	if err := s.authorizeWs(ctx, workspaceID); err != nil {
		return nil, err
	}
	result := &MCPOptions{Packages: p.BuiltinPackageCatalog(), Definitions: []MCPDefinitionOption{}}
	if s.mcpDefinitions == nil {
		return nil, errors.New("MCP catalog unavailable")
	}
	definitions, err := s.mcpDefinitions.ListMCPServerDefinitions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, d := range definitions {
		result.Definitions = append(result.Definitions, MCPDefinitionOption{ID: d.ID, Name: d.DisplayName, Enabled: d.Enabled})
	}
	result.Inherited = []MCPInheritedOption{}
	policy := mcpconfig.DefaultBuiltinWorkspacePolicy(workspaceID)
	if s.mcpBuiltinPolicy != nil {
		snapshot, err := s.mcpBuiltinPolicy.GetBuiltinPolicySnapshot(ctx, workspaceID, "")
		if err != nil {
			return nil, err
		}
		if snapshot != nil && snapshot.Policy != nil {
			policy = snapshot.Policy
		}
	}
	result.TaskDefaults = []p.PackageID{p.PackageCore}
	result.TaskBlocked = []p.PackageID{}
	for _, d := range result.Packages {
		if policy.Packages[d.ID] == mcpconfig.BuiltinPolicyBlocked {
			result.TaskBlocked = append(result.TaskBlocked, d.ID)
		}
		if d.ID != p.PackageCore && policy.Packages[d.ID] == mcpconfig.BuiltinPolicyDefault {
			result.TaskDefaults = append(result.TaskDefaults, d.ID)
		}
	}
	return result, nil
}

// wsMCPOptions exposes validated inheritance choices without returning internal lookup errors.
func wsMCPOptions(svc *Service) ws.HandlerFunc {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var request struct {
			WorkspaceID   string   `json:"workspace_id"`
			ProfileID     string   `json:"agent_profile_id"`
			RepositoryIDs []string `json:"repository_ids"`
		}
		if err := msg.ParsePayload(&request); err != nil || request.WorkspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspace_id required", nil)
		}
		options, err := svc.mcpOptionsWithInheritance(ctx, request.WorkspaceID, request.ProfileID, request.RepositoryIDs)
		if err != nil {
			code := ws.ErrorCodeInternalError
			if errors.Is(err, ErrRepositoryNotInWorkspace) || errors.Is(err, errMCPOptionsTooManyRepositories) {
				code = ws.ErrorCodeBadRequest
			}
			if errors.Is(err, ErrAgentProfileNotFound) {
				code = ws.ErrorCodeNotFound
			}
			return ws.NewError(msg.ID, msg.Action, code, "Unable to load automation MCP options", nil)
		}
		return ws.NewResponse(msg.ID, msg.Action, options)
	}
}

// mcpOptionsWithInheritance validates selected owners before adding their inherited definition
// origins.
func (s *Service) mcpOptionsWithInheritance(ctx context.Context, workspaceID, profileID string, repositories []string) (*MCPOptions, error) {
	if len(repositories) > 100 {
		return nil, errMCPOptionsTooManyRepositories
	}
	options, err := s.MCPOptions(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if err := s.validateAgentProfileID(ctx, profileID); err != nil {
		return nil, err
	}
	reader, ok := s.mcpDefinitions.(interface {
		ListMCPSelections(context.Context, mcpconfig.SelectionScope, string, string) ([]string, error)
	})
	if !ok {
		return options, nil
	}
	origins := make(map[string][]string)
	add := func(scope mcpconfig.SelectionScope, id string) error {
		if id == "" {
			return nil
		}
		ids, err := reader.ListMCPSelections(ctx, scope, workspaceID, id)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !slices.Contains(origins[id], string(scope)) {
				origins[id] = append(origins[id], string(scope))
			}
		}
		return nil
	}
	if err := add(mcpconfig.SelectionScopeProfile, profileID); err != nil {
		return nil, err
	}
	for _, id := range repositories {
		if err := s.validateMCPOptionRepository(ctx, workspaceID, id); err != nil {
			return nil, err
		}
		if err := add(mcpconfig.SelectionScopeRepository, id); err != nil {
			return nil, err
		}
	}
	for _, d := range options.Definitions {
		if sources := origins[d.ID]; len(sources) > 0 {
			options.Inherited = append(options.Inherited, MCPInheritedOption{ID: d.ID, Name: d.Name, Origins: sources})
		}
	}
	return options, nil
}

// validateMCPOptionRepository requires an available repository owned by the selected workspace.
func (s *Service) validateMCPOptionRepository(ctx context.Context, workspaceID, id string) error {
	if s.repoLookup == nil {
		return ErrRepositoryLookupUnavailable
	}
	owner, _, ok := s.repoLookup.GetRepository(ctx, id)
	if !ok || owner != workspaceID {
		return ErrRepositoryNotInWorkspace
	}
	return nil
}
