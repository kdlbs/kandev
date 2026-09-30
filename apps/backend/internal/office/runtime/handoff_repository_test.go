package runtime

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// TestValidateHandoffRepository is AC-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-
// 001.8's repository/base_branch attachment: no prior test drove
// validateHandoffRepository directly, so a broken default-branch fallback or
// a dropped workspace-ownership check would not fail any existing test.
func TestValidateHandoffRepository(t *testing.T) {
	repo := &taskmodels.Repository{ID: "repo-1", WorkspaceID: "ws-target", DefaultBranch: "main"}

	tests := []struct {
		name           string
		repositoryID   string
		baseBranch     string
		tasks          *fakeHandoffTasks
		wantValidation bool
		wantErr        bool
		wantNilRepos   bool
		wantBaseBranch string
	}{
		{
			name:         "no repository_id and no base_branch attaches nothing",
			repositoryID: "",
			baseBranch:   "",
			tasks:        &fakeHandoffTasks{},
			wantNilRepos: true,
		},
		{
			name:           "base_branch without repository_id is a validation error",
			repositoryID:   "",
			baseBranch:     "develop",
			tasks:          &fakeHandoffTasks{},
			wantValidation: true,
		},
		{
			name:           "unknown repository_id is a validation error",
			repositoryID:   "repo-missing",
			tasks:          &fakeHandoffTasks{repositoryErr: repoerrors.ErrRepositoryNotFound},
			wantValidation: true,
		},
		{
			name:           "repository belonging to a different workspace is a validation error",
			repositoryID:   "repo-1",
			tasks:          &fakeHandoffTasks{repository: &taskmodels.Repository{ID: "repo-1", WorkspaceID: "ws-other", DefaultBranch: "main"}},
			wantValidation: true,
		},
		{
			name:         "GetRepository failure propagates as a non-validation error",
			repositoryID: "repo-1",
			tasks:        &fakeHandoffTasks{repositoryErr: errors.New("store unavailable")},
			wantErr:      true,
		},
		{
			name:           "repository_id alone resolves to the repository's default branch",
			repositoryID:   "repo-1",
			baseBranch:     "",
			tasks:          &fakeHandoffTasks{repository: repo},
			wantBaseBranch: "main",
		},
		{
			name:           "explicit base_branch overrides the repository's default branch",
			repositoryID:   "repo-1",
			baseBranch:     "feature/x",
			tasks:          &fakeHandoffTasks{repository: repo},
			wantBaseBranch: "feature/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := NewActions(ActionDependencies{Handoff: HandoffDependencies{Tasks: tt.tasks}})

			repos, err := actions.validateHandoffRepository(context.Background(), tt.repositoryID, tt.baseBranch, "ws-target")

			if tt.wantValidation {
				var validation *HandoffValidationError
				if !errors.As(err, &validation) {
					t.Fatalf("error = %v (%T), want *HandoffValidationError", err, err)
				}
				return
			}
			if tt.wantErr {
				if err == nil {
					t.Fatal("error = nil, want a propagated non-validation error")
				}
				var validation *HandoffValidationError
				if errors.As(err, &validation) {
					t.Fatalf("error = %v, want a non-validation error distinguishable from the not-found case", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateHandoffRepository() error = %v, want nil", err)
			}
			if tt.wantNilRepos {
				if repos != nil {
					t.Errorf("repos = %+v, want nil", repos)
				}
				return
			}
			if len(repos) != 1 {
				t.Fatalf("len(repos) = %d, want 1", len(repos))
			}
			if repos[0].RepositoryID != tt.repositoryID {
				t.Errorf("RepositoryID = %q, want %q", repos[0].RepositoryID, tt.repositoryID)
			}
			if repos[0].BaseBranch != tt.wantBaseBranch {
				t.Errorf("BaseBranch = %q, want %q", repos[0].BaseBranch, tt.wantBaseBranch)
			}
		})
	}
}
