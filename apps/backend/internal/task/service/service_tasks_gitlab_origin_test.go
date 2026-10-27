package service

import (
	"context"
	"errors"
	"testing"
)

func TestCreateTaskGitLabRepositoryOrigin(t *testing.T) {
	const origin = "https://gitlab.example.test:8443"
	for _, tc := range []struct {
		name       string
		url        string
		verifier   bool
		denied     RepositorySelectionErrorCode
		wantCode   RepositorySelectionErrorCode
		wantOrigin string
		trusted    bool
	}{
		{name: "configured HTTPS origin", url: origin + "/group/subgroup/project.git", verifier: true, wantOrigin: origin},
		{name: "configured HTTP origin", url: "http://gitlab.example.test/group/subgroup/project.git", verifier: true, wantOrigin: "http://gitlab.example.test"},
		{name: "unconfigured origin", url: origin + "/group/project.git", wantCode: RepositorySelectionErrorInvalid},
		{name: "mismatched origin", url: origin + "/group/project.git", verifier: true, denied: RepositorySelectionErrorInvalid, wantCode: RepositorySelectionErrorInvalid, wantOrigin: origin},
		{name: "unavailable configuration", url: origin + "/group/project.git", verifier: true, denied: RepositorySelectionErrorUnavailable, wantCode: RepositorySelectionErrorUnavailable, wantOrigin: origin},
		{name: "trusted contribution", url: origin + "/group/project.git", trusted: true},
		{name: "public GitLab", url: "https://gitlab.com/group/project.git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			createRepositorySelectionWorkspace(t, repo)
			verifier := &gitLabOriginVerifierStub{}
			if tc.denied != "" {
				verifier.err = NewRepositorySelectionError(tc.denied, errors.New("private configuration details"))
			}
			if tc.verifier {
				svc.SetGitLabRepositoryOriginVerifier(verifier)
			}
			result, err := svc.CreateTask(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "GitLab task",
				Repositories: []TaskRepositoryInput{{RemoteURL: tc.url, Provider: "gitlab", BaseBranch: "dev", TrustedRemote: tc.trusted}},
			})
			if tc.verifier && (len(verifier.calls) != 1 || verifier.calls[0] != "ws-1|"+tc.wantOrigin) {
				t.Fatalf("verifier calls=%v, want selected workspace and parsed origin %q", verifier.calls, tc.wantOrigin)
			}
			if tc.wantCode != "" {
				assertRepositorySelectionError(t, err, tc.wantCode, "private configuration details")
				tasks, listErr := repo.ListTasks(ctx, "wf-1")
				if listErr != nil || len(tasks) != 0 {
					t.Fatalf("tasks=%+v err=%v, want no persisted tasks", tasks, listErr)
				}
				repositories, listErr := repo.ListRepositories(ctx, "ws-1")
				if listErr != nil || len(repositories) != 0 {
					t.Fatalf("repositories=%+v err=%v, want no persisted repositories", repositories, listErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateTask: %v", err)
			}
			if result.Task == nil || len(result.Task.Repositories) != 1 || result.Task.Repositories[0].BaseBranch != "dev" {
				t.Fatalf("created task=%+v, want repository on requested branch", result.Task)
			}
		})
	}
}
