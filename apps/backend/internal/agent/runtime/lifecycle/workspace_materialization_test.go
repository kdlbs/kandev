package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type workspaceMaterializerClientStub struct {
	requests        []agentctl.MaterializeRepositoryRequest
	removals        []agentctl.RemoveMaterializedRepositoryRequest
	removalContexts []context.Context
	rescans         []string
	reconciles      int
	failAt          int
	rescanErr       error
	reconcileErr    error
}

func (s *workspaceMaterializerClientStub) MaterializeRepository(_ context.Context, request agentctl.MaterializeRepositoryRequest) (*agentctl.MaterializeRepositoryResponse, error) {
	s.requests = append(s.requests, request)
	if s.failAt > 0 && len(s.requests) == s.failAt {
		return nil, context.Canceled
	}
	return &agentctl.MaterializeRepositoryResponse{Destination: request.Destination, Reused: request.Destination == "reused"}, nil
}

func (s *workspaceMaterializerClientStub) RemoveMaterializedRepository(ctx context.Context, request agentctl.RemoveMaterializedRepositoryRequest) error {
	s.removals = append(s.removals, request)
	s.removalContexts = append(s.removalContexts, ctx)
	return nil
}

func (s *workspaceMaterializerClientStub) RescanWorkspace(_ context.Context, workdir string, _ ...[]string) error {
	s.rescans = append(s.rescans, workdir)
	return s.rescanErr
}

func (s *workspaceMaterializerClientStub) ReconcileWorkspace(_ context.Context, _ ...[]string) error {
	s.reconciles++
	return s.reconcileErr
}

func TestMaterializeWorkspaceRepositories_RollsBackOnlyNewCheckoutsInReverseOrder(t *testing.T) {
	client := &workspaceMaterializerClientStub{failAt: 3}
	err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{
		{RepositoryURL: "https://github.com/acme/reused.git", Destination: "reused", BaseBranch: "main"},
		{RepositoryURL: "https://github.com/acme/new.git", Destination: "new", BaseBranch: "main", CheckoutBranch: "feature/new"},
		{RepositoryURL: "https://github.com/acme/fails.git", Destination: "fails", BaseBranch: "main"},
	})
	if err == nil {
		t.Fatal("materializeWorkspaceRepositories succeeded despite cancelled third clone")
	}
	if len(client.removals) != 1 || client.removals[0].Destination != "new" {
		t.Fatalf("removals=%+v; want only newly-created checkout rollback", client.removals)
	}
	if _, ok := client.removalContexts[0].Deadline(); !ok {
		t.Fatal("rollback cleanup context has no deadline")
	}
}

func TestRemoteWorkspaceProjectionFromLaunch_SkipsPrimaryWorkspaceRepository(t *testing.T) {
	projection, err := remoteWorkspaceProjectionFromLaunch(&LaunchRequest{Repositories: []RepoLaunchSpec{
		{RepositoryURL: "https://github.com/acme/one.git", RepoName: "one", BaseBranch: "main"},
		{RepositoryURL: "https://github.com/acme/two.git", RepoName: "two", CheckoutBranch: "feature/next"},
	}})
	if err != nil {
		t.Fatalf("remoteWorkspaceProjectionFromLaunch: %v", err)
	}
	if len(projection) != 1 || projection[0].Destination != "two-feature-next" || projection[0].CheckoutBranch != "feature/next" {
		t.Fatalf("projection=%+v; want only the additional repository", projection)
	}
}

func TestRemoteWorkspaceProjectionFromLaunch_KeepsAdditionalBranchOfPrimaryRepository(t *testing.T) {
	projection, err := remoteWorkspaceProjectionFromLaunch(&LaunchRequest{Repositories: []RepoLaunchSpec{
		{RepositoryURL: "https://github.com/acme/repository.git", RepoName: "repository", BaseBranch: "main"},
		{RepositoryURL: "https://github.com/acme/repository.git", RepoName: "repository", BaseBranch: "main", CheckoutBranch: "release/2026"},
	}})
	if err != nil {
		t.Fatalf("remoteWorkspaceProjectionFromLaunch: %v", err)
	}
	if len(projection) != 1 || projection[0].Destination != "repository-release-2026" || projection[0].BaseBranch != "main" || projection[0].CheckoutBranch != "release/2026" {
		t.Fatalf("projection=%+v; want additional branch checkout", projection)
	}
}

func TestQualifiedPRBase_RemoteWorkspaceProjectionForwardsIdentity(t *testing.T) {
	qualifiedBase := lifecycleTestQualifiedPRBase()
	projection, err := remoteWorkspaceProjectionFromLaunch(&LaunchRequest{Repositories: []RepoLaunchSpec{
		{RepositoryURL: "https://github.com/fork/widget.git", RepoName: "primary", BaseBranch: "main"},
		{RepositoryURL: "https://github.com/fork/widget.git", RepoName: "fork", BaseBranch: "release/next", CheckoutBranch: "feature/work", PRNumber: 42, QualifiedPRBase: &qualifiedBase},
	}})
	if err != nil {
		t.Fatalf("remoteWorkspaceProjectionFromLaunch: %v", err)
	}
	if len(projection) != 1 || projection[0].QualifiedPRBase != &qualifiedBase ||
		projection[0].PRNumber != qualifiedBase.Target.Number {
		t.Fatalf("projection = %#v, want qualified PR identity", projection)
	}
}

func TestMaterializeWorkspaceRepositories_ReconcilesAllBeforeRescan(t *testing.T) {
	client := &workspaceMaterializerClientStub{}
	err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{
		{RepositoryURL: "https://github.com/acme/one.git", Destination: "one-main", BaseBranch: "main"},
		{RepositoryURL: "https://github.com/acme/two.git", Destination: "two-main", BaseBranch: "main"},
	})
	if err != nil {
		t.Fatalf("materializeWorkspaceRepositories: %v", err)
	}
	if len(client.requests) != 2 || len(client.rescans) != 1 || client.rescans[0] != "" {
		t.Fatalf("requests=%+v rescans=%+v; want two requests followed by one rescan", client.requests, client.rescans)
	}
}

func TestMaterializeWorkspaceRepositories_ForwardsBaseAndCheckoutBranches(t *testing.T) {
	client := &workspaceMaterializerClientStub{}
	if err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/acme/repository.git", Destination: "repository-feature", BaseBranch: "main", CheckoutBranch: "feature/work",
	}}); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0]; got.BaseBranch != "main" || got.CheckoutBranch != "feature/work" {
		t.Fatalf("request branches = base:%q checkout:%q", got.BaseBranch, got.CheckoutBranch)
	}
}

func TestQualifiedPRBase_RemoteMaterializationForwardsIdentity(t *testing.T) {
	client := &workspaceMaterializerClientStub{}
	qualifiedBase := lifecycleTestQualifiedPRBase()
	if err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/fork/widget.git", Destination: "fork-feature-work",
		BaseBranch: "release/next", CheckoutBranch: "feature/work", PRNumber: 42, QualifiedPRBase: &qualifiedBase,
	}}); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0]; got.QualifiedPRBase != &qualifiedBase || got.PRNumber != 42 || got.BaseBranch != "release/next" {
		t.Fatalf("materialization request = %#v, want qualified PR base", got)
	}
}

func TestQualifiedPRBase_WorkspaceRepositorySpecRetainsIdentity(t *testing.T) {
	qualifiedBase := lifecycleTestQualifiedPRBase()
	specs := workspaceRepositorySpecsFromLaunch(&LaunchRequest{Repositories: []RepoLaunchSpec{{
		RepositoryID: "repo", RepoName: "fork", RepositoryPath: "/repo",
		BaseBranch: "release/next", CheckoutBranch: "feature/work", PRNumber: 42, QualifiedPRBase: &qualifiedBase,
	}}})
	if len(specs) != 1 || specs[0].QualifiedPRBase != &qualifiedBase || specs[0].PRNumber != 42 {
		t.Fatalf("workspace repository specs = %#v, want qualified PR base", specs)
	}
}

func TestQualifiedPRBase_BuildEnvPrepareRequestForwardsIdentity(t *testing.T) {
	qualifiedBase := lifecycleTestQualifiedPRBase()
	prep := buildEnvPrepareRequest(&LaunchRequest{
		RepositoryID: "repo", RepositoryPath: "/repo", PRNumber: 42, QualifiedPRBase: &qualifiedBase,
		Repositories: []RepoLaunchSpec{{RepositoryID: "repo", RepositoryPath: "/repo", PRNumber: 42, QualifiedPRBase: &qualifiedBase}},
	}, "/workspace", "worktree")
	if prep.QualifiedPRBase != &qualifiedBase || prep.PRNumber != 42 || len(prep.Repositories) != 1 ||
		prep.Repositories[0].QualifiedPRBase != &qualifiedBase || prep.Repositories[0].PRNumber != 42 {
		t.Fatalf("prepare request lost qualified PR base: %#v", prep)
	}
}

func TestMaterializeWorkspaceRepositories_ReturnsRollbackReconcileFailure(t *testing.T) {
	rescanErr := errors.New("rescan failed")
	reconcileErr := errors.New("tracker prune failed")
	client := &workspaceMaterializerClientStub{rescanErr: rescanErr, reconcileErr: reconcileErr}
	err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/acme/repository.git", Destination: "repository-main", BaseBranch: "main",
	}})
	if !errors.Is(err, rescanErr) || !errors.Is(err, reconcileErr) {
		t.Fatalf("rollback error = %v, want rescan and exact reconciliation failures", err)
	}
	if client.reconciles != 1 {
		t.Fatalf("reconcile calls = %d, want 1 after checkout cleanup", client.reconciles)
	}
}

func TestMaterializeRepositoriesForEnvironment_RescansEveryDistinctClient(t *testing.T) {
	materializations := 0
	rescans := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspace/materialize-repository":
			materializations++
			_, _ = w.Write([]byte(`{"destination":"added-main"}`))
		case "/api/v1/workspace/rescan":
			rescans++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	clientOne := workspaceMaterializationAgentctlClient(t, server.URL)
	clientTwo := workspaceMaterializationAgentctlClient(t, server.URL)
	store := NewExecutionStore()
	for _, execution := range []*AgentExecution{
		{ID: "execution-1", SessionID: "session-1", TaskEnvironmentID: "environment-1", agentctl: clientOne},
		{ID: "execution-2", SessionID: "session-2", TaskEnvironmentID: "environment-1", agentctl: clientTwo},
	} {
		if err := store.Add(execution); err != nil {
			t.Fatal(err)
		}
	}
	manager := &Manager{executionStore: store, logger: newTestLogger()}

	_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/acme/added.git",
		Destination:   "added-main",
		BaseBranch:    "main",
	}})
	if err != nil {
		t.Fatalf("MaterializeRepositoriesForEnvironment: %v", err)
	}
	if materializations != 1 {
		t.Fatalf("materializations = %d, want 1", materializations)
	}
	if rescans != 2 {
		t.Fatalf("rescans = %d, want 2 for both live executions", rescans)
	}
}

func TestMaterializeRepositoriesForEnvironment_DeduplicatesSharedAgentctlClient(t *testing.T) {
	rescans := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspace/materialize-repository":
			_, _ = w.Write([]byte(`{"destination":"added-main"}`))
		case "/api/v1/workspace/rescan":
			rescans++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	sharedClient := workspaceMaterializationAgentctlClient(t, server.URL)
	store := NewExecutionStore()
	for _, execution := range []*AgentExecution{
		{ID: "execution-1", SessionID: "session-1", TaskEnvironmentID: "environment-1", agentctl: sharedClient},
		{ID: "execution-2", SessionID: "session-2", TaskEnvironmentID: "environment-1", agentctl: sharedClient},
	} {
		if err := store.Add(execution); err != nil {
			t.Fatal(err)
		}
	}
	manager := &Manager{executionStore: store, logger: newTestLogger()}

	ids, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main"}})
	if err != nil {
		t.Fatalf("MaterializeRepositoriesForEnvironment: %v", err)
	}
	if rescans != 1 {
		t.Fatalf("rescans = %d, want 1 for a shared agentctl client", rescans)
	}
	if !sameStrings(ids, []string{"session-1", "session-2"}) {
		t.Fatalf("adopted session ids = %v, want both live sessions", ids)
	}
}

func TestMaterializeRepositoriesForEnvironment_RemovesCheckoutsBeforeReconcilingPriorClients(t *testing.T) {
	events := make([]string, 0, 5)
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspace/materialize-repository":
			events = append(events, "materialize")
			_, _ = w.Write([]byte(`{"destination":"added-main"}`))
		case "/api/v1/workspace/rescan":
			events = append(events, "rescan-first")
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/reconcile":
			events = append(events, "reconcile-first")
			w.WriteHeader(http.StatusOK)
		case "/api/v1/workspace/materialize-repository/remove":
			events = append(events, "remove")
			_, _ = w.Write([]byte(`{"removed":true}`))
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/rescan" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		events = append(events, "rescan-second")
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer second.Close()

	store := NewExecutionStore()
	for _, execution := range []*AgentExecution{
		{ID: "execution-1", SessionID: "session-1", TaskEnvironmentID: "environment-1", agentctl: workspaceMaterializationAgentctlClient(t, first.URL)},
		{ID: "execution-2", SessionID: "session-2", TaskEnvironmentID: "environment-1", agentctl: workspaceMaterializationAgentctlClient(t, second.URL)},
	} {
		if err := store.Add(execution); err != nil {
			t.Fatal(err)
		}
	}
	manager := &Manager{executionStore: store, logger: newTestLogger()}

	ids, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main"}})
	if err == nil {
		t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite second rescan failure")
	}
	if len(ids) != 0 {
		t.Fatalf("adopted session ids = %v, want none after rollback", ids)
	}
	if !sameStrings(events, []string{"materialize", "rescan-first", "rescan-second", "remove", "reconcile-first"}) {
		t.Fatalf("events = %v, want checkout removal before exact rollback reconciliation", events)
	}
}

func workspaceMaterializationAgentctlClient(t *testing.T, rawURL string) *agentctl.Client {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	return agentctl.NewClient(parsed.Hostname(), port, newTestLogger())
}
