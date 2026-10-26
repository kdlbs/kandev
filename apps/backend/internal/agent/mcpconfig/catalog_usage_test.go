package mcpconfig

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

type usageRepositoryFake struct {
	*selectionRepositoryFake
	refs  []UsageReference
	err   error
	reads int
}

func (r *usageRepositoryFake) ListMCPUsageCandidates(_ context.Context, _, _ string, after UsageReference, limit int) ([]UsageReference, error) {
	r.reads++
	if r.err != nil {
		return nil, r.err
	}
	out := []UsageReference{}
	for _, ref := range r.refs {
		if ref.Scope > after.Scope || (ref.Scope == after.Scope && ref.OwnerID > after.OwnerID) {
			out = append(out, ref)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
func newUsageTestService() (*SelectionService, *usageRepositoryFake) {
	repo := &usageRepositoryFake{selectionRepositoryFake: newSelectionRepositoryFake()}
	catalog := newCatalogRepositoryFake()
	catalog.definitions["definition"] = &MCPServerDefinition{ID: "definition", WorkspaceID: "workspace"}
	svc := NewSelectionService(repo, catalog)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	svc.SetUsageOwnerReader(func(_ context.Context, w string, ref UsageReference) (*UsageOwner, error) {
		return &UsageOwner{UsageReference: ref, WorkspaceID: w, DisplayName: "Named owner", AgentID: "agent"}, nil
	})
	return svc, repo
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.1, AC-AGENTS-MCP-MANAGEMENT-001.3
func TestCatalogUsageAuthorizedOwners(t *testing.T) {
	svc, repo := newUsageTestService()
	repo.refs = []UsageReference{{SelectionScopeProfile, "profile"}, {SelectionScopeTask, "hidden"}, {SelectionScopeTask, "visible"}}
	svc.SetUsageOwnerReader(func(_ context.Context, w string, ref UsageReference) (*UsageOwner, error) {
		if ref.OwnerID == "hidden" {
			return nil, nil
		}
		return &UsageOwner{UsageReference: ref, WorkspaceID: w, DisplayName: "Named owner", AgentID: "agent", OwnerState: "disabled"}, nil
	})
	page, err := svc.CatalogUsage(context.Background(), "workspace", "definition", "", 50)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	require.Equal(t, "profile", page.Items[0].OwnerID)
	require.Equal(t, "visible", page.Items[1].OwnerID)
	require.Equal(t, "agent", page.Items[0].Destination.AgentID)
	require.Equal(t, "disabled", page.Items[0].OwnerState)
	require.Empty(t, page.NextCursor)
	require.Zero(t, repo.replaceCalls)
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.3, AC-AGENTS-MCP-MANAGEMENT-001.4
func TestCatalogUsagePaginationAndErrors(t *testing.T) {
	svc, repo := newUsageTestService()
	for i := 0; i < 202; i++ {
		repo.refs = append(repo.refs, UsageReference{SelectionScopeTask, fmt.Sprintf("owner-%03d", i)})
	}
	svc.SetUsageOwnerReader(func(_ context.Context, w string, ref UsageReference) (*UsageOwner, error) {
		if ref.OwnerID < "owner-200" {
			return nil, nil
		}
		return &UsageOwner{UsageReference: ref, WorkspaceID: w}, nil
	})
	page, err := svc.CatalogUsage(context.Background(), "workspace", "definition", "", 1)
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.NotEmpty(t, page.NextCursor)
	next, err := svc.CatalogUsage(context.Background(), "workspace", "definition", page.NextCursor, 1)
	require.NoError(t, err)
	require.Equal(t, "owner-200", next.Items[0].OwnerID)
	require.NotEmpty(t, next.NextCursor)
	last, err := svc.CatalogUsage(context.Background(), "workspace", "definition", next.NextCursor, 1)
	require.NoError(t, err)
	require.Equal(t, "owner-201", last.Items[0].OwnerID)
	require.Empty(t, last.NextCursor)
	_, err = svc.CatalogUsage(context.Background(), "workspace", "definition", "tampered"+page.NextCursor, 1)
	require.ErrorIs(t, err, ErrMCPUsageCursor)
	repo.err = errors.New("storage unavailable")
	_, err = svc.CatalogUsage(context.Background(), "workspace", "definition", "", 1)
	require.Error(t, err)
}

func TestCatalogUsageAuthorizationBeforeEnumeration(t *testing.T) {
	svc, repo := newUsageTestService()
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return errors.New("denied") })
	_, err := svc.CatalogUsage(context.Background(), "workspace", "definition", "", 50)
	require.ErrorIs(t, err, ErrMCPWorkspaceAccess)
	require.Zero(t, repo.reads)
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	_, err = svc.CatalogUsage(context.Background(), "other", "definition", "", 50)
	require.ErrorIs(t, err, ErrMCPServerDefinitionNotFound)
	require.Zero(t, repo.reads)
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.3, AC-AGENTS-MCP-MANAGEMENT-001.4
func TestCatalogUsageRejectsStaleOwnerAndFailedRead(t *testing.T) {
	svc, repo := newUsageTestService()
	repo.refs = []UsageReference{{SelectionScopeProfile, "profile"}, {SelectionScopeTask, "task"}}
	svc.SetUsageOwnerReader(func(_ context.Context, w string, ref UsageReference) (*UsageOwner, error) {
		if ref.Scope == SelectionScopeProfile {
			return &UsageOwner{UsageReference: ref, WorkspaceID: w, DisplayName: "Unnamed destination"}, nil
		}
		return &UsageOwner{UsageReference: ref, WorkspaceID: "moved-workspace", DisplayName: "Private name"}, nil
	})
	page, err := svc.CatalogUsage(context.Background(), "workspace", "definition", "", 50)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Nil(t, page.Items[0].Destination)
	failure := errors.New("owner read failed")
	svc.SetUsageOwnerReader(func(_ context.Context, w string, ref UsageReference) (*UsageOwner, error) {
		if ref.Scope == SelectionScopeTask {
			return nil, failure
		}
		return &UsageOwner{UsageReference: ref, WorkspaceID: w}, nil
	})
	page, err = svc.CatalogUsage(context.Background(), "workspace", "definition", "", 50)
	require.ErrorIs(t, err, failure)
	require.Empty(t, page.Items)
}

func TestCatalogUsageCursorBoundToDefinition(t *testing.T) {
	svc, repo := newUsageTestService()
	repo.refs = []UsageReference{{SelectionScopeTask, "a"}, {SelectionScopeTask, "b"}}
	page, err := svc.CatalogUsage(context.Background(), "workspace", "definition", "", 1)
	require.NoError(t, err)
	require.NotEmpty(t, page.NextCursor)
	catalog := svc.catalog.(*catalogRepositoryFake)
	catalog.definitions["other"] = &MCPServerDefinition{ID: "other", WorkspaceID: "workspace"}
	_, err = svc.CatalogUsage(context.Background(), "workspace", "other", page.NextCursor, 1)
	require.ErrorIs(t, err, ErrMCPUsageCursor)
	fresh, _ := newUsageTestService()
	_, err = fresh.CatalogUsage(context.Background(), "workspace", "definition", page.NextCursor, 1)
	require.ErrorIs(t, err, ErrMCPUsageCursor)
}
