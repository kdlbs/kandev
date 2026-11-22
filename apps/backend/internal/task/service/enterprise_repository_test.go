package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestEnterpriseRepositorySelectionPreflight(t *testing.T) {
	for _, input := range []TaskRepositoryInput{
		{GitHubURL: "https://github.example.test/acme/widgets"},
		{GitHubURL: "https://gitlab.com/acme/widgets"},
		{RemoteURL: "https://github.example.test/acme/widgets.git", Provider: "github"},
	} {
		if _, err := repositorySelectionNeedsInspection(input); err != nil {
			t.Errorf("Enterprise repository preflight: %v", err)
		}
	}
}

func TestParseEnterpriseRepositoryPreservesHost(t *testing.T) {
	for _, tc := range []struct{ raw, canonical, origin string }{
		{"https://github.example.test/acme/widgets", "https://github.example.test/acme/widgets.git", "https://github.example.test"},
		{"https://github.example.test:8443/acme/widgets", "https://github.example.test:8443/acme/widgets.git", "https://github.example.test:8443"},
		{"https://acme.ghe.com/acme/widgets/pull/42", "https://acme.ghe.com/acme/widgets.git", "https://acme.ghe.com"},
		{"git@github.example.test:acme/widgets.git", "git@github.example.test:acme/widgets.git", "https://github.example.test"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			provider, owner, name, canonical, err := parseRemoteRepositoryURL(tc.raw, "github")
			if err != nil {
				t.Fatal(err)
			}
			if provider != "github" || owner != "acme" || name != "widgets" || canonical != tc.canonical {
				t.Fatalf("parsed = %q %q %q %q", provider, owner, name, canonical)
			}
			if origin := remoteProviderHost(provider, canonical); origin != tc.origin {
				t.Fatalf("origin = %q, want %q", origin, tc.origin)
			}
		})
	}
}

type enterpriseRepositoryOriginStub struct {
	origin string
	calls  []string
}

func (v *enterpriseRepositoryOriginStub) VerifyGitHubRepositoryOrigin(_ context.Context, workspaceID, origin string) error {
	v.calls = append(v.calls, workspaceID+"|"+origin)
	if origin != v.origin {
		return NewRepositorySelectionError(RepositorySelectionErrorInvalid, nil)
	}
	return nil
}

func TestCreateTaskEnterpriseRepository(t *testing.T) {
	for _, tc := range []struct {
		name, origin string
		input        TaskRepositoryInput
	}{
		{"legacy dialog", "https://github.example.test", TaskRepositoryInput{GitHubURL: "https://github.example.test/acme/widgets"}},
		{"bare remote", "https://github.example.test", TaskRepositoryInput{RemoteURL: "https://github.example.test/acme/widgets.git"}},
		{"explicit remote", "https://github.example.test", TaskRepositoryInput{RemoteURL: "https://github.example.test/acme/widgets.git", Provider: "github"}},
		{"custom port", "https://github.example.test:8443", TaskRepositoryInput{GitHubURL: "https://github.example.test:8443/acme/widgets"}},
		{"GHE Cloud", "https://acme.ghe.com", TaskRepositoryInput{GitHubURL: "https://acme.ghe.com/acme/widgets"}},
		{"SSH", "https://github.example.test", TaskRepositoryInput{GitHubURL: "git@github.example.test:acme/widgets.git"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			createRepositorySelectionWorkspace(t, repo)
			public, _, err := svc.FindOrCreateRepository(context.Background(), &FindOrCreateRepositoryRequest{WorkspaceID: "ws-1", Provider: "github", ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widgets", RemoteURL: "https://github.com/acme/widgets.git", DefaultBranch: "main"})
			if err != nil {
				t.Fatal(err)
			}
			verifier := &enterpriseRepositoryOriginStub{origin: tc.origin}
			svc.SetGitHubRepositoryOriginVerifier(verifier)
			tc.input.BaseBranch = "main"
			result, err := svc.CreateTask(context.Background(), &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Enterprise repository", Repositories: []TaskRepositoryInput{tc.input},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Task == nil || len(result.Task.Repositories) != 1 {
				t.Fatalf("task = %+v", result.Task)
			}
			stored, err := repo.GetRepository(context.Background(), result.Task.Repositories[0].RepositoryID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.ID == public.ID || stored.Provider != "github" || stored.ProviderHost != tc.origin || stored.ProviderOwner != "acme" || stored.ProviderName != "widgets" || !strings.Contains(stored.RemoteURL, strings.TrimPrefix(tc.origin, "https://")) {
				t.Fatalf("repository lost Enterprise identity: %+v", stored)
			}
			if len(verifier.calls) == 0 || verifier.calls[0] != "ws-1|"+tc.origin {
				t.Fatalf("verification calls = %v", verifier.calls)
			}
		})
	}
}

func TestEnterpriseRepositoryRejectsUnconfiguredOrForeignOriginBeforeWrites(t *testing.T) {
	for _, configured := range []string{"", "https://other.example.test"} {
		for _, register := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/register=%v", configured, register), func(t *testing.T) {
				svc, _, repo := createTestService(t)
				createRepositorySelectionWorkspace(t, repo)
				if configured != "" {
					svc.SetGitHubRepositoryOriginVerifier(&enterpriseRepositoryOriginStub{origin: configured})
				}
				var err error
				if register {
					_, _, err = svc.RegisterRemoteRepository(context.Background(), &RegisterRemoteRepositoryRequest{WorkspaceID: "ws-1", RemoteURL: "https://github.example.test/acme/widgets"})
				} else {
					_, err = svc.CreateTask(context.Background(), &CreateTaskRequest{WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "denied", Repositories: []TaskRepositoryInput{{GitHubURL: "https://github.example.test/acme/widgets"}}})
				}
				assertRepositorySelectionError(t, err, RepositorySelectionErrorInvalid, "")
				repos, err := repo.ListRepositories(context.Background(), "ws-1")
				if err != nil || len(repos) != 0 {
					t.Fatalf("repository writes = %+v, %v", repos, err)
				}
				tasks, err := repo.ListTasks(context.Background(), "wf-1")
				if err != nil || len(tasks) != 0 {
					t.Fatalf("task writes = %+v, %v", tasks, err)
				}
			})
		}
	}
}

func TestRegisterEnterpriseRepositoryPreservesHost(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	svc.SetGitHubRepositoryOriginVerifier(&enterpriseRepositoryOriginStub{origin: "https://github.example.test"})
	prober := &stubProber{branch: "public-default"}
	svc.SetProviderDefaultBranchProber(prober)
	registered, created, err := svc.RegisterRemoteRepository(context.Background(), &RegisterRemoteRepositoryRequest{
		WorkspaceID: "ws-1", RemoteURL: "https://github.example.test/acme/widgets",
	})
	if err != nil || !created {
		t.Fatalf("register created=%v err=%v", created, err)
	}
	if prober.calls != 0 {
		t.Fatalf("Enterprise locator used the public-only default-branch prober %d times", prober.calls)
	}
	if registered.ProviderHost != "https://github.example.test" || registered.RemoteURL != "https://github.example.test/acme/widgets.git" {
		t.Fatalf("registered=%+v", registered)
	}
}

func TestEnterpriseRepositoryParserRejectsInvalidLocators(t *testing.T) {
	for _, raw := range []string{
		"http://github.example.test/acme/widgets", "https://user:password@github.example.test/acme/widgets",
		"https://api.github.com/acme/widgets", "https://github.example.test/acme/widgets?token=secret",
		"https://github.example.test/acme/widgets#fragment", "https://127.0.0.1/acme/widgets",
	} {
		if _, _, _, _, err := parseRemoteRepositoryURL(raw, "github"); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}
