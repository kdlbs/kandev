package coordinator

import (
	"context"
	"errors"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// fakeWorkspaceAuthorizer is a test double for WorkspaceAuthorizer: err, when
// set, simulates AuthorizeWorkspaceScope failing (e.g.
// repoerrors.ErrWorkspaceNotFound or service.ErrForbidden). It also records
// every scope it was called with, so tests can assert a read route never
// authorizes with the manage scope (or vice versa).
type fakeWorkspaceAuthorizer struct {
	err    error
	scopes []authz.Scope
}

func (f *fakeWorkspaceAuthorizer) AuthorizeWorkspaceScope(_ context.Context, _ string, scope authz.Scope) error {
	f.scopes = append(f.scopes, scope)
	return f.err
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return log
}

func newServiceForTest(
	t *testing.T,
	agents map[string]*settingsmodels.AgentProfile,
	executors map[string]*taskmodels.ExecutorProfile,
	authzErr error,
) *Service {
	t.Helper()
	store := newTestStore(t)
	validator := newValidatorForTest(agents, executors)
	return NewService(store, validator, &fakeWorkspaceAuthorizer{err: authzErr}, newTestLogger(t))
}

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("error type = %T (%v), want *FieldError", err, err)
	}
	if fieldErr.Field != field {
		t.Errorf("FieldError.Field = %q, want %q", fieldErr.Field, field)
	}
}

// assertLastScope fails unless svc's fakeWorkspaceAuthorizer was last called
// with want, catching a read/write scope swap on a coordinator route
// (RV-005: previously only the return value was asserted, never which
// authz.Scope constant was requested).
func assertLastScope(t *testing.T, svc *Service, want authz.Scope) {
	t.Helper()
	fake, ok := svc.authz.(*fakeWorkspaceAuthorizer)
	if !ok {
		t.Fatalf("svc.authz is %T, want *fakeWorkspaceAuthorizer", svc.authz)
	}
	if len(fake.scopes) == 0 {
		t.Fatal("AuthorizeWorkspaceScope was not called")
	}
	if got := fake.scopes[len(fake.scopes)-1]; got != want {
		t.Errorf("AuthorizeWorkspaceScope scope = %v, want %v", got, want)
	}
}

func TestServiceCreateCoordinator(t *testing.T) {
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("creates a trimmed coordinator", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "  Release Coordinator  ", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1", Context: "  standing context  ",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		if created.ID == "" {
			t.Error("CreateCoordinator() did not assign an id")
		}
		if created.Name != "Release Coordinator" {
			t.Errorf("CreateCoordinator() name = %q, want trimmed", created.Name)
		}
		if created.Context != "standing context" {
			t.Errorf("CreateCoordinator() context = %q, want trimmed", created.Context)
		}
		if created.WorkspaceID != workspaceID {
			t.Errorf("CreateCoordinator() workspace_id = %q, want %q", created.WorkspaceID, workspaceID)
		}
	})

	t.Run("invalid name is a FieldError naming name", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		_, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "   ", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		assertFieldError(t, err, "name")
	})

	t.Run("context over the limit is a FieldError naming context", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		over := make([]byte, 4001)
		for i := range over {
			over[i] = 'a'
		}
		_, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1", Context: string(over),
		})
		assertFieldError(t, err, "context")
	})

	t.Run("missing agent profile is a FieldError naming agent_profile_id", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		_, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "does-not-exist", ExecutorProfileID: "ep-1",
		})
		assertFieldError(t, err, "agent_profile_id")
	})

	t.Run("missing executor profile is a FieldError naming executor_profile_id", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		_, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "does-not-exist",
		})
		assertFieldError(t, err, "executor_profile_id")
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, agents, executors, wantErr)
		_, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("CreateCoordinator() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceManage)
	})
}

func TestServiceGetCoordinator(t *testing.T) {
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("returns the coordinator and ok/ok statuses", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		found, agentStatus, executorStatus, err := svc.GetCoordinator(context.Background(), workspaceID, created.ID)
		if err != nil {
			t.Fatalf("GetCoordinator() unexpected error: %v", err)
		}
		if found.ID != created.ID {
			t.Errorf("GetCoordinator() id = %q, want %q", found.ID, created.ID)
		}
		if agentStatus != ProfileStatusOK || executorStatus != ProfileStatusOK {
			t.Errorf("GetCoordinator() statuses = (%q, %q), want (ok, ok)", agentStatus, executorStatus)
		}
	})

	t.Run("reports a missing agent profile without failing", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		// Simulate the agent profile being removed after save.
		svc.validator = newValidatorForTest(nil, executors)
		_, agentStatus, executorStatus, err := svc.GetCoordinator(context.Background(), workspaceID, created.ID)
		if err != nil {
			t.Fatalf("GetCoordinator() unexpected error: %v", err)
		}
		if agentStatus != ProfileStatusMissing {
			t.Errorf("agentStatus = %q, want %q", agentStatus, ProfileStatusMissing)
		}
		if executorStatus != ProfileStatusOK {
			t.Errorf("executorStatus = %q, want %q", executorStatus, ProfileStatusOK)
		}
	})

	t.Run("not found is ErrNotFound", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		_, _, _, err := svc.GetCoordinator(context.Background(), workspaceID, "does-not-exist")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetCoordinator() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, agents, executors, wantErr)
		_, _, _, err := svc.GetCoordinator(context.Background(), workspaceID, "any")
		if !errors.Is(err, wantErr) {
			t.Fatalf("GetCoordinator() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceRead)
	})
}

func TestServiceListCoordinators(t *testing.T) {
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("never nil, and pairs each coordinator with its open proposal count", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		ctx := context.Background()

		empty, err := svc.ListCoordinators(ctx, workspaceID)
		if err != nil {
			t.Fatalf("ListCoordinators() unexpected error: %v", err)
		}
		if empty == nil || len(empty) != 0 {
			t.Fatalf("ListCoordinators() = %#v, want empty non-nil slice", empty)
		}

		created, err := svc.CreateCoordinator(ctx, workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		if err := svc.store.InsertProposal(ctx, &Proposal{
			CoordinatorID: created.ID, WorkspaceID: workspaceID,
			Spec: ProposalSpec{Title: "t", WorkflowID: "wf", StepID: "step", RepositoryID: "repo"},
		}); err != nil {
			t.Fatalf("InsertProposal() unexpected error: %v", err)
		}

		items, err := svc.ListCoordinators(ctx, workspaceID)
		if err != nil {
			t.Fatalf("ListCoordinators() unexpected error: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("ListCoordinators() len = %d, want 1", len(items))
		}
		if items[0].Coordinator.ID != created.ID {
			t.Errorf("ListCoordinators()[0].Coordinator.ID = %q, want %q", items[0].Coordinator.ID, created.ID)
		}
		if items[0].OpenProposals != 1 {
			t.Errorf("ListCoordinators()[0].OpenProposals = %d, want 1", items[0].OpenProposals)
		}
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, agents, executors, wantErr)
		_, err := svc.ListCoordinators(context.Background(), workspaceID)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ListCoordinators() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceRead)
	})
}

func TestServicePatchCoordinator(t *testing.T) {
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{
		"ap-1": {ID: "ap-1", WorkspaceID: workspaceID},
		"ap-2": {ID: "ap-2", WorkspaceID: workspaceID},
	}
	executors := map[string]*taskmodels.ExecutorProfile{
		"ep-1": {ID: "ep-1"},
		"ep-2": {ID: "ep-2"},
	}

	newCoordinator := func(t *testing.T, svc *Service) *Coordinator {
		t.Helper()
		created, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1", Context: "context",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		return created
	}

	t.Run("updates only the sent fields", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created := newCoordinator(t, svc)
		updated, err := svc.PatchCoordinator(context.Background(), workspaceID, created.ID, PatchCoordinatorRequest{
			"name": []byte(`"New name"`),
		})
		if err != nil {
			t.Fatalf("PatchCoordinator() unexpected error: %v", err)
		}
		if updated.Name != "New name" {
			t.Errorf("PatchCoordinator() name = %q, want %q", updated.Name, "New name")
		}
		if updated.Context != "context" {
			t.Errorf("PatchCoordinator() context = %q, want unchanged %q", updated.Context, "context")
		}
		if updated.AgentProfileID != "ap-1" {
			t.Errorf("PatchCoordinator() agent_profile_id = %q, want unchanged %q", updated.AgentProfileID, "ap-1")
		}
	})

	t.Run("a context change clears conversation_task_id and calls the hook", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		conversationTaskID := "task-1"
		seed := &Coordinator{
			WorkspaceID: workspaceID, Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
			Context: "context", ConversationTaskID: &conversationTaskID,
		}
		if err := svc.store.CreateCoordinator(context.Background(), seed); err != nil {
			t.Fatalf("seed CreateCoordinator() unexpected error: %v", err)
		}

		var clearedCoordinatorID, clearedTaskID string
		var hookCalls int
		svc.SetConversationHooks(func(_ context.Context, coordinatorID, oldConversationTaskID string) {
			hookCalls++
			clearedCoordinatorID = coordinatorID
			clearedTaskID = oldConversationTaskID
		}, nil)

		updated, err := svc.PatchCoordinator(context.Background(), workspaceID, seed.ID, PatchCoordinatorRequest{
			"context": []byte(`"new context"`),
		})
		if err != nil {
			t.Fatalf("PatchCoordinator() unexpected error: %v", err)
		}
		if updated.ConversationTaskID != nil {
			t.Errorf("PatchCoordinator() conversation_task_id = %v, want nil", *updated.ConversationTaskID)
		}
		if hookCalls != 1 {
			t.Fatalf("hook calls = %d, want 1", hookCalls)
		}
		if clearedCoordinatorID != seed.ID || clearedTaskID != conversationTaskID {
			t.Errorf("hook called with (%q, %q), want (%q, %q)", clearedCoordinatorID, clearedTaskID, seed.ID, conversationTaskID)
		}
	})

	t.Run("a null field is a FieldError naming the field", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created := newCoordinator(t, svc)
		_, err := svc.PatchCoordinator(context.Background(), workspaceID, created.ID, PatchCoordinatorRequest{
			"name": []byte("null"),
		})
		assertFieldError(t, err, "name")
	})

	t.Run("unknown fields are ignored", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created := newCoordinator(t, svc)
		updated, err := svc.PatchCoordinator(context.Background(), workspaceID, created.ID, PatchCoordinatorRequest{
			"unknown_field": []byte(`"value"`),
		})
		if err != nil {
			t.Fatalf("PatchCoordinator() unexpected error: %v", err)
		}
		if updated.Name != "Coordinator" {
			t.Errorf("PatchCoordinator() name = %q, want unchanged %q", updated.Name, "Coordinator")
		}
	})

	t.Run("an invalid agent profile is a FieldError naming agent_profile_id", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created := newCoordinator(t, svc)
		_, err := svc.PatchCoordinator(context.Background(), workspaceID, created.ID, PatchCoordinatorRequest{
			"agent_profile_id": []byte(`"does-not-exist"`),
		})
		assertFieldError(t, err, "agent_profile_id")
	})

	t.Run("not found is ErrNotFound", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		_, err := svc.PatchCoordinator(context.Background(), workspaceID, "does-not-exist", PatchCoordinatorRequest{
			"name": []byte(`"New name"`),
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("PatchCoordinator() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, agents, executors, wantErr)
		_, err := svc.PatchCoordinator(context.Background(), workspaceID, "any", PatchCoordinatorRequest{
			"name": []byte(`"New name"`),
		})
		if !errors.Is(err, wantErr) {
			t.Fatalf("PatchCoordinator() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceManage)
	})
}

func TestServiceDeleteCoordinator(t *testing.T) {
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("deletes the coordinator and calls the hook", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		created, err := svc.CreateCoordinator(context.Background(), workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		var deletedID string
		svc.SetConversationHooks(nil, func(_ context.Context, coordinatorID string) {
			deletedID = coordinatorID
		})

		if err := svc.DeleteCoordinator(context.Background(), workspaceID, created.ID); err != nil {
			t.Fatalf("DeleteCoordinator() unexpected error: %v", err)
		}
		if deletedID != created.ID {
			t.Errorf("delete hook called with %q, want %q", deletedID, created.ID)
		}
		_, _, _, err = svc.GetCoordinator(context.Background(), workspaceID, created.ID)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetCoordinator() after delete error = %v, want ErrNotFound", err)
		}
	})

	t.Run("not found is ErrNotFound", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		err := svc.DeleteCoordinator(context.Background(), workspaceID, "does-not-exist")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("DeleteCoordinator() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, agents, executors, wantErr)
		err := svc.DeleteCoordinator(context.Background(), workspaceID, "any")
		if !errors.Is(err, wantErr) {
			t.Fatalf("DeleteCoordinator() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceManage)
	})
}

func TestServiceProposalReads(t *testing.T) {
	const workspaceID = "ws-1"
	agents := map[string]*settingsmodels.AgentProfile{"ap-1": {ID: "ap-1", WorkspaceID: workspaceID}}
	executors := map[string]*taskmodels.ExecutorProfile{"ep-1": {ID: "ep-1"}}

	t.Run("lists and gets proposals scoped to the coordinator", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		ctx := context.Background()
		created, err := svc.CreateCoordinator(ctx, workspaceID, CreateCoordinatorRequest{
			Name: "Coordinator", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		proposal := &Proposal{
			CoordinatorID: created.ID, WorkspaceID: workspaceID,
			Spec: ProposalSpec{Title: "t", WorkflowID: "wf", StepID: "step", RepositoryID: "repo"},
		}
		if err := svc.store.InsertProposal(ctx, proposal); err != nil {
			t.Fatalf("InsertProposal() unexpected error: %v", err)
		}

		pending, err := svc.ListProposals(ctx, workspaceID, created.ID, ListProposalsPending)
		if err != nil {
			t.Fatalf("ListProposals() unexpected error: %v", err)
		}
		if len(pending) != 1 || pending[0].ID != proposal.ID {
			t.Fatalf("ListProposals(pending) = %#v, want [proposal]", pending)
		}

		found, err := svc.GetProposal(ctx, workspaceID, created.ID, proposal.ID)
		if err != nil {
			t.Fatalf("GetProposal() unexpected error: %v", err)
		}
		if found.ID != proposal.ID {
			t.Errorf("GetProposal() id = %q, want %q", found.ID, proposal.ID)
		}
	})

	t.Run("a proposal of another coordinator is not found", func(t *testing.T) {
		svc := newServiceForTest(t, agents, executors, nil)
		ctx := context.Background()
		a, err := svc.CreateCoordinator(ctx, workspaceID, CreateCoordinatorRequest{
			Name: "A", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		b, err := svc.CreateCoordinator(ctx, workspaceID, CreateCoordinatorRequest{
			Name: "B", AgentProfileID: "ap-1", ExecutorProfileID: "ep-1",
		})
		if err != nil {
			t.Fatalf("CreateCoordinator() unexpected error: %v", err)
		}
		proposal := &Proposal{
			CoordinatorID: a.ID, WorkspaceID: workspaceID,
			Spec: ProposalSpec{Title: "t", WorkflowID: "wf", StepID: "step", RepositoryID: "repo"},
		}
		if err := svc.store.InsertProposal(ctx, proposal); err != nil {
			t.Fatalf("InsertProposal() unexpected error: %v", err)
		}
		_, err = svc.GetProposal(ctx, workspaceID, b.ID, proposal.ID)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetProposal() error = %v, want ErrNotFound", err)
		}
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, agents, executors, wantErr)
		if _, err := svc.ListProposals(context.Background(), workspaceID, "cid", ListProposalsPending); !errors.Is(err, wantErr) {
			t.Fatalf("ListProposals() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceRead)
		if _, err := svc.GetProposal(context.Background(), workspaceID, "cid", "pid"); !errors.Is(err, wantErr) {
			t.Fatalf("GetProposal() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceRead)
	})
}

func TestServiceListStalls(t *testing.T) {
	const workspaceID = "ws-1"

	t.Run("never nil", func(t *testing.T) {
		svc := newServiceForTest(t, nil, nil, nil)
		stalls, err := svc.ListStalls(context.Background(), workspaceID)
		if err != nil {
			t.Fatalf("ListStalls() unexpected error: %v", err)
		}
		if stalls == nil || len(stalls) != 0 {
			t.Fatalf("ListStalls() = %#v, want empty non-nil slice", stalls)
		}
	})

	t.Run("returns upserted stalls ordered by task_id", func(t *testing.T) {
		svc := newServiceForTest(t, nil, nil, nil)
		ctx := context.Background()
		now := svc.store.now()
		for _, taskID := range []string{"task-b", "task-a"} {
			if _, err := svc.store.UpsertStall(ctx, &Stall{
				TaskID: taskID, WorkspaceID: workspaceID, StalledForMs: 1000, LastEventAt: now, DetectedAt: now,
			}); err != nil {
				t.Fatalf("UpsertStall() unexpected error: %v", err)
			}
		}
		stalls, err := svc.ListStalls(ctx, workspaceID)
		if err != nil {
			t.Fatalf("ListStalls() unexpected error: %v", err)
		}
		if len(stalls) != 2 || stalls[0].TaskID != "task-a" || stalls[1].TaskID != "task-b" {
			t.Fatalf("ListStalls() = %#v, want [task-a, task-b]", stalls)
		}
	})

	t.Run("propagates a workspace authorization failure", func(t *testing.T) {
		wantErr := errors.New("boom")
		svc := newServiceForTest(t, nil, nil, wantErr)
		_, err := svc.ListStalls(context.Background(), workspaceID)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ListStalls() error = %v, want %v", err, wantErr)
		}
		assertLastScope(t, svc, authz.ScopeWorkspaceRead)
	})
}
