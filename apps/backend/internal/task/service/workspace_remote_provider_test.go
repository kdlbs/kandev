package service

import (
	"context"
	"testing"
)

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.10, AC-TASKS-PROJECT-REPOSITORIES-001.13,
// AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.3
func TestRepositoryPreflightRemoteProviderDiscovery(t *testing.T) {
	t.Run("explicit GitLab hint is verified and trusted", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		verifier := &exactGitLabOriginVerifier{origin: gitLabProjectOrigin}
		svc.SetGitLabRepositoryOriginVerifier(verifier)
		inputs := []TaskRepositoryInput{{RemoteURL: gitLabProjectSource, Provider: providerGitLab}}
		if err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs); err != nil {
			t.Fatalf("preflightRepositoryInputs: %v", err)
		}
		if !inputs[0].TrustedRemote {
			t.Fatal("explicit GitLab input did not receive the verified origin marker")
		}
		if len(verifier.calls) != 1 {
			t.Fatalf("verification calls = %v, want exactly one", verifier.calls)
		}
	})

	t.Run("explicit plugin provider never falls back to discovery", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		resolver := &repositorySelectionResolverStub{resolve: authoritativeRepositoryInput}
		svc.SetRepositorySelectionResolver(resolver)
		inputs := []TaskRepositoryInput{{RemoteURL: pluginProjectSource, Provider: pluginProjectProviderID, BaseBranch: "main"}}
		if err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs); err != nil {
			t.Fatalf("preflightRepositoryInputs: %v", err)
		}
		if len(resolver.discoveries) != 0 || len(resolver.calls) != 1 {
			t.Fatalf("discoveries=%d calls=%d, want the explicit provider only", len(resolver.discoveries), len(resolver.calls))
		}
		if inputs[0].RemoteURL != pluginProjectCloneURL {
			t.Fatalf("input = %+v, want the authoritative descriptor", inputs[0])
		}
	})

	t.Run("a unique plugin match survives an unrelated candidate failure", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
			discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
				return RepositorySelectionDiscoveryResult{
					Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)},
					Failure:    NewRepositorySelectionFailure(RepositorySelectionErrorUnavailable, nil),
				}, nil
			},
		})
		inputs := []TaskRepositoryInput{{RemoteURL: pluginProjectSource}}
		if err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs); err != nil {
			t.Fatalf("preflightRepositoryInputs: %v", err)
		}
		if !inputs[0].TrustedProviderDescriptor || inputs[0].Provider != pluginProjectProviderID {
			t.Fatalf("input = %+v, want the discovered descriptor", inputs[0])
		}
	})

	t.Run("built-in and plugin matches are ambiguous", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		svc.SetGitLabRepositoryOriginVerifier(&gitLabOriginVerifierStub{})
		svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
			discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
				return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)}}, nil
			},
		})
		inputs := []TaskRepositoryInput{{RemoteURL: pluginProjectSource}}
		err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs)
		assertRepositorySelectionError(t, err, RepositorySelectionErrorInvalid, "")
	})

	t.Run("no candidates prefer the most severe failure", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
			discover: func(TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
				return RepositorySelectionDiscoveryResult{
					Failure: NewRepositorySelectionFailure(RepositorySelectionErrorNotFound, nil),
				}, nil
			},
		})
		inputs := []TaskRepositoryInput{{RemoteURL: pluginProjectSource}}
		err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs)
		assertRepositorySelectionError(t, err, RepositorySelectionErrorNotFound, "")
	})

	t.Run("an interrupted discovery is an operation error", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
			discover: func(TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
				return RepositorySelectionDiscoveryResult{}, NewRepositorySelectionFailure(RepositorySelectionErrorUnavailable, context.DeadlineExceeded)
			},
		})
		inputs := []TaskRepositoryInput{{RemoteURL: pluginProjectSource}}
		err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs)
		assertRepositorySelectionError(t, err, RepositorySelectionErrorUnavailable, "")
	})

	t.Run("saved identifiers and local paths skip remote admission", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		createRepositorySelectionWorkspace(t, repo)
		resolver := &repositorySelectionResolverStub{
			discover: func(TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
				t.Fatal("discovery must not run for saved or local inputs")
				return RepositorySelectionDiscoveryResult{}, nil
			},
		}
		svc.SetRepositorySelectionResolver(resolver)
		inputs := []TaskRepositoryInput{{RepositoryID: "repo-1"}, {LocalPath: t.TempDir()}}
		if err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs); err != nil {
			t.Fatalf("preflightRepositoryInputs: %v", err)
		}
	})
}

// @covers AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.3,
// AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.4,
// AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.6
func TestRepositoryPreflightRegistrationSharesRemoteProviderAdmission(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
	}{
		{name: "bare registration", provider: ""},
		{name: "explicit GitLab hint", provider: providerGitLab},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			createRepositorySelectionWorkspace(t, repo)
			verifier := &exactGitLabOriginVerifier{origin: gitLabProjectOrigin}
			svc.SetGitLabRepositoryOriginVerifier(verifier)
			registered, created, err := svc.RegisterRemoteRepository(ctx, &RegisterRemoteRepositoryRequest{
				WorkspaceID: "ws-1", RemoteURL: gitLabProjectSource, Provider: test.provider, DefaultBranch: "main",
			})
			if err != nil || !created {
				t.Fatalf("RegisterRemoteRepository created=%v err=%v", created, err)
			}
			if registered.ProviderHost != gitLabProjectOrigin || registered.ProviderOwner != "group/subgroup" ||
				registered.ProviderName != "project" {
				t.Fatalf("registered = %+v", registered)
			}
			if len(verifier.calls) != 1 {
				t.Fatalf("verification calls = %v, want exactly one shared preflight verification", verifier.calls)
			}
		})
	}
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.12
func TestRepositoryPreflightPluginDiscoveryUsesWorkspaceContext(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	seen := []string{}
	svc.SetRepositorySelectionResolver(&repositorySelectionResolverStub{
		discover: func(input TaskRepositoryInput) (RepositorySelectionDiscoveryResult, error) {
			seen = append(seen, input.RemoteURL)
			return RepositorySelectionDiscoveryResult{Candidates: []TaskRepositoryInput{discoveredPluginCandidate(input)}}, nil
		},
	})
	inputs := []TaskRepositoryInput{{RemoteURL: pluginProjectSource}}
	if err := svc.preflightRepositoryInputs(context.Background(), "ws-1", inputs); err != nil {
		t.Fatalf("preflightRepositoryInputs: %v", err)
	}
	if len(seen) != 1 || seen[0] != pluginProjectSource {
		t.Fatalf("discovery URLs = %v", seen)
	}
}

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.10, AC-TASKS-PROJECT-REPOSITORIES-001.11
func TestRepositoryPreflightRejectsCanceledCandidateSet(t *testing.T) {
	svc, _, repo := createTestService(t)
	createRepositorySelectionWorkspace(t, repo)
	svc.SetGitLabRepositoryOriginVerifier(&exactGitLabOriginVerifier{origin: gitLabProjectOrigin})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	inputs := []TaskRepositoryInput{{RemoteURL: gitLabProjectSource}}
	err := svc.preflightRepositoryInputs(ctx, "ws-1", inputs)
	assertRepositorySelectionError(t, err, RepositorySelectionErrorUnavailable, "")
}
