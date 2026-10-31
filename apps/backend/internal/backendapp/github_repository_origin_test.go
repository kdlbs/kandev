package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/github"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type repositoryHostBinderStub struct {
	workspace, origin string
	err               error
}

func (b *repositoryHostBinderStub) BindWorkspaceGitHubHost(ctx context.Context, workspace, origin string) (context.Context, error) {
	b.workspace, b.origin = workspace, origin
	return ctx, b.err
}

func TestGitHubRepositoryOriginVerifier(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code taskservice.RepositorySelectionErrorCode
	}{
		{"matching connection", nil, ""},
		{"foreign host", github.ErrGitHubHostMismatch, taskservice.RepositorySelectionErrorInvalid},
		{"not configured", github.ErrGitHubNotConfigured, taskservice.RepositorySelectionErrorInvalid},
		{"connection read failure", errors.New("connection read failed"), taskservice.RepositorySelectionErrorUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binder := &repositoryHostBinderStub{err: tc.err}
			verifier := gitHubRepositoryOriginVerifier{connections: binder}
			err := verifier.VerifyGitHubRepositoryOrigin(context.Background(), "ws-enterprise", "https://github.example.test:8443")
			if tc.code == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var selection *taskservice.RepositorySelectionError
				if !errors.As(err, &selection) || selection.Code != tc.code {
					t.Fatalf("error = %v, want %s", err, tc.code)
				}
			}
			if binder.workspace != "ws-enterprise" || binder.origin != "https://github.example.test:8443" {
				t.Fatalf("binding = %+v", binder)
			}
		})
	}
}
