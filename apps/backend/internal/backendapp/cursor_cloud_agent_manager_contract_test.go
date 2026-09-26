package backendapp

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/runtime"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCursorCloudCompatibilityMethodMatrix(t *testing.T) {
	repo, sessionID, executionID := seedCursorCloudCompatibilityBinding(t)
	manager := &cursorCloudAgentManager{lifecycleAdapter: &lifecycleAdapter{}, repo: repo}
	ctx := context.Background()

	unsupported := []struct {
		name string
		call func() error
	}{
		{"permission response", func() error { return manager.RespondToPermissionBySessionID(ctx, sessionID, "p", "o", false) }},
		{"permission resolve", func() error {
			_, err := manager.ResolvePermissionBySessionID(ctx, sessionID, "r", "p", "o")
			return err
		}},
		{"permission cancel", func() error { _, err := manager.CancelPermissionBySessionID(ctx, sessionID, "r", "p"); return err }},
		{"restart", func() error { return manager.RestartAgentProcess(ctx, executionID) }},
		{"context reset", func() error { return manager.ResetAgentContext(ctx, executionID) }},
		{"session model change", func() error { return manager.SetSessionModelBySessionID(ctx, sessionID, "other-model") }},
		{"session mode change", func() error { return manager.SetSessionModeBySessionID(ctx, sessionID, "acceptEdits") }},
		{"passthrough stdin", func() error { return manager.WritePassthroughStdin(ctx, sessionID, "data") }},
		{"passthrough config", func() error { _, err := manager.ResolvePassthroughConfig(ctx, sessionID); return err }},
		{"mark passthrough running", func() error { return manager.MarkPassthroughRunning(sessionID) }},
		{"workspace execution", func() error { return manager.EnsureWorkspaceExecutionForSession(ctx, "task-cloud", sessionID) }},
		{"git log", func() error { _, err := manager.GetGitLog(ctx, sessionID, "", 10, ""); return err }},
		{"cumulative diff", func() error { _, err := manager.GetCumulativeDiff(ctx, sessionID, ""); return err }},
		{"git status", func() error { _, err := manager.GetGitStatus(ctx, sessionID); return err }},
		{"fresh git status", func() error { _, err := manager.GetGitStatusFresh(ctx, sessionID); return err }},
		{"agentctl readiness", func() error { return manager.WaitForAgentctlReady(ctx, sessionID) }},
	}
	for _, tc := range unsupported {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); !errors.Is(err, runtime.ErrUnsupported) {
				t.Fatalf("method error = %v, want runtime.ErrUnsupported", err)
			}
		})
	}

	if got, err := manager.ListPendingPermissionsBySessionID(ctx, sessionID); err != nil || len(got) != 0 {
		t.Fatalf("pending permissions = %#v, %v; want empty typed result", got, err)
	}
	if got, err := manager.ProbeBackgroundWorkloads(ctx, sessionID); err != nil || got != agentctlclient.ProbeResultUnknown {
		t.Fatalf("background workload probe = %v, %v; want unknown", got, err)
	}
	if manager.IsPassthroughSession(ctx, sessionID) || len(manager.GetSessionAuthMethods(sessionID)) != 0 {
		t.Fatal("cloud execution reported local passthrough or interactive auth methods")
	}
	if !manager.IsAgentCommandConfigured(executionID) || !manager.WasSessionInitialized(executionID) {
		t.Fatal("accepted managed execution was not reported configured and initialized")
	}
	if !manager.IsAgentRunningForSession(ctx, sessionID) || manager.IsAgentReadyForPrompt(ctx, sessionID) {
		t.Fatal("active managed execution must be running and unavailable for a follow-up")
	}
	if got := manager.LiveSessionIDsForTask("task-cloud"); len(got) != 1 || got[0] != sessionID {
		t.Fatalf("live session IDs = %v, want managed session while provider run is active", got)
	}
	if err := manager.SetExecutionDescription(ctx, executionID, "mutated prompt"); err == nil {
		t.Fatal("managed prompt mutation was accepted")
	}
	if err := manager.SetExecutionEnv(ctx, executionID, nil); err != nil {
		t.Fatalf("empty execution environment: %v", err)
	}
	if err := manager.SetExecutionEnv(ctx, executionID, map[string]string{"TOKEN": "value"}); err == nil {
		t.Fatal("managed execution accepted environment forwarding")
	}
	if got, err := manager.GetExecutionIDForSession(ctx, sessionID); err != nil || got != executionID {
		t.Fatalf("execution lookup = %q, %v; want durable binding", got, err)
	}
	if got, err := manager.GetRemoteRuntimeStatusBySession(ctx, sessionID); err != nil || got == nil || got.State != string(models.ManagedAgentSubmissionAccepted) {
		t.Fatalf("remote status = %#v, %v; want accepted status", got, err)
	}
	binding, err := repo.GetManagedAgentBindingBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	operation, err := repo.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: operation.ID, ExpectedRevision: operation.Revision, ExpectedBindingRevision: binding.Revision,
		State: models.ManagedAgentSubmissionSucceeded,
	}); err != nil {
		t.Fatalf("settle managed operation: %v", err)
	}
	if manager.IsAgentRunningForSession(ctx, sessionID) || !manager.IsAgentReadyForPrompt(ctx, sessionID) {
		t.Fatal("confirmed terminal managed execution must stop and accept a follow-up")
	}
	if got := manager.LiveSessionIDsForTask("task-cloud"); len(got) != 0 {
		t.Fatalf("live session IDs = %v, want no managed session after terminal confirmation", got)
	}
	if err := manager.CleanupStaleExecutionBySessionID(ctx, sessionID); err != nil {
		t.Fatalf("cloud stale cleanup: %v", err)
	}
}

func TestCursorCloudLocalLivenessPreservesLegacyProbeErrorBehavior(t *testing.T) {
	repo, _, _ := seedCursorCloudCompatibilityBinding(t)
	lifecycleManager := lifecycle.NewManager(nil, nil, nil, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), newTestLogger())
	execution := &lifecycle.AgentExecution{
		ID: "local-execution", SessionID: "local-session", Status: v1.AgentStatusReady,
	}
	execution.SetAgentCtlClientForTesting(agentctlclient.NewClient("127.0.0.1", 1, newTestLogger()))
	if err := lifecycleManager.ExecutionStoreForTesting().Add(execution); err != nil {
		t.Fatalf("seed local execution: %v", err)
	}

	manager := &cursorCloudAgentManager{
		lifecycleAdapter: newLifecycleAdapter(lifecycleManager, nil, newTestLogger()),
		repo:             repo,
	}
	if manager.IsAgentRunningForSession(context.Background(), "local-session") {
		t.Fatal("local probe errors must preserve the lifecycle manager's not-running result")
	}
}

func TestCursorCloudLocalPromptFallbackPreservesDispatchCallback(t *testing.T) {
	repo, _, _ := seedCursorCloudCompatibilityBinding(t)
	dispatched := false
	var gotExecutionID, gotPrompt string
	var gotDispatchOnly bool
	manager := &cursorCloudAgentManager{
		repo:    repo,
		enabled: func() bool { return false },
		localPromptWithDispatchCallback: func(_ context.Context, executionID, prompt string, _ []v1.MessageAttachment, dispatchOnly bool, onDispatched func()) (*executor.PromptResult, error) {
			gotExecutionID, gotPrompt, gotDispatchOnly = executionID, prompt, dispatchOnly
			if onDispatched != nil {
				onDispatched()
			}
			return &executor.PromptResult{AgentMessage: "local result"}, nil
		},
	}
	result, err := manager.promptCloud(context.Background(), "local-execution", "follow-up", nil, true, func() { dispatched = true })
	if err != nil {
		t.Fatalf("local prompt fallback: %v", err)
	}
	if gotExecutionID != "local-execution" || gotPrompt != "follow-up" || !gotDispatchOnly {
		t.Fatalf("fallback arguments = execution %q prompt %q dispatch-only %t", gotExecutionID, gotPrompt, gotDispatchOnly)
	}
	if !dispatched {
		t.Fatal("local prompt fallback did not forward the accepted-dispatch callback")
	}
	if result == nil || result.AgentMessage != "local result" {
		t.Fatalf("fallback result = %#v, want local result", result)
	}
}

func TestCursorCloudLocalStopFallbackPreservesReasonAndForce(t *testing.T) {
	repo, _, _ := seedCursorCloudCompatibilityBinding(t)
	log := newTestLogger()
	runtimeBackend := &cursorCloudStopContractBackend{}
	registry := lifecycle.NewExecutorRegistry(log)
	registry.Register(runtimeBackend)
	lifecycleManager := lifecycle.NewManager(nil, nil, registry, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
	for _, executionID := range []string{"local-stop", "local-stop-with-reason"} {
		if err := lifecycleManager.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
			ID: executionID, SessionID: executionID, RuntimeName: agentexecutor.NameStandalone,
		}); err != nil {
			t.Fatalf("seed local lifecycle execution %s: %v", executionID, err)
		}
	}
	manager := &cursorCloudAgentManager{
		lifecycleAdapter: newLifecycleAdapter(lifecycleManager, nil, log),
		repo:             repo,
		enabled:          func() bool { return false },
	}

	if err := manager.StopAgent(context.Background(), "local-stop", true); err != nil {
		t.Fatalf("local stop fallback: %v", err)
	}
	if err := manager.StopAgentWithReason(context.Background(), "local-stop-with-reason", "recoverable agent failure", true); err != nil {
		t.Fatalf("local reasoned stop fallback: %v", err)
	}

	want := []cursorCloudStopCall{
		{executionID: "local-stop", force: true},
		{executionID: "local-stop-with-reason", reason: "recoverable agent failure", force: true},
	}
	if len(runtimeBackend.calls) != len(want) {
		t.Fatalf("runtime stop calls = %#v, want %#v", runtimeBackend.calls, want)
	}
	for i := range want {
		if runtimeBackend.calls[i] != want[i] {
			t.Errorf("runtime stop call %d = %#v, want %#v", i, runtimeBackend.calls[i], want[i])
		}
	}
}

type cursorCloudStopCall struct {
	executionID string
	reason      string
	force       bool
}

type cursorCloudStopContractBackend struct {
	lifecycle.ExecutorBackend
	calls []cursorCloudStopCall
}

func (*cursorCloudStopContractBackend) Name() agentexecutor.Name { return agentexecutor.NameStandalone }

func (b *cursorCloudStopContractBackend) StopInstance(_ context.Context, instance *lifecycle.ExecutorInstance, force bool) error {
	b.calls = append(b.calls, cursorCloudStopCall{
		executionID: instance.InstanceID,
		reason:      instance.StopReason,
		force:       force,
	})
	return nil
}

func TestCursorCloudRequestDigestIncludesFrozenSelection(t *testing.T) {
	base := models.ManagedAgentRequestSnapshot{
		Prompt: "work", MCPMode: "task", RepositoryURL: "https://github.com/acme/repo",
		StartingRef: "main", Model: "model-1", CallbackURL: "https://callback.example.test",
	}
	want := cursorCloudRequestDigest(base)
	cases := []struct {
		name   string
		change func(*models.ManagedAgentRequestSnapshot)
	}{
		{"prompt", func(s *models.ManagedAgentRequestSnapshot) { s.Prompt = "different" }},
		{"turn identity", func(s *models.ManagedAgentRequestSnapshot) { s.TurnID = "turn-2" }},
		{"MCP mode", func(s *models.ManagedAgentRequestSnapshot) { s.MCPMode = "task-title-pending" }},
		{"repository", func(s *models.ManagedAgentRequestSnapshot) { s.RepositoryURL += "/other" }},
		{"published ref", func(s *models.ManagedAgentRequestSnapshot) { s.StartingRef = "release" }},
		{"model", func(s *models.ManagedAgentRequestSnapshot) { s.Model = "model-2" }},
		{"callback", func(s *models.ManagedAgentRequestSnapshot) { s.CallbackURL = "https://other.example.test" }},
		{"PR choice", func(s *models.ManagedAgentRequestSnapshot) { s.AutoCreatePR = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.change(&changed)
			if got := cursorCloudRequestDigest(changed); got == want {
				t.Fatal("changed frozen selection retained the original request digest")
			}
		})
	}
}

func TestNeedsSubmissionCandidateScan(t *testing.T) {
	cases := []struct {
		name      string
		operation *models.ManagedAgentOperation
		want      bool
	}{
		{name: "initial operation accepted", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationCreate, State: models.ManagedAgentSubmissionAccepted}},
		{name: "follow-up accepted", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationFollowup, State: models.ManagedAgentSubmissionAccepted}},
		{name: "follow-up succeeded", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationFollowup, State: models.ManagedAgentSubmissionSucceeded}},
		{name: "unknown initial create", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationCreate, State: models.ManagedAgentSubmissionUnknown}, want: true},
		{name: "submitting initial create", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationCreate, State: models.ManagedAgentSubmissionSubmitting}, want: true},
		{name: "unknown follow-up", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationFollowup, State: models.ManagedAgentSubmissionUnknown}, want: true},
		{name: "submitting follow-up", operation: &models.ManagedAgentOperation{Kind: models.ManagedAgentOperationFollowup, State: models.ManagedAgentSubmissionSubmitting}, want: true},
		{name: "missing operation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := needsSubmissionCandidateScan(tc.operation); got != tc.want {
				t.Fatalf("needsSubmissionCandidateScan() = %v, want %v", got, tc.want)
			}
		})
	}
}

func seedCursorCloudCompatibilityBinding(t *testing.T) (*sqlite.Repository, string, string) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "cursor-cloud-contract.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	database := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqlite.NewWithDB(database, database, nil)
	if err != nil {
		_ = database.Close()
		t.Fatalf("initialize task repository: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-cloud", Name: "Cloud"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-cloud", WorkspaceID: "workspace-cloud", Title: "Cloud task"}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-cloud", TaskID: "task-cloud", ExecutorID: "executor-cloud",
		ExecutorProfileID: "profile-cloud", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Now().UTC()
	binding := &models.ManagedAgentBinding{
		ID: "binding-cloud", SessionID: "session-cloud", TaskID: "task-cloud", WorkspaceID: "workspace-cloud",
		UserID: "user-cloud", ExecutionID: "execution-cloud", ProviderKind: "cursor_cloud", ExecutorID: "executor-cloud",
		ExecutorProfileID: "profile-cloud", CredentialRef: "secret-cloud", RemoteAgentID: "bc-11111111-1111-4111-8111-111111111111",
		Lifecycle: models.ManagedAgentBindingCreating,
		Launch: models.ManagedAgentLaunchSnapshot{
			RepositoryID: "repo-cloud", RepositoryURL: "https://github.com/acme/repo", StartingRef: "main",
			Model: "model-1", CallbackURL: "https://callback.example.test",
		},
	}
	operation := &models.ManagedAgentOperation{
		ID: "operation-cloud", BindingID: binding.ID, PromptTurnID: "cursor-cloud-initial:" + binding.SessionID,
		Kind: models.ManagedAgentOperationCreate, RequestDigest: "digest-cloud",
		RequestSnapshot: models.ManagedAgentRequestSnapshot{
			Prompt: "do work", MCPMode: "task", RepositoryURL: binding.Launch.RepositoryURL,
			StartingRef: binding.Launch.StartingRef, Model: binding.Launch.Model, CallbackURL: binding.Launch.CallbackURL,
		},
	}
	reservedBinding, reserved, replayed, err := repo.ReserveManagedAgentStart(ctx, binding, operation, "owner-cloud", now.Add(time.Minute))
	if err != nil || replayed {
		t.Fatalf("reserve managed start: replayed=%v error=%v", replayed, err)
	}
	submitting, err := repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: reserved.ID, ExpectedRevision: reserved.Revision, ExpectedBindingRevision: reservedBinding.Revision,
		LeaseOwner: "owner-cloud", State: models.ManagedAgentSubmissionSubmitting,
	})
	if err != nil {
		t.Fatalf("mark submitting: %v", err)
	}
	if _, err := repo.CompareAndSwapManagedAgentOperation(ctx, models.ManagedAgentOperationUpdate{
		OperationID: submitting.ID, ExpectedRevision: submitting.Revision, ExpectedBindingRevision: reservedBinding.Revision,
		LeaseOwner: "owner-cloud", State: models.ManagedAgentSubmissionAccepted, RemoteRunID: "run-cloud",
	}); err != nil {
		t.Fatalf("mark accepted: %v", err)
	}
	return repo, binding.SessionID, binding.ExecutionID
}
