package backendapp

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/gitlab"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

func TestCodeHostBranchListerGitLab(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "true")
	const host = "https://gitlab.example.test:8443"
	pool := newGitLabServiceTestPool(t)
	store, err := gitlab.NewStore(pool.Writer(), pool.Reader())
	if err != nil {
		t.Fatal(err)
	}
	svc := gitlab.NewService(host, nil, gitlab.AuthMethodNone, nil, newTestLogger())
	svc.SetStore(store)
	for _, workspace := range []string{"selected", "other"} {
		if err := store.SaveConfigForWorkspace(t.Context(), workspace, &gitlab.GitLabConfig{
			Host: host, AuthMethod: gitlab.AuthMethodGLab,
		}); err != nil {
			t.Fatal(err)
		}
		client, err := svc.ClientForWorkspaceHost(t.Context(), workspace, host)
		if err != nil {
			t.Fatal(err)
		}
		client.(*gitlab.MockClient).SeedBranches("group/subgroup/project", []gitlab.RepoBranch{{Name: workspace + "-branch"}})
	}
	adapter := codeHostBranchListerAdapter{gitlab: svc}
	source := taskservice.RemoteBranchSource{
		WorkspaceID: "selected", Provider: "gitlab", ProviderHost: host,
		Owner: "group/subgroup", Name: "project",
	}
	branches, err := adapter.ListRepoBranches(t.Context(), source)
	want := []taskservice.Branch{{Name: "selected-branch", Type: "remote"}}
	if err != nil || !reflect.DeepEqual(branches, want) {
		t.Fatalf("branches=%+v err=%v, want %+v", branches, err, want)
	}
	for _, origin := range []string{"", "https://other.example.test", "http://gitlab.example.test:8443", "https://gitlab.example.test"} {
		source.ProviderHost = origin
		if _, err := adapter.ListRepoBranches(t.Context(), source); !errors.Is(err, gitlab.ErrWorkspaceHostMismatch) {
			t.Fatalf("origin=%q err=%v, want host mismatch", origin, err)
		}
	}
	source.ProviderHost = host
	source.WorkspaceID = "unconfigured"
	if _, err := adapter.ListRepoBranches(t.Context(), source); !errors.Is(err, gitlab.ErrNotConfigured) {
		t.Fatalf("unconfigured workspace: %v", err)
	}
	if _, err := (codeHostBranchListerAdapter{}).ListRepoBranches(t.Context(), source); err == nil {
		t.Fatal("missing GitLab service accepted")
	}
}
