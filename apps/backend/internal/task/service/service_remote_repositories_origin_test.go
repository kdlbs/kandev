package service

import (
	"context"
	"errors"
	"testing"
)

type gitLabOriginVerifierStub struct {
	calls []string
	err   error
}

func (v *gitLabOriginVerifierStub) VerifyGitLabRepositoryOrigin(_ context.Context, workspaceID, origin string) error {
	v.calls = append(v.calls, workspaceID+"|"+origin)
	return v.err
}

func TestRegisterRemoteRepositorySelfManagedGitLab(t *testing.T) {
	for _, tc := range []struct {
		name     string
		verifier bool
		denied   bool
	}{
		{name: "configured workspace origin", verifier: true},
		{name: "unconfigured", verifier: false},
		{name: "mismatched workspace origin", verifier: true, denied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			createRepositorySelectionWorkspace(t, repo)
			verifier := &gitLabOriginVerifierStub{}
			if tc.denied {
				verifier.err = NewRepositorySelectionError(RepositorySelectionErrorInvalid, errors.New("workspace origin mismatch"))
			}
			if tc.verifier {
				svc.SetGitLabRepositoryOriginVerifier(verifier)
			}
			registered, created, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
				WorkspaceID: "ws-1", RemoteURL: "https://gitlab.example.test:8443/group/subgroup/project.git", Provider: "gitlab",
				ProviderHost: "https://gitlab.example.test:8443", ProviderOwner: "group/subgroup", ProviderName: "project", DefaultBranch: "dev",
			})
			if !tc.verifier || tc.denied {
				var selectionErr *RepositorySelectionError
				if !errors.As(err, &selectionErr) || selectionErr.Code != RepositorySelectionErrorInvalid {
					t.Fatalf("error=%v, want invalid selection", err)
				}
				repositories, listErr := repo.ListRepositories(ctx, "ws-1")
				if listErr != nil || len(repositories) != 0 {
					t.Fatalf("repositories=%+v err=%v, want no writes", repositories, listErr)
				}
				return
			}
			if err != nil || !created {
				t.Fatalf("registration created=%v err=%v", created, err)
			}
			stored, err := repo.GetRepository(ctx, registered.ID)
			if err != nil || stored.ProviderHost != "https://gitlab.example.test:8443" || stored.ProviderOwner != "group/subgroup" || stored.DefaultBranch != "dev" {
				t.Fatalf("stored=%+v err=%v", stored, err)
			}
			if len(verifier.calls) != 1 || verifier.calls[0] != "ws-1|https://gitlab.example.test:8443" {
				t.Fatalf("verifier calls=%v", verifier.calls)
			}
		})
	}
}
