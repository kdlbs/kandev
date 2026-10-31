package mcpconfig

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type importStatusFake struct {
	*selectionRepositoryFake
	state LegacyImportState
	err   error
	reads int
}

func (r *importStatusFake) GetMCPImportState(context.Context, string, string) (LegacyImportState, error) {
	r.reads++
	return r.state, r.err
}

// @covers AC-AGENTS-MCP-MANAGEMENT-002.5
func TestProfileMCPImportStatus(t *testing.T) {
	for _, tc := range []struct {
		stored string
		err    error
		want   string
	}{{LegacyImportStatusComplete, nil, "complete"}, {LegacyImportStatusPending, nil, "pending"}, {"unexpected", nil, "unknown"}, {"complete", errors.New("private failure"), "unknown"}, {"", ErrMCPLegacyImportStateNotFound, "unknown"}} {
		repo := &importStatusFake{selectionRepositoryFake: newSelectionRepositoryFake(), state: LegacyImportState{Status: tc.stored, FailureReason: "private failure"}, err: tc.err}
		svc := NewSelectionService(repo, newCatalogRepositoryFake())
		svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
		svc.SetOwnerValidator(func(context.Context, SelectionScope, string, string) error { return nil })
		require.Equal(t, tc.want, svc.ProfileImportStatus(context.Background(), "workspace", "profile"))
		require.Zero(t, repo.replaceCalls)
		svc.SetOwnerValidator(func(context.Context, SelectionScope, string, string) error { return errors.New("denied") })
		before := repo.reads
		require.Equal(t, "unknown", svc.ProfileImportStatus(context.Background(), "workspace", "profile"))
		require.Equal(t, before, repo.reads)
	}
}
