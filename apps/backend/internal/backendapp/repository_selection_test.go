package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type repositoryProviderInspectorStub struct {
	inspection       *plugins.RepositoryProviderInspection
	err              error
	request          plugins.RepositoryProviderInspectionRequest
	discovery        *plugins.RepositoryProviderDiscoveryResult
	discoveryErr     error
	discoveryRequest plugins.RepositoryProviderDiscoveryRequest
}

func (s *repositoryProviderInspectorStub) InspectRepositoryProvider(
	_ context.Context, _ string, request plugins.RepositoryProviderInspectionRequest,
) (*plugins.RepositoryProviderInspection, error) {
	s.request = request
	return s.inspection, s.err
}

func (s *repositoryProviderInspectorStub) DiscoverRepositoryProviders(
	_ context.Context, _ string, request plugins.RepositoryProviderDiscoveryRequest,
) (*plugins.RepositoryProviderDiscoveryResult, error) {
	s.discoveryRequest = request
	return s.discovery, s.discoveryErr
}

func TestPluginRepositorySelectionResolverUsesAuthoritativeInspection(t *testing.T) {
	inspector := &repositoryProviderInspectorStub{inspection: &plugins.RepositoryProviderInspection{
		ProviderID: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
		ProviderScope: "workspace-a", ProviderRepositoryID: "repo-42", OwnerOrProject: "TEAM",
		Name: "fixture", CloneURL: "https://bitbucket.example.test/scm/TEAM/fixture.git", DefaultBranch: "main",
	}}
	resolver := pluginRepositorySelectionResolver{inspector: inspector}
	input := taskservice.TaskRepositoryInput{
		RemoteURL: "https://bitbucket.example.test/projects/TEAM/fixture",
		Provider:  "fixture-source-control", ProviderScope: "workspace-a", ProviderRepoID: "repo-42",
		ProviderHost: "https://attacker.example.test", ProviderOwner: "attacker", ProviderName: "wrong",
		DefaultBranch: "attacker-default", BaseBranch: "main", CheckoutBranch: "feature/provider-contract",
	}
	resolved, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-1", input)
	if err != nil {
		t.Fatalf("ResolveRepositorySelection: %v", err)
	}
	if inspector.request.Provider != input.Provider || inspector.request.URL != input.RemoteURL ||
		inspector.request.ProviderScope != input.ProviderScope || inspector.request.ProviderRepositoryID != input.ProviderRepoID {
		t.Fatalf("inspection request = %+v", inspector.request)
	}
	if resolved.ProviderHost != "https://bitbucket.example.test" || resolved.ProviderOwner != "TEAM" ||
		resolved.ProviderName != "fixture" || resolved.ProviderRepoID != "repo-42" ||
		resolved.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" || resolved.DefaultBranch != "main" ||
		!resolved.TrustedProviderDescriptor {
		t.Fatalf("resolved input = %+v", resolved)
	}
}

func TestPluginRepositorySelectionResolverMapsTypedProviderFailures(t *testing.T) {
	for _, test := range []struct {
		name         string
		providerCode plugins.RepositoryProviderErrorCode
		want         taskservice.RepositorySelectionErrorCode
	}{
		{name: "invalid", providerCode: plugins.RepositoryProviderErrorInvalid, want: taskservice.RepositorySelectionErrorInvalid},
		{name: "not found", providerCode: plugins.RepositoryProviderErrorNotFound, want: taskservice.RepositorySelectionErrorNotFound},
		{name: "unavailable", providerCode: plugins.RepositoryProviderErrorUnavailable, want: taskservice.RepositorySelectionErrorUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := pluginRepositorySelectionResolver{inspector: &repositoryProviderInspectorStub{
				err: &plugins.RepositoryProviderError{Code: test.providerCode},
			}}
			_, err := resolver.ResolveRepositorySelection(context.Background(), "workspace-1", taskservice.TaskRepositoryInput{
				Provider: "fixture-source-control", RemoteURL: "https://bitbucket.example.test/TEAM/fixture",
			})
			var selectionErr *taskservice.RepositorySelectionError
			if !errors.As(err, &selectionErr) || selectionErr.Code != test.want {
				t.Fatalf("error = %v, want code %q", err, test.want)
			}
		})
	}
}

func TestPluginRepositorySelectionDiscoveryUsesAuthoritativeDescriptors(t *testing.T) {
	inspector := &repositoryProviderInspectorStub{discovery: &plugins.RepositoryProviderDiscoveryResult{
		Candidates: []plugins.RepositoryProviderInspection{
			{
				ProviderID: "fixture-source-control", ProviderHost: "https://bitbucket.example.test",
				ProviderScope: "workspace-a", ProviderRepositoryID: "repo-42", OwnerOrProject: "TEAM",
				Name: "fixture", CloneURL: "https://bitbucket.example.test/scm/TEAM/fixture.git", DefaultBranch: "main",
			},
			{
				ProviderID: "forgejo", ProviderHost: "https://forgejo.example.test",
				ProviderScope: "team", ProviderRepositoryID: "77", OwnerOrProject: "team",
				Name: "widgets", CloneURL: "https://forgejo.example.test/team/widgets.git", DefaultBranch: "trunk",
			},
		},
		Failure: &plugins.RepositoryProviderError{Code: plugins.RepositoryProviderErrorUnavailable},
	}}
	resolver := pluginRepositorySelectionResolver{inspector: inspector}
	input := taskservice.TaskRepositoryInput{
		RemoteURL:  "https://bitbucket.example.test/projects/TEAM/fixture",
		BaseBranch: "main", CheckoutBranch: "feature/provider-contract",
	}
	result, err := resolver.DiscoverRepositorySelections(context.Background(), "workspace-1", input)
	if err != nil {
		t.Fatalf("DiscoverRepositorySelections: %v", err)
	}
	if inspector.discoveryRequest.URL != input.RemoteURL {
		t.Fatalf("discovery request = %+v", inspector.discoveryRequest)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	first := result.Candidates[0]
	if first.Provider != "fixture-source-control" || first.ProviderHost != "https://bitbucket.example.test" ||
		first.ProviderScope != "workspace-a" || first.ProviderRepoID != "repo-42" ||
		first.RemoteURL != "https://bitbucket.example.test/scm/TEAM/fixture.git" ||
		first.DefaultBranch != "main" || first.CheckoutBranch != input.CheckoutBranch ||
		!first.TrustedProviderDescriptor {
		t.Fatalf("first candidate = %+v", first)
	}
	if second := result.Candidates[1]; second.Provider != "forgejo" || second.RemoteURL != "https://forgejo.example.test/team/widgets.git" {
		t.Fatalf("second candidate = %+v", second)
	}
	if result.Failure == nil || result.Failure.Code != taskservice.RepositorySelectionErrorUnavailable {
		t.Fatalf("failure = %v, want unavailable", result.Failure)
	}
}

func TestPluginRepositorySelectionDiscoveryMapsOperationErrors(t *testing.T) {
	resolver := pluginRepositorySelectionResolver{inspector: &repositoryProviderInspectorStub{
		discoveryErr: context.Canceled,
	}}
	_, err := resolver.DiscoverRepositorySelections(context.Background(), "workspace-1", taskservice.TaskRepositoryInput{
		RemoteURL: "https://bitbucket.example.test/TEAM/fixture",
	})
	var selectionErr *taskservice.RepositorySelectionError
	if !errors.As(err, &selectionErr) || selectionErr.Code != taskservice.RepositorySelectionErrorUnavailable {
		t.Fatalf("error = %v, want unavailable", err)
	}
}
