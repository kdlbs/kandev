package github

import "testing"

// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-003.3
func TestCurrentHostWatchSurvivesRetainedOldHostWatch(t *testing.T) {
	for _, path := range []string{"batch", "poller fallback", "passive fallback"} {
		t.Run(path, func(t *testing.T) {
			poller, service, client, store := setupBatchedPollerTest(t)
			t.Cleanup(service.Stop)
			ctx := t.Context()
			connection := &WorkspaceConnection{WorkspaceID: testWorkspaceID, Source: ConnectionSourcePAT,
				GitHubHost: "git.example.com", Status: ConnectionStatusActive, CredentialGeneration: 2}
			service.resolver = NewCredentialResolver(&fakeConnectionReader{workspaces: map[string]*WorkspaceConnection{testWorkspaceID: connection}}, nil)
			service.resolver.SetAutomationProvider(enterpriseWatchCredentials{client: client})
			watches := make([]*PRWatch, 0, 2)
			for _, host := range []string{"github.com", "git.example.com"} {
				seedTask(t, store, host, false)
				watch := withTestWorkspace(&PRWatch{GitHubHost: host, SessionID: host, TaskID: host,
					Owner: "o", Repo: "r", Branch: "feat", PRNumber: 42, LastCheckStatus: "pending"})
				if err := store.CreatePRWatch(ctx, watch); err != nil {
					t.Fatal(err)
				}
				if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: host, WorkspaceID: testWorkspaceID, GitHubHost: host,
					Owner: "o", Repo: "r", PRNumber: 42, PRURL: "https://" + host + "/o/r/pull/42",
					PRTitle: "Before sync", HeadBranch: "feat", BaseBranch: "main", State: "open"}); err != nil {
					t.Fatal(err)
				}
				watches = append(watches, watch)
			}
			client.SetPRDetail("o", "r", 42, &PR{Number: 42, RepoOwner: "o", RepoName: "r", State: "open", Title: "Current PR",
				HTMLURL: "https://git.example.com/o/r/pull/42", HeadBranch: "feat", BaseBranch: "main"}, nil)
			client.prResponses = []string{`{"data":{"repo0":{"pr0":{
				"state":"OPEN","title":"Current PR","url":"https://git.example.com/o/r/pull/42",
				"headRefName":"feat","baseRefName":"main","headRefOid":"abc",
				"author":{"login":"alice"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T00:00:00Z",
				"reviews":{"nodes":[]},"reviewRequests":{"totalCount":0},"commits":{"nodes":[]}
			}}}}`}
			switch path {
			case "batch":
				if _, err := service.SyncWorkspaceWatchesBatched(ctx, testWorkspaceID, watches); err != nil {
					t.Fatal(err)
				}
			case "poller fallback":
				poller.runPRWatchFallback(ctx, watches)
			case "passive fallback":
				if err := service.refreshWatchesPerWatch(ctx, testWorkspaceID, watches); err != nil {
					t.Fatal(err)
				}
			}
			current, err := store.GetTaskPR(ctx, "git.example.com")
			if err != nil || current == nil || current.GitHubHost != "git.example.com" || current.PRTitle != "Current PR" {
				t.Fatalf("current watch not synchronized: pr=%+v err=%v", current, err)
			}
			old, err := store.GetTaskPR(ctx, "github.com")
			if err != nil || old == nil || old.PRTitle != "Before sync" {
				t.Fatalf("old-host task changed: pr=%+v err=%v", old, err)
			}
		})
	}
}
