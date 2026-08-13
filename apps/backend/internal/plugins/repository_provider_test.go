package plugins

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func TestListRepositoryProviderBranchesUsesOwnedDeclaredActionAndVerifiedWorkspace(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), repositoryBranchPackage(t)); err != nil {
		t.Fatalf("install provider plugin: %v", err)
	}
	invocations := 0
	branches, err := svc.listRepositoryProviderBranches(t.Context(), "workspace-1", RepositoryProviderSource{
		Provider: "bitbucket", ProviderHost: "https://bitbucket.org",
		ProviderRepositoryID: "acme/widgets", OwnerOrProject: "acme", Name: "widgets",
		CloneURL: "https://bitbucket.org/acme/widgets.git", DefaultBranch: "main",
	}, func(_ context.Context, id string, _ pluginDispatchGeneration, request *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		invocations++
		if id != "kandev-plugin-bitbucket" || request.ActionKey != repositoryBranchesActionKey || request.Context.WorkspaceID != "workspace-1" {
			t.Fatalf("unexpected dispatch: id=%q request=%+v", id, request)
		}
		if !bytes.Contains(request.Body, []byte(`"owner_or_project":"acme"`)) || !bytes.Contains(request.Body, []byte(`"name":"widgets"`)) {
			t.Fatalf("request omitted host-derived repository identity: %s", request.Body)
		}
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"branches":[{"name":"main","commit":"abc","is_default":true},{"name":"main"},{"name":"feature"}]}`)}, nil
	})
	if err != nil {
		t.Fatalf("ListRepositoryProviderBranches: %v", err)
	}
	if invocations != 1 || len(branches) != 2 || branches[0].Name != "main" || !branches[0].IsDefault || branches[1].Name != "feature" {
		t.Fatalf("unexpected branches: %+v (calls=%d)", branches, invocations)
	}
}

func TestListRepositoryProviderBranchesRejectsMissingContractAndInvalidResponse(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), testPackageWithRepositoryProvider(t, "kandev-plugin-bitbucket", "bitbucket")); err != nil {
		t.Fatalf("install provider plugin: %v", err)
	}
	_, err := svc.listRepositoryProviderBranches(t.Context(), "workspace-1", RepositoryProviderSource{Provider: "bitbucket"}, nil)
	if err == nil {
		t.Fatal("expected missing standardized action to fail")
	}
	if _, err := parseRepositoryProviderBranches(&pluginsdk.PluginActionResponse{Body: []byte(`{"branches":[{"name":""}]}`)}); err == nil {
		t.Fatal("expected empty branch name to fail")
	}
}

func repositoryBranchPackage(t *testing.T) *bytes.Buffer {
	t.Helper()
	manifestYAML := fmt.Sprintf(`
id: kandev-plugin-bitbucket
api_version: 1
version: "1.0.0"
display_name: Bitbucket
repository_providers: [bitbucket]
actions:
  - key: repositories.branches
    scope: workspace
    max_body_bytes: 16384
runtime:
  type: binary
  executables:
    %s-%s: server/plugin
`, runtime.GOOS, runtime.GOARCH)
	var buffer bytes.Buffer
	if err := pkgtartest.WritePackage(&buffer, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buffer
}
