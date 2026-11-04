package mcpconfig

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrMCPUsageUnavailable = errors.New("MCP usage unavailable")
	ErrMCPUsageCursor      = errors.New("invalid MCP usage cursor")
)

const (
	UsageOwnerAvailable = "available"
	UsageOwnerDisabled  = "disabled"
	UsageOwnerArchived  = "archived"
)

type UsageReference struct {
	Scope   SelectionScope `json:"scope"`
	OwnerID string         `json:"owner_id"`
}

type CatalogUsageRepository interface {
	ListMCPUsageCandidates(context.Context, string, string, UsageReference, int) ([]UsageReference, error)
}

// UsageOwner contains only authorized display and navigation metadata.
type UsageOwner struct {
	UsageReference
	WorkspaceID string            `json:"workspace_id"`
	DisplayName string            `json:"display_name,omitempty"`
	AgentID     string            `json:"agent_id,omitempty"`
	TaskID      string            `json:"task_id,omitempty"`
	OwnerState  string            `json:"owner_state"`
	Destination *UsageDestination `json:"destination"`
}

type UsageDestination struct {
	Kind        SelectionScope `json:"kind"`
	WorkspaceID string         `json:"workspace_id"`
	OwnerID     string         `json:"owner_id"`
	AgentID     string         `json:"agent_id,omitempty"`
	TaskID      string         `json:"task_id,omitempty"`
}

type CatalogUsagePage struct {
	WorkspaceID  string       `json:"workspace_id"`
	DefinitionID string       `json:"definition_id"`
	Items        []UsageOwner `json:"items"`
	NextCursor   string       `json:"next_cursor,omitempty"`
}

// UsageOwnerReader returns nil for a missing or inaccessible owner, and an error
// for a failed read. Implementations must not mutate or materialize owner data.
type UsageOwnerReader func(context.Context, string, UsageReference) (*UsageOwner, error)

func (s *SelectionService) SetUsageOwnerReader(reader UsageOwnerReader) { s.usageOwnerReader = reader }

const usageCandidateBudget = 200

func (s *SelectionService) CatalogUsage(ctx context.Context, workspaceID, definitionID, cursor string, limit int) (CatalogUsagePage, error) {
	page := CatalogUsagePage{WorkspaceID: workspaceID, DefinitionID: definitionID, Items: []UsageOwner{}}
	repo, err := s.authorizedUsageRepository(ctx, workspaceID, definitionID)
	if err != nil {
		return page, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return page, ErrMCPInvalidSelection
	}
	after, err := s.usageCursor.decode(cursor, workspaceID, definitionID)
	if err != nil {
		return page, err
	}
	refs, err := repo.ListMCPUsageCandidates(ctx, workspaceID, definitionID, after, usageCandidateBudget+1)
	if err != nil {
		return page, err
	}
	for i, ref := range refs {
		if i == usageCandidateBudget || len(page.Items) == limit {
			page.NextCursor, err = s.usageCursor.encode(after, workspaceID, definitionID)
			return page, err
		}
		owner, readErr := s.usageOwnerReader(ctx, workspaceID, ref)
		if readErr != nil {
			return CatalogUsagePage{}, readErr
		}
		after = ref
		if owner == nil || owner.WorkspaceID != workspaceID || owner.UsageReference != ref {
			continue
		}
		page.Items = append(page.Items, usageOwnerWithDestination(*owner))
	}
	return page, nil
}

func (s *SelectionService) authorizedUsageRepository(ctx context.Context, workspaceID, definitionID string) (CatalogUsageRepository, error) {
	repo, ok := s.repo.(CatalogUsageRepository)
	if !ok || s.catalog == nil || s.authorizer == nil || s.usageOwnerReader == nil {
		return nil, ErrMCPUsageUnavailable
	}
	if strings.TrimSpace(workspaceID) == "" || s.authorizer(ctx, workspaceID) != nil {
		return nil, ErrMCPWorkspaceAccess
	}
	definition, err := s.catalog.GetMCPServerDefinition(ctx, workspaceID, definitionID)
	if err != nil {
		return nil, err
	}
	if definition == nil || definition.WorkspaceID != workspaceID {
		return nil, ErrMCPServerDefinitionNotFound
	}
	return repo, nil
}

func usageOwnerWithDestination(owner UsageOwner) UsageOwner {
	owner.Destination = nil
	if owner.OwnerState == "" {
		owner.OwnerState = UsageOwnerAvailable
	}
	dest := &UsageDestination{Kind: owner.Scope, WorkspaceID: owner.WorkspaceID, OwnerID: owner.OwnerID}
	switch owner.Scope {
	case SelectionScopeProfile:
		if owner.AgentID == "" {
			return owner
		}
		dest.AgentID = owner.AgentID
	case SelectionScopeTaskSession:
		if owner.TaskID == "" {
			return owner
		}
		dest.TaskID = owner.TaskID
	case SelectionScopeRepository, SelectionScopeTask:
	default:
		return owner
	}
	owner.Destination = dest
	return owner
}
