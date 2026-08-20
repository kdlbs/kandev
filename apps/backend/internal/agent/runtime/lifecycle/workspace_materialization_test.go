package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type workspaceMaterializerClientStub struct {
	requests                   []agentctl.MaterializeRepositoryRequest
	removals                   []agentctl.RemoveMaterializedRepositoryRequest
	removalContexts            []context.Context
	rescans                    []string
	rescanRoots                [][]string
	reconciles                 int
	failAt                     int
	rescanErr                  error
	reconcileErr               error
	omitGitMetadataAttestation bool
}

func (s *workspaceMaterializerClientStub) MaterializeRepository(_ context.Context, request agentctl.MaterializeRepositoryRequest) (*agentctl.MaterializeRepositoryResponse, error) {
	s.requests = append(s.requests, request)
	if s.failAt > 0 && len(s.requests) == s.failAt {
		return nil, context.Canceled
	}
	return &agentctl.MaterializeRepositoryResponse{Destination: request.Destination, Reused: request.Destination == "reused", GitMetadataAttested: !s.omitGitMetadataAttestation}, nil
}

func (s *workspaceMaterializerClientStub) RemoveMaterializedRepository(ctx context.Context, request agentctl.RemoveMaterializedRepositoryRequest) error {
	s.removals = append(s.removals, request)
	s.removalContexts = append(s.removalContexts, ctx)
	return nil
}

func (s *workspaceMaterializerClientStub) RescanWorkspace(_ context.Context, workdir string, sourceRoots ...[]string) error {
	s.rescans = append(s.rescans, workdir)
	if len(sourceRoots) > 0 {
		s.rescanRoots = append(s.rescanRoots, append([]string(nil), sourceRoots[0]...))
	}
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

func TestRemoteWorkspaceSourceRootsUseCanonicalExecutorPaths(t *testing.T) {
	got := remoteWorkspaceSourceRoots("/workspace", []WorkspaceRepositoryMaterialization{
		{Destination: "frontend-main"},
		{Destination: "api-feature-next"},
	})
	want := []string{"/workspace", "/workspace/frontend-main", "/workspace/api-feature-next"}
	if !equalStrings(got, want) {
		t.Fatalf("roots = %v, want %v", got, want)
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

func TestMaterializeWorkspaceRepositoriesForwardsCanonicalSourceRoots(t *testing.T) {
	client := &workspaceMaterializerClientStub{}
	roots := []string{"/workspace", "/workspace/frontend-main"}
	if err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/acme/frontend.git", Destination: "frontend-main", BaseBranch: "main",
	}}, roots); err != nil {
		t.Fatalf("materializeWorkspaceRepositories: %v", err)
	}
	if len(client.rescanRoots) != 1 || !equalStrings(client.rescanRoots[0], roots) {
		t.Fatalf("rescan roots = %v, want %v", client.rescanRoots, roots)
	}
}

func TestMaterializeWorkspaceRepositoriesRejectsMissingGitMetadataAttestation(t *testing.T) {
	client := &workspaceMaterializerClientStub{omitGitMetadataAttestation: true}
	err := materializeWorkspaceRepositories(context.Background(), client, []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/acme/frontend.git", Destination: "frontend-main", BaseBranch: "main",
	}})
	if err == nil || !strings.Contains(err.Error(), "attestation missing") {
		t.Fatalf("materializeWorkspaceRepositories() error = %v, want missing attestation", err)
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
			_, _ = w.Write([]byte(`{"destination":"added-main","git_metadata_attested":true}`))
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
	if materializations != 2 {
		t.Fatalf("materializations = %d, want 2 for independent executor workspaces", materializations)
	}
	if rescans != 2 {
		t.Fatalf("rescans = %d, want 2 for both live executions", rescans)
	}
}

func TestMaterializeRepositoriesForEnvironmentRefreshesMutableClonePolicyForEveryRemoteRuntime(t *testing.T) {
	for _, runtimeName := range []executor.Name{executor.NameDocker, executor.NameRemoteDocker, executor.NameSSH, executor.NameSprites} {
		t.Run(string(runtimeName), func(t *testing.T) {
			server := newWorkspaceRebindAgentctlServer(t, false)
			t.Cleanup(server.Close)
			t.Cleanup(server.closeConnections)

			manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/executor/workspace"})
			manager.registry = registry.NewRegistry(newTestLogger())
			manager.registry.LoadDefaults()
			manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
			execution.TaskEnvironmentID = "environment-1"
			execution.RuntimeName = runtimeName
			execution.WorkspacePath = "/executor/workspace"
			execution.Status = v1.AgentStatusReady
			execution.ACPSessionID = "acp-existing"
			execution.AgentID = "codex-acp"
			execution.AgentProfileID = "profile-1"
			execution.AgentCommand = "codex-acp"
			execution.setRuntimeEnvironment(map[string]string{"CODEX_CONFIG": `{}`})

			_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
				RepositoryURL: "https://github.com/acme/added.git",
				Destination:   "added-main",
				BaseBranch:    "main",
			}})
			if err != nil {
				t.Fatalf("MaterializeRepositoriesForEnvironment: %v", err)
			}
			wantRoots := []string{"/executor/workspace", "/executor/workspace/added-main"}
			if !sameStrings(execution.WorkspaceSourceRoots, wantRoots) {
				t.Fatalf("workspace roots = %v, want %v", execution.WorkspaceSourceRoots, wantRoots)
			}
			if configs := server.configuredEnvs(); len(configs) != 1 || !strings.Contains(configs[0]["CODEX_CONFIG"], "/executor/workspace/added-main/.git") {
				t.Fatalf("configured clone policies = %#v, want final attached GitDir", configs)
			}
			if server.attestationCalls() != 1 {
				t.Fatalf("attestation calls = %d, want one final batch attestation", server.attestationCalls())
			}
			if loads := server.loads(); !sameStrings(loads, []string{"acp-existing"}) {
				t.Fatalf("restored ACP sessions = %v, want [acp-existing]", loads)
			}
		})
	}
}

func TestMaterializeRepositoriesForEnvironmentMaterializesEveryDistinctCloneExecutor(t *testing.T) {
	first := newWorkspaceRebindAgentctlServer(t, false)
	second := newWorkspaceRebindAgentctlServer(t, false)
	t.Cleanup(first.Close)
	t.Cleanup(first.closeConnections)
	t.Cleanup(second.Close)
	t.Cleanup(second.closeConnections)

	manager, firstExecution := workspaceSourceTestManager(t, first.URL, []string{"/executor/one"})
	manager.registry = registry.NewRegistry(newTestLogger())
	manager.registry.LoadDefaults()
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
	configureCloneAttachmentExecution(firstExecution, "execution-1", "session-1", "environment-1", "/executor/one")
	secondExecution := &AgentExecution{
		ID:                   "execution-2",
		SessionID:            "session-2",
		TaskEnvironmentID:    "environment-1",
		RuntimeName:          executor.NameSSH,
		WorkspacePath:        "/executor/two",
		WorkspaceSourceRoots: []string{"/executor/two"},
		Status:               v1.AgentStatusReady,
		ACPSessionID:         "acp-two",
		AgentID:              "codex-acp",
		AgentProfileID:       "profile-1",
		AgentCommand:         "codex-acp",
		agentctl:             workspaceMaterializationAgentctlClient(t, second.URL),
	}
	secondExecution.setRuntimeEnvironment(map[string]string{"CODEX_CONFIG": `{}`})
	if err := manager.executionStore.Add(secondExecution); err != nil {
		t.Fatal(err)
	}

	_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
		RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main",
	}})
	if err != nil {
		t.Fatalf("MaterializeRepositoriesForEnvironment: %v", err)
	}
	for _, execution := range []*AgentExecution{firstExecution, secondExecution} {
		want := filepath.Join(execution.WorkspacePath, "added-main", ".git")
		if !strings.Contains(execution.RuntimeEnvironment()["CODEX_CONFIG"], want) {
			t.Fatalf("execution %s policy lacks %q: %s", execution.ID, want, execution.RuntimeEnvironment()["CODEX_CONFIG"])
		}
	}
}

func TestMaterializeRepositoriesForEnvironmentCompensatesFirstCloneExecutorWhenSecondMaterializationFails(t *testing.T) {
	first := newWorkspaceRebindAgentctlServer(t, false)
	second := newWorkspaceRebindAgentctlServer(t, false)
	second.failMaterialize = true
	t.Cleanup(first.Close)
	t.Cleanup(first.closeConnections)
	t.Cleanup(second.Close)
	t.Cleanup(second.closeConnections)

	manager, firstExecution := workspaceSourceTestManager(t, first.URL, []string{"/executor/one"})
	manager.registry = registry.NewRegistry(newTestLogger())
	manager.registry.LoadDefaults()
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
	configureCloneAttachmentExecution(firstExecution, "execution-1", "session-1", "environment-1", "/executor/one")
	secondExecution := &AgentExecution{ID: "execution-2", SessionID: "session-2", TaskEnvironmentID: "environment-1", RuntimeName: executor.NameSSH, WorkspacePath: "/executor/two", WorkspaceSourceRoots: []string{"/executor/two"}, Status: v1.AgentStatusReady, agentctl: workspaceMaterializationAgentctlClient(t, second.URL)}
	secondExecution.setRuntimeEnvironment(map[string]string{"CODEX_CONFIG": `{}`})
	if err := manager.executionStore.Add(secondExecution); err != nil {
		t.Fatal(err)
	}

	_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main"}})
	if err == nil {
		t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite second executor materialization failure")
	}
	if first.hasMaterialized("added-main") {
		t.Fatal("first executor retained a repository after second executor materialization failed")
	}
	if !sameStrings(firstExecution.WorkspaceSourceRoots, []string{"/executor/one"}) || firstExecution.Status != v1.AgentStatusReady {
		t.Fatalf("first execution after compensation = roots:%v status:%q, want untouched ready state", firstExecution.WorkspaceSourceRoots, firstExecution.Status)
	}
}

func TestMaterializeRepositoriesForEnvironmentFailsClosedWhenPartialCloneCleanupFails(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	server.failMaterializeAt = 2
	server.failRemove = true
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)

	manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/executor/workspace"})
	manager.registry = registry.NewRegistry(newTestLogger())
	manager.registry.LoadDefaults()
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
	configureCloneAttachmentExecution(execution, "execution", "session", "environment-1", "/executor/workspace")

	_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{
		{RepositoryURL: "https://github.com/acme/one.git", Destination: "one-main", BaseBranch: "main"},
		{RepositoryURL: "https://github.com/acme/two.git", Destination: "two-main", BaseBranch: "main"},
	})
	if err == nil {
		t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite partial clone cleanup failure")
	}
	if execution.Status != v1.AgentStatusFailed || server.running() {
		t.Fatalf("execution = status:%q running:%v, want failed stopped recovery state", execution.Status, server.running())
	}
	if !strings.Contains(execution.ErrorMessage, "recovery required") {
		t.Fatalf("execution recovery guidance = %q", execution.ErrorMessage)
	}
	if !server.hasMaterialized("one-main") {
		t.Fatal("fixture did not retain the unreverted first checkout")
	}
	if strings.Contains(execution.RuntimeEnvironment()["CODEX_CONFIG"], "one-main/.git") {
		t.Fatalf("partial checkout widened live policy: %s", execution.RuntimeEnvironment()["CODEX_CONFIG"])
	}
}

func TestMaterializeRepositoriesForEnvironmentCompensatesRefreshedCloneExecutors(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		configureSecond   func(*workspaceRebindAgentctlServer)
		wantSecondFailed  bool
		wantSecondRemoved bool
	}{
		{name: "final attestation", configureSecond: func(server *workspaceRebindAgentctlServer) { server.failAttestAt = 1 }, wantSecondRemoved: true},
		{name: "configure", configureSecond: func(server *workspaceRebindAgentctlServer) { server.failConfigureAt = 1 }, wantSecondRemoved: true},
		{name: "start", configureSecond: func(server *workspaceRebindAgentctlServer) { server.failStartAt = 1 }, wantSecondRemoved: true},
		{name: "ACP restoration", configureSecond: func(server *workspaceRebindAgentctlServer) { server.failLoadAt = 1 }, wantSecondRemoved: true},
		{name: "permanent rollback configure", configureSecond: func(server *workspaceRebindAgentctlServer) { server.failConfigure = true }, wantSecondFailed: true, wantSecondRemoved: true},
		{name: "checkout cleanup", configureSecond: func(server *workspaceRebindAgentctlServer) { server.failStartAt = 1; server.failRemove = true }, wantSecondFailed: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			first := newWorkspaceRebindAgentctlServer(t, false)
			second := newWorkspaceRebindAgentctlServer(t, false)
			testCase.configureSecond(second)
			t.Cleanup(first.Close)
			t.Cleanup(first.closeConnections)
			t.Cleanup(second.Close)
			t.Cleanup(second.closeConnections)

			manager, firstExecution := workspaceSourceTestManager(t, first.URL, []string{"/executor/one"})
			manager.registry = registry.NewRegistry(newTestLogger())
			manager.registry.LoadDefaults()
			manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
			configureCloneAttachmentExecution(firstExecution, "execution-1", "session-1", "environment-1", "/executor/one")
			secondExecution := &AgentExecution{ID: "execution-2", SessionID: "session-2", TaskEnvironmentID: "environment-1", RuntimeName: executor.NameSSH, WorkspacePath: "/executor/two", WorkspaceSourceRoots: []string{"/executor/two"}, Status: v1.AgentStatusReady, ACPSessionID: "acp-two", AgentID: "codex-acp", AgentProfileID: "profile-1", AgentCommand: "codex-acp", agentctl: workspaceMaterializationAgentctlClient(t, second.URL)}
			secondExecution.setRuntimeEnvironment(map[string]string{"CODEX_CONFIG": `{}`})
			if err := manager.executionStore.Add(secondExecution); err != nil {
				t.Fatal(err)
			}

			_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main"}})
			if err == nil {
				t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite second post-refresh failure")
			}
			assertRestoredCloneAttachmentExecution(t, first, firstExecution, "/executor/one", "acp-one", false)
			assertRestoredCloneAttachmentExecution(t, second, secondExecution, "/executor/two", "acp-two", testCase.wantSecondFailed)
			if first.hasMaterialized("added-main") {
				t.Fatal("first executor retained the attached checkout after second executor refresh failure")
			}
			if got := second.hasMaterialized("added-main"); got != !testCase.wantSecondRemoved {
				t.Fatalf("second materialization present = %v, want %v", got, !testCase.wantSecondRemoved)
			}
		})
	}
}

func assertRestoredCloneAttachmentExecution(t *testing.T, server *workspaceRebindAgentctlServer, execution *AgentExecution, workspacePath, acpID string, wantFailed bool) {
	t.Helper()
	if !sameStrings(execution.WorkspaceSourceRoots, []string{workspacePath}) {
		t.Fatalf("execution %s roots = %v, want prior root", execution.ID, execution.WorkspaceSourceRoots)
	}
	policy := execution.RuntimeEnvironment()["CODEX_CONFIG"]
	if strings.Contains(policy, "added-main/.git") {
		t.Fatalf("execution %s retained attached Git policy: %s", execution.ID, policy)
	}
	if wantFailed {
		if execution.Status != v1.AgentStatusFailed || server.running() {
			t.Fatalf("execution %s = status:%q running:%v, want failed stopped recovery state", execution.ID, execution.Status, server.running())
		}
		return
	}
	if execution.Status != v1.AgentStatusReady || !server.running() {
		t.Fatalf("execution %s = status:%q running:%v, want restored ready child", execution.ID, execution.Status, server.running())
	}
	if !strings.Contains(policy, filepath.Join(workspacePath, ".git")) {
		t.Fatalf("execution %s policy %q lacks restored GitDir", execution.ID, policy)
	}
	loads := server.loads()
	if len(loads) == 0 || loads[len(loads)-1] != acpID {
		t.Fatalf("execution %s ACP loads = %v, want restored prior session %q", execution.ID, loads, acpID)
	}
}

func TestMaterializeRepositoriesForEnvironmentAttestsOnlyAfterStoppingCloneChild(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	server.failAttestAt = 1
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)
	manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/executor/workspace"})
	manager.registry = registry.NewRegistry(newTestLogger())
	manager.registry.LoadDefaults()
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
	configureCloneAttachmentExecution(execution, "execution", "session", "environment-1", "/executor/workspace")

	_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main"}})
	if err == nil {
		t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite final attestation barrier failure")
	}
	operations := server.operationLog()
	if !sameStrings(operations, []string{"materialize", "stop", "rescan", "attest", "stop", "reconcile", "attest", "configure", "start", "remove"}) {
		t.Fatalf("operations = %v, want final attestation after stop and before configure/start", operations)
	}
	for _, env := range server.configuredEnvs() {
		if strings.Contains(env["CODEX_CONFIG"], "added-main/.git") {
			t.Fatalf("configured widened policy after failed final attestation: %s", env["CODEX_CONFIG"])
		}
	}
}

func TestMaterializeRepositoriesForEnvironmentFailsClosedWhenRollbackCannotReattestPriorRoots(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	server.attestationErr = true
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)
	manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/executor/workspace"})
	manager.registry = registry.NewRegistry(newTestLogger())
	manager.registry.LoadDefaults()
	manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
	configureCloneAttachmentExecution(execution, "execution", "session", "environment-1", "/executor/workspace")

	_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main"}})
	if err == nil {
		t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite permanent attestation failure")
	}
	if execution.Status != v1.AgentStatusFailed {
		t.Fatalf("execution status = %q, want failed when prior roots cannot be re-attested", execution.Status)
	}
	if !strings.Contains(execution.ErrorMessage, "recovery required") {
		t.Fatalf("execution recovery guidance = %q", execution.ErrorMessage)
	}
	if server.startCount != 0 || len(server.configuredEnvs()) != 0 {
		t.Fatalf("unsafe rollback attempted to restart/configure without prior-root proof: starts=%d configs=%#v", server.startCount, server.configuredEnvs())
	}
	if server.hasMaterialized("added-main") {
		t.Fatal("materialized repository remained after failed closed refresh")
	}
}

func configureCloneAttachmentExecution(execution *AgentExecution, id, sessionID, environmentID, workspacePath string) {
	execution.ID = id
	execution.SessionID = sessionID
	execution.TaskEnvironmentID = environmentID
	execution.RuntimeName = executor.NameDocker
	execution.WorkspacePath = workspacePath
	execution.WorkspaceSourceRoots = []string{workspacePath}
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-one"
	execution.AgentID = "codex-acp"
	execution.AgentProfileID = "profile-1"
	execution.AgentCommand = "codex-acp"
	execution.setRuntimeEnvironment(map[string]string{"CODEX_CONFIG": `{}`})
}

func TestMaterializeRepositoriesForEnvironmentRollsBackClonePolicyOnAttestationOrRestartFailure(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		configure     func(*workspaceRebindAgentctlServer)
		wantConfigs   int
		wantStartCall int
	}{
		{name: "attestation", configure: func(server *workspaceRebindAgentctlServer) { server.failAttestAt = 1 }, wantConfigs: 1, wantStartCall: 1},
		{name: "restart", configure: func(server *workspaceRebindAgentctlServer) { server.failStartAt = 1 }, wantConfigs: 2, wantStartCall: 2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := newWorkspaceRebindAgentctlServer(t, false)
			testCase.configure(server)
			t.Cleanup(server.Close)
			t.Cleanup(server.closeConnections)

			manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/executor/workspace"})
			manager.registry = registry.NewRegistry(newTestLogger())
			manager.registry.LoadDefaults()
			manager.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{AgentName: "codex-acp"}}
			execution.TaskEnvironmentID = "environment-1"
			execution.RuntimeName = executor.NameDocker
			execution.WorkspacePath = "/executor/workspace"
			execution.Status = v1.AgentStatusReady
			execution.ACPSessionID = "acp-existing"
			execution.AgentID = "codex-acp"
			execution.AgentProfileID = "profile-1"
			execution.AgentCommand = "codex-acp"
			execution.setRuntimeEnvironment(map[string]string{"CODEX_CONFIG": `{}`})

			_, err := manager.MaterializeRepositoriesForEnvironment(context.Background(), "environment-1", []WorkspaceRepositoryMaterialization{{
				RepositoryURL: "https://github.com/acme/added.git", Destination: "added-main", BaseBranch: "main",
			}})
			if err == nil {
				t.Fatal("MaterializeRepositoriesForEnvironment succeeded despite policy refresh failure")
			}
			if !sameStrings(execution.WorkspaceSourceRoots, []string{"/executor/workspace"}) {
				t.Fatalf("workspace roots after failure = %v, want only prior task root", execution.WorkspaceSourceRoots)
			}
			if got := execution.RuntimeEnvironment()["CODEX_CONFIG"]; !strings.Contains(got, "/executor/workspace/.git") || strings.Contains(got, "added-main/.git") {
				t.Fatalf("runtime policy after failure = %q, want re-attested original policy only", got)
			}
			if configs := server.configuredEnvs(); len(configs) != testCase.wantConfigs {
				t.Fatalf("configure calls = %#v, want %d", configs, testCase.wantConfigs)
			}
			if testCase.wantConfigs > 0 {
				rollbackPolicy := server.configuredEnvs()[testCase.wantConfigs-1]["CODEX_CONFIG"]
				if !strings.Contains(rollbackPolicy, "/executor/workspace/.git") || strings.Contains(rollbackPolicy, "added-main/.git") {
					t.Fatalf("rollback policy = %q, want re-attested original config", rollbackPolicy)
				}
			}
			if testCase.wantStartCall > 0 && server.startCount != testCase.wantStartCall {
				t.Fatalf("start calls = %d, want %d", server.startCount, testCase.wantStartCall)
			}
		})
	}
}

func TestMaterializeRepositoriesForEnvironment_DeduplicatesSharedAgentctlClient(t *testing.T) {
	rescans := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspace/materialize-repository":
			_, _ = w.Write([]byte(`{"destination":"added-main","git_metadata_attested":true}`))
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
			_, _ = w.Write([]byte(`{"destination":"added-main","git_metadata_attested":true}`))
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
		switch r.URL.Path {
		case "/api/v1/workspace/materialize-repository":
			events = append(events, "materialize-second")
			_, _ = w.Write([]byte(`{"destination":"added-main","git_metadata_attested":true}`))
		case "/api/v1/workspace/rescan":
			events = append(events, "rescan-second")
			w.WriteHeader(http.StatusUnprocessableEntity)
		case "/api/v1/workspace/materialize-repository/remove":
			events = append(events, "remove-second")
			_, _ = w.Write([]byte(`{"removed":true}`))
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
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
	if !sameStrings(events, []string{"materialize", "materialize-second", "rescan-first", "rescan-second", "remove-second", "remove", "reconcile-first"}) {
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
