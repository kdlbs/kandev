package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
)

func TestConvertTaskRepositoriesPreservesSelectionPresence(t *testing.T) {
	tests := []struct {
		name     string
		provided bool
		input    []dto.TaskRepositoryInput
		wantNil  bool
	}{
		{name: "omitted selection allows project defaults", wantNil: true},
		{name: "explicit empty selection remains empty", provided: true},
		{
			name:     "selected repositories are converted",
			provided: true,
			input:    []dto.TaskRepositoryInput{{RepositoryID: "repo-1", BaseBranch: "main"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertTaskRepositories(tt.provided, tt.input)
			if tt.wantNil {
				require.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			if len(tt.input) == 0 {
				require.Empty(t, got)
				return
			}
			require.Len(t, got, 1)
			require.Equal(t, "repo-1", got[0].RepositoryID)
			require.Equal(t, "main", got[0].BaseBranch)
		})
	}
}
