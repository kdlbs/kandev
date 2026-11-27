package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type gitLabConnectionReaderStub struct {
	config    *gitlab.GitLabConfig
	err       error
	workspace string
}

func (r *gitLabConnectionReaderStub) GetConfigForWorkspace(_ context.Context, workspaceID string) (*gitlab.GitLabConfig, error) {
	r.workspace = workspaceID
	return r.config, r.err
}

func TestGitLabRepositoryOriginVerifier(t *testing.T) {
	for _, tc := range []struct {
		name, configured, requested, auth string
		err                               error
		want                              taskservice.RepositorySelectionErrorCode
	}{
		{name: "matching self-managed origin", configured: "https://gitlab.example.test:8443", requested: "https://gitlab.example.test:8443", auth: gitlab.AuthMethodPAT},
		{name: "different host", configured: "https://gitlab.example.test:8443", requested: "https://attacker.example.test", auth: gitlab.AuthMethodPAT, want: taskservice.RepositorySelectionErrorInvalid},
		{name: "different scheme", configured: "https://gitlab.example.test", requested: "http://gitlab.example.test", auth: gitlab.AuthMethodPAT, want: taskservice.RepositorySelectionErrorInvalid},
		{name: "different port", configured: "https://gitlab.example.test:8443", requested: "https://gitlab.example.test", auth: gitlab.AuthMethodPAT, want: taskservice.RepositorySelectionErrorInvalid},
		{name: "not connected", requested: "https://gitlab.example.test", want: taskservice.RepositorySelectionErrorInvalid},
		{name: "auth disabled", configured: "https://gitlab.example.test", requested: "https://gitlab.example.test", auth: gitlab.AuthMethodNone, want: taskservice.RepositorySelectionErrorInvalid},
		{name: "connection read failed", requested: "https://gitlab.example.test", err: errors.New("read failed"), want: taskservice.RepositorySelectionErrorUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &gitLabConnectionReaderStub{err: tc.err}
			if tc.configured != "" {
				r.config = &gitlab.GitLabConfig{Host: tc.configured, AuthMethod: tc.auth}
			}
			verifier := gitLabRepositoryOriginVerifier{connections: r}
			err := verifier.VerifyGitLabRepositoryOrigin(context.Background(), "workspace-selected", tc.requested)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("matched configured origin rejected: %v", err)
				}
			} else {
				var selectionErr *taskservice.RepositorySelectionError
				if !errors.As(err, &selectionErr) || selectionErr.Code != tc.want {
					t.Fatalf("err=%v want code=%q", err, tc.want)
				}
			}
			if r.workspace != "workspace-selected" {
				t.Fatalf("read workspace=%q", r.workspace)
			}
		})
	}
}
