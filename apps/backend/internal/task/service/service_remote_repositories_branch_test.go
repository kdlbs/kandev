package service

import (
	"context"
	"errors"
	"testing"
)

func TestRegisterRemoteRepositoryPluginDefaultBranch(t *testing.T) {
	for _, tc := range []struct {
		name, branch, want string
		invalid            bool
	}{
		{name: "selected", branch: " release/next ", want: "release/next"},
		{name: "provider default", want: "main"},
		{name: "invalid selection", branch: "--upload-pack=other", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			createRepositorySelectionWorkspace(t, repo)
			svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{resolve: authoritativeRepositoryInput})
			request := &RegisterRemoteRepositoryRequest{
				WorkspaceID: "ws-1", RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
				Provider: "fixture-source-control", DefaultBranch: tc.branch,
			}
			registered, created, err := svc.RegisterRemoteRepository(ctx, request)
			if tc.invalid {
				if !errors.Is(err, ErrInvalidRepositorySettings) {
					t.Fatalf("error = %v, want invalid repository settings", err)
				}
				repositories, listErr := repo.ListRepositories(ctx, "ws-1")
				if listErr != nil || len(repositories) != 0 {
					t.Fatalf("repositories = %+v, error = %v, want no writes", repositories, listErr)
				}
				return
			}
			if err != nil || !created {
				t.Fatalf("registration: created=%v err=%v", created, err)
			}
			stored, err := repo.GetRepository(ctx, registered.ID)
			if err != nil || stored.DefaultBranch != tc.want {
				t.Fatalf("stored repository = %+v, error = %v, want branch %q", stored, err, tc.want)
			}
			request.DefaultBranch = "different-branch"
			existing, created, err := svc.RegisterRemoteRepository(ctx, request)
			if err != nil || created || existing.ID != registered.ID || existing.DefaultBranch != tc.want {
				t.Fatalf("duplicate: repository=%+v created=%v err=%v, want preserved branch %q", existing, created, err, tc.want)
			}
		})
	}
}
