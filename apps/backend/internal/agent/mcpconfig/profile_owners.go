package mcpconfig

import "context"

// MCPProfileOwner is navigation metadata, not a profile configuration snapshot.
type MCPProfileOwner struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	AgentName   string `json:"agent_name"`
	Name        string `json:"name"`
	WorkspaceID string `json:"workspace_id"`
	Kind        string `json:"kind"`
	Enabled     bool   `json:"enabled"`
	SupportsMCP bool   `json:"supports_mcp"`
}

type MCPProfileOwnerPage struct {
	WorkspaceID string            `json:"workspace_id"`
	Items       []MCPProfileOwner `json:"items"`
	NextCursor  string            `json:"next_cursor,omitempty"`
}

type MCPProfileOwnerRepository interface {
	ListMCPProfileOwners(context.Context, string, string, int) ([]MCPProfileOwner, error)
	GetMCPProfileOwner(context.Context, string, string) (*MCPProfileOwner, error)
}

func (s *SelectionService) profileOwnerRepository(ctx context.Context, workspaceID string) (MCPProfileOwnerRepository, error) {
	repo, ok := s.repo.(MCPProfileOwnerRepository)
	if !ok || s.authorizer == nil {
		return nil, ErrMCPUsageUnavailable
	}
	if workspaceID == "" || s.authorizer(ctx, workspaceID) != nil {
		return nil, ErrMCPWorkspaceAccess
	}
	return repo, nil
}

func (s *SelectionService) ListProfileOwners(ctx context.Context, workspaceID, cursor string, limit int) (MCPProfileOwnerPage, error) {
	page := MCPProfileOwnerPage{WorkspaceID: workspaceID, Items: []MCPProfileOwner{}}
	repo, err := s.profileOwnerRepository(ctx, workspaceID)
	if err != nil {
		return page, err
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return page, ErrMCPInvalidSelection
	}
	rows, err := repo.ListMCPProfileOwners(ctx, workspaceID, cursor, limit+1)
	if err != nil {
		return page, err
	}
	if len(rows) > limit {
		page.NextCursor = rows[limit-1].ID
		rows = rows[:limit]
	}
	page.Items = append(page.Items, rows...)
	return page, nil
}

func (s *SelectionService) GetProfileOwner(ctx context.Context, workspaceID, profileID string) (*MCPProfileOwner, error) {
	repo, err := s.profileOwnerRepository(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if profileID == "" {
		return nil, ErrMCPInvalidSelection
	}
	return repo.GetMCPProfileOwner(ctx, workspaceID, profileID)
}
